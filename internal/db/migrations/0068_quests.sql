-- Quests: three daily quests a person can finish for Gems (one easy, one
-- medium, one hard, new each UTC day, plus a bonus for all three) and the
-- Infinite Trial, one quest at a time, each step harder than the last.

-- One row per quest given. track is 'daily', 'bonus' or 'trial'; period is the
-- UTC day (days since 1970) for daily and bonus, the step for trial; slot is
-- 1 easy, 2 medium or 3 hard for daily, 0 otherwise. progress counts only what
-- happened from started_at. paid is what finishing paid (0 while earning was
-- off), held when the wallet had a check or report open.
CREATE TABLE quests (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  track TEXT NOT NULL,
  period INTEGER NOT NULL,
  slot INTEGER NOT NULL DEFAULT 0,
  kind TEXT NOT NULL,
  target INTEGER NOT NULL,
  progress INTEGER NOT NULL DEFAULT 0,
  reward INTEGER NOT NULL,
  started_at INTEGER NOT NULL,
  done_at INTEGER NOT NULL DEFAULT 0,
  paid INTEGER NOT NULL DEFAULT 0,
  UNIQUE (user_id, track, period, slot)
);
CREATE INDEX quests_open ON quests(user_id, done_at);

-- What a quest counted once: scope 'q:<quest id>' with the server, person or
-- report it counted, so a quest about different ones counts each once; and
-- scope 'ever:friend' or 'ever:join' with every person befriended and server
-- joined, so only new ones count. Deleted with the account, and a person's
-- 'ever:friend' rows about someone go when that someone's account does.
CREATE TABLE quest_seen (
  user_id INTEGER NOT NULL,
  scope TEXT NOT NULL,
  subject INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, subject)
);

-- One row per person with quests: the Trial step they are on (it never goes
-- back), and the UTC day they last swapped a daily quest or skipped a Trial
-- quest (once a day each).
CREATE TABLE quest_players (
  user_id INTEGER PRIMARY KEY,
  trial_step INTEGER NOT NULL DEFAULT 1,
  reroll_day INTEGER NOT NULL DEFAULT 0,
  skip_day INTEGER NOT NULL DEFAULT 0
);

-- friends and servers people have now were not new to them (both sources are unique on the key)
INSERT INTO quest_seen (user_id, scope, subject) SELECT user_id, 'ever:friend', target_id FROM relationships WHERE type = 'friend';
INSERT INTO quest_seen (user_id, scope, subject) SELECT user_id, 'ever:join', server_id FROM server_members;
