package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

var ErrReviewScope = errors.New("historical review unavailable or scope changed")

const ReviewLimit = 50

type ReviewItem struct {
	ID     ids.ID `json:"id"`
	Digest string `json:"digest"`
}
type HistoryReview struct {
	ID             ids.ID       `json:"id"`
	Channel        ids.ID       `json:"channel_id"`
	Owner          ids.ID       `json:"owner_id"`
	First          ids.ID       `json:"first_id"`
	Second         ids.ID       `json:"second_id"`
	Reason         string       `json:"reason"`
	Expires        int64        `json:"expires_at"`
	Items          []ReviewItem `json:"items"`
	Files          bool         `json:"include_files"`
	FirstApproved  bool         `json:"first_approved"`
	SecondApproved bool         `json:"second_approved"`
	Status         string       `json:"status"`
}

const reviewCols = "id,channel_id,owner_id,first_id,second_id,reason,expires_at,manifest,include_files,first_approved,second_approved,status"

func scanReview(row rowScanner) (*HistoryReview, error) {
	var r HistoryReview
	var manifest string
	if err := row.Scan(&r.ID, &r.Channel, &r.Owner, &r.First, &r.Second, &r.Reason, &r.Expires, &manifest, &r.Files, &r.FirstApproved, &r.SecondApproved, &r.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, json.Unmarshal([]byte(manifest), &r.Items)
}
func reviewAudit(ctx context.Context, tx *db.Tx, user *User, r *HistoryReview, action string) error {
	details, _ := json.Marshal(map[string]any{"request_id": r.ID, "channel_id": r.Channel, "owner_id": r.Owner, "items": r.Items, "include_files": r.Files, "expires_at": r.Expires})
	return appendModLogTx(ctx, tx, &ModLogEntry{Action: "history_" + action, ModeratorID: user.ID, ModeratorUsername: user.Username, Reason: r.Reason, Details: details})
}
func versionOf(cipher string) string {
	sum := sha256.Sum256([]byte(cipher))
	return hex.EncodeToString(sum[:])
}

// Only a two-person DM retains a provable fixed roster for its lifetime. A
// group's current members/key grants are insufficient evidence of past membership.
func (s *Store) RequestHistory(ctx context.Context, user *User, channel ids.ID, reason string, files bool, chosen []ids.ID, after, before ids.ID) (*HistoryReview, error) {
	r := &HistoryReview{ID: ids.New(), Channel: channel, Owner: user.ID, Reason: reason, Files: files, Status: "pending", Expires: time.Now().Add(7 * 24 * time.Hour).UnixMilli()}
	err := s.Tx(ctx, func(tx *db.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT p.user_id FROM dm_participants p JOIN channels c ON c.id=p.channel_id WHERE c.id=? AND c.type='dm' ORDER BY p.user_id`, int64(channel))
		if err != nil {
			return err
		}
		var members []ids.ID
		for rows.Next() {
			var id ids.ID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			members = append(members, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(members) != 2 {
			return ErrReviewScope
		}
		r.First, r.Second = members[0], members[1]
		q := `SELECT id,author_id,ciphertext FROM messages WHERE channel_id=? AND owner_recipient IS NULL AND type IN ('default','reply') AND ciphertext IS NOT NULL`
		args := []any{int64(channel)}
		if len(chosen) > 0 {
			q += " AND id IN (" + placeholders(len(chosen)) + ")"
			args = append(args, idArgs(chosen)...)
		}
		if !after.IsZero() {
			q += " AND id >= ?"
			args = append(args, int64(after))
		}
		if !before.IsZero() {
			q += " AND id <= ?"
			args = append(args, int64(before))
		}
		rows, err = tx.QueryContext(ctx, q+" ORDER BY id", args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, author ids.ID
			var cipher string
			if err := rows.Scan(&id, &author, &cipher); err != nil {
				return err
			}
			if author != r.First && author != r.Second {
				return ErrReviewScope
			}
			r.Items = append(r.Items, ReviewItem{id, versionOf(cipher)})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(r.Items) == 0 || (len(chosen) > 0 && len(chosen) != len(r.Items)) {
			return ErrReviewScope
		}
		// Approve large selections in independently inspectable batches. The
		// query above froze every id and version in one database snapshot.
		items := r.Items
		for offset := 0; offset < len(items); offset += ReviewLimit {
			batch := *r
			if offset != 0 {
				batch.ID = ids.New()
			}
			batch.Items = items[offset:min(offset+ReviewLimit, len(items))]
			if err := insertReview(ctx, tx, user, &batch); err != nil {
				return err
			}
			if offset == 0 {
				r.Items = batch.Items
			}
		}
		return nil
	})
	return r, err
}
func insertReview(ctx context.Context, tx *db.Tx, user *User, r *HistoryReview) error {
	manifest, err := json.Marshal(r.Items)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO owner_history_reviews(id,channel_id,owner_id,first_id,second_id,reason,expires_at,manifest,include_files) VALUES(?,?,?,?,?,?,?,?,?)`, int64(r.ID), int64(r.Channel), int64(r.Owner), int64(r.First), int64(r.Second), r.Reason, r.Expires, string(manifest), boolInt(r.Files))
	if err != nil {
		return err
	}
	return reviewAudit(ctx, tx, user, r, "requested")
}

func (s *Store) HistoryReviews(ctx context.Context, uid ids.ID, owner bool, before ids.ID) ([]*HistoryReview, error) {
	query := "SELECT " + reviewCols + " FROM owner_history_reviews WHERE expires_at>? AND (first_id=? OR second_id=?)"
	args := []any{nowMs(), int64(uid), int64(uid)}
	if owner {
		query = "SELECT " + reviewCols + " FROM owner_history_reviews WHERE expires_at>? AND owner_id=?"
		args = []any{nowMs(), int64(uid)}
	}
	if before != 0 {
		query += " AND id<?"
		args = append(args, int64(before))
	}
	rows, err := s.db.R.QueryContext(ctx, query+" ORDER BY id DESC LIMIT 50", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*HistoryReview{}
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Decisions, handoff and owner reads serialize on the same request. A second
// handoff cannot replace the submitted copy. Any change to a source message
// invalidates the whole frozen scope.
func (s *Store) HistoryAction(ctx context.Context, user *User, id ids.ID, action string, boxes map[string][]byte) ([]*Message, error) {
	var out []*Message
	err := s.Tx(ctx, func(tx *db.Tx) error {
		if err := tx.Serialise(ctx, "history-review", int64(id)); err != nil {
			return err
		}
		r, err := scanReview(tx.QueryRowContext(ctx, "SELECT "+reviewCols+" FROM owner_history_reviews WHERE id=?", int64(id)))
		if err != nil {
			return err
		}
		if action == "share" || action == "view" {
			var current ids.ID
			err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE flags & ? != 0 AND flags & ? = 0 ORDER BY id LIMIT 1`, FlagAdmin, FlagDeleted|NotPeople).Scan(&current)
			if err != nil {
				return err
			}
			if current != r.Owner {
				return ErrReviewScope
			}
		}
		if action == "view" {
			if user.ID != r.Owner {
				return ErrNotFound
			}
		} else if user.ID != r.First && user.ID != r.Second {
			return ErrNotFound
		}
		if r.Expires <= nowMs() || r.Status == "cancelled" {
			return ErrReviewScope
		}
		if action == "withdraw" {
			_, err = tx.ExecContext(ctx, "UPDATE owner_history_reviews SET status='cancelled',first_approved=0,second_approved=0,boxes='' WHERE id=?", int64(id))
			if err != nil {
				return err
			}
			return reviewAudit(ctx, tx, user, r, "withdrawn")
		}
		if action == "view" && r.Status != "shared" || action != "view" && r.Status != "pending" {
			return ErrReviewScope
		}
		if action == "view" {
			var saved string
			if err := tx.QueryRowContext(ctx, "SELECT boxes FROM owner_history_reviews WHERE id=?", int64(id)).Scan(&saved); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(saved), &boxes); err != nil {
				return err
			}
		}
		for _, item := range r.Items {
			m, err := scanMessage(tx.QueryRowContext(ctx, "SELECT "+messageCols+" FROM messages WHERE id=? AND channel_id=?", int64(item.ID), int64(r.Channel)))
			if err != nil {
				return err
			}
			if versionOf(string(m.Ciphertext)) != item.Digest || m.OwnerRecipient != nil {
				return ErrReviewScope
			}
			if action == "preview" {
				out = append(out, m)
			}
			if action == "view" {
				var env map[string]any
				if err := json.Unmarshal(m.Ciphertext, &env); err != nil {
					return err
				}
				env["owner"] = map[string]any{"id": r.Owner, "box": boxes[item.ID.String()]}
				m.Ciphertext, err = json.Marshal(env)
				if err != nil {
					return err
				}
				m.OwnerRecipient = &r.Owner
				out = append(out, m)
			}
		}
		switch action {
		case "preview":
			return nil
		case "approve":
			column := "first_approved"
			if user.ID == r.Second {
				column = "second_approved"
			}
			_, err = tx.ExecContext(ctx, "UPDATE owner_history_reviews SET "+column+"=1 WHERE id=?", int64(id))
		case "share":
			if !r.FirstApproved || !r.SecondApproved || len(boxes) != len(r.Items) {
				return ErrReviewScope
			}
			for _, m := range r.Items {
				if n := len(boxes[m.ID.String()]); n <= 60 || n > 65536 {
					return ErrReviewScope
				}
			}
			if r.Files {
				for _, item := range r.Items {
					_, err := tx.ExecContext(ctx, `INSERT INTO owner_history_files(review_id,blob_id) SELECT ?,blob_id FROM attachments WHERE message_id=? ON CONFLICT DO NOTHING`, int64(r.ID), int64(item.ID))
					if err != nil {
						return err
					}
				}
			}
			data, e := json.Marshal(boxes)
			if e != nil {
				return e
			}
			_, err = tx.ExecContext(ctx, "UPDATE owner_history_reviews SET status='shared',boxes=? WHERE id=?", string(data), int64(id))
		case "view":
		default:
			return ErrReviewScope
		}
		if err != nil {
			return err
		}
		return reviewAudit(ctx, tx, user, r, action)
	})
	return out, err
}

func (s *Store) ExpireHistoryReviews(ctx context.Context) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT "+reviewCols+" FROM owner_history_reviews WHERE expires_at<=? ORDER BY id LIMIT 500", nowMs())
		if err != nil {
			return err
		}
		var expired []*HistoryReview
		for rows.Next() {
			r, e := scanReview(rows)
			if e != nil {
				rows.Close()
				return e
			}
			expired = append(expired, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range expired {
			if _, err := tx.ExecContext(ctx, "DELETE FROM owner_history_reviews WHERE id=?", int64(r.ID)); err != nil {
				return err
			}
			if err := reviewAudit(ctx, tx, &User{ID: r.Owner, Username: "system"}, r, "expired"); err != nil {
				return err
			}
		}
		return nil
	})
}

// HistoricalAttachmentReview finds the active review that explicitly includes
// this file. The caller validates its entire frozen scope before serving bytes.
func (s *Store) HistoricalAttachmentReview(ctx context.Context, owner, blob ids.ID) (ids.ID, error) {
	var id ids.ID
	err := s.db.R.QueryRowContext(ctx, `SELECT r.id FROM owner_history_files f JOIN owner_history_reviews r ON r.id=f.review_id
		WHERE f.blob_id=? AND r.owner_id=? AND r.include_files=1 AND r.status='shared' AND r.expires_at>? ORDER BY r.id DESC LIMIT 1`, int64(blob), int64(owner), nowMs()).Scan(&id)
	return id, err
}
