package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/gateway"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// Gems (store/gems.go). A message earns its sender a Gem at most once a
// minute, times the team their profile shows; Gems buy capsules, whose
// collectibles fuse into better materials and trade. The server draws every
// capsule. Earning that looks scripted asks for a captcha first, then files a
// Gem Farming report for staff, holding what was earned until they decide.

func (s *Server) gemRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/gems", s.authed(s.handleGems))
	mux.Handle("POST /api/v1/gems/capsule", s.userLimited(s.limGemSpend, s.handleGemCapsule))
	mux.Handle("POST /api/v1/gems/fuse", s.userLimited(s.limGemSpend, s.handleGemFuse))
	mux.Handle("POST /api/v1/gems/confirm", s.authed(s.handleGemConfirm))
	mux.Handle("GET /api/v1/gems/record", s.authed(s.handleGemRecord))
}

// handleGemRecord is what the data export reads: the wallet, the ledger and
// the timing the farming check keeps.
func (s *Server) handleGemRecord(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	wallet, err := s.Store.GemWallet(r.Context(), me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	ledger, err := s.Store.GemLedger(r.Context(), me.ID, 10_000)
	if err != nil {
		WriteError(w, err)
		return
	}
	watch, err := s.Store.GemWatch(r.Context(), me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"wallet": wallet, "ledger": ledger, "watch": watch})
}

// gemFarmingReason is the reason of the reports a farming flag files. It is
// not in reportReasons, so nobody can send one by hand.
const gemFarmingReason = "gem_farming"

// gemAloneFor is how long nobody else must have spoken for a message to count
// as said to nobody.
const gemAloneFor = 30 * time.Minute

// gemTeamFresh is how long a team's multiplier is used before it is worked
// out again: working it out reads the whole profile's badges.
const gemTeamFresh = 5 * time.Minute

type gemTeamCache struct {
	mu sync.Mutex
	m  map[ids.ID]gemTeamAt
}

type gemTeamAt struct {
	mult int
	at   time.Time
}

func (c *gemTeamCache) get(uid ids.ID) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[uid]
	if !ok || time.Since(v.at) > gemTeamFresh {
		return 0, false
	}
	return v.mult, true
}

func (c *gemTeamCache) put(uid ids.ID, mult int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) > 50_000 {
		c.m = map[ids.ID]gemTeamAt{}
	}
	c.m[uid] = gemTeamAt{mult, time.Now()}
}

func (c *gemTeamCache) forget(uid ids.ID) {
	c.mu.Lock()
	delete(c.m, uid)
	c.mu.Unlock()
}

func (s *Server) gemMultiplier(ctx context.Context, me *store.User) int {
	if m, ok := s.gemTeams.get(me.ID); ok {
		return m
	}
	_, mult, err := s.Store.GemTeam(ctx, me)
	if err != nil {
		return 100
	}
	mult = mult * store.GemPlanBoost(me.Plan) / 100
	s.gemTeams.put(me.ID, mult)
	return mult
}

// earnGem gives the sender of a message just stored what it earns. A failure
// costs them the Gem, never the message.
func (s *Server) earnGem(ctx context.Context, me *store.User, m *store.Message) {
	if me == nil || m == nil || me.Flags&(store.NotPeople|store.FlagBot|store.FlagDeleted) != 0 {
		return
	}
	if ok, _ := s.limGem.Allow(me.ID.String()); !ok {
		return
	}
	alone := true
	if spoke, err := s.Store.SomeoneElseSpoke(ctx, m.ChannelID, me.ID, time.Now().Add(-gemAloneFor)); err == nil {
		alone = !spoke
	}
	sig := store.GemSignal{Alone: alone, Size: len(m.Ciphertext)}
	got, err := s.Store.EarnGem(ctx, me.ID, s.gemMultiplier(ctx, me), sig, time.Now())
	if err != nil {
		s.Log.Warn("gems: earn", "user", me.ID.String(), "err", err)
		return
	}
	if got == nil {
		return
	}
	switch got.Flag {
	case store.GemStateCheck:
		s.emitGems(ctx, me.ID)
	case store.GemStateReport:
		// staff cannot be moderated, so for them it stays a check
		if rankFromFlags(me.Flags, false) != rankNone {
			_ = s.Store.SetGemReport(ctx, me.ID, 0)
		} else {
			s.fileGemReport(ctx, me)
		}
		s.emitGems(ctx, me.ID)
	}
}

// emitGems tells a person their wallet as it is now.
func (s *Server) emitGems(ctx context.Context, uid ids.ID) {
	w, err := s.Store.GemWallet(ctx, uid)
	if err != nil {
		return
	}
	s.Bcast.EmitUsers([]ids.ID{uid}, gateway.EvGemsUpdate, w)
}

// fileGemReport puts a Gem Farming report in the staff queue for me, from
// Nexa's own account, with the timing the check kept and how many other
// accounts share an address with this one. It holds no message or text.
func (s *Server) fileGemReport(ctx context.Context, me *store.User) {
	fail := func(err error) {
		s.Log.Warn("gems: report", "user", me.ID.String(), "err", err)
		_ = s.Store.SetGemReport(ctx, me.ID, 0)
	}
	sys, err := s.systemUser(ctx)
	if err != nil {
		fail(err)
		return
	}
	days, err := s.Store.GemWatch(ctx, me.ID)
	if err != nil {
		fail(err)
		return
	}
	w, err := s.Store.GemWallet(ctx, me.ID)
	if err != nil {
		fail(err)
		return
	}
	alts, _ := s.Store.AltAccounts(ctx, me.ID)
	snap := map[string]any{"id": me.ID, "username": me.Username, "display_name": me.DisplayName,
		"gem_farming": map[string]any{"days": days, "held": w.Held, "earned": w.Earned, "alts": len(alts), "flagged_at": w.StateAt}}
	if me.AvatarID != nil {
		snap["avatar_id"] = *me.AvatarID
	}
	v := &store.SafetyReport{RequestID: "gems-" + me.ID.String() + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10), ReporterID: sys.ID, TargetKind: "user", TargetID: me.ID, SubjectID: me.ID, Part: "whole", Reason: gemFarmingReason, Snapshot: mustJSON(snap)}
	if err := s.Store.CreateSafetyReport(ctx, v, nil); err != nil {
		fail(err)
		return
	}
	if err := s.Store.SetGemReport(ctx, me.ID, v.ID); err != nil {
		s.Log.Warn("gems: report id", "user", me.ID.String(), "err", err)
	}
	s.emitReportChanges(ctx, false, true, v)
}

// settleGemReports gives the outcome of any Gem Farming report among list
// that staff just closed: dismissed, the held Gems go to the person; acted
// on, they are taken, earning stops, and the person gets a case to appeal.
func (s *Server) settleGemReports(ctx context.Context, me *store.User, note string, list ...*store.SafetyReport) {
	for _, v := range list {
		if v == nil || v.Reason != gemFarmingReason || (v.Status != "actioned" && v.Status != "dismissed") {
			continue
		}
		who, err := s.Store.GemReportOwner(ctx, v.ID)
		if err != nil || who == 0 {
			continue
		}
		var c *store.ModCase
		var log *store.ModLogEntry
		actioned := v.Status == "actioned"
		if actioned {
			u, err := s.Store.UserByID(ctx, who)
			if err != nil {
				continue
			}
			w, _ := s.Store.GemWallet(ctx, who)
			until := time.Now().UnixMilli() + store.GemOffFor
			details := mustJSON(map[string]any{"report_id": v.ID, "held": w.Held, "off_until": until})
			c = &store.ModCase{UserID: who, ModeratorID: me.ID, Action: store.ActionGemFarming, Reason: note, Details: details, TargetUsername: u.Username, TargetDisplayName: u.DisplayName, ExpiresAt: &until}
			log = &store.ModLogEntry{Action: store.ActionGemFarming, ModeratorID: me.ID, ModeratorUsername: me.Username, TargetID: &u.ID, TargetUsername: u.Username, Reason: note, Details: details}
		}
		if _, _, err := s.Store.CloseGemReport(ctx, v.ID, actioned, c, log); err != nil {
			s.Log.Warn("gems: settle", "report", v.ID.String(), "err", err)
			continue
		}
		if c != nil {
			s.notifyMember(ctx, c)
			s.emitCaseStaff(ctx, c)
			s.emitLog(ctx, log)
		}
		s.emitGems(ctx, who)
	}
}

// gemsView is what the Gems page shows: the wallet, the team and what it
// multiplies by, the capsule's price and odds, every collectible a capsule can
// give, and the copies of each the person holds.
type gemsView struct {
	Wallet       *store.GemWallet      `json:"wallet"`
	Team         []store.GemTeamMember `json:"team"`
	Multiplier   int                   `json:"multiplier"` // the team's times the plan's, what a message earns
	PlanBoost    int                   `json:"plan_boost"` // the plan's alone, 100 without one
	Price        int                   `json:"capsule_price"`
	FuseCost     int                   `json:"fuse_cost"`
	Odds         map[string]int        `json:"odds"`
	Collectibles []store.Collectible   `json:"collectibles"`
	Collection   []store.TradeLine     `json:"collection"`
}

func (s *Server) gemsFor(ctx context.Context, me *store.User) (*gemsView, error) {
	w, err := s.Store.GemWallet(ctx, me.ID)
	if err != nil {
		return nil, err
	}
	team, mult, err := s.Store.GemTeam(ctx, me)
	if err != nil {
		return nil, err
	}
	boost := store.GemPlanBoost(me.Plan)
	mult = mult * boost / 100
	s.gemTeams.put(me.ID, mult)
	all, err := s.Store.Collectibles(ctx)
	if err != nil {
		return nil, err
	}
	held, err := s.Store.MyTradable(ctx, me)
	if err != nil {
		return nil, err
	}
	mine := []store.TradeLine{}
	for _, l := range held {
		if l.Badge != nil && l.Badge.Rarity != "" {
			mine = append(mine, l)
		}
	}
	return &gemsView{Wallet: w, Team: team, Multiplier: mult, PlanBoost: boost, Price: store.GemCapsulePrice, FuseCost: store.GemFuseCost, Odds: store.CapsuleOdds(all), Collectibles: all, Collection: mine}, nil
}

func (s *Server) handleGems(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	if me.Flags&(store.NotPeople|store.FlagBot) != 0 {
		WriteError(w, errTradePerson)
		return
	}
	v, err := s.gemsFor(r.Context(), me)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, v)
}

var (
	errGemsShort = E(http.StatusConflict, "gems_short", "You do not have enough Gems.")
	errGemsNone  = E(http.StatusConflict, "gems_no_collectibles", "There is nothing in capsules yet.")
	errGemsHeld  = E(http.StatusConflict, "gems_held", "Your Gems are on hold until the check is done.")
	errGemsFuse  = E(http.StatusBadRequest, "gems_fuse", "That cannot be fused.")
)

func gemError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrGemsShort):
		WriteError(w, errGemsShort)
	case errors.Is(err, store.ErrGemsNone):
		WriteError(w, errGemsNone)
	case errors.Is(err, store.ErrGemsFuse):
		WriteError(w, errGemsFuse)
	case errors.Is(err, store.ErrGemsReported), errors.Is(err, store.ErrTradeGemsHeld):
		WriteError(w, errGemsHeld)
	default:
		WriteError(w, err)
	}
}

// heldGems answers when a check or report has the caller's Gems on hold,
// which pauses spending them too.
func (s *Server) heldGems(w http.ResponseWriter, r *http.Request, uid ids.ID) bool {
	wallet, err := s.Store.GemWallet(r.Context(), uid)
	if err != nil {
		WriteError(w, err)
		return true
	}
	if wallet.State == store.GemStateCheck || wallet.State == store.GemStateReport {
		WriteError(w, errGemsHeld)
		return true
	}
	return false
}

func (s *Server) handleGemCapsule(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	if me.Flags&(store.NotPeople|store.FlagBot) != 0 {
		WriteError(w, errTradePerson)
		return
	}
	if s.heldGems(w, r, me.ID) {
		return
	}
	got, key, err := s.Store.OpenCapsule(r.Context(), me.ID)
	if err != nil {
		gemError(w, err)
		return
	}
	s.gemTeams.forget(me.ID)
	v, err := s.gemsFor(r.Context(), me)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"got": got, "key": key, "gems": v})
}

func (s *Server) handleGemFuse(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	var req struct {
		Key    string `json:"key"`
		Copies int    `json:"copies"`
	}
	if err := ReadJSON(w, r, &req, 1<<10); err != nil {
		WriteError(w, err)
		return
	}
	if me.Flags&(store.NotPeople|store.FlagBot) != 0 {
		WriteError(w, errTradePerson)
		return
	}
	key, won, err := s.Store.Fuse(r.Context(), me.ID, req.Key, req.Copies)
	if err != nil {
		gemError(w, err)
		return
	}
	s.gemTeams.forget(me.ID)
	v, err := s.gemsFor(r.Context(), me)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"key": key, "upgraded": won, "gems": v})
}

// handleGemConfirm answers a farming check: a captcha shows someone is there,
// and what was held is theirs again.
func (s *Server) handleGemConfirm(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	if !s.requireCaptcha(w, r, "gems") {
		return
	}
	if _, err := s.Store.ConfirmGems(r.Context(), me.ID); err != nil {
		gemError(w, err)
		return
	}
	v, err := s.gemsFor(r.Context(), me)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, v)
}
