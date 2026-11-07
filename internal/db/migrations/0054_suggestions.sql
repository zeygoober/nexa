-- Suggestions: ideas members send the Nexa team, kept beside bug reports and
-- worked the same way (a thread with the team, the same statuses), so one
-- table holds both. kind says which a row is; every report before this is a bug.
-- Numbers stay one series across both, since number is unique on its own.
ALTER TABLE bug_reports ADD COLUMN kind TEXT NOT NULL DEFAULT 'bug';
CREATE INDEX idx_bug_reports_kind ON bug_reports (kind, last_activity_at DESC, id DESC);
