package db

import (
	"context"
	"path/filepath"
	"testing"
)

// Accounts on the old default, where anyone could message them, move to
// friends and server members with migration 0018. A stricter choice stays.
func TestMigrationMovesTheOldDMDefault(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// user 4 takes the column's default, which is still 'everyone'
	_, err = d.W.Exec(`
	INSERT INTO users(id,username,display_name,auth_hash,kdf_salt,kdf_params,public_key,enc_private_key,dm_privacy,created_at,updated_at) VALUES
	  (1,'a','A','h',x'00','{}',x'00',x'00','everyone',0,0),
	  (2,'b','B','h',x'00','{}',x'00',x'00','friends',0,0),
	  (3,'c','C','h',x'00','{}',x'00',x'00','nobody',0,0);
	INSERT INTO users(id,username,display_name,auth_hash,kdf_salt,kdf_params,public_key,enc_private_key,created_at,updated_at) VALUES
	  (4,'d','D','h',x'00','{}',x'00',x'00',0,0);
	-- back to how version 17 looked: undo what later migrations add
	DROP INDEX idx_bug_reports_kind;
	ALTER TABLE bug_reports DROP COLUMN kind;
	DROP TABLE webhooks;
	ALTER TABLE messages DROP COLUMN webhook_id;
	ALTER TABLE messages DROP COLUMN plain;
	ALTER TABLE polls DROP COLUMN ended_at;
	ALTER TABLE relationships DROP COLUMN request_note;
	ALTER TABLE users DROP COLUMN group_link_only;
	ALTER TABLE dm_participants DROP COLUMN add_notice_by;
	DROP TABLE discord_links;
	DROP TABLE note_keys;
	DROP TABLE note_version_files;
	DROP TABLE note_versions;
	DROP TABLE note_files;
	DROP TABLE note_links;
	DROP TABLE note_invites;
	DROP TABLE note_members;
	DROP TABLE notes;
	ALTER TABLE blobs DROP COLUMN blocked;
	ALTER TABLE sticker_packs DROP COLUMN discover_blocked;
	DROP TABLE blocked_images;
	ALTER TABLE users DROP COLUMN typing_style;
	DROP INDEX users_tag_server;
	ALTER TABLE users DROP COLUMN tag_server_id;
	ALTER TABLE users DROP COLUMN tag_text;
	ALTER TABLE users DROP COLUMN tag_badge;
	ALTER TABLE users DROP COLUMN tag_color;
	ALTER TABLE servers DROP COLUMN tag_text;
	ALTER TABLE servers DROP COLUMN tag_badge;
	ALTER TABLE servers DROP COLUMN tag_color;
	ALTER TABLE servers DROP COLUMN tag_roles;
	ALTER TABLE servers DROP COLUMN tag_invite;
	ALTER TABLE users DROP COLUMN widgets;
	` + undoBots + undoCommunities + `
	PRAGMA user_version = 17;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]string{1: "friends_servers", 2: "friends", 3: "nobody", 4: "friends_servers"} {
		var got string
		if err := d.R.QueryRow(`SELECT dm_privacy FROM users WHERE id = ?`, id).Scan(&got); err != nil || got != want {
			t.Errorf("user %d: dm_privacy %q %v, want %q", id, got, err, want)
		}
	}
}
