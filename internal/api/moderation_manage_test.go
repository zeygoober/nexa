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

// Every way to reach a channel's messages through a server's settings is
// refused, and refusing changes nothing and tells the server nothing.
func TestOwnerManageRefusesIndirectAccess(t *testing.T) {
	f := manageSetup(t)
	view := perms.ViewChannel
	cases := map[string]map[string]any{
		"private channel made public":                 {"action": "overwrite_set", "channel_id": f.secret, "target_id": f.sid, "data": allow(view)},
		"the deny on a channel removed":               {"action": "overwrite_remove", "channel_id": f.secret, "target_id": f.sid},
		"own role let into a channel":                 {"action": "overwrite_set", "channel_id": f.secret, "target_id": f.plainRole, "data": allow(view)},
		"own role given manage roles":                 {"action": "overwrite_set", "channel_id": f.general, "target_id": f.plainRole, "data": allow(perms.ManageRoles)},
		"staff let in by overwrite":                   {"action": "overwrite_set", "channel_id": f.secret, "target_id": f.stafferID, "data": map[string]any{"type": "member", "allow": permStr(view), "deny": "0"}},
		"app let in by overwrite":                     {"action": "overwrite_set", "channel_id": f.secret, "target_id": f.botID, "data": map[string]any{"type": "member", "allow": permStr(view), "deny": "0"}},
		"linked account let in":                       {"action": "overwrite_set", "channel_id": f.secret, "target_id": f.altID, "data": map[string]any{"type": "member", "allow": permStr(view), "deny": "0"}},
		"category opens a synced channel":             {"action": "overwrite_set", "channel_id": f.closedCat, "target_id": f.sid, "data": allow(view)},
		"administrator on @everyone":                  {"action": "role_edit", "role_id": f.sid, "data": map[string]any{"permissions": permStr(perms.Administrator)}},
		"administrator on a held role":                {"action": "role_edit", "role_id": f.plainRole, "data": map[string]any{"permissions": permStr(perms.Administrator)}},
		"manage server on a held role":                {"action": "role_edit", "role_id": f.plainRole, "data": map[string]any{"permissions": permStr(perms.ManageServer)}},
		"deleting the role that denies":               {"action": "role_delete", "role_id": f.deniedRole},
		"taking off the role that denies":             {"action": "member_role_remove", "user_id": f.nexaID, "role_id": f.deniedRole},
		"role opening a channel for self":             {"action": "member_role_add", "user_id": f.nexaID, "role_id": f.secretRole},
		"administrator role for self":                 {"action": "member_role_add", "user_id": f.nexaID, "role_id": f.adminRole},
		"role opening a channel for staff":            {"action": "member_role_add", "user_id": f.stafferID, "role_id": f.secretRole},
		"role opening a channel for an app":           {"action": "member_role_add", "user_id": f.botID, "role_id": f.secretRole},
		"role opening a channel for a linked account": {"action": "member_role_add", "user_id": f.altID, "role_id": f.secretRole},
		"moved into an open category":                 {"action": "channel_move", "channel_id": f.secret, "data": map[string]any{"parent_id": f.openCat}},
		"synced to an open category":                  {"action": "overwrite_sync", "channel_id": f.closedInOpen},
	}
	for name, body := range cases {
		code, out := f.manage(t, f.nexa, body)
		if code != 403 || errCode(out) != "owner_access_refused" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if n := f.auditCount(t); n != 0 {
		t.Errorf("a refused change reached the server's log: %d", n)
	}
	if f.logged(t, store.LogOwnerServerRefused) == nil {
		t.Error("a refused change left no line in the moderation log")
	}
	// the secret channel is still closed to the owner of Nexa and everyone else
	if res, _ := do(t, f.ts, "GET", "/api/v1/channels/"+f.secret+"/messages", f.nexa, nil); res.StatusCode == 200 {
		t.Error("the owner of Nexa can read the private channel")
	}

	// what opens nothing to anyone guarded still goes through
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "overwrite_set", "channel_id": f.secret, "target_id": f.adminRole, "data": allow(perms.SendMessages)}); code != 200 {
		t.Errorf("a harmless overwrite: %d %v", code, out)
	}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "member_role_add", "user_id": f.aliceID, "role_id": f.secretRole}); code != 200 {
		t.Errorf("giving someone else a role: %d %v", code, out)
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
	// a private channel's topic is neither shown nor replaced
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "channel_edit", "channel_id": f.secret, "data": map[string]any{"topic": "x"}}); code != 403 || errCode(out) != "private_topic" {
		t.Errorf("private topic: %d %v", code, out)
	}
}

// A ban is told to everyone it concerns, in both logs, and keeps the messages.
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
	for who, kind := range map[string]string{f.srvOwnerID: "owner", f.aliceID: "banned"} {
		cases, _ := f.srv.Store.ModCasesForUser(ctx, modID(who))
		if len(cases) != 1 || cases[0].Action != store.ActionServerManage || cases[0].Reason != "broke the rules" || !strings.Contains(string(cases[0].Details), `"notice":"`+kind+`"`) {
			t.Errorf("%s's notice: %+v", kind, cases)
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
	if cases, _ := f.srv.Store.ModCasesForUser(ctx, modID(f.srvOwnerID)); len(cases) != 1 {
		t.Errorf("a failed change left a notice: %d", len(cases))
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

// An action arriving during an ordinary edit waits for it, and is judged by
// what that edit left: here the same overwrite is now one that opens the channel.
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
		code, out := f.manage(t, f.nexa, map[string]any{"action": "overwrite_set", "channel_id": f.secret, "target_id": later, "data": allow(perms.ViewChannel)})
		done <- answer{code, out}
	}()
	select {
	case a := <-done:
		t.Fatalf("the action did not wait for the edit: %d %v", a.code, a.out)
	case <-time.After(raceWait):
	}
	if err := f.srv.Store.AddMemberRole(held, modID(f.sid), modID(f.nexaID), modID(later)); err != nil {
		t.Fatal(err)
	}
	release()
	a := <-done
	if a.code != 403 || errCode(a.out) != "owner_access_refused" {
		t.Fatalf("after the edit: %d %v", a.code, a.out)
	}
	if n := f.auditCount(t); n != 0 {
		t.Errorf("the server's log has a refused change: %d", n)
	}
	if f.logged(t, store.LogOwnerServerRefused) == nil {
		t.Error("the refusal is not in the moderation log")
	}
	ch, err := f.srv.Store.ChannelByID(ctx, modID(f.secret))
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range ch.Overwrites {
		if o.TargetID == modID(later) {
			t.Error("the refused overwrite was written")
		}
	}
}

// The server hears of a change only once it has committed, and of a failed one
// never; the moderation log keeps every attempt.
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
	if n := notices(); n != 1 {
		t.Errorf("notices after a change: %d", n)
	}
	if e := f.logged(t, store.LogOwnerServerPrefix+"server_edit"); e == nil || e.CaseID == nil || !strings.Contains(string(e.Details), `"status":"done"`) {
		t.Errorf("outcome line: %+v", e)
	}

	if code, _ := f.manage(t, f.nexa, map[string]any{"action": "server_edit", "data": map[string]any{"default_notifications": 9}}); code != 400 {
		t.Fatalf("bad value: %d", code)
	}
	if f.auditCount(t) != 1 || notices() != 1 {
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

// AutoMod's alert copies what it catches into a channel the server's key opens.
func TestOwnerManageAutomodCannotCopyMessages(t *testing.T) {
	f := manageSetup(t)
	alert := []map[string]any{{"type": "alert", "channel_id": f.general}}
	block := []map[string]any{{"type": "block"}}
	words := map[string]any{"words": []string{"nope"}}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "automod_create", "data": map[string]any{"name": "r", "kind": "words", "config": words, "actions": alert}}); code != 403 || errCode(out) != "automod_alert" {
		t.Errorf("new alert rule: %d %v", code, out)
	}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "automod_create", "data": map[string]any{"name": "r", "kind": "words", "config": words, "actions": block}}); code != 200 {
		t.Fatalf("new blocking rule: %d %v", code, out)
	}
	res, made := do(t, f.ts, "POST", "/api/v1/servers/"+f.sid+"/automod/rules", f.srvOwner, map[string]any{"name": "theirs", "kind": "words", "config": words, "actions": alert, "enabled": false})
	if res.StatusCode != 201 {
		t.Fatalf("server's own rule: %d %v", res.StatusCode, made)
	}
	rid := made["id"].(string)
	for name, data := range map[string]map[string]any{
		"turned on":  {"enabled": true},
		"edited":     {"name": "renamed"},
		"on and off": {"enabled": false, "name": "renamed"},
	} {
		if code, out := f.manage(t, f.nexa, map[string]any{"action": "automod_edit", "rule_id": rid, "data": data}); code != 403 || errCode(out) != "automod_alert" {
			t.Errorf("alert rule %s: %d %v", name, code, out)
		}
	}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "automod_edit", "rule_id": rid, "data": map[string]any{"enabled": false}}); code != 200 {
		t.Errorf("turning off: %d %v", code, out)
	}
	if code, out := f.manage(t, f.nexa, map[string]any{"action": "automod_delete", "rule_id": rid}); code != 200 {
		t.Errorf("deleting: %d %v", code, out)
	}
}

// Opening the page is logged first and shows no private topic.
func TestOwnerManageView(t *testing.T) {
	f := manageSetup(t)
	if res, out := do(t, f.ts, "PATCH", "/api/v1/channels/"+f.secret, f.srvOwner, map[string]any{"topic": "the hidden topic"}); res.StatusCode != 200 {
		t.Fatalf("topic: %d %v", res.StatusCode, out)
	}
	res, out := do(t, f.ts, "GET", "/api/v1/moderation/servers/"+f.sid+"/manage", f.nexa, nil)
	if res.StatusCode != 200 {
		t.Fatalf("view: %d %v", res.StatusCode, out)
	}
	if strings.Contains(mustString(out), "the hidden topic") {
		t.Error("a private channel's topic was shown")
	}
	if channelNamed(out, "secret")["private"] != true {
		t.Error("secret not marked private")
	}
	if f.logged(t, store.LogOwnerServerView) == nil {
		t.Error("opening the page was not logged")
	}
}
