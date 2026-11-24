package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// The owner's quest tools (the owner's request, 2026-11-24): the owner works
// their own quests by hand to see them behave. Each runs the real paying and
// climbing code, so a quest finished here pays and opens the next tower step
// as one finished by playing does, and each is written to mod_log in the same
// transaction as quest_debug with what it did.

const (
	LogQuestDebug = "quest_debug"

	QuestDebugProgress = "progress" // add to one open quest, finishing it at its target
	QuestDebugSet      = "set"      // give one open quest another kind and target
	QuestDebugStep     = "step"     // put the tower at a step
	QuestDebugNewDay   = "new_day"  // draw today's three quests and the bonus again
	QuestDebugUses     = "uses"     // give back today's swap and skip

	// QuestDebugStepMax is the highest step the owner can jump to.
	QuestDebugStepMax = 100_000
)

var ErrQuestDebug = errors.New("not a quest debug request")

// QuestDebug is one owner action on their own quests.
type QuestDebug struct {
	Action  string `json:"action"`
	QuestID ids.ID `json:"quest_id,omitempty"`
	Amount  int64  `json:"amount,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Target  int64  `json:"target,omitempty"`
	Step    int64  `json:"step,omitempty"`
}

// OwnerQuestDebug does d to uid's quests and logs it, answering any quests it
// finished.
func (s *Store) OwnerQuestDebug(ctx context.Context, uid ids.ID, d QuestDebug, log *ModLogEntry, now time.Time) ([]Quest, error) {
	ms := now.UnixMilli()
	day := questDay(ms)
	var done []Quest
	err := s.Tx(ctx, func(tx *db.Tx) error {
		done = nil
		step, err := ensureQuestsTx(ctx, tx, uid, ms)
		if err != nil {
			return err
		}
		open := func() (*Quest, error) {
			q, err := scanQuest(tx.QueryRowContext(ctx, `SELECT `+questCols+` FROM quests WHERE id = ? AND user_id = ? AND done_at = 0
				AND ((track = ? AND period = ?) OR (track = ? AND period = ?))`, int64(d.QuestID), int64(uid), QuestDaily, day, QuestTrial, step))
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrQuestNone
			}
			return q, err
		}
		switch d.Action {
		case QuestDebugProgress:
			if d.Amount < 1 || d.Amount > questTargetMax {
				return ErrQuestDebug
			}
			q, err := open()
			if err != nil {
				return err
			}
			if next := q.Progress + d.Amount; next < q.Target {
				if _, err := tx.ExecContext(ctx, `UPDATE quests SET progress = ? WHERE id = ?`, next, int64(q.ID)); err != nil {
					return err
				}
				break
			}
			if done, err = finishQuestTx(ctx, tx, uid, q, ms); err != nil {
				return err
			}
		case QuestDebugSet:
			if !slices.Contains(questKinds, d.Kind) || d.Target < 1 || d.Target > questTargetMax {
				return ErrQuestDebug
			}
			q, err := open()
			if err != nil {
				return err
			}
			if err := restartQuestTx(ctx, tx, uid, q, d.Kind, d.Target, ms); err != nil {
				return err
			}
		case QuestDebugStep:
			if d.Step < 1 || d.Step > QuestDebugStepMax {
				return ErrQuestDebug
			}
			// the open step goes, and every step kept from the one landed on up, so
			// each opens fresh as the climb reaches it again
			if _, err := tx.ExecContext(ctx, `DELETE FROM quest_seen WHERE user_id = ? AND scope IN (SELECT 'q:' || CAST(id AS TEXT) FROM quests WHERE user_id = ? AND track = ? AND (period >= ? OR done_at = 0))`,
				int64(uid), int64(uid), QuestTrial, d.Step); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM quests WHERE user_id = ? AND track = ? AND (period >= ? OR done_at = 0)`, int64(uid), QuestTrial, d.Step); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE quest_players SET trial_step = ? WHERE user_id = ?`, d.Step, int64(uid)); err != nil {
				return err
			}
			if err := newTrialStepTx(ctx, tx, uid, d.Step, "", ms); err != nil {
				return err
			}
		case QuestDebugNewDay:
			if _, err := tx.ExecContext(ctx, `DELETE FROM quest_seen WHERE user_id = ? AND scope IN (SELECT 'q:' || CAST(id AS TEXT) FROM quests WHERE user_id = ? AND track IN (?, ?) AND period = ?)`,
				int64(uid), int64(uid), QuestDaily, QuestBonus, day); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM quests WHERE user_id = ? AND track IN (?, ?) AND period = ?`, int64(uid), QuestDaily, QuestBonus, day); err != nil {
				return err
			}
			if _, err := ensureQuestsTx(ctx, tx, uid, ms); err != nil {
				return err
			}
		case QuestDebugUses:
			if _, err := tx.ExecContext(ctx, `UPDATE quest_players SET reroll_day = 0, skip_day = 0 WHERE user_id = ?`, int64(uid)); err != nil {
				return err
			}
		default:
			return ErrQuestDebug
		}
		log.Action = LogQuestDebug
		log.Details, _ = json.Marshal(d)
		return appendModLogTx(ctx, tx, log)
	})
	return done, err
}
