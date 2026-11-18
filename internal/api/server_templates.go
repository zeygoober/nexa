package api

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/perms"
	"evilion.xyz/chat/internal/store"
)

// Nexa's own server templates: a server's managers make one link
// (/template/<code>) holding a snapshot of its channels, roles and
// permissions, and anyone signed in can make a new server from it, as from a
// Discord template (discord_templates.go). A server has one template; Sync
// takes the snapshot again under the same code. Messages, members, emoji,
// stickers and pictures are never part of it (store.SnapshotServer).
func (s *Server) serverTemplateRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/servers/{id}/template", s.authed(s.handleServerTemplateGet))
	mux.Handle("PUT /api/v1/servers/{id}/template", s.stepUpUser(s.stepBuild, s.handleServerTemplatePut))
	mux.Handle("POST /api/v1/servers/{id}/template/sync", s.userLimited(s.limManage, s.handleServerTemplateSync))
	mux.Handle("DELETE /api/v1/servers/{id}/template", s.userLimited(s.limManage, s.handleServerTemplateDelete))
	mux.Handle("GET /api/v1/servers/templates/nexa/{code}", s.userLimited(s.limDiscord, s.handleNexaTemplatePreview))
	mux.Handle("POST /api/v1/servers/import/nexa", s.userLimited(s.limImport, s.handleNexaTemplateImport))
}

// maxTemplateDesc is how long a template's description may be.
const maxTemplateDesc = 120

var reNexaTemplateCode = regexp.MustCompile(`^[A-Za-z0-9]{8,32}$`)

func (s *Server) errInvalidNexaTemplate() *Error {
	return E(http.StatusBadRequest, "invalid_template_link", "Paste a Nexa template link, like "+strings.TrimRight(s.Cfg.PublicURL, "/")+"/template/aB3dE5fG7hJ9.")
}

var errNexaTemplateNotFound = E(http.StatusNotFound, "template_not_found", "No template has that link. It may have been deleted.")

// parseNexaTemplateCode takes a template link on one of this Nexa's own
// addresses, or a bare code.
func (s *Server) parseNexaTemplateCode(input string) (string, *Error) {
	raw := strings.TrimSpace(input)
	if reNexaTemplateCode.MatchString(raw) {
		return raw, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", s.errInvalidNexaTemplate()
	}
	host := strings.ToLower(u.Hostname())
	own := false
	for _, a := range append([]string{s.Cfg.PublicURL}, append(s.Cfg.AltURLs, s.Cfg.MovedFrom...)...) {
		if pu, err := url.Parse(a); err == nil && strings.EqualFold(pu.Hostname(), host) {
			own = true
		}
	}
	parts := strings.FieldsFunc(u.EscapedPath(), func(r rune) bool { return r == '/' })
	if !own || len(parts) != 2 || parts[0] != "template" || !reNexaTemplateCode.MatchString(parts[1]) {
		return "", s.errInvalidNexaTemplate()
	}
	return parts[1], nil
}

func (s *Server) serverTemplateView(t *store.ServerTemplate) map[string]any {
	return map[string]any{
		"code":        t.Code,
		"url":         strings.TrimRight(s.Cfg.PublicURL, "/") + "/template/" + t.Code,
		"name":        t.Name,
		"description": t.Description,
		"usage_count": t.UsageCount,
		"created_at":  t.CreatedAt,
		"synced_at":   t.SyncedAt,
	}
}

func (s *Server) handleServerTemplateGet(w http.ResponseWriter, r *http.Request) {
	srv, _, _ := s.serverCtx(w, r, perms.ManageServer)
	if srv == nil {
		return
	}
	t, err := s.Store.ServerTemplateOf(r.Context(), srv.ID)
	if errors.Is(err, store.ErrNotFound) {
		WriteJSON(w, http.StatusOK, map[string]any{"template": nil})
		return
	}
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"template": s.serverTemplateView(t)})
}

// handleServerTemplatePut makes the server's template, or renames and
// describes the one it has (which leaves its snapshot as it was).
func (s *Server) handleServerTemplatePut(w http.ResponseWriter, r *http.Request) {
	srv, me, _ := s.serverCtx(w, r, perms.ManageServer)
	if srv == nil {
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := ReadJSON(w, r, &req, 2<<10); err != nil {
		WriteError(w, err)
		return
	}
	var name, desc *string
	if req.Name != nil {
		clean, ok := cleanServerName(*req.Name)
		if !ok {
			WriteError(w, E(http.StatusBadRequest, "invalid_name", "Template names are 1 to 100 characters."))
			return
		}
		name = &clean
	}
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		if utf8.RuneCountInString(d) > maxTemplateDesc {
			WriteError(w, E(http.StatusBadRequest, "invalid_description", "Descriptions are at most 120 characters."))
			return
		}
		clean := ""
		if d != "" {
			c, ok := CleanText(d, maxTemplateDesc)
			if !ok {
				WriteError(w, E(http.StatusBadRequest, "invalid_description", "Descriptions are at most 120 characters."))
				return
			}
			clean = c
		}
		desc = &clean
	}
	old, err := s.Store.ServerTemplateOf(r.Context(), srv.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		WriteError(w, err)
		return
	}
	if old == nil {
		if name == nil {
			WriteError(w, E(http.StatusBadRequest, "invalid_name", "Give the template a name."))
			return
		}
		d := ""
		if desc != nil {
			d = *desc
		}
		t, err := s.Store.CreateServerTemplate(r.Context(), srv.ID, *name, d)
		if errors.Is(err, store.ErrConflict) {
			WriteError(w, E(http.StatusConflict, "template_exists", "This server already has a template."))
			return
		}
		if err != nil {
			WriteError(w, err)
			return
		}
		s.audit(r, srv.ID, me.ID, "template_create", "server", &srv.ID, map[string]any{"name": t.Name}, "")
		WriteJSON(w, http.StatusCreated, map[string]any{"template": s.serverTemplateView(t)})
		return
	}
	t, err := s.Store.UpdateServerTemplate(r.Context(), srv.ID, name, desc, false)
	if err != nil {
		WriteError(w, err)
		return
	}
	changes := map[string]any{}
	if t.Name != old.Name {
		changes["name"] = map[string]any{"from": old.Name, "to": t.Name}
	}
	if t.Description != old.Description {
		changes["description"] = map[string]any{"from": old.Description, "to": t.Description}
	}
	if len(changes) > 0 {
		s.audit(r, srv.ID, me.ID, "template_update", "server", &srv.ID, changes, "")
	}
	WriteJSON(w, http.StatusOK, map[string]any{"template": s.serverTemplateView(t)})
}

func (s *Server) handleServerTemplateSync(w http.ResponseWriter, r *http.Request) {
	srv, me, _ := s.serverCtx(w, r, perms.ManageServer)
	if srv == nil {
		return
	}
	t, err := s.Store.UpdateServerTemplate(r.Context(), srv.ID, nil, nil, true)
	if errors.Is(err, store.ErrNotFound) {
		WriteError(w, E(http.StatusNotFound, "no_template", "This server has no template."))
		return
	}
	if err != nil {
		WriteError(w, err)
		return
	}
	s.audit(r, srv.ID, me.ID, "template_sync", "server", &srv.ID, nil, "")
	WriteJSON(w, http.StatusOK, map[string]any{"template": s.serverTemplateView(t)})
}

func (s *Server) handleServerTemplateDelete(w http.ResponseWriter, r *http.Request) {
	srv, me, _ := s.serverCtx(w, r, perms.ManageServer)
	if srv == nil {
		return
	}
	if err := s.Store.DeleteServerTemplate(r.Context(), srv.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		WriteError(w, err)
		return
	} else if err == nil {
		s.audit(r, srv.ID, me.ID, "template_delete", "server", &srv.ID, nil, "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// nexaTemplatePreview is a template as the import dialog shows it, the same
// shape a Discord template's preview has.
func nexaTemplatePreview(t *store.ServerTemplate, customName string) (store.TemplateSpec, discordTemplatePreview, *Error) {
	spec := t.Spec
	if strings.TrimSpace(customName) != "" {
		clean, ok := cleanServerName(customName)
		if !ok {
			return store.TemplateSpec{}, discordTemplatePreview{}, E(http.StatusBadRequest, "invalid_name", "Server names are 1–100 characters.")
		}
		spec.Name = clean
	}
	if len(spec.Channels) == 0 {
		return store.TemplateSpec{}, discordTemplatePreview{}, errEmptyTemplate
	}
	roles := make([]discordTemplatePreviewRole, len(spec.Roles))
	for i, r := range spec.Roles {
		roles[i] = discordTemplatePreviewRole{Name: r.Name, Color: r.Color}
	}
	return spec, discordTemplatePreview{
		Code:              t.Code,
		Name:              t.Name,
		Description:       t.Description,
		UsageCount:        t.UsageCount,
		ServerName:        spec.Name,
		ServerDescription: spec.Description,
		Channels:          templatePreviewChannels(spec.EveryonePerms, spec.Roles, spec.Channels),
		Roles:             roles,
	}, nil
}

func (s *Server) nexaTemplate(r *http.Request, input string) (*store.ServerTemplate, *Error) {
	code, apiErr := s.parseNexaTemplateCode(input)
	if apiErr != nil {
		return nil, apiErr
	}
	t, err := s.Store.ServerTemplateByCode(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNexaTemplateNotFound
	}
	if err != nil {
		return nil, E(http.StatusInternalServerError, "internal", "Something went wrong.")
	}
	return t, nil
}

func (s *Server) handleNexaTemplatePreview(w http.ResponseWriter, r *http.Request) {
	if mutedFor(w, r) {
		return
	}
	t, apiErr := s.nexaTemplate(r, r.PathValue("code"))
	if apiErr != nil {
		WriteError(w, apiErr)
		return
	}
	_, preview, apiErr := nexaTemplatePreview(t, "")
	if apiErr != nil {
		WriteError(w, apiErr)
		return
	}
	WriteJSON(w, http.StatusOK, preview)
}

func (s *Server) handleNexaTemplateImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireCaptcha(w, r, "create_server") {
		return
	}
	me := auth.UserFrom(r.Context())
	var req struct {
		URL  string `json:"url"`
		Name string `json:"name"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	t, apiErr := s.nexaTemplate(r, req.URL)
	if apiErr != nil {
		WriteError(w, apiErr)
		return
	}
	spec, _, apiErr := nexaTemplatePreview(t, req.Name)
	if apiErr != nil {
		WriteError(w, apiErr)
		return
	}
	srv, err := s.Store.CreateServerFromTemplate(r.Context(), me.ID, spec)
	if err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.CountServerTemplateUse(r.Context(), t.Code); err != nil {
		s.Log.Warn("template use not counted", "err", err)
	}
	view, users, err := s.serverView(r.Context(), srv, me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	s.Bcast.EmitUsers([]ids.ID{me.ID}, "server_create", serverCreateEvent{view, users})
	WriteJSON(w, http.StatusCreated, view)
}
