package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"evilion.xyz/chat/internal/ids"
)

// setQuest gives someone's open quest on track a known kind and target, since
// the draw is random.
func setQuest(t *testing.T, s *Store, uid ids.ID, track string, slot int64, kind string, target int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.QuestBoard(ctx, uid, time.Now()); err != nil {
		t.Fatal(err)
	}
	// every other quest waits on something the test never sends
	if _, err := s.db.W.ExecContext(ctx, `UPDATE quests SET kind = ? WHERE user_id = ? AND track != ?`, QuestIdeas, int64(uid), QuestBonus); err != nil {
		t.Fatal(err)
	}
	q := `UPDATE quests SET kind = ?, target = ? WHERE user_id = ? AND track = ? AND done_at = 0`
	args := []any{kind, target, int64(uid), track}
	if track == QuestDaily {
		q += ` AND slot = ?`
		args = append(args, slot)
	}
	if _, err := s.db.W.ExecContext(ctx, q, args...); err != nil {
		t.Fatal(err)
	}
}

// A quest pays once however many writes finish it together, into the wallet
// as the farming check has it: held during a check, nothing while earning is
// off. Its Gems never count as earned, so they cannot feed a Gems quest.
func TestQuestPaysOnceAndMindsTheWallet(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now()
	a := mkUser(t, s, "alice")
	setQuest(t, s, a.ID, QuestDaily, 1, QuestMessages, 3)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.AddQuestProgress(ctx, a.ID, []QuestStep{{Kind: QuestMessages, N: 3}}, now); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	w, _ := s.GemWallet(ctx, a.ID)
	if w.Gems != questDailyRewards[0] || w.Earned != 0 {
		t.Fatalf("paid %d Gems, %d earned", w.Gems, w.Earned)
	}

	b := mkUser(t, s, "bob")
	c := mkUser(t, s, "carol")
	if _, err := s.db.W.ExecContext(ctx, `INSERT INTO gem_wallets (user_id, state) VALUES (?, 'check')`, int64(b.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.W.ExecContext(ctx, `INSERT INTO gem_wallets (user_id, state, off_until) VALUES (?, 'off', ?)`, int64(c.ID), now.Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	for _, u := range []*User{b, c} {
		setQuest(t, s, u.ID, QuestDaily, 2, QuestCapsules, 1)
		if done, err := s.AddQuestProgress(ctx, u.ID, []QuestStep{{Kind: QuestCapsules, N: 1}}, now); err != nil || len(done) != 1 {
			t.Fatalf("%s: %v %v", u.Username, done, err)
		}
	}
	if w, _ := s.GemWallet(ctx, b.ID); w.Gems != 0 || w.Held != questDailyRewards[1] {
		t.Errorf("during a check: %d Gems, %d held", w.Gems, w.Held)
	}
	if w, _ := s.GemWallet(ctx, c.ID); w.Gems != 0 || w.Held != 0 {
		t.Errorf("earning off: %d Gems, %d held", w.Gems, w.Held)
	}
}

// A quest about different servers counts each once, and a Trial step's count
// starts again at zero on the step after it.
func TestQuestCountsEachOnceAndTheTrialStartsOver(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now()
	a := mkUser(t, s, "alice")
	setQuest(t, s, a.ID, QuestTrial, 0, QuestServers, 2)
	one, two := ids.New(), ids.New()
	for _, v := range []ids.ID{one, one, one} {
		if _, err := s.AddQuestProgress(ctx, a.ID, []QuestStep{{Kind: QuestServers, N: 1, Subject: v}}, now); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := s.QuestBoard(ctx, a.ID, now)
	if b.Trial.Progress != 1 || b.Cleared != 0 {
		t.Fatalf("one server three times: %+v", b.Trial)
	}
	done, err := s.AddQuestProgress(ctx, a.ID, []QuestStep{{Kind: QuestServers, N: 1, Subject: two}}, now)
	if err != nil || len(done) != 1 || done[0].Paid != QuestTrialReward(1) {
		t.Fatalf("finished: %v %v", done, err)
	}
	b, _ = s.QuestBoard(ctx, a.ID, now)
	if b.Cleared != 1 || b.Trial.Level != 2 || b.Trial.Progress != 0 || b.Trial.Kind == QuestServers {
		t.Fatalf("next step: %+v", b.Trial)
	}
}

// A quest that spends Gems pays less than doing it costs, at every Trial step
// and every daily slot, so quests never turn Gems into more Gems.
func TestQuestSpendingNeverPays(t *testing.T) {
	spend := map[string]bool{QuestCapsules: true, QuestFuses: true, QuestMaterials: true}
	for step := int64(1); step <= 5000; step++ {
		for k := range spend {
			if QuestTrialTarget(k, step)*GemCapsulePrice <= QuestTrialReward(step) {
				t.Fatalf("step %d: %s pays more than it costs", step, k)
			}
		}
	}
	for slot, pool := range questDailyPool {
		for _, p := range pool {
			if spend[p.kind] && p.target*GemCapsulePrice <= questDailyRewards[slot] {
				t.Fatalf("daily %s pays more than it costs", p.kind)
			}
		}
	}
}
