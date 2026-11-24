package api

import (
	"context"
	"strings"
	"testing"

	"evilion.xyz/chat/internal/ids"
)

// The law-enforcement report has two jobs that pull against each other: say
// everything Nexa holds, and say nothing it does not. The tests below
// check both ends — that a real trail shows up in the document, and that the
// secrets sitting next to that trail in the same tables do not.
func TestLegalReport(t *testing.T) {
	srv, ts := newTestServer(t)
	tokA, idA := register(t, ts, "alice") // the first account is the admin
	tokB, idB := register(t, ts, "bob")

	// Bob leaves the trail an ordinary account leaves.
	server := createServer(t, ts, tokB, "Bob's place")
	sid := server["id"].(string)
	friend(t, ts, tokB, "alice", tokA, idB)
	res, body := do(t, ts, "POST", "/api/v1/users/@me/channels", tokB, map[string]any{"recipient_ids": []string{idA}})
	if res.StatusCode >= 300 {
		t.Fatalf("create dm: %d %v", res.StatusCode, body)
	}
	dm := body["id"].(string)
	epoch := createEpoch(t, ts, tokB, dm)
	if res, body = sendMsg(t, ts, tokB, dm, dm, epoch, "hello", "n1", map[string]any{"mentions": []string{idA}}); res.StatusCode != 201 {
		t.Fatalf("send: %d %v", res.StatusCode, body)
	}

	// Only an admin may ask, and the account has to be named and has to exist.
	if res, body = do(t, ts, "GET", "/api/v1/admin/legal-report?user=bob", tokB, nil); res.StatusCode != 403 || errCode(body) != "admin_only" {
		t.Errorf("non-admin: %d %v", res.StatusCode, body)
	}
	if res, body = do(t, ts, "GET", "/api/v1/admin/legal-report", tokA, nil); res.StatusCode != 400 || errCode(body) != "no_account" {
		t.Errorf("no account named: %d %v", res.StatusCode, body)
	}
	if res, body = do(t, ts, "GET", "/api/v1/admin/legal-report?user=nobody", tokA, nil); res.StatusCode != 404 || errCode(body) != "no_such_account" {
		t.Errorf("unknown account: %d %v", res.StatusCode, body)
	}

	// An officer may write down either identifier, with or without the @.
	res, body = do(t, ts, "GET", "/api/v1/admin/legal-report?user=@bob", tokA, nil)
	if res.StatusCode != 200 || body["username"] != "bob" || body["user_id"] != idB {
		t.Fatalf("by username: %d %v", res.StatusCode, body)
	}
	doc, _ := body["document"].(string)
	if doc == "" {
		t.Fatal("no document in the response")
	}
	if name, _ := body["filename"].(string); !strings.HasPrefix(name, "nexa-law-enforcement-response-bob-") || !strings.HasSuffix(name, ".txt") {
		t.Errorf("filename: %q", name)
	}
	if res, byID := do(t, ts, "GET", "/api/v1/admin/legal-report?user="+idB, tokA, nil); res.StatusCode != 200 || byID["user_id"] != idB {
		t.Errorf("by id: %d %v", res.StatusCode, byID)
	}

	// The header, the two sections that answer a request before it is repeated,
	// and the provenance the officer needs to cite the document.
	for _, want := range []string{
		"NEXA — LAW ENFORCEMENT RESPONSE",
		"Account requested:", "@bob", idB,
		"Report compiled by:", "@alice", idA,
		"Snapshot taken:",
		"DATA NOT AVAILABLE", "DATA NOT COLLECTED", "PROVENANCE",
		"Tables read for this report:", legalTablesRead[0], legalTablesRead[len(legalTablesRead)-1],
		"END OF REPORT",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("report does not mention %q", want)
		}
	}

	// What Bob actually did is in it, from the tables it came from.
	for _, want := range []string{
		"Bob's place",    // servers, through his membership and his ownership
		"IP last used:", // sessions.ip
		"Addresses used:", // user_ips
		"User agent:",    // sessions.user_agent
		"account " + idA, // relationships, both directions
		"dm " + dm,       // dm_participants joined to channels
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("report does not mention %q", want)
		}
	}
	for label, want := range map[string]string{
		"Username:":          "bob",
		"Account id:":        idB,
		"Sessions held now:": "1",
		"Memberships:":       "1",
		"Communities owned:": "1",
		"Conversations:":     "1",
		"Messages sent:":     "1",
		"Rows they created:": "1",
		"Rows naming them:":  "1",
	} {
		if got := fieldValue(doc, label); got != want {
			t.Errorf("%s reads %q, want %q", label, got, want)
		}
	}

	// The stated limits have to be unmistakable, because their whole purpose is
	// that the same request does not come back. Since owner access to new
	// messages (migration 0069) the report says what the owner can read.
	for _, want := range []string{
		"Historical access requires",
		"Calls are not recorded.",
		"No email address is collected",
		"recording facility in this software",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("report does not state %q", want)
		}
	}

	// Nothing that sits beside the metadata in those same tables comes out with
	// it: not the ciphertext, not a bearer token, not a wrapped key.
	for what, secret := range map[string]string{
		"message ciphertext":   b64([]byte("ct:hello")),
		"bearer token":         tokB,
		"wrapped private key":  b64(wrapped),
		"wrapped conversation": b64([]byte("wrapped-for-" + idB)),
	} {
		if strings.Contains(doc, secret) {
			t.Errorf("the %s is in the report", what)
		}
	}

	// A mention travels outside the ciphertext, so Alice's report counts the
	// message Bob wrote about her and Bob's does not.
	if got := fieldValue(doc, "Mentions of them:"); got != "0" {
		t.Errorf("bob should be mentioned in nothing, reads %q", got)
	}
	_, alice := do(t, ts, "GET", "/api/v1/admin/legal-report?user=alice", tokA, nil)
	aliceDoc, _ := alice["document"].(string)
	if got := fieldValue(aliceDoc, "Mentions of them:"); got != "1" {
		t.Errorf("alice should be mentioned once, reads %q", got)
	}

	// Compiling a report is a read. Nothing about Nexa changes, and in
	// particular nothing lands in a community's audit log, where the people who
	// moderate that community would see that one of their members is the
	// subject of legal process.
	_, after := do(t, ts, "GET", "/api/v1/admin/stats", tokA, nil)
	if after["users"].(float64) != 2 || after["messages"].(float64) != 1 {
		t.Errorf("the report changed something: %v", after)
	}
	serverID, err := ids.Parse(sid)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := srv.Store.AuditEntries(context.Background(), serverID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Action, "legal") || strings.Contains(e.Action, "report") {
			t.Errorf("the report was written into a community audit log: %v", e)
		}
	}
}

// An account that has done nothing at all is the case the renderer is most
// likely to get wrong, and the one an officer is most likely to be given: a
// name from somewhere else that turns out to have barely used Nexa.
func TestLegalReportEmptyAccount(t *testing.T) {
	_, ts := newTestServer(t)
	tokA, _ := register(t, ts, "alice")
	_, idC := register(t, ts, "carol")

	res, body := do(t, ts, "GET", "/api/v1/admin/legal-report?user=carol", tokA, nil)
	if res.StatusCode != 200 || body["user_id"] != idC {
		t.Fatalf("empty account: %d %v", res.StatusCode, body)
	}
	doc, _ := body["document"].(string)
	for _, want := range []string{
		"No second factor has ever been set up",
		"No relationship rows exist in either direction.",
		"This account is not a member of any community.",
		"This account is not in any direct message, group or subchat.",
		"This account has uploaded no files.",
		"No crash reports from this account are held.",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("empty report does not say %q", want)
		}
	}
	if got := fieldValue(doc, "Messages sent:"); got != "0" {
		t.Errorf("messages sent reads %q, want 0", got)
	}
	// Registering signs the new account in, so there is exactly one session and
	// the document says where from rather than leaving the section blank.
	if got := fieldValue(doc, "Sessions held now:"); got != "1" {
		t.Errorf("sessions reads %q, want the registration session", got)
	}
	if got := fieldValue(doc, "IP last used:"); got == "" {
		t.Error("the registration session has no address in the report")
	}
}

// fieldValue reads back one of the document's label/value rows, so a test can
// assert on what a line says without depending on how far the columns are
// padded apart.
func fieldValue(doc, label string) string {
	for _, line := range strings.Split(doc, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, label) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, label))
		}
	}
	return ""
}
