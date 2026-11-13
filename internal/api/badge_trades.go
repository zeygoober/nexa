package api

import (
	"errors"
	"net/http"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// Badge trades (store/badge_trades.go). A trade is a window two people share:
// each puts badges of their own on their side, both watch, and when both press
// Ready on what is there, it happens. It also lives as a system_trade message in
// their direct messages, so it is still there when the other person was not
// when it began. Like every system line it is not end-to-end encrypted: the
// server knows each person's badges and moves them. Every change goes to both
// people as a trade_update.

func (s *Server) badgeTradeRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/trades/holdings", s.authed(s.handleTradeMine))
	mux.Handle("POST /api/v1/trades", s.userLimited(s.limTrade, s.handleTradeOpen))
	mux.Handle("GET /api/v1/trades/{id}", s.authed(s.handleTradeGet))
	mux.Handle("PUT /api/v1/trades/{id}/offer", s.authed(s.handleTradeOffer))
	mux.Handle("PUT /api/v1/trades/{id}/ready", s.authed(s.handleTradeReady))
	mux.Handle("POST /api/v1/trades/{id}/end", s.authed(s.handleTradeEnd))
}

var (
	errTradeSelf    = E(http.StatusBadRequest, "trade_self", "You cannot trade with yourself.")
	errTradeEmpty   = E(http.StatusBadRequest, "trade_empty", "Put at least one badge in the trade first.")
	errTradeInvalid = E(http.StatusBadRequest, "trade_invalid", "Some of those badges cannot be traded.")
	errTradePerson  = E(http.StatusBadRequest, "trade_person", "Only people can trade badges.")
	errTradeClosed  = E(http.StatusConflict, "trade_closed", "This trade is over.")
	errTradeChanged = E(http.StatusConflict, "trade_changed", "The trade changed. Look at it again before you press Ready.")
	errTradeMany    = E(http.StatusTooManyRequests, "trade_too_many", "You have too many trades open. Finish or end some first.")
)

// tradeError answers what a trade call failed with. Who lacks a badge is told
// from the caller's side, so each app says whose it is.
func tradeError(w http.ResponseWriter, err error, caller ids.ID) {
	var short *store.ErrTradeShort
	switch {
	case errors.As(err, &short):
		if short.Who == caller {
			WriteError(w, E(http.StatusConflict, "trade_you_short", "You no longer have every badge you put in this trade."))
		} else {
			WriteError(w, E(http.StatusConflict, "trade_they_short", "They no longer have every badge they put in this trade."))
		}
	case errors.Is(err, store.ErrTradeEmpty):
		WriteError(w, errTradeEmpty)
	case errors.Is(err, store.ErrTradeLines):
		WriteError(w, errTradeInvalid)
	case errors.Is(err, store.ErrTradeGone):
		WriteError(w, ErrNotFound)
	case errors.Is(err, store.ErrTradeClosed):
		WriteError(w, errTradeClosed)
	case errors.Is(err, store.ErrTradeChanged):
		WriteError(w, errTradeChanged)
	case errors.Is(err, store.ErrTradeTooMany):
		WriteError(w, errTradeMany)
	default:
		WriteError(w, err)
	}
}

// handleTradeMine lists the badges the caller can put in a trade, with how many
// copies of each: only they are told that.
func (s *Server) handleTradeMine(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	list, err := s.Store.MyTradable(r.Context(), me)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, list)
}

// handleTradeOpen starts a trade with someone, or returns the one already open
// between the two, whoever began it. It follows the rules of a message (a block,
// their DM privacy, Nexa's own account, a captcha for a first message to a
// stranger), and the trade's line reaches their messages at once.
func (s *Server) handleTradeOpen(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	ctx := r.Context()
	var req struct {
		To ids.ID `json:"to"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if req.To == me.ID {
		WriteError(w, errTradeSelf)
		return
	}
	if me.Flags&(store.NotPeople|store.FlagBot|store.FlagDeleted) != 0 {
		WriteError(w, errTradePerson)
		return
	}
	target, ok := s.dmGate(w, r, me, req.To)
	if !ok {
		return
	}
	if target.Flags&(store.NotPeople|store.FlagBot) != 0 {
		WriteError(w, errTradePerson)
		return
	}
	if open, err := s.Store.PendingTradeBetween(ctx, me.ID, target.ID); err != nil {
		WriteError(w, err)
		return
	} else if open != nil {
		WriteJSON(w, http.StatusOK, open)
		return
	}
	ch, created, err := s.Store.StartDM(ctx, me.ID, target.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if created {
		s.Bcast.EmitUsers([]ids.ID{me.ID}, "channel_create", ch)
	} else if reopened, err := s.Store.ReopenDM(ctx, ch.ID, me.ID); err == nil && reopened {
		s.Bcast.EmitUsers([]ids.ID{me.ID}, "channel_create", ch)
	}
	// it mentions the person, so it reaches them as a message to them does
	m := &store.Message{ID: ids.New(), ChannelID: ch.ID, AuthorID: me.ID, Type: "system_trade", Mentions: []ids.ID{target.ID}}
	t := &store.Trade{ID: ids.New(), FromID: me.ID, ToID: target.ID, ChannelID: ch.ID, MessageID: m.ID}
	if err := s.Store.OpenTrade(ctx, t); err != nil {
		tradeError(w, err, me.ID)
		return
	}
	if err := s.Store.InsertMessage(ctx, m); err != nil {
		s.Store.DeleteTrade(ctx, t.ID)
		WriteError(w, err)
		return
	}
	view, err := s.Store.TradeByID(ctx, t.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	m.Trade = view
	m.MentionRoles = []ids.ID{}
	m.Attachments = []store.Attachment{}
	m.Reactions = []store.ReactionSummary{}
	s.revealDM(ctx, ch)
	s.Bcast.EmitChannel(ch.ID, "message_create", m)
	WriteJSON(w, http.StatusCreated, view)
}

// tradeID reads a trade's id from the path.
func tradeID(w http.ResponseWriter, r *http.Request) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return 0, false
	}
	return id, true
}

// changed tells both people what the trade is now.
func (s *Server) changed(w http.ResponseWriter, t *store.Trade) {
	s.Bcast.EmitUsers([]ids.ID{t.FromID, t.ToID}, "trade_update", t)
	WriteJSON(w, http.StatusOK, t)
}

// blockedTrade answers when either person has blocked the other: a block ends
// what is shared across it.
func (s *Server) blockedTrade(w http.ResponseWriter, r *http.Request, t *store.Trade) bool {
	ab, ba, err := s.Store.Blocked(r.Context(), t.FromID, t.ToID)
	if err != nil {
		WriteError(w, err)
		return true
	}
	if ab || ba {
		WriteError(w, E(http.StatusForbidden, "blocked", "You cannot trade with this user."))
		return true
	}
	return false
}

func (s *Server) handleTradeGet(w http.ResponseWriter, r *http.Request) {
	id, ok := tradeID(w, r)
	if !ok {
		return
	}
	me := auth.UserFrom(r.Context())
	t, err := s.Store.TradeByID(r.Context(), id)
	if err != nil || (t.FromID != me.ID && t.ToID != me.ID) {
		WriteError(w, ErrNotFound)
		return
	}
	WriteJSON(w, http.StatusOK, t)
}

func (s *Server) handleTradeOffer(w http.ResponseWriter, r *http.Request) {
	id, ok := tradeID(w, r)
	if !ok {
		return
	}
	me := auth.UserFrom(r.Context())
	var req struct {
		Lines []store.TradeLine `json:"lines"`
	}
	if err := ReadJSON(w, r, &req, 64<<10); err != nil {
		WriteError(w, err)
		return
	}
	cur, err := s.Store.TradeByID(r.Context(), id)
	if err != nil || (cur.FromID != me.ID && cur.ToID != me.ID) {
		WriteError(w, ErrNotFound)
		return
	}
	if s.blockedTrade(w, r, cur) {
		return
	}
	t, err := s.Store.SetOffer(r.Context(), id, me.ID, req.Lines)
	if err != nil {
		tradeError(w, err, me.ID)
		return
	}
	s.changed(w, t)
}

func (s *Server) handleTradeReady(w http.ResponseWriter, r *http.Request) {
	id, ok := tradeID(w, r)
	if !ok {
		return
	}
	me := auth.UserFrom(r.Context())
	var req struct {
		Ready   bool `json:"ready"`
		Version int  `json:"version"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	cur, err := s.Store.TradeByID(r.Context(), id)
	if err != nil || (cur.FromID != me.ID && cur.ToID != me.ID) {
		WriteError(w, ErrNotFound)
		return
	}
	if s.blockedTrade(w, r, cur) {
		return
	}
	t, err := s.Store.SetReady(r.Context(), id, me.ID, req.Ready, req.Version)
	if err != nil {
		tradeError(w, err, me.ID)
		return
	}
	s.changed(w, t)
}

// handleTradeEnd closes a trade that has not happened: declining it when it was
// opened with the caller, cancelling it when they opened it.
func (s *Server) handleTradeEnd(w http.ResponseWriter, r *http.Request) {
	id, ok := tradeID(w, r)
	if !ok {
		return
	}
	me := auth.UserFrom(r.Context())
	t, err := s.Store.EndTrade(r.Context(), id, me.ID)
	if err != nil {
		tradeError(w, err, me.ID)
		return
	}
	s.changed(w, t)
}
