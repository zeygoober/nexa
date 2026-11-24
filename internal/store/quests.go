package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// Quests (migration 0068). Every UTC day a person has three daily quests, one
// easy, one medium and one hard, and a bonus for finishing all three; and the
// Quest Tower (the trial track), one quest at a time, each step asking more
// than the last.
// A quest pays its Gems the moment it is finished, into the same wallet the
// farming check watches: held while a check or report is open, nothing while
// earning is off. Quest Gems never count towards a Gems quest.
//
// Progress counts only what happened while the quest was open (from
// started_at), never anything from before, and the server alone counts it.

// The kinds of quest. A quest about different ones (servers, people, reports)
// counts each once; a call's size counts its largest.
const (
	QuestMessages    = "messages"     // messages sent where someone else spoke in the last half hour
	QuestServers     = "servers"      // different servers messaged in
	QuestCallMinutes = "call_minutes" // minutes in a call with someone else
	QuestCallSize    = "call_size"    // people in the largest call (accounts, not guests)
	QuestFriends     = "friends"      // people befriended for the first time
	QuestJoins       = "joins"        // servers joined for the first time
	QuestCapsules    = "capsules"     // capsules opened
	QuestFuses       = "fuses"        // fuses tried
	QuestMaterials   = "materials"    // fuses that reached the next material
	QuestTrades      = "trades"       // trades done, with different people
	QuestGems        = "gems"         // Gems earned by chatting
	QuestBugs        = "bugs"         // bug reports staff resolved
	QuestIdeas       = "ideas"        // suggestions staff resolved
	// QuestDailies is the bonus: the day's three finished.
	QuestDailies = "dailies"
)

// The tracks a quest is on.
const (
	QuestDaily = "daily"
	QuestBonus = "bonus"
	QuestTrial = "trial"
)

// questDistinct are the kinds whose events name what they are about, each counted once per quest.
var questDistinct = map[string]bool{QuestServers: true, QuestFriends: true, QuestJoins: true, QuestTrades: true, QuestBugs: true, QuestIdeas: true}

type questPick struct {
	kind   string
	target int64
}

// questDailyRewards are what an easy, a medium and a hard daily quest pay;
// QuestBonusReward is what finishing all three adds. A full day is 80 Gems,
// more than a capsule (GemCapsulePrice), less than an hour of chatting.
var questDailyRewards = [3]int64{10, 20, 35}

const QuestBonusReward = 15

// questDailyPool is what each daily slot draws from. Bug reports and
// suggestions are staff's to resolve, too slow for a day, so only the Trial
// asks for them. A quest that spends Gems (capsules, fuses, materials) always
// pays less than it costs.
var questDailyPool = [3][]questPick{
	{{QuestMessages, 10}, {QuestServers, 2}, {QuestCallMinutes, 5}, {QuestCapsules, 1}, {QuestFuses, 1}, {QuestGems, 20}},
	{{QuestMessages, 25}, {QuestServers, 3}, {QuestCallMinutes, 15}, {QuestCallSize, 3}, {QuestFriends, 1}, {QuestJoins, 1}, {QuestFuses, 3}, {QuestTrades, 1}, {QuestGems, 50}},
	{{QuestMessages, 50}, {QuestServers, 5}, {QuestCallMinutes, 30}, {QuestCallSize, 4}, {QuestFriends, 2}, {QuestJoins, 2}, {QuestCapsules, 3}, {QuestMaterials, 1}, {QuestTrades, 2}, {QuestGems, 120}},
}

// questRung is a kind on the Quest Tower: its target at step 1, the most it
// can ask (0 for kinds with no natural end), how often the tower draws it and
// the first step it can appear.
type questRung struct {
	base, most int64
	weight     int
	from       int64
}

// The Quest Tower grows exponentially: every target is questTowerGrowth times
// the last step's, and the reward questRewardGrowth times, which is slower, so
// each step pays more Gems than the last but fewer per hour of effort. A kind
// with a natural end (there are only so many new friends to make) leaves the
// draw once its target passes most, so deep steps are messages, call minutes
// and Gems and the tower never flattens. The owner's request, 2026-11-24: the
// first curve grew by a fixed amount and stopped rising at step 43.
const (
	questTowerGrowth  = 1.07
	questRewardGrowth = 1.04
	questTargetMax    = 1_000_000_000 // past any step a person can reach; keeps the numbers in range
	questRewardMax    = 1_000_000
)

// questLadder is the Quest Tower (the trial track): a kind is drawn by weight
// (never the last step's), its target grows with the step, and staff-gated
// kinds start at step 5.
var questLadder = map[string]questRung{
	QuestMessages:    {10, 0, 3, 1},
	QuestServers:     {2, 10, 2, 1},
	QuestCallMinutes: {5, 0, 2, 1},
	QuestCallSize:    {2, 6, 1, 1},
	QuestFriends:     {1, 5, 1, 1},
	QuestJoins:       {1, 5, 1, 1},
	QuestCapsules:    {1, 10, 1, 1},
	QuestFuses:       {1, 15, 1, 1},
	QuestMaterials:   {1, 3, 1, 1},
	QuestTrades:      {1, 5, 1, 1},
	QuestGems:        {20, 0, 3, 1},
	QuestBugs:        {1, 3, 1, 5},
	QuestIdeas:       {1, 3, 1, 5},
}

// questKinds is every kind the Trial can draw, in a fixed order, so a draw
// depends only on the random number.
var questKinds = []string{QuestMessages, QuestServers, QuestCallMinutes, QuestCallSize, QuestFriends, QuestJoins, QuestCapsules, QuestFuses, QuestMaterials, QuestTrades, QuestGems, QuestBugs, QuestIdeas}

// QuestTrialTarget is what step asks of kind, or 0 once kind has passed its
// end and is no longer drawn.
func QuestTrialTarget(kind string, step int64) int64 {
	r := questLadder[kind]
	v := float64(r.base) * math.Pow(questTowerGrowth, float64(max(step, 1)-1))
	n := int64(questTargetMax)
	if v < questTargetMax {
		n = max(r.base, int64(math.Round(v)))
	}
	if r.most > 0 && n > r.most {
		return 0
	}
	return n
}

// QuestTrialReward is what a Quest Tower step pays: 15 at the first, growing
// by questRewardGrowth each step, in fives from 20.
func QuestTrialReward(step int64) int64 {
	v := 15 * math.Pow(questRewardGrowth, float64(max(step, 1)-1))
	switch {
	case v >= questRewardMax:
		return questRewardMax
	case v < 20:
		return int64(math.Round(v))
	}
	return 5 * int64(math.Round(v/5))
}

var (
	ErrQuestNone = errors.New("no such quest")
	ErrQuestUsed = errors.New("already used today")
)

// Quest is one quest as its holder sees it.
type Quest struct {
	ID       ids.ID `json:"id"`
	Track    string `json:"track"`
	Level    int64  `json:"level"` // daily: 1 easy, 2 medium, 3 hard; trial: the step
	Kind     string `json:"kind"`
	Target   int64  `json:"target"`
	Progress int64  `json:"progress"`
	Reward   int64  `json:"reward"`
	Started  int64  `json:"started_at"`
	DoneAt   int64  `json:"done_at"`
	Paid     int64  `json:"paid"`
}

const questCols = `id, track, period, slot, kind, target, progress, reward, started_at, done_at, paid`

func scanQuest(r rowScanner) (*Quest, error) {
	q := &Quest{}
	var id, period, slot int64
	if err := r.Scan(&id, &q.Track, &period, &slot, &q.Kind, &q.Target, &q.Progress, &q.Reward, &q.Started, &q.DoneAt, &q.Paid); err != nil {
		return nil, err
	}
	q.ID = ids.ID(id)
	q.Level = slot
	if q.Track == QuestTrial {
		q.Level = period
	}
	return q, nil
}

// QuestBoard is a person's quests now.
type QuestBoard struct {
	Day        int64   `json:"day"`
	ResetsAt   int64   `json:"resets_at"`
	Daily      []Quest `json:"daily"`
	Bonus      *Quest  `json:"bonus"`
	Trial      *Quest  `json:"trial"`
	Cleared    int64   `json:"trial_cleared"` // Trial steps finished
	RerollLeft bool    `json:"reroll_left"`
	SkipLeft   bool    `json:"skip_left"`
}

// QuestStep is one thing that happened which a quest may count: n more of
// kind, about subject when the kind counts different ones, or, for a call's
// size, n people now.
type QuestStep struct {
	Kind    string
	N       int64
	Subject ids.ID
}

func questDay(ms int64) int64 { return ms / gemDayMs }

// weighted picks an index by weight; weights must not all be 0.
func weighted(weights []int) (int, error) {
	total := 0
	for _, w := range weights {
		total += w
	}
	n, err := randBelow(total)
	if err != nil {
		return 0, err
	}
	for i, w := range weights {
		if n < w {
			return i, nil
		}
		n -= w
	}
	return len(weights) - 1, nil
}

// drawDaily picks a quest for a daily slot (0 easy to 2 hard), none of whose
// kind is in not.
func drawDaily(slot int, not map[string]bool) (questPick, error) {
	pool := questDailyPool[slot]
	weights := make([]int, len(pool))
	for i, p := range pool {
		if !not[p.kind] {
			weights[i] = 1
		}
	}
	i, err := weighted(weights)
	if err != nil {
		return questPick{}, err
	}
	return pool[i], nil
}

// drawTrial picks the Trial's kind at step, never not.
func drawTrial(step int64, not string) (string, error) {
	weights := make([]int, len(questKinds))
	for i, k := range questKinds {
		if r := questLadder[k]; k != not && step >= r.from && QuestTrialTarget(k, step) > 0 {
			weights[i] = r.weight
		}
	}
	i, err := weighted(weights)
	if err != nil {
		return "", err
	}
	return questKinds[i], nil
}

func insertQuestTx(ctx context.Context, tx *db.Tx, uid ids.ID, track string, period, slot int64, kind string, target, reward, ms int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO quests (id, user_id, track, period, slot, kind, target, reward, started_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		int64(ids.New()), int64(uid), track, period, slot, kind, target, reward, ms)
	return err
}

// ensureQuestsTx gives someone today's quests and their Trial step's when they
// have none yet, and answers the step.
func ensureQuestsTx(ctx context.Context, tx *db.Tx, uid ids.ID, ms int64) (int64, error) {
	if _, err := tx.ExecContext(ctx, `INSERT INTO quest_players (user_id) VALUES (?) ON CONFLICT DO NOTHING`, int64(uid)); err != nil {
		return 0, err
	}
	var step int64
	if err := tx.QueryRowContext(ctx, `SELECT trial_step FROM quest_players WHERE user_id = ?`, int64(uid)).Scan(&step); err != nil {
		return 0, err
	}
	day := questDay(ms)
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM quests WHERE user_id = ? AND track = ? AND period = ?`, int64(uid), QuestDaily, day).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		taken := map[string]bool{}
		for slot := range 3 {
			p, err := drawDaily(slot, taken)
			if err != nil {
				return 0, err
			}
			taken[p.kind] = true
			if err := insertQuestTx(ctx, tx, uid, QuestDaily, day, int64(slot+1), p.kind, p.target, questDailyRewards[slot], ms); err != nil {
				return 0, err
			}
		}
		if err := insertQuestTx(ctx, tx, uid, QuestBonus, day, 0, QuestDailies, 3, QuestBonusReward, ms); err != nil {
			return 0, err
		}
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM quests WHERE user_id = ? AND track = ? AND period = ?`, int64(uid), QuestTrial, step).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		if err := newTrialStepTx(ctx, tx, uid, step, "", ms); err != nil {
			return 0, err
		}
	}
	return step, nil
}

func newTrialStepTx(ctx context.Context, tx *db.Tx, uid ids.ID, step int64, not string, ms int64) error {
	kind, err := drawTrial(step, not)
	if err != nil {
		return err
	}
	return insertQuestTx(ctx, tx, uid, QuestTrial, step, 0, kind, QuestTrialTarget(kind, step), QuestTrialReward(step), ms)
}

func openQuestsTx(ctx context.Context, tx *db.Tx, uid ids.ID, step, day int64, kind string) ([]*Quest, error) {
	q := `SELECT ` + questCols + ` FROM quests WHERE user_id = ? AND done_at = 0 AND ((track = ? AND period = ?) OR (track = ? AND period = ?))`
	args := []any{int64(uid), QuestDaily, day, QuestTrial, step}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, kind)
	}
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Quest
	for rows.Next() {
		x, err := scanQuest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// OpenQuestKinds gives someone their quests for today if they have none yet
// and answers the kinds still open, so events nobody's quest wants cost
// nothing more.
func (s *Store) OpenQuestKinds(ctx context.Context, uid ids.ID, now time.Time) (map[string]bool, error) {
	ms := now.UnixMilli()
	kinds := map[string]bool{}
	err := s.Tx(ctx, func(tx *db.Tx) error {
		clear(kinds)
		step, err := ensureQuestsTx(ctx, tx, uid, ms)
		if err != nil {
			return err
		}
		open, err := openQuestsTx(ctx, tx, uid, step, questDay(ms), "")
		if err != nil {
			return err
		}
		for _, q := range open {
			kinds[q.Kind] = true
		}
		return nil
	})
	return kinds, err
}

// payQuestTx marks q finished and pays its reward, once: a quest already
// finished pays nothing and answers false. While a check or report is open
// the reward is held with what chatting earns; while earning is off it pays
// nothing.
func payQuestTx(ctx context.Context, tx *db.Tx, uid ids.ID, q *Quest, ms int64) (bool, error) {
	w, err := gemWalletTx(ctx, tx, uid)
	if err != nil {
		return false, err
	}
	paid := q.Reward
	if w.State == GemStateOff && w.OffUntil > ms {
		paid = 0
	}
	res, err := tx.ExecContext(ctx, `UPDATE quests SET progress = target, done_at = ?, paid = ? WHERE id = ? AND done_at = 0`, ms, paid, int64(q.ID))
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM quest_seen WHERE user_id = ? AND scope = ?`, int64(uid), questScope(q.ID)); err != nil {
		return false, err
	}
	q.Progress, q.DoneAt, q.Paid = q.Target, ms, paid
	switch {
	case paid == 0:
	case w.State == GemStateCheck || w.State == GemStateReport:
		if _, err := tx.ExecContext(ctx, `UPDATE gem_wallets SET held = held + ? WHERE user_id = ?`, paid, int64(uid)); err != nil {
			return false, err
		}
	default:
		if err := giveGems(ctx, tx, uid, paid, "quest", q.ID.String()); err != nil {
			return false, err
		}
	}
	return true, nil
}

func questScope(id ids.ID) string { return "q:" + id.String() }

// finishQuestTx pays q and what finishing it brings: a daily quest moves the
// bonus on, a Trial step opens the next. It answers every quest it finished.
func finishQuestTx(ctx context.Context, tx *db.Tx, uid ids.ID, q *Quest, ms int64) ([]Quest, error) {
	ok, err := payQuestTx(ctx, tx, uid, q, ms)
	if err != nil || !ok {
		return nil, err
	}
	done := []Quest{*q}
	switch q.Track {
	case QuestDaily:
		day := questDay(q.Started)
		if _, err := tx.ExecContext(ctx, `UPDATE quests SET progress = progress + 1 WHERE user_id = ? AND track = ? AND period = ? AND done_at = 0`, int64(uid), QuestBonus, day); err != nil {
			return nil, err
		}
		b, err := scanQuest(tx.QueryRowContext(ctx, `SELECT `+questCols+` FROM quests WHERE user_id = ? AND track = ? AND period = ?`, int64(uid), QuestBonus, day))
		if errors.Is(err, sql.ErrNoRows) {
			return done, nil
		}
		if err != nil {
			return nil, err
		}
		if b.DoneAt == 0 && b.Progress >= b.Target {
			more, err := finishQuestTx(ctx, tx, uid, b, ms)
			if err != nil {
				return nil, err
			}
			done = append(done, more...)
		}
	case QuestTrial:
		res, err := tx.ExecContext(ctx, `UPDATE quest_players SET trial_step = ? WHERE user_id = ? AND trial_step = ?`, q.Level+1, int64(uid), q.Level)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			if err := newTrialStepTx(ctx, tx, uid, q.Level+1, q.Kind, ms); err != nil {
				return nil, err
			}
		}
	}
	return done, nil
}

// AddQuestProgress counts what happened towards someone's open quests and
// pays any it finishes, answering those.
func (s *Store) AddQuestProgress(ctx context.Context, uid ids.ID, steps []QuestStep, now time.Time) ([]Quest, error) {
	ms := now.UnixMilli()
	var done []Quest
	err := s.Tx(ctx, func(tx *db.Tx) error {
		done = nil
		step, err := ensureQuestsTx(ctx, tx, uid, ms)
		if err != nil {
			return err
		}
		for _, st := range steps {
			if st.N <= 0 {
				continue
			}
			// read again for each, since finishing one opens the next Trial step
			open, err := openQuestsTx(ctx, tx, uid, step, questDay(ms), st.Kind)
			if err != nil {
				return err
			}
			for _, q := range open {
				if questDistinct[st.Kind] {
					if st.Subject == 0 {
						continue
					}
					res, err := tx.ExecContext(ctx, `INSERT INTO quest_seen (user_id, scope, subject) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, int64(uid), questScope(q.ID), int64(st.Subject))
					if err != nil {
						return err
					}
					if n, _ := res.RowsAffected(); n != 1 {
						continue
					}
				}
				next := q.Progress + st.N
				if st.Kind == QuestCallSize {
					next = max(q.Progress, st.N)
				}
				next = min(next, q.Target)
				if next == q.Progress {
					continue
				}
				if next < q.Target {
					if _, err := tx.ExecContext(ctx, `UPDATE quests SET progress = ? WHERE id = ? AND done_at = 0`, next, int64(q.ID)); err != nil {
						return err
					}
					continue
				}
				more, err := finishQuestTx(ctx, tx, uid, q, ms)
				if err != nil {
					return err
				}
				done = append(done, more...)
			}
			if err := tx.QueryRowContext(ctx, `SELECT trial_step FROM quest_players WHERE user_id = ?`, int64(uid)).Scan(&step); err != nil {
				return err
			}
		}
		return nil
	})
	return done, err
}

// QuestFirstTime records that someone befriended a person or joined a server
// (what: "friend" or "join") and answers whether it is the first time.
func (s *Store) QuestFirstTime(ctx context.Context, uid ids.ID, what string, subject ids.ID) (bool, error) {
	res, err := s.db.W.ExecContext(ctx, `INSERT INTO quest_seen (user_id, scope, subject) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, int64(uid), "ever:"+what, int64(subject))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// QuestBoard gives someone their quests for today if they have none yet and
// answers all of them.
func (s *Store) QuestBoard(ctx context.Context, uid ids.ID, now time.Time) (*QuestBoard, error) {
	ms := now.UnixMilli()
	day := questDay(ms)
	b := &QuestBoard{}
	err := s.Tx(ctx, func(tx *db.Tx) error {
		*b = QuestBoard{Day: day, ResetsAt: (day + 1) * gemDayMs, Daily: []Quest{}}
		step, err := ensureQuestsTx(ctx, tx, uid, ms)
		if err != nil {
			return err
		}
		var reroll, skip int64
		if err := tx.QueryRowContext(ctx, `SELECT reroll_day, skip_day FROM quest_players WHERE user_id = ?`, int64(uid)).Scan(&reroll, &skip); err != nil {
			return err
		}
		b.Cleared, b.RerollLeft, b.SkipLeft = step-1, reroll != day, skip != day
		rows, err := tx.QueryContext(ctx, `SELECT `+questCols+` FROM quests WHERE user_id = ? AND ((track IN (?, ?) AND period = ?) OR (track = ? AND period = ?)) ORDER BY slot`,
			int64(uid), QuestDaily, QuestBonus, day, QuestTrial, step)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			q, err := scanQuest(rows)
			if err != nil {
				return err
			}
			switch q.Track {
			case QuestDaily:
				b.Daily = append(b.Daily, *q)
			case QuestBonus:
				b.Bonus = q
			case QuestTrial:
				b.Trial = q
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

// restartQuestTx gives an open quest a new kind, target and reward and starts
// its count again, as a swap or a skip does.
func restartQuestTx(ctx context.Context, tx *db.Tx, uid ids.ID, q *Quest, kind string, target, ms int64) error {
	res, err := tx.ExecContext(ctx, `UPDATE quests SET kind = ?, target = ?, progress = 0, started_at = ? WHERE id = ? AND done_at = 0`, kind, target, ms, int64(q.ID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrQuestNone
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM quest_seen WHERE user_id = ? AND scope = ?`, int64(uid), questScope(q.ID))
	return err
}

// RerollQuest swaps one of today's unfinished daily quests for another of the
// same difficulty, once a day.
func (s *Store) RerollQuest(ctx context.Context, uid, id ids.ID, now time.Time) error {
	ms := now.UnixMilli()
	day := questDay(ms)
	return s.Tx(ctx, func(tx *db.Tx) error {
		if _, err := ensureQuestsTx(ctx, tx, uid, ms); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE quest_players SET reroll_day = ? WHERE user_id = ? AND reroll_day != ?`, day, int64(uid), day)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrQuestUsed
		}
		q, err := scanQuest(tx.QueryRowContext(ctx, `SELECT `+questCols+` FROM quests WHERE id = ? AND user_id = ? AND track = ? AND period = ? AND done_at = 0`, int64(id), int64(uid), QuestDaily, day))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrQuestNone
		}
		if err != nil {
			return err
		}
		taken := map[string]bool{}
		rows, err := tx.QueryContext(ctx, `SELECT kind FROM quests WHERE user_id = ? AND track = ? AND period = ?`, int64(uid), QuestDaily, day)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return err
			}
			taken[k] = true
		}
		rows.Close()
		p, err := drawDaily(int(q.Level)-1, taken)
		if err != nil {
			return err
		}
		return restartQuestTx(ctx, tx, uid, q, p.kind, p.target, ms)
	})
}

// SkipTrialQuest swaps the Trial's open step for another kind at the same
// step, once a day, so a quest nobody could do never stops the climb.
func (s *Store) SkipTrialQuest(ctx context.Context, uid ids.ID, now time.Time) error {
	ms := now.UnixMilli()
	day := questDay(ms)
	return s.Tx(ctx, func(tx *db.Tx) error {
		step, err := ensureQuestsTx(ctx, tx, uid, ms)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE quest_players SET skip_day = ? WHERE user_id = ? AND skip_day != ?`, day, int64(uid), day)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrQuestUsed
		}
		q, err := scanQuest(tx.QueryRowContext(ctx, `SELECT `+questCols+` FROM quests WHERE user_id = ? AND track = ? AND period = ? AND done_at = 0`, int64(uid), QuestTrial, step))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrQuestNone
		}
		if err != nil {
			return err
		}
		kind, err := drawTrial(step, q.Kind)
		if err != nil {
			return err
		}
		return restartQuestTx(ctx, tx, uid, q, kind, QuestTrialTarget(kind, step), ms)
	})
}

// QuestRecord is every quest kept for someone, newest first, for their data
// export and the legal report.
func (s *Store) QuestRecord(ctx context.Context, uid ids.ID) ([]Quest, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT `+questCols+` FROM quests WHERE user_id = ? ORDER BY started_at DESC, slot`, int64(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Quest{}
	for rows.Next() {
		q, err := scanQuest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *q)
	}
	return out, rows.Err()
}

// questKeepDays is how long a day's quests and a finished Trial step are kept.
const questKeepDays = 14

// SweepQuests drops days of quests and finished Trial steps two weeks old,
// and what they had counted.
func (s *Store) SweepQuests(ctx context.Context, now time.Time) error {
	ms := now.UnixMilli()
	if _, err := s.db.W.ExecContext(ctx, `DELETE FROM quests WHERE (track IN (?, ?) AND period < ?) OR (track = ? AND done_at > 0 AND done_at < ?)`,
		QuestDaily, QuestBonus, questDay(ms)-questKeepDays, QuestTrial, ms-questKeepDays*gemDayMs); err != nil {
		return err
	}
	_, err := s.db.W.ExecContext(ctx, `DELETE FROM quest_seen WHERE scope LIKE 'q:%' AND NOT EXISTS (SELECT 1 FROM quests WHERE quests.done_at = 0 AND quest_seen.scope = 'q:' || CAST(quests.id AS TEXT))`)
	return err
}
