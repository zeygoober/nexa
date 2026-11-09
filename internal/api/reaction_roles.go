package api

import (
	"errors"
	"net/http"

	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// reactionRoleUnsafe is what a role given to anyone who reacts may not carry:
// anything over other people or the server itself. Being able to see or say
// more is what such roles are for; running the place is not.
const reactionRoleUnsafe = perms.Administrator | perms.ManageServer | perms.ManageRoles | perms.ManageChannels |
	perms.KickMembers | perms.BanMembers | perms.ModerateMembers | perms.ManageMessages | perms.ManageWebhooks |
	perms.MentionEveryone | perms.ManageNicknames | perms.ManageThreads | perms.MoveMembers | perms.MuteMembers |
	perms.DeafenMembers | perms.ViewAuditLog

func (s *Server) reactionRoleRoutes(mux *http.ServeMux) {
	const path = "/api/v1/channels/{id}/messages/{mid}/reaction-roles/{emoji}"
	mux.Handle("PUT "+path, s.userLimited(s.limManage, s.handleReactionRoleSet))
	mux.Handle("DELETE "+path, s.userLimited(s.limManage, s.handleReactionRoleRemove))
}

// reactionRoleManager finds the message for someone setting what its reactions
// give, which is for those who manage roles in its server.
func (s *Server) reactionRoleManager(w http.ResponseWriter, r *http.Request) (*store.Server, *store.Channel, *store.User, *store.Message, string) {
	ch, me, m, emoji := s.reactionTarget(w, r, NeedHistory)
	if ch == nil {
		return nil, nil, nil, nil, ""
	}
	if ch.ServerID == nil {
		WriteError(w, E(http.StatusBadRequest, "not_in_server", "Only messages in a server can give roles."))
		return nil, nil, nil, nil, ""
	}
	srv, err := s.Store.ServerByID(r.Context(), *ch.ServerID)
	if err != nil {
		WriteError(w, err)
		return nil, nil, nil, nil, ""
	}
	p, err := s.Store.ServerPerms(r.Context(), srv.ID, me.ID)
	if err != nil {
		WriteError(w, err)
		return nil, nil, nil, nil, ""
	}
	if !p.Has(perms.ManageRoles) {
		WriteError(w, E(http.StatusForbidden, "missing_permission", "You do not have permission to do that."))
		return nil, nil, nil, nil, ""
	}
	return srv, ch, me, m, emoji
}

func (s *Server) emitReactionRoles(r *http.Request, ch *store.Channel, m *store.Message) []store.ReactionRole {
	list, err := s.Store.ReactionRolesOf(r.Context(), m.ID)
	if err != nil {
		list = []store.ReactionRole{}
	}
	s.Bcast.EmitChannel(ch.ID, "reaction_roles_update", map[string]any{"channel_id": ch.ID, "message_id": m.ID, "reaction_roles": list})
	return list
}

func (s *Server) handleReactionRoleSet(w http.ResponseWriter, r *http.Request) {
	srv, ch, me, m, emoji := s.reactionRoleManager(w, r)
	if srv == nil {
		return
	}
	var req struct {
		RoleID ids.ID `json:"role_id"`
	}
	if err := ReadJSON(w, r, &req, 1<<10); err != nil {
		WriteError(w, err)
		return
	}
	role, err := s.Store.RoleByID(r.Context(), req.RoleID)
	if err != nil || role.ServerID != srv.ID {
		WriteError(w, E(http.StatusNotFound, "unknown_role", "That role does not exist."))
		return
	}
	if role.ID == srv.ID || role.ManagedBy != nil {
		WriteError(w, E(http.StatusBadRequest, "role_not_assignable", "That role cannot be given by a reaction."))
		return
	}
	if role.Permissions&reactionRoleUnsafe != 0 {
		WriteError(w, errReactionRoleUnsafe)
		return
	}
	highest, isOwner := s.actorHighest(r.Context(), srv, me.ID)
	if !perms.CanManageRole(highest, isOwner, perms.Role{ID: role.ID, Position: role.Position}) {
		WriteError(w, E(http.StatusForbidden, "role_hierarchy", "You can only assign roles below your highest role."))
		return
	}
	if !s.mayAddReaction(w, r, ch, me, m, emoji) {
		return
	}
	// the setter's own reaction puts the emoji on the message for others to
	// press, and gives them nothing: the mapping does not exist yet
	added, err := s.Store.AddReaction(r.Context(), m.ID, me.ID, emoji)
	if err != nil {
		if errors.Is(err, store.ErrLimit) {
			WriteError(w, E(http.StatusBadRequest, "too_many_reactions", "A message can have at most 20 different reactions."))
			return
		}
		WriteError(w, err)
		return
	}
	if added {
		s.Bcast.EmitChannel(ch.ID, "reaction_add", map[string]any{"channel_id": ch.ID, "message_id": m.ID, "user_id": me.ID, "emoji": emoji})
	}
	if err := s.Store.SetReactionRole(r.Context(), srv.ID, m.ID, emoji, role.ID); err != nil {
		WriteError(w, err)
		return
	}
	s.audit(r, srv.ID, me.ID, "reaction_role_set", "role", &role.ID, map[string]any{"message": m.ID, "emoji": emoji}, "")
	WriteJSON(w, http.StatusOK, map[string]any{"reaction_roles": s.emitReactionRoles(r, ch, m)})
}

func (s *Server) handleReactionRoleRemove(w http.ResponseWriter, r *http.Request) {
	srv, ch, me, m, emoji := s.reactionRoleManager(w, r)
	if srv == nil {
		return
	}
	roleID, err := s.Store.RemoveReactionRole(r.Context(), m.ID, emoji)
	if err != nil {
		WriteError(w, err)
		return
	}
	s.audit(r, srv.ID, me.ID, "reaction_role_remove", "role", &roleID, map[string]any{"message": m.ID, "emoji": emoji}, "")
	WriteJSON(w, http.StatusOK, map[string]any{"reaction_roles": s.emitReactionRoles(r, ch, m)})
}

var errReactionRoleUnsafe = E(http.StatusForbidden, "reaction_role_unsafe", "A role with moderation or management permissions cannot be given by a reaction.")

// reactionRoleOf is the role this reaction would give or take away, nil when
// there is none or nothing would change. A false answer has already refused
// the request, before the reaction changes.
func (s *Server) reactionRoleOf(w http.ResponseWriter, r *http.Request, ch *store.Channel, me *store.User, m *store.Message, emoji string, add bool) (*store.Role, bool) {
	if ch.ServerID == nil || me.Flags&store.FlagBot != 0 {
		return nil, true
	}
	var roleID ids.ID
	for _, rr := range m.ReactionRoles {
		if rr.Emoji == emoji {
			roleID = rr.RoleID
		}
	}
	if roleID == 0 {
		return nil, true
	}
	for _, sum := range m.Reactions {
		if sum.Emoji == emoji && sum.Me == add {
			return nil, true
		}
	}
	if !add && !hasOwnReaction(m, emoji) {
		return nil, true
	}
	role, err := s.Store.RoleByID(r.Context(), roleID)
	if err != nil {
		return nil, true
	}
	// the role may have gained these since the mapping was made
	if role.Permissions&reactionRoleUnsafe != 0 {
		if add {
			WriteError(w, errReactionRoleUnsafe)
			return nil, false
		}
		return nil, true
	}
	if ok, retry := s.limReactRole.Allow(me.ID.String()); !ok {
		w.Header().Set("Retry-After", retryAfter(retry))
		WriteError(w, ErrRateLimited)
		return nil, false
	}
	return role, true
}

func hasOwnReaction(m *store.Message, emoji string) bool {
	for _, sum := range m.Reactions {
		if sum.Emoji == emoji {
			return sum.Me
		}
	}
	return false
}

// setRoleByReaction gives or takes the role. The reaction has already changed
// and stays changed whatever happens here. Only this person's view is looked
// at: a reaction is a small thing, and the whole server's would be far too much
// work for one.
func (s *Server) setRoleByReaction(r *http.Request, ch *store.Channel, me *store.User, role *store.Role, add bool) {
	if role == nil {
		return
	}
	ctx := r.Context()
	serverID := *ch.ServerID
	member, err := s.Store.Member(ctx, serverID, me.ID)
	if err != nil {
		return
	}
	has := false
	for _, id := range member.RoleIDs {
		has = has || id == role.ID
	}
	if has == add {
		return
	}
	seen := func() (map[ids.ID]bool, error) {
		vis, err := s.Store.ViewableServerChannels(ctx, serverID, me.ID)
		if err != nil {
			return nil, err
		}
		set := make(map[ids.ID]bool, len(vis))
		for id := range vis {
			set[id] = true
		}
		return set, nil
	}
	before, err := seen()
	if err != nil {
		s.Log.Warn("reaction role", "server", serverID.String(), "err", err)
		return
	}
	action := "member_role_add"
	if add {
		err = s.Store.AddMemberRole(ctx, serverID, me.ID, role.ID)
	} else {
		action = "member_role_remove"
		err = s.Store.RemoveMemberRole(ctx, serverID, me.ID, role.ID)
	}
	if err != nil {
		s.Log.Warn("reaction role", "server", serverID.String(), "err", err)
		return
	}
	s.audit(r, serverID, me.ID, action, "user", &me.ID, map[string]any{"role": role.ID, "via": "reaction"}, "")
	if updated, err := s.Store.Member(ctx, serverID, me.ID); err == nil {
		s.emitServer(ctx, serverID, "member_update", updated)
	}
	if after, err := seen(); err == nil {
		s.applyVisibility(ctx, serverID, visibility{me.ID: before}, visibility{me.ID: after}, s.leaveRotates, nil)
	}
	s.recheckSFUServer(ctx, serverID)
	if !add {
		s.RecheckTagWearers(ctx, serverID, []ids.ID{me.ID})
	}
}
