-- Existing history has no owner recipient. Its keys remain with the participants.
ALTER TABLE messages ADD COLUMN owner_recipient BIGINT;
CREATE INDEX messages_owner_recipient ON messages(owner_recipient, id) WHERE owner_recipient IS NOT NULL;

-- A historical request is immutable. Status and the two independent decisions
-- may change. Submitted boxes expire together or are removed on withdrawal.
CREATE TABLE owner_history_reviews (
  id BIGINT PRIMARY KEY,
  channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
  owner_id BIGINT NOT NULL, first_id BIGINT NOT NULL, second_id BIGINT NOT NULL,
  reason TEXT NOT NULL, expires_at BIGINT NOT NULL, manifest TEXT NOT NULL,
  include_files INTEGER NOT NULL,
  first_approved INTEGER NOT NULL DEFAULT 0, second_approved INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending', boxes TEXT NOT NULL DEFAULT ''
);
CREATE INDEX owner_history_first ON owner_history_reviews(first_id, id);
CREATE INDEX owner_history_second ON owner_history_reviews(second_id, id);
CREATE INDEX owner_history_owner ON owner_history_reviews(owner_id, id);
CREATE TABLE owner_history_files (
  review_id BIGINT NOT NULL REFERENCES owner_history_reviews(id) ON DELETE CASCADE,
  blob_id BIGINT NOT NULL REFERENCES blobs(id) ON DELETE CASCADE,
  PRIMARY KEY(review_id,blob_id)
);
CREATE INDEX owner_history_blob ON owner_history_files(blob_id);
