package api

import (
	"context"
	"net/http/httptest"
	"testing"

	"evilion.xyz/chat/internal/ids"
)

func tradeLines(pairs ...any) []map[string]any {
	out := []map[string]any{}
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, map[string]any{"key": pairs[i], "qty": pairs[i+1]})
	}
	return out
}

// tradeOf is a trade as the API says it stands.
type tradeOf struct {
	t  *testing.T
	ts *httptest.Server
	id string
}

func openTrade(t *testing.T, ts *httptest.Server, tok, to string) (*tradeOf, int, map[string]any) {
	t.Helper()
	res, body := do(t, ts, "POST", "/api/v1/trades", tok, map[string]any{"to": to})
	id, _ := body["id"].(string)
	return &tradeOf{t, ts, id}, res.StatusCode, body
}

func (x *tradeOf) offer(tok string, lines []map[string]any) (int, map[string]any) {
	x.t.Helper()
	res, body := do(x.t, x.ts, "PUT", "/api/v1/trades/"+x.id+"/offer", tok, map[string]any{"lines": lines})
	return res.StatusCode, body
}

func (x *tradeOf) ready(tok string, version float64) (int, map[string]any) {
	x.t.Helper()
	res, body := do(x.t, x.ts, "PUT", "/api/v1/trades/"+x.id+"/ready", tok, map[string]any{"ready": true, "version": version})
	return res.StatusCode, body
}

func (x *tradeOf) get(tok string) (int, map[string]any) {
	x.t.Helper()
	res, body := do(x.t, x.ts, "GET", "/api/v1/trades/"+x.id, tok, nil)
	return res.StatusCode, body
}

func holds(t *testing.T, ts *httptest.Server, tok string) map[string]int {
	t.Helper()
	res, body := do(t, ts, "GET", "/api/v1/trades/holdings", tok, nil)
	if res.StatusCode != 200 {
		t.Fatalf("holdings: %d", res.StatusCode)
	}
	out := map[string]int{}
	for _, l := range body["_list"].([]any) {
		m := l.(map[string]any)
		out[m["key"].(string)] = int(m["qty"].(float64))
	}
	return out
}

// Trading moves copies and makes none: the earned copy leaves with its owner
// and is not earned again (staff giving it once more changes nothing), more
// than one can be held through trades, and the Badge Index of its owner alone
// says how many.
func TestBadgeTradeMovesCopiesAndEarnedOnesStayGone(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "owner")
	aliceTok, aliceID := register(t, ts, "alice")
	bobTok, bobID := register(t, ts, "bob")
	give := func(id string) {
		t.Helper()
		if res, body := do(t, ts, "PUT", "/api/v1/moderation/users/"+id+"/badges", ownTok, map[string]any{"badges": []string{"contributor"}}); res.StatusCode != 200 {
			t.Fatalf("give: %d %v", res.StatusCode, body)
		}
	}
	give(aliceID)
	give(bobID)

	x, code, body := openTrade(t, ts, aliceTok, bobID)
	if code != 201 {
		t.Fatalf("open: %d %v", code, body)
	}
	if code, body := x.offer(aliceTok, tradeLines("contributor", 1)); code != 200 {
		t.Fatalf("offer: %d %v", code, body)
	}
	_, now := x.get(bobTok)
	v := now["version"].(float64)
	if code, _ := x.ready(aliceTok, v); code != 200 {
		t.Fatalf("alice ready: %d", code)
	}
	if code, body := x.ready(bobTok, v); code != 200 || body["status"] != "accepted" {
		t.Fatalf("bob ready: %d %v", code, body)
	}
	if code, _ := x.ready(bobTok, v); code != 409 {
		t.Errorf("a finished trade took a Ready: %d", code)
	}
	if h := holds(t, ts, aliceTok); h["contributor"] != 0 {
		t.Errorf("alice kept the copy she traded: %v", h)
	}
	if h := holds(t, ts, bobTok); h["contributor"] != 2 {
		t.Errorf("bob should hold two: %v", h)
	}
	// staff saying it again does not make the earned copy again
	give(aliceID)
	if h := holds(t, ts, aliceTok); h["contributor"] != 0 {
		t.Errorf("the earned badge came back: %v", h)
	}
	_, mine := do(t, ts, "GET", "/api/v1/users/@me/badges", aliceTok, nil)
	for _, b := range mine["badges"].([]any) {
		if b.(map[string]any)["id"] == "contributor" {
			t.Error("a traded away badge is still on the profile")
		}
	}
	// Exist counts copies: bob's two, and none left with alice
	_, bobs := do(t, ts, "GET", "/api/v1/users/@me/badges", bobTok, nil)
	for _, b := range bobs["badges"].([]any) {
		if m := b.(map[string]any); m["id"] == "contributor" && m["exist"] != float64(2) {
			t.Errorf("Exist should count the two copies: %v", m["exist"])
		}
	}
	// the quantity is in the owner's index only
	_, idx := do(t, ts, "GET", "/api/v1/users/@me/badges/index", bobTok, nil)
	found := false
	for _, e := range idx["entries"].([]any) {
		if m := e.(map[string]any); m["art"] == "contributor" {
			found = true
			if m["qty"] != float64(2) {
				t.Errorf("index quantity: %v", m["qty"])
			}
		}
	}
	if !found {
		t.Error("contributor not in the index")
	}
	_, prof := do(t, ts, "GET", "/api/v1/users/"+bobID+"/profile", aliceTok, nil)
	if raw := prof["badges"]; raw != nil {
		for _, b := range raw.([]any) {
			if _, leaked := b.(map[string]any)["qty"]; leaked {
				t.Error("a profile carries a quantity")
			}
		}
	}
}

// A Ready is for the offers it was given for: changing an offer clears both
// marks, and a Ready for what was there before is refused, so nothing can be
// swapped in under someone's click. A copy offered in two trades goes with the
// first to finish, and nothing can be offered that is not held.
func TestBadgeTradeReadyIsForWhatWasSeen(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "owner")
	aliceTok, aliceID := register(t, ts, "alice")
	bobTok, bobID := register(t, ts, "bob")
	carolTok, carolID := register(t, ts, "carol")
	if res, body := do(t, ts, "PUT", "/api/v1/moderation/users/"+aliceID+"/badges", ownTok, map[string]any{"badges": []string{"legend", "mentor"}}); res.StatusCode != 200 {
		t.Fatalf("give: %d %v", res.StatusCode, body)
	}
	x, _, _ := openTrade(t, ts, aliceTok, bobID)
	if code, body := x.offer(bobTok, tradeLines("legend", 1)); code != 409 || errCode(body) != "trade_you_short" {
		t.Errorf("offered what bob lacks: %d %v", code, body)
	}
	_, empty := x.get(aliceTok)
	if code, body := x.ready(aliceTok, empty["version"].(float64)); code != 400 || errCode(body) != "trade_empty" {
		t.Errorf("ready on nothing: %d %v", code, body)
	}
	x.offer(aliceTok, tradeLines("legend", 1))
	_, seen := x.get(bobTok)
	v := seen["version"].(float64)
	if code, _ := x.ready(bobTok, v); code != 200 {
		t.Fatalf("bob ready: %d", code)
	}
	// alice swaps her offer: bob's mark goes, and a Ready for the old one is refused
	x.offer(aliceTok, tradeLines("mentor", 1))
	_, after := x.get(bobTok)
	if after["to_ready"] == true || after["from_ready"] == true {
		t.Errorf("a change left a Ready standing: %v", after)
	}
	if code, body := x.ready(bobTok, v); code != 409 || errCode(body) != "trade_changed" {
		t.Errorf("a Ready for the old offer: %d %v", code, body)
	}

	// the same copy in two trades
	y, _, _ := openTrade(t, ts, aliceTok, carolID)
	x.offer(aliceTok, tradeLines("legend", 1))
	y.offer(aliceTok, tradeLines("legend", 1))
	_, xs := x.get(aliceTok)
	_, ys := y.get(aliceTok)
	x.ready(aliceTok, xs["version"].(float64))
	y.ready(aliceTok, ys["version"].(float64))
	if code, _ := x.ready(bobTok, xs["version"].(float64)); code != 200 {
		t.Fatalf("first trade: %d", code)
	}
	if code, body := y.ready(carolTok, ys["version"].(float64)); code != 409 || errCode(body) != "trade_they_short" {
		t.Errorf("the same copy went twice: %d %v", code, body)
	}
	if h := holds(t, ts, carolTok); h["legend"] != 0 {
		t.Errorf("carol was given a copy that was gone: %v", h)
	}
}

// A trade is between its two people alone, follows the rules of a message (a
// block stops it), and names only badges people can trade.
func TestBadgeTradeIsPrivateAndFollowsMessageRules(t *testing.T) {
	srv, ts := newTestServer(t)
	register(t, ts, "owner")
	aliceTok, aliceID := register(t, ts, "alice")
	_, bobID := register(t, ts, "bob")
	carolTok, _ := register(t, ts, "carol")
	x, code, body := openTrade(t, ts, aliceTok, bobID)
	if code != 201 {
		t.Fatalf("open: %d %v", code, body)
	}
	if code, _ := x.get(carolTok); code != 404 {
		t.Errorf("a stranger read the trade: %d", code)
	}
	if code, _ := x.offer(carolTok, tradeLines("legend", 1)); code != 404 {
		t.Errorf("a stranger offered: %d", code)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/trades/"+x.id+"/end", carolTok, nil); res.StatusCode != 404 {
		t.Errorf("a stranger ended it: %d", res.StatusCode)
	}
	for _, key := range []string{"staff", "plan", "event:0", "bug-9", "nonsense"} {
		if code, body := x.offer(aliceTok, tradeLines(key, 1)); code != 400 || errCode(body) != "trade_invalid" {
			t.Errorf("%s was offered: %d %v", key, code, body)
		}
	}
	// opening again gives the same trade, from either side
	if y, code, _ := openTrade(t, ts, aliceTok, bobID); code != 200 || y.id != x.id {
		t.Errorf("a second trade between the same two: %d", code)
	}
	if res, body := do(t, ts, "POST", "/api/v1/trades/"+x.id+"/end", aliceTok, nil); res.StatusCode != 200 || body["status"] != "cancelled" {
		t.Errorf("end: %d %v", res.StatusCode, body)
	}
	a, _ := ids.Parse(aliceID)
	b, _ := ids.Parse(bobID)
	if err := srv.Store.Block(context.Background(), b, a); err != nil {
		t.Fatal(err)
	}
	if _, code, body := openTrade(t, ts, aliceTok, bobID); code != 403 || errCode(body) != "blocked" {
		t.Errorf("a blocked person opened a trade: %d %v", code, body)
	}
}
