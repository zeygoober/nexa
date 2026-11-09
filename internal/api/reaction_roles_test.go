package api

import (
	"net/url"
	"slices"
	"strconv"
	"testing"

	"evilion.xyz/chat/internal/perms"
)

// Who may set a reaction role, what a role may carry, and that only reacting
// and un-reacting move it.
func TestReactionRoles(t *testing.T) {
	_, ts := newTestServer(t)
	owner, _ := register(t, ts, "owner")
	member, memberID := register(t, ts, "member")
	view := createServer(t, ts, owner, "Roles")
	sid := view["id"].(string)
	general := channelNamed(view, "general")["id"].(string)
	join(t, ts, member, inviteFor(t, ts, owner, general))

	epoch := createEpoch(t, ts, owner, sid)
	_, msg := sendMsg(t, ts, owner, general, sid, epoch, "pick", "", nil)
	base := "/api/v1/channels/" + general + "/messages/" + msg["id"].(string)
	emoji := url.PathEscape("🎮")
	set := base + "/reaction-roles/" + emoji
	react := base + "/reactions/" + emoji + "/@me"

	gamers := createRole(t, ts, owner, sid, "Gamers", perms.DefaultEveryone)
	mods := createRole(t, ts, owner, sid, "Mods", perms.DefaultEveryone|perms.KickMembers)

	if res, _ := do(t, ts, "PUT", set, member, map[string]any{"role_id": gamers}); res.StatusCode != 403 {
		t.Fatalf("a member without Manage Roles set one: %d", res.StatusCode)
	}
	if res, body := do(t, ts, "PUT", set, owner, map[string]any{"role_id": mods}); res.StatusCode != 403 || errCode(body) != "reaction_role_unsafe" {
		t.Fatalf("a role with moderation permissions: %d %v", res.StatusCode, body)
	}
	if res, _ := do(t, ts, "PUT", set, owner, map[string]any{"role_id": sid}); res.StatusCode != 400 {
		t.Fatalf("@everyone: %d", res.StatusCode)
	}
	if res, body := do(t, ts, "PUT", set, owner, map[string]any{"role_id": gamers}); res.StatusCode != 200 {
		t.Fatalf("set: %d %v", res.StatusCode, body)
	}

	roles := func() []any {
		_, m := do(t, ts, "GET", "/api/v1/servers/"+sid+"/members/"+memberID, owner, nil)
		list, _ := m["role_ids"].([]any)
		return list
	}
	if res, _ := do(t, ts, "PUT", react, member, nil); res.StatusCode != 204 {
		t.Fatalf("react: %d", res.StatusCode)
	}
	if !slices.Contains(roles(), any(gamers)) {
		t.Fatalf("reacting gave no role: %v", roles())
	}
	if res, _ := do(t, ts, "DELETE", react, member, nil); res.StatusCode != 204 {
		t.Fatalf("un-react: %d", res.StatusCode)
	}
	if slices.Contains(roles(), any(gamers)) {
		t.Fatalf("taking the reaction off kept the role: %v", roles())
	}

	// a role that gains power afterwards is not handed out any more
	if res, body := do(t, ts, "PATCH", "/api/v1/servers/"+sid+"/roles/"+gamers, owner, map[string]any{"permissions": strconv.FormatUint(uint64(perms.DefaultEveryone|perms.BanMembers), 10)}); res.StatusCode != 200 {
		t.Fatalf("raise the role: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "PUT", react, member, nil); res.StatusCode != 403 || errCode(body) != "reaction_role_unsafe" {
		t.Fatalf("reacting for a role now unsafe: %d %v", res.StatusCode, body)
	}
	if slices.Contains(roles(), any(gamers)) {
		t.Fatalf("an unsafe role was given: %v", roles())
	}
}
