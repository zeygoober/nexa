-- See the SQLite migration of the same number.
CREATE TABLE reaction_roles (
  message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  emoji TEXT NOT NULL,
  server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (message_id, emoji)
);
CREATE INDEX reaction_roles_role ON reaction_roles(role_id);
