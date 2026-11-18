package api

import (
	"context"
	"testing"
)

// Gems move in a trade only between accounts the alt check does not link, and
// only what someone holds; a message earns at most one a minute.
func TestGemsTradeOnlyBetweenUnlinkedAccounts(t *testing.T) {
	srv, ts := newTestServer(t)
	register(t, ts, "owner")
	aliceTok, aliceID := register(t, ts, "alice")
	bobTok, bobID := register(t, ts, "bob")
	if _, err := srv.Store.DB().W.Exec(`INSERT INTO gem_wallets (user_id, gems) VALUES (?, 1000)`, modID(aliceID)); err != nil {
		t.Fatal(err)
	}
	x, code, body := openTrade(t, ts, aliceTok, bobID)
	if code != 201 {
		t.Fatalf("open: %d %v", code, body)
	}
	// the test server sees everyone at one address, so they are linked
	if code, body := x.offer(aliceTok, tradeLines("gems", 600)); code != 409 || errCode(body) != "trade_gems_linked" {
		t.Fatalf("linked accounts traded Gems: %d %v", code, body)
	}
	if _, err := srv.Store.DB().W.Exec(`DELETE FROM user_ips WHERE user_id = ?`, modID(bobID)); err != nil {
		t.Fatal(err)
	}
	_ = srv.Store.RecordUserIP(context.Background(), modID(bobID), "192.0.2.77", 1)
	if code, body := x.offer(aliceTok, tradeLines("gems", 1001)); code != 409 || errCode(body) != "trade_you_short" {
		t.Fatalf("offered Gems not held: %d %v", code, body)
	}
	if code, body := x.offer(aliceTok, tradeLines("gems", 600)); code != 200 {
		t.Fatalf("offer: %d %v", code, body)
	}
	_, now := x.get(bobTok)
	v := now["version"].(float64)
	x.ready(aliceTok, v)
	if code, body := x.ready(bobTok, v); code != 200 || body["status"] != "accepted" {
		t.Fatalf("accept: %d %v", code, body)
	}
	gems := func(tok string) float64 {
		t.Helper()
		_, b := do(t, ts, "GET", "/api/v1/gems", tok, nil)
		return b["wallet"].(map[string]any)["gems"].(float64)
	}
	if a, b := gems(aliceTok), gems(bobTok); a != 400 || b != 600 {
		t.Fatalf("after the trade: alice %v, bob %v", a, b)
	}
	dm := dmBetween(t, ts, aliceTok, bobID)
	createEpoch(t, ts, aliceTok, dm)
	for _, text := range []string{"one", "two", "three"} {
		if res, body := sendMsg(t, ts, aliceTok, dm, dm, 0, text, "", nil); res.StatusCode != 201 {
			t.Fatalf("send: %d %v", res.StatusCode, body)
		}
	}
	if a := gems(aliceTok); a != 401 {
		t.Fatalf("three messages in a minute earned %v", a-400)
	}
}

// A Gem Farming report holds the Gems until staff decide; acting on it takes
// them, stops earning and gives the member a case.
func TestGemFarmingReportActedOn(t *testing.T) {
	srv, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "owner")
	aliceTok, aliceID := register(t, ts, "alice")
	if _, err := srv.Store.DB().W.Exec(`INSERT INTO gem_wallets (user_id, gems, held, state, state_at) VALUES (?, 10, 70, 'report', 1)`, modID(aliceID)); err != nil {
		t.Fatal(err)
	}
	u, err := srv.Store.UserByID(context.Background(), modID(aliceID))
	if err != nil {
		t.Fatal(err)
	}
	srv.fileGemReport(context.Background(), u)
	_, q := do(t, ts, "GET", "/api/v1/moderation/reports", ownTok, nil)
	var id string
	for _, r := range q["reports"].([]any) {
		if m := r.(map[string]any); m["reason"] == "gem_farming" {
			id = m["id"].(string)
		}
	}
	if id == "" {
		t.Fatalf("no report filed: %v", q)
	}
	if res, body := do(t, ts, "POST", "/api/v1/gems/capsule", aliceTok, nil); res.StatusCode != 409 || errCode(body) != "gems_held" {
		t.Fatalf("spent while held: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PATCH", "/api/v1/moderation/reports/"+id, ownTok, map[string]any{"version": 1, "status": "actioned", "note": "Scripted messages", "assigned_to": "0"}); res.StatusCode != 200 {
		t.Fatalf("review: %d %v", res.StatusCode, body)
	}
	_, b := do(t, ts, "GET", "/api/v1/gems", aliceTok, nil)
	w := b["wallet"].(map[string]any)
	if w["gems"] != float64(10) || w["held"] != float64(0) || w["state"] != "off" {
		t.Fatalf("after action: %v", w)
	}
	_, feed := do(t, ts, "GET", "/api/v1/nexa/feed", aliceTok, nil)
	found := false
	items, _ := feed["items"].([]any)
	for _, it := range items {
		if c, _ := it.(map[string]any)["case"].(map[string]any); c != nil && c["action"] == "gem_farming" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no case for the member: %v", feed)
	}
}
