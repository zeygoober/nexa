package store

import (
	"context"
	"sort"
	"strings"

	"evilion.xyz/chat/internal/ids"
)

// BadgeOwner is someone holding copies of a badge now, the owner's view of it
// (api/moderation_owner_badges.go): earned or given copies not traded away, plus
// what trades gave, each level or material its own line. The holders list says
// who earned or was given a badge; this says who has it.
type BadgeOwner struct {
	UserID ids.ID      `json:"user_id"`
	Lines  []TradeLine `json:"lines"`
	Total  int         `json:"total"`
	// Traded is someone whose copies trades changed, gained or given away.
	Traded bool `json:"traded,omitempty"`
}

// BadgeOwners lists everyone holding a badge by its id (bug_hunter, og,
// event:12; a collectible's every material), most copies first. It works as the
// "X exist" count does (countTrades): who earned or was given it, with the
// holdings trades wrote laid over, so it never asks each account for its badges.
// Staff ranks and plans cannot be traded and have no owners list.
func (s *Store) BadgeOwners(ctx context.Context, badge string) ([]BadgeOwner, error) {
	notPeople := int64(NotPeople | FlagBot | FlagDeleted)
	natural := map[ids.ID]map[string]bool{}
	add := func(uid ids.ID, key string) {
		if natural[uid] == nil {
			natural[uid] = map[string]bool{}
		}
		natural[uid][key] = true
	}
	var tier *tieredBadge
	for i := range tieredBadges {
		if tieredBadges[i].id == badge {
			tier = &tieredBadges[i]
		}
	}
	switch {
	case tier != nil:
		all, _, err := s.allBadgeHolders(ctx, badge)
		if err != nil {
			return nil, err
		}
		for _, h := range all {
			for n := 1; n <= min(h.Level, tier.top); n++ {
				add(h.UserID, TradeKey(badge, n))
			}
		}
	case badge == BadgeOG || badge == BadgeBooster:
		all, _, err := s.allBadgeHolders(ctx, badge)
		if err != nil {
			return nil, err
		}
		for _, h := range all {
			add(h.UserID, badge)
		}
	case IsGrantableBadge(badge) || IsSecretBadge(badge):
		// a collectible is never in user_badges, so only its holdings below count
		rows, err := s.db.R.QueryContext(ctx, `SELECT u.id FROM user_badges g JOIN users u ON u.id = g.user_id WHERE g.badge = ? AND u.flags & ? = 0`, badge, notPeople)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			add(ids.ID(id), badge)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	default:
		return nil, ErrBadBadge
	}

	// what trades changed, read by the key's own badge so event:12 never takes event:123's
	held := map[ids.ID]map[string]holdingRow{}
	rows, err := s.db.R.QueryContext(ctx, `SELECT h.user_id, h.badge, h.copies, h.spent FROM badge_holdings h JOIN users u ON u.id = h.user_id WHERE u.flags & ? = 0 AND (h.badge = ? OR h.badge LIKE ? ESCAPE '\')`, notPeople, badge, likePrefix(holdingsPrefix(badge, tier)))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var key string
		var r holdingRow
		var spent int
		if err := rows.Scan(&id, &key, &r.copies, &spent); err != nil {
			rows.Close()
			return nil, err
		}
		if of, _, ok := ParseTradeKey(key); !ok || of != badge {
			continue
		}
		r.spent = spent != 0
		uid := ids.ID(id)
		if held[uid] == nil {
			held[uid] = map[string]holdingRow{}
		}
		held[uid][key] = r
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	people := map[ids.ID]bool{}
	for uid := range natural {
		people[uid] = true
	}
	for uid := range held {
		people[uid] = true
	}
	out := make([]BadgeOwner, 0, len(people))
	for uid := range people {
		qty := quantities(natural[uid], held[uid])
		o := BadgeOwner{UserID: uid, Lines: []TradeLine{}, Traded: len(held[uid]) > 0}
		for k, n := range qty {
			o.Lines = append(o.Lines, TradeLine{Key: k, Qty: n})
			o.Total += n
		}
		if o.Total == 0 {
			continue
		}
		// highest level or material first
		sort.Slice(o.Lines, func(i, j int) bool {
			_, a, _ := ParseTradeKey(o.Lines[i].Key)
			_, b, _ := ParseTradeKey(o.Lines[j].Key)
			return a > b
		})
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].UserID < out[j].UserID
	})
	// one look-up of the badges to draw, for every line at once
	view := make([]*Trade, len(out))
	for i := range out {
		view[i] = &Trade{FromOffer: out[i].Lines}
	}
	if err := s.lookTrades(ctx, view); err != nil {
		return nil, err
	}
	return out, nil
}

// holdingsPrefix narrows the holdings read to a badge's keys: a tiered badge's
// levels (bug-), a collectible's materials (event:12-). ParseTradeKey has the
// last word, so a prefix only has to catch every key, not only them.
func holdingsPrefix(badge string, tier *tieredBadge) string {
	if tier != nil {
		return tier.art + "-"
	}
	return badge + "-"
}

// TradeLog is every trade newest first, the owner's Trade Log: of one status
// when status is set, with someone whose username starts with who on either side
// when who is set, before the trade id before when it is set. Lines carry what
// draws them.
func (s *Store) TradeLog(ctx context.Context, status, who string, before ids.ID, limit int) ([]*Trade, error) {
	q := `SELECT ` + prefixCols("t.", tradeCols) + ` FROM badge_trades t`
	var where []string
	var args []any
	if who != "" {
		q += ` JOIN users f ON f.id = t.from_id JOIN users o ON o.id = t.to_id`
		like := likePrefix(strings.ToLower(who))
		where = append(where, `(lower(f.username) LIKE ? ESCAPE '\' OR lower(o.username) LIKE ? ESCAPE '\')`)
		args = append(args, like, like)
	}
	if status != "" {
		where = append(where, `t.status = ?`)
		args = append(args, status)
	}
	if !before.IsZero() {
		where = append(where, `t.id < ?`)
		args = append(args, int64(before))
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY t.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Trade{}
	for rows.Next() {
		t, err := scanTrade(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.lookTrades(ctx, out)
}

// prefixCols names every column of a list with a table's alias.
func prefixCols(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = alias + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}

// likePrefix is a LIKE pattern for text starting with s, its wildcards escaped.
func likePrefix(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s) + "%"
}
