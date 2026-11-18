package api

import (
	"testing"

	"evilion.xyz/chat/internal/perms"
)

// A template is made only by those who manage the server, and carries roles
// and role overwrites but never what belongs to one member: an overwrite for a
// person would hand their id to everyone with the link. The server made from
// it keeps a private channel private.
func TestServerTemplateKeepsRolesAndPrivacyButNoMembers(t *testing.T) {
	_, ts := newTestServer(t)
	tokA, _ := register(t, ts, "alice")
	tokB, idB := register(t, ts, "bob")
	view := createServer(t, ts, tokA, "Source")
	sid := view["id"].(string)
	join(t, ts, tokB, inviteFor(t, ts, tokA, channelNamed(view, "general")["id"].(string)))

	mod := createRole(t, ts, tokA, sid, "Mods", perms.KickMembers)
	res, ch := do(t, ts, "POST", "/api/v1/servers/"+sid+"/channels", tokA, map[string]any{"type": "text", "name": "staff room"})
	if res.StatusCode != 201 {
		t.Fatalf("channel: %d %v", res.StatusCode, ch)
	}
	room := ch["id"].(string)
	for target, body := range map[string]map[string]any{
		sid: {"type": "role", "deny": permStr(perms.ViewChannel)},
		mod: {"type": "role", "allow": permStr(perms.ViewChannel)},
		idB: {"type": "member", "allow": permStr(perms.ViewChannel)},
	} {
		if res, b := do(t, ts, "PUT", "/api/v1/channels/"+room+"/permissions/"+target, tokA, body); res.StatusCode != 200 && res.StatusCode != 204 {
			t.Fatalf("overwrite %s: %d %v", target, res.StatusCode, b)
		}
	}

	if res, _ := do(t, ts, "PUT", "/api/v1/servers/"+sid+"/template", tokB, map[string]any{"name": "Copy"}); res.StatusCode != 403 {
		t.Fatalf("a member without Manage Server made a template: %d", res.StatusCode)
	}
	res, body := do(t, ts, "PUT", "/api/v1/servers/"+sid+"/template", tokA, map[string]any{"name": "Copy", "description": "Our layout"})
	if res.StatusCode != 201 {
		t.Fatalf("template: %d %v", res.StatusCode, body)
	}
	link := body["template"].(map[string]any)["url"].(string)

	res, preview := do(t, ts, "GET", "/api/v1/servers/templates/nexa/"+body["template"].(map[string]any)["code"].(string), tokB, nil)
	if res.StatusCode != 200 || preview["name"] != "Copy" || preview["server_name"] != "Source" {
		t.Fatalf("preview: %d %v", res.StatusCode, preview)
	}
	res, made := do(t, ts, "POST", "/api/v1/servers/import/nexa", tokB, map[string]any{"url": link, "name": "Mine"})
	if res.StatusCode != 201 || made["name"] != "Mine" {
		t.Fatalf("import: %d %v", res.StatusCode, made)
	}
	copied := channelNamed(made, "staff room")
	if copied == nil || copied["key_conv_id"] != copied["id"] {
		t.Fatalf("the private channel is not private in the copy: %v", copied)
	}
	roles := map[string]bool{}
	for _, r := range made["roles"].([]any) {
		roles[r.(map[string]any)["name"].(string)] = true
	}
	if !roles["Mods"] {
		t.Fatalf("roles: %v", made["roles"])
	}
	for _, ow := range copied["overwrites"].([]any) {
		if o := ow.(map[string]any); o["type"] == "member" || o["target_type"] == "member" || o["target_id"] == idB {
			t.Fatalf("a member's overwrite came along: %v", o)
		}
	}
	if len(copied["overwrites"].([]any)) != 2 {
		t.Fatalf("role overwrites: %v", copied["overwrites"])
	}

	if res, _ := do(t, ts, "DELETE", "/api/v1/servers/"+sid+"/template", tokA, nil); res.StatusCode != 204 {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if res, _ := do(t, ts, "POST", "/api/v1/servers/import/nexa", tokB, map[string]any{"url": link}); res.StatusCode != 404 {
		t.Fatalf("a deleted template still made a server: %d", res.StatusCode)
	}
}
