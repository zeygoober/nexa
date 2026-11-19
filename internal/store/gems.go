package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"math/bits"
	"sort"
	"time"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// Gems (migration 0064). A person earns one for a message, at most one a
// minute, times their team: the first five badges on their profile. Gems buy
// capsules, which give a collectible (a staff-made badge with a rarity) at the
// odds GemOdds says; five copies of a collectible fuse into one of the next
// material. Gems can be traded. Nothing about Gems is bought with money.
//
// Earning writes the wallet and one row of timing per day (gem_watch), never
// what was said or where. A day that looks like a script (gemScore) puts the
// account in a check: what it earns is held until it shows it is there (a
// captcha). Ignoring the check while the pattern goes on, or being flagged
// again soon after one, files a Gem Farming report for staff, and the held Gems
// wait on their decision.

const (
	// GemsKey names Gems in a trade's lines.
	GemsKey = "gems"
	// GemMaterials are a collectible's materials: bronze, silver, gold, emerald, prism.
	GemMaterials = 5
	// GemCapsulePrice is what one capsule costs.
	GemCapsulePrice = 50
	// GemFuseCost is how many copies make a fuse certain; each is that share of the chance.
	GemFuseCost = 5
	// MaxTradeGems is the most Gems one side can put in a trade.
	MaxTradeGems = 1_000_000

	// the in-memory limiter (limGem) keeps the minute; this backs it up a
	// little short of it, so the two never disagree by a refill's edge
	gemCooldownMs  = 55_000
	gemQuickMs     = 65_000
	gemDayMs       = 24 * 60 * 60 * 1000
	gemWatchDays   = 14
	gemFlagScore   = 60
	gemCheckWaitMs = gemDayMs
	gemRecheckMs   = 7 * gemDayMs
	// GemOffFor is how long earning stays off after staff act on a report.
	GemOffFor = 30 * gemDayMs
)

// gemWatchMin is how many earns a day takes before its pattern counts, about
// half an hour of earning every minute. It was 240 until 2026-11-19, which let a
// script run four hours before anything happened. Tests lower it.
var gemWatchMin = 30

// The states of a wallet.
const (
	GemStateFine   = ""
	GemStateCheck  = "check"
	GemStateReport = "report"
	GemStateOff    = "off"
)

// GemRarities are a collectible's rarities, commonest first.
var GemRarities = []string{"common", "rare", "epic", "huge"}

// GemOdds is each rarity's chance from a capsule, in hundredths of a percent.
// When no collectible has some rarity, the others share its chance.
var GemOdds = map[string]int{"common": 7000, "rare": 2500, "epic": 490, "huge": 10}

// gemRarityPower is what a bronze collectible adds to its holder's team, in
// percent; each material above multiplies it.
var gemRarityPower = map[string]int{"common": 4, "rare": 8, "epic": 16, "huge": 40}

// gemMaterialColors is each material's two colours.
var gemMaterialColors = [GemMaterials][2]int{
	{0xd08a55, 0x8a4f2c}, // bronze
	{0xdfe5ec, 0x8e9aab}, // silver
	{0xffd55a, 0xd18f12}, // gold
	{0x55e3a5, 0x13905c}, // emerald
	{0xff7ad9, 0x6c8cff}, // prism
}

var (
	ErrGemsShort     = errors.New("not enough gems")
	ErrGemsNone      = errors.New("no collectibles to draw from")
	ErrGemsReported  = errors.New("a gem farming report is open")
	ErrGemsFuse      = errors.New("that cannot be fused")
	ErrTradeGemsHeld = errors.New("gems are held")
	ErrTradeGemsAlt  = errors.New("gems cannot go between linked accounts")
)

// CollectibleArt is the art staff can pick for a collectible, drawn at each
// material by tools/badge_art.py (COLLECTIBLE_ART, kept equal to this) as
// col-<art>-<material>.svg; the apps list it from those files.
var CollectibleArt = map[string]bool{"moon": true, "bolt": true, "flame": true, "leaf": true, "mushroom": true, "fish": true, "cat": true, "ghost": true, "paw": true, "cherry": true, "dice": true, "key": true}

// IsGemRarity reports whether r is a collectible's rarity.
func IsGemRarity(r string) bool {
	_, ok := GemOdds[r]
	return ok
}

// collectibleBadge is a collectible at a material, as profiles and trades draw it.
func collectibleBadge(d *BadgeDef, material int) Badge {
	b := d.badge()
	b.Level, b.Rarity = material, d.Rarity
	c := gemMaterialColors[min(max(material, 1), GemMaterials)-1]
	c1, c2 := c[0], c[1]
	b.Color, b.Color2, b.Glyph, b.Art = &c1, &c2, "", d.Art
	return b
}

// heldCollectibles is a badge for each collectible someone holds, at the
// highest material they hold, oldest made first.
func (s *Store) heldCollectibles(ctx context.Context, qty map[string]int) ([]Badge, error) {
	top := map[ids.ID]int{}
	var list []ids.ID
	for k, n := range qty {
		v, m, ok := eventKey(k)
		if !ok || m == 0 || n <= 0 {
			continue
		}
		if _, seen := top[v]; !seen {
			list = append(list, v)
		}
		top[v] = max(top[v], m)
	}
	if len(list) == 0 {
		return nil, nil
	}
	defs, err := s.badgeDefsByID(ctx, list)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	var out []Badge
	for _, v := range list {
		if d := defs[v]; d != nil && d.Rarity != "" {
			out = append(out, collectibleBadge(d, top[v]))
		}
	}
	return out, nil
}

// GemWallet is a person's Gems, as only they see it.
type GemWallet struct {
	Gems     int64  `json:"gems"`
	Held     int64  `json:"held"`
	Earned   int64  `json:"earned"`
	LastEarn int64  `json:"last_earn"`
	State    string `json:"state"`
	StateAt  int64  `json:"state_at"`
	OffUntil int64  `json:"off_until"`
	ReportID ids.ID `json:"-"`

	carry     int
	checkedAt int64
	exists    bool
}

const gemWalletCols = `gems, carry, held, earned, last_earn, state, state_at, off_until, checked_at, report_id`

func scanGemWallet(r rowScanner) (*GemWallet, error) {
	w := &GemWallet{}
	var report int64
	err := r.Scan(&w.Gems, &w.carry, &w.Held, &w.Earned, &w.LastEarn, &w.State, &w.StateAt, &w.OffUntil, &w.checkedAt, &report)
	if errors.Is(err, sql.ErrNoRows) {
		return &GemWallet{}, nil
	}
	if err != nil {
		return nil, err
	}
	w.ReportID, w.exists = ids.ID(report), true
	return w, nil
}

func gemWalletTx(ctx context.Context, tx *db.Tx, uid ids.ID) (*GemWallet, error) {
	return scanGemWallet(tx.QueryRowContext(ctx, `SELECT `+gemWalletCols+` FROM gem_wallets WHERE user_id = ?`, int64(uid)))
}

// GemWallet reads someone's wallet; an empty one when they have never earned.
func (s *Store) GemWallet(ctx context.Context, uid ids.ID) (*GemWallet, error) {
	w, err := scanGemWallet(s.db.R.QueryRowContext(ctx, `SELECT `+gemWalletCols+` FROM gem_wallets WHERE user_id = ?`, int64(uid)))
	if err != nil {
		return nil, err
	}
	if w.State == GemStateOff && w.OffUntil <= nowMs() {
		w.State, w.OffUntil = GemStateFine, 0
	}
	return w, nil
}

// GemSignal is what the server saw of a message that may earn: whether
// nobody else had spoken in its conversation for half an hour, and the size of
// its sealed text. Neither is kept as such.
type GemSignal struct {
	Alone bool
	Size  int
}

// GemEarn is what one message earned.
type GemEarn struct {
	Gained int64  // added to the balance
	Held   int64  // added to what is held
	Flag   string // the state this earn put the wallet in, when it changed it
}

// gemDay is one day of gem_watch.
type gemDay struct {
	earns, quick, alone int
	hours, sizes        int64
}

// gemScore is how much a day looks like a script, from 0 to 100: earning at
// nearly every minute it could, where nobody else was talking, through most
// hours of the day, with messages all of one or two sizes. A busy person in a
// busy place earns at most a third of it.
func gemScore(d gemDay) int {
	if d.earns < gemWatchMin {
		return 0
	}
	score := 0
	if d.quick*10 >= d.earns*9 {
		score += 30
	}
	if d.alone*10 >= d.earns*8 {
		score += 25
	}
	if bits.OnesCount64(uint64(d.hours)) >= 16 {
		score += 25
	}
	if bits.OnesCount64(uint64(d.sizes)) <= 2 {
		score += 20
	}
	return score
}

// EarnGem gives a person what a message just sent earns them, at mult percent
// (100 for one Gem), if a minute has passed since the last one. It answers nil
// when nothing was earned.
func (s *Store) EarnGem(ctx context.Context, uid ids.ID, mult int, sig GemSignal, now time.Time) (*GemEarn, error) {
	ms := now.UnixMilli()
	var out *GemEarn
	err := s.Tx(ctx, func(tx *db.Tx) error {
		out = nil
		w, err := gemWalletTx(ctx, tx, uid)
		if err != nil {
			return err
		}
		if w.LastEarn > 0 && ms-w.LastEarn < gemCooldownMs {
			return nil
		}
		if w.State == GemStateOff {
			if w.OffUntil > ms {
				return nil
			}
			w.State, w.OffUntil = GemStateFine, 0
		}
		quick := 0
		if w.LastEarn > 0 && ms-w.LastEarn < gemQuickMs {
			quick = 1
		}
		alone := 0
		if sig.Alone {
			alone = 1
		}
		day := ms / gemDayMs
		hour := int64(1) << ((ms / 3_600_000) % 24)
		size := int64(1) << min(sig.Size/8, 62)
		if _, err := tx.ExecContext(ctx, `INSERT INTO gem_watch (user_id, day, earns, quick, alone, hours, sizes) VALUES (?, ?, 1, ?, ?, ?, ?)
			ON CONFLICT(user_id, day) DO UPDATE SET earns = gem_watch.earns + 1, quick = gem_watch.quick + excluded.quick, alone = gem_watch.alone + excluded.alone, hours = gem_watch.hours | excluded.hours, sizes = gem_watch.sizes | excluded.sizes`,
			int64(uid), day, quick, alone, hour, size); err != nil {
			return err
		}
		var d gemDay
		if err := tx.QueryRowContext(ctx, `SELECT earns, quick, alone, hours, sizes FROM gem_watch WHERE user_id = ? AND day = ?`, int64(uid), day).Scan(&d.earns, &d.quick, &d.alone, &d.hours, &d.sizes); err != nil {
			return err
		}
		out = &GemEarn{}
		flagged := gemScore(d) >= gemFlagScore
		switch {
		case w.State == GemStateFine && flagged && w.checkedAt > 0 && ms-w.checkedAt < gemRecheckMs:
			// shown to be there not long ago, and the same again: staff look
			w.State, w.StateAt, out.Flag = GemStateReport, ms, GemStateReport
		case w.State == GemStateFine && flagged:
			w.State, w.StateAt, out.Flag = GemStateCheck, ms, GemStateCheck
		case w.State == GemStateCheck && flagged && ms-w.StateAt >= gemCheckWaitMs:
			// a day without answering the check, and the pattern goes on
			w.State, w.StateAt, out.Flag = GemStateReport, ms, GemStateReport
		}
		total := w.carry + max(mult, 100)
		add := int64(total / 100)
		carry := total % 100
		gems, held, earned := w.Gems, w.Held, w.Earned
		if w.State == GemStateFine {
			gems += add
			earned += add
			out.Gained = add
		} else {
			held += add
			out.Held = add
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO gem_wallets (user_id, gems, carry, held, earned, last_earn, state, state_at, off_until, checked_at, report_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id) DO UPDATE SET gems = excluded.gems, carry = excluded.carry, held = excluded.held, earned = excluded.earned, last_earn = excluded.last_earn, state = excluded.state, state_at = excluded.state_at, off_until = excluded.off_until
			WHERE gem_wallets.last_earn = ?`,
			int64(uid), gems, carry, held, earned, ms, w.State, w.StateAt, w.OffUntil, w.checkedAt, int64(w.ReportID), w.LastEarn)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			out = nil
		}
		return nil
	})
	return out, err
}

// GemWatchSummary is the farming check's view of someone's last two weeks,
// for staff and for the person's own export: days, not messages.
type GemWatchDay struct {
	Day   int64 `json:"day"` // days since 1970, UTC
	Earns int   `json:"earns"`
	Quick int   `json:"quick"`
	Alone int   `json:"alone"`
	Hours int   `json:"hours"` // how many hours of the day had an earn
	Sizes int   `json:"sizes"` // how many sizes of message earned
	Score int   `json:"score"`
}

// GemWatch is someone's kept days of timing, newest first.
func (s *Store) GemWatch(ctx context.Context, uid ids.ID) ([]GemWatchDay, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT day, earns, quick, alone, hours, sizes FROM gem_watch WHERE user_id = ? ORDER BY day DESC`, int64(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GemWatchDay{}
	for rows.Next() {
		var d gemDay
		var v GemWatchDay
		if err := rows.Scan(&v.Day, &d.earns, &d.quick, &d.alone, &d.hours, &d.sizes); err != nil {
			return nil, err
		}
		v.Earns, v.Quick, v.Alone = d.earns, d.quick, d.alone
		v.Hours, v.Sizes = bits.OnesCount64(uint64(d.hours)), bits.OnesCount64(uint64(d.sizes))
		v.Score = gemScore(d)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SweepGemWatch drops timing older than two weeks.
func (s *Store) SweepGemWatch(ctx context.Context, now time.Time) error {
	_, err := s.db.W.ExecContext(ctx, `DELETE FROM gem_watch WHERE day < ?`, now.UnixMilli()/gemDayMs-gemWatchDays)
	return err
}

// SetGemReport records the report a flag filed, or, with report 0, puts a
// wallet whose report could not be filed back in the check.
func (s *Store) SetGemReport(ctx context.Context, uid, report ids.ID) error {
	if report == 0 {
		_, err := s.db.W.ExecContext(ctx, `UPDATE gem_wallets SET state = ? WHERE user_id = ? AND state = ? AND report_id = 0`, GemStateCheck, int64(uid), GemStateReport)
		return err
	}
	_, err := s.db.W.ExecContext(ctx, `UPDATE gem_wallets SET report_id = ? WHERE user_id = ? AND state = ?`, int64(report), int64(uid), GemStateReport)
	return err
}

// ConfirmGems ends a check someone answered: what was held is theirs, and the
// pattern starts again from nothing. A report open is staff's to close.
func (s *Store) ConfirmGems(ctx context.Context, uid ids.ID) (*GemWallet, error) {
	err := s.Tx(ctx, func(tx *db.Tx) error {
		w, err := gemWalletTx(ctx, tx, uid)
		if err != nil {
			return err
		}
		switch w.State {
		case GemStateReport:
			return ErrGemsReported
		case GemStateCheck:
		default:
			return nil
		}
		at := nowMs()
		if _, err := tx.ExecContext(ctx, `UPDATE gem_wallets SET gems = gems + held, earned = earned + held, held = 0, state = '', state_at = ?, checked_at = ? WHERE user_id = ? AND state = ?`, at, at, int64(uid), GemStateCheck); err != nil {
			return err
		}
		if w.Held > 0 {
			if err := gemLedgerTx(ctx, tx, uid, w.Held, "held_released", "check"); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM gem_watch WHERE user_id = ?`, int64(uid))
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GemWallet(ctx, uid)
}

// GemReportOwner is whose Gems a report holds, or 0.
func (s *Store) GemReportOwner(ctx context.Context, report ids.ID) (ids.ID, error) {
	var uid int64
	err := s.db.R.QueryRowContext(ctx, `SELECT user_id FROM gem_wallets WHERE report_id = ? AND state = ?`, int64(report), GemStateReport).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return ids.ID(uid), err
}

// ActionGemFarming is the case staff acting on a Gem Farming report opens:
// the held Gems taken and earning off for GemOffFor.
const ActionGemFarming = "gem_farming"

// LogGemFarmingLift is staff lifting a Gem Farming case.
const LogGemFarmingLift = "gem_farming_lift"

// CloseGemReport settles a Gem Farming report: dismissed, the held Gems are
// the person's; acted on, they are taken, earning stops for GemOffFor, and c
// and log (for that person) are written in the same transaction. It answers
// who it was about and how many Gems moved, or 0 when the report was not one
// holding anyone's Gems.
func (s *Store) CloseGemReport(ctx context.Context, report ids.ID, actioned bool, c *ModCase, log *ModLogEntry) (ids.ID, int64, error) {
	var who ids.ID
	var moved int64
	err := s.Tx(ctx, func(tx *db.Tx) error {
		who, moved = 0, 0
		var uid, held int64
		err := tx.QueryRowContext(ctx, `SELECT user_id, held FROM gem_wallets WHERE report_id = ? AND state = ?`, int64(report), GemStateReport).Scan(&uid, &held)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		who, moved = ids.ID(uid), held
		at := nowMs()
		if actioned {
			if c == nil || log == nil || c.UserID != who {
				return errors.New("gem report: a case for someone else")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE gem_wallets SET held = 0, state = ?, state_at = ?, off_until = ?, report_id = 0 WHERE user_id = ?`, GemStateOff, at, at+GemOffFor, uid); err != nil {
				return err
			}
			if err := createCaseTx(ctx, tx, c); err != nil {
				return err
			}
			log.CaseID, log.CaseNumber = &c.ID, &c.Number
			if err := appendModLogTx(ctx, tx, log); err != nil {
				return err
			}
			if held > 0 {
				if err := gemLedgerTx(ctx, tx, who, -held, "held_taken", report.String()); err != nil {
					return err
				}
			}
		} else {
			// staff found a person there, so the next flag starts with a check again
			if _, err := tx.ExecContext(ctx, `UPDATE gem_wallets SET gems = gems + held, earned = earned + held, held = 0, state = '', state_at = ?, checked_at = 0, report_id = 0 WHERE user_id = ?`, at, uid); err != nil {
				return err
			}
			if held > 0 {
				if err := gemLedgerTx(ctx, tx, who, held, "held_released", report.String()); err != nil {
					return err
				}
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM gem_watch WHERE user_id = ?`, uid)
		return err
	})
	return who, moved, err
}

// liftGemFarmingTx undoes a Gem Farming case staff lifted: earning starts
// again and the Gems it took come back. A case whose 30 days are over still
// gives them back.
func liftGemFarmingTx(ctx context.Context, tx *db.Tx, c *ModCase, now int64) error {
	var d struct {
		Held int64 `json:"held"`
	}
	_ = json.Unmarshal(c.Details, &d)
	d.Held = max(d.Held, 0)
	res, err := tx.ExecContext(ctx, `UPDATE gem_wallets SET state = CASE WHEN state = 'off' THEN '' ELSE state END, state_at = ?, off_until = 0, gems = gems + ?, earned = earned + ? WHERE user_id = ? AND state <> 'report'`,
		now, d.Held, d.Held, int64(c.UserID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotRestricted
	}
	if d.Held > 0 {
		return gemLedgerTx(ctx, tx, c.UserID, d.Held, "held_returned", c.ID.String())
	}
	return nil
}

func gemLedgerTx(ctx context.Context, tx *db.Tx, uid ids.ID, delta int64, kind, ref string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO gem_ledger (id, user_id, delta, kind, ref, created_at) VALUES (?, ?, ?, ?, ?, ?)`, int64(ids.New()), int64(uid), delta, kind, ref, nowMs())
	return err
}

// takeGems spends n of someone's Gems, inside a transaction, only if they
// have them and nothing of theirs is held.
func takeGems(ctx context.Context, tx *db.Tx, uid ids.ID, n int64, kind, ref string) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE gem_wallets SET gems = gems - ? WHERE user_id = ? AND gems >= ? AND state IN ('', 'off')`, n, int64(uid), n)
	if err != nil {
		return false, err
	}
	if k, _ := res.RowsAffected(); k != 1 {
		return false, nil
	}
	return true, gemLedgerTx(ctx, tx, uid, -n, kind, ref)
}

func giveGems(ctx context.Context, tx *db.Tx, uid ids.ID, n int64, kind, ref string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO gem_wallets (user_id, gems) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET gems = gem_wallets.gems + excluded.gems`, int64(uid), n); err != nil {
		return err
	}
	return gemLedgerTx(ctx, tx, uid, n, kind, ref)
}

// The ledger's words for what the owner gave and took (OwnerGems).
const (
	GemOwnerGiven = "owner_given"
	GemOwnerTaken = "owner_taken"
)

// OwnerGems gives someone Gems (delta above 0) or takes them (below 0) for the
// owner, writing the ledger and the log in the same transaction. Taking more
// than they have to spend is ErrGemsShort; held Gems are never touched, and a
// check, report or farming case stays as it was.
func (s *Store) OwnerGems(ctx context.Context, uid ids.ID, delta int64, log *ModLogEntry) (*GemWallet, error) {
	var out *GemWallet
	err := s.Tx(ctx, func(tx *db.Tx) error {
		if log.ID.IsZero() {
			log.ID = ids.New()
		}
		ref := log.ID.String()
		if delta > 0 {
			if err := giveGems(ctx, tx, uid, delta, GemOwnerGiven, ref); err != nil {
				return err
			}
		} else {
			res, err := tx.ExecContext(ctx, `UPDATE gem_wallets SET gems = gems - ? WHERE user_id = ? AND gems >= ?`, -delta, int64(uid), -delta)
			if err != nil {
				return err
			}
			if k, _ := res.RowsAffected(); k != 1 {
				return ErrGemsShort
			}
			if err := gemLedgerTx(ctx, tx, uid, delta, GemOwnerTaken, ref); err != nil {
				return err
			}
		}
		w, err := gemWalletTx(ctx, tx, uid)
		if err != nil {
			return err
		}
		out = w
		log.Details, _ = json.Marshal(map[string]int64{"amount": delta, "gems": w.Gems})
		return appendModLogTx(ctx, tx, log)
	})
	return out, err
}

// GemLedgerEntry is one spend, trade or decision about someone's Gems.
type GemLedgerEntry struct {
	Delta     int64  `json:"delta"`
	Kind      string `json:"kind"`
	Ref       string `json:"ref"`
	CreatedAt int64  `json:"created_at"`
}

// GemLedger is someone's ledger, newest first.
func (s *Store) GemLedger(ctx context.Context, uid ids.ID, limit int) ([]GemLedgerEntry, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT delta, kind, ref, created_at FROM gem_ledger WHERE user_id = ? ORDER BY id DESC LIMIT ?`, int64(uid), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GemLedgerEntry{}
	for rows.Next() {
		var e GemLedgerEntry
		if err := rows.Scan(&e.Delta, &e.Kind, &e.Ref, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// gemsIn is how many Gems some lines offer.
func gemsIn(lines []TradeLine) int64 {
	var n int64
	for _, l := range lines {
		if l.Key == GemsKey {
			n += int64(l.Qty)
		}
	}
	return n
}

// canTradeGems is whether by may put n Gems in t: only Gems they hold, never
// while a check or report holds theirs, and never to an account the alt check
// links to theirs, since that is how farmed Gems would be gathered.
func (s *Store) canTradeGems(ctx context.Context, t *Trade, by ids.ID, n int64) error {
	w, err := s.GemWallet(ctx, by)
	if err != nil {
		return err
	}
	if w.State == GemStateCheck || w.State == GemStateReport {
		return ErrTradeGemsHeld
	}
	if w.Gems < n {
		return &ErrTradeShort{Who: by}
	}
	other := t.FromID
	if by == t.FromID {
		other = t.ToID
	}
	alts, err := s.AltAccounts(ctx, by)
	if err != nil {
		return err
	}
	for _, a := range alts {
		if a.UserID == other {
			return ErrTradeGemsAlt
		}
	}
	return nil
}

// Collectible is a staff-made badge a capsule can give.
type Collectible struct {
	ID     ids.ID `json:"id"`
	Name   string `json:"name"`
	Rarity string `json:"rarity"`
	Badge  Badge  `json:"badge"` // drawn at bronze
}

// Collectibles is every collectible, oldest first.
func (s *Store) Collectibles(ctx context.Context) ([]Collectible, error) {
	defs, err := s.BadgeDefs(ctx)
	if err != nil {
		return nil, err
	}
	out := []Collectible{}
	for i := len(defs) - 1; i >= 0; i-- {
		if d := defs[i]; d.Rarity != "" {
			out = append(out, Collectible{ID: d.ID, Name: d.Name, Rarity: d.Rarity, Badge: collectibleBadge(d, 1)})
		}
	}
	return out, nil
}

// CapsuleOdds is each rarity's chance now, in hundredths of a percent: the
// rarities no collectible has share theirs among the rest.
func CapsuleOdds(list []Collectible) map[string]int {
	have := map[string]bool{}
	for _, c := range list {
		have[c.Rarity] = true
	}
	total := 0
	for r, p := range GemOdds {
		if have[r] {
			total += p
		}
	}
	out := map[string]int{}
	if total == 0 {
		return out
	}
	left, last := 10000, ""
	for _, r := range GemRarities {
		if have[r] {
			out[r] = GemOdds[r] * 10000 / total
			left -= out[r]
			last = r
		}
	}
	out[last] += left // what rounding left goes to the rarest, never the commonest
	return out
}

func randBelow(n int) (int, error) {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

// drawCollectible picks a rarity at the odds, then one of its collectibles.
func drawCollectible(list []Collectible) (*Collectible, error) {
	odds := CapsuleOdds(list)
	if len(odds) == 0 {
		return nil, ErrGemsNone
	}
	roll, err := randBelow(10000)
	if err != nil {
		return nil, err
	}
	rarity := ""
	for _, r := range GemRarities {
		if p, ok := odds[r]; ok {
			rarity = r
			if roll < p {
				break
			}
			roll -= p
		}
	}
	var of []Collectible
	for _, c := range list {
		if c.Rarity == rarity {
			of = append(of, c)
		}
	}
	i, err := randBelow(len(of))
	if err != nil {
		return nil, err
	}
	return &of[i], nil
}

// OpenCapsule spends GemCapsulePrice of someone's Gems on a collectible at
// bronze, drawn here, never by the app. It answers what came out and its key.
func (s *Store) OpenCapsule(ctx context.Context, uid ids.ID) (*Collectible, string, error) {
	list, err := s.Collectibles(ctx)
	if err != nil {
		return nil, "", err
	}
	got, err := drawCollectible(list)
	if err != nil {
		return nil, "", err
	}
	key := TradeKey(EventBadgePrefix+got.ID.String(), 1)
	err = s.Tx(ctx, func(tx *db.Tx) error {
		ok, err := takeGems(ctx, tx, uid, GemCapsulePrice, "capsule", key)
		if err != nil {
			return err
		}
		if !ok {
			return ErrGemsShort
		}
		return give(ctx, tx, uid, key, 1)
	})
	if err != nil {
		return nil, "", err
	}
	s.exist.forget()
	return got, key, nil
}

// fuseRoll draws a number below n for a fuse; tests replace it.
var fuseRoll = randBelow

// Fuse puts 1 to GemFuseCost copies of a collectible at one material in for
// one of the next. Each copy is 1 in GemFuseCost of the chance, so a full set
// always works; one that fails keeps the copies anyway. The server rolls,
// never the app. It answers the key that came out and whether it worked.
func (s *Store) Fuse(ctx context.Context, uid ids.ID, key string, copies int) (string, bool, error) {
	v, m, ok := eventKey(key)
	if !ok || m < 1 || m >= GemMaterials || copies < 1 || copies > GemFuseCost {
		return "", false, ErrGemsFuse
	}
	defs, err := s.badgeDefsByID(ctx, []ids.ID{v})
	if err != nil {
		return "", false, err
	}
	if d := defs[v]; d == nil || d.Rarity == "" {
		return "", false, ErrGemsFuse
	}
	roll, err := fuseRoll(GemFuseCost)
	if err != nil {
		return "", false, err
	}
	won := roll < copies
	next := TradeKey(EventBadgePrefix+v.String(), m+1)
	err = s.Tx(ctx, func(tx *db.Tx) error {
		// collectibles are never earned, so only copies count
		ok, err := take(ctx, tx, uid, false, key, copies)
		if err != nil {
			return err
		}
		if !ok {
			return ErrGemsShort
		}
		if !won {
			return nil
		}
		return give(ctx, tx, uid, next, 1)
	})
	if err != nil {
		return "", false, err
	}
	s.exist.forget()
	if !won {
		return key, false, nil
	}
	return next, true, nil
}

// GemTeamMember is one badge of a person's team and what it adds, in percent.
type GemTeamMember struct {
	Badge Badge `json:"badge"`
	Power int   `json:"power"`
}

// GemPlanBoost is what a plan multiplies everything a person earns by, in
// percent, on top of their team (the owner's choice, 2026-11-18): Supporter
// earns half as much again and Pro twice as much. The plan's badge still adds
// nothing to a team, so paying counts once.
func GemPlanBoost(plan string) int {
	switch plan {
	case PlanPro:
		return 200
	case PlanSupporter:
		return 150
	}
	return 100
}

// GemTeamSize is how many badges make a team: the ones a profile shows first.
const GemTeamSize = 5

// gemPower is what one badge adds to a team. A staff rank, a plan, Booster and
// Gift Giver add nothing: a plan boosts earning through GemPlanBoost instead,
// and the rest come of paying.
func gemPower(b Badge) int {
	switch {
	case b.ID == BadgeStaff || b.ID == BadgePlan || b.ID == BadgeBooster || b.ID == BadgeGiftGiver:
		// boosts come with Pro and gifts are bought
		return 0
	case b.Rarity != "":
		return gemRarityPower[b.Rarity] * max(b.Level, 1)
	case b.Level > 0:
		return 2 * min(b.Level, 5)
	}
	return 2
}

// GemTeam is the first GemTeamSize badges on someone's profile, each with its
// power, and the multiplier they make together, in percent.
func (s *Store) GemTeam(ctx context.Context, u *User) ([]GemTeamMember, int, error) {
	list, err := s.ProfileBadges(ctx, u)
	if err != nil {
		return nil, 100, err
	}
	team := []GemTeamMember{}
	mult := 100
	for _, b := range list[:min(len(list), GemTeamSize)] {
		p := gemPower(b)
		team = append(team, GemTeamMember{Badge: b, Power: p})
		mult += p
	}
	return team, mult, nil
}

// SomeoneElseSpoke is whether anyone but uid sent a message in a channel
// since a time. It reads ids only.
func (s *Store) SomeoneElseSpoke(ctx context.Context, channel, uid ids.ID, since time.Time) (bool, error) {
	var one int
	err := s.db.R.QueryRowContext(ctx, `SELECT 1 FROM messages WHERE channel_id = ? AND id > ? AND author_id <> ? LIMIT 1`, int64(channel), int64(ids.FromTime(since)), int64(uid)).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
