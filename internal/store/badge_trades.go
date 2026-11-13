package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// Trades: two people each offer badges of their own, either side possibly
// nothing, and watch each other's offer change. A trade lives as a system_trade
// message in their direct messages and is edited in a window (api/badge_trades.go).
// It happens when both have pressed Ready on the offers as they stand: any change
// to an offer clears both marks and counts up Version, and a Ready names the version
// it was given for, so nobody can swap something in under the other's click.
// Nothing is held back while it waits, so the moment it happens both sides are
// checked again.

// Statuses of a trade.
const (
	TradePending   = "pending"
	TradeAccepted  = "accepted"
	TradeDeclined  = "declined"
	TradeCancelled = "cancelled"
)

// Limits on a request, and on how many one person may leave waiting.
const (
	MaxTradeLines      = 20
	MaxTradeQty        = 999
	MaxPendingTrades   = 25
	MaxPendingPerRecip = 5
)

var (
	// ErrTradeLines is a request naming a badge that cannot be traded, an
	// amount out of range, or nothing on both sides.
	ErrTradeLines = errors.New("trade names badges that cannot be traded")
	// ErrTradeEmpty is a Ready on a trade with nothing in it.
	ErrTradeEmpty = errors.New("trade has nothing in it")
	// ErrTradeGone is a trade that is not there for this person.
	ErrTradeGone = errors.New("no such trade")
	// ErrTradeClosed is a trade already settled.
	ErrTradeClosed = errors.New("trade is no longer pending")
	// ErrTradeTooMany is too many requests waiting.
	ErrTradeTooMany = errors.New("too many trade requests waiting")
)

// ErrTradeChanged is a Ready for offers that are not the ones now on the table.
var ErrTradeChanged = errors.New("trade changed since it was seen")

// ErrTradeShort is a person who does not hold everything they offer.
type ErrTradeShort struct{ Who ids.ID }

func (e *ErrTradeShort) Error() string { return "a side no longer holds what the trade needs" }

// TradeLine is some copies of one badge, named by its Badge Index key. Badge is
// how to draw it, filled in when a trade is sent out.
type TradeLine struct {
	Key   string `json:"key"`
	Qty   int    `json:"qty"`
	Badge *Badge `json:"badge,omitempty"`
}

// Trade is a trade of two people: each side's offer, who has pressed Ready, and
// the version the offers are at.
type Trade struct {
	ID         ids.ID      `json:"id"`
	FromID     ids.ID      `json:"from_id"`
	ToID       ids.ID      `json:"to_id"`
	FromOffer  []TradeLine `json:"from_offer"`
	ToOffer    []TradeLine `json:"to_offer"`
	FromReady  bool        `json:"from_ready"`
	ToReady    bool        `json:"to_ready"`
	Version    int         `json:"version"`
	Status     string      `json:"status"`
	ChannelID  ids.ID      `json:"channel_id"`
	MessageID  ids.ID      `json:"message_id"`
	CreatedAt  int64       `json:"created_at"`
	ResolvedAt *int64      `json:"resolved_at"`
}

// CleanTradeLines checks what one side of a request names: each key a badge
// people can trade (staff made ones must exist), each amount from 1 to
// MaxTradeQty, the same badge named twice added together.
func (s *Store) CleanTradeLines(ctx context.Context, lines []TradeLine) ([]TradeLine, error) {
	sum := map[string]int{}
	var order []string
	var events []ids.ID
	for _, l := range lines {
		if _, _, ok := ParseTradeKey(l.Key); !ok || l.Qty < 1 || l.Qty > MaxTradeQty {
			return nil, ErrTradeLines
		}
		if _, seen := sum[l.Key]; !seen {
			order = append(order, l.Key)
			if rest, ok := strings.CutPrefix(l.Key, EventBadgePrefix); ok {
				if v, err := parseID(rest); err == nil {
					events = append(events, v)
				}
			}
		}
		sum[l.Key] += l.Qty
	}
	if len(order) > MaxTradeLines {
		return nil, ErrTradeLines
	}
	if len(events) > 0 {
		defs, err := s.badgeDefsByID(ctx, events)
		if err != nil {
			return nil, err
		}
		for _, id := range events {
			if defs[id] == nil {
				return nil, ErrTradeLines
			}
		}
	}
	out := make([]TradeLine, 0, len(order))
	for _, k := range order {
		if sum[k] > MaxTradeQty {
			return nil, ErrTradeLines
		}
		out = append(out, TradeLine{Key: k, Qty: sum[k]})
	}
	return out, nil
}

func parseID(s string) (ids.ID, error) {
	var v int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, ErrTradeLines
		}
		v = v*10 + int64(c-'0')
	}
	return ids.ID(v), nil
}

// Holdings is how many copies of each badge an account holds, trades included.
func (s *Store) Holdings(ctx context.Context, u *User) (map[string]int, error) {
	_, qty, err := s.heldBadges(ctx, u, time.Now())
	if qty == nil {
		qty = map[string]int{}
	}
	return qty, err
}

// HoldsAll is whether an account holds every line asked of it.
func holdsAll(qty map[string]int, lines []TradeLine) bool {
	for _, l := range lines {
		if qty[l.Key] < l.Qty {
			return false
		}
	}
	return true
}

// OpenTrade records a new, empty trade between two people.
func (s *Store) OpenTrade(ctx context.Context, t *Trade) error {
	if t.ID.IsZero() {
		t.ID = ids.New()
	}
	t.Status, t.CreatedAt, t.Version = TradePending, nowMs(), 0
	t.FromOffer, t.ToOffer = []TradeLine{}, []TradeLine{}
	return s.Tx(ctx, func(tx *db.Tx) error {
		var total, pair int
		if err := tx.QueryRowContext(ctx, `SELECT count(*), COALESCE(sum(CASE WHEN to_id = ? THEN 1 ELSE 0 END), 0) FROM badge_trades WHERE from_id = ? AND status = 'pending'`, int64(t.ToID), int64(t.FromID)).Scan(&total, &pair); err != nil {
			return err
		}
		if total >= MaxPendingTrades || pair >= MaxPendingPerRecip {
			return ErrTradeTooMany
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO badge_trades (id, from_id, to_id, status, channel_id, message_id, created_at) VALUES (?,?,?,?,?,?,?)`,
			int64(t.ID), int64(t.FromID), int64(t.ToID), TradePending, int64(t.ChannelID), int64(t.MessageID), t.CreatedAt)
		return err
	})
}

// PendingTradeBetween is the open trade of two people, whoever began it, or nil.
func (s *Store) PendingTradeBetween(ctx context.Context, a, b ids.ID) (*Trade, error) {
	t, err := scanTrade(s.db.R.QueryRowContext(ctx, `SELECT `+tradeCols+` FROM badge_trades WHERE status = 'pending' AND ((from_id = ? AND to_id = ?) OR (from_id = ? AND to_id = ?)) ORDER BY id DESC LIMIT 1`, int64(a), int64(b), int64(b), int64(a)))
	if errors.Is(err, ErrTradeGone) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, s.lookTrades(ctx, []*Trade{t})
}

func stripLooks(lines []TradeLine) []TradeLine {
	out := make([]TradeLine, len(lines))
	for i, l := range lines {
		out[i] = TradeLine{Key: l.Key, Qty: l.Qty}
	}
	return out
}

// DeleteTrade removes a request that never got its message.
func (s *Store) DeleteTrade(ctx context.Context, id ids.ID) {
	_, _ = s.db.W.ExecContext(ctx, `DELETE FROM badge_trades WHERE id = ?`, int64(id))
}

const tradeCols = `id, from_id, to_id, from_offer, to_offer, from_ready, to_ready, version, status, channel_id, message_id, created_at, resolved_at`

func scanTrade(r rowScanner) (*Trade, error) {
	var t Trade
	var from, to, ch, msg int64
	var give, want string
	var resolved sql.NullInt64
	var id int64
	var fr, tr int
	if err := r.Scan(&id, &from, &to, &give, &want, &fr, &tr, &t.Version, &t.Status, &ch, &msg, &t.CreatedAt, &resolved); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTradeGone
		}
		return nil, err
	}
	t.ID, t.FromID, t.ToID, t.ChannelID, t.MessageID = ids.ID(id), ids.ID(from), ids.ID(to), ids.ID(ch), ids.ID(msg)
	t.FromReady, t.ToReady = fr != 0, tr != 0
	if resolved.Valid {
		t.ResolvedAt = &resolved.Int64
	}
	if json.Unmarshal([]byte(give), &t.FromOffer) != nil || json.Unmarshal([]byte(want), &t.ToOffer) != nil {
		return nil, errors.New("trade lines unreadable")
	}
	if t.FromOffer == nil {
		t.FromOffer = []TradeLine{}
	}
	if t.ToOffer == nil {
		t.ToOffer = []TradeLine{}
	}
	return &t, nil
}

// TradeByID reads one trade, with its badges to draw.
func (s *Store) TradeByID(ctx context.Context, id ids.ID) (*Trade, error) {
	t, err := scanTrade(s.db.R.QueryRowContext(ctx, `SELECT `+tradeCols+` FROM badge_trades WHERE id = ?`, int64(id)))
	if err != nil {
		return nil, err
	}
	return t, s.lookTrades(ctx, []*Trade{t})
}

// hydrateTrades fills the trade of each system_trade message in msgs.
func (s *Store) hydrateTrades(ctx context.Context, msgs []*Message) error {
	byMsg := map[ids.ID]*Message{}
	var list []ids.ID
	for _, m := range msgs {
		if m.Type == "system_trade" {
			byMsg[m.ID] = m
			list = append(list, m.ID)
		}
	}
	if len(list) == 0 {
		return nil
	}
	rows, err := s.db.R.QueryContext(ctx, `SELECT `+tradeCols+` FROM badge_trades WHERE message_id IN (`+placeholders(len(list))+`)`, idArgs(list)...)
	if err != nil {
		return err
	}
	var found []*Trade
	for rows.Next() {
		t, err := scanTrade(rows)
		if err != nil {
			rows.Close()
			return err
		}
		found = append(found, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if err := s.lookTrades(ctx, found); err != nil {
		return err
	}
	for _, t := range found {
		if m := byMsg[t.MessageID]; m != nil {
			m.Trade = t
		}
	}
	return nil
}

// lookTrades gives every line of trades the badge to draw it with.
func (s *Store) lookTrades(ctx context.Context, trades []*Trade) error {
	need := map[string]bool{}
	var events []ids.ID
	for _, t := range trades {
		for _, l := range append(append([]TradeLine{}, t.FromOffer...), t.ToOffer...) {
			if need[l.Key] {
				continue
			}
			need[l.Key] = true
			if rest, ok := strings.CutPrefix(l.Key, EventBadgePrefix); ok {
				if v, err := parseID(rest); err == nil {
					events = append(events, v)
				}
			}
		}
	}
	var defs map[ids.ID]*BadgeDef
	if len(events) > 0 {
		var err error
		if defs, err = s.badgeDefsByID(ctx, events); err != nil {
			return err
		}
	}
	look := func(lines []TradeLine) {
		for i, l := range lines {
			id, level, ok := ParseTradeKey(l.Key)
			if !ok {
				continue
			}
			b := Badge{ID: id, Level: level}
			if rest, isEvent := strings.CutPrefix(l.Key, EventBadgePrefix); isEvent {
				v, _ := parseID(rest)
				d := defs[v]
				if d == nil {
					continue
				}
				b = d.badge()
			}
			lines[i].Badge = &b
		}
	}
	for _, t := range trades {
		look(t.FromOffer)
		look(t.ToOffer)
	}
	return nil
}

// take removes n copies of a key from someone, inside the accepting
// transaction. Traded copies go first and the earned one last; every removal is
// a conditional write, so two trades cannot both spend the same copy.
func take(ctx context.Context, tx *db.Tx, uid ids.ID, earned bool, key string, n int) (bool, error) {
	var copies, spent int
	err := tx.QueryRowContext(ctx, `SELECT copies, spent FROM badge_holdings WHERE user_id = ? AND badge = ?`, int64(uid), key).Scan(&copies, &spent)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	c := min(copies, n)
	rest := n - c
	if rest > 1 || (rest == 1 && (!earned || spent != 0)) {
		return false, nil
	}
	if c > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE badge_holdings SET copies = copies - ? WHERE user_id = ? AND badge = ? AND copies >= ?`, c, int64(uid), key, c)
		if err != nil {
			return false, err
		}
		if k, _ := res.RowsAffected(); k != 1 {
			return false, nil
		}
	}
	if rest == 1 {
		res, err := tx.ExecContext(ctx, `INSERT INTO badge_holdings (user_id, badge, copies, spent) VALUES (?, ?, 0, 1) ON CONFLICT(user_id, badge) DO UPDATE SET spent = 1 WHERE badge_holdings.spent = 0`, int64(uid), key)
		if err != nil {
			return false, err
		}
		if k, _ := res.RowsAffected(); k != 1 {
			return false, nil
		}
	}
	return true, nil
}

func give(ctx context.Context, tx *db.Tx, uid ids.ID, key string, n int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO badge_holdings (user_id, badge, copies, spent) VALUES (?, ?, ?, 0) ON CONFLICT(user_id, badge) DO UPDATE SET copies = badge_holdings.copies + excluded.copies`, int64(uid), key, n)
	return err
}

// side says whose offer and ready mark a person owns in a trade.
func (t *Trade) side(by ids.ID) (col string, ok bool) {
	switch by {
	case t.FromID:
		return "from", true
	case t.ToID:
		return "to", true
	}
	return "", false
}

// participant loads a trade for one of its two people, and says no to anyone else.
func (s *Store) participant(ctx context.Context, id, by ids.ID) (*Trade, error) {
	t, err := s.TradeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, ok := t.side(by); !ok {
		return nil, ErrTradeGone
	}
	return t, nil
}

func sameLines(a, b []TradeLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Qty != b[i].Qty {
			return false
		}
	}
	return true
}

// SetOffer replaces what one person offers, from badges they hold now. Both
// Ready marks go, and the version counts up, so nothing agreed to before the
// change still stands.
func (s *Store) SetOffer(ctx context.Context, id, by ids.ID, lines []TradeLine) (*Trade, error) {
	t, err := s.participant(ctx, id, by)
	if err != nil {
		return nil, err
	}
	if t.Status != TradePending {
		return nil, ErrTradeClosed
	}
	lines, err = s.CleanTradeLines(ctx, lines)
	if err != nil {
		return nil, err
	}
	me, err := s.UserByID(ctx, by)
	if err != nil {
		return nil, ErrTradeGone
	}
	qty, err := s.Holdings(ctx, me)
	if err != nil {
		return nil, err
	}
	if !holdsAll(qty, lines) {
		return nil, &ErrTradeShort{Who: by}
	}
	col, _ := t.side(by)
	current := t.FromOffer
	if col == "to" {
		current = t.ToOffer
	}
	if sameLines(current, lines) {
		return t, nil
	}
	raw, err := json.Marshal(stripLooks(lines))
	if err != nil {
		return nil, err
	}
	res, err := s.db.W.ExecContext(ctx, `UPDATE badge_trades SET `+col+`_offer = ?, from_ready = 0, to_ready = 0, version = version + 1 WHERE id = ? AND status = 'pending'`, string(raw), int64(id))
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrTradeClosed
	}
	return s.TradeByID(ctx, id)
}

// SetReady marks one person ready, or not, for the offers at the version they
// were looking at. When both are ready the trade happens, in the same
// transaction: every copy moves both ways or none, each removal checked as it is
// made, so a copy offered in two trades goes to the first.
func (s *Store) SetReady(ctx context.Context, id, by ids.ID, ready bool, version int) (*Trade, error) {
	t, err := s.participant(ctx, id, by)
	if err != nil {
		return nil, err
	}
	if t.Status != TradePending {
		return nil, ErrTradeClosed
	}
	if t.Version != version {
		return nil, ErrTradeChanged
	}
	col, _ := t.side(by)
	flag := 0
	if ready {
		if len(t.FromOffer) == 0 && len(t.ToOffer) == 0 {
			return nil, ErrTradeEmpty
		}
		flag = 1
	}
	var earned map[ids.ID]map[string]bool
	if ready {
		earned = map[ids.ID]map[string]bool{}
		for _, uid := range []ids.ID{t.FromID, t.ToID} {
			u, err := s.UserByID(ctx, uid)
			if err != nil || u.Flags&(NotPeople|FlagBot|FlagDeleted) != 0 {
				return nil, ErrTradeGone
			}
			levels, granted, err := s.naturalHoldings(ctx, u)
			if err != nil {
				return nil, err
			}
			earned[uid] = naturalKeys(levels, granted)
		}
	}
	happened := false
	err = s.Tx(ctx, func(tx *db.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE badge_trades SET `+col+`_ready = ? WHERE id = ? AND status = 'pending' AND version = ?`, flag, int64(id), version)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrTradeChanged
		}
		if !ready {
			return nil
		}
		var fr, tr int
		if err := tx.QueryRowContext(ctx, `SELECT from_ready, to_ready FROM badge_trades WHERE id = ?`, int64(id)).Scan(&fr, &tr); err != nil {
			return err
		}
		if fr == 0 || tr == 0 {
			return nil
		}
		res, err = tx.ExecContext(ctx, `UPDATE badge_trades SET status = ?, resolved_at = ? WHERE id = ? AND status = 'pending' AND version = ?`, TradeAccepted, nowMs(), int64(id), version)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrTradeClosed
		}
		type move struct {
			from, to ids.ID
			line     TradeLine
		}
		var moves []move
		for _, l := range t.FromOffer {
			moves = append(moves, move{t.FromID, t.ToID, l})
		}
		for _, l := range t.ToOffer {
			moves = append(moves, move{t.ToID, t.FromID, l})
		}
		// the same order every time, so two trades do not wait on each other
		sort.Slice(moves, func(i, j int) bool {
			if moves[i].from != moves[j].from {
				return moves[i].from < moves[j].from
			}
			return moves[i].line.Key < moves[j].line.Key
		})
		for _, m := range moves {
			ok, err := take(ctx, tx, m.from, earned[m.from][m.line.Key], m.line.Key, m.line.Qty)
			if err != nil {
				return err
			}
			if !ok {
				return &ErrTradeShort{Who: m.from}
			}
		}
		for _, m := range moves {
			if err := give(ctx, tx, m.to, m.line.Key, m.line.Qty); err != nil {
				return err
			}
		}
		happened = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if happened {
		s.exist.forget()
	}
	return s.TradeByID(ctx, id)
}

// EndTrade closes a trade before it happened: declined when the person it was
// opened with ends it, cancelled when the person who opened it does.
func (s *Store) EndTrade(ctx context.Context, id, by ids.ID) (*Trade, error) {
	t, err := s.participant(ctx, id, by)
	if err != nil {
		return nil, err
	}
	status := TradeCancelled
	if by == t.ToID {
		status = TradeDeclined
	}
	res, err := s.db.W.ExecContext(ctx, `UPDATE badge_trades SET status = ?, resolved_at = ?, from_ready = 0, to_ready = 0 WHERE id = ? AND status = 'pending'`, status, nowMs(), int64(id))
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrTradeClosed
	}
	return s.TradeByID(ctx, id)
}

// MyTradable is what someone can offer: every badge they hold, with how many
// copies of each, staff ranks and plans left out.
func (s *Store) MyTradable(ctx context.Context, u *User) ([]TradeLine, error) {
	qty, err := s.Holdings(ctx, u)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(qty))
	for k := range qty {
		if _, _, ok := ParseTradeKey(k); ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	t := &Trade{}
	for _, k := range keys {
		t.FromOffer = append(t.FromOffer, TradeLine{Key: k, Qty: qty[k]})
	}
	if err := s.lookTrades(ctx, []*Trade{t}); err != nil {
		return nil, err
	}
	if t.FromOffer == nil {
		t.FromOffer = []TradeLine{}
	}
	return t.FromOffer, nil
}
