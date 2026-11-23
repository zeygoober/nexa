-- The built-in Secret badges (Hopeful, Curious, Nosy, Lucky) are gone: how each
-- was found could be read in the public source. Every copy goes, and a trade
-- still offering one ends, as either person ending it would.
UPDATE badge_trades SET status = 'cancelled', resolved_at = CAST(strftime('%s','now') AS INTEGER) * 1000, from_ready = 0, to_ready = 0
WHERE status = 'pending' AND (from_offer LIKE '%"key":"hopeful"%' OR from_offer LIKE '%"key":"curious"%' OR from_offer LIKE '%"key":"nosy"%' OR from_offer LIKE '%"key":"lucky"%'
  OR to_offer LIKE '%"key":"hopeful"%' OR to_offer LIKE '%"key":"curious"%' OR to_offer LIKE '%"key":"nosy"%' OR to_offer LIKE '%"key":"lucky"%');
DELETE FROM user_badges WHERE badge IN ('hopeful', 'curious', 'nosy', 'lucky');
DELETE FROM badge_holdings WHERE badge IN ('hopeful', 'curious', 'nosy', 'lucky');
DELETE FROM badge_overrides WHERE badge IN ('hopeful', 'curious', 'nosy', 'lucky');
-- a badge staff made Secret stays on its holders' profiles and leaves the index; staff can list it as Exclusive
UPDATE badge_defs SET index_kind = 'hidden' WHERE index_kind = 'special';
