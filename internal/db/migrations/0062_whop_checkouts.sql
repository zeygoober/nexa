-- The checkouts Nexa opened at Whop, by Whop's id (ch_...), with whom each was
-- for and what it buys: a payment names its checkout, and this is what says
-- which account a payment is for. Swept a month after.
CREATE TABLE whop_checkouts (
  id TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL,
  kind TEXT NOT NULL,   -- plan, gift or pwyw
  meta TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL
);
CREATE INDEX whop_checkouts_user ON whop_checkouts(user_id);
