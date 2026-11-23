package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"evilion.xyz/chat/internal/db"
)

func fundGems(t *testing.T, s *Store, u *User, n int64) {
	t.Helper()
	if err := s.Tx(context.Background(), func(tx *db.Tx) error { return giveGems(context.Background(), tx, u.ID, n, "test", "") }); err != nil {
		t.Fatal(err)
	}
}

// Gems are spent once: two capsules opened at the same moment with the price
// of one give one collectible, and fusing needs every copy it takes.
func TestGemCapsuleSpendsOnce(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	staff := mkUser(t, s, "staff")
	u := mkUser(t, s, "alice")
	d := &BadgeDef{Name: "Comet", Icon: "star", Color: 0x336699, Color2: -1, Glyph: "light", Shape: "circle", IndexKind: BadgeExclusive, Rarity: "common", Art: "moon", CreatedBy: staff.ID}
	if err := s.CreateBadgeDef(ctx, d); err != nil {
		t.Fatal(err)
	}
	fundGems(t, s, u, GemCapsulePrice)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, errs[i] = s.OpenCapsule(ctx, u.ID) }()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if !errors.Is(err, ErrGemsShort) {
			t.Fatalf("open: %v", err)
		}
	}
	w, _ := s.GemWallet(ctx, u.ID)
	if ok != 1 || w.Gems != 0 {
		t.Fatalf("%d capsules opened, %d Gems left", ok, w.Gems)
	}
	key := TradeKey(EventBadgePrefix+d.ID.String(), 1)
	if _, _, err := s.Fuse(ctx, u.ID, key, 2); !errors.Is(err, ErrGemsShort) {
		t.Fatalf("fused two copies holding one: %v", err)
	}
	// a fuse that fails still spends its copies; a full set never fails
	was := fuseRoll
	t.Cleanup(func() { fuseRoll = was })
	fuseRoll = func(int) (int, error) { return 1, nil }
	if _, won, err := s.Fuse(ctx, u.ID, key, 1); err != nil || won {
		t.Fatalf("a 20%% fuse rolling 1 worked: %v %v", won, err)
	}
	if err := s.Tx(ctx, func(tx *db.Tx) error { return give(ctx, tx, u.ID, key, GemFuseCost) }); err != nil {
		t.Fatal(err)
	}
	fuseRoll = func(int) (int, error) { return GemFuseCost - 1, nil }
	if next, won, err := s.Fuse(ctx, u.ID, key, GemFuseCost); err != nil || !won || next != TradeKey(EventBadgePrefix+d.ID.String(), 2) {
		t.Fatalf("a full set: %q %v %v", next, won, err)
	}
	if _, _, err := s.Fuse(ctx, u.ID, key, 1); !errors.Is(err, ErrGemsShort) {
		t.Fatalf("the failed fuse's copy was given back: %v", err)
	}
	// a staff badge that is no collectible cannot be given by hand either way round
	if err := s.UpdateBadgeDef(ctx, &BadgeDef{ID: d.ID, Name: "Comet", Icon: "star", Color: 0x336699, Color2: -1, Glyph: "light", Shape: "circle", IndexKind: BadgeExclusive}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.BadgeDef(ctx, d.ID); got.Rarity != "common" {
		t.Fatalf("a collectible stopped being one: %q", got.Rarity)
	}
}

// A day that looks scripted holds what it earns until a captcha; the same
// again within a week goes to staff, whose dismissal gives the Gems back and
// whose action takes them and stops earning.
func TestGemFarmingHoldsUntilCheckedOrDecided(t *testing.T) {
	was := gemWatchMin
	gemWatchMin = 10
	t.Cleanup(func() { gemWatchMin = was })
	s := newStore(t)
	ctx := context.Background()
	u := mkUser(t, s, "bot")
	staff := mkUser(t, s, "staff")
	at := time.Date(2026, 11, 16, 3, 0, 0, 0, time.UTC)
	script := func(n int) *GemEarn {
		var last *GemEarn
		for range n {
			at = at.Add(time.Minute)
			got, err := s.EarnGem(ctx, u.ID, 100, GemSignal{Alone: true, Size: 120}, at)
			if err != nil {
				t.Fatal(err)
			}
			if got != nil && got.Flag != "" {
				last = got
			}
		}
		return last
	}
	if f := script(10); f == nil || f.Flag != GemStateCheck {
		t.Fatalf("no check: %+v", f)
	}
	if got, _ := s.EarnGem(ctx, u.ID, 100, GemSignal{Alone: true, Size: 120}, at.Add(30*time.Second)); got != nil {
		t.Fatal("earned inside the minute")
	}
	script(3)
	w, _ := s.GemWallet(ctx, u.ID)
	if w.Held != 4 || w.Gems != 9 {
		t.Fatalf("held %d, balance %d", w.Held, w.Gems)
	}
	if _, err := s.ConfirmGems(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if w, _ = s.GemWallet(ctx, u.ID); w.Held != 0 || w.Gems != 13 || w.State != "" {
		t.Fatalf("after the captcha: %+v", w)
	}
	// the same pattern a few days later goes straight to staff
	at = at.Add(3 * 24 * time.Hour)
	if f := script(10); f == nil || f.Flag != GemStateReport {
		t.Fatalf("no report: %+v", f)
	}
	if _, err := s.ConfirmGems(ctx, u.ID); !errors.Is(err, ErrGemsReported) {
		t.Fatalf("a captcha settled a report: %v", err)
	}
	report := mkUser(t, s, "stand-in-report").ID
	if err := s.SetGemReport(ctx, u.ID, report); err != nil {
		t.Fatal(err)
	}
	w, _ = s.GemWallet(ctx, u.ID)
	held := w.Held
	c := &ModCase{UserID: u.ID, ModeratorID: staff.ID, Action: ActionGemFarming}
	if _, _, err := s.CloseGemReport(ctx, report, true, c, &ModLogEntry{Action: ActionGemFarming, ModeratorID: staff.ID}); err != nil {
		t.Fatal(err)
	}
	w, _ = s.GemWallet(ctx, u.ID)
	if held == 0 || w.Held != 0 || w.Gems != 22 || w.State != GemStateOff || c.Number == 0 {
		t.Fatalf("after action: held was %d, now %+v, case %d", held, w, c.Number)
	}
	if got, _ := s.EarnGem(ctx, u.ID, 100, GemSignal{Size: 300}, at.Add(time.Hour)); got != nil {
		t.Fatal("earned while earning is off")
	}
	// a second decision on the same report changes nothing
	if who, _, _ := s.CloseGemReport(ctx, report, false, nil, nil); who != 0 {
		t.Fatal("a closed report was settled again")
	}
}

// Nothing bought adds to a team: plans, boosts and gifts make no Gems.
func TestGemTeamCountsNothingBought(t *testing.T) {
	for _, id := range []string{BadgePlan, BadgeBooster, BadgeGiftGiver, BadgeStaff} {
		if p := gemPower(Badge{ID: id, Level: 3}); p != 0 {
			t.Errorf("%s adds %d", id, p)
		}
	}
	if p := gemPower(Badge{ID: "event:1", Rarity: "huge", Level: 5}); p != 200 {
		t.Errorf("a prism Huge adds %d", p)
	}
}

// A collectible held shows on its holder's profile, which is where the team is
// read from (it once never did, so collectibles added nothing), and in the
// index as each of its materials; one not held there is its drawing blacked
// out, with its name only on the preview its card reads.
func TestCollectiblesShowAndLockInTheIndex(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	staff := mkUser(t, s, "staff")
	u := mkUser(t, s, "alice")
	other := mkUser(t, s, "bob")
	d := &BadgeDef{Name: "Paper Lantern", Icon: "star", Color: 0x336699, Color2: -1, Glyph: "light", Shape: "circle", IndexKind: BadgeExclusive, Rarity: "common", Art: "ghost", CreatedBy: staff.ID}
	if err := s.CreateBadgeDef(ctx, d); err != nil {
		t.Fatal(err)
	}
	fundGems(t, s, u, GemCapsulePrice)
	if _, _, err := s.OpenCapsule(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, mult, err := s.GemTeam(ctx, u); err != nil || mult != 100+gemRarityPower["common"] {
		t.Fatalf("a bronze common makes the team %d%% (%v)", mult, err)
	}
	mine, err := s.MyBadgeIndex(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := s.MyBadgeIndex(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	held, locked := 0, 0
	for _, e := range mine.Entries {
		if e.Category == BadgeCollectible && e.Badge != nil {
			held++
		}
	}
	for _, e := range theirs.Entries {
		if e.Category != BadgeCollectible {
			continue
		}
		if e.Badge != nil || e.Art != "col-ghost" || e.Preview == nil || e.Preview.Name != "Paper Lantern" || e.Qty != 0 {
			t.Fatalf("a collectible bob lacks reads %+v", e)
		}
		locked++
	}
	if locked != GemMaterials || held != 1 {
		t.Fatalf("%d locked, %d held", locked, held)
	}
}

// Scripts that sent a little slower than once a minute scored 45 and were
// never flagged (2026-11-19, these rows from the live server); a person
// chatting nonstop in a busy place, or alone in their own words, is not.
func TestGemScoreCatchesSlowScripts(t *testing.T) {
	for _, c := range []struct {
		name string
		d    gemDay
		flag bool
	}{
		{"666", gemDay{earns: 597, quick: 272, alone: 593, hours: 36863, sizes: 1064960}, true},
		{"mm", gemDay{earns: 593, quick: 275, alone: 593, hours: 4095, sizes: 16384}, true},
		{"busy person", gemDay{earns: 400, quick: 390, alone: 20, hours: 0xffff, sizes: 0xfff0}, false},
		{"talking to themselves", gemDay{earns: 60, quick: 58, alone: 60, hours: 0x7, sizes: 0xff00}, false},
	} {
		if got := gemScore(c.d) >= gemFlagScore; got != c.flag {
			t.Errorf("%s: score %d, flagged %v", c.name, gemScore(c.d), got)
		}
	}
}
