-- Badge trading. Earned badges are worked out, never stored, so what trading
-- changes is kept beside them: copies a person gained through trades, and
-- whether they traded away the one copy they earned (which is never earned
-- again). A person holds copies + 1 for an earned badge not yet spent.
CREATE TABLE badge_holdings (
  user_id INTEGER NOT NULL,
  badge TEXT NOT NULL,            -- the Badge Index's key: bug-3, og, event:123
  copies INTEGER NOT NULL DEFAULT 0,
  spent INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, badge)
);

-- A trade between two people, which both edit: each side's offer is a JSON list
-- of {key, qty} of their own badges. Any change to an offer clears both Ready
-- flags and counts up version, so Ready only ever agrees to the offers as the
-- person saw them. It stays pending until both are ready (and then it happens),
-- or either ends it.
CREATE TABLE badge_trades (
  id INTEGER PRIMARY KEY,
  from_id INTEGER NOT NULL,
  to_id INTEGER NOT NULL,
  from_offer TEXT NOT NULL DEFAULT '[]',
  to_offer TEXT NOT NULL DEFAULT '[]',
  from_ready INTEGER NOT NULL DEFAULT 0,
  to_ready INTEGER NOT NULL DEFAULT 0,
  version INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',   -- pending, accepted, declined, cancelled
  channel_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  resolved_at INTEGER
);
CREATE INDEX badge_trades_from ON badge_trades(from_id, status);
CREATE INDEX badge_trades_to ON badge_trades(to_id, status);
CREATE INDEX badge_trades_message ON badge_trades(message_id);
