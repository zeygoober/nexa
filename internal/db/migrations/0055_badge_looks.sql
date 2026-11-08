-- Badges staff make get a description (their tooltip), a second colour for a
-- gradient (-1 for none), a white or dark glyph, a shape and, instead of the
-- icon, an uploaded picture (a public blob of kind badge, 0 for none).
ALTER TABLE badge_defs ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE badge_defs ADD COLUMN color2 INTEGER NOT NULL DEFAULT -1;
ALTER TABLE badge_defs ADD COLUMN glyph TEXT NOT NULL DEFAULT 'light';
ALTER TABLE badge_defs ADD COLUMN shape TEXT NOT NULL DEFAULT 'circle';
ALTER TABLE badge_defs ADD COLUMN image_id INTEGER NOT NULL DEFAULT 0;
-- Staff overriding an earned badge: level 0 takes it away whatever the account
-- earned, a level above 0 gives it at that tier. It holds until cleared.
CREATE TABLE badge_overrides (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  badge TEXT NOT NULL,
  level INTEGER NOT NULL,
  set_by INTEGER NOT NULL,
  set_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, badge)
);
CREATE INDEX badge_overrides_badge ON badge_overrides (badge);
