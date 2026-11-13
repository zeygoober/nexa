-- See the SQLite migration of the same number.
CREATE TABLE whop_checkouts (
  id TEXT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  kind TEXT NOT NULL,
  meta TEXT NOT NULL DEFAULT '{}',
  created_at BIGINT NOT NULL
);
CREATE INDEX whop_checkouts_user ON whop_checkouts(user_id);
