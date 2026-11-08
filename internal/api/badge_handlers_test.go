package api

import "testing"

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
