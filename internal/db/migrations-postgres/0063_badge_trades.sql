-- See the SQLite migration of the same number.
CREATE TABLE badge_holdings (
  user_id BIGINT NOT NULL,
  badge TEXT NOT NULL,
  copies INTEGER NOT NULL DEFAULT 0,
  spent INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, badge)
);

CREATE TABLE badge_trades (
  id BIGINT PRIMARY KEY,
  from_id BIGINT NOT NULL,
  to_id BIGINT NOT NULL,
  from_offer TEXT NOT NULL DEFAULT '[]',
  to_offer TEXT NOT NULL DEFAULT '[]',
  from_ready INTEGER NOT NULL DEFAULT 0,
  to_ready INTEGER NOT NULL DEFAULT 0,
  version INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  channel_id BIGINT NOT NULL,
  message_id BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  resolved_at BIGINT
);
CREATE INDEX badge_trades_from ON badge_trades(from_id, status);
CREATE INDEX badge_trades_to ON badge_trades(to_id, status);
CREATE INDEX badge_trades_message ON badge_trades(message_id);
