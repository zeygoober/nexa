package api

import (
	"net/http"
	"strconv"
	"strings"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// The owner's view of who holds badges and of every trade (the owner's request,
// 2026-11-18). Both are the owner's alone and logged before anything is read, as
// the owner's other views are.

// handleModBadgeOwners lists who holds copies of a badge now, a page at a time
// (?offset, ?limit up to 100): earned or given copies not traded away, what
// trades gave, a tiered badge's levels and a collectible's materials apart.
func (s *Server) handleModBadgeOwners(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	badge := r.PathValue("badge")
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset = max(offset, 0)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset == 0 {
		details := map[string]any{"badge": badge}
		// a staff-made badge's key is only a number, so the log keeps its name as it was then
		if rest, ok := strings.CutPrefix(badge, "event:"); ok {
			if id, err := ids.Parse(rest); err == nil {
				if d, err := s.Store.BadgeDef(ctx, id); err == nil {
					details["name"] = d.Name
				}
			}
		}
		if err := s.Store.AppendModLog(ctx, &store.ModLogEntry{Action: store.LogBadgeOwners, ModeratorID: me.ID, ModeratorUsername: me.Username, Details: mustJSON(details)}); err != nil {
			WriteError(w, err)
			return
		}
	}
	all, err := s.Store.BadgeOwners(ctx, badge)
	if err == store.ErrBadBadge {
		WriteError(w, ErrNotFound)
		return
	}
	if err != nil {
		WriteError(w, err)
		return
	}
	copies := 0
	for _, o := range all {
		copies += o.Total
	}
	page := all[min(offset, len(all)):min(offset+limit, len(all))]
	want := make([]ids.ID, 0, len(page))
	for _, o := range page {
		want = append(want, o.UserID)
	}
	users := s.publicUsers(ctx, want)
	WriteJSON(w, http.StatusOK, map[string]any{"owners": page, "users": users, "total": len(all), "copies": copies})
}

// handleModTradeLog lists every trade newest first, settled or not, since
// trading began (?status, ?q a username either side starts with, ?before a
// trade id, ?limit up to 100). A trade goes only with the account of one of
// its two people.
func (s *Server) handleModTradeLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	q := r.URL.Query()
	status := q.Get("status")
	switch status {
	case "", store.TradePending, store.TradeAccepted, store.TradeDeclined, store.TradeCancelled:
	default:
		WriteError(w, E(http.StatusBadRequest, "bad_status", "That is not a trade status."))
		return
	}
	who := strings.TrimPrefix(strings.TrimSpace(q.Get("q")), "@")
	if len(who) > 64 {
		who = who[:64]
	}
	var before ids.ID
	if b := q.Get("before"); b != "" {
		var err error
		if before, err = ids.Parse(b); err != nil {
			WriteError(w, E(http.StatusBadRequest, "bad_cursor", "That is not a trade id."))
			return
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if before.IsZero() {
		if err := s.Store.AppendModLog(ctx, &store.ModLogEntry{Action: store.LogTradeLog, ModeratorID: me.ID, ModeratorUsername: me.Username, Details: mustJSON(map[string]any{"status": status, "q": who})}); err != nil {
			WriteError(w, err)
			return
		}
	}
	trades, err := s.Store.TradeLog(ctx, status, who, before, limit)
	if err != nil {
		WriteError(w, err)
		return
	}
	seen := map[ids.ID]bool{}
	want := []ids.ID{}
	for _, t := range trades {
		for _, id := range []ids.ID{t.FromID, t.ToID} {
			if !seen[id] {
				seen[id] = true
				want = append(want, id)
			}
		}
	}
	users := s.publicUsers(ctx, want)
	WriteJSON(w, http.StatusOK, map[string]any{"trades": trades, "users": users, "more": len(trades) == limit})
}
