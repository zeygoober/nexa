package api

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/media"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/ratelimit"
	"evilion.xyz/chat/internal/store"
)

// Incoming webhooks. A webhook is a secret URL that posts into one server
// channel, for programs that have something to say there: a build finishing,
// a feed updating, a repository changing. The URL takes the same requests as
// a Discord webhook URL does, including its Slack- and GitHub-compatible
// forms, so anything written for those works when pointed here.
//
// What a webhook sends reaches this server readable, so it is stored as it
// came and never end-to-end encrypted; the app marks every such message.

func (s *Server) webhookRoutes(mux *http.ServeMux) {
	// managing webhooks, from the app
	mux.Handle("POST /api/v1/channels/{id}/webhooks", s.userLimited(s.limHookMgr, s.handleWebhookCreate))
	mux.Handle("GET /api/v1/channels/{id}/webhooks", s.authed(s.handleChannelWebhooks))
	mux.Handle("GET /api/v1/servers/{id}/webhooks", s.authed(s.handleServerWebhooks))
	mux.Handle("GET /api/v1/webhooks/{wid}", s.authed(s.handleWebhookGet))
	mux.Handle("PATCH /api/v1/webhooks/{wid}", s.userLimited(s.limHookMgr, s.handleWebhookPatch))
	mux.Handle("DELETE /api/v1/webhooks/{wid}", s.userLimited(s.limHookMgr, s.handleWebhookDelete))
	// using one, with its token: at /api/webhooks/..., and at the versioned
	// paths Discord's libraries build when given this server as their base
	for _, prefix := range []string{"/api", "/api/v1", "/api/v6", "/api/v7", "/api/v8", "/api/v9", "/api/v10"} {
		base := prefix + "/webhooks/{wid}/{token}"
		mux.Handle("GET "+base, s.hookRoute(s.handleWebhookTokenGet))
		mux.Handle("PATCH "+base, s.hookRoute(s.handleWebhookTokenPatch))
		mux.Handle("DELETE "+base, s.hookRoute(s.handleWebhookTokenDelete))
		// an application's id with an interaction's token is a followup
		mux.Handle("POST "+base, s.hookRoute(s.orFollowup(s.handleWebhookExecute(""), s.followupCreate)))
		mux.Handle("POST "+base+"/slack", s.hookRoute(s.handleWebhookExecute("slack")))
		mux.Handle("POST "+base+"/github", s.hookRoute(s.handleWebhookExecute("github")))
		mux.Handle("GET "+base+"/messages/{mid}", s.hookRoute(s.orFollowup(s.handleWebhookMessageGet, s.followupGet)))
		mux.Handle("PATCH "+base+"/messages/{mid}", s.hookRoute(s.orFollowup(s.handleWebhookMessageEdit, s.followupEdit)))
		mux.Handle("DELETE "+base+"/messages/{mid}", s.hookRoute(s.orFollowup(s.handleWebhookMessageDelete, s.followupDelete)))
		for _, p := range []string{base, base + "/slack", base + "/github", base + "/messages/{mid}"} {
			mux.Handle("OPTIONS "+p, s.hookRoute(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		}
	}
}

// hookRoute is a route that takes a webhook's token instead of a session.
// Browser tools that build webhook messages post straight to the URL, and the
// token in it is the only credential (no cookie ever goes along), so any page
// may call it.
func (s *Server) hookRoute(h http.HandlerFunc) http.Handler {
	return ipLimited(s.limGeneric, func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Access-Control-Allow-Origin", "*")
		hd.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		hd.Set("Access-Control-Allow-Headers", "Content-Type, X-Audit-Log-Reason, X-GitHub-Event, X-GitHub-Delivery")
		hd.Set("Access-Control-Expose-Headers", "Retry-After, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, X-RateLimit-Reset-After, X-RateLimit-Bucket, X-RateLimit-Scope")
		hd.Set("Access-Control-Max-Age", "86400")
		h(w, r)
	})
}

// --- tokens -----------------------------------------------------------------------

// newWebhookToken is a webhook URL's secret: 48 random bytes, as 64
// characters of URL-safe base64.
func newWebhookToken() string {
	b := make([]byte, 48)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func webhookTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// sealWebhookToken keeps a token so the people who manage the webhook can
// copy its URL again later, and nobody who only has the database can.
func (s *Server) sealWebhookToken(token string) ([]byte, error) {
	gcm, err := s.webhookSeal()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(token), nil), nil
}

func (s *Server) openWebhookToken(sealed []byte) (string, error) {
	gcm, err := s.webhookSeal()
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("sealed token is truncated")
	}
	out, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	return string(out), err
}

func (s *Server) webhookSeal() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.derivedKey("webhook-token-v1"))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// webhookURL is where a webhook takes messages.
func (s *Server) webhookURL(id ids.ID, token string) string {
	return strings.TrimRight(s.Cfg.PublicURL, "/") + "/api/webhooks/" + id.String() + "/" + token
}

// --- the author account ---------------------------------------------------------------

var webhookAuthorMu sync.Mutex

// webhookUser is the account webhook messages are filed under, made the first
// time a webhook needs it.
func (s *Server) webhookUser(ctx context.Context) (*store.User, error) {
	webhookAuthorMu.Lock()
	defer webhookAuthorMu.Unlock()
	u, err := s.Store.WebhookUser(ctx)
	if errors.Is(err, store.ErrNotFound) {
		b := make([]byte, 48)
		_, _ = rand.Read(b)
		u = &store.User{Username: store.WebhookUsername, DisplayName: "Webhook", AuthHash: "!", KDFSalt: b[:16], KDFParams: defaultKDF,
			PublicKey: b[16:48], EncPrivateKey: []byte{}, Flags: store.FlagWebhook, DMPrivacy: store.DMNobody}
		err = s.Store.CreateUser(ctx, u)
		if errors.Is(err, store.ErrUsernameTaken) {
			// another process made it a moment ago
			u, err = s.Store.WebhookUser(ctx)
		}
	}
	return u, err
}

// --- the webhook object ------------------------------------------------------------------

// webhookView is a webhook as Discord's API shows one, with this server's
// own names alongside (server_id, avatar_id). The token and URL are only for
// people who may manage it, and for its own token's holder.
func (s *Server) webhookView(ctx context.Context, wh *store.Webhook, withToken, withUser bool) map[string]any {
	var avatar any
	if wh.AvatarID != nil {
		avatar = wh.AvatarID.String()
	}
	v := map[string]any{
		"id": wh.ID.String(), "type": wh.Type, "guild_id": wh.ServerID.String(), "server_id": wh.ServerID.String(),
		"channel_id": wh.ChannelID.String(), "name": wh.Name, "avatar": avatar, "avatar_id": avatar, "application_id": nil,
		"created_at": wh.CreatedAt,
	}
	if withToken {
		if token, err := s.openWebhookToken(wh.TokenSealed); err == nil {
			v["token"] = token
			v["url"] = s.webhookURL(wh.ID, token)
		} else {
			s.Log.Warn("webhook: token does not open", "webhook", wh.ID.String(), "err", err)
		}
	}
	if withUser && wh.CreatorID != nil {
		if u, err := s.Store.UserByID(ctx, *wh.CreatorID); err == nil && u.Flags&store.FlagDeleted == 0 {
			pu := u.Public()
			v["user"] = map[string]any{"id": pu.ID.String(), "username": pu.Username, "global_name": pu.DisplayName, "display_name": pu.DisplayName, "avatar": pu.AvatarID, "avatar_id": pu.AvatarID}
		}
	}
	return v
}

// --- managing, from the app ------------------------------------------------------------

// webhookChannel loads a channel webhooks can live in (a server's text
// channel) and checks the caller may manage webhooks there.
func (s *Server) webhookChannel(w http.ResponseWriter, r *http.Request, id ids.ID) (*store.Channel, *store.User) {
	me := auth.UserFrom(r.Context())
	ch, err := s.Store.ChannelByID(r.Context(), id)
	if err != nil || ch.ServerID == nil {
		WriteError(w, ErrNotFound)
		return nil, nil
	}
	ok, err := s.canAccess(r, me, ch, NeedWebhooks)
	if actingOwner(r.Context(), *ch.ServerID, me.ID) {
		ok, err = true, nil
	}
	if err != nil {
		WriteError(w, err)
		return nil, nil
	}
	if !ok {
		if canView, _ := s.canAccess(r, me, ch, NeedView); canView {
			WriteError(w, E(http.StatusForbidden, "missing_permission", "You need Manage Webhooks in this channel."))
			return nil, nil
		}
		WriteError(w, ErrNotFound)
		return nil, nil
	}
	if ch.Type != "text" {
		WriteError(w, E(http.StatusBadRequest, "not_text", "Webhooks can only post in text channels."))
		return nil, nil
	}
	return ch, me
}

// cleanWebhookName checks a webhook's name: 1 to 80 characters on one line,
// and not a name only Nexa's own account may use.
func cleanWebhookName(raw string) (string, *Error) {
	name, _ := CleanText(raw, 1<<12)
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || utf8.RuneCountInString(name) > whMaxName {
		return "", E(http.StatusBadRequest, "bad_name", "A webhook's name is 1 to 80 characters.")
	}
	if reservedDisplayName(name) {
		return "", E(http.StatusBadRequest, "reserved_name", "That name belongs to Nexa's own account.")
	}
	return name, nil
}

// webhookAvatar stores an avatar sent as image data ("data:image/png;base64,
// ..."), as Discord's API takes one, no larger than a webhook picture. A
// picture already that small is kept as it is, so one the staff took down
// still matches; a larger one is cut square here, and a moving one stays
// moving. It returns the blob, holding a reference for its caller.
func (s *Server) webhookAvatar(ctx context.Context, data string, owner ids.ID) (*store.Blob, error) {
	head, body, ok := strings.Cut(data, ",")
	if !ok || !strings.HasPrefix(head, "data:image/") || !strings.HasSuffix(head, ";base64") {
		return nil, E(http.StatusBadRequest, "bad_avatar", "The avatar must be image data: data:image/png;base64,...")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body))
	if err != nil {
		if raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(strings.TrimSpace(body), "=")); err != nil {
			return nil, E(http.StatusBadRequest, "bad_avatar", "The avatar's base64 could not be read.")
		}
	}
	lim := s.limitsFor("avatar")
	if int64(len(raw)) > lim.MaxBytes {
		return nil, media.ErrTooLarge
	}
	width, height, err := media.ImageDims(bytes.NewReader(raw))
	if err != nil {
		return nil, media.ErrUnsupported
	} else if err := lim.Check(width, height); err != nil {
		return nil, err
	}
	out := raw
	if width > webhookAvatarSide || height > webhookAvatarSide {
		var err error
		out, _, err = media.Crop(raw, media.CropSpec{MaxW: webhookAvatarSide, MaxH: webhookAvatarSide})
		if err != nil {
			return nil, cropError(err)
		}
	}
	return s.Media.PutPublic(ctx, "avatar", bytes.NewReader(out), media.Limits{MaxBytes: int64(len(out)) + 1, MaxW: webhookAvatarSide, MaxH: webhookAvatarSide}, owner)
}

// webhookChanged tells whoever manages a server's webhooks that its list
// changed, as Discord's WEBHOOKS_UPDATE does.
func (s *Server) webhookChanged(ctx context.Context, serverID, channelID ids.ID) {
	s.emitServerWithPerm(ctx, serverID, perms.ManageWebhooks, "webhooks_update", map[string]any{"server_id": serverID, "guild_id": serverID, "channel_id": channelID})
}

func auditReason(r *http.Request) string {
	raw := r.Header.Get("X-Audit-Log-Reason")
	if raw == "" {
		return ""
	}
	if dec, err := url.QueryUnescape(raw); err == nil {
		raw = dec
	}
	out, _ := CleanText(raw, 512)
	return out
}

func (s *Server) handleWebhookCreate(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	ch, me := s.webhookChannel(w, r, id)
	if ch == nil {
		return
	}
	var req struct {
		Name   string  `json:"name"`
		Avatar *string `json:"avatar"`
	}
	if err := ReadJSON(w, r, &req, (s.Cfg.MaxAvatarBytes*4)/3+16<<10); err != nil {
		WriteError(w, err)
		return
	}
	name, apiErr := cleanWebhookName(req.Name)
	if apiErr != nil {
		WriteError(w, apiErr)
		return
	}
	ctx := r.Context()
	if _, err := s.webhookUser(ctx); err != nil {
		WriteError(w, err)
		return
	}
	token := newWebhookToken()
	sealed, err := s.sealWebhookToken(token)
	if err != nil {
		WriteError(w, err)
		return
	}
	creator := me.ID
	wh := &store.Webhook{ServerID: *ch.ServerID, ChannelID: ch.ID, CreatorID: &creator, Name: name, TokenHash: webhookTokenHash(token), TokenSealed: sealed}
	if req.Avatar != nil && *req.Avatar != "" {
		b, err := s.webhookAvatar(ctx, *req.Avatar, me.ID)
		if err != nil {
			WriteError(w, err)
			return
		}
		wh.AvatarID = &b.ID
	}
	if err := s.Store.CreateWebhook(ctx, wh); err != nil {
		if wh.AvatarID != nil {
			s.releaseBlob(ctx, *wh.AvatarID)
		}
		if errors.Is(err, store.ErrTooMany) {
			WriteError(w, E(http.StatusBadRequest, "too_many_webhooks", fmt.Sprintf("A channel can have at most %d webhooks.", store.MaxWebhooksPerChannel)))
			return
		}
		WriteError(w, err)
		return
	}
	s.audit(r, wh.ServerID, me.ID, "webhook_create", "webhook", &wh.ID, map[string]any{"name": wh.Name, "channel_id": wh.ChannelID}, auditReason(r))
	s.webhookChanged(ctx, wh.ServerID, wh.ChannelID)
	WriteJSON(w, http.StatusOK, s.webhookView(ctx, wh, true, true))
}

func (s *Server) handleChannelWebhooks(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	ch, _ := s.webhookChannel(w, r, id)
	if ch == nil {
		return
	}
	list, err := s.Store.WebhooksByChannel(r.Context(), ch.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, wh := range list {
		out = append(out, s.webhookView(r.Context(), wh, true, true))
	}
	WriteJSON(w, http.StatusOK, out)
}

// handleServerWebhooks lists the webhooks of every channel in the server in
// which the caller may manage webhooks.
func (s *Server) handleServerWebhooks(w http.ResponseWriter, r *http.Request) {
	srv, me, _ := s.serverCtx(w, r, 0)
	if srv == nil {
		return
	}
	list, err := s.Store.WebhooksByServer(r.Context(), srv.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	allowed := map[ids.ID]bool{}
	out := make([]map[string]any, 0, len(list))
	for _, wh := range list {
		ok, seen := allowed[wh.ChannelID]
		if !seen {
			if ch, err := s.Store.ChannelByID(r.Context(), wh.ChannelID); err == nil {
				ok, _ = s.canAccess(r, me, ch, NeedWebhooks)
			}
			allowed[wh.ChannelID] = ok
		}
		if ok {
			out = append(out, s.webhookView(r.Context(), wh, true, true))
		}
	}
	WriteJSON(w, http.StatusOK, out)
}

// managedWebhook loads the webhook in the path for someone who may manage it.
func (s *Server) managedWebhook(w http.ResponseWriter, r *http.Request) (*store.Webhook, *store.Channel, *store.User) {
	id, err := ids.Parse(r.PathValue("wid"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return nil, nil, nil
	}
	wh, err := s.Store.WebhookByID(r.Context(), id)
	if err != nil {
		WriteError(w, ErrNotFound)
		return nil, nil, nil
	}
	ch, me := s.webhookChannel(w, r, wh.ChannelID)
	if ch == nil {
		return nil, nil, nil
	}
	return wh, ch, me
}

func (s *Server) handleWebhookGet(w http.ResponseWriter, r *http.Request) {
	wh, _, _ := s.managedWebhook(w, r)
	if wh == nil {
		return
	}
	WriteJSON(w, http.StatusOK, s.webhookView(r.Context(), wh, true, true))
}

// webhookPatch is a change to a webhook: its name, its avatar (image data, or
// null to take it away) and, from the app, its channel.
type webhookPatch struct {
	Name      *string         `json:"name"`
	Avatar    json.RawMessage `json:"avatar"`
	ChannelID *ids.ID         `json:"channel_id"`
}

// applyWebhookPatch changes a webhook. It returns what changed, for the audit
// log, or an error to send back as the caller's kind of error.
func (s *Server) applyWebhookPatch(ctx context.Context, wh *store.Webhook, p *webhookPatch, owner ids.ID) (map[string]any, error) {
	changes := map[string]any{}
	patch := store.WebhookPatch{}
	if p.Name != nil {
		name, apiErr := cleanWebhookName(*p.Name)
		if apiErr != nil {
			return nil, apiErr
		}
		if name != wh.Name {
			patch.Name = &name
			changes["name"] = name
		}
	}
	if p.ChannelID != nil && *p.ChannelID != wh.ChannelID {
		patch.ChannelID = p.ChannelID
		changes["channel_id"] = *p.ChannelID
	}
	if patch.Name != nil || patch.ChannelID != nil {
		if err := s.Store.UpdateWebhook(ctx, wh.ID, patch); err != nil {
			if errors.Is(err, store.ErrTooMany) {
				return nil, E(http.StatusBadRequest, "too_many_webhooks", fmt.Sprintf("A channel can have at most %d webhooks.", store.MaxWebhooksPerChannel))
			}
			return nil, err
		}
	}
	if len(p.Avatar) > 0 {
		var data *string
		if err := json.Unmarshal(p.Avatar, &data); err != nil {
			return nil, E(http.StatusBadRequest, "bad_avatar", "The avatar must be image data or null.")
		}
		var newID *ids.ID
		if data != nil && *data != "" {
			b, err := s.webhookAvatar(ctx, *data, owner)
			if err != nil {
				return nil, err
			}
			newID = &b.ID
		}
		if err := s.Store.SwapImage(ctx, "webhooks", "avatar_id", wh.ID, newID); err != nil {
			if newID != nil {
				s.releaseBlob(ctx, *newID)
			}
			return nil, err
		}
		changes["avatar"] = newID != nil
	}
	if _, renamed := changes["name"]; !renamed && len(changes) > 0 {
		// so the log can say which webhook it was
		changes["current_name"] = wh.Name
	}
	return changes, nil
}

func (s *Server) handleWebhookPatch(w http.ResponseWriter, r *http.Request) {
	wh, _, me := s.managedWebhook(w, r)
	if wh == nil {
		return
	}
	var p webhookPatch
	if err := ReadJSON(w, r, &p, (s.Cfg.MaxAvatarBytes*4)/3+16<<10); err != nil {
		WriteError(w, err)
		return
	}
	ctx := r.Context()
	oldChannel := wh.ChannelID
	if p.ChannelID != nil && *p.ChannelID != wh.ChannelID {
		// moving it needs Manage Webhooks where it goes, in the same server
		target, _ := s.webhookChannel(w, r, *p.ChannelID)
		if target == nil {
			return
		}
		if *target.ServerID != wh.ServerID {
			WriteError(w, E(http.StatusBadRequest, "bad_channel", "A webhook can only move within its server."))
			return
		}
	}
	changes, err := s.applyWebhookPatch(ctx, wh, &p, me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	updated, err := s.Store.WebhookByID(ctx, wh.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if len(changes) > 0 {
		s.audit(r, wh.ServerID, me.ID, "webhook_update", "webhook", &wh.ID, changes, auditReason(r))
		s.webhookChanged(ctx, wh.ServerID, updated.ChannelID)
		if oldChannel != updated.ChannelID {
			s.webhookChanged(ctx, wh.ServerID, oldChannel)
		}
	}
	WriteJSON(w, http.StatusOK, s.webhookView(ctx, updated, true, true))
}

func (s *Server) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	wh, _, me := s.managedWebhook(w, r)
	if wh == nil {
		return
	}
	if err := s.Store.DeleteWebhook(r.Context(), wh.ID); err != nil {
		WriteError(w, err)
		return
	}
	s.audit(r, wh.ServerID, me.ID, "webhook_delete", "webhook", &wh.ID, map[string]any{"name": wh.Name, "channel_id": wh.ChannelID}, auditReason(r))
	s.webhookChanged(r.Context(), wh.ServerID, wh.ChannelID)
	w.WriteHeader(http.StatusNoContent)
}

// --- using one, with its token ----------------------------------------------------------------

// tokenWebhook loads the webhook a token route names and checks its token,
// answering in Discord's shape when either is wrong.
func (s *Server) tokenWebhook(w http.ResponseWriter, r *http.Request) *store.Webhook {
	// the id in either form: as Nexa writes it in the address it hands out,
	// or as the bot API wrote it to a library
	id, err := parseBotID(r.PathValue("wid"))
	if err != nil || id.IsZero() {
		writeDiscordError(w, dErrUnknownWebhook)
		return nil
	}
	wh, err := s.Store.WebhookByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeDiscordError(w, dErrUnknownWebhook)
		} else {
			writeDiscordError(w, err)
		}
		return nil
	}
	token := r.PathValue("token")
	if len(token) > 256 || !hmac.Equal([]byte(webhookTokenHash(token)), []byte(wh.TokenHash)) {
		writeDiscordError(w, dErrBadToken)
		return nil
	}
	return wh
}

// hookAllow spends one of a webhook's requests (and, for one that posts,
// one of its channel's messages), answering a 429 the way Discord does when
// there is none to spend.
func (s *Server) hookAllow(w http.ResponseWriter, wh *store.Webhook, posts bool) bool {
	type bucket struct {
		lim   *ratelimit.Limiter
		key   string
		limit int
		name  string
	}
	buckets := []bucket{{s.limHook, "wh:" + wh.ID.String(), 5, "webhook-" + wh.ID.String()}}
	if posts {
		buckets = append(buckets, bucket{s.limHookCh, "whc:" + wh.ChannelID.String(), 30, "channel-" + wh.ChannelID.String()})
	}
	for _, b := range buckets {
		if ok, retry := b.lim.Allow(b.key); !ok {
			secs := retry.Seconds()
			if secs < 0.001 {
				secs = 0.001
			}
			hd := w.Header()
			hd.Set("Retry-After", retryAfter(retry))
			hd.Set("X-RateLimit-Limit", strconv.Itoa(b.limit))
			hd.Set("X-RateLimit-Remaining", "0")
			hd.Set("X-RateLimit-Reset", strconv.FormatFloat(float64(time.Now().Add(retry).UnixMilli())/1000, 'f', 3, 64))
			hd.Set("X-RateLimit-Reset-After", strconv.FormatFloat(secs, 'f', 3, 64))
			hd.Set("X-RateLimit-Bucket", b.name)
			hd.Set("X-RateLimit-Scope", "shared")
			// discord.py waits out a 429 only when it came through a proxy it
			// recognises by this header; without it the sender gives up at once
			hd.Set("Via", "1.1 nexa")
			WriteJSON(w, http.StatusTooManyRequests, map[string]any{"message": "You are being rate limited.", "retry_after": secs, "global": false})
			return false
		}
	}
	w.Header().Set("X-RateLimit-Limit", "5")
	w.Header().Set("X-RateLimit-Bucket", "webhook-"+wh.ID.String())
	return true
}

func (s *Server) handleWebhookTokenGet(w http.ResponseWriter, r *http.Request) {
	wh := s.tokenWebhook(w, r)
	if wh == nil || !s.hookAllow(w, wh, false) {
		return
	}
	WriteJSON(w, http.StatusOK, s.webhookView(r.Context(), wh, true, false))
}

func (s *Server) handleWebhookTokenPatch(w http.ResponseWriter, r *http.Request) {
	wh := s.tokenWebhook(w, r)
	if wh == nil || !s.hookAllow(w, wh, false) {
		return
	}
	var p webhookPatch
	if err := ReadJSON(w, r, &p, (s.Cfg.MaxAvatarBytes*4)/3+16<<10); err != nil {
		writeDiscordError(w, dErrBadJSON)
		return
	}
	// with its token a webhook can be renamed and given a picture, but not moved
	p.ChannelID = nil
	ctx := r.Context()
	author, err := s.webhookUser(ctx)
	if err != nil {
		writeDiscordError(w, err)
		return
	}
	changes, err := s.applyWebhookPatch(ctx, wh, &p, author.ID)
	if err != nil {
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest {
			f := &formErrors{}
			field := "name"
			if strings.Contains(apiErr.Code, "avatar") {
				field = "avatar"
			}
			f.add([]string{field}, strings.ToUpper(apiErr.Code), apiErr.Message)
			writeDiscordError(w, f.err())
			return
		}
		writeDiscordError(w, err)
		return
	}
	updated, err := s.Store.WebhookByID(ctx, wh.ID)
	if err != nil {
		writeDiscordError(w, err)
		return
	}
	if len(changes) > 0 {
		s.audit(r, wh.ServerID, author.ID, "webhook_update", "webhook", &wh.ID, changes, auditReason(r))
		s.webhookChanged(ctx, wh.ServerID, wh.ChannelID)
	}
	WriteJSON(w, http.StatusOK, s.webhookView(ctx, updated, true, false))
}

func (s *Server) handleWebhookTokenDelete(w http.ResponseWriter, r *http.Request) {
	wh := s.tokenWebhook(w, r)
	if wh == nil || !s.hookAllow(w, wh, false) {
		return
	}
	ctx := r.Context()
	if err := s.Store.DeleteWebhook(ctx, wh.ID); err != nil {
		writeDiscordError(w, err)
		return
	}
	if author, err := s.webhookUser(ctx); err == nil {
		s.audit(r, wh.ServerID, author.ID, "webhook_delete", "webhook", &wh.ID, map[string]any{"name": wh.Name, "channel_id": wh.ChannelID}, auditReason(r))
	}
	s.webhookChanged(ctx, wh.ServerID, wh.ChannelID)
	w.WriteHeader(http.StatusNoContent)
}

// boolQuery reads a boolean query parameter the way Discord does.
func boolQuery(r *http.Request, key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	switch v {
	case "true", "1", "yes", "y", "t", "on":
		return true
	case "false", "0", "no", "n", "f", "off":
		return false
	}
	return def
}

// whFile is a file that came with a request, already stored.
type whFile struct {
	N        string
	Filename string
	Blob     *store.Blob
}

// maxWebhookJSON bounds a request's JSON: the largest message Discord's limits
// allow is well under it.
const maxWebhookJSON = 1 << 20

// readWebhookBody reads a request as JSON, as multipart form data (a
// payload_json part, other fields and files), or as a plain form, storing the
// files as they arrive. What it returns is the JSON the request amounts to.
func (s *Server) readWebhookBody(w http.ResponseWriter, r *http.Request, owner ids.ID) (map[string]any, []whFile, *dError) {
	ct, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	decode := func(rd io.Reader) (map[string]any, *dError) {
		dec := json.NewDecoder(rd)
		dec.UseNumber()
		var raw map[string]any
		if err := dec.Decode(&raw); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				return nil, dErrTooLarge
			}
			if errors.Is(err, io.EOF) {
				return map[string]any{}, nil
			}
			return nil, dErrBadJSON
		}
		if raw == nil {
			raw = map[string]any{}
		}
		return raw, nil
	}
	switch ct {
	case "multipart/form-data":
		if params["boundary"] == "" {
			return nil, nil, dErrBadJSON
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.Cfg.MaxUploadBytes+maxWebhookJSON+64<<10)
		mr, err := r.MultipartReader()
		if err != nil {
			return nil, nil, dErrBadJSON
		}
		raw := map[string]any{}
		var files []whFile
		fail := func(e *dError) (map[string]any, []whFile, *dError) {
			s.dropFiles(files)
			return nil, nil, e
		}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					return fail(dErrTooLarge)
				}
				return fail(dErrBadJSON)
			}
			name := part.FormName()
			if part.FileName() != "" || strings.HasPrefix(name, "files[") {
				if len(files) >= whMaxFiles {
					part.Close()
					f := &formErrors{}
					f.add([]string{"files"}, "BASE_TYPE_MAX_LENGTH", fmt.Sprintf("Must be %d or fewer in length.", whMaxFiles))
					return fail(f.err())
				}
				n := strconv.Itoa(len(files))
				if inner, ok := strings.CutPrefix(name, "files["); ok && strings.HasSuffix(inner, "]") {
					n = strings.TrimSuffix(inner, "]")
				}
				b, err := s.Media.PutPlain(r.Context(), part, s.Cfg.MaxUploadBytes, owner, part.Header.Get("Content-Type"))
				part.Close()
				if err != nil {
					var maxErr *http.MaxBytesError
					if errors.Is(err, media.ErrTooLarge) {
						return fail(dErrFileTooLarge)
					}
					if errors.As(err, &maxErr) {
						return fail(dErrTooLarge)
					}
					s.Log.Warn("webhook: store file", "err", err)
					return fail(dErrInternal)
				}
				fname := part.FileName()
				if fname == "" {
					fname = "file"
				}
				files = append(files, whFile{N: n, Filename: cleanFilename(fname), Blob: b})
				continue
			}
			value, err := io.ReadAll(io.LimitReader(part, maxWebhookJSON+1))
			part.Close()
			if err != nil || len(value) > maxWebhookJSON {
				return fail(dErrTooLarge)
			}
			if name == "payload_json" {
				payload, derr := decode(bytes.NewReader(value))
				if derr != nil {
					return fail(derr)
				}
				for k, v := range payload {
					raw[k] = v
				}
				continue
			}
			if _, set := raw[name]; !set && name != "" {
				raw[name] = string(value)
			}
		}
		return raw, files, nil
	case "application/x-www-form-urlencoded":
		r.Body = http.MaxBytesReader(w, r.Body, maxWebhookJSON)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, nil, dErrTooLarge
		}
		// a JSON body sent without saying so, as curl -d does
		if t := bytes.TrimSpace(body); len(t) > 0 && t[0] == '{' {
			raw, derr := decode(bytes.NewReader(t))
			return raw, nil, derr
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, nil, dErrBadJSON
		}
		raw := map[string]any{}
		for k, v := range form {
			if len(v) > 0 {
				raw[k] = v[0]
			}
		}
		if p, ok := raw["payload_json"].(string); ok {
			payload, derr := decode(strings.NewReader(p))
			if derr != nil {
				return nil, nil, derr
			}
			delete(raw, "payload_json")
			for k, v := range payload {
				raw[k] = v
			}
		}
		return raw, nil, nil
	default:
		r.Body = http.MaxBytesReader(w, r.Body, maxWebhookJSON)
		raw, derr := decode(r.Body)
		return raw, nil, derr
	}
}

// dropFiles removes files stored for a request that then went nowhere.
func (s *Server) dropFiles(files []whFile) {
	for _, f := range files {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.Media.Remove(ctx, f.Blob); err != nil {
			s.Log.Warn("webhook: drop file", "err", err)
		}
		cancel()
	}
}

// fileAttachments turns a request's files into the message's attachments,
// with the names, descriptions and spoilers its attachments list gives them.
func fileAttachments(files []whFile, reqs []whAttachReq) ([]wmAttachment, []store.Attachment) {
	byID := map[string]whAttachReq{}
	for _, a := range reqs {
		byID[a.ID] = a
	}
	plain := make([]wmAttachment, 0, len(files))
	rows := make([]store.Attachment, 0, len(files))
	for _, f := range files {
		meta := byID[f.N]
		name := f.Filename
		if meta.Filename != "" {
			name = meta.Filename
		}
		a := wmAttachment{ID: ids.New(), BlobID: f.Blob.ID, Filename: name, ContentType: f.Blob.Mime, Size: f.Blob.Size, Width: f.Blob.Width, Height: f.Blob.Height,
			Spoiler: strings.HasPrefix(name, "SPOILER_"), Title: meta.Title, Description: meta.Description}
		if meta.Spoiler != nil {
			a.Spoiler = *meta.Spoiler
		}
		plain = append(plain, a)
		rows = append(rows, store.Attachment{ID: a.ID, BlobID: a.BlobID, Size: a.Size})
	}
	return plain, rows
}

// resolveMedia points a picture at one of the message's files (for an
// attachment://name reference) or at this server's proxy (for a link), so
// readers load it from here and never from whoever hosts it.
func (s *Server) resolveMedia(m *wmMedia, atts []wmAttachment) {
	if m == nil {
		return
	}
	if name, ok := strings.CutPrefix(m.URL, "attachment://"); ok {
		for i := range atts {
			if atts[i].Filename == name {
				a := &atts[i]
				a.Inline = true
				id, blob := a.ID, a.BlobID
				m.AttachmentID, m.BlobID = &id, &blob
				m.Width, m.Height, m.ContentType = a.Width, a.Height, a.ContentType
				return
			}
		}
		return
	}
	m.ProxyURL = s.ProxyURL(m.URL)
}

// resolveIcon is resolveMedia for an embed's author or footer icon.
func (s *Server) resolveIcon(u string, atts []wmAttachment) (proxy string, blob *ids.ID) {
	if u == "" {
		return "", nil
	}
	m := &wmMedia{URL: u}
	s.resolveMedia(m, atts)
	return m.ProxyURL, m.BlobID
}

// finishPlain resolves every picture of a message against its files and the
// proxy, and marks which files the embeds and components show.
func (s *Server) finishPlain(p *wmPlain, v2 bool) {
	for i := range p.Attachments {
		p.Attachments[i].Inline = false
	}
	for i := range p.Embeds {
		e := &p.Embeds[i]
		s.resolveMedia(e.Image, p.Attachments)
		s.resolveMedia(e.Thumbnail, p.Attachments)
		if e.Author != nil {
			e.Author.ProxyIconURL, e.Author.IconBlobID = s.resolveIcon(e.Author.IconURL, p.Attachments)
		}
		if e.Footer != nil {
			e.Footer.ProxyIconURL, e.Footer.IconBlobID = s.resolveIcon(e.Footer.IconURL, p.Attachments)
		}
	}
	var walk func(list []wmComponent)
	walk = func(list []wmComponent) {
		for i := range list {
			c := &list[i]
			s.resolveMedia(c.Media, p.Attachments)
			if c.File != nil {
				s.resolveMedia(c.File, p.Attachments)
				for _, a := range p.Attachments {
					if c.File.AttachmentID != nil && a.ID == *c.File.AttachmentID {
						c.Name, c.Size = a.Filename, a.Size
					}
				}
			}
			for j := range c.Items {
				s.resolveMedia(&c.Items[j].Media, p.Attachments)
			}
			walk(c.Components)
			if c.Accessory != nil {
				one := []wmComponent{*c.Accessory}
				walk(one)
				c.Accessory = &one[0]
			}
		}
	}
	walk(p.Components)
	if v2 {
		// a message laid out by components shows its files only through them
		for i := range p.Attachments {
			p.Attachments[i].Inline = true
		}
	}
	if p.AvatarURL != "" {
		p.AvatarProxy = s.ProxyURL(p.AvatarURL)
	}
}

// hasBody reports whether a message would show anything at all.
func (p *wmPlain) hasBody() bool {
	return strings.TrimSpace(p.Content) != "" || len(p.Embeds) > 0 || len(p.Components) > 0 || len(p.Attachments) > 0 || p.Poll != nil
}

// handleWebhookExecute posts a message through a webhook: as sent, or
// converted from Slack's or GitHub's format first (source).
func (s *Server) handleWebhookExecute(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wh := s.tokenWebhook(w, r)
		if wh == nil || !s.hookAllow(w, wh, true) {
			return
		}
		ctx := r.Context()
		author, err := s.webhookUser(ctx)
		if err != nil {
			writeDiscordError(w, err)
			return
		}
		// Slack's and GitHub's senders read the answer only for success, so
		// theirs wait for the message by default, as Discord's do
		wait := boolQuery(r, "wait", source != "")
		if r.URL.Query().Get("thread_id") != "" {
			writeDiscordError(w, dErrUnknownChannel)
			return
		}
		raw, files, derr := s.readWebhookBody(w, r, author.ID)
		if derr != nil {
			writeDiscordError(w, derr)
			return
		}
		keep := false
		defer func() {
			if !keep {
				s.dropFiles(files)
			}
		}()
		switch source {
		case "slack":
			raw = slackToWebhook(raw)
		case "github":
			event := r.Header.Get("X-GitHub-Event")
			converted, ok := githubToWebhook(event, raw)
			if !ok {
				// an event with nothing to say, or one that is not supported:
				// accepted, so the sender does not keep retrying, and dropped
				w.WriteHeader(http.StatusNoContent)
				return
			}
			raw = converted
		}
		in, errs := parseWebhookBody(raw, whParseOpts{withComponents: boolQuery(r, "with_components", false)})
		if !errs.empty() {
			writeDiscordError(w, errs.err())
			return
		}
		if in.ThreadName != "" {
			writeDiscordError(w, dErrThreadName)
			return
		}
		ch, err := s.Store.ChannelByID(ctx, wh.ChannelID)
		if err != nil {
			writeDiscordError(w, dErrUnknownChannel)
			return
		}
		if ch.Type != "text" || ch.ServerID == nil {
			writeDiscordError(w, dErrNotText)
			return
		}
		atts, rows := fileAttachments(files, in.Attach)
		p := &wmPlain{Name: wh.Name, AvatarID: wh.AvatarID, Content: in.Content, TTS: in.TTS, Embeds: in.Embeds, Components: in.Components, Attachments: atts, Source: source}
		if in.Username != "" {
			p.Name = in.Username
		}
		if in.AvatarURL != "" {
			p.AvatarURL, p.AvatarID = in.AvatarURL, nil
		}
		now := time.Now()
		var newPoll *store.NewPoll
		if in.Poll != nil {
			expires := now.Add(time.Duration(in.PollHours) * time.Hour)
			in.Poll.Expiry = expires.UTC().Format("2006-01-02T15:04:05.000000+00:00")
			p.Poll = in.Poll
			newPoll = &store.NewPoll{Answers: len(in.Poll.Answers), Multiselect: in.Poll.AllowMultiselect, ExpiresAt: expires.UnixMilli()}
		}
		v2 := in.Flags&store.MsgFlagComponentsV2 != 0
		s.finishPlain(p, v2)
		if !p.hasBody() {
			writeDiscordError(w, dErrEmpty)
			return
		}
		plain, err := json.Marshal(p)
		if err != nil {
			writeDiscordError(w, err)
			return
		}
		users, roles, everyone := s.webhookMentions(ctx, ch, p.texts(), in.Mentions)
		whID := wh.ID
		m := &store.Message{ChannelID: ch.ID, AuthorID: author.ID, Type: "default", WebhookID: &whID, Webhook: plain,
			Mentions: users, MentionRoles: roles, MentionEveryone: everyone, Flags: in.Flags, NewPoll: newPoll, Attachments: rows}
		if err := s.Store.InsertMessage(ctx, m); err != nil {
			writeDiscordError(w, err)
			return
		}
		keep = true
		full, err := s.Store.MessageByID(ctx, m.ID, 0)
		if err != nil {
			writeDiscordError(w, err)
			return
		}
		s.Bcast.EmitChannel(ch.ID, "message_create", full)
		// a message with embeds of its own gets no link previews on top, as on Discord
		if in.Flags&store.MsgFlagSuppressEmbeds == 0 && !v2 && len(p.Embeds) == 0 {
			s.unfurlWebhookMessage(m.ID, ch.ID, p.Content)
		}
		if !wait {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if source == "slack" {
			// what Slack's own webhooks answer, which Slack senders check for
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok")
			return
		}
		WriteJSON(w, http.StatusOK, s.webhookMessageObj(r.Context(), full, ch))
	}
}

// webhookMessage loads a message the webhook in the path sent.
func (s *Server) webhookMessage(w http.ResponseWriter, r *http.Request, wh *store.Webhook) *store.Message {
	if r.URL.Query().Get("thread_id") != "" {
		writeDiscordError(w, dErrUnknownChannel)
		return nil
	}
	mid, err := parseBotID(r.PathValue("mid"))
	if err != nil {
		writeDiscordError(w, dErrUnknownMessage)
		return nil
	}
	m, err := s.Store.MessageByID(r.Context(), mid, 0)
	if err != nil || m.WebhookID == nil || *m.WebhookID != wh.ID {
		writeDiscordError(w, dErrUnknownMessage)
		return nil
	}
	return m
}

func (s *Server) handleWebhookMessageGet(w http.ResponseWriter, r *http.Request) {
	wh := s.tokenWebhook(w, r)
	if wh == nil || !s.hookAllow(w, wh, false) {
		return
	}
	m := s.webhookMessage(w, r, wh)
	if m == nil {
		return
	}
	WriteJSON(w, http.StatusOK, s.webhookMessageObj(r.Context(), m, nil))
}

func (s *Server) handleWebhookMessageEdit(w http.ResponseWriter, r *http.Request) {
	wh := s.tokenWebhook(w, r)
	if wh == nil || !s.hookAllow(w, wh, false) {
		return
	}
	m := s.webhookMessage(w, r, wh)
	if m == nil {
		return
	}
	ctx := r.Context()
	author, err := s.webhookUser(ctx)
	if err != nil {
		writeDiscordError(w, err)
		return
	}
	raw, files, derr := s.readWebhookBody(w, r, author.ID)
	if derr != nil {
		writeDiscordError(w, derr)
		return
	}
	keep := false
	defer func() {
		if !keep {
			s.dropFiles(files)
		}
	}()
	var p wmPlain
	if err := json.Unmarshal(m.Webhook, &p); err != nil {
		writeDiscordError(w, err)
		return
	}
	wasV2 := m.Flags&store.MsgFlagComponentsV2 != 0
	in, errs := parseWebhookBody(raw, whParseOpts{edit: true, withComponents: boolQuery(r, "with_components", false) || wasV2})
	if !errs.empty() {
		writeDiscordError(w, errs.err())
		return
	}
	flags := m.Flags &^ store.MsgFlagPinned
	if in.Present["flags"] {
		// embeds can be suppressed and shown again; a layout by components stays
		flags = (flags &^ store.MsgFlagSuppressEmbeds) | (in.Flags & store.MsgFlagSuppressEmbeds) | (in.Flags & store.MsgFlagComponentsV2)
	}
	v2 := flags&store.MsgFlagComponentsV2 != 0
	if in.Present["content"] {
		p.Content = in.Content
	}
	if in.Present["embeds"] {
		p.Embeds = in.Embeds
	}
	if in.Present["components"] {
		p.Components = in.Components
	}
	if v2 && (strings.TrimSpace(p.Content) != "" || len(p.Embeds) > 0 || p.Poll != nil) {
		f := &formErrors{}
		f.add([]string{"flags"}, "MESSAGE_CANNOT_USE_LEGACY_FIELDS_WITH_COMPONENTS_V2", "The IS_COMPONENTS_V2 flag cannot be used with content, embeds, polls or stickers.")
		writeDiscordError(w, f.err())
		return
	}
	// attachments lists the files to keep, and new files are added to them
	var keepIDs []ids.ID
	if in.Present["attachments"] {
		keepIDs = []ids.ID{}
		metas := map[ids.ID]whAttachReq{}
		for _, a := range in.Attach {
			if id, err := ids.Parse(a.ID); err == nil {
				metas[id] = a
			}
		}
		kept := p.Attachments[:0:0]
		for _, a := range p.Attachments {
			meta, ok := metas[a.ID]
			if !ok {
				continue
			}
			if meta.HasDesc {
				a.Description = meta.Description
			}
			if meta.Spoiler != nil {
				a.Spoiler = *meta.Spoiler
			}
			kept = append(kept, a)
			keepIDs = append(keepIDs, a.ID)
		}
		p.Attachments = kept
	}
	added, rows := fileAttachments(files, in.Attach)
	if len(p.Attachments)+len(added) > whMaxFiles {
		f := &formErrors{}
		f.add([]string{"attachments"}, "BASE_TYPE_MAX_LENGTH", fmt.Sprintf("Must be %d or fewer in length.", whMaxFiles))
		writeDiscordError(w, f.err())
		return
	}
	p.Attachments = append(p.Attachments, added...)
	p.LinkEmbeds = nil
	s.finishPlain(&p, v2)
	if !p.hasBody() {
		writeDiscordError(w, dErrEmpty)
		return
	}
	plain, err := json.Marshal(&p)
	if err != nil {
		writeDiscordError(w, err)
		return
	}
	ch, err := s.Store.ChannelByID(ctx, m.ChannelID)
	if err != nil {
		writeDiscordError(w, dErrUnknownChannel)
		return
	}
	users, roles, everyone := s.webhookMentions(ctx, ch, p.texts(), in.Mentions)
	if err := s.Store.EditWebhookMessage(ctx, m.ID, store.WebhookMessageEdit{Plain: plain, Flags: flags, Mentions: users, MentionRoles: roles, MentionEveryone: everyone,
		Keep: keepIDs, Add: rows, EditedAt: time.Now().UnixMilli()}); err != nil {
		writeDiscordError(w, err)
		return
	}
	keep = true
	updated, err := s.Store.MessageByID(ctx, m.ID, 0)
	if err != nil {
		writeDiscordError(w, err)
		return
	}
	s.Bcast.EmitChannel(ch.ID, "message_update", messageUpdateEvent{updated, mentionsBefore{m.Mentions, m.MentionRoles, m.MentionEveryone}})
	if flags&store.MsgFlagSuppressEmbeds == 0 && !v2 && len(p.Embeds) == 0 {
		s.unfurlWebhookMessage(m.ID, ch.ID, p.Content)
	}
	WriteJSON(w, http.StatusOK, s.webhookMessageObj(ctx, updated, ch))
}

func (s *Server) handleWebhookMessageDelete(w http.ResponseWriter, r *http.Request) {
	wh := s.tokenWebhook(w, r)
	if wh == nil || !s.hookAllow(w, wh, false) {
		return
	}
	m := s.webhookMessage(w, r, wh)
	if m == nil {
		return
	}
	if err := s.Store.DeleteMessage(r.Context(), m.ID); err != nil {
		writeDiscordError(w, err)
		return
	}
	ev := map[string]any{"id": m.ID, "channel_id": m.ChannelID, "author_id": m.AuthorID,
		"mentions": m.Mentions, "mention_roles": m.MentionRoles, "mention_everyone": m.MentionEveryone}
	s.withLastMessage(r.Context(), ev, m.ChannelID)
	s.Bcast.EmitChannel(m.ChannelID, "message_delete", ev)
	w.WriteHeader(http.StatusNoContent)
}
