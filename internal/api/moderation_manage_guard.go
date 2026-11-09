package api

import (
	"context"
	"net/http"

	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// What the owner's server management must never hand out. A channel's
// messages are read with View Channel and Read Message History (and Connect
// for a voice channel's chat); Manage Roles in a channel gives the rest of
// these. On the server, the rest are what would let an account take the same
// steps through the ordinary routes.
const (
	guardChannelBits = perms.ViewChannel | perms.ReadMessageHistory | perms.Connect | perms.ManageRoles
	guardServerBits  = perms.Administrator | perms.ManageRoles | perms.ManageServer | perms.ViewAuditLog
)

// manageWorld is what decides who can do what in a server, in memory, so a
// change can be tried on it before it is made.
type manageWorld struct {
	everyone ids.ID
	roles    []perms.Role
	channels []*worldChannel
	// subjects are the accounts that must not gain anything: the owner acting,
	// members holding a staff rank, apps, accounts sharing an address with the
	// owner, and an account with no roles at all, which stands for anyone who
	// joins later (so also for the owner, by Join Server).
	subjects []perms.Member
	// privileged is the same people by id, without the one with no roles.
	privileged map[ids.ID]bool
}

type worldChannel struct {
	id         ids.ID
	category   bool
	parent     *ids.ID
	overwrites []perms.Overwrite
}

func toPermOverwrites(list []store.Overwrite) []perms.Overwrite {
	out := make([]perms.Overwrite, len(list))
	for i, o := range list {
		out[i] = perms.Overwrite{TargetID: o.TargetID, Kind: o.TargetType, Allow: o.Allow, Deny: o.Deny}
	}
	return out
}

func (s *Server) loadManageWorld(ctx context.Context, srv *store.Server, me ids.ID) (*manageWorld, error) {
	roles, err := s.Store.Roles(ctx, srv.ID)
	if err != nil {
		return nil, err
	}
	chans, err := s.Store.ServerChannels(ctx, srv.ID)
	if err != nil {
		return nil, err
	}
	w := &manageWorld{everyone: srv.ID}
	for _, ro := range roles {
		w.roles = append(w.roles, perms.Role{ID: ro.ID, Position: ro.Position, Permissions: ro.Permissions})
	}
	for _, c := range chans {
		w.channels = append(w.channels, &worldChannel{id: c.ID, category: c.Type == "category", parent: c.ParentID, overwrites: toPermOverwrites(c.Overwrites)})
	}
	alts, err := s.Store.AltAccounts(ctx, me)
	if err != nil {
		return nil, err
	}
	also := []ids.ID{me}
	for _, a := range alts {
		also = append(also, a.UserID)
	}
	if w.privileged, err = s.Store.PrivilegedMembers(ctx, srv.ID, also); err != nil {
		return nil, err
	}
	// never a member yet: the owner counts as someone holding no roles
	w.subjects = []perms.Member{{UserID: 0}, {UserID: me}}
	for id := range w.privileged {
		if id == me {
			continue
		}
		w.subjects = append(w.subjects, perms.Member{UserID: id})
	}
	for i := range w.subjects {
		if w.subjects[i].UserID.IsZero() {
			continue
		}
		if m, err := s.Store.Member(ctx, srv.ID, w.subjects[i].UserID); err == nil {
			w.subjects[i].RoleIDs = append([]ids.ID{}, m.RoleIDs...)
		}
	}
	return w, nil
}

func (w *manageWorld) clone() *manageWorld {
	out := &manageWorld{everyone: w.everyone, privileged: w.privileged}
	out.roles = append([]perms.Role{}, w.roles...)
	for _, c := range w.channels {
		cc := *c
		cc.overwrites = append([]perms.Overwrite{}, c.overwrites...)
		out.channels = append(out.channels, &cc)
	}
	for _, m := range w.subjects {
		mm := m
		mm.RoleIDs = append([]ids.ID{}, m.RoleIDs...)
		out.subjects = append(out.subjects, mm)
	}
	return out
}

func (w *manageWorld) channel(id ids.ID) *worldChannel {
	for _, c := range w.channels {
		if c.id == id {
			return c
		}
	}
	return nil
}

func sameWorldOverwrites(a, b []perms.Overwrite) bool {
	if len(a) != len(b) {
		return false
	}
	byTarget := make(map[ids.ID]perms.Overwrite, len(a))
	for _, o := range a {
		byTarget[o.TargetID] = o
	}
	for _, o := range b {
		if x, ok := byTarget[o.TargetID]; !ok || x != o {
			return false
		}
	}
	return true
}

func withoutOverwrite(list []perms.Overwrite, target ids.ID) []perms.Overwrite {
	out := make([]perms.Overwrite, 0, len(list))
	for _, o := range list {
		if o.TargetID != target {
			out = append(out, o)
		}
	}
	return out
}

// synced lists a category's channels that follow it, before it changes.
func (w *manageWorld) synced(cat *worldChannel) []*worldChannel {
	if !cat.category {
		return nil
	}
	var out []*worldChannel
	for _, c := range w.channels {
		if c.parent != nil && *c.parent == cat.id && sameWorldOverwrites(c.overwrites, cat.overwrites) {
			out = append(out, c)
		}
	}
	return out
}

func (w *manageWorld) setOverwrite(id ids.ID, o perms.Overwrite) {
	c := w.channel(id)
	if c == nil {
		return
	}
	for _, x := range append(w.synced(c), c) {
		x.overwrites = append(withoutOverwrite(x.overwrites, o.TargetID), o)
	}
}

func (w *manageWorld) removeOverwrite(id, target ids.ID) {
	c := w.channel(id)
	if c == nil {
		return
	}
	for _, x := range append(w.synced(c), c) {
		x.overwrites = withoutOverwrite(x.overwrites, target)
	}
}

// takeCategory gives a channel its category's overwrites.
func (w *manageWorld) takeCategory(id, category ids.ID) {
	c, cat := w.channel(id), w.channel(category)
	if c == nil || cat == nil {
		return
	}
	c.overwrites = append([]perms.Overwrite{}, cat.overwrites...)
}

func (w *manageWorld) setRolePerms(id ids.ID, p perms.Perm) {
	for i := range w.roles {
		if w.roles[i].ID == id {
			w.roles[i].Permissions = p
		}
	}
}

// deleteRole also takes the role away from whoever held it, which can open a
// channel: an overwrite denying that role goes with it.
func (w *manageWorld) deleteRole(id ids.ID) {
	kept := w.roles[:0:0]
	for _, ro := range w.roles {
		if ro.ID != id {
			kept = append(kept, ro)
		}
	}
	w.roles = kept
	for _, c := range w.channels {
		c.overwrites = withoutOverwrite(c.overwrites, id)
	}
	for i := range w.subjects {
		w.subjects[i].RoleIDs = withoutID(w.subjects[i].RoleIDs, id)
	}
}

func (w *manageWorld) memberRole(uid, role ids.ID, add bool) {
	for i := range w.subjects {
		if w.subjects[i].UserID != uid {
			continue
		}
		w.subjects[i].RoleIDs = withoutID(w.subjects[i].RoleIDs, role)
		if add {
			w.subjects[i].RoleIDs = append(w.subjects[i].RoleIDs, role)
		}
	}
}

// gained says whether anyone guarded holds something in after that they did
// not hold in w, in a server or a channel that already existed. A channel
// made by the change holds no messages to reach.
func (w *manageWorld) gained(after *manageWorld) bool {
	for i, m := range w.subjects {
		now := after.subjects[i]
		was := perms.Compute(w.everyone, w.roles, m, false, nil)
		is := perms.Compute(after.everyone, after.roles, now, false, nil)
		if is&^was&guardServerBits != 0 {
			return true
		}
		for _, c := range w.channels {
			ac := after.channel(c.id)
			if ac == nil {
				continue
			}
			was := perms.Compute(w.everyone, w.roles, m, false, c.overwrites)
			is := perms.Compute(after.everyone, after.roles, now, false, ac.overwrites)
			if is&^was&guardChannelBits != 0 {
				return true
			}
		}
	}
	return false
}

// isPrivate says whether someone with no roles cannot see a channel.
func (w *manageWorld) isPrivate(c *worldChannel) bool {
	return !perms.Compute(w.everyone, w.roles, perms.Member{}, false, c.overwrites).Has(perms.ViewChannel)
}

var errManageOpensAccess = E(http.StatusForbidden, "owner_access_refused", "That would open messages to you, to staff, to an app or to anyone who joins, so it is not allowed here.")
