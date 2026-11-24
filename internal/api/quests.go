package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/gateway"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// Quests (store/quests.go): three daily quests and the Quest Tower, paid
// in Gems the moment they are finished. What people do is counted here as it
// happens; messages and calls, which come often, are added up in memory and
// written together every questFlushEvery, everything else at once.

const (
	questFlushEvery = 10 * time.Second
	// questKindsFresh is how long the kinds someone's open quests want are
	// trusted before they are read again.
	questKindsFresh = 10 * time.Minute
)

func (s *Server) questRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/quests", s.authed(s.handleQuests))
	mux.Handle("POST /api/v1/quests/{id}/reroll", s.userLimited(s.limGemSpend, s.handleQuestReroll))
	mux.Handle("POST /api/v1/quests/trial/skip", s.userLimited(s.limGemSpend, s.handleTrialSkip))
}

// questBook is what this process knows of quests between writes: the kinds
// each person's open quests want, and what is waiting to be written.
type questBook struct {
	mu      sync.Mutex
	kinds   map[ids.ID]questKinds
	pending map[ids.ID]*questPending
}

type questKinds struct {
	day   int64
	at    time.Time
	kinds map[string]bool
}

type questPending struct {
	counts  map[string]int64
	servers map[ids.ID]bool
	size    int64
}

// questPerson is whether an account does quests: people, not bots, apps or
// Nexa's own accounts.
func questPerson(u *store.User) bool {
	return u != nil && u.Flags&(store.NotPeople|store.FlagBot|store.FlagDeleted) == 0
}

// questWants is whether uid has an open quest of kind. When it cannot tell it
// says yes, and the write finds out.
func (s *Server) questWants(ctx context.Context, uid ids.ID, kind string) bool {
	now := time.Now()
	day := now.UnixMilli() / (24 * 60 * 60 * 1000)
	b := &s.quests
	b.mu.Lock()
	k, ok := b.kinds[uid]
	b.mu.Unlock()
	if ok && k.day == day && now.Sub(k.at) < questKindsFresh {
		return k.kinds[kind]
	}
	kinds, err := s.Store.OpenQuestKinds(ctx, uid, now)
	if err != nil {
		return true
	}
	b.mu.Lock()
	if b.kinds == nil {
		b.kinds = map[ids.ID]questKinds{}
	}
	b.kinds[uid] = questKinds{day: day, at: now, kinds: kinds}
	b.mu.Unlock()
	return kinds[kind]
}

func (s *Server) questForget(uid ids.ID) {
	s.quests.mu.Lock()
	delete(s.quests.kinds, uid)
	s.quests.mu.Unlock()
}

// questQueue keeps something that happens often (messages, calls) for the next write.
func (s *Server) questQueue(uid ids.ID, f func(p *questPending)) {
	b := &s.quests
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending == nil {
		b.pending = map[ids.ID]*questPending{}
	}
	p := b.pending[uid]
	if p == nil {
		p = &questPending{counts: map[string]int64{}, servers: map[ids.ID]bool{}}
		b.pending[uid] = p
	}
	f(p)
}

// questMessage counts a message just stored: towards messages when someone
// else spoke in its conversation in the last half hour, and towards the
// servers messaged in.
func (s *Server) questMessage(ctx context.Context, me *store.User, m *store.Message) {
	if !questPerson(me) || m == nil {
		return
	}
	wantsCount := s.questWants(ctx, me.ID, store.QuestMessages)
	wantsServer := s.questWants(ctx, me.ID, store.QuestServers)
	var server ids.ID
	if wantsServer {
		if ch, err := s.Store.ChannelByID(ctx, m.ChannelID); err == nil && ch.ServerID != nil {
			server = *ch.ServerID
		}
	}
	counted := false
	if wantsCount {
		if spoke, err := s.Store.SomeoneElseSpoke(ctx, m.ChannelID, me.ID, time.Now().Add(-gemAloneFor)); err == nil && spoke {
			counted = true
		}
	}
	if !counted && server == 0 {
		return
	}
	s.questQueue(me.ID, func(p *questPending) {
		if counted {
			p.counts[store.QuestMessages]++
		}
		if server != 0 {
			p.servers[server] = true
		}
	})
}

// questCallMinute counts a minute of calls: everyone in a call with someone
// else, and the size of each call in accounts.
func (s *Server) questCallMinute(ctx context.Context) {
	for uid, size := range s.Voice.CallSizes() {
		wantsTime := s.questWants(ctx, uid, store.QuestCallMinutes)
		wantsSize := size >= 2 && s.questWants(ctx, uid, store.QuestCallSize)
		if !wantsTime && !wantsSize {
			continue
		}
		s.questQueue(uid, func(p *questPending) {
			if wantsTime {
				p.counts[store.QuestCallMinutes]++
			}
			if wantsSize {
				p.size = max(p.size, int64(size))
			}
		})
	}
}

// flushQuests writes what was kept in memory.
func (s *Server) flushQuests(ctx context.Context) {
	s.quests.mu.Lock()
	pending := s.quests.pending
	s.quests.pending = nil
	s.quests.mu.Unlock()
	for uid, p := range pending {
		var steps []store.QuestStep
		for k, n := range p.counts {
			steps = append(steps, store.QuestStep{Kind: k, N: n})
		}
		for v := range p.servers {
			steps = append(steps, store.QuestStep{Kind: store.QuestServers, N: 1, Subject: v})
		}
		if p.size > 0 {
			steps = append(steps, store.QuestStep{Kind: store.QuestCallSize, N: p.size})
		}
		s.questProgress(ctx, uid, steps...)
	}
}

// questProgress writes steps now and tells the person of any quest they finished.
func (s *Server) questProgress(ctx context.Context, uid ids.ID, steps ...store.QuestStep) {
	if len(steps) == 0 {
		return
	}
	done, err := s.Store.AddQuestProgress(ctx, uid, steps, time.Now())
	if err != nil {
		if ctx.Err() == nil {
			s.Log.Warn("quests: progress", "user", uid.String(), "err", err)
		}
		return
	}
	if len(done) == 0 {
		return
	}
	s.questForget(uid)
	s.gemTeams.forget(uid)
	s.emitQuests(ctx, uid, done)
}

// questNow counts one thing that does not happen often, at once.
func (s *Server) questNow(ctx context.Context, u *store.User, kind string, n int64, subject ids.ID) {
	if !questPerson(u) || n <= 0 || !s.questWants(ctx, u.ID, kind) {
		return
	}
	s.questProgress(ctx, u.ID, store.QuestStep{Kind: kind, N: n, Subject: subject})
}

// questsLinked is whether the alt check ties two accounts together, so a quest
// about other people does not count one person's own second account.
func (s *Server) questsLinked(ctx context.Context, a, b ids.ID) bool {
	alts, err := s.Store.AltAccounts(ctx, a)
	if err != nil {
		return true
	}
	for _, x := range alts {
		if x.UserID == b {
			return true
		}
	}
	return false
}

// questFriends counts two people who just became friends, each for the other,
// the first time only.
func (s *Server) questFriends(ctx context.Context, a, b *store.User) {
	if !questPerson(a) || !questPerson(b) {
		return
	}
	linked := s.questsLinked(ctx, a.ID, b.ID)
	for _, p := range [][2]*store.User{{a, b}, {b, a}} {
		first, err := s.Store.QuestFirstTime(ctx, p[0].ID, "friend", p[1].ID)
		if err != nil || !first || linked {
			continue
		}
		s.questNow(ctx, p[0], store.QuestFriends, 1, p[1].ID)
	}
}

// questJoined counts someone joining a server for the first time, unless its
// owner is their own second account.
func (s *Server) questJoined(ctx context.Context, me *store.User, srv *store.Server) {
	if !questPerson(me) || srv == nil {
		return
	}
	first, err := s.Store.QuestFirstTime(ctx, me.ID, "join", srv.ID)
	if err != nil || !first || srv.OwnerID == me.ID || s.questsLinked(ctx, me.ID, srv.OwnerID) {
		return
	}
	s.questNow(ctx, me, store.QuestJoins, 1, srv.ID)
}

// questTraded counts a trade that just happened, for both, when each side
// gave something and the two are not one person's accounts.
func (s *Server) questTraded(ctx context.Context, t *store.Trade) {
	if t == nil || t.Status != store.TradeAccepted || len(t.FromOffer) == 0 || len(t.ToOffer) == 0 || s.questsLinked(ctx, t.FromID, t.ToID) {
		return
	}
	for _, p := range [][2]ids.ID{{t.FromID, t.ToID}, {t.ToID, t.FromID}} {
		if u, err := s.Store.UserByID(ctx, p[0]); err == nil {
			s.questNow(ctx, u, store.QuestTrades, 1, p[1])
		}
	}
}

// questBugResolved counts a bug report or suggestion staff just resolved for
// whoever sent it, once however often it is reopened.
func (s *Server) questBugResolved(ctx context.Context, before string, b *store.BugReport) {
	if b == nil || before == "resolved" || b.Status != "resolved" {
		return
	}
	kind := store.QuestBugs
	if b.Kind == "suggestion" {
		kind = store.QuestIdeas
	}
	if u, err := s.Store.UserByID(ctx, b.UserID); err == nil {
		s.questNow(ctx, u, kind, 1, b.ID)
	}
}

// questsUpdate is the event that tells someone a quest finished, with the
// board as it is now.
type questsUpdate struct {
	*store.QuestBoard
	Completed []store.Quest `json:"completed"`
}

func (s *Server) emitQuests(ctx context.Context, uid ids.ID, done []store.Quest) {
	b, err := s.Store.QuestBoard(ctx, uid, time.Now())
	if err != nil {
		return
	}
	if done == nil {
		done = []store.Quest{}
	}
	s.Bcast.EmitUsers([]ids.ID{uid}, gateway.EvQuestsUpdate, questsUpdate{b, done})
	s.emitGems(ctx, uid)
}

var (
	errQuestNone = E(http.StatusNotFound, "quest_none", "That quest is finished or gone.")
	errQuestUsed = E(http.StatusConflict, "quest_used", "You can do that again tomorrow.")
)

func questError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrQuestNone):
		WriteError(w, errQuestNone)
	case errors.Is(err, store.ErrQuestUsed):
		WriteError(w, errQuestUsed)
	default:
		WriteError(w, err)
	}
}

func (s *Server) questBoardFor(w http.ResponseWriter, r *http.Request, me *store.User) {
	b, err := s.Store.QuestBoard(r.Context(), me.ID, time.Now())
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, b)
}

func (s *Server) handleQuests(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	if !questPerson(me) {
		WriteError(w, errTradePerson)
		return
	}
	s.questBoardFor(w, r, me)
}

func (s *Server) handleQuestReroll(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, errQuestNone)
		return
	}
	if !questPerson(me) {
		WriteError(w, errTradePerson)
		return
	}
	if err := s.Store.RerollQuest(r.Context(), me.ID, id, time.Now()); err != nil {
		questError(w, err)
		return
	}
	s.questForget(me.ID)
	s.questBoardFor(w, r, me)
}

func (s *Server) handleTrialSkip(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	if !questPerson(me) {
		WriteError(w, errTradePerson)
		return
	}
	if err := s.Store.SkipTrialQuest(r.Context(), me.ID, time.Now()); err != nil {
		questError(w, err)
		return
	}
	s.questForget(me.ID)
	s.questBoardFor(w, r, me)
}
