package api

import (
	"context"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// manageFixture is a server with everything an indirect way to its private
// channels could use, with the owner of Nexa joined as an ordinary member.
type manageFixture struct {
	srv                                     *Server
	ts                                      *httptest.Server
	nexa, nexaID, srvOwner, srvOwnerID, sid string
	general, secret, half, closedCat, inner string
	openCat, closedInOpen                   string
	aliceID, stafferID, altID, botID        string
	plainRole, deniedRole, secretRole       string
	adminRole                               string
}

func manageSetup(t *testing.T) *manageFixture {
	t.Helper()
	srv, ts := newTestServer(t)
	f := &manageFixture{srv: srv, ts: ts}
	f.nexa, f.nexaID = register(t, ts, "nexaowner")
	f.srvOwner, f.srvOwnerID = register(t, ts, "srvowner")
	view := createServer(t, ts, f.srvOwner, "Place")
	f.sid = view["id"].(string)
	f.general = channelNamed(view, "general")["id"].(string)
	code := inviteFor(t, ts, f.srvOwner, f.general)

	aliceTok, aliceID := register(t, ts, "alice")
	stafferTok, stafferID := register(t, ts, "staffer")
	altTok, altID := register(t, ts, "altacct")
	f.aliceID, f.stafferID, f.altID = aliceID, stafferID, altID
	setRank(t, ts, f.nexa, stafferID, "helper")
	for _, tok := range []string{f.nexa, aliceTok, stafferTok, altTok} {
		join(t, ts, tok, code)
	}
	app, _ := makeApp(t, ts, aliceTok, "Reader")
	f.botID = app["id"].(string)
	q := url.Values{"client_id": {f.botID}, "scope": {"bot applications.commands"}, "permissions": {"0"}}
	if res, body := do(t, ts, "POST", "/api/v1/oauth2/authorize", f.srvOwner, map[string]any{"query": q.Encode(), "guild_id": f.sid}); res.StatusCode != 200 {
		t.Fatalf("add bot: %d %v", res.StatusCode, body)
	}

	channel := func(name, typ string, parent string) string {
		body := map[string]any{"type": typ, "name": name}
		if parent != "" {
			body["parent_id"] = parent
		}
		res, out := do(t, ts, "POST", "/api/v1/servers/"+f.sid+"/channels", f.srvOwner, body)
		if res.StatusCode != 201 {
			t.Fatalf("channel %s: %d %v", name, res.StatusCode, out)
		}
		return out["id"].(string)
	}
	overwrite := func(ch, target string, allow, deny perms.Perm) {
		res, out := do(t, ts, "PUT", "/api/v1/channels/"+ch+"/permissions/"+target, f.srvOwner, map[string]any{"type": "role", "allow": permStr(allow), "deny": permStr(deny)})
		if res.StatusCode != 200 {
			t.Fatalf("overwrite: %d %v", res.StatusCode, out)
		}
	}
	give := func(uid, role string) {
		if res, out := do(t, ts, "PUT", "/api/v1/servers/"+f.sid+"/members/"+uid+"/roles/"+role, f.srvOwner, nil); res.StatusCode != 200 {
			t.Fatalf("give role: %d %v", res.StatusCode, out)
		}
	}
	f.secret = channel("secret", "text", "")
	overwrite(f.secret, f.sid, 0, perms.ViewChannel)
	f.half = channel("half", "text", "")
	f.openCat = channel("open", "category", "")
	f.closedCat = channel("closed", "category", "")
	overwrite(f.closedCat, f.sid, 0, perms.ViewChannel)
	f.inner = channel("inner", "text", f.closedCat)
	f.closedInOpen = channel("closedinopen", "text", f.openCat)
	overwrite(f.closedInOpen, f.sid, 0, perms.ViewChannel)

	f.plainRole = createRole(t, ts, f.srvOwner, f.sid, "plain", 0)
	f.deniedRole = createRole(t, ts, f.srvOwner, f.sid, "denied", 0)
	f.secretRole = createRole(t, ts, f.srvOwner, f.sid, "secretrole", 0)
	f.adminRole = createRole(t, ts, f.srvOwner, f.sid, "adminrole", perms.Administrator)
	overwrite(f.secret, f.secretRole, perms.ViewChannel, 0)
	overwrite(f.half, f.deniedRole, 0, perms.ViewChannel)
	give(f.nexaID, f.deniedRole)
	give(f.nexaID, f.plainRole)

	// every test account shares the test client's address; only these two are linked
	ctx := context.Background()
	if _, err := srv.Store.DB().W.Exec(`DELETE FROM user_ips`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{f.nexaID, f.altID} {
		_ = srv.Store.RecordUserIP(ctx, modID(id), "192.0.2.77", 1)
	}
	return f
}

// quiet forgets the test client's address, which each request records again.
func (f *manageFixture) quiet(t *testing.T) {
	t.Helper()
	if _, err := f.srv.Store.DB().W.Exec(`DELETE FROM user_ips WHERE ip != '192.0.2.77'`); err != nil {
		t.Fatal(err)
	}
}

func (f *manageFixture) manage(t *testing.T, tok string, body map[string]any) (int, map[string]any) {
	t.Helper()
	f.quiet(t)
	if _, ok := body["reason"]; !ok {
		body["reason"] = "looking after this server"
	}
	res, out := do(t, f.ts, "POST", "/api/v1/moderation/servers/"+f.sid+"/manage", tok, body)
	return res.StatusCode, out
}

func (f *manageFixture) auditCount(t *testing.T) int {
	t.Helper()
	list, err := f.srv.Store.AuditEntries(context.Background(), modID(f.sid), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range list {
		if strings.Contains(string(e.Changes), "nexa_owner") {
			n++
		}
	}
	return n
}

func (f *manageFixture) logged(t *testing.T, action string) *store.ModLogEntry {
	t.Helper()
	list, err := f.srv.Store.ModLog(context.Background(), store.ModLogFilter{Actions: []string{action}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

func (f *manageFixture) loggedCount(t *testing.T, action string) int {
	t.Helper()
	list, err := f.srv.Store.ModLog(context.Background(), store.ModLogFilter{Actions: []string{action}})
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

func (f *manageFixture) holdsRole(t *testing.T, uid, role string) bool {
	t.Helper()
	m, err := f.srv.Store.Member(context.Background(), modID(f.sid), modID(uid))
	if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(m.RoleIDs, modID(role))
}

func allow(p perms.Perm) map[string]any {
	return map[string]any{"type": "role", "allow": permStr(p), "deny": "0"}
}

// Nexa's owner holds every permission in a server they are in, so what used
// to be refused as a way into a private channel now goes through, and each
// change is in both logs (the owner's decision, 2026-11-17).
func TestOwnerManageOpensWhatItIsAsked(t *testing.T) {
	f := manageSetup(t)
	if res, _ := do(t, f.ts, "GET", "/api/v1/channels/"+f.secret+"/messages", f.nexa, nil); res.StatusCode != 200 {
		t.Errorf("Nexa's owner cannot read the private channel: %d", res.StatusCode)
	}
	steps := []map[string]any{
		{"action": "overwrite_set", "channel_id": f.secret, "target_id": f.plainRole, "data": allow(perms.ViewChannel)},
		{"action": "member_role_add", "user_id": f.nexaID, "role_id": f.adminRole},
		{"action": "role_edit", "role_id": f.plainRole, "data": map[string]any{"permissions": permStr(perms.ManageServer)}},
		{"action": "channel_move", "channel_id": f.secret, "data": map[string]any{"parent_id": f.openCat}},
	}
	for _, body := range steps {
		if code, out := f.manage(t, f.nexa, body); code != 200 {
			t.Errorf("%v: %d %v", body["action"], code, out)
		}
	}
	if n := f.auditCount(t); n != len(steps) {
		t.Errorf("audit entries: %d, want %d", n, len(steps))
	}
	if n := f.loggedCount(t, store.LogOwnerServerBegin); n != len(steps) {
		t.Errorf("attempt lines: %d, want %d", n, len(steps))
	}
}

// Only what is on the list exists, only on this server, only for the owner.
func TestOwnerManageLimits(t *testing.T) {
	f := manageSetup(t)
	other := createServer(t, f.ts, f.srvOwner, "Elsewhere")
	otherGeneral := channelNamed(other, "general")["id"].(string)
	otherRole := createRole(t, f.ts, f.srvOwner, other["id"].(string), "theirs", 0)

	for _, action := range []string{"app_add", "message_read", "keys_rotate", "message_delete", "join", ""} {
		if code, out := f.manage(t, f.nexa, map[string]any{"action": action, "app_id": f.botID}); code != 400 || errCode(out) != "unknown_action" {
			t.Errorf("action %q: %d %v", action, code, out)
		}
	}
	if code, _ := f.manage(t, f.nexa, map[string]any{"action": "channel_delete", "channel_id": otherGeneral}); code != 404 {
		t.Errorf("another server's channel: %d", code)
	}
	if code, _ := f.manage(t, f.nexa, map[string]any{"action": "role_delete", "role_id": otherRole}); code != 404 {
		t.Errorf("another server's role: %d", code)
	}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "server_edit", "reason": " ", "data": map[string]any{"name": "New"}}); code != 400 || errCode(out) != "reason_required" {
		t.Errorf("no reason: %d %v", code, out)
	}
	devTok, devID := register(t, f.ts, "adeveloper")
	setRank(t, f.ts, f.nexa, devID, "developer")
	if code, _ := f.manage(t, devTok, map[string]any{"action": "server_edit", "data": map[string]any{"name": "New"}}); code != 403 {
		t.Errorf("a developer: %d", code)
	}
	if res, _ := do(t, f.ts, "GET", "/api/v1/moderation/servers/"+f.sid+"/manage", devTok, nil); res.StatusCode != 403 {
		t.Errorf("a developer opened the page: %d", res.StatusCode)
	}
	// a field the panel does not offer never reaches the handler
	if code, _ := f.manage(t, f.nexa, map[string]any{"action": "server_edit", "data": map[string]any{"name": "Kept", "owner_id": f.nexaID, "discovery_hidden": true}}); code != 200 {
		t.Errorf("server_edit: %d", code)
	}
	got, _ := f.srv.Store.ServerByID(context.Background(), modID(f.sid))
	if got.Name != "Kept" || got.OwnerID.String() != f.srvOwnerID {
		t.Errorf("server after edit: %q owner %v", got.Name, got.OwnerID)
	}
	// a private channel's topic can be replaced like any other
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "channel_edit", "channel_id": f.secret, "data": map[string]any{"topic": "x"}}); code != 200 {
		t.Errorf("private topic: %d %v", code, out)
	}
}

// A ban is in both logs, and nobody gets a case or a notice about it: the
// server's audit log is the record (the owner's decision, 2026-11-17).
func TestOwnerManageIsVisible(t *testing.T) {
	f := manageSetup(t)
	ctx := context.Background()
	code, out := f.manage(t, f.nexa, map[string]any{"action": "member_ban", "user_id": f.aliceID, "reason": "broke the rules", "data": map[string]any{"delete_message_days": 7}})
	if code != 200 {
		t.Fatalf("ban: %d %v", code, out)
	}
	if strings.Contains(strings.ToLower(mustString(out)), "key_actions") {
		t.Error("a handler's key actions reached the panel")
	}
	list, _ := f.srv.Store.AuditEntries(ctx, modID(f.sid), 0, 5)
	if len(list) == 0 || list[0].Action != "member_ban" || list[0].ActorID.String() != f.nexaID || list[0].Reason != "broke the rules" || !strings.Contains(string(list[0].Changes), `"nexa_owner":true`) || !strings.Contains(string(list[0].Changes), `"before"`) {
		t.Fatalf("server's log: %+v", list[0])
	}
	if e := f.logged(t, store.LogOwnerServerPrefix+"member_ban"); e == nil || e.Reason != "broke the rules" || !strings.Contains(string(e.Details), `"status":"done"`) {
		t.Fatalf("moderation log: %+v", e)
	}
	for _, who := range []string{f.srvOwnerID, f.aliceID} {
		if cases, _ := f.srv.Store.ModCasesForUser(ctx, modID(who)); len(cases) != 0 {
			t.Errorf("%s was sent a case: %+v", who, cases)
		}
	}
	// a change that fails leaves the server's log and everyone's notices alone
	before := f.auditCount(t)
	if code, _ := f.manage(t, f.nexa, map[string]any{"action": "server_edit", "data": map[string]any{"default_notifications": 9}}); code != 400 {
		t.Fatalf("bad value: %d", code)
	}
	if f.auditCount(t) != before {
		t.Error("a failed change stayed in the server's log")
	}
	if e := f.logged(t, store.LogOwnerServerPrefix+"server_edit"); e == nil || !strings.Contains(string(e.Details), `"status":"failed"`) {
		t.Errorf("a failed change in the moderation log: %+v", e)
	}
}

// raceWait is long enough for an unhindered edit to land many times over.
const raceWait = 200 * time.Millisecond

// An ordinary edit that arrives after an action has been checked waits until
// the action is finished. The overwrite is safe only while the owner of Nexa
// does not hold the role, so an edit landing in between would open the channel.
func TestOwnerManageHoldsOrdinaryEditsOff(t *testing.T) {
	f := manageSetup(t)
	later := createRole(t, f.ts, f.srvOwner, f.sid, "later", 0)
	edited := make(chan int, 1)
	f.srv.manageHook = func(stage string) {
		if stage != "validated" {
			return
		}
		go func() {
			res, _ := do(t, f.ts, "PUT", "/api/v1/servers/"+f.sid+"/members/"+f.nexaID+"/roles/"+later, f.srvOwner, nil)
			edited <- res.StatusCode
		}()
		select {
		case code := <-edited:
			edited <- code
			t.Errorf("an ordinary edit landed between the check and the change: %d", code)
		case <-time.After(raceWait):
		}
		if f.holdsRole(t, f.nexaID, later) {
			t.Error("the role was given while the action was under way")
		}
	}
	code, out := f.manage(t, f.nexa, map[string]any{"action": "overwrite_set", "channel_id": f.secret, "target_id": later, "data": allow(perms.ViewChannel)})
	if code != 200 {
		t.Fatalf("action: %d %v", code, out)
	}
	if got := <-edited; got != 200 {
		t.Errorf("the edit that waited: %d", got)
	}
}

// An action arriving during an ordinary edit waits for it, and runs on what
// that edit left.
func TestOwnerManageWaitsForOrdinaryEdits(t *testing.T) {
	f := manageSetup(t)
	ctx := context.Background()
	later := createRole(t, f.ts, f.srvOwner, f.sid, "later", 0)
	held, release, err := f.srv.Store.ShareServerEdits(ctx, modID(f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	type answer struct {
		code int
		out  map[string]any
	}
	done := make(chan answer, 1)
	go func() {
		code, out := f.manage(t, f.nexa, map[string]any{"action": "role_delete", "role_id": later})
		done <- answer{code, out}
	}()
	select {
	case a := <-done:
		t.Fatalf("the action did not wait for the edit: %d %v", a.code, a.out)
	case <-time.After(raceWait):
	}
	if err := f.srv.Store.AddMemberRole(held, modID(f.sid), modID(f.aliceID), modID(later)); err != nil {
		t.Fatal(err)
	}
	release()
	if a := <-done; a.code != 200 {
		t.Fatalf("after the edit: %d %v", a.code, a.out)
	}
	if f.holdsRole(t, f.aliceID, later) {
		t.Error("the role the edit gave outlived its deletion")
	}
}

// The server's log has a change only once it has committed, and a failed one
// never; nobody gets a notice; the moderation log keeps every attempt.
func TestOwnerManageRecordsOnlyWhatHappened(t *testing.T) {
	f := manageSetup(t)
	ctx := context.Background()
	notices := func() int {
		cases, err := f.srv.Store.ModCasesForUser(ctx, modID(f.srvOwnerID))
		if err != nil {
			t.Fatal(err)
		}
		return len(cases)
	}
	var atChange bool
	f.srv.manageHook = func(stage string) {
		if stage != "changed" {
			return
		}
		atChange = true
		if got, _ := f.srv.Store.ServerByID(ctx, modID(f.sid)); got.Name != "Renamed" {
			t.Errorf("not committed yet: %q", got.Name)
		}
		if n := f.auditCount(t); n != 0 {
			t.Errorf("the server's log was written before the change: %d", n)
		}
		if n := notices(); n != 0 {
			t.Errorf("a notice was written before the change: %d", n)
		}
		if n := f.loggedCount(t, store.LogOwnerServerBegin); n != 1 {
			t.Errorf("attempt lines at the change: %d", n)
		}
	}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "server_edit", "data": map[string]any{"name": "Renamed"}}); code != 200 {
		t.Fatalf("rename: %d %v", code, out)
	}
	if !atChange {
		t.Fatal("the stage after the change never ran")
	}
	f.srv.manageHook = nil
	if n := f.auditCount(t); n != 1 {
		t.Errorf("audit entries after a change: %d", n)
	}
	if n := notices(); n != 0 {
		t.Errorf("notices after a change: %d", n)
	}
	if e := f.logged(t, store.LogOwnerServerPrefix+"server_edit"); e == nil || !strings.Contains(string(e.Details), `"status":"done"`) {
		t.Errorf("outcome line: %+v", e)
	}

	if code, _ := f.manage(t, f.nexa, map[string]any{"action": "server_edit", "data": map[string]any{"default_notifications": 9}}); code != 400 {
		t.Fatalf("bad value: %d", code)
	}
	if f.auditCount(t) != 1 || notices() != 0 {
		t.Errorf("a failed change was told to the server: audit %d, notices %d", f.auditCount(t), notices())
	}
	if n := f.loggedCount(t, store.LogOwnerServerBegin); n != 2 {
		t.Errorf("attempt lines after a failure: %d", n)
	}
	if e := f.logged(t, store.LogOwnerServerPrefix+"server_edit"); e == nil || !strings.Contains(string(e.Details), `"status":"failed"`) {
		t.Errorf("failed line: %+v", e)
	}
}

func mustString(m map[string]any) string {
	return string(mustJSON(m))
}

// A server changes hands only when its owner is gone, and never to anyone who
// would gain its private channels by being trusted by Nexa.
func TestOwnerManageTransfer(t *testing.T) {
	f := manageSetup(t)
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "transfer", "user_id": f.aliceID}); code != 403 || errCode(out) != "owner_active" {
		t.Fatalf("owner still here: %d %v", code, out)
	}
	mustAct(t, f.ts, f.nexa, f.srvOwnerID, map[string]any{"action": "ban", "reason": "testing the rules", "permanent": true})
	for name, id := range map[string]string{"self": f.nexaID, "staff": f.stafferID, "an app": f.botID, "a linked account": f.altID} {
		if code, out := f.manage(t, f.nexa, map[string]any{"action": "transfer", "user_id": id}); code != 403 || errCode(out) != "owner_access_refused" {
			t.Errorf("transfer to %s: %d %v", name, code, out)
		}
	}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "transfer", "user_id": f.aliceID}); code != 200 {
		t.Fatalf("transfer: %d %v", code, out)
	}
	ctx := context.Background()
	got, _ := f.srv.Store.ServerByID(ctx, modID(f.sid))
	if got.OwnerID.String() != f.aliceID {
		t.Fatalf("owner now %v", got.OwnerID)
	}
	res, out := do(t, f.ts, "GET", "/api/v1/channels/"+f.general+"/messages", f.nexa, nil)
	if res.StatusCode != 200 || !strings.Contains(mustString(out), "system_server_transfer") {
		t.Errorf("no notice in the server: %d %v", res.StatusCode, out)
	}
}

// AutoMod rules that alert can be made and changed like any other.
func TestOwnerManageAutomod(t *testing.T) {
	f := manageSetup(t)
	alert := []map[string]any{{"type": "alert", "channel_id": f.general}}
	words := map[string]any{"words": []string{"nope"}}
	code, out := f.manage(t, f.nexa, map[string]any{"action": "automod_create", "data": map[string]any{"name": "r", "kind": "words", "config": words, "actions": alert}})
	if code != 200 {
		t.Fatalf("new alert rule: %d %v", code, out)
	}
	list, err := f.srv.Store.AutomodRules(context.Background(), modID(f.sid))
	if err != nil || len(list) != 1 {
		t.Fatalf("rules: %v %v", list, err)
	}
	rid := list[0].ID.String()
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "automod_edit", "rule_id": rid, "data": map[string]any{"enabled": true}}); code != 200 {
		t.Errorf("turning on: %d %v", code, out)
	}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "automod_delete", "rule_id": rid}); code != 200 {
		t.Errorf("deleting: %d %v", code, out)
	}
}

// Opening the page is logged first, and shows private channels' topics too.
func TestOwnerManageView(t *testing.T) {
	f := manageSetup(t)
	if res, out := do(t, f.ts, "PATCH", "/api/v1/channels/"+f.secret, f.srvOwner, map[string]any{"topic": "the hidden topic"}); res.StatusCode != 200 {
		t.Fatalf("topic: %d %v", res.StatusCode, out)
	}
	res, out := do(t, f.ts, "GET", "/api/v1/moderation/servers/"+f.sid+"/manage", f.nexa, nil)
	if res.StatusCode != 200 {
		t.Fatalf("view: %d %v", res.StatusCode, out)
	}
	if channelNamed(out, "secret")["topic"] != "the hidden topic" {
		t.Errorf("a private channel's topic: %v", channelNamed(out, "secret")["topic"])
	}
	if channelNamed(out, "secret")["private"] != true {
		t.Error("secret not marked private")
	}
	if f.logged(t, store.LogOwnerServerView) == nil {
		t.Error("opening the page was not logged")
	}
}
