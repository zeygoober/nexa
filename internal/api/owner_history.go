package api

import (
	"errors"
	"net/http"
	"strings"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

func (s *Server) ownerHistoryRoutes(m *http.ServeMux) {
	m.Handle("POST /api/v1/moderation/history-reviews", s.userLimited(s.limManage, s.holdsRank(rankOwner, s.requestHistory)))
	m.Handle("GET /api/v1/moderation/history-reviews", s.staff(rankOwner, s.listHistory(true)))
	m.Handle("GET /api/v1/moderation/history-reviews/{id}/messages", s.staff(rankOwner, s.historyAction("view")))
	m.Handle("GET /api/v1/users/@me/history-reviews", s.authed(s.listHistory(false)))
	for _, action := range []string{"preview", "approve", "share", "withdraw"} {
		m.Handle("POST /api/v1/users/@me/history-reviews/{id}/"+action, s.authed(s.historyAction(action)))
	}
}
func reviewError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrReviewScope) {
		err = E(409, "review_scope", "Review unavailable. Historical sharing needs a verifiable two-person DM and both approvals for the exact selection.")
	}
	WriteError(w, err)
}
func (s *Server) requestHistory(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Channel ids.ID   `json:"channel_id"`
		Reason  string   `json:"reason"`
		Files   bool     `json:"include_files"`
		IDs     []ids.ID `json:"message_ids"`
		After   ids.ID   `json:"from_id"`
		Before  ids.ID   `json:"through_id"`
	}
	if err := ReadJSON(w, r, &b, 8192); err != nil {
		WriteError(w, err)
		return
	}
	b.Reason = strings.TrimSpace(b.Reason)
	if b.Reason == "" || len(b.Reason) > 500 || len(b.IDs) > store.ReviewLimit {
		WriteError(w, E(400, "invalid_review", "Provide a reason and a bounded message selection."))
		return
	}
	review, err := s.Store.RequestHistory(r.Context(), auth.UserFrom(r.Context()), b.Channel, b.Reason, b.Files, b.IDs, b.After, b.Before)
	if err != nil {
		reviewError(w, err)
		return
	}
	WriteJSON(w, 201, review)
}
func (s *Server) listHistory(owner bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var before ids.ID
		if raw := r.URL.Query().Get("before"); raw != "" {
			var err error
			before, err = ids.Parse(raw)
			if err != nil {
				WriteError(w, E(400, "bad_cursor", "Invalid cursor."))
				return
			}
		}
		list, err := s.Store.HistoryReviews(r.Context(), auth.UserFrom(r.Context()).ID, owner, before)
		if err != nil {
			reviewError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		WriteJSON(w, 200, list)
	}
}
func (s *Server) historyAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			WriteError(w, ErrNotFound)
			return
		}
		var boxes map[string][]byte
		if action == "share" {
			if err := ReadJSON(w, r, &boxes, maxChatEnvelopeBytes*store.ReviewLimit); err != nil {
				WriteError(w, err)
				return
			}
		}
		list, err := s.Store.HistoryAction(r.Context(), auth.UserFrom(r.Context()), id, action, boxes)
		if err != nil {
			reviewError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		if action == "preview" || action == "view" {
			WriteJSON(w, 200, list)
		} else {
			w.WriteHeader(204)
		}
	}
}
