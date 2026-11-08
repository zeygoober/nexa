package api

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// What only the owner does from the panel (the owner's decision, 2026-11-08):
// reading an account's addresses and sessions, joining a server without an
// invite, previewing a server's layout, and making gift links without paying. Each is written to mod_log, and
// the Privacy Policy, the Terms and ENCRYPTION.md say that they exist.

func (s *Server) modOwnerRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/moderation/users/{id}/account", s.staff(rankOwner, s.handleModAccount))
	mux.Handle("POST /api/v1/moderation/servers/{id}/join", s.staff(rankOwner, s.handleModServerJoin))
	mux.Handle("GET /api/v1/moderation/servers/{id}/preview", s.staff(rankOwner, s.handleModServerPreview))
	mux.Handle("GET /api/v1/moderation/gift-links", s.staff(rankOwner, s.handleModGiftLinks))
	mux.Handle("POST /api/v1/moderation/gift-links", s.staff(rankOwner, s.handleModGiftLinkCreate))
	mux.Handle("DELETE /api/v1/moderation/gift-links/{code}", s.staff(rankOwner, s.handleModGiftLinkRevoke))
}

// handleModAccount answers an account's addresses, sessions and plan. The view
// is logged before anything is read, so nothing can be seen without leaving a
// line in the log. Nexa never asks for an email address, so there is none here.
func (s *Server) handleModAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	u, err := s.Store.UserByID(ctx, id)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	if err := s.Store.AppendModLog(ctx, &store.ModLogEntry{Action: store.LogAccountView, ModeratorID: me.ID, ModeratorUsername: me.Username, TargetID: &u.ID, TargetUsername: u.Username}); err != nil {
		WriteError(w, err)
		return
	}
	sessions, addresses, err := s.Store.SignIns(ctx, u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	type address struct {
		IP          string `json:"ip"`
		FirstSeenAt int64  `json:"first_seen_at"`
		LastSeenAt  int64  `json:"last_seen_at"`
	}
	type session struct {
		UserAgent  string `json:"user_agent"`
		IP         string `json:"ip"`
		CreatedAt  int64  `json:"created_at"`
		LastSeenAt int64  `json:"last_seen_at"`
	}
	out := map[string]any{"created_at": u.CreatedAt, "plan": u.Plan, "addresses": []address{}, "sessions": []session{}, "subscription": nil}
	as := make([]address, 0, len(addresses))
	for _, a := range addresses {
		as = append(as, address{a.IP, a.FirstSeenAt, a.LastSeenAt})
	}
	ss := make([]session, 0, len(sessions))
	for _, x := range sessions {
		ss = append(ss, session{x.UserAgent, x.IP, x.CreatedAt, x.LastSeenAt})
	}
	out["addresses"], out["sessions"] = as, ss
	if b, err := s.Store.BillingFor(ctx, u.ID); err == nil {
		out["subscription"] = map[string]any{"plan": b.Plan, "period": b.Period, "status": b.Status, "period_end": b.PeriodEnd, "cancel_at_end": b.CancelAtEnd}
	} else if !errors.Is(err, store.ErrNotFound) {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, out)
}

// handleModServerJoin makes the owner an ordinary member of a server, as an
// invite would: everyone there sees it, the system channel says so, and keys
// reach them from the members' apps as they do for anyone. A server's bans
// hold; its paused invites do not, since those keep strangers out of a raid.
func (s *Server) handleModServerJoin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	m := s.modServerFor(w, r)
	if m == nil {
		return
	}
	srv, err := s.Store.ServerByID(ctx, m.ID)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	s.forgetCounts(srv.ID)
	log := &store.ModLogEntry{Action: store.LogOwnerJoin, ModeratorID: me.ID, ModeratorUsername: me.Username, TargetID: &srv.OwnerID, Details: mustJSON(map[string]string{"server_name": srv.Name})}
	member, err := s.Store.JoinFromPanel(ctx, srv.ID, me.ID, log)
	switch {
	case errors.Is(err, store.ErrBanned):
		WriteError(w, E(http.StatusForbidden, "banned", "You are banned from this server."))
		return
	case err != nil && !errors.Is(err, store.ErrAlreadyExists):
		WriteError(w, err)
		return
	}
	status := http.StatusOK
	if err == nil {
		status = http.StatusCreated
		s.joined(r, srv, me, member)
	}
	view, users, err := s.serverView(ctx, srv, me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if status == http.StatusCreated {
		s.Bcast.EmitUsers([]ids.ID{me.ID}, "server_create", serverCreateEvent{view, users})
	}
	WriteJSON(w, status, map[string]any{"server": view, "users": users})
}

// previewMembers is how many members a preview lists, oldest accounts first.
const previewMembers = 200

// previewChannel is all a preview says about a channel: never its topic, its
// overwrites or when something was last said in it.
type previewChannel struct {
	ID       ids.ID  `json:"id"`
	ServerID ids.ID  `json:"server_id"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Position int     `json:"position"`
	ParentID *ids.ID `json:"parent_id"`
}

// previewChannels lays out what a new member could see, channels without a
// category first and each category then followed by its own, and counts the
// rest without naming them. So a preview shows no more than joining would.
func previewChannels(serverID ids.ID, roles []*store.Role, all []*store.Channel) ([]previewChannel, int) {
	pr := make([]perms.Role, len(roles))
	for i, ro := range roles {
		pr[i] = perms.Role{ID: ro.ID, Position: ro.Position, Permissions: ro.Permissions}
	}
	open := func(c *store.Channel) bool {
		if c.KeyConvID == c.ID {
			return false
		}
		po := make([]perms.Overwrite, len(c.Overwrites))
		for i, ow := range c.Overwrites {
			po[i] = perms.Overwrite{TargetID: ow.TargetID, Kind: ow.TargetType, Allow: ow.Allow, Deny: ow.Deny}
		}
		return perms.Compute(serverID, pr, perms.Member{}, false, po).Has(perms.ViewChannel)
	}
	view := func(c *store.Channel) previewChannel {
		return previewChannel{c.ID, serverID, c.Type, c.Name, c.Position, c.ParentID}
	}
	shown := map[ids.ID]bool{}
	for _, c := range all {
		if c.Type == "category" && open(c) {
			shown[c.ID] = true
		}
	}
	top, under, hidden := []previewChannel{}, map[ids.ID][]previewChannel{}, 0
	for _, c := range all {
		switch {
		case c.Type == "category":
		case !open(c):
			hidden++
		case c.ParentID != nil && shown[*c.ParentID]:
			under[*c.ParentID] = append(under[*c.ParentID], view(c))
		default:
			v := view(c)
			v.ParentID = nil
			top = append(top, v)
		}
	}
	out := top
	for _, c := range all {
		// a category nothing is listed under says nothing worth saying
		if kids := under[c.ID]; c.Type == "category" && len(kids) > 0 {
			out = append(append(out, view(c)), kids...)
		}
	}
	return out, hidden
}

// handleModServerPreview shows the owner a server's layout without joining it:
// the channels a new member could see, its roles and its first members, all of
// which the server holds readable. It carries no messages and no keys. Nobody in
// the server is told, so it is logged before anything is read, and the Privacy
// Policy, the Terms and ENCRYPTION.md say it exists.
func (s *Server) handleModServerPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	m := s.modServerFor(w, r)
	if m == nil {
		return
	}
	if err := s.Store.AppendModLog(ctx, &store.ModLogEntry{Action: store.LogServerPreview, ModeratorID: me.ID, ModeratorUsername: me.Username, TargetID: &m.OwnerID, Details: mustJSON(map[string]string{"server_name": m.Name})}); err != nil {
		WriteError(w, err)
		return
	}
	roles, err := s.Store.Roles(ctx, m.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	all, err := s.Store.ServerChannels(ctx, m.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	members, err := s.Store.Members(ctx, m.ID, 0, previewMembers, "")
	if err != nil {
		WriteError(w, err)
		return
	}
	type role struct {
		ID    ids.ID `json:"id"`
		Name  string `json:"name"`
		Color int    `json:"color"`
	}
	type member struct {
		UserID   ids.ID `json:"user_id"`
		Nickname string `json:"nickname"`
	}
	sort.SliceStable(roles, func(i, j int) bool { return roles[i].Position > roles[j].Position })
	rs := make([]role, 0, len(roles))
	for _, ro := range roles {
		if ro.ID != m.ID { // @everyone is every member
			rs = append(rs, role{ro.ID, ro.Name, ro.Color})
		}
	}
	ms, who := make([]member, 0, len(members)), make([]ids.ID, 0, len(members))
	for _, x := range members {
		ms = append(ms, member{x.UserID, x.Nickname})
		who = append(who, x.UserID)
	}
	channels, hidden := previewChannels(m.ID, roles, all)
	WriteJSON(w, http.StatusOK, map[string]any{"channels": channels, "private_channels": hidden, "roles": rs, "members": ms, "member_count": m.MemberCount, "people": s.people(ctx, who...)})
}

func (s *Server) handleModGiftLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.Store.OwnerGiftLinks(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"links": links})
}

// handleModGiftLinkCreate makes a gift link nobody paid for. Anyone signed in
// who opens it first gets the plan, as with one that was bought.
func (s *Server) handleModGiftLinkCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	var req struct {
		Plan   string `json:"plan"`
		Period string `json:"period"`
	}
	if err := ReadJSON(w, r, &req, 1<<10); err != nil {
		WriteError(w, err)
		return
	}
	days := giftDays[req.Period]
	if req.Plan == store.GiftShortName {
		days = 0 // nothing runs out: it gives one change of name
	} else if days == 0 || giftPrices[req.Plan] == nil {
		WriteError(w, E(http.StatusBadRequest, "invalid_plan", "Pick supporter or pro, for a month or a year."))
		return
	}
	code, err := s.Store.AddOwnerGiftLink(ctx, me.ID, req.Plan, days)
	if err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.AppendModLog(ctx, &store.ModLogEntry{Action: store.LogGiftLinkCreate, ModeratorID: me.ID, ModeratorUsername: me.Username, Details: mustJSON(map[string]any{"plan": req.Plan, "days": days})}); err != nil {
		s.Log.Warn("mod log", "err", err)
	}
	g, err := s.Store.GiftLinkByCode(ctx, code)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, g)
}

// handleModGiftLinkRevoke takes back a link the owner made, while nobody has
// claimed it. A claimed one has already given its plan.
func (s *Server) handleModGiftLinkRevoke(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	code := strings.TrimSpace(r.PathValue("code"))
	g, err := s.Store.RevokeOwnerGiftLink(ctx, code)
	switch {
	case errors.Is(err, store.ErrGiftClaimed):
		WriteError(w, E(http.StatusGone, "gift_claimed", "Someone has already claimed this gift."))
		return
	case errors.Is(err, store.ErrNotFound):
		WriteError(w, E(http.StatusNotFound, "unknown_gift", "That gift link doesn't exist."))
		return
	case err != nil:
		WriteError(w, err)
		return
	}
	if err := s.Store.AppendModLog(ctx, &store.ModLogEntry{Action: store.LogGiftLinkRevoke, ModeratorID: me.ID, ModeratorUsername: me.Username, Details: mustJSON(map[string]any{"plan": g.Plan, "days": g.Days})}); err != nil {
		s.Log.Warn("mod log", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
