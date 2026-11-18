-- Nexa server templates: a link that makes new servers with one server's
-- channels, roles and permissions as they were when it was made or last synced.
-- One per server. spec is the snapshot as JSON (store.TemplateSpec); it holds
-- names, topics, colours and permission bits, never messages, members or files.
CREATE TABLE server_templates (
  code TEXT PRIMARY KEY,
  server_id INTEGER NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  spec TEXT NOT NULL,
  usage_count INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  synced_at INTEGER NOT NULL
);
