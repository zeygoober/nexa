package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// BadgeProgress is what Regular and Open Mic count, for one account.
type BadgeProgress struct {
	LastDay     int64 `json:"last_day"` // days since 1970 in UTC; 0 for never
	Streak      int   `json:"streak"`
	BestStreak  int   `json:"best_streak"`
	CallSeconds int64 `json:"call_seconds"`
}

// utcDay numbers the UTC day a moment falls in.
func utcDay(t time.Time) int64 { return t.Unix() / 86400 }

// nextStreak is the run of days after an account is active on today: the next
// day carries it on, a gap starts it again, and the best is never given back.
func nextStreak(p BadgeProgress, today int64) (streak, best int) {
	switch {
	case p.LastDay == today-1:
		streak = p.Streak + 1
	case p.LastDay >= today: // already counted, or a clock that went back
		streak = p.Streak
	default:
		streak = 1
	}
	best = p.BestStreak
	if streak > best {
		best = streak
	}
	return streak, best
}

// GetBadgeProgress reads an account's record; an account without one has done
// nothing yet.
func (s *Store) GetBadgeProgress(ctx context.Context, uid ids.ID) (BadgeProgress, error) {
	return badgeProgressFrom(s.db.R.QueryRowContext(ctx, badgeProgressSQL, int64(uid)))
}

const badgeProgressSQL = `SELECT last_day, streak, best_streak, call_seconds FROM badge_progress WHERE user_id = ?`

func badgeProgressFrom(r rowScanner) (BadgeProgress, error) {
	var p BadgeProgress
	err := r.Scan(&p.LastDay, &p.Streak, &p.BestStreak, &p.CallSeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return BadgeProgress{}, nil
	}
	return p, err
}

// onlyPeople ends the SELECT that feeds an upsert below: what has no such
// account row (a guest) or is no person (a bot, a deleted account) writes nothing.
// The values there are CAST because Postgres would otherwise take them for text.
const onlyPeople = ` FROM users WHERE id = ? AND flags & ? = 0 ON CONFLICT(user_id) DO UPDATE SET `

// MarkActiveDay counts today towards an account's run of days. Memory keeps the
// accounts already counted today, so it costs one write a day each however
// often it is asked.
func (s *Store) MarkActiveDay(ctx context.Context, uid ids.ID, now time.Time) error {
	today := utcDay(now)
	a := &s.activeDays
	a.Lock()
	if a.day != today || a.seen == nil {
		a.day, a.seen = today, map[ids.ID]struct{}{}
	}
	_, done := a.seen[uid]
	a.seen[uid] = struct{}{}
	a.Unlock()
	if done {
		return nil
	}
	err := s.Tx(ctx, func(tx *db.Tx) error {
		p, err := badgeProgressFrom(tx.QueryRowContext(ctx, badgeProgressSQL, int64(uid)))
		if err != nil {
			return err
		}
		streak, best := nextStreak(p, today)
		last := max(p.LastDay, today)
		_, err = tx.ExecContext(ctx, `INSERT INTO badge_progress (user_id, last_day, streak, best_streak) SELECT id, CAST(? AS BIGINT), CAST(? AS BIGINT), CAST(? AS BIGINT)`+onlyPeople+
			`last_day = excluded.last_day, streak = excluded.streak, best_streak = excluded.best_streak`,
			last, streak, best, int64(uid), int64(NotPeople|FlagBot|FlagDeleted))
		return err
	})
	if err != nil {
		// the next heartbeat tries again
		a.Lock()
		delete(a.seen, uid)
		a.Unlock()
	}
	return err
}

// AddCallSeconds adds time spent in calls with someone else.
func (s *Store) AddCallSeconds(ctx context.Context, seconds map[ids.ID]int64) error {
	if len(seconds) == 0 {
		return nil
	}
	return s.Tx(ctx, func(tx *db.Tx) error {
		for uid, n := range seconds {
			if _, err := tx.ExecContext(ctx, `INSERT INTO badge_progress (user_id, call_seconds) SELECT id, CAST(? AS BIGINT)`+onlyPeople+
				`call_seconds = badge_progress.call_seconds + excluded.call_seconds`,
				n, int64(uid), int64(NotPeople|FlagBot|FlagDeleted)); err != nil {
				return err
			}
		}
		return nil
	})
}
