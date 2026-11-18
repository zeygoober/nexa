-- See the SQLite migration of the same number.
CREATE TABLE server_templates (
  code TEXT PRIMARY KEY,
  server_id BIGINT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  spec TEXT NOT NULL,
  usage_count INTEGER NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  synced_at BIGINT NOT NULL
);
