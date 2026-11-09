-- Apps their developers chose to show on Discovery; staff can take one off it.
ALTER TABLE applications ADD COLUMN discoverable INTEGER NOT NULL DEFAULT 0;
ALTER TABLE applications ADD COLUMN discovery_hidden INTEGER NOT NULL DEFAULT 0;
