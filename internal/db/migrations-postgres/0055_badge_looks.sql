-- See the SQLite migration of the same number.
ALTER TABLE badge_defs ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE badge_defs ADD COLUMN color2 BIGINT NOT NULL DEFAULT -1;
ALTER TABLE badge_defs ADD COLUMN glyph TEXT NOT NULL DEFAULT 'light';
ALTER TABLE badge_defs ADD COLUMN shape TEXT NOT NULL DEFAULT 'circle';
ALTER TABLE badge_defs ADD COLUMN image_id BIGINT NOT NULL DEFAULT 0;
CREATE TABLE badge_overrides (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  badge TEXT NOT NULL,
  level INTEGER NOT NULL,
  set_by BIGINT NOT NULL,
  set_at BIGINT NOT NULL,
  PRIMARY KEY (user_id, badge)
);
CREATE INDEX badge_overrides_badge ON badge_overrides (badge);
