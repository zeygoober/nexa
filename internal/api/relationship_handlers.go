package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

func (s *Server) relationshipRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/users/@me/relationships", s.authed(s.handleRelationships))
	mux.Handle("POST /api/v1/users/@me/relationships", s.userLimited(s.limFriend, s.handleFriendRequest))
	mux.Handle("PUT /api/v1/users/@me/relationships/{id}", s.userLimited(s.limRelate, s.handleFriendAccept))
	mux.Handle("DELETE /api/v1/users/@me/relationships/{id}", s.userLimited(s.limRelate, s.handleRelationshipDelete))
	mux.Handle("PATCH /api/v1/users/@me/relationships/{id}", s.userLimited(s.limRelate, s.handleRelationshipNickname))
	mux.Handle("PUT /api/v1/users/@me/relationships/{id}/note", s.userLimited(s.limRelate, s.handleRequestNote))
	mux.Handle("PUT /api/v1/users/@me/blocks/{id}", s.userLimited(s.limRelate, s.handleBlock))
	mux.Handle("DELETE /api/v1/users/@me/blocks/{id}", s.userLimited(s.limRelate, s.handleUnblock))
	s.Profile.MutualFriends = func(r *http.Request, me, other ids.ID) ([]ids.ID, error) {
		return s.Store.MutualFriends(r.Context(), me, other)
	}
}

type relationshipView = store.RelationshipView

func (s *Server) handleRelationships(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	rels, err := s.Store.Relationships(r.Context(), me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	targets := make([]ids.ID, 0, len(rels))
	for _, rel := range rels {
		targets = append(targets, rel.TargetID)
	}
	users, err := s.Store.UsersByIDs(r.Context(), targets)
	if err != nil {
		WriteError(w, err)
		return
	}
	out := make([]relationshipView, 0, len(rels))
	for _, rel := range rels {
		if u := users[rel.TargetID]; u != nil {
			out = append(out, rel.View(u.Public()))
		}
	}
	WriteJSON(w, http.StatusOK, out)
}

// blockedErr turns a block that landed while a request was on its way into
// the answer the block check gives.
func blockedErr(err error) error {
	if errors.Is(err, store.ErrBlocked) {
		return E(http.StatusForbidden, "blocked", "You cannot send a request to this user.")
	}
	return err
}

// emitRelationship tells both sides about their current relationship rows.
func (s *Server) emitRelationship(r *http.Request, a, b *store.User) {
	s.emitRelationshipTo(r, a, b)
	s.emitRelationshipTo(r, b, a)
}

// emitRelationshipTo tells owner about their current row with other.
func (s *Server) emitRelationshipTo(r *http.Request, owner, other *store.User) {
	rel, err := s.Store.Relationship(r.Context(), owner.ID, other.ID)
	if err != nil {
		// Only "there is no row" means the relationship went. A read that
		// merely failed used to send the same relationship_remove, so a
		// transient error after two people became friends dropped one of
		// them from the other's list while the database still said friend.
		if !errors.Is(err, store.ErrNotFound) {
			s.Log.Warn("relationship read-back", "user", owner.ID.String(), "other", other.ID.String(), "err", err)
			return
		}
		s.Bcast.EmitUsers([]ids.ID{owner.ID}, "relationship_remove", map[string]any{"user_id": other.ID})
		return
	}
	s.Bcast.EmitUsers([]ids.ID{owner.ID}, "relationship_add", rel.View(other.Public()))
}

func (s *Server) handleFriendRequest(w http.ResponseWriter, r *http.Request) {
	if !s.requireCaptcha(w, r, "friend_request") {
		return
	}
	me := auth.UserFrom(r.Context())
	var req struct {
		Username string `json:"username"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	ctx := r.Context()
	target, err := s.Store.UserByUsername(ctx, strings.ToLower(strings.TrimSpace(req.Username)))
	if err != nil || target.Flags&store.FlagDeleted != 0 || isWebhookAccount(target) {
		WriteError(w, E(http.StatusNotFound, "user_not_found", "No user with that username."))
		return
	}
	if target.ID == me.ID {
		WriteError(w, E(http.StatusBadRequest, "self", "You cannot add yourself."))
		return
	}
	if isSystem(target) {
		WriteError(w, E(http.StatusForbidden, "nexa_system", "Nexa is the system account; it can't be added as a friend."))
		return
	}
	ab, ba, err := s.Store.Blocked(ctx, me.ID, target.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if ab || ba {
		WriteError(w, E(http.StatusForbidden, "blocked", "You cannot send a request to this user."))
		return
	}
	existing, err := s.Store.Relationship(ctx, me.ID, target.ID)
	if err == nil {
		switch existing.Type {
		case "friend":
			WriteError(w, E(http.StatusBadRequest, "already_friends", "You are already friends."))
			return
		case "outgoing":
			WriteError(w, E(http.StatusBadRequest, "already_sent", "You already sent a request."))
			return
		case "incoming":
			if err := s.Store.SetRelationshipPair(ctx, me.ID, target.ID, "friend", "friend"); err != nil {
				WriteError(w, blockedErr(err))
				return
			}
			s.emitRelationship(r, me, target)
			WriteJSON(w, http.StatusOK, map[string]any{"type": "friend", "user": target.Public()})
			return
		}
	}
	if err := s.Store.SetRelationshipPair(ctx, me.ID, target.ID, "outgoing", "incoming"); err != nil {
		WriteError(w, blockedErr(err))
		return
	}
	s.emitRelationship(r, me, target)
	WriteJSON(w, http.StatusOK, map[string]any{"type": "outgoing", "user": target.Public()})
}

func (s *Server) handleFriendAccept(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	rel, err := s.Store.Relationship(r.Context(), me.ID, id)
	if err != nil || rel.Type != "incoming" {
		WriteError(w, E(http.StatusNotFound, "no_request", "There is no pending request from that user."))
		return
	}
	target, err := s.Store.UserByID(r.Context(), id)
	if err != nil || target.Flags&store.FlagDeleted != 0 {
		WriteError(w, E(http.StatusNotFound, "no_request", "There is no pending request from that user."))
		return
	}
	if err := s.Store.SetRelationshipPair(r.Context(), me.ID, id, "friend", "friend"); err != nil {
		WriteError(w, blockedErr(err))
		return
	}
	s.emitRelationship(r, me, target)
	// Becoming friends opens the conversation straight away, so neither of you
	// has to go looking for a way to start it.
	resp := map[string]any{"type": "friend", "user": target.Public()}
	if ch, created, err := s.Store.CreateDM(r.Context(), me.ID, target.ID); err == nil {
		if created {
			s.Bcast.EmitUsers(ch.Recipients, "channel_create", ch)
		} else if unhidden, err := s.Store.UnhideDM(r.Context(), ch.ID); err == nil && len(unhidden) > 0 {
			s.Bcast.EmitUsers(unhidden, "channel_create", ch)
		}
		resp["channel"] = ch
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRelationshipDelete(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	rel, err := s.Store.Relationship(r.Context(), me.ID, id)
	if err != nil || rel.Type == "blocked" {
		WriteError(w, ErrNotFound)
		return
	}
	if err := s.Store.DeleteRelationshipPair(r.Context(), me.ID, id); err != nil {
		WriteError(w, err)
		return
	}
	if target, err := s.Store.UserByID(r.Context(), id); err == nil {
		s.emitRelationship(r, me, target)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil || id == me.ID {
		WriteError(w, ErrNotFound)
		return
	}
	target, err := s.Store.UserByID(r.Context(), id)
	if err != nil || isWebhookAccount(target) {
		WriteError(w, ErrNotFound)
		return
	}
	// what Nexa sends is how it reaches everyone; there is no
	// conversation with it to close
	if isSystem(target) {
		WriteError(w, E(http.StatusForbidden, "nexa_system", "Nexa is the system account and can't be blocked."))
		return
	}
	if err := s.Store.Block(r.Context(), me.ID, id); err != nil {
		WriteError(w, err)
		return
	}
	s.emitRelationship(r, me, target)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	rel, err := s.Store.Relationship(r.Context(), me.ID, id)
	if err != nil || rel.Type != "blocked" {
		WriteError(w, ErrNotFound)
		return
	}
	if err := s.Store.DeleteRelationship(r.Context(), me.ID, id); err != nil {
		WriteError(w, err)
		return
	}
	if target, err := s.Store.UserByID(r.Context(), id); err == nil {
		s.emitRelationship(r, me, target)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRelationshipNickname sets a private nickname for a friend.
func (s *Server) handleRelationshipNickname(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	other, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := ReadJSON(w, r, &req, 2<<10); err != nil {
		WriteError(w, err)
		return
	}
	nick, ok := CleanText(strings.Join(strings.Fields(req.Nickname), " "), 32)
	if !ok {
		WriteError(w, E(http.StatusBadRequest, "invalid_nickname", "Nicknames are at most 32 characters."))
		return
	}
	if err := s.Store.SetRelationshipNickname(r.Context(), me.ID, other, nick); err != nil {
		WriteError(w, err)
		return
	}
	rel, err := s.Store.Relationship(r.Context(), me.ID, other)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	u, err := s.Store.UserByID(r.Context(), other)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	view := rel.View(u.Public())
	s.Bcast.EmitUsers([]ids.ID{me.ID}, "relationship_add", view)
	WriteJSON(w, http.StatusOK, view)
}

// A request note is a sealed box (keys.ts seal): the sender's ephemeral
// x25519 key, an iv and a GCM tag around the text. The app keeps the text to
// 200 characters, which fits well inside the ceiling in any script.
const (
	requestNoteMin = 32 + 12 + 16
	requestNoteMax = 1024
)

// handleRequestNote puts a note on a friend request the caller sent and is
// still waiting on. The sender's device seals it to the recipient's identity
// key, so the server can only check that it is shaped like a sealed box,
// keep it on the recipient's row, and pass it on to them.
func (s *Server) handleRequestNote(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	// Standard base64, padded or not. Go's decoder would skip line breaks, so
	// those are refused here rather than stored.
	sealed, err := base64.StdEncoding.DecodeString(req.Note)
	if err != nil {
		sealed, err = base64.RawStdEncoding.DecodeString(req.Note)
	}
	if err != nil || strings.ContainsAny(req.Note, "\r\n") {
		WriteError(w, E(http.StatusBadRequest, "invalid_note", "The note must be a sealed box in standard base64."))
		return
	}
	if len(sealed) < requestNoteMin || len(sealed) > requestNoteMax {
		WriteError(w, E(http.StatusBadRequest, "invalid_note", "A sealed note is between 60 and 1024 bytes."))
		return
	}
	noRequest := E(http.StatusNotFound, "no_request", "You have no pending request to that user.")
	rel, err := s.Store.Relationship(r.Context(), me.ID, id)
	if err != nil || rel.Type != "outgoing" {
		WriteError(w, noRequest)
		return
	}
	target, err := s.Store.UserByID(r.Context(), id)
	if err != nil || target.Flags&store.FlagDeleted != 0 {
		WriteError(w, noRequest)
		return
	}
	// kept padded, whichever way it came, so every client reads it the same way
	ok, err := s.Store.SetRequestNote(r.Context(), me.ID, id, base64.StdEncoding.EncodeToString(sealed))
	if err != nil {
		WriteError(w, err)
		return
	}
	if !ok {
		// answered, taken back or blocked since the check above
		WriteError(w, noRequest)
		return
	}
	s.emitRelationshipTo(r, target, me)
	w.WriteHeader(http.StatusNoContent)
}
