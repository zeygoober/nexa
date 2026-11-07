package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// Bug reports are what members send the Nexa team from the app: a subject, a
// description, the screenshots or recording they chose to add and, if they
// kept it, a few facts about their app and device. Each has a thread with the
// team, which works the way a moderation case's does: the reporter writes as
// themselves, staff write as Nexa with their own name kept, and whoever wrote
// last is no longer the one being waited on.

// Where a report stands. Open is new; in progress is being worked on; the
// other three are finished: fixed, not going to change, or closed for another
// reason (a duplicate, not a bug, no answer).
const (
	BugOpen       = "open"
	BugInProgress = "in_progress"
	BugResolved   = "resolved"
	BugWontFix    = "wont_fix"
	BugClosed     = "closed"
)

// BugEventStatus names the system line a status change leaves in the thread;
// its body is the new status.
const BugEventStatus = "status"

// BugBlobKind is the kind of blob a report's files are stored as. It is not a
// public kind, so /media/{kind}/{id} never serves one.
const BugBlobKind = "bug"

// ValidBugStatus reports whether s is one of the states above.
func ValidBugStatus(s string) bool {
	switch s {
	case BugOpen, BugInProgress, BugResolved, BugWontFix, BugClosed:
		return true
	}
	return false
}

// ErrBugAttachment means a file named for a new report is not one its
// reporter uploaded for a report and has not sent with one yet.
var ErrBugAttachment = errors.New("bug attachment not available")

// ErrBugClosed means a report's thread was written in after staff finished it
// (resolved, won't fix or closed). Nobody writes in it until staff set it
// back to open or in progress.
var ErrBugClosed = errors.New("bug report closed")

// What a report is: something broken, or an idea for something new. Both are
// worked the same way, in the same table; the staff see them in separate tabs.
const (
	BugKindBug        = "bug"
	BugKindSuggestion = "suggestion"
)

// ValidBugKind reports whether k is one of the kinds above.
func ValidBugKind(k string) bool {
	return k == BugKindBug || k == BugKindSuggestion
}

// BugFinished reports whether a status ends a report's conversation.
func BugFinished(status string) bool {
	return status == BugResolved || status == BugWontFix || status == BugClosed
}

// BugReport is one report, without its files and thread.
type BugReport struct {
	ID                  ids.ID          `json:"id"`
	Kind                string          `json:"kind"`
	Number              int64           `json:"number"`
	UserID              ids.ID          `json:"user_id"`
	ReporterUsername    string          `json:"reporter_username"`
	ReporterDisplayName string          `json:"reporter_display_name"`
	Subject             string          `json:"subject"`
	Description         string          `json:"description"`
	Context             json.RawMessage `json:"context"`
	Status              string          `json:"status"`
	StatusAt            *int64          `json:"status_at"`
	StatusBy            *ids.ID         `json:"status_by"`
	Awaiting            string          `json:"awaiting"`
	AttachmentCount     int             `json:"attachment_count"`
	LastActivityAt      int64           `json:"last_activity_at"`
	MemberReadID        ids.ID          `json:"-"`
	CreatedAt           int64           `json:"created_at"`
}

// BugAttachment is one file sent with a report, or with a line of its
// thread, with what its blob says about it.
type BugAttachment struct {
	ReportID  ids.ID `json:"-"`
	MessageID ids.ID `json:"-"`
	BlobID    ids.ID `json:"id"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Size      int64  `json:"size"`
	Width     *int   `json:"width"`
	Height    *int   `json:"height"`
}

// BugMessage is one line of a report's thread.
type BugMessage struct {
	ID        ids.ID `json:"id"`
	ReportID  ids.ID `json:"report_id"`
	AuthorID  ids.ID `json:"author_id"`
	Kind      string `json:"kind"`
	Event     string `json:"event"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
	// Files are the pictures and recordings sent with the line.
	Files []BugAttachment `json:"files,omitempty"`
}

// bugNews is the lines of a report's thread that are news to its reporter:
// what staff wrote, and the status changes they made.
const bugNews = `(m.kind = 'staff' OR (m.kind = 'system' AND m.event = 'status'))`

const bugCols = `id, number, user_id, reporter_username, reporter_display_name, subject, description, context, status, status_at, status_by, awaiting, attachment_count, last_activity_at, member_read_id, created_at, kind`

func scanBugReport(r rowScanner) (*BugReport, error) {
	var b BugReport
	var context string
	var statusAt, statusBy sql.NullInt64
	err := r.Scan(&b.ID, &b.Number, &b.UserID, &b.ReporterUsername, &b.ReporterDisplayName, &b.Subject, &b.Description, &context,
		&b.Status, &statusAt, &statusBy, &b.Awaiting, &b.AttachmentCount, &b.LastActivityAt, &b.MemberReadID, &b.CreatedAt, &b.Kind)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	b.Context = json.RawMessage(context)
	if !json.Valid(b.Context) {
		b.Context = json.RawMessage(`{}`)
	}
	b.StatusAt, b.StatusBy = int64Ptr(statusAt), idPtr(statusBy)
	return &b, nil
}

func (s *Store) queryBugReports(ctx context.Context, q string, args ...any) ([]*BugReport, error) {
	rows, err := s.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*BugReport{}
	for rows.Next() {
		b, err := scanBugReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func bugReportTx(ctx context.Context, tx *db.Tx, id ids.ID) (*BugReport, error) {
	return scanBugReport(tx.QueryRowContext(ctx, `SELECT `+bugCols+` FROM bug_reports WHERE id = ?`, int64(id)))
}

// CreateBugReport numbers and stores a report with its files, in one
// transaction. Each file has to be a report file its reporter uploaded and has
// not sent with a report yet; the report takes a reference on it, so the
// collector leaves it alone from then on. Numbers are handed out under a lock
// so two reports sent at once on Postgres cannot draw the same one.
func (s *Store) CreateBugReport(ctx context.Context, r *BugReport, files []BugAttachment) error {
	if r.ID.IsZero() {
		r.ID = ids.New()
	}
	if r.CreatedAt == 0 {
		r.CreatedAt = nowMs()
	}
	r.LastActivityAt = r.CreatedAt
	if r.Status == "" {
		r.Status = BugOpen
	}
	if !ValidBugKind(r.Kind) {
		r.Kind = BugKindBug
	}
	if r.Awaiting == "" {
		// a new report is waiting for someone on the team to look at it
		r.Awaiting = AwaitStaff
	}
	if len(r.Context) == 0 || !json.Valid(r.Context) {
		r.Context = json.RawMessage(`{}`)
	}
	r.AttachmentCount = len(files)
	return s.Tx(ctx, func(tx *db.Tx) error {
		if err := tx.Serialise(ctx, "bug-report-number"); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(number), 0) + 1 FROM bug_reports`).Scan(&r.Number); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO bug_reports (`+bugCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			int64(r.ID), r.Number, int64(r.UserID), r.ReporterUsername, r.ReporterDisplayName, r.Subject, r.Description, string(r.Context),
			r.Status, nullInt64(r.StatusAt), nullID(r.StatusBy), r.Awaiting, r.AttachmentCount, r.LastActivityAt, int64(r.MemberReadID), r.CreatedAt, r.Kind); err != nil {
			return err
		}
		for i, f := range files {
			// the reference is taken only on a file that is the reporter's own,
			// a report's kind, and not sent with anything yet
			res, err := tx.ExecContext(ctx, `UPDATE blobs SET ref_count = ref_count + 1 WHERE id = ? AND kind = ? AND owner_id = ? AND ref_count = 0`,
				int64(f.BlobID), BugBlobKind, int64(r.UserID))
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return ErrBugAttachment
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO bug_attachments (report_id, blob_id, position, name) VALUES (?,?,?,?)`,
				int64(r.ID), int64(f.BlobID), i, f.Name); err != nil {
				if db.IsUniqueViolation(err) {
					return ErrBugAttachment
				}
				return err
			}
		}
		return nil
	})
}

// BugReportByID loads one report.
func (s *Store) BugReportByID(ctx context.Context, id ids.ID) (*BugReport, error) {
	return scanBugReport(s.db.R.QueryRowContext(ctx, `SELECT `+bugCols+` FROM bug_reports WHERE id = ?`, int64(id)))
}

// MaxBugReportsPerUser bounds how many of one account's reports a read returns.
const MaxBugReportsPerUser = 500

// BugReportsForUser lists an account's reports, newest first.
func (s *Store) BugReportsForUser(ctx context.Context, uid ids.ID) ([]*BugReport, error) {
	return s.queryBugReports(ctx, `SELECT `+bugCols+` FROM bug_reports WHERE user_id = ? ORDER BY id DESC LIMIT ?`, int64(uid), MaxBugReportsPerUser)
}

// BugFilter picks which reports the staff list shows, and in what order.
type BugFilter struct {
	// Kind is bug or suggestion; "" lists both.
	Kind string
	// Status is one state, "active" (open or in progress), or "" for all.
	Status string
	// Waiting keeps the reports waiting on staff: new ones, and ones the
	// reporter wrote in last.
	Waiting bool
	// WithFiles keeps the reports that came with screenshots or a recording.
	WithFiles bool
	// Query matches the subject, the description, the reporter's names as
	// they were sent, or a number ("#12" or "12").
	Query string
	// Sort is activity (most recently active first, the default), newest or
	// oldest.
	Sort string
	// The cursor is the last report of the page before: its last activity
	// and id when sorting by activity, its id otherwise. A page boundary
	// never skips or repeats a report.
	BeforeAt int64
	BeforeID ids.ID
	Limit    int
}

// BugReportList pages reports for the staff.
func (s *Store) BugReportList(ctx context.Context, f BugFilter) ([]*BugReport, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	var where []string
	var args []any
	if ValidBugKind(f.Kind) {
		where = append(where, `kind = ?`)
		args = append(args, f.Kind)
	}
	switch {
	case f.Status == "active":
		where = append(where, `status IN ('open', 'in_progress')`)
	case ValidBugStatus(f.Status):
		where = append(where, `status = ?`)
		args = append(args, f.Status)
	}
	if f.Waiting {
		where = append(where, `awaiting = 'staff'`)
	}
	if f.WithFiles {
		where = append(where, `attachment_count > 0`)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		// cut on a character, not a byte: half a character is text Postgres refuses
		if r := []rune(q); len(r) > maxSearchQuery {
			q = string(r[:maxSearchQuery])
		}
		like := "%" + escapeLike(strings.ToLower(strings.TrimPrefix(q, "@"))) + "%"
		cond := s.db.Dialect.Lower("subject") + ` LIKE ? ESCAPE '\' OR ` + s.db.Dialect.Lower("description") + ` LIKE ? ESCAPE '\' OR ` +
			s.db.Dialect.Lower("reporter_username") + ` LIKE ? ESCAPE '\' OR ` + s.db.Dialect.Lower("reporter_display_name") + ` LIKE ? ESCAPE '\'`
		args = append(args, like, like, like, like)
		if n, err := strconv.ParseInt(strings.TrimPrefix(q, "#"), 10, 64); err == nil && n > 0 {
			cond += ` OR number = ?`
			args = append(args, n)
		}
		where = append(where, `(`+cond+`)`)
	}
	order := `last_activity_at DESC, id DESC`
	switch f.Sort {
	case "newest":
		order = `id DESC`
		if !f.BeforeID.IsZero() {
			where = append(where, `id < ?`)
			args = append(args, int64(f.BeforeID))
		}
	case "oldest":
		order = `id ASC`
		if !f.BeforeID.IsZero() {
			where = append(where, `id > ?`)
			args = append(args, int64(f.BeforeID))
		}
	default:
		if f.BeforeAt > 0 {
			where = append(where, `(last_activity_at < ? OR (last_activity_at = ? AND id < ?))`)
			args = append(args, f.BeforeAt, f.BeforeAt, int64(f.BeforeID))
		}
	}
	q := `SELECT ` + bugCols + ` FROM bug_reports`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	args = append(args, f.Limit)
	return s.queryBugReports(ctx, q+` ORDER BY `+order+` LIMIT ?`, args...)
}

// BugCounts is how many reports each view of the staff list holds.
type BugCounts struct {
	// Waiting is the reports waiting on staff, whatever their status.
	Waiting    int `json:"waiting"`
	Open       int `json:"open"`
	InProgress int `json:"in_progress"`
	Resolved   int `json:"resolved"`
	WontFix    int `json:"wont_fix"`
	Closed     int `json:"closed"`
	All        int `json:"all"`
}

// CountBugReports counts the reports of one kind in each view.
func (s *Store) CountBugReports(ctx context.Context, kind string) (BugCounts, error) {
	var c BugCounts
	err := s.db.R.QueryRowContext(ctx, `SELECT
		coalesce(sum(CASE WHEN awaiting = 'staff' THEN 1 ELSE 0 END), 0),
		coalesce(sum(CASE WHEN status = 'open' THEN 1 ELSE 0 END), 0),
		coalesce(sum(CASE WHEN status = 'in_progress' THEN 1 ELSE 0 END), 0),
		coalesce(sum(CASE WHEN status = 'resolved' THEN 1 ELSE 0 END), 0),
		coalesce(sum(CASE WHEN status = 'wont_fix' THEN 1 ELSE 0 END), 0),
		coalesce(sum(CASE WHEN status = 'closed' THEN 1 ELSE 0 END), 0),
		count(*)
		FROM bug_reports WHERE kind = ?`, kind).Scan(&c.Waiting, &c.Open, &c.InProgress, &c.Resolved, &c.WontFix, &c.Closed, &c.All)
	return c, err
}

// BugAttachments returns the files of each listed report, in the order they were sent.
func (s *Store) BugAttachments(ctx context.Context, reportIDs []ids.ID) (map[ids.ID][]BugAttachment, error) {
	out := map[ids.ID][]BugAttachment{}
	for start := 0; start < len(reportIDs); start += 200 {
		chunk := reportIDs[start:min(start+200, len(reportIDs))]
		rows, err := s.db.R.QueryContext(ctx, `SELECT a.report_id, a.blob_id, a.name, b.mime, b.size, b.width, b.height
			FROM bug_attachments a JOIN blobs b ON b.id = a.blob_id
			WHERE a.report_id IN (`+placeholders(len(chunk))+`) ORDER BY a.report_id, a.position`, idArgs(chunk)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var a BugAttachment
			var w, h sql.NullInt64
			if err := rows.Scan(&a.ReportID, &a.BlobID, &a.Name, &a.Mime, &a.Size, &w, &h); err != nil {
				rows.Close()
				return nil, err
			}
			a.Width, a.Height = intPtr(w), intPtr(h)
			out[a.ReportID] = append(out[a.ReportID], a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// BugFileOwner says whose a report file is, and whether it has been sent: the
// reporter of the report it went with (in the report itself or a line of its
// thread, whoever wrote that), or, before it is sent, whoever uploaded it.
// Anything that is not a report file is not found.
func (s *Store) BugFileOwner(ctx context.Context, blobID ids.ID) (owner ids.ID, sent bool, err error) {
	var reporter sql.NullInt64
	err = s.db.R.QueryRowContext(ctx, `SELECT r.user_id FROM bug_attachments a JOIN bug_reports r ON r.id = a.report_id WHERE a.blob_id = ?
		UNION ALL SELECT r.user_id FROM bug_message_files f JOIN bug_reports r ON r.id = f.report_id WHERE f.blob_id = ?
		LIMIT 1`, int64(blobID), int64(blobID)).Scan(&reporter)
	if err == nil && reporter.Valid {
		return ids.ID(reporter.Int64), true, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	var up sql.NullInt64
	err = s.db.R.QueryRowContext(ctx, `SELECT owner_id FROM blobs WHERE id = ? AND kind = ?`, int64(blobID), BugBlobKind).Scan(&up)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !up.Valid) {
		return 0, false, ErrNotFound
	}
	if err != nil {
		return 0, false, err
	}
	return ids.ID(up.Int64), false, nil
}

// --- the thread ----------------------------------------------------------------

// MaxBugMessages bounds one page of a thread.
const MaxBugMessages = 200

const bugMsgCols = `id, report_id, author_id, kind, event, body, created_at`

func scanBugMessage(r rowScanner) (*BugMessage, error) {
	var m BugMessage
	if err := r.Scan(&m.ID, &m.ReportID, &m.AuthorID, &m.Kind, &m.Event, &m.Body, &m.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (s *Store) queryBugMessages(ctx context.Context, q string, args ...any) ([]*BugMessage, error) {
	rows, err := s.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*BugMessage{}
	for rows.Next() {
		m, err := scanBugMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// BugMessages returns a report's thread in order, the oldest page first, or
// the page after `after`.
func (s *Store) BugMessages(ctx context.Context, reportID, after ids.ID, limit int) ([]*BugMessage, error) {
	if limit <= 0 || limit > MaxBugMessages {
		limit = MaxBugMessages
	}
	out, err := s.queryBugMessages(ctx, `SELECT `+bugMsgCols+` FROM bug_messages WHERE report_id = ? AND id > ? ORDER BY id LIMIT ?`, int64(reportID), int64(after), limit)
	if err != nil {
		return nil, err
	}
	return out, s.withBugFiles(ctx, out)
}

// attachBugFilesTx takes the files a new line names: each must be a report
// file its writer uploaded and has not sent before. Their details come from
// the blobs themselves, never from what the writer said about them.
func attachBugFilesTx(ctx context.Context, tx *db.Tx, m *BugMessage, files []BugAttachment) error {
	if len(files) > MaxCaseFiles {
		return ErrCaseFile
	}
	m.Files = nil
	for i, f := range files {
		res, err := tx.ExecContext(ctx, `UPDATE blobs SET ref_count = ref_count + 1 WHERE id = ? AND kind = ? AND owner_id = ? AND ref_count = 0`,
			int64(f.BlobID), BugBlobKind, int64(m.AuthorID))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrCaseFile
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO bug_message_files (message_id, report_id, blob_id, position, name) VALUES (?,?,?,?,?)`,
			int64(m.ID), int64(m.ReportID), int64(f.BlobID), i, f.Name); err != nil {
			if db.IsUniqueViolation(err) {
				return ErrCaseFile
			}
			return err
		}
		got := BugAttachment{ReportID: m.ReportID, MessageID: m.ID, BlobID: f.BlobID, Name: f.Name}
		var w, h sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT mime, size, width, height FROM blobs WHERE id = ?`, int64(f.BlobID)).Scan(&got.Mime, &got.Size, &w, &h); err != nil {
			return err
		}
		got.Width, got.Height = intPtr(w), intPtr(h)
		m.Files = append(m.Files, got)
	}
	return nil
}

// withBugFiles fills in the files of the lines given, which may come from
// several reports.
func (s *Store) withBugFiles(ctx context.Context, msgs []*BugMessage) error {
	byID := make(map[ids.ID]*BugMessage, len(msgs))
	list := make([]ids.ID, 0, len(msgs))
	for _, m := range msgs {
		if m == nil || m.Kind == ModMsgSystem {
			continue
		}
		byID[m.ID] = m
		list = append(list, m.ID)
	}
	for start := 0; start < len(list); start += 200 {
		chunk := list[start:min(start+200, len(list))]
		rows, err := s.db.R.QueryContext(ctx, `SELECT f.message_id, f.report_id, f.blob_id, f.name, b.mime, b.size, b.width, b.height
			FROM bug_message_files f JOIN blobs b ON b.id = f.blob_id
			WHERE f.message_id IN (`+placeholders(len(chunk))+`) ORDER BY f.message_id, f.position`, idArgs(chunk)...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var f BugAttachment
			var w, h sql.NullInt64
			if err := rows.Scan(&f.MessageID, &f.ReportID, &f.BlobID, &f.Name, &f.Mime, &f.Size, &w, &h); err != nil {
				rows.Close()
				return err
			}
			f.Width, f.Height = intPtr(w), intPtr(h)
			if m := byID[f.MessageID]; m != nil {
				m.Files = append(m.Files, f)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

func insertBugMessageTx(ctx context.Context, tx *db.Tx, m *BugMessage) error {
	if m.ID.IsZero() {
		m.ID = ids.New()
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = nowMs()
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO bug_messages (`+bugMsgCols+`) VALUES (?,?,?,?,?,?,?)`,
		int64(m.ID), int64(m.ReportID), int64(m.AuthorID), m.Kind, m.Event, m.Body, m.CreatedAt)
	return err
}

// AddBugMessage writes one line to a report's thread, with the files it
// names, and moves the report along: whoever wrote is no longer the one being
// waited on. Unlike a moderation case, a report the reporter writes in keeps
// its status (staff decide whether it is fixed), but it waits on staff again,
// so the reply is seen. A finished report (resolved, won't fix or closed) is
// closed to everyone, staff included, until staff set it going again
// (ErrBugClosed), as a closed case is.
func (s *Store) AddBugMessage(ctx context.Context, reportID, author ids.ID, kind, body string, files []BugAttachment) (*BugMessage, *BugReport, error) {
	var msg *BugMessage
	var r *BugReport
	err := s.Tx(ctx, func(tx *db.Tx) error {
		msg, r = nil, nil
		if err := tx.Serialise(ctx, "bug-report", int64(reportID)); err != nil {
			return err
		}
		cur, err := bugReportTx(ctx, tx, reportID)
		if err != nil {
			return err
		}
		if BugFinished(cur.Status) {
			return ErrBugClosed
		}
		now := nowMs()
		awaiting := cur.Awaiting
		switch kind {
		case ModMsgMember:
			awaiting = AwaitStaff
		case ModMsgStaff:
			awaiting = AwaitMember
		}
		msg = &BugMessage{ReportID: reportID, AuthorID: author, Kind: kind, Body: body, CreatedAt: now}
		if err := insertBugMessageTx(ctx, tx, msg); err != nil {
			return err
		}
		if err := attachBugFilesTx(ctx, tx, msg, files); err != nil {
			return err
		}
		readID := int64(cur.MemberReadID)
		if kind == ModMsgMember {
			// what they just wrote, and everything before it, they have seen
			readID = int64(msg.ID)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE bug_reports SET awaiting = ?, last_activity_at = ?, member_read_id = ? WHERE id = ?`,
			awaiting, now, readID, int64(reportID)); err != nil {
			return err
		}
		r, err = bugReportTx(ctx, tx, reportID)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return msg, r, nil
}

// SetBugStatus moves a report to another status for the staff, with a line in
// its thread saying who did it. Asking for the status it already has changes
// nothing. A report that was waiting on staff is not any more: a change of
// status is the team's answer, and the reporter sees it in the thread.
func (s *Store) SetBugStatus(ctx context.Context, reportID, by ids.ID, status string) (*BugReport, *BugMessage, error) {
	if !ValidBugStatus(status) {
		return nil, nil, errors.New("bug status: unknown status " + status)
	}
	var r *BugReport
	var line *BugMessage
	err := s.Tx(ctx, func(tx *db.Tx) error {
		r, line = nil, nil
		if err := tx.Serialise(ctx, "bug-report", int64(reportID)); err != nil {
			return err
		}
		cur, err := bugReportTx(ctx, tx, reportID)
		if err != nil {
			return err
		}
		if cur.Status == status {
			r = cur
			return nil
		}
		now := nowMs()
		if _, err := tx.ExecContext(ctx, `UPDATE bug_reports SET status = ?, status_at = ?, status_by = ?, last_activity_at = ?,
			awaiting = CASE WHEN awaiting = 'staff' THEN 'none' ELSE awaiting END WHERE id = ?`, status, now, int64(by), now, int64(reportID)); err != nil {
			return err
		}
		line = &BugMessage{ReportID: reportID, AuthorID: by, Kind: ModMsgSystem, Event: BugEventStatus, Body: status, CreatedAt: now}
		if err := insertBugMessageTx(ctx, tx, line); err != nil {
			return err
		}
		r, err = bugReportTx(ctx, tx, reportID)
		return err
	})
	return r, line, err
}

// MarkBugRead moves the reporter's read position in their own report forward.
func (s *Store) MarkBugRead(ctx context.Context, reportID, uid, upTo ids.ID) error {
	res, err := s.db.W.ExecContext(ctx, `UPDATE bug_reports SET member_read_id = `+s.db.Dialect.Greatest("member_read_id", "?")+` WHERE id = ? AND user_id = ?`, int64(upTo), int64(reportID), int64(uid))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// BugUnreadByReport counts, per report of the member's, the staff replies and
// status changes they have not read yet. Reports with none are left out.
func (s *Store) BugUnreadByReport(ctx context.Context, uid ids.ID) (map[ids.ID]int, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT m.report_id, count(*) FROM bug_messages m JOIN bug_reports r ON r.id = m.report_id
		WHERE r.user_id = ? AND `+bugNews+` AND m.id > r.member_read_id GROUP BY m.report_id`, int64(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.ID]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[ids.ID(id)] = n
	}
	return out, rows.Err()
}

// LastBugMessages returns the newest line of each listed report's thread, for
// the previews in the staff list.
func (s *Store) LastBugMessages(ctx context.Context, reportIDs []ids.ID) (map[ids.ID]*BugMessage, error) {
	out := map[ids.ID]*BugMessage{}
	for start := 0; start < len(reportIDs); start += 200 {
		chunk := reportIDs[start:min(start+200, len(reportIDs))]
		list, err := s.queryBugMessages(ctx, `SELECT `+bugMsgCols+` FROM bug_messages WHERE id IN (SELECT max(id) FROM bug_messages WHERE report_id IN (`+placeholders(len(chunk))+`) GROUP BY report_id)`, idArgs(chunk)...)
		if err != nil {
			return nil, err
		}
		for _, m := range list {
			out[m.ReportID] = m
		}
	}
	// a preview of a line that is only a picture says so
	list := make([]*BugMessage, 0, len(out))
	for _, m := range out {
		list = append(list, m)
	}
	return out, s.withBugFiles(ctx, list)
}

// BugNewsForUser returns what the team said in the member's reports (their
// replies and status changes), newest first, before `before` (0: now), for
// the Nexa conversation.
func (s *Store) BugNewsForUser(ctx context.Context, uid ids.ID, before int64, limit int) ([]*BugMessage, error) {
	if limit <= 0 || limit > 101 {
		limit = 50
	}
	q := `SELECT m.id, m.report_id, m.author_id, m.kind, m.event, m.body, m.created_at FROM bug_messages m JOIN bug_reports r ON r.id = m.report_id
		WHERE r.user_id = ? AND ` + bugNews
	args := []any{int64(uid)}
	if before > 0 {
		q += ` AND m.created_at < ?`
		args = append(args, before)
	}
	args = append(args, limit)
	out, err := s.queryBugMessages(ctx, q+` ORDER BY m.created_at DESC, m.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	return out, s.withBugFiles(ctx, out)
}

// deleteUserBugReportsTx removes every report an account sent, with its
// thread, and lets go of its files, which the collector then takes. Reports
// are the account's own words and pictures, and go with it like its crash
// reports do.
func deleteUserBugReportsTx(ctx context.Context, tx *db.Tx, uid int64) error {
	if err := releaseBlobRefs(ctx, tx, `SELECT a.blob_id FROM bug_attachments a JOIN bug_reports r ON r.id = a.report_id WHERE r.user_id = ?
		UNION ALL SELECT f.blob_id FROM bug_message_files f JOIN bug_reports r ON r.id = f.report_id WHERE r.user_id = ?`, uid, uid); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM bug_attachments WHERE report_id IN (SELECT id FROM bug_reports WHERE user_id = ?)`,
		`DELETE FROM bug_message_files WHERE report_id IN (SELECT id FROM bug_reports WHERE user_id = ?)`,
		`DELETE FROM mod_reads WHERE kind = 'bug' AND item_id IN (SELECT id FROM bug_reports WHERE user_id = ?)`,
		`DELETE FROM bug_messages WHERE report_id IN (SELECT id FROM bug_reports WHERE user_id = ?)`,
		`DELETE FROM bug_reports WHERE user_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, uid); err != nil {
			return err
		}
	}
	return nil
}
