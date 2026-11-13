package store

import (
	"context"
	"strconv"
	"sync"
	"time"

	"evilion.xyz/chat/internal/ids"
)

// badgeCountsFresh is how long a count of who holds what is used before it is
// worked out again. Earned badges are stored nowhere, so counting them walks
// every kind of thing that earns one: never for each profile opened.
const badgeCountsFresh = 5 * time.Minute

// badgeCounts is how many people hold each badge, by badgeCountKey. A level
// counts everyone who has reached it, as the Badge Index does.
type badgeCounts struct {
	n     map[string]int
	owner ids.ID
}

// badgeCountCache keeps the latest count. An old one is answered while a new
// one is made in the background; only the first, or the first after staff
// changed who holds what, makes its asker wait.
type badgeCountCache struct {
	mu   sync.Mutex
	now  *badgeCounts
	at   time.Time
	busy bool
}

// forget drops the count, so a badge staff just gave counts at once.
func (c *badgeCountCache) forget() {
	c.mu.Lock()
	c.now = nil
	c.mu.Unlock()
}

// badgeCounts answers the latest count; nil when it could not be made.
func (s *Store) badgeCounts(ctx context.Context) *badgeCounts {
	c := &s.exist
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.now != nil {
		if time.Since(c.at) >= badgeCountsFresh && !c.busy {
			c.busy = true
			go s.refreshBadgeCounts()
		}
		return c.now
	}
	n, err := s.countBadges(ctx)
	if err != nil {
		return nil
	}
	c.now, c.at = n, time.Now()
	return n
}

func (s *Store) refreshBadgeCounts() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	n, err := s.countBadges(ctx)
	c := &s.exist
	c.mu.Lock()
	defer c.mu.Unlock()
	c.busy = false
	if err != nil {
		c.at = time.Now() // try again when this one would have aged
		return
	}
	c.now, c.at = n, time.Now()
}

func badgeCountKey(id string, level int) string {
	if level > 0 {
		return id + ":" + strconv.Itoa(level)
	}
	return id
}

// staffCountKey names the highest rank of an account, as the apps draw it.
func staffCountKey(uid ids.ID, flags int64, owner ids.ID) string {
	switch {
	case uid == owner:
		return BadgeStaff + ":owner"
	case flags&FlagAdmin != 0:
		return BadgeStaff + ":developer"
	case flags&FlagStaffAdmin != 0:
		return BadgeStaff + ":admin"
	case flags&FlagStaffMod != 0:
		return BadgeStaff + ":mod"
	}
	return BadgeStaff + ":helper"
}

// countBadges works out how many hold every badge.
func (s *Store) countBadges(ctx context.Context) (*badgeCounts, error) {
	out := map[string]int{}
	people := int64(NotPeople | FlagBot | FlagDeleted)
	for badge := range OverridableBadges {
		all, _, err := s.allBadgeHolders(ctx, badge)
		if err != nil {
			return nil, err
		}
		perLevel, top := map[int]int{}, 0
		for _, h := range all {
			perLevel[h.Level]++
			if h.Level > top {
				top = h.Level
			}
		}
		out[badge] = perLevel[0]
		reached := 0
		for l := top; l >= 1; l-- {
			reached += perLevel[l]
			out[badgeCountKey(badge, l)] = reached
		}
	}
	tally := func(q string, args ...any) error {
		rows, err := s.db.R.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int
			if err := rows.Scan(&k, &n); err != nil {
				return err
			}
			out[k] = n
		}
		return rows.Err()
	}
	if err := tally(`SELECT g.badge, count(*) FROM user_badges g JOIN users u ON u.id = g.user_id WHERE u.flags & ? = 0 GROUP BY g.badge`, people); err != nil {
		return nil, err
	}
	if err := tally(`SELECT 'plan:' || u.plan, count(*) FROM users u WHERE u.plan IN ('supporter', 'pro') AND u.flags & ? = 0 GROUP BY u.plan`, people); err != nil {
		return nil, err
	}
	owner, err := s.InstanceOwnerID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.R.QueryContext(ctx, `SELECT id, flags FROM users WHERE flags & ? != 0 AND flags & ? = 0`, int64(StaffFlags), people)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, flags int64
		if err := rows.Scan(&id, &flags); err != nil {
			return nil, err
		}
		out[staffCountKey(ids.ID(id), flags, owner)]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.countTrades(ctx, out); err != nil {
		return nil, err
	}
	return &badgeCounts{n: out, owner: owner}, nil
}

// countTrades corrects the counts for what trades changed: they count copies,
// so someone who traded away the copy they earned no longer adds one, and
// someone given three adds three. Only accounts with trades are walked.
func (s *Store) countTrades(ctx context.Context, out map[string]int) error {
	rows, err := s.db.R.QueryContext(ctx, `SELECT DISTINCT user_id FROM badge_holdings`)
	if err != nil {
		return err
	}
	var traders []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		traders = append(traders, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range traders {
		u, err := s.UserByID(ctx, ids.ID(id))
		if err != nil || u == nil || u.Flags&(NotPeople|FlagBot|FlagDeleted) != 0 {
			continue
		}
		levels, granted, err := s.naturalHoldings(ctx, u)
		if err != nil {
			return err
		}
		held, err := s.holdingRows(ctx, id)
		if err != nil {
			return err
		}
		nat := naturalKeys(levels, granted)
		qty := quantities(nat, held)
		// the counts above hold one for each earned copy; what is held now differs by this
		change := map[string]int{}
		for k := range nat {
			change[k]--
		}
		for k, n := range qty {
			change[k] += n
		}
		for k, d := range change {
			if d != 0 {
				out[tradeCountKey(k)] += d
			}
		}
	}
	return nil
}

// tradeCountKey is where a key is counted: a level in its badge's reached
// counts, anything else under its id.
func tradeCountKey(key string) string {
	id, level, _ := ParseTradeKey(key)
	return badgeCountKey(id, level)
}

// of is how many hold this badge of u's. Someone holds it, so never under one,
// however old the count; 0 only when there is no count at all.
func (c *badgeCounts) of(b Badge, u *User) int {
	if c == nil {
		return 0
	}
	key := badgeCountKey(b.ID, b.Level)
	switch b.ID {
	case BadgeStaff:
		key = staffCountKey(u.ID, u.Flags, c.owner)
	case BadgePlan:
		key = BadgePlan + ":" + u.Plan
	}
	return max(c.n[key], 1)
}
