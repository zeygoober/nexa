package store

import (
	"context"
	"testing"
	"time"

	"evilion.xyz/chat/internal/ids"
)

// A streak is a permanent record people work for: a wrong write cannot be given back.
func TestRegularKeepsTheBestRun(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	u := mkUser(t, s, "regular")
	const d = int64(20000)
	day := func(n int64, wantStreak, wantBest int) {
		t.Helper()
		if err := s.MarkActiveDay(ctx, u.ID, time.Unix(n*86400+3600, 0)); err != nil {
			t.Fatal(err)
		}
		p, err := s.GetBadgeProgress(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Streak != wantStreak || p.BestStreak != wantBest {
			t.Fatalf("day %d: streak %d best %d, want %d and %d", n-d, p.Streak, p.BestStreak, wantStreak, wantBest)
		}
	}
	day(d, 1, 1)
	day(d, 1, 1) // the same day counts once
	day(d+1, 2, 2)
	day(d+2, 3, 3)
	day(d+5, 1, 3) // a gap starts again and the best stays
	day(d+6, 2, 3)
}

// What only exists to award badges goes with the account, and comes back for nobody.
func TestBadgeProgressGoesWithTheAccount(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	u := mkUser(t, s, "gone")
	stranger := ids.New() // a guest has no account row
	if err := s.AddCallSeconds(ctx, map[ids.ID]int64{u.ID: 60, stranger: 60}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCallSeconds(ctx, map[ids.ID]int64{u.ID: 30}); err != nil {
		t.Fatal(err)
	}
	if p, err := s.GetBadgeProgress(ctx, u.ID); err != nil || p.CallSeconds != 90 {
		t.Fatalf("call seconds %d, err %v", p.CallSeconds, err)
	}
	if p, _ := s.GetBadgeProgress(ctx, stranger); p != (BadgeProgress{}) {
		t.Fatalf("a guest got a record: %+v", p)
	}
	if err := s.DeleteAccount(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	// a flush still on its way when the account went
	if err := s.AddCallSeconds(ctx, map[ids.ID]int64{u.ID: 60}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.GetBadgeProgress(ctx, u.ID); p != (BadgeProgress{}) {
		t.Fatalf("a deleted account kept its record: %+v", p)
	}
}
