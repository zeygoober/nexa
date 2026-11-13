-- Where a badge staff made sits in the Badge Index: exclusive, special or hidden.
ALTER TABLE badge_defs ADD COLUMN index_kind TEXT NOT NULL DEFAULT 'exclusive';
