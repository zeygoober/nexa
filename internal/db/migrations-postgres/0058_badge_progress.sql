-- See the SQLite migration of the same number.
CREATE TABLE badge_progress (
  user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  last_day BIGINT NOT NULL DEFAULT 0,
  streak INTEGER NOT NULL DEFAULT 0,
  best_streak INTEGER NOT NULL DEFAULT 0,
  call_seconds BIGINT NOT NULL DEFAULT 0
);
