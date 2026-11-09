package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// Badges staff give are theirs alone to give (a mod cannot), and one its
// holder hid is not in anyone else's view of the profile; a staff rank can
// never be hidden, so nobody passes for a member while acting as staff.
func TestBadgesStaffGiveAndHiddenStayHidden(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, ownID := register(t, ts, "owner")
	modTok, modID := register(t, ts, "mod")
	bobTok, bobID := register(t, ts, "bob")
	if res, body := do(t, ts, "PATCH", "/api/v1/admin/users/"+modID, ownTok, map[string]any{"rank": "mod"}); res.StatusCode != 200 {
		t.Fatalf("make mod: %d %v", res.StatusCode, body)
	}
	give := map[string]any{"badges": []string{"contributor"}}
	for _, tok := range []string{bobTok, modTok} {
		if res, _ := do(t, ts, "PUT", "/api/v1/moderation/users/"+bobID+"/badges", tok, give); res.StatusCode != 403 {
			t.Errorf("gave a badge without being an admin: %d", res.StatusCode)
		}
	}
	if res, body := do(t, ts, "PUT", "/api/v1/moderation/users/"+bobID+"/badges", ownTok, map[string]any{"badges": []string{"og"}}); res.StatusCode != 400 {
		t.Errorf("gave an earned badge by hand: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PUT", "/api/v1/moderation/users/"+bobID+"/badges", ownTok, give); res.StatusCode != 200 {
		t.Fatalf("give contributor: %d %v", res.StatusCode, body)
	}
	shown := func(tok, id string) map[string]bool {
		_, body := do(t, ts, "GET", "/api/v1/users/"+id+"/profile", tok, nil)
		out := map[string]bool{}
		for _, b := range body["badges"].([]any) {
			out[b.(map[string]any)["id"].(string)] = true
		}
		return out
	}
	if !shown(modTok, bobID)["contributor"] {
		t.Fatal("contributor not on the profile")
	}
	if res, body := do(t, ts, "PUT", "/api/v1/users/@me/badges", bobTok, map[string]any{"order": []string{"contributor"}, "hidden": []string{"contributor"}}); res.StatusCode != 200 {
		t.Fatalf("hide: %d %v", res.StatusCode, body)
	}
	if shown(modTok, bobID)["contributor"] || shown(bobTok, bobID)["contributor"] {
		t.Error("a hidden badge still shows on the profile")
	}
	_, mine := do(t, ts, "GET", "/api/v1/users/@me/badges", bobTok, nil)
	if l := mine["badges"].([]any); len(l) == 0 || l[0].(map[string]any)["hidden"] != true {
		t.Errorf("own list lost the hidden badge: %v", mine)
	}
	// the owner tries to hide their rank
	do(t, ts, "PUT", "/api/v1/users/@me/badges", ownTok, map[string]any{"hidden": []string{"staff"}})
	if !shown(bobTok, ownID)["staff"] {
		t.Error("a staff rank was hidden")
	}
}

// An earned badge staff give or take stays so until set back to automatic,
// only an admin can do it, and the holders list says where each came from.
func TestBadgeOverridesAndHolders(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "owner")
	modTok, modID := register(t, ts, "mod")
	_, bobID := register(t, ts, "bob")
	if res, body := do(t, ts, "PATCH", "/api/v1/admin/users/"+modID, ownTok, map[string]any{"rank": "mod"}); res.StatusCode != 200 {
		t.Fatalf("make mod: %d %v", res.StatusCode, body)
	}
	holder := "/api/v1/moderation/badges/og/holders/" + bobID
	if res, _ := do(t, ts, "PUT", holder, modTok, map[string]any{"state": "give", "level": 1}); res.StatusCode != 403 {
		t.Errorf("a mod gave an earned badge: %d", res.StatusCode)
	}
	if res, _ := do(t, ts, "PUT", "/api/v1/moderation/badges/plan/holders/"+bobID, ownTok, map[string]any{"state": "give", "level": 1}); res.StatusCode != 400 {
		t.Errorf("gave a plan by override: %d", res.StatusCode)
	}
	if res, body := do(t, ts, "PUT", holder, ownTok, map[string]any{"state": "give", "level": 1}); res.StatusCode != 204 {
		t.Fatalf("give og: %d %v", res.StatusCode, body)
	}
	badges := func() map[string]float64 {
		_, body := do(t, ts, "GET", "/api/v1/users/"+bobID+"/profile", ownTok, nil)
		out := map[string]float64{}
		for _, b := range body["badges"].([]any) {
			m := b.(map[string]any)
			lv, _ := m["level"].(float64)
			out[m["id"].(string)] = lv
		}
		return out
	}
	if _, ok := badges()["og"]; !ok {
		t.Fatal("given og not on the profile")
	}
	_, list := do(t, ts, "GET", "/api/v1/moderation/badges/og/holders", ownTok, nil)
	hs, _ := list["holders"].([]any)
	if len(hs) != 1 || hs[0].(map[string]any)["source"] != "given" || hs[0].(map[string]any)["user"] == nil {
		t.Errorf("holders: %v", list)
	}
	// the person's page sets them all at once; years at 3, og back to automatic
	put := map[string]any{"badges": []string{}, "overrides": map[string]int{"years": 3}}
	if res, body := do(t, ts, "PUT", "/api/v1/moderation/users/"+bobID+"/badges", ownTok, put); res.StatusCode != 200 {
		t.Fatalf("overrides: %d %v", res.StatusCode, body)
	}
	got := badges()
	if _, ok := got["og"]; ok || got["years"] != 3 {
		t.Errorf("after overrides: %v", got)
	}
	// an older app sends no overrides and changes none
	if res, _ := do(t, ts, "PUT", "/api/v1/moderation/users/"+bobID+"/badges", ownTok, map[string]any{"badges": []string{"contributor"}}); res.StatusCode != 200 {
		t.Fatal("older app's save failed")
	}
	if got := badges(); got["years"] != 3 {
		t.Errorf("an older app's save dropped overrides: %v", got)
	}
}

// The index counts each level and no staff rank, and says nothing about a
// badge until it is held: no name, meaning or colour to spoil how to get it.
func TestBadgeIndexCountsLevelsAndSpoilsNothing(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "owner")
	bobTok, bobID := register(t, ts, "bob")
	res, body := do(t, ts, "POST", "/api/v1/moderation/badges", ownTok, map[string]any{"name": "Secret Name", "description": "Secret way to get it", "icon": "star", "color": 0x112233, "color2": -1, "glyph": "light", "shape": "circle"})
	if res.StatusCode >= 300 {
		t.Fatalf("make badge: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PUT", "/api/v1/moderation/badges/bug_hunter/holders/"+bobID, ownTok, map[string]any{"state": "give", "level": 2}); res.StatusCode >= 300 {
		t.Fatalf("give level: %d %v", res.StatusCode, body)
	}
	_, index := do(t, ts, "GET", "/api/v1/users/@me/badges/index", bobTok, nil)
	raw, _ := json.Marshal(index)
	for _, leak := range []string{"Secret", "color", "bug_hunter\",\"level\":3"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the index gave away %q: %s", leak, raw)
		}
	}
	if index["collected"] != float64(2) {
		t.Errorf("two levels held should be two collected: %v", index["collected"])
	}
	_, staff := do(t, ts, "GET", "/api/v1/users/@me/badges/index", ownTok, nil)
	if staff["collected"] != float64(0) || staff["total"] != index["total"] {
		t.Errorf("a staff rank counted: %v of %v", staff["collected"], staff["total"])
	}
}

// Only the owner arranges the index, everyone's follows it, and nobody else is told what they lack.
func TestBadgeIndexOrderIsTheOwnersAlone(t *testing.T) {
	_, ts := newTestServer(t)
	owner, _ := register(t, ts, "owner")
	admin, adminID := register(t, ts, "admin")
	member, memberID := register(t, ts, "member")
	setRank(t, ts, owner, adminID, "admin")

	const path = "/api/v1/moderation/badges/index-order"
	put := func(tok string, order ...string) int {
		if order == nil {
			order = []string{}
		}
		res, _ := do(t, ts, "PUT", path, tok, map[string]any{"order": order})
		return res.StatusCode
	}
	for _, tok := range []string{admin, member} {
		if res, _ := do(t, ts, "GET", path, tok, nil); res.StatusCode < 400 {
			t.Fatalf("a non-owner read the arrangement: %d", res.StatusCode)
		}
		if code := put(tok, "booster"); code < 400 {
			t.Fatalf("a non-owner arranged the index: %d", code)
		}
	}
	if code := put(owner, "no-such-badge"); code != 400 {
		t.Fatalf("unknown key: %d", code)
	}
	if code := put(owner, "og", "og"); code != 400 {
		t.Fatalf("repeated key: %d", code)
	}

	_, got := do(t, ts, "GET", path, owner, nil)
	entries, _ := got["entries"].([]any)
	aToZ, _ := got["a_to_z"].([]any)
	if got["custom"] != false || len(entries) == 0 || len(entries) != len(aToZ) {
		t.Fatalf("arrangement: %v, %d entries, %d in A to Z", got["custom"], len(entries), len(aToZ))
	}
	if code := put(owner, "friendly-3", "booster"); code != 200 {
		t.Fatalf("save: %d", code)
	}

	// what someone holds comes first, in the owner's order; what they lack follows from A to Z
	first := func() (string, map[string]any) {
		_, index := do(t, ts, "GET", "/api/v1/users/@me/badges/index", member, nil)
		e := index["entries"].([]any)
		return e[0].(map[string]any)["art"].(string), index
	}
	if res, body := do(t, ts, "PUT", "/api/v1/moderation/users/"+memberID+"/badges", owner, map[string]any{"overrides": map[string]int{"booster": 1, "friendly": 3}}); res.StatusCode != 200 {
		t.Fatalf("overrides: %d %v", res.StatusCode, body)
	}
	art, index := first()
	entries2 := index["entries"].([]any)
	if second := entries2[1].(map[string]any)["art"]; art != "friendly-3" {
		t.Fatalf("the member's index does not follow the order: %v, %v", art, second)
	}
	held := 0
	for _, e := range entries2 {
		if e.(map[string]any)["badge"] != nil {
			held++
		}
	}
	locked := entries2[held:]
	if got := locked[0].(map[string]any)["art"]; got != "ideas-4" {
		t.Fatalf("the first locked badge is %v, want Architect (ideas-4)", got)
	}
	for _, e := range locked {
		m := e.(map[string]any)
		if m["badge"] != nil {
			t.Fatalf("a held badge among the locked: %v", m)
		}
		if _, named := m["name"]; named {
			t.Fatalf("a locked badge was sent with its name: %v", m)
		}
	}
	raw, _ := json.Marshal(index)
	if strings.Contains(string(raw), `"key"`) || strings.Contains(string(raw), `"a_to_z"`) {
		t.Fatalf("the member's index says more than it did: %s", raw)
	}
	if code := put(owner); code != 200 {
		t.Fatalf("clear: %d", code)
	}
	if art, _ := first(); art != "booster" {
		t.Fatalf("clearing did not go back to A to Z: %v", art)
	}
}
