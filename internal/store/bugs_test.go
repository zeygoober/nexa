package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"evilion.xyz/chat/internal/ids"
)

// bugFile stores a blob as a report file uploaded by owner and not sent yet.
func bugFile(t *testing.T, s *Store, owner ids.ID) ids.ID {
	t.Helper()
	o := owner
	b := &Blob{SHA256: "f00d" + ids.New().String(), Size: 1000, Mime: "image/png", Kind: BugBlobKind, OwnerID: &o, Width: ptr(10), Height: ptr(10)}
	if err := s.InsertBlob(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func newBug(t *testing.T, s *Store, u *User, subject string, files ...ids.ID) *BugReport {
	t.Helper()
	r := &BugReport{UserID: u.ID, ReporterUsername: u.Username, ReporterDisplayName: u.DisplayName, Subject: subject, Description: "It broke when I pressed the button."}
	list := make([]BugAttachment, 0, len(files))
	for _, f := range files {
		list = append(list, BugAttachment{BlobID: f, Name: "shot.png"})
	}
	if err := s.CreateBugReport(context.Background(), r, list); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBugReportsNumberAndTakeTheirFiles(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	bob := mkUser(t, s, "bob")
	carol := mkUser(t, s, "carol")
	f1, f2 := bugFile(t, s, bob.ID), bugFile(t, s, bob.ID)
	a := newBug(t, s, bob, "Calls drop", f1, f2)
	b := newBug(t, s, carol, "Menu cut off")
	if a.Number != 1 || b.Number != 2 {
		t.Fatalf("numbers %d, %d", a.Number, b.Number)
	}
	if a.Status != BugOpen || a.Awaiting != AwaitStaff || a.AttachmentCount != 2 {
		t.Fatalf("new report: %+v", a)
	}
	for _, f := range []ids.ID{f1, f2} {
		blob, err := s.BlobByID(ctx, f)
		if err != nil || blob.RefCount != 1 {
			t.Fatalf("file %s: ref %v %v", f, blob, err)
		}
		if owner, sent, err := s.BugFileOwner(ctx, f); err != nil || owner != bob.ID || !sent {
			t.Fatalf("owner of a sent file: %v %v %v", owner, sent, err)
		}
	}
	files, err := s.BugAttachments(ctx, []ids.ID{a.ID, b.ID})
	if err != nil || len(files[a.ID]) != 2 || files[a.ID][0].BlobID != f1 || files[a.ID][1].BlobID != f2 || len(files[b.ID]) != 0 {
		t.Fatalf("attachments %v %v", files, err)
	}
	if files[a.ID][0].Mime != "image/png" || *files[a.ID][0].Width != 10 || files[a.ID][0].Name != "shot.png" {
		t.Fatalf("attachment view %+v", files[a.ID][0])
	}

	// a file cannot be sent twice, by someone else, or when it is not a report file
	cf := bugFile(t, s, carol.ID)
	for name, f := range map[string]ids.ID{"sent already": f1, "someone else's": cf} {
		r := &BugReport{UserID: bob.ID, ReporterUsername: "bob", ReporterDisplayName: "bob", Subject: "x", Description: "y"}
		if err := s.CreateBugReport(ctx, r, []BugAttachment{{BlobID: f}}); !errors.Is(err, ErrBugAttachment) {
			t.Errorf("%s: %v", name, err)
		}
	}
	avatar := &Blob{SHA256: "cafe", Size: 1, Mime: "image/png", Kind: "avatar", OwnerID: &bob.ID}
	if err := s.InsertBlob(ctx, avatar); err != nil {
		t.Fatal(err)
	}
	r := &BugReport{UserID: bob.ID, ReporterUsername: "bob", ReporterDisplayName: "bob", Subject: "x", Description: "y"}
	if err := s.CreateBugReport(ctx, r, []BugAttachment{{BlobID: avatar.ID}}); !errors.Is(err, ErrBugAttachment) {
		t.Errorf("an avatar as a report file: %v", err)
	}
	// the failed reports took nothing and used no number
	if blob, _ := s.BlobByID(ctx, cf); blob.RefCount != 0 {
		t.Errorf("carol's file took a reference: %d", blob.RefCount)
	}
	if c := newBug(t, s, bob, "Third"); c.Number != 3 {
		t.Errorf("a failed report used a number: %d", c.Number)
	}
	if owner, sent, err := s.BugFileOwner(ctx, cf); err != nil || owner != carol.ID || sent {
		t.Errorf("owner of an unsent file: %v %v %v", owner, sent, err)
	}
	if _, _, err := s.BugFileOwner(ctx, avatar.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an avatar is not a report file: %v", err)
	}
}

func TestBugThreadMovesTheReportAlong(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	bob := mkUser(t, s, "bob")
	alice := mkUser(t, s, "alice")
	r := newBug(t, s, bob, "Calls drop")

	reply, got, err := s.AddBugMessage(ctx, r.ID, alice.ID, ModMsgStaff, "Which browser?", nil)
	if err != nil || got.Awaiting != AwaitMember || reply.ReportID != r.ID {
		t.Fatalf("staff reply: %+v %v", got, err)
	}
	unread, _ := s.BugUnreadByReport(ctx, bob.ID)
	if unread[r.ID] != 1 {
		t.Fatalf("unread after a reply: %v", unread)
	}
	n, _ := s.NexaUnread(ctx, bob.ID, bob.CreatedAt)
	latest, _ := s.NexaLatest(ctx, bob.ID, bob.CreatedAt)
	if n != 1 || latest != reply.CreatedAt {
		t.Fatalf("nexa unread %d latest %d, want 1 and %d", n, latest, reply.CreatedAt)
	}

	// a change of status is news too, and answers a report that was waiting
	mine, got, err := s.AddBugMessage(ctx, r.ID, bob.ID, ModMsgMember, "Firefox", nil)
	if err != nil || got.Awaiting != AwaitStaff {
		t.Fatalf("member reply: %+v %v", got, err)
	}
	if got.MemberReadID != mine.ID {
		t.Errorf("writing reads what came before: read %s, wrote %s", got.MemberReadID, mine.ID)
	}
	if unread, _ := s.BugUnreadByReport(ctx, bob.ID); unread[r.ID] != 0 {
		t.Errorf("their own reply left unread: %v", unread)
	}
	got, line, err := s.SetBugStatus(ctx, r.ID, alice.ID, BugResolved)
	if err != nil || line == nil || line.Event != BugEventStatus || line.Body != BugResolved || line.Kind != ModMsgSystem {
		t.Fatalf("status: %+v %+v %v", got, line, err)
	}
	if got.Status != BugResolved || got.Awaiting != AwaitNone || got.StatusBy == nil || *got.StatusBy != alice.ID {
		t.Fatalf("resolved report: %+v", got)
	}
	if unread, _ := s.BugUnreadByReport(ctx, bob.ID); unread[r.ID] != 1 {
		t.Errorf("a status change is news: %v", unread)
	}
	// the same status again changes nothing and says nothing
	if again, line, err := s.SetBugStatus(ctx, r.ID, alice.ID, BugResolved); err != nil || line != nil || again.Status != BugResolved {
		t.Errorf("same status: %+v %v", line, err)
	}
	if _, _, err := s.SetBugStatus(ctx, r.ID, alice.ID, "fixed"); err == nil {
		t.Error("an unknown status was taken")
	}
	// a finished report is closed to everyone, staff too, until staff set it going again
	if _, _, err := s.AddBugMessage(ctx, r.ID, bob.ID, ModMsgMember, "Still happens", nil); !errors.Is(err, ErrBugClosed) {
		t.Errorf("reply on a resolved report: %v", err)
	}
	if _, _, err := s.AddBugMessage(ctx, r.ID, alice.ID, ModMsgStaff, "One more thing", nil); !errors.Is(err, ErrBugClosed) {
		t.Errorf("staff reply on a resolved report: %v", err)
	}
	if got, _, _ := s.SetBugStatus(ctx, r.ID, alice.ID, BugOpen); got.Status != BugOpen {
		t.Fatalf("reopened: %+v", got)
	}
	_, got, _ = s.AddBugMessage(ctx, r.ID, bob.ID, ModMsgMember, "Still happens", nil)
	if got.Status != BugOpen || got.Awaiting != AwaitStaff {
		t.Errorf("reply on a reopened report: %+v", got)
	}
	// a status change while waiting on the member leaves them the one to answer
	_, _, _ = s.AddBugMessage(ctx, r.ID, alice.ID, ModMsgStaff, "Can you send a recording?", nil)
	if got, _, _ := s.SetBugStatus(ctx, r.ID, alice.ID, BugInProgress); got.Awaiting != AwaitMember {
		t.Errorf("status change took the question away: %+v", got)
	}

	msgs, err := s.BugMessages(ctx, r.ID, 0, 0)
	if err != nil || len(msgs) != 7 {
		t.Fatalf("thread: %d lines %v", len(msgs), err)
	}
	if err := s.MarkBugRead(ctx, r.ID, bob.ID, msgs[len(msgs)-1].ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.NexaUnread(ctx, bob.ID, bob.CreatedAt); n != 0 {
		t.Errorf("unread after reading: %d", n)
	}
	if err := s.MarkBugRead(ctx, r.ID, alice.ID, msgs[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading someone else's report: %v", err)
	}
	last, _ := s.LastBugMessages(ctx, []ids.ID{r.ID})
	if last[r.ID] == nil || last[r.ID].ID != msgs[len(msgs)-1].ID {
		t.Errorf("last line %+v", last[r.ID])
	}
	news, err := s.BugNewsForUser(ctx, bob.ID, 0, 10)
	if err != nil || len(news) != 5 {
		t.Fatalf("news: %d %v", len(news), err)
	}
	for _, m := range news {
		if m.Kind == ModMsgMember {
			t.Errorf("the reporter's own line is not news: %+v", m)
		}
	}
	if older, _ := s.BugNewsForUser(ctx, bob.ID, news[1].CreatedAt, 10); len(older) == 0 || older[0].CreatedAt >= news[1].CreatedAt {
		t.Errorf("paging news: %+v", older)
	}
}

func TestBugListFiltersSortsAndCounts(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	bob := mkUser(t, s, "bob")
	carol := mkUser(t, s, "carol")
	alice := mkUser(t, s, "alice")
	calls := newBug(t, s, bob, "Calls drop", bugFile(t, s, bob.ID))
	menu := newBug(t, s, carol, "Menu cut off in German")
	emoji := newBug(t, s, bob, "Emoji picker is slow")
	if _, _, err := s.SetBugStatus(ctx, menu.ID, alice.ID, BugInProgress); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SetBugStatus(ctx, emoji.ID, alice.ID, BugWontFix); err != nil {
		t.Fatal(err)
	}

	c, err := s.CountBugReports(ctx, BugKindBug)
	if err != nil {
		t.Fatal(err)
	}
	if c != (BugCounts{Waiting: 1, Open: 1, InProgress: 1, WontFix: 1, All: 3}) {
		t.Fatalf("counts %+v", c)
	}
	subjects := func(f BugFilter) []string {
		t.Helper()
		list, err := s.BugReportList(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, r := range list {
			out = append(out, r.Subject)
		}
		return out
	}
	eq := func(name string, got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("%s: %v, want %v", name, got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: %v, want %v", name, got, want)
				return
			}
		}
	}
	// most recently active first: the two status changes came after calls was sent
	eq("all", subjects(BugFilter{}), "Emoji picker is slow", "Menu cut off in German", "Calls drop")
	eq("active", subjects(BugFilter{Status: "active"}), "Menu cut off in German", "Calls drop")
	eq("wont fix", subjects(BugFilter{Status: BugWontFix}), "Emoji picker is slow")
	eq("waiting", subjects(BugFilter{Waiting: true}), "Calls drop")
	eq("with files", subjects(BugFilter{WithFiles: true}), "Calls drop")
	eq("search subject", subjects(BugFilter{Query: "GERMAN"}), "Menu cut off in German")
	eq("search reporter", subjects(BugFilter{Query: "@carol"}), "Menu cut off in German")
	eq("search description", subjects(BugFilter{Query: "pressed the button", Sort: "oldest"}), "Calls drop", "Menu cut off in German", "Emoji picker is slow")
	eq("search number", subjects(BugFilter{Query: "#2"}), "Menu cut off in German")
	eq("like is escaped", subjects(BugFilter{Query: "%"}))
	eq("newest", subjects(BugFilter{Sort: "newest"}), "Emoji picker is slow", "Menu cut off in German", "Calls drop")

	// pages meet without a gap or a repeat, in each order
	for _, sort := range []string{"", "newest", "oldest"} {
		var seen []string
		f := BugFilter{Sort: sort, Limit: 2}
		for page := 0; page < 3; page++ {
			list, err := s.BugReportList(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range list {
				seen = append(seen, r.Subject)
			}
			if len(list) < 2 {
				break
			}
			last := list[len(list)-1]
			f.BeforeAt, f.BeforeID = last.LastActivityAt, last.ID
		}
		if len(seen) != 3 || seen[0] == seen[1] || seen[1] == seen[2] || seen[0] == seen[2] {
			t.Errorf("sort %q paged as %v", sort, seen)
		}
	}
	mine, err := s.BugReportsForUser(ctx, bob.ID)
	if err != nil || len(mine) != 2 || mine[0].ID != emoji.ID || mine[1].ID != calls.ID {
		t.Errorf("bob's reports: %v %v", mine, err)
	}
}

func TestDeletingAnAccountTakesItsBugReports(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	bob := mkUser(t, s, "bob")
	carol := mkUser(t, s, "carol")
	f := bugFile(t, s, bob.ID)
	r := newBug(t, s, bob, "Calls drop", f)
	keep := newBug(t, s, carol, "Menu cut off")
	if _, _, err := s.AddBugMessage(ctx, r.ID, carol.ID, ModMsgStaff, "Thanks", nil); err != nil {
		t.Fatal(err)
	}
	var ctxJSON map[string]string
	_ = json.Unmarshal(r.Context, &ctxJSON)
	if len(ctxJSON) != 0 {
		t.Errorf("no context was sent, got %v", ctxJSON)
	}
	if err := s.PurgeUserLeftovers(ctx, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BugReportByID(ctx, r.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("report outlived its account: %v", err)
	}
	if msgs, _ := s.BugMessages(ctx, r.ID, 0, 0); len(msgs) != 0 {
		t.Errorf("thread outlived its report: %d", len(msgs))
	}
	if blob, err := s.BlobByID(ctx, f); err != nil || blob.RefCount != 0 {
		t.Errorf("the file was not let go: %+v %v", blob, err)
	}
	if _, err := s.BugReportByID(ctx, keep.ID); err != nil {
		t.Errorf("someone else's report went too: %v", err)
	}
}

// Suggestions share the table, the numbers and the read positions with bug
// reports, but each kind has its own list, counts and staff badge.
func TestSuggestionsStayApartFromBugs(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	bob := mkUser(t, s, "bob")
	alice := mkUser(t, s, "alice")
	bug := newBug(t, s, bob, "Calls drop")
	idea := &BugReport{Kind: BugKindSuggestion, UserID: bob.ID, ReporterUsername: bob.Username, Subject: "Dark scrollbars", Description: "Scrollbars could follow the theme."}
	if err := s.CreateBugReport(ctx, idea, nil); err != nil {
		t.Fatal(err)
	}
	if bug.Kind != BugKindBug || idea.Number != bug.Number+1 {
		t.Fatalf("kinds and numbers: %q %d, %q %d", bug.Kind, bug.Number, idea.Kind, idea.Number)
	}
	if got, err := s.BugReportByID(ctx, idea.ID); err != nil || got.Kind != BugKindSuggestion {
		t.Fatalf("stored kind: %+v %v", got, err)
	}
	for kind, want := range map[string]ids.ID{BugKindBug: bug.ID, BugKindSuggestion: idea.ID} {
		list, err := s.BugReportList(ctx, BugFilter{Kind: kind})
		if err != nil || len(list) != 1 || list[0].ID != want {
			t.Errorf("%s list: %v %v", kind, list, err)
		}
		if c, err := s.CountBugReports(ctx, kind); err != nil || c.All != 1 || c.Waiting != 1 {
			t.Errorf("%s counts: %+v %v", kind, c, err)
		}
	}
	u, err := s.ModUnreadFor(ctx, alice.ID)
	if err != nil || len(u.Bugs) != 1 || u.Bugs[0] != bug.ID || len(u.Suggestions) != 1 || u.Suggestions[0] != idea.ID {
		t.Fatalf("unread: %+v %v", u, err)
	}
	if err := s.MarkModRead(ctx, alice.ID, ModReadBug, idea.ID, 0); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.ModUnreadFor(ctx, alice.ID); len(u.Suggestions) != 0 || len(u.Bugs) != 1 {
		t.Errorf("reading the suggestion: %+v", u)
	}
}
