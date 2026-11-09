package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/botproto"
	"evilion.xyz/chat/internal/gateway"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/media"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// The developer portal: Settings, Developer Portal, where someone makes an
// application and its bot and looks after them. An application's bot is an
// account of its own (flag 64) whose id is the application's; its token is
// shown once, when it is made, and can be made again (which ends the old
// one). The bot's X25519 identity, which the sealed copies of messages
// addressed to it are opened with, and the key interactions sent to its
// endpoint are signed with are made here and kept sealed with the server's
// secret.

const (
	appMaxName        = 32
	appMinName        = 2
	appMaxDescription = 400
	appMaxTags        = 5
	appMaxTag         = 20
	appMaxRedirects   = 10
	appMaxURL         = 2048
)

func (s *Server) portalRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/applications", s.authed(s.handleAppsList))
	mux.Handle("POST /api/v1/applications", s.userLimited(s.limApps, s.handleAppCreate))
	mux.Handle("GET /api/v1/applications/{id}", s.authed(s.handleAppGet))
	mux.Handle("PATCH /api/v1/applications/{id}", s.userLimited(s.limApps, s.handleAppPatch))
	mux.Handle("POST /api/v1/applications/{id}/delete", s.userLimited(s.limApps, s.handleAppDelete))
	mux.Handle("PATCH /api/v1/applications/{id}/bot", s.userLimited(s.limApps, s.handleAppBotPatch))
	mux.Handle("POST /api/v1/applications/{id}/bot/reset", s.userLimited(s.limApps, s.handleAppTokenReset))
	mux.Handle("POST /api/v1/applications/{id}/secret/reset", s.userLimited(s.limApps, s.handleAppSecretReset))
	mux.Handle("GET /api/v1/applications/{id}/commands", s.authed(s.handleAppCommandsView))
	mux.Handle("DELETE /api/v1/applications/{id}/commands", s.userLimited(s.limApps, s.handleAppCommandsClear))
	mux.Handle("DELETE /api/v1/applications/{id}/commands/{cid}", s.userLimited(s.limApps, s.handleAppCommandRemove))
	mux.Handle("GET /api/v1/applications/{id}/public", s.authed(s.handleAppPublic))
}

// --- what the portal shows ---------------------------------------------------------------

// appView is an application as its owner sees it in the portal.
type appView struct {
	ID                  ids.ID                                 `json:"id"`
	Name                string                                 `json:"name"`
	Description         string                                 `json:"description"`
	IconID              *ids.ID                                `json:"icon_id"`
	CoverID             *ids.ID                                `json:"cover_id"`
	Tags                []string                               `json:"tags"`
	TermsURL            string                                 `json:"terms_url"`
	PrivacyURL          string                                 `json:"privacy_url"`
	InteractionsURL     string                                 `json:"interactions_url"`
	CustomInstallURL    string                                 `json:"custom_install_url"`
	InstallParams       *store.InstallParams                   `json:"install_params"`
	IntegrationTypes    map[string]store.IntegrationTypeConfig `json:"integration_types"`
	RedirectURIs        []string                               `json:"redirect_uris"`
	BotPublic           bool                                   `json:"bot_public"`
	Discoverable        bool                                   `json:"discoverable"`
	DiscoveryHidden     bool                                   `json:"discovery_hidden"`
	BotRequireCodeGrant bool                                   `json:"bot_require_code_grant"`
	VerifyKey           string                                 `json:"verify_key"`
	EventWebhooksURL    string                                 `json:"event_webhooks_url"`
	EventWebhooksOn     bool                                   `json:"event_webhooks_enabled"`
	EventWebhooksTypes  []string                               `json:"event_webhooks_types"`
	Intents             appIntents                             `json:"intents"`
	Flags               int64                                  `json:"flags"`
	HasSecret           bool                                   `json:"has_secret"`
	// the id bot libraries know it by, for the owner to copy
	ProtocolID string           `json:"protocol_id"`
	Bot        store.PublicUser `json:"bot"`
	Servers    int              `json:"server_count"`
	Users      int              `json:"user_install_count"`
	CreatedAt  int64            `json:"created_at"`
}

// appIntents are the privileged intents an application holds.
type appIntents struct {
	Presence       bool `json:"presence"`
	Members        bool `json:"members"`
	MessageContent bool `json:"message_content"`
}

func intentsOf(flags int64) appIntents {
	return appIntents{
		Presence:       flags&(botproto.AppFlagGatewayPresence|botproto.AppFlagGatewayPresenceLimited) != 0,
		Members:        flags&(botproto.AppFlagGatewayGuildMembers|botproto.AppFlagGatewayGuildMembersLimited) != 0,
		MessageContent: flags&(botproto.AppFlagGatewayMessageContent|botproto.AppFlagGatewayMessageContentLimited) != 0,
	}
}

func (s *Server) appView(ctx context.Context, app *store.Application) (*appView, error) {
	bot, err := s.Store.UserByID(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	v := &appView{ID: app.ID, Name: app.Name, Description: app.Description, IconID: app.IconID, CoverID: app.CoverID, Tags: app.Tags,
		TermsURL: app.TermsURL, PrivacyURL: app.PrivacyURL, InteractionsURL: app.InteractionsURL, CustomInstallURL: app.CustomInstallURL,
		InstallParams: app.InstallParams, IntegrationTypes: app.IntegrationTypes, RedirectURIs: app.RedirectURIs, BotPublic: app.BotPublic, Discoverable: app.Discoverable, DiscoveryHidden: app.DiscoveryHidden,
		BotRequireCodeGrant: app.BotRequireCodeGrant, VerifyKey: app.VerifyKey, EventWebhooksURL: app.EventWebhooksURL,
		EventWebhooksOn: app.EventWebhooksStatus == 2, EventWebhooksTypes: app.EventWebhooksTypes, Intents: intentsOf(app.Flags), Flags: app.Flags,
		HasSecret: app.SecretHash != "", ProtocolID: botproto.OutString(app.ID), Bot: bot.Public(), CreatedAt: app.CreatedAt}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if v.RedirectURIs == nil {
		v.RedirectURIs = []string{}
	}
	if v.EventWebhooksTypes == nil {
		v.EventWebhooksTypes = []string{}
	}
	v.Servers, _ = s.Store.ApplicationGuildCount(ctx, app.ID)
	v.Users, _ = s.Store.ApplicationUserCount(ctx, app.ID)
	return v, nil
}

// ownedApp loads the {id} of a portal route, which must be the caller's.
func (s *Server) ownedApp(w http.ResponseWriter, r *http.Request) (*store.Application, *store.User) {
	me := auth.UserFrom(r.Context())
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return nil, nil
	}
	app, err := s.Store.ApplicationByID(r.Context(), id)
	if err != nil || app.OwnerID != me.ID {
		WriteError(w, ErrNotFound)
		return nil, nil
	}
	return app, me
}

func (s *Server) handleAppsList(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	list, err := s.Store.ApplicationsByOwner(r.Context(), me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	out := make([]*appView, 0, len(list))
	for _, app := range list {
		if v, err := s.appView(r.Context(), app); err == nil {
			out = append(out, v)
		}
	}
	WriteJSON(w, http.StatusOK, out)
}

func (s *Server) handleAppGet(w http.ResponseWriter, r *http.Request) {
	app, _ := s.ownedApp(w, r)
	if app == nil {
		return
	}
	v, err := s.appView(r.Context(), app)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, v)
}

// handleAppPublic is what anyone is shown of an application: before adding
// it, on its bot's profile.
func (s *Server) handleAppPublic(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	ctx := r.Context()
	app, err := s.Store.ApplicationByID(ctx, id)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	bot, err := s.Store.UserByID(ctx, app.ID)
	if err != nil || bot.Flags&(store.FlagDisabled|store.FlagDeleted) != 0 {
		WriteError(w, ErrNotFound)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"id": app.ID, "name": app.Name, "description": app.Description, "icon_id": app.IconID, "cover_id": app.CoverID,
		"tags": app.Tags, "terms_url": app.TermsURL, "privacy_url": app.PrivacyURL, "bot_public": app.BotPublic, "bot": bot.Public(),
		"install_params": app.InstallParams, "integration_types": app.IntegrationTypes, "custom_install_url": app.CustomInstallURL,
		"intents": intentsOf(app.Flags), "owner_id": app.OwnerID, "created_at": app.CreatedAt})
}

// --- making one ----------------------------------------------------------------------------

// newBotToken is a bot's token, in the shape bot libraries expect: the bot's
// id in base64, a short stamp, and a long secret, joined by dots.
func newBotToken(botID ids.ID) string {
	secret := make([]byte, 30)
	stamp := make([]byte, 5)
	_, _ = rand.Read(secret)
	_, _ = rand.Read(stamp)
	return base64.RawStdEncoding.EncodeToString([]byte(botproto.OutString(botID))) + "." +
		base64.RawURLEncoding.EncodeToString(stamp)[:6] + "." + base64.RawURLEncoding.EncodeToString(secret)
}

// newClientSecret is an application's OAuth2 client secret.
func newClientSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// cleanAppName checks an application's name.
func cleanAppName(v string) (string, bool) {
	name, ok := CleanDisplayName(v)
	if !ok {
		return "", false
	}
	n := utf8.RuneCountInString(name)
	return name, n >= appMinName && n <= appMaxName
}

// botUsername makes a username for an application's bot from its name.
func botUsername(name string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
			b.WriteRune(c)
		case c == ' ' || c == '-' || c == '.':
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if len(out) < 2 {
		out = "bot"
	}
	if len(out) > 26 {
		out = out[:26]
	}
	return out
}

func (s *Server) handleAppCreate(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	if me.Flags&(store.FlagBot|store.FlagSystem|store.FlagWebhook) != 0 {
		WriteError(w, ErrForbidden)
		return
	}
	// every app brings a bot account with it
	if !s.requireCaptcha(w, r, "create_app") {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	name, ok := cleanAppName(req.Name)
	if !ok {
		WriteError(w, E(http.StatusBadRequest, "invalid_name", fmt.Sprintf("Names are %d to %d characters.", appMinName, appMaxName)))
		return
	}
	ctx := r.Context()
	pub, priv, err := newBotIdentity()
	if err != nil {
		WriteError(w, err)
		return
	}
	botKey, err := s.sealBotSecret(priv)
	if err != nil {
		WriteError(w, err)
		return
	}
	verify, sign, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		WriteError(w, err)
		return
	}
	signKey, err := s.sealBotSecret(sign.Seed())
	if err != nil {
		WriteError(w, err)
		return
	}
	bot := &store.User{ID: ids.New(), DisplayName: name, PublicKey: pub, Flags: store.FlagBot, AuthHash: "!", KDFSalt: []byte{}, EncPrivateKey: []byte{}}
	token := newBotToken(bot.ID)
	app := &store.Application{OwnerID: me.ID, Name: name, VerifyKey: hex.EncodeToString(verify), SignKeySealed: signKey, BotKeySealed: botKey,
		BotPublic: true, TokenHash: tokenHash(token), EventWebhooksStatus: 1,
		InstallParams: &store.InstallParams{Scopes: []string{"bot", "applications.commands"}, Permissions: "0"}}
	base := botUsername(name)
	for attempt := 0; ; attempt++ {
		bot.Username = base
		if attempt > 0 {
			bot.Username = fmt.Sprintf("%s_%04d", base, randomInt(10000))
		}
		err = s.Store.CreateApplication(ctx, app, bot)
		if !errors.Is(err, store.ErrUsernameTaken) || attempt >= 8 {
			break
		}
	}
	if errors.Is(err, store.ErrTooMany) {
		WriteError(w, E(http.StatusBadRequest, "too_many_apps", fmt.Sprintf("You can have at most %d apps.", store.MaxApplicationsPerOwner)))
		return
	}
	if err != nil {
		WriteError(w, err)
		return
	}
	v, err := s.appView(ctx, app)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"application": v, "token": token})
}

// randomInt is a random number below n.
func randomInt(n int) int {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return int(uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3])) % n
}

// --- changing one ----------------------------------------------------------------------

func (s *Server) handleAppPatch(w http.ResponseWriter, r *http.Request) {
	app, _ := s.ownedApp(w, r)
	if app == nil {
		return
	}
	var raw map[string]any
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, s.Cfg.MaxAvatarBytes*3+64<<10))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		WriteError(w, E(http.StatusBadRequest, "bad_json", "The request could not be read."))
		return
	}
	if on, _ := raw["discoverable"].(bool); on && !app.Discoverable {
		public := app.BotPublic
		if v, ok := raw["bot_public"].(bool); ok {
			public = v
		}
		if !public {
			WriteError(w, E(http.StatusBadRequest, "app_not_public", "Only a public app can be on Discovery."))
			return
		}
		// everyone on Nexa sees it, so as with a server, listing one is a captcha away
		if !s.requireCaptcha(w, r, "discovery_list") {
			return
		}
	}
	if err := s.patchApplication(r, app, raw, false); err != nil {
		writeAppError(w, err)
		return
	}
	fresh, err := s.Store.ApplicationByID(r.Context(), app.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	v, err := s.appView(r.Context(), fresh)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, v)
}

// writeAppError answers one of patchApplication's errors in the app's own
// shape: a protocol error's first problem, as a sentence.
func writeAppError(w http.ResponseWriter, err error) {
	var de *dError
	if errors.As(err, &de) {
		msg := de.Message
		if de.Form != nil {
			if first := de.Form.first(); first != "" {
				msg = first
			}
		}
		status := de.Status
		if status < 400 {
			status = http.StatusBadRequest
		}
		WriteError(w, E(status, "invalid_application", msg))
		return
	}
	WriteError(w, err)
}

// patchApplication applies changes to an application, from its owner in the
// portal or (fromBot) from its bot over the bot API, which may change less:
// not its name, nor who may add it, nor its privileged intents.
func (s *Server) patchApplication(r *http.Request, app *store.Application, raw map[string]any, fromBot bool) error {
	ctx := r.Context()
	errs := &formErrors{}
	rd := jsonReader{errs}
	var p store.ApplicationPatch
	urlField := func(key string, dst **string) {
		if _, present := raw[key]; !present {
			return
		}
		v, _ := rd.str(raw, key, nil)
		v = strings.TrimSpace(v)
		if v != "" {
			u, err := url.Parse(v)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || len(v) > appMaxURL {
				errs.add(fieldPath(key), "URL_TYPE_INVALID_URL", "Not a well formed URL.")
				return
			}
		}
		*dst = &v
	}
	if !fromBot {
		if _, present := raw["name"]; present {
			v, _ := rd.str(raw, "name", nil)
			if name, ok := cleanAppName(v); ok {
				p.Name = &name
			} else {
				errs.add(fieldPath("name"), "BASE_TYPE_BAD_LENGTH", fmt.Sprintf("Must be between %d and %d in length.", appMinName, appMaxName))
			}
		}
		for _, k := range []string{"terms_of_service_url", "terms_url"} {
			urlField(k, &p.TermsURL)
		}
		for _, k := range []string{"privacy_policy_url", "privacy_url"} {
			urlField(k, &p.PrivacyURL)
		}
		if v, ok := rd.boolean(raw, "bot_public", nil); ok {
			p.BotPublic = &v
		}
		if v, ok := rd.boolean(raw, "discoverable", nil); ok {
			p.Discoverable = &v
		}
		if v, ok := rd.boolean(raw, "bot_require_code_grant", nil); ok {
			p.BotRequireCodeGrant = &v
		}
		if list, ok := rd.list(raw, "redirect_uris", nil); ok {
			if len(list) > appMaxRedirects {
				errs.add(fieldPath("redirect_uris"), "BASE_TYPE_MAX_LENGTH", fmt.Sprintf("Must be %d or fewer in length.", appMaxRedirects))
			}
			uris := []string{}
			for i, v := range list {
				sv, _ := v.(string)
				u, err := url.Parse(strings.TrimSpace(sv))
				if err != nil || u.Scheme == "" || len(sv) > appMaxURL || u.Fragment != "" {
					errs.add(fieldPath("redirect_uris", i), "URL_TYPE_INVALID_URL", "Not a well formed URL.")
					continue
				}
				uris = append(uris, strings.TrimSpace(sv))
			}
			p.RedirectURIs = &uris
		}
	}
	if _, present := raw["description"]; present {
		v, _ := rd.str(raw, "description", nil)
		d, ok := CleanText(v, appMaxDescription)
		if !ok {
			errs.add(fieldPath("description"), "BASE_TYPE_MAX_LENGTH", fmt.Sprintf("Must be %d or fewer in length.", appMaxDescription))
		} else {
			p.Description = &d
		}
	}
	if list, ok := rd.list(raw, "tags", nil); ok {
		if len(list) > appMaxTags {
			errs.add(fieldPath("tags"), "BASE_TYPE_MAX_LENGTH", fmt.Sprintf("Must be %d or fewer in length.", appMaxTags))
		}
		tags := []string{}
		for i, v := range list {
			sv, _ := v.(string)
			t, ok := CleanText(sv, appMaxTag)
			if !ok || t == "" {
				errs.add(fieldPath("tags", i), "BASE_TYPE_BAD_LENGTH", fmt.Sprintf("Must be between 1 and %d in length.", appMaxTag))
				continue
			}
			tags = append(tags, t)
		}
		p.Tags = &tags
	}
	urlField("custom_install_url", &p.CustomInstallURL)
	urlField("role_connections_verification_url", &p.RoleConnectionsURL)
	var newEndpoint *string
	for _, k := range []string{"interactions_endpoint_url", "interactions_url"} {
		urlField(k, &newEndpoint)
	}
	urlField("event_webhooks_url", &p.EventWebhooksURL)
	if v, ok := rd.integer(raw, "event_webhooks_status", nil); ok && (v == 1 || v == 2) {
		n := int(v)
		p.EventWebhooksStatus = &n
	}
	if v, ok := rd.boolean(raw, "event_webhooks_enabled", nil); ok {
		n := 1
		if v {
			n = 2
		}
		p.EventWebhooksStatus = &n
	}
	if list, ok := rd.list(raw, "event_webhooks_types", nil); ok {
		types := []string{}
		for _, v := range list {
			if sv, ok := v.(string); ok && (sv == "APPLICATION_AUTHORIZED" || sv == "APPLICATION_DEAUTHORIZED") {
				types = append(types, sv)
			}
		}
		p.EventWebhooksTypes = &types
	}
	if v, present := raw["install_params"]; present {
		if v == nil {
			var none *store.InstallParams
			p.InstallParams = &none
		} else if obj, ok := v.(map[string]any); ok {
			ip := readInstallParams(rd, obj, fieldPath("install_params"))
			p.InstallParams = &ip
		}
	}
	for _, key := range []string{"integration_types_config", "integration_types"} {
		obj, ok := rd.object(raw, key, nil)
		if !ok {
			continue
		}
		types := map[string]store.IntegrationTypeConfig{}
		for k, v := range obj {
			if k != "0" && k != "1" {
				errs.add(fieldPath(key, k), "ENUM_TYPE_COERCE", "Value is not a valid enum value.")
				continue
			}
			cfg := store.IntegrationTypeConfig{}
			if vo, ok := v.(map[string]any); ok {
				if ip, ok := vo["oauth2_install_params"].(map[string]any); ok {
					params := readInstallParams(rd, ip, fieldPath(key, k, "oauth2_install_params"))
					cfg.OAuth2InstallParams = params
				}
			}
			types[k] = cfg
		}
		if len(types) == 0 {
			errs.add(fieldPath(key), "INTEGRATION_TYPES_EMPTY", "An app must be installable somewhere.")
		}
		p.IntegrationTypes = &types
	}
	var intents *appIntents
	if !fromBot {
		if obj, ok := rd.object(raw, "intents", nil); ok {
			cur := intentsOf(app.Flags)
			if v, ok := rd.boolean(obj, "presence", nil); ok {
				cur.Presence = v
			}
			if v, ok := rd.boolean(obj, "members", nil); ok {
				cur.Members = v
			}
			if v, ok := rd.boolean(obj, "message_content", nil); ok {
				cur.MessageContent = v
			}
			intents = &cur
		}
	}
	var icon, cover *rawImage
	if v, present := raw["icon"]; present {
		icon = s.readImageField(rd, v, "icon")
	}
	if v, present := raw["cover_image"]; present {
		cover = s.readImageField(rd, v, "cover_image")
	}
	if !errs.empty() {
		return errs.err()
	}
	// a new endpoint must answer a ping, as the protocol checks one
	if newEndpoint != nil && *newEndpoint != app.InteractionsURL {
		if *newEndpoint != "" {
			if err := s.pingEndpoint(ctx, app, *newEndpoint); err != nil {
				var ae *Error
				if errors.As(err, &ae) {
					return dFormError("APPLICATION_INTERACTIONS_ENDPOINT_URL_INVALID", ae.Message, "interactions_endpoint_url")
				}
				return err
			}
		}
		p.InteractionsURL = newEndpoint
	}
	if err := s.Store.UpdateApplication(ctx, app.ID, p); err != nil {
		return err
	}
	for _, img := range []struct {
		v   *rawImage
		col string
	}{{icon, "icon_id"}, {cover, "cover_id"}} {
		if img.v == nil {
			continue
		}
		var blobID *ids.ID
		if img.v.data != nil {
			kind := "icon"
			maxW, maxH := avatarSide, avatarSide
			if img.col == "cover_id" {
				kind, maxW, maxH = "banner", serverBannerW, serverBannerH
			}
			b, err := s.Media.PutPublic(ctx, kind, bytes.NewReader(img.v.data), media.Limits{MaxBytes: int64(len(img.v.data)) + 1, MaxW: maxW, MaxH: maxH}, app.OwnerID)
			if err != nil {
				return botError(err, nil)
			}
			blobID = &b.ID
		}
		if err := s.Store.SwapImage(ctx, "applications", img.col, app.ID, blobID); err != nil {
			if blobID != nil {
				s.releaseBlob(ctx, *blobID)
			}
			return err
		}
	}
	if p.Description != nil {
		about, _ := CleanText(*p.Description, maxAboutMe)
		_ = s.Store.UpdateProfile(ctx, app.ID, store.ProfilePatch{AboutMe: &about})
	}
	if p.Name != nil {
		// the bot is shown under the application's name
		_ = s.Store.UpdateProfile(ctx, app.ID, store.ProfilePatch{DisplayName: p.Name})
	}
	if intents != nil {
		if err := s.setAppIntents(r, app, *intents); err != nil {
			return err
		}
	}
	if p.Name != nil || p.Description != nil || p.InteractionsURL != nil {
		if bot, err := s.Store.UserByID(ctx, app.ID); err == nil {
			s.Bcast.UserUpdated(bot)
		}
	}
	return nil
}

// rawImage is a picture a request set, or cleared (data nil).
type rawImage struct {
	data []byte
}

// readImageField reads a picture given as a data URI, or null to clear it.
func (s *Server) readImageField(rd jsonReader, v any, key string) *rawImage {
	if v == nil {
		return &rawImage{}
	}
	sv, ok := v.(string)
	if !ok {
		rd.errs.add(fieldPath(key), "IMAGE_INVALID", "Invalid image data")
		return nil
	}
	img, err := imageData(sv)
	if err != nil {
		rd.errs.add(fieldPath(key), "IMAGE_INVALID", "Invalid image data")
		return nil
	}
	if int64(len(img.data)) > s.Cfg.MaxAvatarBytes {
		rd.errs.add(fieldPath(key), "BINARY_TYPE_MAX_SIZE", "File cannot be larger than the maximum size.")
		return nil
	}
	return &rawImage{data: img.data}
}

// readInstallParams reads {scopes, permissions}: what adding an application
// asks for by default.
func readInstallParams(rd jsonReader, obj map[string]any, path []string) *store.InstallParams {
	ip := &store.InstallParams{Scopes: []string{}, Permissions: "0"}
	if list, ok := rd.list(obj, "scopes", path); ok {
		for i, v := range list {
			sv, _ := v.(string)
			if !knownScopes[sv] {
				rd.errs.add(fieldPath(path, "scopes", i), "OAUTH2_SCOPE_INVALID", "Invalid OAuth2 scope")
				continue
			}
			ip.Scopes = append(ip.Scopes, sv)
		}
	}
	if v, ok := rd.str(obj, "permissions", path); ok {
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
		if err != nil {
			rd.errs.add(fieldPath(path, "permissions"), "NUMBER_TYPE_COERCE", "Value is not a number.")
		} else {
			ip.Permissions = strconv.FormatUint(n&uint64(perms.All), 10)
		}
	}
	return ip
}

// setAppIntents switches an application's privileged intents. Message
// Content decides whether its bot holds the keys of the servers it is in:
// switched on, members' apps hand it them; switched off, it gives them back
// and every conversation it could read starts a new key.
func (s *Server) setAppIntents(r *http.Request, app *store.Application, in appIntents) error {
	ctx := r.Context()
	flags := app.Flags &^ (botproto.AppFlagGatewayPresence | botproto.AppFlagGatewayPresenceLimited | botproto.AppFlagGatewayGuildMembers |
		botproto.AppFlagGatewayGuildMembersLimited | botproto.AppFlagGatewayMessageContent | botproto.AppFlagGatewayMessageContentLimited)
	if in.Presence {
		flags |= botproto.AppFlagGatewayPresenceLimited
	}
	if in.Members {
		flags |= botproto.AppFlagGatewayGuildMembersLimited
	}
	if in.MessageContent {
		flags |= botproto.AppFlagGatewayMessageContentLimited
	}
	if flags == app.Flags {
		return nil
	}
	had := intentsOf(app.Flags).MessageContent
	if err := s.Store.SetApplicationFlags(ctx, app.ID, flags, in.MessageContent); err != nil {
		return err
	}
	// a bot connected with intents it no longer holds starts again
	s.disconnectBot(app.ID)
	if had == in.MessageContent {
		return nil
	}
	// the people who see the bot learn what it reads now: their apps mark the
	// channels it can read, and hand it the keys it is now entitled to
	if bot, err := s.Store.UserByID(ctx, app.ID); err == nil {
		s.Bcast.UserUpdated(bot)
	}
	servers, err := s.Store.BotServers(ctx, app.ID)
	if err != nil {
		return err
	}
	if in.MessageContent {
		for _, g := range servers {
			s.Bcast.KeysMaybeNeeded(g)
		}
		return nil
	}
	s.forgetBot(app.ID)
	if err := s.Store.DeleteUserGrants(ctx, app.ID); err != nil {
		return err
	}
	for _, g := range servers {
		if srv, err := s.Store.ServerByID(ctx, g); err == nil {
			s.rotateAfter(r, srv, app.ID)
		}
	}
	return nil
}

// --- the bot account -----------------------------------------------------------------------

func (s *Server) handleAppBotPatch(w http.ResponseWriter, r *http.Request) {
	app, _ := s.ownedApp(w, r)
	if app == nil {
		return
	}
	var raw map[string]any
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, s.Cfg.MaxAvatarBytes*3+64<<10))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		WriteError(w, E(http.StatusBadRequest, "bad_json", "The request could not be read."))
		return
	}
	bot, err := s.patchBotUser(r, app.ID, raw)
	if err != nil {
		writeAppError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, bot.Public())
}

// patchBotUser changes a bot's account: its username, picture and banner,
// from its owner or (PATCH /users/@me) from the bot itself.
func (s *Server) patchBotUser(r *http.Request, botID ids.ID, raw map[string]any) (*store.User, error) {
	ctx := r.Context()
	errs := &formErrors{}
	rd := jsonReader{errs}
	var p store.ProfilePatch
	if _, present := raw["username"]; present {
		v, _ := rd.str(raw, "username", nil)
		v = strings.ToLower(strings.TrimSpace(v))
		if !ValidUsername(v) {
			errs.add(fieldPath("username"), "USERNAME_INVALID", "Usernames are 2 to 32 characters of a-z, 0-9, _ and .")
		} else {
			p.Username = &v
		}
	}
	var avatar, banner *rawImage
	if v, present := raw["avatar"]; present {
		avatar = s.readImageField(rd, v, "avatar")
	}
	if v, present := raw["banner"]; present {
		banner = s.readImageField(rd, v, "banner")
	}
	if !errs.empty() {
		return nil, errs.err()
	}
	if p.Username != nil {
		if err := s.Store.UpdateProfile(ctx, botID, p); err != nil {
			if errors.Is(err, store.ErrUsernameTaken) {
				return nil, dFormError("USERNAME_ALREADY_TAKEN", "Username is unavailable. Try another username.", "username")
			}
			return nil, err
		}
	}
	for _, img := range []struct {
		v    *rawImage
		kind string
	}{{avatar, "avatar"}, {banner, "banner"}} {
		if img.v == nil {
			continue
		}
		var blobID *ids.ID
		if img.v.data != nil {
			maxW, maxH := avatarSide, avatarSide
			if img.kind == "banner" {
				maxW, maxH = userBannerW, userBannerH
			}
			lim := s.limitsFor(img.kind)
			width, height, err := media.ImageDims(bytes.NewReader(img.v.data))
			if err != nil {
				return nil, botError(media.ErrUnsupported, nil)
			} else if err := lim.Check(width, height); err != nil {
				return nil, botError(err, nil)
			}
			out := img.v.data
			if width > maxW || height > maxH {
				if out, _, err = media.Crop(img.v.data, media.CropSpec{MaxW: maxW, MaxH: maxH}); err != nil {
					return nil, botError(cropError(err), nil)
				}
			}
			b, err := s.Media.PutPublic(ctx, img.kind, bytes.NewReader(out), media.Limits{MaxBytes: int64(len(out)) + 1, MaxW: maxW, MaxH: maxH}, botID)
			if err != nil {
				return nil, botError(err, nil)
			}
			blobID = &b.ID
		}
		if err := s.Store.SwapImage(ctx, "users", img.kind+"_id", botID, blobID); err != nil {
			if blobID != nil {
				s.releaseBlob(ctx, *blobID)
			}
			return nil, err
		}
	}
	bot, err := s.Store.UserByID(ctx, botID)
	if err != nil {
		return nil, err
	}
	s.Bcast.UserUpdated(bot)
	return bot, nil
}

// handleAppTokenReset makes the bot a new token, ending the old one and
// every session it had. It takes the owner's password, as a token is the
// whole of a bot.
func (s *Server) handleAppTokenReset(w http.ResponseWriter, r *http.Request) {
	app, me := s.ownedApp(w, r)
	if app == nil {
		return
	}
	var req struct {
		AuthKey []byte `json:"auth_key"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if err := s.requirePassword(w, me, req.AuthKey, "The password is wrong."); err != nil {
		WriteError(w, err)
		return
	}
	token := newBotToken(app.ID)
	if err := s.Store.SetApplicationToken(r.Context(), app.ID, tokenHash(token)); err != nil {
		WriteError(w, err)
		return
	}
	s.disconnectBot(app.ID)
	WriteJSON(w, http.StatusOK, map[string]any{"token": token})
}

// handleAppSecretReset makes the application a new OAuth2 client secret.
func (s *Server) handleAppSecretReset(w http.ResponseWriter, r *http.Request) {
	app, me := s.ownedApp(w, r)
	if app == nil {
		return
	}
	var req struct {
		AuthKey []byte `json:"auth_key"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if err := s.requirePassword(w, me, req.AuthKey, "The password is wrong."); err != nil {
		WriteError(w, err)
		return
	}
	secret := newClientSecret()
	if err := s.Store.SetApplicationSecret(r.Context(), app.ID, tokenHash(secret)); err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"secret": secret})
}

// handleAppCommandsView lists an application's commands, for its owner.
func (s *Server) handleAppCommandsView(w http.ResponseWriter, r *http.Request) {
	app, _ := s.ownedApp(w, r)
	if app == nil {
		return
	}
	ctx := r.Context()
	global, err := s.Store.Commands(ctx, app.ID, nil)
	if err != nil {
		WriteError(w, err)
		return
	}
	type ownedCommand struct {
		commandView
		// GuildName names the server a command was made for alone
		GuildName string `json:"guild_name,omitempty"`
	}
	out := []ownedCommand{}
	for _, c := range global {
		out = append(out, ownedCommand{commandView: commandView{ID: c.ID, ApplicationID: c.ApplicationID, Version: c.Version, commandDef: *readCommandDef(c)}})
	}
	// and what it made for single servers, for the servers it is in (the first fifty)
	if servers, err := s.Store.ApplicationGuilds(ctx, app.ID); err == nil {
		if len(servers) > 50 {
			servers = servers[:50]
		}
		for _, g := range servers {
			gid := g
			list, err := s.Store.Commands(ctx, app.ID, &gid)
			if err != nil || len(list) == 0 {
				continue
			}
			name := ""
			if srv, err := s.Store.ServerByID(ctx, g); err == nil {
				name = srv.Name
			}
			for _, c := range list {
				out = append(out, ownedCommand{commandView: commandView{ID: c.ID, ApplicationID: c.ApplicationID, GuildID: &gid, Version: c.Version, commandDef: *readCommandDef(c)}, GuildName: name})
			}
		}
	}
	WriteJSON(w, http.StatusOK, out)
}

// handleAppCommandRemove takes one of an application's commands away, for its owner.
func (s *Server) handleAppCommandRemove(w http.ResponseWriter, r *http.Request) {
	app, _ := s.ownedApp(w, r)
	if app == nil {
		return
	}
	cid, err := ids.Parse(r.PathValue("cid"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	c, err := s.Store.CommandByID(r.Context(), cid)
	if err != nil || c.ApplicationID != app.ID {
		WriteError(w, ErrNotFound)
		return
	}
	if err := s.Store.DeleteCommand(r.Context(), c); err != nil {
		WriteError(w, err)
		return
	}
	s.commandsChanged(r.Context(), app.ID, c.GuildID)
	w.WriteHeader(http.StatusNoContent)
}

// handleAppCommandsClear takes away every command an application made, everywhere
// and for single servers, so its bot can register them again from nothing.
func (s *Server) handleAppCommandsClear(w http.ResponseWriter, r *http.Request) {
	app, _ := s.ownedApp(w, r)
	if app == nil {
		return
	}
	ctx := r.Context()
	if err := s.Store.ReplaceCommands(ctx, app.ID, nil, nil); err != nil {
		WriteError(w, err)
		return
	}
	s.commandsChanged(ctx, app.ID, nil)
	if servers, err := s.Store.ApplicationGuilds(ctx, app.ID); err == nil {
		for _, g := range servers {
			gid := g
			if list, err := s.Store.Commands(ctx, app.ID, &gid); err != nil || len(list) == 0 {
				continue
			}
			if err := s.Store.ReplaceCommands(ctx, app.ID, &gid, nil); err != nil {
				WriteError(w, err)
				return
			}
			s.commandsChanged(ctx, app.ID, &gid)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- deleting one ---------------------------------------------------------------------------

func (s *Server) handleAppDelete(w http.ResponseWriter, r *http.Request) {
	app, me := s.ownedApp(w, r)
	if app == nil {
		return
	}
	var req struct {
		AuthKey []byte `json:"auth_key"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if err := s.requirePassword(w, me, req.AuthKey, "The password is wrong."); err != nil {
		WriteError(w, err)
		return
	}
	if err := s.deleteApplication(r, app); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteApplication takes an application down: its bot leaves every server,
// its gateway sessions end, and its bot's account stays only as a deleted
// one, for the messages it wrote.
func (s *Server) deleteApplication(r *http.Request, app *store.Application) error {
	ctx := r.Context()
	s.disconnectBot(app.ID)
	servers, err := s.Store.BotServers(ctx, app.ID)
	if err != nil {
		return err
	}
	for _, g := range servers {
		srv, err := s.Store.ServerByID(ctx, g)
		if err != nil {
			continue
		}
		if err := s.removeMember(r, srv, app.ID, "member_remove", map[string]any{"reason": "app_deleted"}); err != nil {
			return err
		}
	}
	installs, _ := s.Store.ApplicationGuilds(ctx, app.ID)
	if err := s.Store.DeleteApplicationRow(ctx, app.ID); err != nil {
		return err
	}
	for _, g := range installs {
		s.emitServer(ctx, g, gateway.EvIntegrationsUpdate, map[string]any{"server_id": g})
	}
	s.forgetBot(app.ID)
	if err := s.Store.DeleteAccount(ctx, app.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if bot, err := s.Store.UserByID(ctx, app.ID); err == nil {
		s.Bcast.UserUpdated(bot)
	}
	return nil
}

// deleteOwnedApplications takes down every application an account owns,
// when the account goes.
func (s *Server) deleteOwnedApplications(r *http.Request, owner ids.ID) error {
	list, err := s.Store.ApplicationsByOwner(r.Context(), owner)
	if err != nil {
		return err
	}
	for _, app := range list {
		if err := s.deleteApplication(r, app); err != nil {
			return err
		}
	}
	return nil
}

// botLeftServer tidies what a bot leaves behind in a server it is no longer
// in: the role it brought, its application's install, and the server's
// list of applications.
func (s *Server) botLeftServer(r *http.Request, srv *store.Server, botID ids.ID) {
	ctx := r.Context()
	roles, err := s.Store.Roles(ctx, srv.ID)
	if err == nil {
		for _, role := range roles {
			if role.ManagedBy == nil || *role.ManagedBy != botID {
				continue
			}
			if err := s.Store.DeleteRole(ctx, role.ID); err == nil {
				s.emitServer(ctx, srv.ID, "role_delete", map[string]any{"id": role.ID, "server_id": srv.ID})
			}
		}
	}
	if removed, err := s.Store.RemoveGuildInstall(ctx, botID, srv.ID); err == nil && removed {
		s.emitServer(ctx, srv.ID, gateway.EvIntegrationsUpdate, map[string]any{"server_id": srv.ID})
		g := srv.ID
		s.tellBot(botID, &g, "GUILD_INTEGRATIONS_UPDATE", map[string]any{"guild_id": snow(srv.ID)})
		s.sendAppEvent(ctx, botID, "APPLICATION_DEAUTHORIZED", map[string]any{"user": nil, "guild": map[string]any{"id": snow(srv.ID)}})
	}
}
