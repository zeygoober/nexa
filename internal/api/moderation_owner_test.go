package api

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// What only the owner may do stays the owner's, however high another rank is.
func TestOwnerToolsRefuseEveryOtherRank(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "theowner")
	devTok, devID := register(t, ts, "adeveloper")
	_, bobID := register(t, ts, "bobby")
	setRank(t, ts, ownTok, devID, "developer")
	for _, c := range [][2]string{
		{"GET", "/api/v1/moderation/users/" + bobID + "/account"},
		{"POST", "/api/v1/moderation/servers/1/join"},
		{"GET", "/api/v1/moderation/servers/1/preview"},
		{"GET", "/api/v1/moderation/gift-links"},
		{"POST", "/api/v1/moderation/gift-links"},
		{"DELETE", "/api/v1/moderation/gift-links/abc"},
		{"GET", "/api/v1/moderation/users/" + bobID + "/gems"},
		{"POST", "/api/v1/moderation/users/" + bobID + "/gems"},
		{"GET", "/api/v1/moderation/users/" + bobID + "/collectables"},
		{"POST", "/api/v1/moderation/users/" + bobID + "/collectables"},
	} {
		if res, _ := do(t, ts, c[0], c[1], devTok, map[string]any{"plan": "pro", "period": "year", "amount": 500}); res.StatusCode != 403 {
			t.Errorf("%s %s as a developer: %d", c[0], c[1], res.StatusCode)
		}
	}
}

// The owner's Gems never take a wallet below nothing, and each change is in
// the person's ledger and the log.
func TestOwnerGemsNeverGoBelowNothing(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "theowner")
	bobTok, bobID := register(t, ts, "bobby")
	path := "/api/v1/moderation/users/" + bobID + "/gems"
	if res, body := do(t, ts, "POST", path, ownTok, map[string]any{"amount": -1}); errCode(body) != "gems_short" {
		t.Fatalf("taking from an empty wallet: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "POST", path, ownTok, map[string]any{"amount": 700, "reason": "event prize"}); res.StatusCode != 200 || body["gems"] != float64(700) {
		t.Fatalf("giving: %d %v", res.StatusCode, body)
	}
	if _, body := do(t, ts, "POST", path, ownTok, map[string]any{"amount": -701}); errCode(body) != "gems_short" {
		t.Fatalf("taking more than they have: %v", body)
	}
	if _, body := do(t, ts, "POST", path, ownTok, map[string]any{"amount": -200}); body["gems"] != float64(500) {
		t.Fatalf("taking: %v", body)
	}
	if _, rec := do(t, ts, "GET", "/api/v1/gems/record", bobTok, nil); !strings.Contains(fmt.Sprint(rec), "owner_given") || !strings.Contains(fmt.Sprint(rec), "owner_taken") {
		t.Fatalf("the ledger is missing a change: %v", rec)
	}
	if _, log := do(t, ts, "GET", "/api/v1/moderation/log?action=gems_give", ownTok, nil); !strings.Contains(fmt.Sprint(log), "event prize") {
		t.Fatalf("the gift is not in the log: %v", log)
	}
}

// The owner gives and takes copies of a collectable at one material only, never
// takes more than someone holds, and never makes an ordinary badge one.
func TestOwnerCollectablesStayWithinWhatIsHeld(t *testing.T) {
	srv, ts := newTestServer(t)
	ownTok, ownID := register(t, ts, "theowner")
	_, bobID := register(t, ts, "bobby")
	ctx := context.Background()
	owner, _ := ids.Parse(ownID)
	col := &store.BadgeDef{Name: "Comet", Icon: "star", Color: 0x336699, Color2: -1, IndexKind: store.BadgeExclusive, Rarity: "common", Art: "moon", CreatedBy: owner}
	plain := &store.BadgeDef{Name: "Helper of the Month", Icon: "star", Color: 0x336699, Color2: -1, IndexKind: store.BadgeExclusive, CreatedBy: owner}
	for _, d := range []*store.BadgeDef{col, plain} {
		if err := srv.Store.CreateBadgeDef(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/v1/moderation/users/" + bobID + "/collectables"
	silver := "event:" + col.ID.String() + "-2"
	if _, body := do(t, ts, "POST", path, ownTok, map[string]any{"badge": "event:" + plain.ID.String() + "-1", "amount": 1}); errCode(body) != "not_collectable" {
		t.Fatalf("giving an ordinary badge as a collectable: %v", body)
	}
	if _, body := do(t, ts, "POST", path, ownTok, map[string]any{"badge": silver, "amount": -1}); errCode(body) != "copies_short" {
		t.Fatalf("taking what they never had: %v", body)
	}
	if res, body := do(t, ts, "POST", path, ownTok, map[string]any{"badge": silver, "amount": 3, "reason": "event prize"}); res.StatusCode != 200 || body["copies"] != float64(3) {
		t.Fatalf("giving: %d %v", res.StatusCode, body)
	}
	if _, body := do(t, ts, "POST", path, ownTok, map[string]any{"badge": silver, "amount": -4}); errCode(body) != "copies_short" {
		t.Fatalf("taking more than they hold: %v", body)
	}
	if _, body := do(t, ts, "POST", path, ownTok, map[string]any{"badge": silver, "amount": -2}); body["copies"] != float64(1) {
		t.Fatalf("taking: %v", body)
	}
	if _, body := do(t, ts, "GET", path, ownTok, nil); fmt.Sprint(body["held"]) != "map["+silver+":1]" {
		t.Fatalf("held: %v", body["held"])
	}
	if _, log := do(t, ts, "GET", "/api/v1/moderation/log?action=collectable_give", ownTok, nil); !strings.Contains(fmt.Sprint(log), "event prize") {
		t.Fatalf("the gift is not in the log: %v", log)
	}
}

// Nothing about an account can be read without a line in the log.
func TestAccountDetailsAreLogged(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "theowner")
	_, bobID := register(t, ts, "bobby")
	res, body := do(t, ts, "GET", "/api/v1/moderation/users/"+bobID+"/account", ownTok, nil)
	if res.StatusCode != 200 {
		t.Fatalf("account: %d %v", res.StatusCode, body)
	}
	if body["created_at"] == nil || body["sessions"] == nil || body["addresses"] == nil {
		t.Fatalf("account is missing fields: %v", body)
	}
	if _, log := do(t, ts, "GET", "/api/v1/moderation/log?action=account_view", ownTok, nil); !strings.Contains(fmt.Sprint(log), "bobby") {
		t.Fatalf("the view is not in the log: %v", log)
	}
}

// A preview goes in the log, adds nobody to the server, and names only what a
// new member could see.
func TestServerPreviewIsLoggedAndShowsNoMore(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, ownID := register(t, ts, "theowner")
	bobTok, _ := register(t, ts, "bobby")
	_, sv := do(t, ts, "POST", "/api/v1/servers", bobTok, map[string]any{"name": "Quiet Place"})
	sid := sv["id"].(string)
	_, ch := do(t, ts, "POST", "/api/v1/servers/"+sid+"/channels", bobTok, map[string]any{"name": "secret-plans", "type": "text", "topic": "the hidden topic"})
	cid := ch["id"].(string)
	if res, body := do(t, ts, "PUT", "/api/v1/channels/"+cid+"/permissions/"+sid, bobTok, map[string]any{"type": "role", "allow": "0", "deny": "1024"}); res.StatusCode >= 300 {
		t.Fatalf("hiding the channel: %d %v", res.StatusCode, body)
	}
	_, open := do(t, ts, "POST", "/api/v1/servers/"+sid+"/channels", bobTok, map[string]any{"name": "lobby-chat", "type": "text", "topic": "an open topic"})
	if open["id"] == nil {
		t.Fatalf("no open channel: %v", open)
	}
	res, body := do(t, ts, "GET", "/api/v1/moderation/servers/"+sid+"/preview", ownTok, nil)
	if res.StatusCode != 200 {
		t.Fatalf("preview: %d %v", res.StatusCode, body)
	}
	said := fmt.Sprint(body)
	if !strings.Contains(said, "lobby-chat") || !strings.Contains(said, "bobby") {
		t.Fatalf("preview is missing what it should show: %v", body)
	}
	for _, kept := range []string{"secret-plans", "hidden topic", "open topic", "overwrites", "last_message_id", "key_conv_id"} {
		if strings.Contains(said, kept) {
			t.Fatalf("preview shows %q: %v", kept, body)
		}
	}
	if n, _ := body["private_channels"].(float64); n != 1 {
		t.Fatalf("private channels counted: %v", body["private_channels"])
	}
	if res, _ := do(t, ts, "GET", "/api/v1/servers/"+sid+"/members/"+ownID, bobTok, nil); res.StatusCode != 404 {
		t.Fatalf("the preview joined the owner: %d", res.StatusCode)
	}
	if _, log := do(t, ts, "GET", "/api/v1/moderation/log?action=server_preview", ownTok, nil); !strings.Contains(fmt.Sprint(log), "Quiet Place") {
		t.Fatalf("the preview is not in the log: %v", log)
	}
}

// A join from the panel makes an ordinary member, on the record, and a
// server's ban still holds against it.
func TestOwnerJoinIsVisibleAndLogged(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, ownID := register(t, ts, "theowner")
	bobTok, _ := register(t, ts, "bobby")
	_, sv := do(t, ts, "POST", "/api/v1/servers", bobTok, map[string]any{"name": "Private"})
	sid := sv["id"].(string)
	if res, body := do(t, ts, "POST", "/api/v1/moderation/servers/"+sid+"/join", ownTok, nil); res.StatusCode != 201 {
		t.Fatalf("join: %d %v", res.StatusCode, body)
	}
	if res, _ := do(t, ts, "GET", "/api/v1/servers/"+sid+"/members/"+ownID, bobTok, nil); res.StatusCode != 200 {
		t.Fatalf("the owner is not a member: %d", res.StatusCode)
	}
	if _, log := do(t, ts, "GET", "/api/v1/moderation/log?action=owner_join", ownTok, nil); !strings.Contains(fmt.Sprint(log), "Private") {
		t.Fatalf("the join is not in the log: %v", log)
	}

	_, banned := do(t, ts, "POST", "/api/v1/servers", bobTok, map[string]any{"name": "Closed"})
	bid := banned["id"].(string)
	if res, _ := do(t, ts, "PUT", "/api/v1/servers/"+bid+"/bans/"+ownID, bobTok, map[string]any{"reason": "no"}); res.StatusCode != 204 {
		t.Fatalf("ban: %d", res.StatusCode)
	}
	if res, body := do(t, ts, "POST", "/api/v1/moderation/servers/"+bid+"/join", ownTok, nil); res.StatusCode < 400 {
		t.Fatalf("a banned owner joined: %d %v", res.StatusCode, body)
	}
}

// A free link works once, can be taken back only until it is claimed, and
// earns nobody Gift Giver.
func TestOwnerGiftLinks(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "theowner")
	bobTok, _ := register(t, ts, "bobby")
	carolTok, _ := register(t, ts, "carol")
	make := func() string {
		t.Helper()
		res, body := do(t, ts, "POST", "/api/v1/moderation/gift-links", ownTok, map[string]any{"plan": "supporter", "period": "month"})
		if res.StatusCode != 200 && res.StatusCode != 201 {
			t.Fatalf("make: %d %v", res.StatusCode, body)
		}
		return body["code"].(string)
	}
	code := make()
	if res, body := do(t, ts, "POST", "/api/v1/gifts/"+code+"/claim", bobTok, nil); res.StatusCode != 200 {
		t.Fatalf("claim: %d %v", res.StatusCode, body)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/gifts/"+code+"/claim", carolTok, nil); res.StatusCode < 400 {
		t.Fatalf("a second claim passed: %d", res.StatusCode)
	}
	if res, _ := do(t, ts, "DELETE", "/api/v1/moderation/gift-links/"+code, ownTok, nil); res.StatusCode != 410 {
		t.Fatalf("revoked a claimed link: %d", res.StatusCode)
	}
	open := make()
	if res, _ := do(t, ts, "DELETE", "/api/v1/moderation/gift-links/"+open, ownTok, nil); res.StatusCode != 204 {
		t.Fatalf("revoke: %d", res.StatusCode)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/gifts/"+open+"/claim", carolTok, nil); res.StatusCode < 400 {
		t.Fatalf("claimed a revoked link: %d", res.StatusCode)
	}
	if _, body := do(t, ts, "GET", "/api/v1/users/@me/badges", ownTok, nil); strings.Contains(fmt.Sprint(body), "gift_giver") {
		t.Fatalf("a free link earned Gift Giver: %v", body)
	}
}

// Who holds what and every trade between two people are the owner's alone:
// an Admin, who has the holders list, is refused both.
func TestBadgeOwnersAndTradeLogAreTheOwnersAlone(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "theowner")
	admTok, admID := register(t, ts, "anadmin")
	setRank(t, ts, ownTok, admID, "admin")
	for _, path := range []string{"/api/v1/moderation/badges/og/owners", "/api/v1/moderation/trades"} {
		if res, _ := do(t, ts, "GET", path, admTok, nil); res.StatusCode != 403 {
			t.Fatalf("an admin read %s: %d", path, res.StatusCode)
		}
		if res, body := do(t, ts, "GET", path, ownTok, nil); res.StatusCode != 200 {
			t.Fatalf("the owner could not read %s: %d %v", path, res.StatusCode, body)
		}
	}
	if res, _ := do(t, ts, "GET", "/api/v1/moderation/badges/staff/owners", ownTok, nil); res.StatusCode != 404 {
		t.Fatalf("staff ranks have owners: %d", res.StatusCode)
	}
}
