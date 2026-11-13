package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// ErrLimit is returned when a per-object cap is reached.
var ErrLimit = errors.New("limit reached")

var ErrStaleEpoch = errors.New("message key epoch is no longer current")

// MaxReactionEmoji caps distinct emoji per message.
const MaxReactionEmoji = 20

// messageCols are what a message is read with. has_attachments is written
// (the legal record sums it) but never read back: the attachments themselves
// come with the message.
const messageCols = `id, channel_id, author_id, type, ciphertext, key_conv_id, key_epoch, reply_to_id, mentions, mention_roles, mention_everyone, flags, nonce, edited_at, created_at, webhook_id, plain`

// messageInsertCols are what a new message is written with.
const messageInsertCols = messageCols + `, has_attachments`

// messageColsM is messageCols qualified with the alias m, for joins.
const messageColsM = `m.id, m.channel_id, m.author_id, m.type, m.ciphertext, m.key_conv_id, m.key_epoch, m.reply_to_id, m.mentions, m.mention_roles, m.mention_everyone, m.flags, m.nonce, m.edited_at, m.created_at, m.webhook_id, m.plain`

// MessageQuery selects a page of messages. Results are always ascending by id.
type MessageQuery struct {
	Before ids.ID
	After  ids.ID
	Around ids.ID
	Limit  int
}

func idsJSON(list []ids.ID) string {
	if len(list) == 0 {
		return "[]"
	}
	strs := make([]string, len(list))
	for i, id := range list {
		strs[i] = id.String()
	}
	b, _ := json.Marshal(strs)
	return string(b)
}

func parseIDsJSON(s string) []ids.ID {
	var strs []string
	if err := json.Unmarshal([]byte(s), &strs); err != nil {
		return []ids.ID{}
	}
	out := make([]ids.ID, 0, len(strs))
	for _, v := range strs {
		if id, err := ids.Parse(v); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func scanMessage(r rowScanner) (*Message, error) {
	var m Message
	var ct sql.NullString
	var conv, epoch, reply, edited sql.NullInt64
	var mentions, roles string
	var everyone int64
	var nonce, plain sql.NullString
	var webhook sql.NullInt64
	if err := r.Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Type, &ct, &conv, &epoch, &reply, &mentions, &roles, &everyone, &m.Flags, &nonce, &edited, &m.CreatedAt, &webhook, &plain); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if ct.Valid {
		m.Ciphertext = json.RawMessage(ct.String)
	} else {
		m.Ciphertext = json.RawMessage("null")
	}
	m.KeyConvID = idPtr(conv)
	m.KeyEpoch = intPtr(epoch)
	m.ReplyToID = idPtr(reply)
	m.EditedAt = int64Ptr(edited)
	m.Mentions = parseIDsJSON(mentions)
	m.MentionRoles = parseIDsJSON(roles)
	m.MentionEveryone = everyone == 1
	m.Nonce = nonce.String
	m.WebhookID = idPtr(webhook)
	if plain.Valid && plain.String != "" {
		if m.Type == "crosspost" {
			m.Crosspost = json.RawMessage(plain.String)
		} else if m.Type == "system_automod" {
			m.Automod = json.RawMessage(plain.String)
		} else if m.WebhookID == nil && m.Type == "system_poll_result" && !strings.Contains(plain.String, `"embeds"`) {
			// the line closing a person's poll: numbers the server wrote
			// itself (a bot's poll's line says it in words, as a webhook's does)
			var out PollOutcome
			if json.Unmarshal([]byte(plain.String), &out) == nil {
				m.PollResult = &out
			}
		} else {
			m.Webhook = json.RawMessage(plain.String)
		}
	}
	m.Attachments = []Attachment{}
	m.Reactions = []ReactionSummary{}
	return &m, nil
}

// InsertMessage stores m with its attachments, bumps the channel's last
// message and references attachment blobs. A duplicate nonce returns
// ErrAlreadyExists.
func (s *Store) InsertMessage(ctx context.Context, m *Message) error {
	return s.insertMessage(ctx, m, false)
}

// InsertEncryptedMessage checks the current epoch under the same transaction
// lock as epoch creation, so a rotation cannot race the message's admission.
func (s *Store) InsertEncryptedMessage(ctx context.Context, m *Message) error {
	return s.insertMessage(ctx, m, true)
}

func (s *Store) insertMessage(ctx context.Context, m *Message, currentKey bool) error {
	if m.ID.IsZero() {
		m.ID = ids.New()
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = m.ID.Time().UnixMilli()
	}
	if m.Type == "" {
		m.Type = "default"
	}
	var ct any
	if len(m.Ciphertext) > 0 && string(m.Ciphertext) != "null" {
		ct = string(m.Ciphertext)
	}
	var nonce any
	if m.Nonce != "" {
		nonce = m.Nonce
	}
	var epoch any
	if m.KeyEpoch != nil {
		epoch = int64(*m.KeyEpoch)
	}
	var plain any
	if len(m.Crosspost) > 0 {
		plain = string(m.Crosspost)
	} else if len(m.Automod) > 0 {
		plain = string(m.Automod)
	} else if len(m.Webhook) > 0 {
		plain = string(m.Webhook)
	} else if m.PollResult != nil {
		b, err := json.Marshal(m.PollResult)
		if err != nil {
			return err
		}
		plain = string(b)
	}
	return s.Tx(ctx, func(tx *db.Tx) error {
		if currentKey {
			if m.KeyConvID == nil || m.KeyEpoch == nil {
				return ErrStaleEpoch
			}
			if err := requireCurrentEpoch(ctx, tx, *m.KeyConvID, *m.KeyEpoch); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO messages (`+messageInsertCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(channel_id, author_id, nonce) WHERE nonce IS NOT NULL DO NOTHING`,
			int64(m.ID), int64(m.ChannelID), int64(m.AuthorID), m.Type, ct, nullID(m.KeyConvID), epoch, nullID(m.ReplyToID), idsJSON(m.Mentions), idsJSON(m.MentionRoles), boolInt(m.MentionEveryone), m.Flags, nonce, nullInt64(m.EditedAt), m.CreatedAt, nullID(m.WebhookID), plain, boolInt(len(m.Attachments) > 0))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrAlreadyExists
		}
		if m.NewPoll != nil {
			if _, err := tx.ExecContext(ctx, `INSERT INTO polls (message_id, options_count, multiselect, expires_at) VALUES (?,?,?,?)`,
				int64(m.ID), m.NewPoll.Answers, boolInt(m.NewPoll.Multiselect), m.NewPoll.ExpiresAt); err != nil {
				return err
			}
		}
		for i := range m.Attachments {
			a := &m.Attachments[i]
			if a.ID.IsZero() {
				a.ID = ids.New()
			}
			a.MessageID, a.ChannelID, a.CreatedAt = m.ID, m.ChannelID, m.CreatedAt
			if _, err := tx.ExecContext(ctx, `INSERT INTO attachments (id, message_id, channel_id, uploader_id, blob_id, size, created_at) VALUES (?,?,?,?,?,?,?)`, int64(a.ID), int64(m.ID), int64(m.ChannelID), int64(m.AuthorID), int64(a.BlobID), a.Size, m.CreatedAt); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE blobs SET ref_count = ref_count + 1 WHERE id = ?`, int64(a.BlobID)); err != nil {
				return err
			}
		}
		// what people say is counted, for threads to show how much is in them
		said := 0
		if !strings.HasPrefix(m.Type, "system_") {
			said = 1
		}
		_, err = tx.ExecContext(ctx, `UPDATE channels SET last_message_id = `+tx.Dialect().Greatest("coalesce(last_message_id, 0)", "?")+`, message_count = message_count + ? WHERE id = ?`, int64(m.ID), said, int64(m.ChannelID))
		return err
	})
}

// MessageByNonce finds the message a duplicate send refers to.
func (s *Store) MessageByNonce(ctx context.Context, chID, author ids.ID, nonce string, forUser ids.ID) (*Message, error) {
	m, err := scanMessage(s.db.R.QueryRowContext(ctx, `SELECT `+messageCols+` FROM messages WHERE channel_id = ? AND author_id = ? AND nonce = ?`, int64(chID), int64(author), nonce))
	if err != nil {
		return nil, err
	}
	if err := s.hydrate(ctx, []*Message{m}, forUser); err != nil {
		return nil, err
	}
	return m, nil
}

// MessageByID loads one message with attachments and reactions.
func (s *Store) MessageByID(ctx context.Context, id ids.ID, forUser ids.ID) (*Message, error) {
	m, err := scanMessage(s.db.R.QueryRowContext(ctx, `SELECT `+messageCols+` FROM messages WHERE id = ?`, int64(id)))
	if err != nil {
		return nil, err
	}
	if err := s.hydrate(ctx, []*Message{m}, forUser); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Store) queryMessages(ctx context.Context, q string, args ...any) ([]*Message, error) {
	rows, err := s.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Messages returns a page of a channel's messages, ascending by id.
func (s *Store) Messages(ctx context.Context, chID ids.ID, q MessageQuery, forUser ids.ID) ([]*Message, error) {
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var list []*Message
	var err error
	switch {
	case !q.Around.IsZero():
		half := limit / 2
		before, err := s.queryMessages(ctx, `SELECT `+messageCols+` FROM messages WHERE channel_id = ? AND id < ? ORDER BY id DESC LIMIT ?`, int64(chID), int64(q.Around), half)
		if err != nil {
			return nil, err
		}
		after, err := s.queryMessages(ctx, `SELECT `+messageCols+` FROM messages WHERE channel_id = ? AND id >= ? ORDER BY id ASC LIMIT ?`, int64(chID), int64(q.Around), limit-half)
		if err != nil {
			return nil, err
		}
		list = append(before, after...)
	case !q.After.IsZero():
		list, err = s.queryMessages(ctx, `SELECT `+messageCols+` FROM messages WHERE channel_id = ? AND id > ? ORDER BY id ASC LIMIT ?`, int64(chID), int64(q.After), limit)
	case !q.Before.IsZero():
		list, err = s.queryMessages(ctx, `SELECT `+messageCols+` FROM messages WHERE channel_id = ? AND id < ? ORDER BY id DESC LIMIT ?`, int64(chID), int64(q.Before), limit)
	default:
		list, err = s.queryMessages(ctx, `SELECT `+messageCols+` FROM messages WHERE channel_id = ? ORDER BY id DESC LIMIT ?`, int64(chID), limit)
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	if list == nil {
		list = []*Message{}
	}
	if err := s.hydrate(ctx, list, forUser); err != nil {
		return nil, err
	}
	return list, nil
}

// maxLookup is the most messages one MessagesByIDs call reads.
const maxLookup = 100

// MessagesByIDs loads the listed messages of one channel, ascending by id.
// Ids that are gone, or belong to another channel, are left out, so what is
// missing from the answer has been deleted.
func (s *Store) MessagesByIDs(ctx context.Context, chID ids.ID, list []ids.ID, forUser ids.ID) ([]*Message, error) {
	if len(list) > maxLookup {
		list = list[:maxLookup]
	}
	if len(list) == 0 {
		return []*Message{}, nil
	}
	args := append([]any{int64(chID)}, idArgs(list)...)
	out, err := s.queryMessages(ctx, `SELECT `+messageCols+` FROM messages WHERE channel_id = ? AND id IN (`+placeholders(len(list))+`) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []*Message{}
	}
	if err := s.hydrate(ctx, out, forUser); err != nil {
		return nil, err
	}
	return out, nil
}

// WithReferences fills in Referenced on every message in msgs that points at
// another: the message it answers as it is now, or null when that has been
// deleted. A message only ever points within its own channel; one pointing
// anywhere else is treated as pointing at nothing that exists.
func (s *Store) WithReferences(ctx context.Context, msgs []*Message, forUser ids.ID) error {
	byChannel := map[ids.ID][]ids.ID{}
	seen := map[ids.ID]bool{}
	for _, m := range msgs {
		if m == nil || m.ReplyToID == nil || seen[*m.ReplyToID] {
			continue
		}
		seen[*m.ReplyToID] = true
		byChannel[m.ChannelID] = append(byChannel[m.ChannelID], *m.ReplyToID)
	}
	if len(byChannel) == 0 {
		return nil
	}
	found := map[ids.ID][]byte{}
	for chID, want := range byChannel {
		for len(want) > 0 {
			n := min(len(want), maxLookup)
			list, err := s.MessagesByIDs(ctx, chID, want[:n], forUser)
			if err != nil {
				return err
			}
			for _, t := range list {
				b, err := json.Marshal(t)
				if err != nil {
					return err
				}
				found[t.ID] = b
			}
			want = want[n:]
		}
	}
	for _, m := range msgs {
		if m == nil || m.ReplyToID == nil {
			continue
		}
		if b, ok := found[*m.ReplyToID]; ok {
			m.Referenced = b
		} else {
			m.Referenced = json.RawMessage("null")
		}
	}
	return nil
}

// hydrate fills attachments and reaction summaries for msgs.
func (s *Store) hydrate(ctx context.Context, msgs []*Message, forUser ids.ID) error {
	if len(msgs) == 0 {
		return nil
	}
	byID := make(map[ids.ID]*Message, len(msgs))
	list := make([]ids.ID, 0, len(msgs))
	for _, m := range msgs {
		byID[m.ID] = m
		list = append(list, m.ID)
	}
	ph := placeholders(len(list))
	rows, err := s.db.R.QueryContext(ctx, `SELECT id, message_id, channel_id, uploader_id, blob_id, size, created_at FROM attachments WHERE message_id IN (`+ph+`) ORDER BY id`, idArgs(list)...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.ChannelID, &a.UploaderID, &a.BlobID, &a.Size, &a.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		if m := byID[a.MessageID]; m != nil {
			m.Attachments = append(m.Attachments, a)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if err := s.fillReactionRoles(ctx, byID, ph, list); err != nil {
		return err
	}
	args := append([]any{int64(forUser)}, idArgs(list)...)
	rrows, err := s.db.R.QueryContext(ctx, `SELECT message_id, emoji, count(*), max(CASE WHEN user_id = ? THEN 1 ELSE 0 END)
		FROM reactions WHERE message_id IN (`+ph+`)
		GROUP BY message_id, emoji ORDER BY min(created_at), min(user_id)`, args...)
	if err != nil {
		return err
	}
	defer rrows.Close()
	for rrows.Next() {
		var mid int64
		var rs ReactionSummary
		var me int64
		if err := rrows.Scan(&mid, &rs.Emoji, &rs.Count, &me); err != nil {
			return err
		}
		rs.Me = me == 1
		if m := byID[ids.ID(mid)]; m != nil {
			m.Reactions = append(m.Reactions, rs)
		}
	}
	if err := rrows.Err(); err != nil {
		return err
	}
	if err := s.hydrateTrades(ctx, msgs); err != nil {
		return err
	}
	return s.hydratePolls(ctx, msgs, forUser)
}

// UpdateMessage replaces the ciphertext and mention metadata.
func (s *Store) UpdateMessage(ctx context.Context, id ids.ID, ciphertext json.RawMessage, keyConv ids.ID, keyEpoch int, mentions, roles []ids.ID, everyone bool, editedAt int64) error {
	return s.updateMessage(ctx, id, ciphertext, keyConv, keyEpoch, mentions, roles, everyone, editedAt, false)
}

// UpdateEncryptedMessage applies the same epoch rule to edits as new sends.
func (s *Store) UpdateEncryptedMessage(ctx context.Context, id ids.ID, ciphertext json.RawMessage, keyConv ids.ID, keyEpoch int, mentions, roles []ids.ID, everyone bool, editedAt int64) error {
	return s.updateMessage(ctx, id, ciphertext, keyConv, keyEpoch, mentions, roles, everyone, editedAt, true)
}

func requireCurrentEpoch(ctx context.Context, tx *db.Tx, conv ids.ID, epoch int) error {
	if err := tx.Serialise(ctx, "epoch", int64(conv)); err != nil {
		return err
	}
	var latest sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT max(epoch) FROM key_epochs WHERE conv_id = ?`, int64(conv)).Scan(&latest); err != nil {
		return err
	}
	if !latest.Valid || latest.Int64 != int64(epoch) {
		return ErrStaleEpoch
	}
	return nil
}

func (s *Store) updateMessage(ctx context.Context, id ids.ID, ciphertext json.RawMessage, keyConv ids.ID, keyEpoch int, mentions, roles []ids.ID, everyone bool, editedAt int64, currentKey bool) error {
	// written the way InsertMessage writes it: no conversation is NULL, not 0
	var conv *ids.ID
	if !keyConv.IsZero() {
		conv = &keyConv
	}
	return s.Tx(ctx, func(tx *db.Tx) error {
		if currentKey {
			if err := requireCurrentEpoch(ctx, tx, keyConv, keyEpoch); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `UPDATE messages SET ciphertext = ?, key_conv_id = ?, key_epoch = ?, mentions = ?, mention_roles = ?, mention_everyone = ?, edited_at = ? WHERE id = ?`, string(ciphertext), nullID(conv), keyEpoch, idsJSON(mentions), idsJSON(roles), boolInt(everyone), editedAt, int64(id))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// SetMessageFlags overwrites the flags bitfield.
func (s *Store) SetMessageFlags(ctx context.Context, id ids.ID, flags int64) error {
	res, err := s.db.W.ExecContext(ctx, `UPDATE messages SET flags = ? WHERE id = ?`, flags, int64(id))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMessage removes a message, releasing attachment blobs and fixing the
// channel's last_message_id.
func (s *Store) DeleteMessage(ctx context.Context, id ids.ID) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		var chID int64
		if err := tx.QueryRowContext(ctx, `SELECT channel_id FROM messages WHERE id = ?`, int64(id)).Scan(&chID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := deleteMessageTx(ctx, tx, id); err != nil {
			return err
		}
		return fixLastMessageTx(ctx, tx, ids.ID(chID))
	})
}

// MessageAuthors returns who wrote each listed message that is in the channel;
// ids that are gone or belong to another channel are left out.
func (s *Store) MessageAuthors(ctx context.Context, chID ids.ID, list []ids.ID) (map[ids.ID]ids.ID, error) {
	out := make(map[ids.ID]ids.ID, len(list))
	if len(list) == 0 {
		return out, nil
	}
	args := []any{int64(chID)}
	for _, id := range list {
		args = append(args, int64(id))
	}
	rows, err := s.db.R.QueryContext(ctx, `SELECT id, author_id FROM messages WHERE channel_id = ? AND id IN (`+placeholders(len(list))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, author int64
		if err := rows.Scan(&id, &author); err != nil {
			return nil, err
		}
		out[ids.ID(id)] = ids.ID(author)
	}
	return out, rows.Err()
}

// MentionMeta is what a message mentioned, which is what unread badges
// counted it by.
type MentionMeta struct {
	ID              ids.ID   `json:"id"`
	AuthorID        ids.ID   `json:"author_id"`
	Mentions        []ids.ID `json:"mentions"`
	MentionRoles    []ids.ID `json:"mention_roles"`
	MentionEveryone bool     `json:"mention_everyone"`
}

// Deleted is what a deletion took from one channel: the ids, and what those
// of them that mentioned anyone mentioned. The mentions are for readers who
// never loaded the messages: their badges counted them, and only the
// deletion's event can tell them which to take back.
type Deleted struct {
	IDs      []ids.ID
	Mentions []MentionMeta
}

// mentionCols are the columns mentionOf reads, after id and channel_id.
const mentionCols = `author_id, mentions, mention_roles, mention_everyone`

// note records one message found for deletion in its channel's Deleted.
func (d *Deleted) note(id, author int64, mentions, roles string, everyone int64) {
	d.IDs = append(d.IDs, ids.ID(id))
	if everyone == 1 || mentions != "[]" || roles != "[]" {
		d.Mentions = append(d.Mentions, MentionMeta{ID: ids.ID(id), AuthorID: ids.ID(author), Mentions: parseIDsJSON(mentions), MentionRoles: parseIDsJSON(roles), MentionEveryone: everyone == 1})
	}
}

func newDeleted() *Deleted { return &Deleted{IDs: []ids.ID{}, Mentions: []MentionMeta{}} }

// DeleteMessages removes several messages of one channel in a single
// transaction, releasing their attachment blobs and fixing the channel's
// last_message_id. Ids that are gone or belong to another channel are
// skipped; what was actually deleted is returned.
func (s *Store) DeleteMessages(ctx context.Context, chID ids.ID, list []ids.ID) (*Deleted, error) {
	var deleted *Deleted
	err := s.Tx(ctx, func(tx *db.Tx) error {
		deleted = newDeleted()
		for _, id := range list {
			var author, everyone int64
			var mentions, roles string
			err := tx.QueryRowContext(ctx, `SELECT `+mentionCols+` FROM messages WHERE id = ? AND channel_id = ?`, int64(id), int64(chID)).Scan(&author, &mentions, &roles, &everyone)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if err := deleteMessageTx(ctx, tx, id); err != nil {
				return err
			}
			deleted.note(int64(id), author, mentions, roles, everyone)
		}
		if len(deleted.IDs) == 0 {
			return nil
		}
		return fixLastMessageTx(ctx, tx, chID)
	})
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

// deleteMessageTx drops one message row, releasing one blob reference per
// attachment row, however many rows share a blob.
func deleteMessageTx(ctx context.Context, tx *db.Tx, id ids.ID) error {
	if _, err := tx.ExecContext(ctx, `UPDATE blobs SET ref_count = `+refFloor(tx.Dialect(), "ref_count - (SELECT count(*) FROM attachments a WHERE a.message_id = ? AND a.blob_id = blobs.id)")+` WHERE id IN (SELECT blob_id FROM attachments WHERE message_id = ?)`, int64(id), int64(id)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, int64(id))
	return err
}

// fixLastMessageTx points the channel's last_message_id at its newest
// remaining message, and counts again what has been said in it.
func fixLastMessageTx(ctx context.Context, tx *db.Tx, chID ids.ID) error {
	_, err := tx.ExecContext(ctx, `UPDATE channels SET last_message_id = (SELECT max(id) FROM messages WHERE channel_id = ?),
		message_count = (SELECT count(*) FROM messages WHERE channel_id = ? AND type NOT LIKE 'system!_%' ESCAPE '!') WHERE id = ?`, int64(chID), int64(chID), int64(chID))
	return err
}

// AddReaction records a reaction; added is false when it already existed.
func (s *Store) AddReaction(ctx context.Context, msgID, uid ids.ID, emoji string) (added bool, err error) {
	err = s.Tx(ctx, func(tx *db.Tx) error {
		// the distinct-emoji count is read and then written against, so this
		// message belongs to this transaction until it commits
		if err := tx.Serialise(ctx, "reactions", int64(msgID)); err != nil {
			return err
		}
		var distinct, present int
		if err := tx.QueryRowContext(ctx, `SELECT count(DISTINCT emoji), coalesce(sum(CASE WHEN emoji = ? THEN 1 ELSE 0 END), 0) FROM reactions WHERE message_id = ?`, emoji, int64(msgID)).Scan(&distinct, &present); err != nil {
			return err
		}
		if present == 0 && distinct >= MaxReactionEmoji {
			return ErrLimit
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO reactions (message_id, user_id, emoji, created_at) VALUES (?,?,?,?) ON CONFLICT DO NOTHING`, int64(msgID), int64(uid), emoji, nowMs())
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		added = n > 0
		return nil
	})
	return added, err
}

// RemoveReaction deletes a reaction; removed is false when it did not exist.
func (s *Store) RemoveReaction(ctx context.Context, msgID, uid ids.ID, emoji string) (bool, error) {
	res, err := s.db.W.ExecContext(ctx, `DELETE FROM reactions WHERE message_id = ? AND user_id = ? AND emoji = ?`, int64(msgID), int64(uid), emoji)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Reactors lists who reacted with emoji, oldest first.
func (s *Store) Reactors(ctx context.Context, msgID ids.ID, emoji string, limit int) ([]ids.ID, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.R.QueryContext(ctx, `SELECT user_id FROM reactions WHERE message_id = ? AND emoji = ? ORDER BY created_at, user_id LIMIT ?`, int64(msgID), emoji, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ids.ID{}
	for rows.Next() {
		var u int64
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, ids.ID(u))
	}
	return out, rows.Err()
}

// ReactorsAfter lists who reacted with emoji, by account id, after one id,
// at most limit: the bot API's paging, which goes by id.
func (s *Store) ReactorsAfter(ctx context.Context, msgID ids.ID, emoji string, after ids.ID, limit int) ([]ids.ID, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	return s.idList(ctx, `SELECT user_id FROM reactions WHERE message_id = ? AND emoji = ? AND user_id > ? ORDER BY user_id LIMIT ?`, int64(msgID), emoji, int64(after), limit)
}

// RemoveAllReactions takes every reaction off a message, and says whether
// there were any.
func (s *Store) RemoveAllReactions(ctx context.Context, msgID ids.ID) (bool, error) {
	res, err := s.db.W.ExecContext(ctx, `DELETE FROM reactions WHERE message_id = ?`, int64(msgID))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RemoveEmojiReactions takes one emoji's reactions off a message, everyone's.
func (s *Store) RemoveEmojiReactions(ctx context.Context, msgID ids.ID, emoji string) (bool, error) {
	res, err := s.db.W.ExecContext(ctx, `DELETE FROM reactions WHERE message_id = ? AND emoji = ?`, int64(msgID), emoji)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MaxPins is how many messages one channel can have pinned. Pins lists them
// all at once, so past this a list would have had to stop partway, silently.
const MaxPins = 500

// PinMessage pins a message in its channel; ErrTooMany once MaxPins are.
func (s *Store) PinMessage(ctx context.Context, chID, msgID, by ids.ID) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		// counted and then added to, so the channel is this transaction's alone
		if err := tx.Serialise(ctx, "pins", int64(chID)); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pins WHERE channel_id = ?`, int64(chID)).Scan(&n); err != nil {
			return err
		}
		if n >= MaxPins {
			var already int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pins WHERE channel_id = ? AND message_id = ?`, int64(chID), int64(msgID)).Scan(&already); err != nil {
				return err
			}
			if already == 0 {
				return ErrTooMany
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO pins (channel_id, message_id, pinned_by, pinned_at) VALUES (?,?,?,?) ON CONFLICT DO NOTHING`, int64(chID), int64(msgID), int64(by), nowMs())
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrAlreadyExists
		}
		_, err = tx.ExecContext(ctx, `UPDATE messages SET flags = flags | ? WHERE id = ? AND channel_id = ?`, MsgFlagPinned, int64(msgID), int64(chID))
		return err
	})
}

// UnpinMessage removes a pin.
func (s *Store) UnpinMessage(ctx context.Context, chID, msgID ids.ID) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM pins WHERE channel_id = ? AND message_id = ?`, int64(chID), int64(msgID))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `UPDATE messages SET flags = flags & ? WHERE id = ?`, ^int64(MsgFlagPinned), int64(msgID))
		return err
	})
}

// Pins lists pinned messages, most recently pinned first — all of them, as
// PinMessage stops at MaxPins. Two pinned in the same millisecond come back
// in the same order every time.
// LastPinAt is when a message was last pinned in a channel, 0 for none.
func (s *Store) LastPinAt(ctx context.Context, chID ids.ID) (int64, error) {
	var at sql.NullInt64
	err := s.db.R.QueryRowContext(ctx, `SELECT max(pinned_at) FROM pins WHERE channel_id = ?`, int64(chID)).Scan(&at)
	return at.Int64, err
}

// PinsPage lists a channel's pins newest first, with when each was pinned,
// pinned before a moment (0 for the newest), at most limit of them.
func (s *Store) PinsPage(ctx context.Context, chID ids.ID, before int64, limit int, forUser ids.ID) ([]*Message, []int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	q := `SELECT m.id, p.pinned_at FROM messages m JOIN pins p ON p.message_id = m.id WHERE p.channel_id = ?`
	args := []any{int64(chID)}
	if before > 0 {
		q += ` AND p.pinned_at < ?`
		args = append(args, before)
	}
	rows, err := s.db.R.QueryContext(ctx, q+` ORDER BY p.pinned_at DESC, p.message_id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, nil, err
	}
	var order []ids.ID
	at := map[ids.ID]int64{}
	for rows.Next() {
		var id ids.ID
		var when int64
		if err := rows.Scan(&id, &when); err != nil {
			rows.Close()
			return nil, nil, err
		}
		order = append(order, id)
		at[id] = when
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	list, err := s.MessagesByIDs(ctx, chID, order, forUser)
	if err != nil {
		return nil, nil, err
	}
	byID := map[ids.ID]*Message{}
	for _, m := range list {
		byID[m.ID] = m
	}
	out := make([]*Message, 0, len(order))
	times := make([]int64, 0, len(order))
	for _, id := range order {
		if m := byID[id]; m != nil {
			out = append(out, m)
			times = append(times, at[id])
		}
	}
	return out, times, nil
}

func (s *Store) Pins(ctx context.Context, chID ids.ID, forUser ids.ID) ([]*Message, error) {
	list, err := s.queryMessages(ctx, `SELECT `+messageColsM+` FROM messages m JOIN pins p ON p.message_id = m.id WHERE p.channel_id = ? ORDER BY p.pinned_at DESC, p.message_id DESC LIMIT ?`, int64(chID), MaxPins)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*Message{}
	}
	if err := s.hydrate(ctx, list, forUser); err != nil {
		return nil, err
	}
	return list, nil
}

// CountPins returns how many messages are pinned in a channel.
func (s *Store) CountPins(ctx context.Context, chID ids.ID) (int, error) {
	var n int
	err := s.db.R.QueryRowContext(ctx, `SELECT count(*) FROM pins WHERE channel_id = ?`, int64(chID)).Scan(&n)
	return n, err
}

// AttachmentByID loads one attachment row.
func (s *Store) AttachmentByID(ctx context.Context, id ids.ID) (*Attachment, error) {
	var a Attachment
	err := s.db.R.QueryRowContext(ctx, `SELECT id, message_id, channel_id, uploader_id, blob_id, size, created_at FROM attachments WHERE id = ?`, int64(id)).Scan(&a.ID, &a.MessageID, &a.ChannelID, &a.UploaderID, &a.BlobID, &a.Size, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AttachmentByBlob finds the attachment row that owns a blob, if any.
func (s *Store) AttachmentByBlob(ctx context.Context, blobID ids.ID) (*Attachment, error) {
	var a Attachment
	// The channel this names is what the download handler checks access
	// against, so which row comes back must not be up to the query planner.
	err := s.db.R.QueryRowContext(ctx, `SELECT id, message_id, channel_id, uploader_id, blob_id, size, created_at FROM attachments WHERE blob_id = ? ORDER BY id LIMIT 1`, int64(blobID)).Scan(&a.ID, &a.MessageID, &a.ChannelID, &a.UploaderID, &a.BlobID, &a.Size, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AttachmentChannels lists the channels whose messages hold a blob (a post
// published to channels following it holds its files in each), oldest first.
func (s *Store) AttachmentChannels(ctx context.Context, blobID ids.ID) ([]ids.ID, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT channel_id FROM attachments WHERE blob_id = ? GROUP BY channel_id ORDER BY min(id) LIMIT 100`, int64(blobID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ids.ID
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, ids.ID(id))
	}
	return out, rows.Err()
}

// MaxPurgeMessages is how many of a banned member's messages one purge takes.
// The whole purge is a single transaction, so this is also how long it can
// hold the writer; a ban that would go over takes the newest that many and
// leaves the rest, which a repeat of the same ban picks up.
const MaxPurgeMessages = 5000

// DeleteMessagesByUserSince removes a user's recent messages in a server
// (ban with message purge). Returns the deleted ids per channel.
//
// It is one transaction rather than one per message. Deleting them one at a
// time meant a failure halfway left hundreds of messages already gone from the
// database with the error swallowing the whole result, so no message_delete_bulk
// was sent for any of them and every connected client went on showing messages
// that no longer existed. All or none, and what comes back is what went.
func (s *Store) DeleteMessagesByUserSince(ctx context.Context, serverID, uid ids.ID, since int64) (map[ids.ID]*Deleted, error) {
	out := map[ids.ID]*Deleted{}
	err := s.Tx(ctx, func(tx *db.Tx) error {
		clear(out)
		rows, err := tx.QueryContext(ctx, `SELECT m.id, m.channel_id, m.author_id, m.mentions, m.mention_roles, m.mention_everyone FROM messages m JOIN channels c ON c.id = m.channel_id
			WHERE c.server_id = ? AND m.author_id = ? AND m.created_at >= ? ORDER BY m.id DESC LIMIT ?`,
			int64(serverID), int64(uid), since, MaxPurgeMessages)
		if err != nil {
			return err
		}
		// The cursor is drained before anything else runs, so the delete
		// statements are not asking the pool for a second connection while
		// this one is still holding a result set open.
		var order []ids.ID
		for rows.Next() {
			var id, ch, author, everyone int64
			var mentions, roles string
			if err := rows.Scan(&id, &ch, &author, &mentions, &roles, &everyone); err != nil {
				rows.Close()
				return err
			}
			if out[ids.ID(ch)] == nil {
				out[ids.ID(ch)] = newDeleted()
			}
			out[ids.ID(ch)].note(id, author, mentions, roles, everyone)
			order = append(order, ids.ID(id))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range order {
			if err := deleteMessageTx(ctx, tx, id); err != nil {
				return err
			}
		}
		for ch := range out {
			if err := fixLastMessageTx(ctx, tx, ch); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LastMessageAt returns the creation time of the newest user-authored message by uid in a channel (0 when none); system messages do not count.
func (s *Store) LastMessageAt(ctx context.Context, chID, uid ids.ID) (int64, error) {
	var t sql.NullInt64
	err := s.db.R.QueryRowContext(ctx, `SELECT max(created_at) FROM messages WHERE channel_id = ? AND author_id = ? AND type IN ('default', 'reply')`, int64(chID), int64(uid)).Scan(&t)
	if err != nil {
		return 0, err
	}
	return t.Int64, nil
}
