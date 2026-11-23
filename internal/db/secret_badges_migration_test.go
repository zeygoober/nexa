package db

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
)

// Migration 0066 takes the built-in Secret badges from everyone and ends a
// pending trade offering one, leaving every other badge, holding and trade as
// it was; a badge staff made Secret leaves the index but stays held.
func TestMigrationTakesSecretBadgesAway(t *testing.T) {
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
	  (1,'a','A','h',x'00','{}',x'00',x'00',0,0), (2,'b','B','h',x'00','{}',x'00',x'00',0,0);
	INSERT INTO user_badges(user_id,badge,granted_by,granted_at) VALUES (1,'hopeful',1,0),(1,'nosy',1,0),(1,'contributor',2,0),(2,'event:70',1,0);
	INSERT INTO badge_holdings(user_id,badge,copies,spent) VALUES (1,'lucky',2,0),(1,'og',1,1),(2,'curious',1,0);
	INSERT INTO badge_trades(id,from_id,to_id,from_offer,to_offer,status,channel_id,message_id,created_at) VALUES
	  (50,1,2,'[{"key":"og","qty":1}]','[{"key":"curious","qty":1}]','pending',9,9,0),
	  (51,1,2,'[{"key":"og","qty":1}]','[{"key":"gems","qty":40}]','pending',9,9,0),
	  (52,1,2,'[{"key":"nosy","qty":1}]','[]','accepted',9,9,0);
	INSERT INTO badge_defs(id,name,icon,color,created_by,created_at,index_kind) VALUES (70,'Hidden Door','ghost',0,1,0,'special'),(71,'Party','star',0,1,0,'exclusive');
	`)
	if err != nil {
		t.Fatal(err)
	}
	found, _ := fs.Glob(migrations, "migrations/0066_*.sql")
	if len(found) != 1 {
		t.Fatalf("migration 0066: %v", found)
	}
	sql, _ := migrations.ReadFile(found[0])
	if _, err := d.W.Exec(string(sql)); err != nil {
		t.Fatal(err)
	}
	count := func(q string) int {
		var n int
		if err := d.W.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM user_badges WHERE badge IN ('hopeful','nosy','curious','lucky')`); n != 0 {
		t.Errorf("%d Secret badges left", n)
	}
	if count(`SELECT count(*) FROM user_badges`) != 2 || count(`SELECT count(*) FROM badge_holdings`) != 1 {
		t.Error("other badges went too")
	}
	status := func(id int) string {
		var s string
		if err := d.W.QueryRow(`SELECT status FROM badge_trades WHERE id = ?`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if status(50) != "cancelled" || status(51) != "pending" || status(52) != "accepted" {
		t.Errorf("trades: %s %s %s", status(50), status(51), status(52))
	}
	if count(`SELECT count(*) FROM badge_defs WHERE id = 70 AND index_kind = 'hidden'`) != 1 || count(`SELECT count(*) FROM badge_defs WHERE id = 71 AND index_kind = 'exclusive'`) != 1 {
		t.Error("staff badges' places")
	}
}
