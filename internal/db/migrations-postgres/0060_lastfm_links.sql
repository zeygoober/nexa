-- See the SQLite migration of the same number.
CREATE TABLE lastfm_links (
  user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  username TEXT NOT NULL,
  show_activity INTEGER NOT NULL DEFAULT 1,
  created_at BIGINT NOT NULL
);
