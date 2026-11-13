package store

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Trading badges (migration 0063). An earned badge is worked out from what the
// account is or did and stored nowhere, so what trading changes is kept beside
// it, per Badge Index key (bug-3, og, event:12): the copies someone gained
// through trades (badge_holdings.copies), and whether they traded away the one
// copy they earned (spent). Someone holds copies, plus one if they earned it
// and have not spent it. An earned copy exists once and is never earned again,
// so a trade can move it but never make another; only trades make more.
// Nothing is written until a trade is accepted, since profiles read this on
// every open.

// tieredBadge is a badge with levels, each level a badge of its own in the Index.
type tieredBadge struct {
	id  string
	art string
	top int
}

// tieredBadges are the badges with levels, as the Badge Index names them.
var tieredBadges = []tieredBadge{
	{BadgeBugHunter, "bug", 5}, {BadgeIdeas, "ideas", 5}, {BadgeAppDeveloper, "bot-maker", 3},
	{BadgeGiftGiver, "gift", 3}, {BadgeFriendly, "friendly", 3}, {BadgeHost, "host", 3},
	{BadgeRegular, "regular", 4}, {BadgeOpenMic, "open-mic", 3}, {BadgeYears, "years", 50},
}

// TradeKey is how a held badge is named in trades and holdings: the Badge Index's
// own key, a tiered badge's level included.
func TradeKey(id string, level int) string {
	for _, t := range tieredBadges {
		if t.id == id {
			return t.art + "-" + strconv.Itoa(level)
		}
	}
	return id
}

// ParseTradeKey reads a key back into a badge id and level. It says no to
// anything people cannot trade: a staff rank, a plan, and whatever is no badge.
func ParseTradeKey(key string) (id string, level int, ok bool) {
	switch {
	case key == BadgeOG || key == BadgeBooster || key == BadgeContributor || IsHandBadge(key) || IsSecretBadge(key):
		return key, 0, true
	case strings.HasPrefix(key, EventBadgePrefix):
		n := key[len(EventBadgePrefix):]
		if _, err := strconv.ParseInt(n, 10, 64); err != nil || len(n) == 0 || len(n) > 19 || n[0] == '0' || n[0] == '-' || n[0] == '+' {
			return "", 0, false
		}
		return key, 0, true
	}
	for _, t := range tieredBadges {
		if rest, found := strings.CutPrefix(key, t.art+"-"); found {
			n, err := strconv.Atoi(rest)
			if err == nil && n >= 1 && n <= t.top && strconv.Itoa(n) == rest {
				return t.id, n, true
			}
		}
	}
	return "", 0, false
}

// naturalKeys is every badge an account earned or was given, by key: each level
// a tiered badge has reached counts, as the Badge Index counts them.
func naturalKeys(levels map[string]int, granted map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, t := range tieredBadges {
		for n := 1; n <= min(levels[t.id], t.top); n++ {
			out[TradeKey(t.id, n)] = true
		}
	}
	for _, id := range []string{BadgeOG, BadgeBooster} {
		if levels[id] > 0 {
			out[id] = true
		}
	}
	for id := range granted {
		if _, _, ok := ParseTradeKey(id); ok {
			out[id] = true
		}
	}
	return out
}

// naturalHoldings is what an account earned or was given, by badge id and
// level, overrides applied and trades left out.
func (s *Store) naturalHoldings(ctx context.Context, u *User) (levels map[string]int, granted map[string]bool, err error) {
	levels, _, err = s.naturalLevels(ctx, u, time.Now())
	if err != nil {
		return nil, nil, err
	}
	over, err := s.BadgeOverrides(ctx, u.ID)
	if err != nil {
		return nil, nil, err
	}
	for b, l := range over {
		levels[b] = l
	}
	granted, err = s.grantedBadges(ctx, u.ID)
	return levels, granted, err
}

// holdingRow is what trades changed for one key.
type holdingRow struct {
	copies int
	spent  bool
}

func (s *Store) holdingRows(ctx context.Context, uid int64) (map[string]holdingRow, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT badge, copies, spent FROM badge_holdings WHERE user_id = ?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]holdingRow{}
	for rows.Next() {
		var k string
		var r holdingRow
		var spent int
		if err := rows.Scan(&k, &r.copies, &spent); err != nil {
			return nil, err
		}
		r.spent = spent != 0
		out[k] = r
	}
	return out, rows.Err()
}

// quantities is how many copies of each key someone holds.
func quantities(nat map[string]bool, rows map[string]holdingRow) map[string]int {
	qty := map[string]int{}
	for k := range nat {
		if !rows[k].spent {
			qty[k] = 1
		}
	}
	for k, r := range rows {
		qty[k] += r.copies
		if qty[k] <= 0 {
			delete(qty, k)
		}
	}
	return qty
}

// applyHoldings turns what an account earned into what it holds after its
// trades: levels and granted are changed in place to say so, and the quantity of
// every key held and the badges trades touched (whose count, the join place of
// an OG, no longer says anything) come back.
func (s *Store) applyHoldings(ctx context.Context, uid int64, levels map[string]int, granted map[string]bool) (qty map[string]int, touched map[string]bool, err error) {
	rows, err := s.holdingRows(ctx, uid)
	if err != nil {
		return nil, nil, err
	}
	qty = quantities(naturalKeys(levels, granted), rows)
	touched = map[string]bool{}
	if len(rows) == 0 {
		return qty, touched, nil
	}
	for k := range rows {
		if id, _, ok := ParseTradeKey(k); ok {
			touched[id] = true
		}
	}
	for _, t := range tieredBadges {
		lvl := 0
		for n := 1; n <= t.top; n++ {
			if qty[TradeKey(t.id, n)] > 0 {
				lvl = n
			}
		}
		if lvl > 0 {
			levels[t.id] = lvl
		} else {
			delete(levels, t.id)
		}
	}
	for _, id := range []string{BadgeOG, BadgeBooster} {
		if qty[id] > 0 {
			levels[id] = 1
		} else {
			delete(levels, id)
		}
	}
	for id := range granted {
		if _, _, ok := ParseTradeKey(id); ok {
			delete(granted, id)
		}
	}
	for k := range qty {
		id, _, ok := ParseTradeKey(k)
		if !ok || id != k || k == BadgeOG || k == BadgeBooster {
			continue
		}
		granted[k] = true
	}
	return qty, touched, nil
}
