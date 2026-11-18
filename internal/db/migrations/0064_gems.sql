-- Gems: earned by sending messages (one a minute at most, times the profile's
-- team), spent on capsules that give collectibles, fused five into one of the
-- next material, and traded.

-- A collectible is a staff-made badge with a rarity: common, rare, epic or
-- huge. '' is an ordinary staff-made badge. Collectibles come only from
-- capsules, fusing and trades, and are held only as badge_holdings copies
-- under event:<id>-<material>, material 1 (bronze) to 5 (prism).
ALTER TABLE badge_defs ADD COLUMN rarity TEXT NOT NULL DEFAULT '';
-- art is a collectible's drawing (store.CollectibleArt: col-<art>-<material>.svg
-- in the apps), picked by staff; '' for every other badge.
ALTER TABLE badge_defs ADD COLUMN art TEXT NOT NULL DEFAULT '';

-- One row per person who has earned. carry is the hundredths a multiplier
-- left over. held is what was earned while a farming check or report is open,
-- given or taken when it closes. state: '' fine, 'check' asked to show they
-- are there (a captcha), 'report' a staff report is open, 'off' earning is off
-- until off_until.
CREATE TABLE gem_wallets (
  user_id INTEGER PRIMARY KEY,
  gems INTEGER NOT NULL DEFAULT 0,
  carry INTEGER NOT NULL DEFAULT 0,
  held INTEGER NOT NULL DEFAULT 0,
  earned INTEGER NOT NULL DEFAULT 0,
  last_earn INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL DEFAULT '',
  state_at INTEGER NOT NULL DEFAULT 0,
  off_until INTEGER NOT NULL DEFAULT 0,
  checked_at INTEGER NOT NULL DEFAULT 0,
  report_id INTEGER NOT NULL DEFAULT 0
);

-- What Gems were spent, traded or taken, and why. Earning writes no row.
CREATE TABLE gem_ledger (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  delta INTEGER NOT NULL,
  kind TEXT NOT NULL,             -- capsule, trade, held_released, held_taken
  ref TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX gem_ledger_user ON gem_ledger(user_id, id);

-- A day of earning's timing, kept 14 days, for the farming check: how many
-- earns, how many came right after the cooldown, how many where nobody else
-- had spoken for half an hour, which UTC hours had one, and which sizes of
-- message (in 8 byte steps) earned. No message, channel or text is kept.
CREATE TABLE gem_watch (
  user_id INTEGER NOT NULL,
  day INTEGER NOT NULL,           -- days since 1970, UTC
  earns INTEGER NOT NULL DEFAULT 0,
  quick INTEGER NOT NULL DEFAULT 0,
  alone INTEGER NOT NULL DEFAULT 0,
  hours INTEGER NOT NULL DEFAULT 0,
  sizes INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, day)
);
