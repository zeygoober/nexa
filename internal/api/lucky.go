package api

import (
	"context"
	"crypto/rand"
	"math/big"
	"net/http"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/store"
)

// luckyOdds is the chance of Lucky at each draw: one in this many. A person
// gets at most one draw a minute (limLucky), however much they send, so a
// script sending around the clock still waits weeks. Tests lower it.
var luckyOdds int64 = 50000

// luckyDraw gives the sender of a message just stored their draw for Lucky.
func (s *Server) luckyDraw(ctx context.Context, me *store.User) {
	if me == nil || me.Flags&(store.NotPeople|store.FlagBot|store.FlagDeleted) != 0 {
		return
	}
	if ok, _ := s.limLucky.Allow(me.ID.String()); !ok {
		return
	}
	n, err := rand.Int(rand.Reader, big.NewInt(luckyOdds))
	if err != nil || n.Sign() != 0 {
		return
	}
	s.awardSecret(ctx, me, store.BadgeLucky)
}

// awardSecret gives a Secret badge; a failure costs the person only the badge.
func (s *Server) awardSecret(ctx context.Context, me *store.User, badge string) {
	if _, err := s.Store.AwardSecretBadge(ctx, me, badge); err != nil {
		s.Log.Warn("badges: secret", "user", me.ID.String(), "badge", badge, "err", err)
	}
}

// handleBadgeFound takes a Secret badge the app saw someone find
// (store.AppFoundBadges). The app cannot be checked, so only those count.
func (s *Server) handleBadgeFound(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if ok, retry := s.limProfile.Allow("badges:" + u.ID.String()); !ok {
		w.Header().Set("Retry-After", retryAfter(retry))
		WriteError(w, ErrRateLimited)
		return
	}
	var body struct {
		Badge string `json:"badge"`
	}
	if err := ReadJSON(w, r, &body, 1<<10); err != nil {
		WriteError(w, err)
		return
	}
	if !store.AppFoundBadges[body.Badge] {
		WriteError(w, E(http.StatusBadRequest, "bad_badge", "That badge is not found that way."))
		return
	}
	isNew, err := s.Store.AwardSecretBadge(r.Context(), u, body.Badge)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"new": isNew})
}
