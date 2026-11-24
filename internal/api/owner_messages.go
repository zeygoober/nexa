package api

import (
	"context"
	"encoding/json"
	"net/http"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

const maxChatEnvelopeBytes = 2*maxEnvelopeBytes + 1024

// chatEnvelope is only for user-authored chat. Call summaries, note keys and
// follow secrets also use v1 envelopes and must never get an owner recipient.
func (s *Server) chatEnvelope(ctx context.Context, body *messageBody, required bool) (*envelope, error) {
	if len(body.Ciphertext) > maxChatEnvelopeBytes {
		return nil, errMessageTooLong
	}
	var env envelope
	if err := json.Unmarshal(body.Ciphertext, &env); err != nil {
		return nil, E(400, "bad_envelope", "Invalid message envelope.")
	}
	copy := env.Owner
	env.Owner = nil
	ordinary, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	if len(ordinary) > maxEnvelopeBytes {
		return nil, errMessageTooLong
	}
	if _, ok := validEnvelope(ordinary); !ok {
		return nil, E(400, "bad_envelope", "Invalid message envelope.")
	}
	if required {
		if copy == nil {
			return nil, E(409, "owner_copy_required", "Update Nexa to send messages. The instance owner can read new messages.")
		}
		owner, err := s.Store.InstanceOwnerID(ctx)
		if err != nil {
			return nil, err
		}
		if owner.IsZero() || copy.ID != owner || len(copy.Box) <= 60 || len(copy.Box) > maxEnvelopeBytes {
			return nil, E(400, "bad_owner_copy", "Invalid copy for the instance owner.")
		}
	} else if copy != nil {
		return nil, E(400, "unexpected_owner_copy", "This message requires participant approval to share with the owner.")
	}
	env.Owner = copy
	body.Ciphertext, err = json.Marshal(env)
	return &env, err
}

func (s *Server) checkChatEnvelope(w http.ResponseWriter, r *http.Request, ch *store.Channel, body *messageBody, required bool) (*envelope, bool) {
	env, err := s.chatEnvelope(r.Context(), body, required)
	if err != nil {
		WriteError(w, err)
		return nil, false
	}
	return s.checkEnvelopeEpoch(w, r, ch, body, env)
}

func (s *Server) handleOwnerMessages(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	var before ids.ID
	if q := r.URL.Query().Get("before"); q != "" {
		var err error
		before, err = ids.Parse(q)
		if err != nil || before.IsZero() {
			WriteError(w, E(400, "bad_cursor", "Invalid message cursor."))
			return
		}
	}
	if err := s.Store.AppendModLog(r.Context(), &store.ModLogEntry{Action: "owner_message_view", ModeratorID: me.ID, ModeratorUsername: me.Username, Details: mustJSON(map[string]any{"before": before})}); err != nil {
		WriteError(w, err)
		return
	}
	list, err := s.Store.OwnerMessages(r.Context(), me.ID, before)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if list == nil {
		list = []*store.Message{}
	}
	WriteJSON(w, 200, list)
}

// Bot and webhook posts already reach the service readable. Index new ones
// for the same owner view without changing what recipients or apps receive.
func (s *Server) insertAppMessage(ctx context.Context, m *store.Message) error {
	if s.Cfg.OwnerMessageAccess {
		id, err := s.Store.InstanceOwnerID(ctx)
		if err != nil {
			return err
		}
		if id.IsZero() {
			return E(409, "owner_unavailable", "The instance owner is unavailable.")
		}
		m.OwnerRecipient = &id
	}
	return s.Store.InsertMessage(ctx, m)
}
