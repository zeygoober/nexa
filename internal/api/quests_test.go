package api

import (
	"context"
	"testing"

	"evilion.xyz/chat/internal/store"
)

// Quests about other people count only new people who are not one person's
// second account, and a report staff resolve twice counts once.
func TestQuestsCountOnlyNewPeopleAndReportsOnce(t *testing.T) {
	srv, ts := newTestServer(t)
	aliceTok, _ := register(t, ts, "alice") // the first account, staff
	bobTok, bobID := register(t, ts, "bob")
	carolTok, carolID := register(t, ts, "carol")
	daveTok, _ := register(t, ts, "dave")
	ctx := context.Background()
	w := srv.Store.DB().W

	if res, out := do(t, ts, "GET", "/api/v1/quests", bobTok, nil); res.StatusCode != 200 {
		t.Fatalf("board: %d %v", res.StatusCode, out)
	}
	// bob's quests: five friends on the Trial, two bug reports on the hard daily
	if _, err := w.Exec(`UPDATE quests SET kind = ? WHERE user_id = ? AND track != 'bonus'`, store.QuestIdeas, modID(bobID)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`UPDATE quests SET kind = ?, target = 5 WHERE user_id = ? AND track = 'trial'`, store.QuestFriends, modID(bobID)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`UPDATE quests SET kind = ?, target = 2 WHERE user_id = ? AND track = 'daily' AND slot = 3`, store.QuestBugs, modID(bobID)); err != nil {
		t.Fatal(err)
	}
	progress := func(track string) float64 {
		t.Helper()
		_, b := do(t, ts, "GET", "/api/v1/quests", bobTok, nil)
		if track == "trial" {
			return b["trial"].(map[string]any)["progress"].(float64)
		}
		return b["daily"].([]any)[2].(map[string]any)["progress"].(float64)
	}

	// the test server sees everyone at one address, so carol is bob's own
	befriend(t, ts, bobTok, carolTok, "carol", bobID)
	if p := progress("trial"); p != 0 {
		t.Fatalf("a linked friend counted: %v", p)
	}
	// and stays not new once bob moves away
	if _, err := w.Exec(`DELETE FROM user_ips WHERE user_id = ?`, modID(bobID)); err != nil {
		t.Fatal(err)
	}
	_ = srv.Store.RecordUserIP(ctx, modID(bobID), "192.0.2.77", 1)
	do(t, ts, "DELETE", "/api/v1/users/@me/relationships/"+carolID, bobTok, nil)
	befriend(t, ts, bobTok, carolTok, "carol", bobID)
	befriend(t, ts, bobTok, daveTok, "dave", bobID)
	if p := progress("trial"); p != 1 {
		t.Fatalf("friends counted: %v, want only dave", p)
	}

	id := mustBug(t, ts, bobTok, bugBody("Calls drop"))["id"].(string)
	for _, st := range []string{"resolved", "in_progress", "resolved"} {
		if res, out := do(t, ts, "POST", "/api/v1/moderation/bugs/"+id+"/status", aliceTok, map[string]any{"status": st}); res.StatusCode != 200 {
			t.Fatalf("%s: %d %v", st, res.StatusCode, out)
		}
	}
	if p := progress("daily"); p != 1 {
		t.Fatalf("a report resolved twice counted %v times", p)
	}
}
