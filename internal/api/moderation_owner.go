package api

import (
	"errors"
	"net/http"
	"strings"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// What only the owner does from the panel (the owner's decision, 2026-11-08):
// reading an account's addresses and sessions, joining a server without an
// invite, and making gift links without paying. Each is written to mod_log, and
// the Privacy Policy, the Terms and ENCRYPTION.md say that they exist.

func (s *Server) modOwnerRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/moderation/users/{id}/account", s.staff(rankOwner, s.handleModAccount))
	mux.Handle("POST /api/v1/moderation/servers/{id}/join", s.staff(rankOwner, s.handleModServerJoin))
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
	if days == 0 || giftPrices[req.Plan] == nil {
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
