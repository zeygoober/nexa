-- A Nexa account's Last.fm username, for showing what the person is listening
-- to as their activity. Nothing but the public username is kept; what they
-- listen to is only held in memory while it plays.
CREATE TABLE lastfm_links (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  username TEXT NOT NULL,
  show_activity INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);
