package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// The owner's server management: named changes the owner of Nexa can make to a
// server from the panel, whether or not they belong to it. Each runs the same
// handler a server's own managers use, as the owner's real account, so people
// see who did it. Nothing here may reach a conversation's messages or keys:
// every change is first tried on a copy of the server's permissions
// (moderation_manage_guard.go) and refused if it would give the owner, staff,
// apps or new members access to anything they lack.

func (s *Server) modManageRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/moderation/servers/{id}/manage", s.staff(rankOwner, s.handleModManageView))
	mux.Handle("GET /api/v1/moderation/servers/{id}/manage/members", s.staff(rankOwner, s.handleModManageMembers))
	mux.Handle("POST /api/v1/moderation/servers/{id}/manage", s.userLimited(s.limManage, s.holdsRank(rankOwner, s.handleModManage)))
}

// ownerActing is in a request's context only while handleModManage runs one of
// its handlers. The gates that read it let the owner through for that server
// alone, and audit entries the handler writes are collected here instead,
// since the action has its own entry already.
type ownerActing struct {
	serverID ids.ID
	actor    ids.ID
	changes  map[string]json.RawMessage
}

type ownerActingKey struct{}

func actingOwner(ctx context.Context, serverID, uid ids.ID) bool {
	a, _ := ctx.Value(ownerActingKey{}).(*ownerActing)
	return a != nil && a.serverID == serverID && a.actor == uid
}

// noteAudit keeps what a handler would have written to the audit log. It says
// whether the entry was taken.
func noteAudit(ctx context.Context, serverID ids.ID, raw json.RawMessage) bool {
	a, _ := ctx.Value(ownerActingKey{}).(*ownerActing)
	if a == nil || a.serverID != serverID {
		return false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		for k, v := range obj {
			a.changes[k] = v
		}
	}
	return true
}

// serverPerms is a member's permissions in a server, or everything for the
// owner in the middle of an action there.
func (s *Server) serverPerms(ctx context.Context, serverID, uid ids.ID) (perms.Perm, error) {
	if actingOwner(ctx, serverID, uid) {
		return perms.All, nil
	}
	return s.Store.ServerPerms(ctx, serverID, uid)
}

// channelPerms is the same for a server channel.
func (s *Server) channelPerms(ctx context.Context, ch *store.Channel, uid ids.ID) (perms.Perm, error) {
	if ch.ServerID != nil && actingOwner(ctx, *ch.ServerID, uid) {
		return perms.All, nil
	}
	p, _, err := s.Store.ChannelPerms(ctx, ch.ID, uid)
	return p, err
}

type manageRequest struct {
	Action    string                     `json:"action"`
	Reason    string                     `json:"reason"`
	ChannelID *ids.ID                    `json:"channel_id"`
	RoleID    *ids.ID                    `json:"role_id"`
	UserID    *ids.ID                    `json:"user_id"`
	TargetID  *ids.ID                    `json:"target_id"`
	WebhookID *ids.ID                    `json:"webhook_id"`
	AppID     *ids.ID                    `json:"app_id"`
	RuleID    *ids.ID                    `json:"rule_id"`
	Code      string                     `json:"code"`
	Data      map[string]json.RawMessage `json:"data"`
}

// managePlan is an action made ready: which handler runs and with what, what
// it is recorded as, and what it would do to who can see what.
type managePlan struct {
	handler    http.HandlerFunc
	method     string
	path       map[string]string
	query      url.Values
	body       any
	audit      string
	targetType string
	target     *ids.ID
	targetName string
	before     map[string]any
	// subject is someone the action happens to, who is told; notice says how
	subject *store.User
	notice  string
	mutate  func(*manageWorld)
}

type manageCall struct {
	s     *Server
	ctx   context.Context
	srv   *store.Server
	owner *store.User
	me    *store.User
	req   *manageRequest
	world *manageWorld
}

var (
	errManageTarget = E(http.StatusNotFound, "not_in_server", "That is not part of this server.")
	errManageBody   = E(http.StatusBadRequest, "invalid_value", "Those details cannot be used.")
)

// pick keeps only the named fields of what the panel sent: anything else a
// handler might accept never reaches it.
func (m *manageCall) pick(names ...string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for _, n := range names {
		if v, ok := m.req.Data[n]; ok {
			out[n] = v
		}
	}
	return out
}

// channel is the channel asked about, which must be this server's own: never a
// thread, a subchat or a conversation.
func (m *manageCall) channel() (*store.Channel, error) {
	if m.req.ChannelID == nil {
		return nil, errManageTarget
	}
	ch, err := m.s.Store.ChannelByID(m.ctx, *m.req.ChannelID)
	if err != nil || ch.ServerID == nil || *ch.ServerID != m.srv.ID || store.IsThread(ch.Type) || store.IsDMLike(ch.Type) {
		return nil, errManageTarget
	}
	return ch, nil
}

func (m *manageCall) role() (*store.Role, error) {
	if m.req.RoleID == nil {
		return nil, errManageTarget
	}
	ro, err := m.s.Store.RoleByID(m.ctx, *m.req.RoleID)
	if err != nil || ro.ServerID != m.srv.ID {
		return nil, errManageTarget
	}
	return ro, nil
}

// member is the member asked about, who must be in this server.
func (m *manageCall) member() (*store.User, error) {
	if m.req.UserID == nil {
		return nil, errManageTarget
	}
	if ok, _ := m.s.Store.IsMember(m.ctx, m.srv.ID, *m.req.UserID); !ok {
		return nil, errManageTarget
	}
	u, err := m.s.Store.UserByID(m.ctx, *m.req.UserID)
	if err != nil {
		return nil, errManageTarget
	}
	return u, nil
}

// person is a member who can be removed or banned: not the server's owner,
// not an app (removing the app does that), not the owner acting, not staff.
func (m *manageCall) person() (*store.User, error) {
	u, err := m.member()
	if err != nil {
		return nil, err
	}
	if u.ID == m.srv.OwnerID {
		return nil, E(http.StatusBadRequest, "is_owner", "The server's owner cannot be removed. Transfer the server first.")
	}
	if u.Flags&store.FlagBot != 0 {
		return nil, E(http.StatusBadRequest, "is_app", "Remove the app instead.")
	}
	if code := m.s.moderationBlock(m.ctx, m.me, u); code != "" {
		return nil, blockError(code)
	}
	return u, nil
}

func chanBefore(ch *store.Channel, private bool, fields map[string]json.RawMessage) map[string]any {
	all := map[string]any{"name": ch.Name, "nsfw": ch.NSFW, "slowmode_seconds": ch.SlowmodeSeconds, "user_limit": ch.UserLimit, "glyph": ch.Glyph}
	if !private {
		all["topic"] = ch.Topic
	}
	out := map[string]any{}
	for k := range fields {
		if v, ok := all[k]; ok {
			out[k] = v
		}
	}
	return out
}

func roleBefore(ro *store.Role) map[string]any {
	return map[string]any{"name": ro.Name, "color": ro.Color, "hoist": ro.Hoist, "mentionable": ro.Mentionable, "permissions": ro.Permissions}
}

func (m *manageCall) isPrivateChannel(id ids.ID) bool {
	c := m.world.channel(id)
	return c == nil || m.world.isPrivate(c)
}

// ownerGone says whether a server has been left without anyone to run it.
func (m *manageCall) ownerGone() bool {
	if m.owner.Flags&(store.FlagDeleted|store.FlagDisabled) != 0 {
		return true
	}
	_, ban, err := m.s.Store.ActiveRestrictions(m.ctx, m.owner.ID, time.Now().UnixMilli())
	return err == nil && ban.Active(time.Now().UnixMilli())
}

// plan turns a request into what will run. It is the whole list of what the
// owner can do to a server; anything not named here does not exist.
func (m *manageCall) plan() (*managePlan, error) {
	s, srv := m.s, m.srv
	sid := srv.ID.String()
	switch m.req.Action {
	case "server_edit":
		fields := m.pick("name", "description", "system_channel_id", "default_notifications", "nsfw", "invites_paused_until")
		if len(fields) == 0 {
			return nil, errManageBody
		}
		all := map[string]any{"name": srv.Name, "description": srv.Description, "system_channel_id": srv.SystemChannelID, "default_notifications": srv.DefaultNotifications, "nsfw": srv.NSFW, "invites_paused_until": srv.InvitesPausedUntil}
		before := map[string]any{}
		for k := range fields {
			before[k] = all[k]
		}
		return &managePlan{handler: s.handleServerPatch, method: "PATCH", path: map[string]string{"id": sid}, body: fields, audit: "server_update", targetType: "server", target: &srv.ID, targetName: srv.Name, before: before}, nil
	case "server_icon_clear", "server_banner_clear":
		kind, was := "icon", srv.IconID
		if m.req.Action == "server_banner_clear" {
			kind, was = "banner", srv.BannerID
		}
		if was == nil {
			return nil, E(http.StatusBadRequest, "nothing_to_remove", "There is nothing to remove.")
		}
		return &managePlan{handler: s.handleServerImageDelete(kind), method: "DELETE", path: map[string]string{"id": sid}, audit: "server_update", targetType: "server", target: &srv.ID, targetName: srv.Name, before: map[string]any{kind + "_id": was}}, nil

	case "channel_create":
		fields := m.pick("type", "name", "parent_id", "url")
		return &managePlan{handler: s.handleServerChannelCreate, method: "POST", path: map[string]string{"id": sid}, body: fields, audit: "channel_create", targetType: "channel", before: map[string]any{}}, nil
	case "channel_edit":
		ch, err := m.channel()
		if err != nil {
			return nil, err
		}
		private := m.isPrivateChannel(ch.ID)
		fields := m.pick("name", "nsfw", "slowmode_seconds", "user_limit", "glyph")
		// a private channel's topic is its members' to read, so also to replace
		if t, ok := m.req.Data["topic"]; ok {
			if private {
				return nil, E(http.StatusForbidden, "private_topic", "A private channel's topic cannot be changed here.")
			}
			fields["topic"] = t
		}
		if len(fields) == 0 {
			return nil, errManageBody
		}
		return &managePlan{handler: func(w http.ResponseWriter, r *http.Request) { s.handleServerChannelPatch(w, r, ch, m.me) }, method: "PATCH", path: map[string]string{"id": ch.ID.String()}, body: fields, audit: "channel_update", targetType: "channel", target: &ch.ID, targetName: ch.Name, before: chanBefore(ch, private, fields)}, nil
	case "channel_move":
		ch, err := m.channel()
		if err != nil {
			return nil, err
		}
		item := map[string]any{"id": ch.ID}
		var pid *ids.ID
		if v, ok := m.req.Data["parent_id"]; ok {
			if json.Unmarshal(v, &pid) != nil {
				return nil, errManageBody
			}
			item["parent_id"] = pid
		}
		item["position"] = ch.Position
		if v, ok := m.req.Data["position"]; ok {
			var n int
			if json.Unmarshal(v, &n) != nil {
				return nil, errManageBody
			}
			item["position"] = n
		}
		return &managePlan{handler: s.handleServerChannelPositions, method: "PATCH", path: map[string]string{"id": sid}, body: []any{item}, audit: "channel_update", targetType: "channel", target: &ch.ID, targetName: ch.Name,
			before: map[string]any{"name": ch.Name, "parent_id": ch.ParentID, "position": ch.Position},
			mutate: func(w *manageWorld) {
				// whether it syncs depends on what it had; the worst is that it does
				if pid != nil {
					w.takeCategory(ch.ID, *pid)
					if c := w.channel(ch.ID); c != nil {
						c.parent = pid
					}
				}
			}}, nil
	case "channel_delete":
		ch, err := m.channel()
		if err != nil {
			return nil, err
		}
		return &managePlan{handler: s.handleServerChannelDelete, method: "DELETE", path: map[string]string{"id": ch.ID.String()}, audit: "channel_delete", targetType: "channel", target: &ch.ID, targetName: ch.Name, before: map[string]any{"name": ch.Name, "type": ch.Type}}, nil

	case "overwrite_set":
		ch, err := m.channel()
		if err != nil {
			return nil, err
		}
		var body struct {
			Type  string     `json:"type"`
			Allow perms.Perm `json:"allow"`
			Deny  perms.Perm `json:"deny"`
		}
		raw, _ := json.Marshal(m.req.Data)
		if json.Unmarshal(raw, &body) != nil || m.req.TargetID == nil || !OneOf(body.Type, "role", "member") {
			return nil, errManageBody
		}
		target := *m.req.TargetID
		var before any
		for _, o := range ch.Overwrites {
			if o.TargetID == target {
				before = o
			}
		}
		return &managePlan{handler: s.handleOverwritePut, method: "PUT", path: map[string]string{"id": ch.ID.String(), "target": target.String()}, body: body, audit: "overwrite_update", targetType: "channel", target: &ch.ID, targetName: ch.Name, before: map[string]any{"overwrite": before},
			mutate: func(w *manageWorld) {
				w.setOverwrite(ch.ID, perms.Overwrite{TargetID: target, Kind: body.Type, Allow: body.Allow &^ body.Deny, Deny: body.Deny})
			}}, nil
	case "overwrite_remove":
		ch, err := m.channel()
		if err != nil || m.req.TargetID == nil {
			return nil, errManageTarget
		}
		target := *m.req.TargetID
		var before any
		for _, o := range ch.Overwrites {
			if o.TargetID == target {
				before = o
			}
		}
		if before == nil {
			return nil, errManageTarget
		}
		return &managePlan{handler: s.handleOverwriteDelete, method: "DELETE", path: map[string]string{"id": ch.ID.String(), "target": target.String()}, audit: "overwrite_delete", targetType: "channel", target: &ch.ID, targetName: ch.Name, before: map[string]any{"overwrite": before},
			mutate: func(w *manageWorld) { w.removeOverwrite(ch.ID, target) }}, nil
	case "overwrite_sync":
		ch, err := m.channel()
		if err != nil {
			return nil, err
		}
		if ch.ParentID == nil {
			return nil, E(http.StatusBadRequest, "no_category", "That channel is not in a category.")
		}
		parent := *ch.ParentID
		return &managePlan{handler: s.handlePermissionsSync, method: "POST", path: map[string]string{"id": ch.ID.String()}, audit: "overwrite_sync", targetType: "channel", target: &ch.ID, targetName: ch.Name, before: map[string]any{"overwrites": ch.Overwrites},
			mutate: func(w *manageWorld) { w.takeCategory(ch.ID, parent) }}, nil

	case "role_create":
		fields := m.pick("name", "color", "hoist", "mentionable", "permissions", "color2", "color3", "icon_emoji")
		return &managePlan{handler: s.handleRoleCreate, method: "POST", path: map[string]string{"id": sid}, body: fields, audit: "role_create", targetType: "role", before: map[string]any{}}, nil
	case "role_edit":
		ro, err := m.role()
		if err != nil {
			return nil, err
		}
		fields := m.pick("name", "color", "hoist", "mentionable", "permissions", "color2", "color3", "icon_emoji")
		if len(fields) == 0 {
			return nil, errManageBody
		}
		next, set := ro.Permissions, false
		if v, ok := fields["permissions"]; ok {
			if json.Unmarshal(v, &next) != nil {
				return nil, errManageBody
			}
			set = true
		}
		return &managePlan{handler: s.handleRolePatch, method: "PATCH", path: map[string]string{"id": sid, "rid": ro.ID.String()}, body: fields, audit: "role_update", targetType: "role", target: &ro.ID, targetName: ro.Name, before: roleBefore(ro),
			mutate: func(w *manageWorld) {
				if set {
					w.setRolePerms(ro.ID, next)
				}
			}}, nil
	case "role_delete":
		ro, err := m.role()
		if err != nil {
			return nil, err
		}
		return &managePlan{handler: s.handleRoleDelete, method: "DELETE", path: map[string]string{"id": sid, "rid": ro.ID.String()}, audit: "role_delete", targetType: "role", target: &ro.ID, targetName: ro.Name, before: roleBefore(ro),
			mutate: func(w *manageWorld) { w.deleteRole(ro.ID) }}, nil
	case "member_role_add", "member_role_remove":
		add := m.req.Action == "member_role_add"
		u, err := m.member()
		if err != nil {
			return nil, err
		}
		ro, err := m.role()
		if err != nil {
			return nil, err
		}
		audit := "member_role_remove"
		if add {
			audit = "member_role_add"
		}
		return &managePlan{handler: s.handleMemberRole(add), method: map[bool]string{true: "PUT", false: "DELETE"}[add], path: map[string]string{"id": sid, "uid": u.ID.String(), "rid": ro.ID.String()}, audit: audit, targetType: "user", target: &u.ID, targetName: u.Username, before: map[string]any{"role": ro.Name},
			mutate: func(w *manageWorld) { w.memberRole(u.ID, ro.ID, add) }}, nil

	case "member_remove":
		u, err := m.person()
		if err != nil {
			return nil, err
		}
		return &managePlan{handler: s.handleMemberKick, method: "DELETE", path: map[string]string{"id": sid, "uid": u.ID.String()}, query: url.Values{"reason": {m.req.Reason}}, audit: "member_kick", targetType: "user", target: &u.ID, targetName: u.Username, before: map[string]any{"username": u.Username}, subject: u, notice: "removed"}, nil
	case "member_ban":
		u, err := m.person()
		if err != nil {
			return nil, err
		}
		// a ban here never takes anyone's messages with it
		return &managePlan{handler: s.handleBanPut, method: "PUT", path: map[string]string{"id": sid, "uid": u.ID.String()}, body: map[string]any{"reason": m.req.Reason, "delete_message_days": 0}, audit: "member_ban", targetType: "user", target: &u.ID, targetName: u.Username, before: map[string]any{"username": u.Username}, subject: u, notice: "banned"}, nil
	case "member_unban":
		if m.req.UserID == nil {
			return nil, errManageTarget
		}
		bans, err := s.Store.Bans(m.ctx, srv.ID)
		if err != nil {
			return nil, err
		}
		for _, b := range bans {
			if b.UserID == *m.req.UserID {
				name := ""
				if u, err := s.Store.UserByID(m.ctx, b.UserID); err == nil {
					name = u.Username
				}
				return &managePlan{handler: s.handleBanDelete, method: "DELETE", path: map[string]string{"id": sid, "uid": b.UserID.String()}, audit: "member_unban", targetType: "user", target: &b.UserID, targetName: name, before: map[string]any{"reason": b.Reason}}, nil
			}
		}
		return nil, errManageTarget

	case "invite_revoke":
		in, err := s.Store.InviteByCode(m.ctx, m.req.Code)
		if err != nil || in.ServerID == nil || *in.ServerID != srv.ID {
			return nil, errManageTarget
		}
		return &managePlan{handler: s.handleInviteDelete, method: "DELETE", path: map[string]string{"code": in.Code}, audit: "invite_delete", targetType: "invite", targetName: in.Code, before: map[string]any{"code": in.Code, "uses": in.Uses}}, nil
	case "invites_revoke_all":
		return &managePlan{handler: s.handleRevokeAllInvites, method: "DELETE", path: map[string]string{"id": sid}, audit: "invite_delete_all", targetType: "server", target: &srv.ID, targetName: srv.Name, before: map[string]any{}}, nil
	case "vanity_clear":
		in, err := s.Store.VanityInvite(m.ctx, srv.ID)
		if err != nil || in == nil {
			return nil, E(http.StatusBadRequest, "nothing_to_remove", "There is nothing to remove.")
		}
		return &managePlan{handler: s.handleVanitySet, method: "PUT", path: map[string]string{"id": sid}, body: map[string]any{"code": nil}, audit: "vanity_url_update", targetType: "server", target: &srv.ID, targetName: in.Code, before: map[string]any{"code": in.Code}}, nil

	case "webhook_delete":
		if m.req.WebhookID == nil {
			return nil, errManageTarget
		}
		wh, err := s.Store.WebhookByID(m.ctx, *m.req.WebhookID)
		if err != nil || wh.ServerID != srv.ID {
			return nil, errManageTarget
		}
		return &managePlan{handler: s.handleWebhookDelete, method: "DELETE", path: map[string]string{"wid": wh.ID.String()}, audit: "webhook_delete", targetType: "webhook", target: &wh.ID, targetName: wh.Name, before: map[string]any{"name": wh.Name, "channel_id": wh.ChannelID}}, nil
	case "app_remove":
		if m.req.AppID == nil {
			return nil, errManageTarget
		}
		if _, err := s.Store.GuildInstall(m.ctx, *m.req.AppID, srv.ID); err != nil {
			return nil, errManageTarget
		}
		name := ""
		if a, err := s.Store.ApplicationByID(m.ctx, *m.req.AppID); err == nil {
			name = a.Name
		}
		return &managePlan{handler: s.handleIntegrationRemove, method: "DELETE", path: map[string]string{"id": sid, "app": m.req.AppID.String()}, audit: "integration_delete", targetType: "integration", target: m.req.AppID, targetName: name, before: map[string]any{"name": name}}, nil

	case "automod_create":
		fields := m.pick("name", "kind", "enabled", "config", "actions", "exempt_roles", "exempt_channels")
		if err := automodFieldsSafe(fields); err != nil {
			return nil, err
		}
		return &managePlan{handler: s.handleAutomodRuleSave, method: "POST", path: map[string]string{"id": sid}, body: fields, audit: "automod_rule_create", targetType: "automod_rule", before: map[string]any{}}, nil
	case "automod_edit":
		rule, err := m.rule()
		if err != nil {
			return nil, err
		}
		fields := m.pick("name", "kind", "enabled", "config", "actions", "exempt_roles", "exempt_channels")
		if len(fields) == 0 {
			return nil, errManageBody
		}
		if automodAlerts(rule.Actions) {
			// a rule that copies what it catches somewhere can only be turned off
			if v, ok := fields["enabled"]; !ok || len(fields) != 1 || strings.TrimSpace(string(v)) != "false" {
				return nil, errManageAlerts
			}
		} else if err := automodFieldsSafe(fields); err != nil {
			return nil, err
		}
		return &managePlan{handler: s.handleAutomodRuleSave, method: "PATCH", path: map[string]string{"id": sid, "rid": rule.ID.String()}, body: fields, audit: "automod_rule_update", targetType: "automod_rule", target: &rule.ID, targetName: rule.Name, before: map[string]any{"name": rule.Name, "kind": rule.Kind, "enabled": rule.Enabled}}, nil
	case "automod_delete":
		rule, err := m.rule()
		if err != nil {
			return nil, err
		}
		return &managePlan{handler: s.handleAutomodRuleDelete, method: "DELETE", path: map[string]string{"id": sid, "rid": rule.ID.String()}, audit: "automod_rule_delete", targetType: "automod_rule", target: &rule.ID, targetName: rule.Name, before: map[string]any{"name": rule.Name, "kind": rule.Kind, "enabled": rule.Enabled}}, nil

	case "transfer":
		u, err := m.member()
		if err != nil {
			return nil, err
		}
		if !m.ownerGone() {
			return nil, E(http.StatusForbidden, "owner_active", "A server can only be transferred here when its owner is deleted, disabled or banned.")
		}
		// the owner sees every channel, so the new one can be nobody who must
		// not gain access: not the owner of Nexa, staff, an app or a linked account
		if m.world.privileged[u.ID] || u.ID == m.me.ID {
			return nil, errManageOpensAccess
		}
		return &managePlan{handler: s.handleServerTransfer, method: "POST", path: map[string]string{"id": sid}, body: map[string]any{"user_id": u.ID}, audit: "server_transfer", targetType: "user", target: &u.ID, targetName: u.Username, before: map[string]any{"owner_id": m.owner.ID, "owner_username": m.owner.Username}, subject: u, notice: "heir"}, nil
	}
	return nil, E(http.StatusBadRequest, "unknown_action", "That is not something the owner can do here.")
}

func (m *manageCall) rule() (*store.AutomodRule, error) {
	if m.req.RuleID == nil {
		return nil, errManageTarget
	}
	rule, err := m.s.Store.AutomodRule(m.ctx, *m.req.RuleID)
	if err != nil || rule.ServerID != m.srv.ID {
		return nil, errManageTarget
	}
	return rule, nil
}

var errManageAlerts = E(http.StatusForbidden, "automod_alert", "A rule that posts what it catches can only be turned off or deleted here: it would copy private messages somewhere they can be read.")

// automodAlerts says whether a rule's actions post what it catches. The list
// is parsed strictly: anything unreadable counts as posting.
func automodAlerts(raw json.RawMessage) bool {
	var list []struct {
		Type string `json:"type"`
	}
	if len(raw) == 0 {
		return false
	}
	if json.Unmarshal(raw, &list) != nil {
		return true
	}
	for _, a := range list {
		if !OneOf(a.Type, actBlock, actTimeout, actQuarantine) {
			return true
		}
	}
	return false
}

func automodFieldsSafe(fields map[string]json.RawMessage) error {
	if v, ok := fields["actions"]; ok && automodAlerts(v) {
		return errManageAlerts
	}
	return nil
}

// errManageNotRecorded is a change that was made and could not be written up.
var errManageNotRecorded = E(http.StatusInternalServerError, "owner_not_recorded", "The change was made, but the record of it could not be written.")

func (s *Server) manageStage(stage string) {
	if s.manageHook != nil {
		s.manageHook(stage)
	}
}

// captured keeps a handler's answer from reaching the panel, so nothing a
// handler returns for its usual caller (key actions, say) leaves here.
type captured struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *captured) Header() http.Header { return c.header }
func (c *captured) WriteHeader(n int) {
	if c.status == 0 {
		c.status = n
	}
}
func (c *captured) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(b)
}

func (s *Server) runManaged(r *http.Request, act *ownerActing, p *managePlan) (*captured, error) {
	var raw []byte
	if p.body != nil {
		var err error
		if raw, err = json.Marshal(p.body); err != nil {
			return nil, err
		}
	}
	sub := r.Clone(context.WithValue(r.Context(), ownerActingKey{}, act))
	sub.Method = p.method
	sub.Body = http.NoBody
	sub.ContentLength = 0
	if raw != nil {
		sub.Body = readCloser{bytes.NewReader(raw)}
		sub.ContentLength = int64(len(raw))
	}
	sub.URL.RawQuery = ""
	if p.query != nil {
		sub.URL.RawQuery = p.query.Encode()
	}
	sub.Header.Del("X-Audit-Log-Reason")
	for k, v := range p.path {
		sub.SetPathValue(k, v)
	}
	c := &captured{header: http.Header{}}
	p.handler(c, sub)
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c, nil
}

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }

func (s *Server) handleModManage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	mod := s.modServerFor(w, r)
	if mod == nil {
		return
	}
	var req manageRequest
	if err := ReadJSON(w, r, &req, 256<<10); err != nil {
		WriteError(w, err)
		return
	}
	reason, ok := readReason(w, req.Reason)
	if !ok {
		return
	}
	req.Reason = reason

	// Everything from reading the server to the final record happens with its
	// gate held, so what the guard reads is what the change is made to. Past
	// this point a client going away cannot leave an action half done.
	held, unlock, err := s.Store.LockServerEdits(ctx, mod.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer unlock()
	r = r.WithContext(context.WithoutCancel(held))
	ctx = r.Context()
	srv, err := s.Store.ServerByID(ctx, mod.ID)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	owner, err := s.Store.UserByID(ctx, srv.OwnerID)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	// a server whose owner's account is gone still needs looking after
	if code := s.moderationBlock(ctx, me, owner); code != "" && code != blockDeleted {
		WriteError(w, serverBlockError(code))
		return
	}
	world, err := s.loadManageWorld(ctx, srv, me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	call := &manageCall{s: s, ctx: ctx, srv: srv, owner: owner, me: me, req: &req, world: world}
	plan, err := call.plan()
	if err != nil {
		if errors.Is(err, errManageOpensAccess) {
			s.logManageRefused(ctx, me, srv, &req, reason)
		}
		WriteError(w, err)
		return
	}
	if plan.mutate != nil {
		after := world.clone()
		plan.mutate(after)
		if world.gained(after) {
			s.logManageRefused(ctx, me, srv, &req, reason)
			WriteError(w, errManageOpensAccess)
			return
		}
	}

	action := store.LogOwnerServerPrefix + req.Action
	details := func(status string, extra map[string]json.RawMessage) json.RawMessage {
		d := map[string]any{"server_id": srv.ID, "server_name": srv.Name, "what": req.Action, "target": plan.targetName, "before": plan.before, "status": status}
		if extra != nil {
			d["after"] = extra
		}
		return mustJSON(d)
	}
	changes := func(extra map[string]json.RawMessage) json.RawMessage {
		c := map[string]any{"nexa_owner": true, "before": plan.before}
		for k, v := range extra {
			if k != "nexa_owner" && k != "before" {
				c[k] = v
			}
		}
		return mustJSON(c)
	}
	begin := &store.ModLogEntry{Action: store.LogOwnerServerBegin, ModeratorID: me.ID, ModeratorUsername: me.Username, TargetID: &owner.ID, TargetUsername: owner.Username, Reason: reason, Details: details("started", nil)}
	rec := &store.OwnerManage{
		Log:   &store.ModLogEntry{Action: action, ModeratorID: me.ID, ModeratorUsername: me.Username, TargetID: &owner.ID, TargetUsername: owner.Username, Reason: reason},
		Audit: &store.AuditEntry{ServerID: srv.ID, ActorID: me.ID, Action: plan.audit, TargetType: plan.targetType, TargetID: plan.target, Reason: reason},
	}
	notice := func(u *store.User, kind string) *store.ModCase {
		return &store.ModCase{UserID: u.ID, ModeratorID: me.ID, Action: store.ActionServerManage, Reason: reason, TargetUsername: u.Username, TargetDisplayName: u.DisplayName,
			Details: mustJSON(map[string]any{"server_id": srv.ID, "server_name": srv.Name, "what": req.Action, "notice": kind, "target": plan.targetName})}
	}
	if owner.Flags&store.FlagDeleted == 0 {
		rec.Cases = append(rec.Cases, notice(owner, "owner"))
	}
	if plan.subject != nil {
		rec.Cases = append(rec.Cases, notice(plan.subject, plan.notice))
	}
	s.manageStage("validated")
	// only the attempt is written first: an action that cannot say it was tried
	// does not go ahead, and the server hears of nothing until it has happened
	if err := s.Store.AppendModLog(ctx, begin); err != nil {
		WriteError(w, err)
		return
	}

	act := &ownerActing{serverID: srv.ID, actor: me.ID, changes: map[string]json.RawMessage{}}
	res, err := s.runManaged(r, act, plan)
	if err != nil || res.status >= 400 {
		rec.Log.Details = details("failed", nil)
		if lerr := s.Store.AppendModLog(ctx, rec.Log); lerr != nil {
			s.Log.Error("owner server management: log failure", "err", lerr)
		} else {
			s.emitLog(ctx, rec.Log)
		}
		if err != nil {
			WriteError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.status)
		_, _ = w.Write(res.body.Bytes())
		return
	}
	s.manageStage("changed")
	rec.Log.Details = details("done", act.changes)
	if err := s.Store.OwnerManageRecord(ctx, rec, changes(act.changes)); err != nil {
		// the change stands, with no notice and no entry in the server's log;
		// the staff log is the last place left to say so
		s.Log.Error("owner server management: record", "server", srv.ID, "action", req.Action, "err", err)
		rec.Log.Details = details("done_not_recorded", act.changes)
		if lerr := s.Store.AppendModLog(ctx, rec.Log); lerr != nil {
			s.Log.Error("owner server management: log unrecorded change", "server", srv.ID, "action", req.Action, "err", lerr)
		} else {
			s.emitLog(ctx, rec.Log)
		}
		WriteError(w, errManageNotRecorded)
		return
	}
	s.emitServerWithPerm(ctx, srv.ID, perms.ViewAuditLog, "audit_log_entry", rec.Audit)
	for _, c := range rec.Cases {
		s.notifyMember(ctx, c)
		s.emitCaseStaff(ctx, c)
	}
	s.emitLog(ctx, rec.Log)
	if req.Action == "transfer" {
		s.announceTransfer(r, srv, me, plan.subject, world)
	}
	s.writeManageView(w, r, srv.ID)
}

func (s *Server) logManageRefused(ctx context.Context, me *store.User, srv *store.Server, req *manageRequest, reason string) {
	err := s.Store.AppendModLog(ctx, &store.ModLogEntry{Action: store.LogOwnerServerRefused, ModeratorID: me.ID, ModeratorUsername: me.Username, Reason: reason,
		Details: mustJSON(map[string]any{"server_id": srv.ID, "server_name": srv.Name, "what": req.Action, "status": "refused"})})
	if err != nil {
		s.Log.Warn("owner server management: log refusal", "err", err)
	}
}

// announceTransfer says in the server who gave it a new owner, in the system
// channel or else the first text channel everyone can see.
func (s *Server) announceTransfer(r *http.Request, srv *store.Server, me, heir *store.User, world *manageWorld) {
	ctx := r.Context()
	chans, err := s.Store.ServerChannels(ctx, srv.ID)
	if err != nil {
		return
	}
	var where *store.Channel
	for _, c := range chans {
		wc := world.channel(c.ID)
		if c.Type != "text" || wc == nil || world.isPrivate(wc) {
			continue
		}
		if srv.SystemChannelID != nil && *srv.SystemChannelID == c.ID {
			where = c
			break
		}
		if where == nil || c.Position < where.Position {
			where = c
		}
	}
	if where != nil {
		s.systemMessage(r, where, me.ID, "system_server_transfer", []ids.ID{heir.ID}, nil)
	}
}

// --- what the panel shows -------------------------------------------------

type manageChannelView struct {
	ID              ids.ID            `json:"id"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	ParentID        *ids.ID           `json:"parent_id"`
	Position        int               `json:"position"`
	Private         bool              `json:"private"`
	Topic           string            `json:"topic,omitempty"`
	NSFW            bool              `json:"nsfw"`
	SlowmodeSeconds int               `json:"slowmode_seconds"`
	UserLimit       int               `json:"user_limit"`
	Overwrites      []store.Overwrite `json:"overwrites"`
}

type manageWebhookView struct {
	ID        ids.ID `json:"id"`
	Name      string `json:"name"`
	ChannelID ids.ID `json:"channel_id"`
}

type manageAppView struct {
	ID       ids.ID `json:"id"`
	Name     string `json:"name"`
	ReadsAll bool   `json:"reads_all"`
}

type manageRuleView struct {
	ID      ids.ID `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
	Alerts  bool   `json:"alerts"`
	// the words of a word rule, so that the rule can be edited
	Words []string `json:"words,omitempty"`
}

type manageBanView struct {
	User   store.PublicUser `json:"user"`
	Reason string           `json:"reason"`
}

// handleModManageView opens the page. Opening it is logged before anything is
// read, since it names private channels.
func (s *Server) handleModManageView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	mod := s.modServerFor(w, r)
	if mod == nil {
		return
	}
	log := &store.ModLogEntry{Action: store.LogOwnerServerView, ModeratorID: me.ID, ModeratorUsername: me.Username, TargetID: &mod.OwnerID,
		Details: mustJSON(map[string]any{"server_id": mod.ID, "server_name": mod.Name})}
	if err := s.Store.AppendModLog(ctx, log); err != nil {
		WriteError(w, err)
		return
	}
	s.emitLog(ctx, log)
	s.writeManageView(w, r, mod.ID)
}

func (s *Server) writeManageView(w http.ResponseWriter, r *http.Request, serverID ids.ID) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	srv, err := s.Store.ServerByID(ctx, serverID)
	if err != nil {
		// a change can end with the server gone from under it
		WriteJSON(w, http.StatusOK, map[string]any{"gone": true})
		return
	}
	owner, err := s.Store.UserByID(ctx, srv.OwnerID)
	if err != nil {
		WriteError(w, err)
		return
	}
	world, err := s.loadManageWorld(ctx, srv, me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	roles, err := s.Store.Roles(ctx, srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	chans, err := s.Store.ServerChannels(ctx, srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	channels := make([]manageChannelView, 0, len(chans))
	for _, c := range chans {
		v := manageChannelView{ID: c.ID, Name: c.Name, Type: c.Type, ParentID: c.ParentID, Position: c.Position, NSFW: c.NSFW, SlowmodeSeconds: c.SlowmodeSeconds, UserLimit: c.UserLimit, Overwrites: c.Overwrites}
		if v.Overwrites == nil {
			v.Overwrites = []store.Overwrite{}
		}
		if wc := world.channel(c.ID); wc == nil || world.isPrivate(wc) {
			v.Private = true
		} else {
			v.Topic = c.Topic
		}
		channels = append(channels, v)
	}
	bans, err := s.Store.Bans(ctx, srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	banned := make([]ids.ID, len(bans))
	for i, b := range bans {
		banned[i] = b.UserID
	}
	banUsers, err := s.Store.UsersByIDs(ctx, banned)
	if err != nil {
		WriteError(w, err)
		return
	}
	banViews := make([]manageBanView, 0, len(bans))
	for _, b := range bans {
		if u := banUsers[b.UserID]; u != nil {
			banViews = append(banViews, manageBanView{User: u.Public(), Reason: b.Reason})
		}
	}
	invites, err := s.Store.InvitesForServer(ctx, srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if invites == nil {
		invites = []*store.Invite{}
	}
	vanity := ""
	if in, err := s.Store.VanityInvite(ctx, srv.ID); err == nil && in != nil {
		vanity = in.Code
	}
	hooks, err := s.Store.WebhooksByServer(ctx, srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	webhooks := make([]manageWebhookView, 0, len(hooks))
	for _, h := range hooks {
		webhooks = append(webhooks, manageWebhookView{ID: h.ID, Name: h.Name, ChannelID: h.ChannelID})
	}
	installs, err := s.Store.GuildInstalls(ctx, srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	apps := make([]manageAppView, 0, len(installs))
	for _, in := range installs {
		app, err := s.Store.ApplicationByID(ctx, in.ApplicationID)
		if err != nil {
			continue
		}
		v := manageAppView{ID: app.ID, Name: app.Name}
		if bot, err := s.Store.UserByID(ctx, in.ApplicationID); err == nil {
			v.ReadsAll = bot.Flags&store.FlagBotReadsAll != 0
		}
		apps = append(apps, v)
	}
	list, err := s.Store.AutomodRules(ctx, srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	rules := make([]manageRuleView, 0, len(list))
	for _, ru := range list {
		v := manageRuleView{ID: ru.ID, Name: ru.Name, Kind: ru.Kind, Enabled: ru.Enabled, Alerts: automodAlerts(ru.Actions)}
		if ru.Kind == "words" {
			var cfg ruleConfig
			if json.Unmarshal(ru.Config, &cfg) == nil {
				v.Words = cfg.Words
			}
		}
		rules = append(rules, v)
	}
	call := &manageCall{s: s, ctx: ctx, srv: srv, owner: owner, me: me, world: world}
	WriteJSON(w, http.StatusOK, map[string]any{
		"server": map[string]any{"id": srv.ID, "name": srv.Name, "description": srv.Description, "icon_id": srv.IconID, "banner_id": srv.BannerID, "nsfw": srv.NSFW, "system_channel_id": srv.SystemChannelID, "default_notifications": srv.DefaultNotifications, "invites_paused_until": srv.InvitesPausedUntil},
		"owner":  owner.Public(), "owner_gone": call.ownerGone(), "owner_deleted": owner.Flags&store.FlagDeleted != 0,
		"channels": channels, "roles": roles, "bans": banViews, "invites": invites, "vanity": vanity, "webhooks": webhooks, "apps": apps, "automod": rules,
	})
}

// handleModManageMembers finds members for the page: names and roles only.
func (s *Server) handleModManageMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mod := s.modServerFor(w, r)
	if mod == nil {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n < 100 {
		limit = n
	}
	members, err := s.Store.Members(ctx, mod.ID, 0, limit, q)
	if err != nil {
		WriteError(w, err)
		return
	}
	who := make([]ids.ID, len(members))
	for i, m := range members {
		who[i] = m.UserID
	}
	users, err := s.Store.UsersByIDs(ctx, who)
	if err != nil {
		WriteError(w, err)
		return
	}
	type row struct {
		User    store.PublicUser `json:"user"`
		RoleIDs []ids.ID         `json:"role_ids"`
	}
	out := make([]row, 0, len(members))
	for _, m := range members {
		if u := users[m.UserID]; u != nil {
			roles := m.RoleIDs
			if roles == nil {
				roles = []ids.ID{}
			}
			out = append(out, row{User: u.Public(), RoleIDs: roles})
		}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"members": out})
}
