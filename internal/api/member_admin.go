package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// The Members page and Mod View: every member with when they came, how old
// their account is, how they joined and what might want a look, for the
// people who look after a server; pruning the ones long gone; banning many
// at once. And each member's own picture and banner in one server.

// membersPagePerms are the permissions that open the Members page, any one of them.
const membersPagePerms = perms.ManageServer | perms.ManageRoles | perms.ManageNicknames | perms.KickMembers | perms.BanMembers | perms.ModerateMembers

const maxBulkBan = 200

func (s *Server) memberAdminRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/servers/{id}/member-search", s.authed(s.handleMemberSearch))
	mux.Handle("GET /api/v1/servers/{id}/members/{uid}/mod", s.authed(s.handleModView))
	mux.Handle("GET /api/v1/servers/{id}/prune", s.authed(s.handlePrunePreview))
	mux.Handle("POST /api/v1/servers/{id}/prune", s.userLimited(s.limManage, s.handlePrune))
	mux.Handle("POST /api/v1/servers/{id}/bulk-ban", s.userLimited(s.limManage, s.handleBulkBan))
	mux.Handle("DELETE /api/v1/servers/{id}/invites", s.userLimited(s.limManage, s.handleRevokeAllInvites))
	mux.Handle("PUT /api/v1/servers/{id}/members/{uid}/avatar", s.userLimited(s.limUpload, s.handleMemberImage(false)))
	mux.Handle("PUT /api/v1/servers/{id}/members/{uid}/banner", s.userLimited(s.limUpload, s.handleMemberImage(true)))
	mux.Handle("DELETE /api/v1/servers/{id}/members/{uid}/avatar", s.authed(s.handleMemberImageDelete(false)))
	mux.Handle("DELETE /api/v1/servers/{id}/members/{uid}/banner", s.authed(s.handleMemberImageDelete(true)))
	mux.Handle("PUT /api/v1/servers/{id}/roles/{rid}/icon", s.userLimited(s.limUpload, s.handleRoleIcon))
	mux.Handle("DELETE /api/v1/servers/{id}/roles/{rid}/icon", s.authed(s.handleRoleIconDelete))
}

// membersPageCtx loads the server for someone who may open its Members page.
func (s *Server) membersPageCtx(w http.ResponseWriter, r *http.Request) (*store.Server, perms.Perm) {
	srv, _, p := s.serverCtx(w, r, 0)
	if srv == nil {
		return nil, 0
	}
	if p&membersPagePerms == 0 {
		WriteError(w, E(http.StatusForbidden, "missing_permission", "You need a moderator's permissions to see this."))
		return nil, 0
	}
	return srv, p
}

func idList(raw string) []ids.ID {
	var out []ids.ID
	for _, part := range strings.Split(raw, ",") {
		if id, err := ids.Parse(strings.TrimSpace(part)); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func (s *Server) handleMemberSearch(w http.ResponseWriter, r *http.Request) {
	srv, p := s.membersPageCtx(w, r)
	if srv == nil {
		return
	}
	q := r.URL.Query()
	num := func(k string) int64 {
		n, _ := strconv.ParseInt(q.Get(k), 10, 64)
		return n
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	mq := store.MemberQuery{Query: q.Get("q"), Roles: idList(q.Get("roles")), JoinedAfter: num("joined_after"), JoinedBefore: num("joined_before"),
		CreatedAfter: num("created_after"), CreatedBefore: num("created_before"), Sort: q.Get("sort"), Offset: offset, Limit: limit}
	if sig := q.Get("signals"); sig != "" {
		mq.Signals = strings.Split(sig, ",")
	}
	// how people joined, and with which invite, is for those who see the invites
	if p.Has(perms.ManageServer) {
		mq.JoinMethod, mq.JoinCode = q.Get("join_method"), q.Get("join_code")
	}
	rows, total, err := s.Store.SearchMembers(r.Context(), srv.ID, mq)
	if err != nil {
		WriteError(w, err)
		return
	}
	who := make([]ids.ID, 0, len(rows))
	for _, row := range rows {
		who = append(who, row.UserID)
		if p.Has(perms.ManageServer) {
			j := row.Member.Join()
			row.Join = &j
			if j.InvitedBy != nil {
				who = append(who, *j.InvitedBy)
			}
		}
	}
	resp := map[string]any{"members": rows, "total": total, "users": s.publicUsers(r.Context(), who)}
	if p.Has(perms.ManageServer) {
		if codes, err := s.Store.JoinCodes(r.Context(), srv.ID); err == nil {
			resp["join_codes"] = codes
		}
	}
	WriteJSON(w, http.StatusOK, resp)
}

// handleModView is one member as Mod View shows them: their row, what they
// have said in the server, when they were last about, what the moderators
// and AutoMod have done about them.
func (s *Server) handleModView(w http.ResponseWriter, r *http.Request) {
	srv, p := s.membersPageCtx(w, r)
	if srv == nil {
		return
	}
	uid, err := ids.Parse(r.PathValue("uid"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	ctx := r.Context()
	d, err := s.Store.MemberDetailOf(ctx, srv.ID, uid)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	who := []ids.ID{uid}
	if p.Has(perms.ManageServer) {
		j := d.Member.Join()
		d.Join = &j
		if j.InvitedBy != nil {
			who = append(who, *j.InvitedBy)
		}
	}
	resp := map[string]any{"member": d}
	if p.Has(perms.ViewAuditLog) {
		if entries, err := s.Store.AuditEntriesFor(ctx, srv.ID, store.AuditFilter{Target: &uid}, 0, 25); err == nil {
			resp["audit"] = entries
			for _, e := range entries {
				who = append(who, e.ActorID)
			}
		}
		if events, err := s.Store.AutomodEvents(ctx, srv.ID, &uid, 0, 25); err == nil {
			resp["automod"] = events
		}
	}
	resp["users"] = s.publicUsers(ctx, who)
	WriteJSON(w, http.StatusOK, resp)
}

// pruneParams reads how long people have to have been gone, and which roles
// do not keep them in.
func pruneParams(days int, roles []ids.ID) (int, []ids.ID, bool) {
	if days < 1 || days > 365 {
		return 0, nil, false
	}
	return days, roles, true
}

func (s *Server) handlePrunePreview(w http.ResponseWriter, r *http.Request) {
	srv, _, _ := s.serverCtx(w, r, perms.KickMembers)
	if srv == nil {
		return
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("days"))
	days, roles, ok := pruneParams(n, idList(r.URL.Query().Get("roles")))
	if !ok {
		WriteError(w, E(http.StatusBadRequest, "invalid_days", "Prune members gone between 1 and 365 days."))
		return
	}
	list, err := s.Store.PruneCandidates(r.Context(), srv.ID, srv.OwnerID, days, roles)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"count": len(list)})
}

// handlePrune removes the members nobody has seen for a while who hold no
// role but the ones named: the same as kicking each, in one go.
func (s *Server) handlePrune(w http.ResponseWriter, r *http.Request) {
	srv, me, _ := s.serverCtx(w, r, perms.KickMembers)
	if srv == nil {
		return
	}
	var req struct {
		Days   int      `json:"days"`
		Roles  []ids.ID `json:"roles"`
		Reason string   `json:"reason"`
	}
	if err := ReadJSON(w, r, &req, 16<<10); err != nil {
		WriteError(w, err)
		return
	}
	days, roles, ok := pruneParams(req.Days, req.Roles)
	if !ok {
		WriteError(w, E(http.StatusBadRequest, "invalid_days", "Prune members gone between 1 and 365 days."))
		return
	}
	reason, _ := CleanText(req.Reason, maxBanReason)
	list, err := s.Store.PruneCandidates(r.Context(), srv.ID, srv.OwnerID, days, roles)
	if err != nil {
		WriteError(w, err)
		return
	}
	pruned := 0
	for _, uid := range list {
		if uid == me.ID || !s.canActOn(r.Context(), srv, me.ID, uid) {
			continue
		}
		if err := s.removeMember(r, srv, uid, "member_remove", map[string]any{"reason": "kick"}); err == nil {
			pruned++
		}
	}
	s.audit(r, srv.ID, me.ID, "member_prune", "server", &srv.ID, map[string]any{"days": days, "count": pruned}, reason)
	WriteJSON(w, http.StatusOK, map[string]any{"pruned": pruned})
}

// handleBulkBan bans many people at once: Ban Members and Manage Server both.
func (s *Server) handleBulkBan(w http.ResponseWriter, r *http.Request) {
	srv, me, p := s.serverCtx(w, r, perms.BanMembers)
	if srv == nil {
		return
	}
	if !p.Has(perms.ManageServer) {
		WriteError(w, E(http.StatusForbidden, "missing_permission", "Banning many people at once takes Manage Server as well as Ban Members."))
		return
	}
	var req struct {
		UserIDs           []ids.ID `json:"user_ids"`
		Reason            string   `json:"reason"`
		DeleteMessageDays int      `json:"delete_message_days"`
	}
	if err := ReadJSON(w, r, &req, 64<<10); err != nil {
		WriteError(w, err)
		return
	}
	list := dedupeIDs(req.UserIDs, maxBulkBan+1)
	if len(list) == 0 || len(list) > maxBulkBan {
		WriteError(w, E(http.StatusBadRequest, "bad_ids", "Ban between 1 and 200 people at a time."))
		return
	}
	reason, _ := CleanText(req.Reason, maxBanReason)
	days := min(max(req.DeleteMessageDays, 0), 7)
	ctx := r.Context()
	banned, failed := []ids.ID{}, []ids.ID{}
	for _, uid := range list {
		if uid == me.ID || uid == srv.OwnerID {
			failed = append(failed, uid)
			continue
		}
		if _, err := s.Store.UserByID(ctx, uid); err != nil {
			failed = append(failed, uid)
			continue
		}
		if ok, _ := s.Store.IsMember(ctx, srv.ID, uid); ok && !s.canActOn(ctx, srv, me.ID, uid) {
			failed = append(failed, uid)
			continue
		}
		if err := s.Store.Ban(ctx, &store.Ban{ServerID: srv.ID, UserID: uid, Reason: reason, BannedBy: me.ID}); err != nil {
			failed = append(failed, uid)
			continue
		}
		if ok, _ := s.Store.IsMember(ctx, srv.ID, uid); ok {
			if err := s.removeMember(r, srv, uid, "member_remove", map[string]any{"reason": "ban"}); err != nil && !errors.Is(err, store.ErrNotFound) {
				s.Log.Warn("bulk ban: remove", "err", err)
			}
		}
		target := uid
		s.audit(r, srv.ID, me.ID, "member_ban", "user", &target, map[string]any{"delete_message_days": days, "bulk": true}, reason)
		s.emitServerWithPerm(ctx, srv.ID, perms.BanMembers, "ban_add", map[string]any{"server_id": srv.ID, "user_id": uid, "reason": reason})
		if days > 0 {
			since := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
			if deleted, err := s.Store.DeleteMessagesByUserSince(ctx, srv.ID, uid, since); err == nil {
				for ch, d := range deleted {
					s.Bcast.EmitChannel(ch, "message_delete_bulk", s.bulkDeleteEvent(ctx, ch, d))
				}
			}
		}
		banned = append(banned, uid)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"banned": banned, "failed": failed})
}

// handleRevokeAllInvites revokes every invite the server has at once.
func (s *Server) handleRevokeAllInvites(w http.ResponseWriter, r *http.Request) {
	srv, me, _ := s.serverCtx(w, r, perms.ManageServer)
	if srv == nil {
		return
	}
	codes, err := s.Store.DeleteServerInvites(r.Context(), srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	s.audit(r, srv.ID, me.ID, "invite_delete_all", "server", &srv.ID, map[string]any{"count": len(codes)}, "")
	for _, code := range codes {
		s.emitServerWithPerm(r.Context(), srv.ID, perms.ManageServer, "invite_delete", map[string]any{"server_id": srv.ID, "code": code})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"revoked": len(codes)})
}

// memberImageTarget is whose picture a member image route is about: the
// caller's own (@me), or someone else's for a moderator taking theirs away.
func (s *Server) memberImageTarget(w http.ResponseWriter, r *http.Request, srv *store.Server, me *store.User, p perms.Perm, removing bool) (ids.ID, bool) {
	raw := r.PathValue("uid")
	if raw == "@me" || raw == me.ID.String() {
		return me.ID, true
	}
	uid, err := ids.Parse(raw)
	if err != nil || !removing {
		WriteError(w, E(http.StatusForbidden, "missing_permission", "You can only change your own profile in a server."))
		return 0, false
	}
	if !p.Has(perms.ManageNicknames) || !s.canActOn(r.Context(), srv, me.ID, uid) {
		WriteError(w, E(http.StatusForbidden, "missing_permission", "You need Manage Nicknames to reset someone's server profile."))
		return 0, false
	}
	return uid, true
}

func (s *Server) handleMemberImage(banner bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srv, me, p := s.serverCtx(w, r, 0)
		if srv == nil {
			return
		}
		uid, ok := s.memberImageTarget(w, r, srv, me, p, false)
		if !ok {
			return
		}
		kind := map[bool]string{true: "banner", false: "avatar"}[banner]
		maxW, maxH := avatarSide, avatarSide
		if banner {
			maxW, maxH = userBannerW, userBannerH
		}
		b := s.cropUpload(w, r, kind, maxW, maxH, me.ID)
		if b == nil {
			return
		}
		bid := b.ID
		if err := s.Store.SwapMemberImage(r.Context(), srv.ID, uid, banner, &bid); err != nil {
			s.releaseBlob(r.Context(), b.ID)
			WriteError(w, err)
			return
		}
		s.memberProfileChanged(w, r, srv, uid)
	}
}

func (s *Server) handleMemberImageDelete(banner bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srv, me, p := s.serverCtx(w, r, 0)
		if srv == nil {
			return
		}
		uid, ok := s.memberImageTarget(w, r, srv, me, p, true)
		if !ok {
			return
		}
		if err := s.Store.SwapMemberImage(r.Context(), srv.ID, uid, banner, nil); err != nil {
			WriteError(w, err)
			return
		}
		if uid != me.ID {
			target := uid
			s.audit(r, srv.ID, me.ID, "member_update", "user", &target, map[string]any{map[bool]string{true: "server_banner", false: "server_avatar"}[banner]: nil}, "")
		}
		s.memberProfileChanged(w, r, srv, uid)
	}
}

// memberProfileChanged sends a member's profile in a server to everyone in
// it, and answers with it.
func (s *Server) memberProfileChanged(w http.ResponseWriter, r *http.Request, srv *store.Server, uid ids.ID) {
	updated, err := s.Store.Member(r.Context(), srv.ID, uid)
	if err != nil {
		WriteError(w, err)
		return
	}
	s.emitServer(r.Context(), srv.ID, "member_update", updated)
	WriteJSON(w, http.StatusOK, updated)
}

// handleRoleIcon gives a role a picture of its own, cut here square and
// small (animated ones keep moving).
func (s *Server) handleRoleIcon(w http.ResponseWriter, r *http.Request) {
	srv, me, role, _ := s.roleCtx(w, r)
	if srv == nil {
		return
	}
	if role.ID == srv.ID {
		WriteError(w, E(http.StatusBadRequest, "everyone_role", "@everyone has no icon."))
		return
	}
	b := s.cropUpload(w, r, "icon", roleIconSide, roleIconSide, me.ID)
	if b == nil {
		return
	}
	bid := b.ID
	if err := s.Store.SwapRoleIcon(r.Context(), role.ID, &bid); err != nil {
		s.releaseBlob(r.Context(), b.ID)
		WriteError(w, err)
		return
	}
	// a picture takes the place of an emoji
	empty := ""
	_ = s.Store.UpdateRole(r.Context(), role.ID, store.RolePatch{IconEmoji: &empty})
	s.roleIconChanged(w, r, srv, me, role.ID, "icon")
}

func (s *Server) handleRoleIconDelete(w http.ResponseWriter, r *http.Request) {
	srv, me, role, _ := s.roleCtx(w, r)
	if srv == nil {
		return
	}
	if err := s.Store.SwapRoleIcon(r.Context(), role.ID, nil); err != nil {
		WriteError(w, err)
		return
	}
	s.roleIconChanged(w, r, srv, me, role.ID, "")
}

func (s *Server) roleIconChanged(w http.ResponseWriter, r *http.Request, srv *store.Server, me *store.User, roleID ids.ID, what string) {
	updated, err := s.Store.RoleByID(r.Context(), roleID)
	if err != nil {
		WriteError(w, err)
		return
	}
	s.audit(r, srv.ID, me.ID, "role_update", "role", &roleID, map[string]any{"icon": what != ""}, "")
	s.emitServer(r.Context(), srv.ID, "role_update", updated)
	WriteJSON(w, http.StatusOK, updated)
}
