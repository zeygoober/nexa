package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// modAct runs one moderation action and returns the response.
func modAct(t *testing.T, ts *httptest.Server, tok, target string, body map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	return do(t, ts, "POST", "/api/v1/moderation/users/"+target+"/actions", tok, body)
}

// mustAct runs an action that has to succeed and returns its case.
func mustAct(t *testing.T, ts *httptest.Server, tok, target string, body map[string]any) map[string]any {
	t.Helper()
	res, out := modAct(t, ts, tok, target, body)
	if res.StatusCode != 201 {
		t.Fatalf("%v: %d %v", body["action"], res.StatusCode, out)
	}
	return out["case"].(map[string]any)
}

func meNexa(t *testing.T, ts *httptest.Server, tok string) map[string]any {
	t.Helper()
	res, me := do(t, ts, "GET", "/api/v1/users/@me", tok, nil)
	if res.StatusCode != 200 {
		t.Fatalf("me: %d %v", res.StatusCode, me)
	}
	st, _ := me["nexa"].(map[string]any)
	if st == nil {
		t.Fatalf("me has no nexa state: %v", me)
	}
	return st
}

// dialRaw connects to the gateway and identifies, returning the socket and
// the first frame or close code it gets back.
func dialRaw(t *testing.T, ts *httptest.Server, token string) (*websocket.Conn, map[string]any, websocket.StatusCode) {
	t.Helper()
	ws, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(ts.URL, "http")+"/gateway", &websocket.DialOptions{HTTPHeader: map[string][]string{"Origin": {"http://localhost"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.CloseNow() })
	b, _ := json.Marshal(map[string]any{"op": "identify", "d": map[string]string{"token": token}})
	if err := ws.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, raw, err := ws.Read(ctx)
	if err != nil {
		return ws, nil, websocket.CloseStatus(err)
	}
	var f map[string]any
	_ = json.Unmarshal(raw, &f)
	return ws, f, -1
}

func TestModerationIsStaffOnly(t *testing.T) {
	_, ts := newTestServer(t)
	register(t, ts, "owner")
	tok, _ := register(t, ts, "member")
	for _, p := range []string{"/api/v1/moderation/summary", "/api/v1/moderation/users", "/api/v1/moderation/cases", "/api/v1/moderation/log", "/api/v1/moderation/announcements", "/api/v1/moderation/packs", "/api/v1/moderation/uploads", "/api/v1/moderation/users/1/alts"} {
		if res, body := do(t, ts, "GET", p, tok, nil); res.StatusCode != 403 || errCode(body) != "admin_only" {
			t.Errorf("%s as a member: %d %v", p, res.StatusCode, body)
		}
	}
	if res, _ := do(t, ts, "GET", "/api/v1/moderation/users", "", nil); res.StatusCode != 401 {
		t.Errorf("signed out: %d", res.StatusCode)
	}
}

func TestModerationHierarchy(t *testing.T) {
	srv, ts := newTestServer(t)
	ownTok, ownID := register(t, ts, "owner")
	modTok, modID := register(t, ts, "mod")
	_, otherModID := register(t, ts, "othermod")
	_, memID := register(t, ts, "member")
	for _, id := range []string{modID, otherModID} {
		if res, body := do(t, ts, "PATCH", "/api/v1/admin/users/"+id, ownTok, map[string]any{"admin": true}); res.StatusCode != 200 {
			t.Fatalf("promote: %d %v", res.StatusCode, body)
		}
	}
	sys, err := srv.systemUser(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	mute := map[string]any{"action": "mute", "reason": "testing the rules", "duration_ms": 3600000}
	for _, c := range []struct {
		tok, target, code string
	}{
		{modTok, modID, "moderate_self"},
		{modTok, ownID, "moderate_owner"},
		{modTok, otherModID, "moderate_staff"},
		{ownTok, modID, "moderate_staff"},
		{modTok, sys.ID.String(), "moderate_system"},
	} {
		if res, body := modAct(t, ts, c.tok, c.target, mute); errCode(body) != c.code {
			t.Errorf("acting on %s: %d %v, want %s", c.target, res.StatusCode, body, c.code)
		}
	}
	// nobody takes the owner's access away, so nobody can get round the rule above
	if res, body := do(t, ts, "PATCH", "/api/v1/admin/users/"+ownID, modTok, map[string]any{"admin": false}); res.StatusCode != 403 || errCode(body) != "owner_protected" {
		t.Errorf("demote the owner: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PATCH", "/api/v1/admin/users/"+ownID, modTok, map[string]any{"disabled": true}); res.StatusCode != 403 || errCode(body) != "owner_protected" {
		t.Errorf("disable the owner: %d %v", res.StatusCode, body)
	}
	// every access change is in the log
	_, logBody := do(t, ts, "GET", "/api/v1/moderation/log?action=rank_change", ownTok, nil)
	if n := len(logBody["entries"].([]any)); n != 2 {
		t.Errorf("rank changes logged: %d", n)
	}
	if res, body := modAct(t, ts, modTok, memID, map[string]any{"action": "mute", "reason": "x", "duration_ms": 3600000}); errCode(body) != "reason_required" {
		t.Errorf("short reason: %d %v", res.StatusCode, body)
	}
	if res, body := modAct(t, ts, modTok, memID, map[string]any{"action": "explode", "reason": "a reason"}); errCode(body) != "unknown_action" {
		t.Errorf("unknown action: %d %v", res.StatusCode, body)
	}
	for _, d := range []int64{0, 1000, 366 * 24 * 3600 * 1000} {
		if res, body := modAct(t, ts, modTok, memID, map[string]any{"action": "mute", "reason": "a reason", "duration_ms": d}); errCode(body) != "bad_duration" {
			t.Errorf("mute for %dms: %d %v", d, res.StatusCode, body)
		}
	}
}

func TestNexaSystemAccount(t *testing.T) {
	srv, ts := newTestServer(t)
	tok, _ := register(t, ts, "alice")
	st := meNexa(t, ts, tok)
	nexaID, _ := st["account_id"].(string)
	if nexaID == "" || nexaID == "0" {
		t.Fatalf("no Nexa account: %v", st)
	}
	sys, err := srv.systemUser(t.Context())
	if err != nil || sys.ID.String() != nexaID || sys.Username != "nexa" || sys.DisplayName != "Nexa" || sys.Flags&store.FlagSystem == 0 {
		t.Fatalf("system account: %v %+v", err, sys)
	}
	// its profile says what it is
	_, pub := do(t, ts, "GET", "/api/v1/users/"+nexaID, tok, nil)
	if int64(pub["flags"].(float64))&store.FlagSystem == 0 {
		t.Errorf("public flags do not mark the system account: %v", pub)
	}
	// nobody signs in as it
	_, kdf := do(t, ts, "GET", "/api/v1/auth/kdf?username=nexa", "", nil)
	if kdf["salt"] == b64(sys.KDFSalt) {
		t.Error("the system account's real salt was handed out")
	}
	if res, _ := do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "nexa", "auth_key": b64(key32)}); res.StatusCode != 401 {
		t.Errorf("sign in as Nexa: %d", res.StatusCode)
	}
	// nobody messages, befriends or blocks it, or finds it in search
	if res, body := do(t, ts, "POST", "/api/v1/users/@me/channels", tok, map[string]any{"recipient_ids": []string{nexaID}}); res.StatusCode != 403 || errCode(body) != "nexa_system" {
		t.Errorf("DM Nexa: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "POST", "/api/v1/users/@me/relationships", tok, map[string]any{"username": "nexa"}); res.StatusCode != 403 || errCode(body) != "nexa_system" {
		t.Errorf("befriend Nexa: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PUT", "/api/v1/users/@me/blocks/"+nexaID, tok, nil); res.StatusCode != 403 || errCode(body) != "nexa_system" {
		t.Errorf("block Nexa: %d %v", res.StatusCode, body)
	}
	if _, body := do(t, ts, "GET", "/api/v1/users/search?q=nex", tok, nil); len(body["_list"].([]any)) != 0 {
		t.Errorf("Nexa in search: %v", body)
	}
	// and nobody dresses up as it
	for _, name := range []string{"Nexa", "nexa", " NEXA ", "Nexa Team", "nexa_staff", "N.e.x.a"} {
		if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, map[string]any{"display_name": name}); errCode(body) != "reserved_display_name" {
			t.Errorf("display name %q: %d %v", name, res.StatusCode, body)
		}
	}
	if res, _ := do(t, ts, "PATCH", "/api/v1/users/@me", tok, map[string]any{"display_name": "Nexa fan"}); res.StatusCode != 200 {
		t.Errorf("a name that merely mentions Nexa: %d", res.StatusCode)
	}
	b := registerBody("bob")
	b["display_name"] = "Nexa"
	if res, body := do(t, ts, "POST", "/api/v1/auth/register", "", b); errCode(body) != "reserved_display_name" {
		t.Errorf("register as Nexa: %d %v", res.StatusCode, body)
	}
}

// The Nexa account can exist before anyone registers; the first person must
// still become Nexa's admin.
func TestFirstAccountIsAdminEvenAfterNexa(t *testing.T) {
	srv, ts := newTestServer(t)
	if _, err := srv.systemUser(t.Context()); err != nil {
		t.Fatal(err)
	}
	res, body := do(t, ts, "POST", "/api/v1/auth/register", "", registerBody("founder"))
	if res.StatusCode != 201 {
		t.Fatalf("%d %v", res.StatusCode, body)
	}
	if int64(body["user"].(map[string]any)["flags"].(float64))&store.FlagAdmin == 0 {
		t.Error("the first person to register is not an admin")
	}
	_, stats := do(t, ts, "GET", "/api/v1/admin/stats", body["token"].(string), nil)
	if stats["users"].(float64) != 1 {
		t.Errorf("stats count Nexa as a user: %v", stats)
	}
}

func TestRemoveProfilePicture(t *testing.T) {
	srv, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	tok, id := register(t, ts, "victim")
	for _, kind := range []string{"avatar", "banner"} {
		if res, body := rawPut(t, ts.URL+"/api/v1/users/@me/"+kind, tok, "image/png", pngBytes(32, 32)); res.StatusCode != 200 {
			t.Fatalf("%s: %d %s", kind, res.StatusCode, body)
		}
	}
	before, _ := srv.Store.UserByID(t.Context(), modID(id))
	g := connectGW(t, ts, tok)

	if res, body := modAct(t, ts, modTok, id, map[string]any{"action": "avatar_remove", "reason": "an offensive picture", "images": map[string]bool{}}); errCode(body) != "nothing_selected" {
		t.Errorf("nothing chosen: %d %v", res.StatusCode, body)
	}
	c := mustAct(t, ts, modTok, id, map[string]any{"action": "avatar_remove", "reason": "an offensive picture", "images": map[string]bool{"avatar": true, "banner": true}})
	if c["action"] != "avatar_remove" || c["number"].(float64) != 1 || c["status"] != "open" {
		t.Errorf("case: %v", c)
	}
	after, _ := srv.Store.UserByID(t.Context(), modID(id))
	if after.AvatarID != nil || after.BannerID != nil {
		t.Errorf("pictures still there: %v %v", after.AvatarID, after.BannerID)
	}
	for _, bid := range []*ids.ID{before.AvatarID, before.BannerID} {
		b, err := srv.Store.BlobByID(t.Context(), *bid)
		if err != nil || b.RefCount != 0 {
			t.Errorf("blob %v still referenced: %v %+v", *bid, err, b)
		}
	}
	// the member hears at once, with the reason and who did it
	n := g.next("nexa_notice", 3*time.Second)
	if n == nil {
		t.Fatal("no notice")
	}
	nc := n["case"].(map[string]any)
	if nc["reason"] != "an offensive picture" || nc["moderator_id"] == nil || n["people"].(map[string]any)[nc["moderator_id"].(string)] == nil {
		t.Errorf("notice lacks the reason or the moderator: %v", n)
	}
	// and it waits for them until dismissed
	st := meNexa(t, ts, tok)
	if len(st["pending"].([]any)) != 1 || st["unread"].(float64) < 1 {
		t.Errorf("pending notice: %v", st)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/nexa/cases/"+c["id"].(string)+"/ack", tok, nil); res.StatusCode != 204 {
		t.Errorf("ack: %d", res.StatusCode)
	}
	if st := meNexa(t, ts, tok); len(st["pending"].([]any)) != 0 {
		t.Errorf("dismissed notice still pending: %v", st)
	}
	// nothing left to remove
	if res, body := modAct(t, ts, modTok, id, map[string]any{"action": "avatar_remove", "reason": "again"}); errCode(body) != "no_avatar" {
		t.Errorf("remove a missing avatar: %d %v", res.StatusCode, body)
	}
	// someone else's case is not theirs to see
	otherTok, _ := register(t, ts, "bystander")
	if res, _ := do(t, ts, "GET", "/api/v1/nexa/cases/"+c["id"].(string), otherTok, nil); res.StatusCode != 404 {
		t.Errorf("another member reads the case: %d", res.StatusCode)
	}
}

var resetValue = regexp.MustCompile(`^MODERATOR_REMOVED_[a-z0-9]{8}$`)
var resetUsername = regexp.MustCompile(`^moderator_removed_[a-z0-9]{8}$`)

func TestResetFieldsKeepsSignInWorking(t *testing.T) {
	srv, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	tok, id := register(t, ts, "badname")
	if res, _ := do(t, ts, "PATCH", "/api/v1/users/@me", tok, map[string]any{"about_me": "rude", "pronouns": "x/y", "status_text": "hello", "display_name": "Rude Name"}); res.StatusCode != 200 {
		t.Fatal("set profile")
	}
	c := mustAct(t, ts, modTok, id, map[string]any{"action": "field_reset", "reason": "hateful profile", "fields": []string{"username", "display_name", "about_me", "pronouns", "status"}})
	u, _ := srv.Store.UserByID(t.Context(), modID(id))
	if !resetUsername.MatchString(u.Username) || !ValidUsername(u.Username) {
		t.Errorf("username %q is not a valid, lowercase reset name", u.Username)
	}
	for _, v := range []string{u.DisplayName, u.AboutMe, u.Pronouns} {
		if !resetValue.MatchString(v) {
			t.Errorf("reset value %q", v)
		}
	}
	if u.StatusText != "" || u.StatusEmoji != "" {
		t.Errorf("status not cleared: %q %q", u.StatusText, u.StatusEmoji)
	}
	var details struct {
		Old map[string]string `json:"old"`
		New map[string]string `json:"new"`
	}
	raw, _ := json.Marshal(c["details"])
	_ = json.Unmarshal(raw, &details)
	if details.Old["username"] != "badname" || details.New["username"] != u.Username {
		t.Errorf("case details: %s", raw)
	}
	// The member can sign in with the new name — and with the old one, which
	// is all they know if they were away when it happened.
	for _, name := range []string{u.Username, "badname", "BadName"} {
		_, kdf := do(t, ts, "GET", "/api/v1/auth/kdf?username="+name, "", nil)
		if kdf["salt"] != b64(salt16) {
			t.Errorf("kdf for %q gave a stand-in salt: %v", name, kdf)
		}
		res, body := do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": name, "auth_key": b64(key32)})
		if res.StatusCode != 200 || body["user"].(map[string]any)["id"] != id {
			t.Errorf("sign in as %q: %d %v", name, res.StatusCode, body)
		}
	}
	// and recover with it
	if res, body := do(t, ts, "POST", "/api/v1/auth/recover/begin", "", map[string]any{"username": "badname", "recovery_auth_key": b64(key32b)}); res.StatusCode != 200 {
		t.Errorf("recover by the old name: %d %v", res.StatusCode, body)
	}
	// nobody can take the old name: not someone new, and not them
	if res, body := do(t, ts, "POST", "/api/v1/auth/register", "", registerBody("badname")); res.StatusCode != 409 || errCode(body) != "username_taken" {
		t.Errorf("register the reset name: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, map[string]any{"username": "badname"}); res.StatusCode != 409 {
		t.Errorf("take the old name back: %d %v", res.StatusCode, body)
	}
	// they can pick a new one of their own
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, map[string]any{"username": "goodname"}); res.StatusCode != 200 {
		t.Errorf("rename: %d %v", res.StatusCode, body)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "goodname", "auth_key": b64(key32)}); res.StatusCode != 200 {
		t.Errorf("sign in by the new own name: %d", res.StatusCode)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "badname", "auth_key": b64(key32)}); res.StatusCode != 200 {
		t.Errorf("the held name stopped working after a rename: %d", res.StatusCode)
	}
	// an empty field has nothing to reset
	if res, body := modAct(t, ts, modTok, id, map[string]any{"action": "field_reset", "reason": "again", "fields": []string{"status"}}); errCode(body) != "field_empty" {
		t.Errorf("reset an empty status: %d %v", res.StatusCode, body)
	}
}

func TestMuteIsReadOnly(t *testing.T) {
	srv, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	tokA, idA := register(t, ts, "alice")
	tokB, idB := register(t, ts, "bob")
	_, body := do(t, ts, "POST", "/api/v1/users/@me/channels", tokA, map[string]any{"recipient_ids": []string{idB}})
	dm := body["id"].(string)
	createEpoch(t, ts, tokA, dm)
	res, body := sendMsg(t, ts, tokA, dm, dm, 0, "before", "", nil)
	if res.StatusCode != 201 {
		t.Fatalf("send: %d %v", res.StatusCode, body)
	}
	mine := body["id"].(string)
	_, body = sendMsg(t, ts, tokB, dm, dm, 0, "theirs", "", nil)
	theirs := body["id"].(string)

	c := mustAct(t, ts, modTok, idA, map[string]any{"action": "mute", "reason": "spamming", "duration_ms": 3600000})
	if c["expires_at"] == nil || !c["in_effect"].(bool) {
		t.Errorf("mute case: %v", c)
	}
	st := meNexa(t, ts, tokA)
	if st["mute"] == nil || st["mute"].(map[string]any)["reason"] != "spamming" {
		t.Errorf("state lacks the mute: %v", st)
	}
	// reading works; saying anything does not
	if res, _ := do(t, ts, "GET", "/api/v1/channels/"+dm+"/messages", tokA, nil); res.StatusCode != 200 {
		t.Errorf("read: %d", res.StatusCode)
	}
	refused := func(what string, res *http.Response, body map[string]any) {
		t.Helper()
		if res.StatusCode != 403 || errCode(body) != "account_muted" {
			t.Errorf("%s while muted: %d %v", what, res.StatusCode, body)
		}
	}
	res, body = sendMsg(t, ts, tokA, dm, dm, 0, "during", "", nil)
	refused("send", res, body)
	res, body = do(t, ts, "PUT", "/api/v1/channels/"+dm+"/messages/"+theirs+"/reactions/%F0%9F%91%8D/@me", tokA, nil)
	refused("react", res, body)
	res, body = do(t, ts, "PATCH", "/api/v1/users/@me", tokA, map[string]any{"display_name": "new name"})
	refused("rename", res, body)
	res, body = do(t, ts, "PATCH", "/api/v1/users/@me", tokA, map[string]any{"about_me": "new words"})
	refused("about me", res, body)
	res, body = rawPutJSON(t, ts, "/api/v1/users/@me/avatar", tokA)
	refused("avatar", res, body)
	res, body = do(t, ts, "POST", "/api/v1/servers", tokA, map[string]any{"name": "mine"})
	refused("new server", res, body)
	res, body = do(t, ts, "POST", "/api/v1/users/@me/relationships", tokA, map[string]any{"username": "owner"})
	refused("friend request", res, body)
	res, body = do(t, ts, "POST", "/api/v1/channels/"+dm+"/typing", tokA, nil)
	refused("typing", res, body)
	res, body = do(t, ts, "DELETE", "/api/v1/channels/"+dm+"/messages/"+theirs, tokA, nil)
	if res.StatusCode != 403 {
		t.Errorf("delete someone else's message while muted: %d %v", res.StatusCode, body)
	}
	// private things, and taking things down, still work
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", "/api/v1/users/@me", map[string]any{"presence_pref": "dnd"}},
		{"PATCH", "/api/v1/users/@me", map[string]any{"about_me": ""}},
		{"POST", "/api/v1/channels/" + dm + "/messages/" + theirs + "/ack", nil},
		{"DELETE", "/api/v1/channels/" + dm + "/messages/" + mine, nil},
		{"PUT", "/api/v1/users/@me/settings", map[string]any{"blob": b64([]byte("settings")), "version": 0}},
	} {
		if res, body := do(t, ts, c.method, c.path, tokA, c.body); res.StatusCode >= 300 {
			t.Errorf("%s %s while muted: %d %v", c.method, c.path, res.StatusCode, body)
		}
	}
	// no calls
	g := connectGW(t, ts, tokA)
	voice, _ := json.Marshal(map[string]any{"op": "voice_state", "d": map[string]any{"channel_id": dm, "sfu": srv.Realtime != nil}})
	_ = g.ws.Write(context.Background(), websocket.MessageText, voice)
	if e := g.next("voice_error", 3*time.Second); e == nil || e["code"] != "account_muted" {
		t.Errorf("join a call while muted: %v", e)
	}
	// the appeal is open to them
	if res, body := do(t, ts, "POST", "/api/v1/nexa/cases/"+c["id"].(string)+"/messages", tokA, map[string]any{"body": "I'm sorry"}); res.StatusCode != 201 {
		t.Errorf("appeal while muted: %d %v", res.StatusCode, body)
	}
	// lifting it puts everything back
	if res, body := do(t, ts, "POST", "/api/v1/moderation/cases/"+c["id"].(string)+"/lift", modTok, map[string]any{"reason": "appeal accepted"}); res.StatusCode != 200 {
		t.Fatalf("lift: %d %v", res.StatusCode, body)
	}
	if res, body := sendMsg(t, ts, tokA, dm, dm, 0, "after", "", nil); res.StatusCode != 201 {
		t.Errorf("send after the lift: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "POST", "/api/v1/moderation/cases/"+c["id"].(string)+"/lift", modTok, map[string]any{"reason": "again"}); res.StatusCode != 409 || errCode(body) != "not_in_effect" {
		t.Errorf("lift twice: %d %v", res.StatusCode, body)
	}
	_, logBody := do(t, ts, "GET", "/api/v1/moderation/log", modTok, nil)
	actions := []string{}
	for _, e := range logBody["entries"].([]any) {
		actions = append(actions, e.(map[string]any)["action"].(string))
	}
	if strings.Join(actions, ",") != "unmute,mute" {
		t.Errorf("log: %v", actions)
	}
}

// rawPutJSON sends a PUT with no body worth reading, for routes a restriction
// refuses before they look at one.
func rawPutJSON(t *testing.T, ts *httptest.Server, path, tok string) (*http.Response, map[string]any) {
	t.Helper()
	res, raw := rawPut(t, ts.URL+path, tok, "image/png", pngBytes(8, 8))
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return res, out
}

func TestMuteRunsOut(t *testing.T) {
	srv, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	tok, id := register(t, ts, "alice")
	c := mustAct(t, ts, modTok, id, map[string]any{"action": "mute", "reason": "cool off", "duration_ms": 60000})
	g := connectGW(t, ts, tok)
	// wind the clock on: the mute's time has passed
	if _, err := srv.Store.DB().W.Exec(`UPDATE mod_restrictions SET expires_at = ?`, time.Now().Add(-time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	// no longer in force the moment it runs out, sweeper or not
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, map[string]any{"display_name": "Alice"}); res.StatusCode != 200 {
		t.Errorf("edit after the mute ran out: %d %v", res.StatusCode, body)
	}
	srv.sweepRestrictions(t.Context())
	ev := g.next("nexa_case", 3*time.Second)
	if ev == nil {
		t.Fatal("the member was not told the mute ended")
	}
	if ev["case"].(map[string]any)["ended"] != true || ev["state"].(map[string]any)["mute"] != nil {
		t.Errorf("ended case: %v", ev)
	}
	lines := ev["messages"].([]any)
	if len(lines) != 1 || lines[0].(map[string]any)["event"] != "expired" {
		t.Errorf("thread line: %v", lines)
	}
	_ = c
}

func TestBanClosesTheDoor(t *testing.T) {
	_, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	tok, id := register(t, ts, "alice")
	g := connectGW(t, ts, tok)
	c := mustAct(t, ts, modTok, id, map[string]any{"action": "ban", "reason": "threats", "permanent": true})
	if c["expires_at"] != nil {
		t.Errorf("a permanent ban ends: %v", c)
	}
	// the notice arrives before the socket is closed on them
	if g.next("nexa_notice", 3*time.Second) == nil {
		t.Error("no notice before the disconnect")
	}
	select {
	case <-g.done:
	case <-time.After(3 * time.Second):
		t.Error("the banned member's socket stayed open")
	}
	// and they cannot come back in
	if _, first, code := dialRaw(t, ts, tok); code != 4003 {
		t.Errorf("reconnect while banned: frame %v code %d", first, code)
	}
	// the session survives, for the ban screen and the appeal, and nothing else
	st := meNexa(t, ts, tok)
	if st["ban"] == nil || st["ban"].(map[string]any)["reason"] != "threats" {
		t.Errorf("ban state: %v", st)
	}
	for _, p := range []string{"/api/v1/users/@me/channels", "/api/v1/users/@me/relationships", "/api/v1/users/search?q=ow"} {
		if res, body := do(t, ts, "GET", p, tok, nil); res.StatusCode != 403 || errCode(body) != "account_banned" {
			t.Errorf("GET %s while banned: %d %v", p, res.StatusCode, body)
		}
	}
	if res, _ := do(t, ts, "GET", "/api/v1/nexa/feed", tok, nil); res.StatusCode != 200 {
		t.Errorf("feed while banned: %d", res.StatusCode)
	}
	if res, body := do(t, ts, "POST", "/api/v1/nexa/cases/"+c["id"].(string)+"/messages", tok, map[string]any{"body": "please reconsider"}); res.StatusCode != 201 {
		t.Errorf("appeal while banned: %d %v", res.StatusCode, body)
	}
	// the appeal is waiting on the staff
	_, cases := do(t, ts, "GET", "/api/v1/moderation/cases?view=needs_reply", modTok, nil)
	if len(cases["cases"].([]any)) != 1 || cases["counts"].(map[string]any)["needs_reply"].(float64) != 1 {
		t.Errorf("needs reply: %v", cases)
	}
	if res, body := do(t, ts, "POST", "/api/v1/moderation/cases/"+c["id"].(string)+"/lift", modTok, map[string]any{"reason": "we talked it over"}); res.StatusCode != 200 {
		t.Fatalf("lift: %d %v", res.StatusCode, body)
	}
	if _, first, code := dialRaw(t, ts, tok); code != -1 || first["t"] != "ready" {
		t.Errorf("connect after the lift: frame %v code %d", first, code)
	}
	if res, _ := do(t, ts, "GET", "/api/v1/users/@me/channels", tok, nil); res.StatusCode != 200 {
		t.Errorf("use the app after the lift: %d", res.StatusCode)
	}
}

// A second mute replaces the first; the first case says which one took over.
func TestSecondMuteReplacesTheFirst(t *testing.T) {
	_, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	tok, id := register(t, ts, "alice")
	first := mustAct(t, ts, modTok, id, map[string]any{"action": "mute", "reason": "first", "duration_ms": 3600000})
	second := mustAct(t, ts, modTok, id, map[string]any{"action": "mute", "reason": "second", "duration_ms": 7200000})
	st := meNexa(t, ts, tok)
	if st["mute"].(map[string]any)["case_id"] != second["id"] {
		t.Errorf("the mute in force is not the newer one: %v", st["mute"])
	}
	_, thread := do(t, ts, "GET", "/api/v1/nexa/cases/"+first["id"].(string), tok, nil)
	fc := thread["case"].(map[string]any)
	if fc["in_effect"] != false || fc["ended"] != true {
		t.Errorf("replaced case: %v", fc)
	}
	lines := thread["messages"].([]any)
	if len(lines) != 1 || lines[0].(map[string]any)["event"] != "replaced" || lines[0].(map[string]any)["body"] != "2" {
		t.Errorf("replaced line: %v", lines)
	}
	if res, body := do(t, ts, "POST", "/api/v1/moderation/cases/"+first["id"].(string)+"/lift", modTok, map[string]any{"reason": "old one"}); res.StatusCode != 409 {
		t.Errorf("lift the replaced mute: %d %v", res.StatusCode, body)
	}
}

func TestCaseThread(t *testing.T) {
	_, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	mod2Tok, mod2ID := register(t, ts, "second")
	if res, _ := do(t, ts, "PATCH", "/api/v1/admin/users/"+mod2ID, modTok, map[string]any{"admin": true}); res.StatusCode != 200 {
		t.Fatal("promote")
	}
	tok, id := register(t, ts, "alice")
	c := mustAct(t, ts, modTok, id, map[string]any{"action": "mute", "reason": "spam", "duration_ms": 3600000})
	cid := c["id"].(string)
	staffGW := connectGW(t, ts, mod2Tok)
	memberGW := connectGW(t, ts, tok)

	if res, body := do(t, ts, "POST", "/api/v1/nexa/cases/"+cid+"/messages", tok, map[string]any{"body": "   "}); errCode(body) != "empty_message" {
		t.Errorf("empty reply: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "POST", "/api/v1/nexa/cases/"+cid+"/messages", tok, map[string]any{"body": strings.Repeat("x", 2001)}); errCode(body) != "too_long" {
		t.Errorf("long reply: %d %v", res.StatusCode, body)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/nexa/cases/"+cid+"/messages", tok, map[string]any{"body": "why?"}); res.StatusCode != 201 {
		t.Fatal("member reply")
	}
	// any member of staff sees it land, waiting on them
	ev := staffGW.next("mod_case", 3*time.Second)
	if ev == nil || ev["case"].(map[string]any)["awaiting"] != "staff" {
		t.Fatalf("staff event: %v", ev)
	}
	// and any of them can answer: it shows as Nexa, with their name
	res, body := do(t, ts, "POST", "/api/v1/moderation/cases/"+cid+"/messages", mod2Tok, map[string]any{"body": "Because of the spam."})
	if res.StatusCode != 201 {
		t.Fatalf("staff reply: %d %v", res.StatusCode, body)
	}
	if body["message"].(map[string]any)["author_id"] != mod2ID || body["message"].(map[string]any)["kind"] != "staff" {
		t.Errorf("staff line: %v", body["message"])
	}
	// their own line came back to them first (for their other tabs); the
	// staff reply is the event after it
	var mev map[string]any
	for mev == nil {
		ev := memberGW.next("nexa_case", 3*time.Second)
		if ev == nil {
			t.Fatal("the member did not hear the reply")
		}
		for _, m := range ev["messages"].([]any) {
			if m.(map[string]any)["kind"] == "staff" {
				mev = ev
			}
		}
	}
	people := mev["people"].(map[string]any)
	if people[mod2ID] == nil {
		t.Errorf("the reply does not say who wrote it: %v", mev)
	}
	if mev["case"].(map[string]any)["unread"].(float64) != 1 || mev["state"].(map[string]any)["unread"].(float64) < 1 {
		t.Errorf("unread: %v", mev)
	}
	_, thread := do(t, ts, "GET", "/api/v1/nexa/cases/"+cid, tok, nil)
	msgs := thread["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)["id"].(string)
	if res, _ := do(t, ts, "POST", "/api/v1/nexa/cases/"+cid+"/read", tok, map[string]any{"message_id": last}); res.StatusCode != 204 {
		t.Error("read")
	}
	_, thread = do(t, ts, "GET", "/api/v1/nexa/cases/"+cid, tok, nil)
	if thread["case"].(map[string]any)["unread"].(float64) != 0 {
		t.Errorf("still unread: %v", thread["case"])
	}
	// a closed case is closed to everyone: the member cannot write in it or
	// open it again by writing, and neither can staff until they reopen it
	if res, body := do(t, ts, "POST", "/api/v1/moderation/cases/"+cid+"/close", modTok, nil); res.StatusCode != 200 || body["case"].(map[string]any)["status"] != "closed" {
		t.Fatalf("close: %d %v", res.StatusCode, body)
	}
	res, body = do(t, ts, "POST", "/api/v1/nexa/cases/"+cid+"/messages", tok, map[string]any{"body": "one more thing"})
	if res.StatusCode != 409 || errCode(body) != "case_closed" {
		t.Errorf("member reply on a closed case: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "POST", "/api/v1/moderation/cases/"+cid+"/messages", mod2Tok, map[string]any{"body": "and another"}); res.StatusCode != 409 || errCode(body) != "case_closed" {
		t.Errorf("staff reply on a closed case: %d %v", res.StatusCode, body)
	}
	_, thread = do(t, ts, "GET", "/api/v1/nexa/cases/"+cid, tok, nil)
	if thread["case"].(map[string]any)["status"] != "closed" || len(thread["messages"].([]any)) != len(msgs)+1 {
		t.Errorf("the refused lines changed the case: %v", thread)
	}
	// staff open it again on purpose, and then both can write
	if res, body := do(t, ts, "POST", "/api/v1/moderation/cases/"+cid+"/reopen", modTok, nil); res.StatusCode != 200 || body["case"].(map[string]any)["status"] != "open" {
		t.Fatalf("reopen: %d %v", res.StatusCode, body)
	}
	res, body = do(t, ts, "POST", "/api/v1/nexa/cases/"+cid+"/messages", tok, map[string]any{"body": "one more thing"})
	if res.StatusCode != 201 || body["case"].(map[string]any)["status"] != "open" || len(body["messages"].([]any)) != 1 {
		t.Errorf("reply on a reopened case: %d %v", res.StatusCode, body)
	}
	// the staff's view of the user
	_, user := do(t, ts, "GET", "/api/v1/moderation/users/"+id, modTok, nil)
	if len(user["cases"].([]any)) != 1 || user["user"].(map[string]any)["mute"] == nil || user["user"].(map[string]any)["awaiting_cases"].(float64) != 1 {
		t.Errorf("user view: %v", user)
	}
}

func TestModLogCannotBeRewritten(t *testing.T) {
	srv, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	_, id := register(t, ts, "alice")
	mustAct(t, ts, modTok, id, map[string]any{"action": "mute", "reason": "spam", "duration_ms": 3600000})
	for _, q := range []string{`UPDATE mod_log SET reason = 'nothing happened'`, `DELETE FROM mod_log`} {
		if _, err := srv.Store.DB().W.Exec(q); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: %v", q, err)
		}
	}
	entries, err := srv.Store.ModLog(t.Context(), store.ModLogFilter{})
	if err != nil || len(entries) != 1 || entries[0].Reason != "spam" || entries[0].ModeratorUsername != "owner" || entries[0].TargetUsername != "alice" {
		t.Errorf("log after the attempts: %v %+v", err, entries)
	}
}

func TestAnnouncements(t *testing.T) {
	srv, ts := newTestServer(t)
	modTok, modID := register(t, ts, "owner")
	tok, _ := register(t, ts, "alice")
	g := connectGW(t, ts, tok)
	if res, body := do(t, ts, "POST", "/api/v1/moderation/announcements", modTok, map[string]any{"title": "Hi", "body": ""}); errCode(body) != "empty_message" {
		t.Errorf("empty: %d %v", res.StatusCode, body)
	}
	res, body := do(t, ts, "POST", "/api/v1/moderation/announcements", modTok, map[string]any{"title": "Maintenance", "body": "We'll be down **briefly** tonight."})
	if res.StatusCode != 201 {
		t.Fatalf("announce: %d %v", res.StatusCode, body)
	}
	aid := body["announcement"].(map[string]any)["id"].(string)
	ev := g.next("nexa_announcement", 3*time.Second)
	if ev == nil || ev["announcement"].(map[string]any)["author_id"] != modID || ev["people"].(map[string]any)[modID] == nil {
		t.Fatalf("announcement event names its author: %v", ev)
	}
	_, feed := do(t, ts, "GET", "/api/v1/nexa/feed", tok, nil)
	items := feed["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["kind"] != "announcement" {
		t.Errorf("feed: %v", feed)
	}
	if st := meNexa(t, ts, tok); st["unread"].(float64) != 1 || st["latest"].(float64) == 0 {
		t.Errorf("unread announcement: %v", st)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/nexa/seen", tok, nil); res.StatusCode != 204 {
		t.Error("seen")
	}
	if st := meNexa(t, ts, tok); st["unread"].(float64) != 0 {
		t.Errorf("unread after seen: %v", st)
	}
	// someone who joins afterwards never had it
	lateTok, _ := register(t, ts, "late")
	_, feed = do(t, ts, "GET", "/api/v1/nexa/feed", lateTok, nil)
	if len(feed["items"].([]any)) != 0 {
		t.Errorf("a later account sees an earlier announcement: %v", feed)
	}
	// retracting takes it back from everyone, and the log keeps both
	if res, _ := do(t, ts, "DELETE", "/api/v1/moderation/announcements/"+aid, modTok, nil); res.StatusCode != 204 {
		t.Fatal("retract")
	}
	if g.next("nexa_announcement_delete", 3*time.Second) == nil {
		t.Error("no retraction event")
	}
	_, feed = do(t, ts, "GET", "/api/v1/nexa/feed", tok, nil)
	if len(feed["items"].([]any)) != 0 {
		t.Errorf("retracted announcement still shows: %v", feed)
	}
	entries, _ := srv.Store.ModLog(t.Context(), store.ModLogFilter{})
	if len(entries) != 2 || entries[0].Action != "announcement_delete" || entries[1].Action != "announcement" {
		t.Errorf("log: %+v", entries)
	}
}

func TestDeleteAccountTakesEverything(t *testing.T) {
	srv, ts := newTestServer(t)
	ctx := t.Context()
	modTok, _ := register(t, ts, "owner")
	vicTok, vicID := register(t, ts, "victim")
	bobTok, bobID := register(t, ts, "bob")
	carolTok, carolID := register(t, ts, "carol")
	befriend(t, ts, vicTok, bobTok, "bob", vicID)
	befriend(t, ts, vicTok, carolTok, "carol", vicID)
	// a server of theirs with bob in it, and bob's server with them in it
	mine := createServer(t, ts, vicTok, "victim's place")["id"].(string)
	if _, err := srv.Store.AddMember(ctx, modID(mine), modID(bobID)); err != nil {
		t.Fatal(err)
	}
	theirs := createServer(t, ts, bobTok, "bob's place")["id"].(string)
	if _, err := srv.Store.AddMember(ctx, modID(theirs), modID(vicID)); err != nil {
		t.Fatal(err)
	}
	// a group of theirs, and a DM with bob
	_, grp := do(t, ts, "POST", "/api/v1/users/@me/channels", vicTok, map[string]any{"recipient_ids": []string{bobID, carolID}, "name": "the gang"})
	group := grp["id"].(string)
	_, d := do(t, ts, "POST", "/api/v1/users/@me/channels", vicTok, map[string]any{"recipient_ids": []string{bobID}})
	dm := d["id"].(string)
	createEpoch(t, ts, vicTok, dm)
	for i := 0; i < 3; i++ {
		if res, body := sendMsg(t, ts, vicTok, dm, dm, 0, "from victim", "", nil); res.StatusCode != 201 {
			t.Fatalf("send: %d %v", res.StatusCode, body)
		}
	}
	_, b := sendMsg(t, ts, bobTok, dm, dm, 0, "from bob", "", nil)
	bobMsg := b["id"].(string)
	if res, _ := do(t, ts, "PUT", "/api/v1/channels/"+dm+"/messages/"+bobMsg+"/reactions/%F0%9F%91%8D/@me", vicTok, nil); res.StatusCode != 204 {
		t.Fatal("react")
	}
	bobGW := connectGW(t, ts, bobTok)
	vicGW := connectGW(t, ts, vicTok)

	_, preview := do(t, ts, "GET", "/api/v1/moderation/users/"+vicID+"/purge-preview", modTok, nil)
	if preview["messages"].(float64) < 3 || preview["owned_servers"].(float64) != 1 || preview["owned_groups"].(float64) != 1 || preview["reactions"].(float64) != 1 {
		t.Errorf("preview: %v", preview)
	}
	if res, body := modAct(t, ts, modTok, vicID, map[string]any{"action": "delete", "reason": "illegal content", "confirm": "Victim"}); errCode(body) != "confirm_mismatch" {
		t.Errorf("wrong confirmation: %d %v", res.StatusCode, body)
	}
	c := mustAct(t, ts, modTok, vicID, map[string]any{"action": "delete", "reason": "illegal content", "confirm": "victim"})
	if c["status"] != "closed" || c["details"].(map[string]any)["complete"] != true {
		t.Errorf("delete case: %v", c)
	}
	// they were told why, then signed out
	if vicGW.next("nexa_notice", 3*time.Second) == nil {
		t.Error("the deleted member was not told")
	}
	if res, _ := do(t, ts, "GET", "/api/v1/users/@me", vicTok, nil); res.StatusCode != 401 {
		t.Errorf("their session still works: %d", res.StatusCode)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "victim", "auth_key": b64(key32)}); res.StatusCode != 401 {
		t.Errorf("they can still sign in: %d", res.StatusCode)
	}
	u, _ := srv.Store.UserByID(ctx, modID(vicID))
	if u.Flags&store.FlagDeleted == 0 || u.Username == "victim" || u.DisplayName != "Deleted User" {
		t.Errorf("account row: %+v", u)
	}
	// their server is gone, and bob heard; they left bob's
	if _, err := srv.Store.ServerByID(ctx, modID(mine)); err == nil {
		t.Error("their server survived")
	}
	if ok, _ := srv.Store.IsMember(ctx, modID(theirs), modID(vicID)); ok {
		t.Error("still in bob's server")
	}
	if _, err := srv.Store.ChannelByID(ctx, modID(group)); err == nil {
		t.Error("their group survived")
	}
	deletes := map[string]bool{}
	for time.Now().Before(time.Now().Add(time.Second)) {
		ev := bobGW.next("server_delete", 500*time.Millisecond)
		if ev == nil {
			break
		}
		deletes[ev["id"].(string)] = true
	}
	if !deletes[mine] {
		t.Errorf("bob was not told the server went: %v", deletes)
	}
	// the DM stays for bob, with his words and none of theirs
	var n int
	_ = srv.Store.DB().R.QueryRow(`SELECT count(*) FROM messages WHERE author_id = ?`, modID(vicID)).Scan(&n)
	if n != 0 {
		t.Errorf("%d of their messages survived", n)
	}
	if _, err := srv.Store.MessageByID(ctx, modID(bobMsg), modID(bobID)); err != nil {
		t.Errorf("bob's own message went too: %v", err)
	}
	_ = srv.Store.DB().R.QueryRow(`SELECT count(*) FROM reactions WHERE user_id = ?`, modID(vicID)).Scan(&n)
	if n != 0 {
		t.Errorf("their reactions survived: %d", n)
	}
	// the record stays
	_, logBody := do(t, ts, "GET", "/api/v1/moderation/log?target="+vicID, modTok, nil)
	if es := logBody["entries"].([]any); len(es) != 1 || es[0].(map[string]any)["target_username"] != "victim" {
		t.Errorf("log: %v", logBody)
	}
	if res, body := modAct(t, ts, modTok, vicID, map[string]any{"action": "mute", "reason": "too late", "duration_ms": 60000}); errCode(body) != "moderate_deleted" {
		t.Errorf("act on a deleted account: %d %v", res.StatusCode, body)
	}
}

// Every route the restriction gate names must be one the server serves: a
// typo there would quietly shut a muted or banned account out of it.
func TestRestrictionAllowlistsNameRealRoutes(t *testing.T) {
	srv, _ := newTestServer(t)
	mux := http.NewServeMux()
	srv.routes(mux)
	param := regexp.MustCompile(`\{[a-z]+\}`)
	for name, list := range map[string]map[string]bool{"banAllowed": banAllowed, "muteAllowed": muteAllowed} {
		for pattern := range list {
			method, path, _ := strings.Cut(pattern, " ")
			req := httptest.NewRequest(method, param.ReplaceAllString(path, "123"), nil)
			if _, got := mux.Handler(req); got != pattern {
				t.Errorf("%s names %q, which the server routes to %q", name, pattern, got)
			}
		}
	}
}

func TestLegalReportListsInstanceCases(t *testing.T) {
	_, ts := newTestServer(t)
	modTok, _ := register(t, ts, "owner")
	_, id := register(t, ts, "alice")
	mustAct(t, ts, modTok, id, map[string]any{"action": "mute", "reason": "rude words", "duration_ms": 3600000})
	res, body := do(t, ts, "GET", "/api/v1/admin/legal-report?user=alice", modTok, nil)
	if res.StatusCode != 200 {
		t.Fatalf("%d %v", res.StatusCode, body)
	}
	doc := body["document"].(string)
	for _, want := range []string{"Nexa cases on them:", "case #1 (mute)", "rude words", "append-only"} {
		if !strings.Contains(doc, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	if strings.Contains(doc, "no Nexa-wide moderation log") {
		t.Error("report still says there is no Nexa-wide moderation log")
	}
}

// modID parses an id the test already knows is one.
func modID(s string) ids.ID {
	id, err := ids.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

// The grid says how each account looks right now: online with what they are
// doing, offline when they are away or have chosen to look it, and the Online
// filter keeps to the first.
func TestModGridPresence(t *testing.T) {
	_, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "owner")
	songTok, songID := register(t, ts, "listener")
	hidTok, hidID := register(t, ts, "hidden")
	_, awayID := register(t, ts, "away")
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", songTok, map[string]any{"status_text": "on repeat", "status_emoji": "🎧"}); res.StatusCode != 200 {
		t.Fatalf("status: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", hidTok, map[string]any{"presence_pref": "invisible"}); res.StatusCode != 200 {
		t.Fatalf("invisible: %d %v", res.StatusCode, body)
	}
	song := connectGW(t, ts, songTok)
	connectGW(t, ts, hidTok)
	b, _ := json.Marshal(map[string]any{"op": "activities", "d": map[string]any{"activities": []map[string]any{{"name": "Spotify", "type": 2, "details": "Never Gonna Give You Up", "state": "Rick Astley"}}}})
	if err := song.ws.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}

	presence := func(show string) map[string]map[string]any {
		t.Helper()
		res, body := do(t, ts, "GET", "/api/v1/moderation/users?show="+show, ownTok, nil)
		if res.StatusCode != 200 {
			t.Fatalf("grid %s: %d %v", show, res.StatusCode, body)
		}
		out := map[string]map[string]any{}
		for _, u := range body["users"].([]any) {
			m := u.(map[string]any)
			p, _ := m["presence"].(map[string]any)
			p["status_text"] = m["status_text"]
			out[m["id"].(string)] = p
		}
		return out
	}
	// activities are resolved a moment after they arrive
	var all map[string]map[string]any
	for i := 0; i < 50; i++ {
		all = presence("all")
		if acts, _ := all[songID]["activities"].([]any); len(acts) == 1 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if p := all[songID]; p["status"] != "online" || p["status_text"] != "on repeat" {
		t.Fatalf("listener: %v", p)
	}
	if acts, _ := all[songID]["activities"].([]any); len(acts) != 1 || acts[0].(map[string]any)["state"] != "Rick Astley" {
		t.Fatalf("listener's song: %v", all[songID])
	}
	if p := all[hidID]; p["status"] != "offline" || p["activities"] != nil {
		t.Fatalf("invisible looks online: %v", p)
	}
	if p := all[awayID]; p["status"] != "offline" {
		t.Fatalf("away: %v", p)
	}

	online := presence("online")
	if _, ok := online[songID]; !ok || len(online) != 1 {
		t.Fatalf("online filter: %v", online)
	}

	// one account's page carries it too
	res, body := do(t, ts, "GET", "/api/v1/moderation/users/"+songID, ownTok, nil)
	if res.StatusCode != 200 {
		t.Fatalf("user: %d %v", res.StatusCode, body)
	}
	if p, _ := body["user"].(map[string]any)["presence"].(map[string]any); p["status"] != "online" {
		t.Fatalf("user presence: %v", body["user"])
	}
}

// Accounts used from the same address show as each other's alts, in the grid
// and in the full list; a deleted one drops out of both.
func TestModerationAltAccounts(t *testing.T) {
	srv, ts := newTestServer(t)
	ownTok, _ := register(t, ts, "owner")
	_, aID := register(t, ts, "alta")
	_, bID := register(t, ts, "altb")
	_, cID := register(t, ts, "altc")
	// the test server sees every request from one address; give c its own
	if _, err := srv.Store.DB().W.Exec(`DELETE FROM user_ips WHERE user_id = ?`, modID(cID)); err != nil {
		t.Fatal(err)
	}
	_ = srv.Store.RecordUserIP(context.Background(), modID(cID), "192.0.2.77", 1)
	_, body := do(t, ts, "GET", "/api/v1/moderation/users?show=all", ownTok, nil)
	counts := map[string]float64{}
	for _, u := range body["users"].([]any) {
		m := u.(map[string]any)
		counts[m["id"].(string)] = m["alt_count"].(float64)
	}
	if counts[aID] != 2 || counts[bID] != 2 || counts[cID] != 0 {
		t.Fatalf("alt counts: %v", counts)
	}
	res, body := do(t, ts, "GET", "/api/v1/moderation/users/"+aID+"/alts", ownTok, nil)
	alts, _ := body["alts"].([]any)
	if res.StatusCode != 200 || len(alts) != 2 {
		t.Fatalf("alts of a: %d %v", res.StatusCode, body)
	}
	found := false
	for _, a := range alts {
		m := a.(map[string]any)
		if m["user"].(map[string]any)["id"] == bID && m["shared_ips"].(float64) == 1 {
			found = true
		}
		if _, leaked := m["ip"]; leaked {
			t.Error("the address itself is shown")
		}
	}
	if !found {
		t.Errorf("b missing from a's alts: %v", alts)
	}
	_, body = do(t, ts, "GET", "/api/v1/moderation/users/"+aID, ownTok, nil)
	if n := body["user"].(map[string]any)["alt_count"]; n != float64(2) {
		t.Errorf("detail alt_count %v", n)
	}
}

// A name staff took down must not come free when the account goes, and a
// one-character name takes a gift, once.
func TestModeratedNamesStayTakenAndShortNamesTakeAGift(t *testing.T) {
	srv, ts := newTestServer(t)
	ctx := t.Context()
	modTok, _ := register(t, ts, "owner")
	_, vicID := register(t, ts, "vandal")
	mustAct(t, ts, modTok, vicID, map[string]any{"action": "delete", "reason": "illegal content", "confirm": "vandal"})
	if free, err := srv.Store.UsernameFree(ctx, "vandal"); err != nil || free {
		t.Fatalf("the name of an account staff deleted is free again: %v %v", free, err)
	}

	tok, id := register(t, ts, "shorty")
	name := map[string]any{"username": "x"}
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, name); errCode(body) != "short_name_gift" {
		t.Fatalf("no gift: %d %v", res.StatusCode, body)
	}
	_, made := do(t, ts, "POST", "/api/v1/moderation/gift-links", modTok, map[string]any{"plan": store.GiftShortName})
	code, _ := made["code"].(string)
	if code == "" {
		t.Fatalf("make the link: %v", made)
	}
	if res, body := do(t, ts, "POST", "/api/v1/gifts/"+code+"/claim", tok, nil); res.StatusCode != 200 {
		t.Fatalf("claim: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, name); res.StatusCode != 200 || body["username"] != "x" || body["short_name_credits"].(float64) != 0 {
		t.Fatalf("use the gift: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, map[string]any{"username": "y"}); errCode(body) != "short_name_gift" {
		t.Fatalf("a second name from one gift: %d %v", res.StatusCode, body)
	}
	if _, err := srv.Store.UserByID(ctx, modID(id)); err != nil {
		t.Fatal(err)
	}
}
