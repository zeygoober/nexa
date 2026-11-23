-- See the SQLite migration of the same number.
CREATE TABLE quests (
  id BIGINT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  track TEXT NOT NULL,
  period BIGINT NOT NULL,
  slot INTEGER NOT NULL DEFAULT 0,
  kind TEXT NOT NULL,
  target BIGINT NOT NULL,
  progress BIGINT NOT NULL DEFAULT 0,
  reward BIGINT NOT NULL,
  started_at BIGINT NOT NULL,
  done_at BIGINT NOT NULL DEFAULT 0,
  paid BIGINT NOT NULL DEFAULT 0,
  UNIQUE (user_id, track, period, slot)
);
CREATE INDEX quests_open ON quests(user_id, done_at);

CREATE TABLE quest_seen (
  user_id BIGINT NOT NULL,
  scope TEXT NOT NULL,
  subject BIGINT NOT NULL,
  PRIMARY KEY (user_id, scope, subject)
);

CREATE TABLE quest_players (
  user_id BIGINT PRIMARY KEY,
  trial_step BIGINT NOT NULL DEFAULT 1,
  reroll_day BIGINT NOT NULL DEFAULT 0,
  skip_day BIGINT NOT NULL DEFAULT 0
);

INSERT INTO quest_seen (user_id, scope, subject) SELECT user_id, 'ever:friend', target_id FROM relationships WHERE type = 'friend';
INSERT INTO quest_seen (user_id, scope, subject) SELECT user_id, 'ever:join', server_id FROM server_members;
