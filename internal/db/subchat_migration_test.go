package db

import (
	"context"
	"path/filepath"
	"testing"
)

// Groups that lost members, or were deleted, before subchats followed their
// parent's membership are tidied up by migration 0007.
func TestMigrationTidiesSubchatMembers(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = d.W.Exec(`
	INSERT INTO users(id,username,display_name,auth_hash,kdf_salt,kdf_params,public_key,enc_private_key,created_at,updated_at) VALUES
	  (1,'a','A','h',x'00','{}',x'00',x'00',0,0),
	  (2,'b','B','h',x'00','{}',x'00',x'00',0,0),
	  (3,'c','C','h',x'00','{}',x'00',x'00',0,0);
	INSERT INTO channels(id,type,created_at,updated_at) VALUES (10,'group_dm',0,0);
	INSERT INTO channels(id,type,parent_id,created_at,updated_at) VALUES (11,'subchat',10,0,0);
	INSERT INTO channels(id,type,parent_id,created_at,updated_at) VALUES (12,'subchat',NULL,0,0);
	INSERT INTO dm_participants(channel_id,user_id,added_at) VALUES (10,1,0),(10,2,0),(11,1,0),(11,2,0),(11,3,0),(12,1,0);
	INSERT INTO blobs(id,sha256,size,mime,kind,encrypted,ref_count,created_at) VALUES (5,'s',1,'x','attachment',1,1,0);
	INSERT INTO messages(id,channel_id,author_id,created_at) VALUES (20,12,1,0);
	INSERT INTO attachments(id,message_id,channel_id,uploader_id,blob_id,size,created_at) VALUES (30,20,12,1,5,1,0);
	INSERT INTO key_epochs(conv_id,epoch,created_by,created_at) VALUES (12,0,1,0);
	-- back to how version 6 looked: undo what later migrations add
	DROP INDEX idx_bug_reports_kind;
	ALTER TABLE bug_reports DROP COLUMN kind;
	ALTER TABLE uploads DROP COLUMN blob_id;
	DROP TABLE realtime_usage;
	DROP TABLE guests;
	DROP TABLE guest_links;
	ALTER TABLE channels DROP COLUMN lock_salt;
	ALTER TABLE channels DROP COLUMN lock_params;
	ALTER TABLE channels DROP COLUMN lock_hash;
	DROP TABLE channel_unlocks;
	ALTER TABLE users DROP COLUMN signup_invite;
	ALTER TABLE channels DROP COLUMN glyph;
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
	PRAGMA user_version = 6;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	d.R.QueryRow(`SELECT count(*) FROM dm_participants WHERE channel_id = 11`).Scan(&n)
	if n != 2 {
		t.Errorf("carol left the group but is still in its subchat: %d members", n)
	}
	d.R.QueryRow(`SELECT count(*) FROM key_flags WHERE conv_id = 11 AND needs_rotation = 1`).Scan(&n)
	if n != 1 {
		t.Error("the subchat she left should wait for a new key")
	}
	d.R.QueryRow(`SELECT count(*) FROM channels WHERE id = 12`).Scan(&n)
	if n != 0 {
		t.Error("a subchat without a conversation should be gone")
	}
	d.R.QueryRow(`SELECT count(*) FROM key_epochs WHERE conv_id = 12`).Scan(&n)
	if n != 0 {
		t.Error("its keys should be gone")
	}
	d.R.QueryRow(`SELECT ref_count FROM blobs WHERE id = 5`).Scan(&n)
	if n != 0 {
		t.Errorf("its files should be released: ref_count %d", n)
	}
}
