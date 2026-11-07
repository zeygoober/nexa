-- See the SQLite migration of the same number.
ALTER TABLE bug_reports ADD COLUMN kind TEXT NOT NULL DEFAULT 'bug';
CREATE INDEX idx_bug_reports_kind ON bug_reports (kind, last_activity_at DESC, id DESC);
