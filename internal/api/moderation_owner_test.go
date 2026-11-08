package api

import (
	"fmt"
	"strings"
	"testing"
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
		{"GET", "/api/v1/moderation/gift-links"},
		{"POST", "/api/v1/moderation/gift-links"},
		{"DELETE", "/api/v1/moderation/gift-links/abc"},
	} {
		if res, _ := do(t, ts, c[0], c[1], devTok, map[string]any{"plan": "pro", "period": "year"}); res.StatusCode != 403 {
			t.Errorf("%s %s as a developer: %d", c[0], c[1], res.StatusCode)
		}
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
