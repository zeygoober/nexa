-- What two badges count that nothing else keeps. Regular: the consecutive UTC
-- days an account has had Nexa open (last_day is days since 1970 in UTC, streak
-- the run ending there, best_streak the longest there has been). Open Mic: the
-- seconds it has spent in a call with someone else. One row an account, no
-- dates of use or history, and it goes with the account.
CREATE TABLE badge_progress (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  last_day INTEGER NOT NULL DEFAULT 0,
  streak INTEGER NOT NULL DEFAULT 0,
  best_streak INTEGER NOT NULL DEFAULT 0,
  call_seconds INTEGER NOT NULL DEFAULT 0
);
