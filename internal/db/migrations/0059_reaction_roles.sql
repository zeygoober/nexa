-- A reaction on a message that gives the person who adds it a role, and takes
-- it back when they take the reaction off. Only a message's server's managers
-- set these (api/reaction_roles.go).
CREATE TABLE reaction_roles (
  message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  emoji TEXT NOT NULL,
  server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (message_id, emoji)
);
CREATE INDEX reaction_roles_role ON reaction_roles(role_id);
