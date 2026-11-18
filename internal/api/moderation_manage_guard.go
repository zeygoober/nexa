package api

import (
	"context"
	"net/http"

	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// manageWorld is what decides who can see what in a server, read once per
// action: which channels are private (for the view and a transfer's notice)
// and who may not be handed the server.
type manageWorld struct {
	everyone ids.ID
	roles    []perms.Role
	channels []*worldChannel
	// subjects are the accounts a transfer may not go to: the owner acting,
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

func (w *manageWorld) channel(id ids.ID) *worldChannel {
	for _, c := range w.channels {
		if c.id == id {
			return c
		}
	}
	return nil
}

// isPrivate says whether someone with no roles cannot see a channel.
func (w *manageWorld) isPrivate(c *worldChannel) bool {
	return !perms.Compute(w.everyone, w.roles, perms.Member{}, false, c.overwrites).Has(perms.ViewChannel)
}

var errManageOpensAccess = E(http.StatusForbidden, "owner_access_refused", "That would open messages to you, to staff, to an app or to anyone who joins, so it is not allowed here.")
