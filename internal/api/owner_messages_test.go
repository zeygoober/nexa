package api

import (
	"bytes"
	"testing"
)

// A real outsider owns the instance. The existing DM and its keys belong to
// Alice and Bob, and a second developer must never inherit the owner's access.
func TestOwnerMessagesCutoverAndHistoricalConsent(t *testing.T) {
	s, ts := newTestServer(t)
	owner, oid := register(t, ts, "owner")
	alice, _ := register(t, ts, "alice")
	bob, bid := register(t, ts, "bob")
	dev, did := register(t, ts, "developer")
	setRank(t, ts, owner, did, "developer")
	ch := dmBetween(t, ts, alice, bid)
	createEpoch(t, ts, alice, ch)
	_, old := sendMsg(t, ts, alice, ch, ch, 0, "old", "old", nil)
	oldID := old["id"].(string)
	s.Cfg.OwnerMessageAccess = true
	if r, _ := sendMsg(t, ts, alice, ch, ch, 0, "legacy client", "no-copy", nil); r.StatusCode != 409 {
		t.Fatal("legacy client admitted", r.StatusCode)
	}
	box := b64(bytes.Repeat([]byte{5}, 100))
	env := envelopeFor(ch, 0, "new")
	env["owner"] = map[string]any{"id": did, "box": box}
	if r, _ := sendMsg(t, ts, alice, ch, ch, 0, "", "wrong", map[string]any{"ciphertext": env}); r.StatusCode != 400 {
		t.Fatal("wrong owner admitted", r.StatusCode)
	}
	env["owner"] = map[string]any{"id": oid, "box": box}
	edit := map[string]any{"ciphertext": env, "key_conv_id": ch, "key_epoch": 0}
	if r, _ := do(t, ts, "PATCH", "/api/v1/channels/"+ch+"/messages/"+oldID, alice, edit); r.StatusCode != 400 {
		t.Fatal("legacy edit opted in", r.StatusCode)
	}
	r, newMsg := sendMsg(t, ts, alice, ch, ch, 0, "", "new", edit)
	if r.StatusCode != 201 {
		t.Fatal(r.StatusCode, newMsg)
	}
	mid := newMsg["id"].(string)
	feed := "/api/v1/moderation/messages"
	for _, token := range []string{alice, bob, dev, ""} {
		r, _ := do(t, ts, "GET", feed, token, nil)
		if r.StatusCode != 401 && r.StatusCode != 403 {
			t.Fatal("nonowner read feed", r.StatusCode)
		}
	}
	r, body := do(t, ts, "GET", feed, owner, nil)
	rows := body["_list"].([]any)
	if r.StatusCode != 200 || len(rows) != 1 || rows[0].(map[string]any)["id"] != mid {
		t.Fatal(r.StatusCode, body)
	}
	if r, _ := do(t, ts, "GET", "/api/v1/channels/"+ch+"/messages", owner, nil); r.StatusCode == 200 {
		t.Fatal("ordinary history opened")
	}
	const requests = "/api/v1/moderation/history-reviews"
	makeReview := func() string {
		r, b := do(t, ts, "POST", requests, owner, map[string]any{"channel_id": ch, "reason": "Check a reported incident", "message_ids": []string{oldID}})
		if r.StatusCode != 201 {
			t.Fatal(r.StatusCode, b)
		}
		return b["id"].(string)
	}
	id := makeReview()
	part := "/api/v1/users/@me/history-reviews/" + id + "/"
	view := requests + "/" + id + "/messages"
	call := func(method, path, token string, data any, want int) {
		t.Helper()
		r, b := do(t, ts, method, path, token, data)
		if r.StatusCode != want {
			t.Fatalf("%s %s got %d want %d %v", method, path, r.StatusCode, want, b)
		}
	}
	call("POST", part+"preview", dev, nil, 404)
	call("POST", part+"approve", owner, nil, 404)
	call("POST", part+"preview", bob, nil, 200)
	call("POST", part+"approve", alice, nil, 204)
	call("POST", part+"approve", alice, nil, 204)
	boxes := map[string]string{oldID: box}
	call("POST", part+"share", alice, boxes, 409)
	call("GET", view, owner, nil, 409)
	call("POST", part+"approve", bob, nil, 204)
	call("POST", part+"share", alice, boxes, 204)
	call("POST", part+"share", alice, boxes, 409)
	call("GET", view, dev, nil, 403)
	call("GET", view, owner, nil, 200)
	call("POST", part+"withdraw", bob, nil, 204)
	call("GET", view, owner, nil, 409)
	id = makeReview()
	part = "/api/v1/users/@me/history-reviews/" + id + "/"
	call("POST", part+"approve", bob, nil, 204)
	call("PATCH", "/api/v1/channels/"+ch+"/messages/"+oldID, alice, map[string]any{"ciphertext": envelopeFor(ch, 0, "changed"), "key_conv_id": ch, "key_epoch": 0}, 200)
	call("POST", part+"approve", alice, nil, 409)
	call("DELETE", "/api/v1/channels/"+ch+"/messages/"+mid, alice, nil, 204)
	_, body = do(t, ts, "GET", feed, owner, nil)
	if len(body["_list"].([]any)) != 0 {
		t.Fatal("deleted owner copy survived")
	}
}
