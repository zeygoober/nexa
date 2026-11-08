package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/media"
	"evilion.xyz/chat/internal/store"
	"evilion.xyz/chat/internal/textutil"
)

// Profile badges (internal/store/badges.go): a person's own list with the
// order and hidden set they chose, and what staff do: make event badges and
// give them, and Contributor, by hand. Everyone else sees badges in a
// profile's answer (handleUserProfile, handleSharedProfile).
func (s *Server) badgeRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/users/@me/badges", s.authed(s.handleMyBadges))
	mux.Handle("PUT /api/v1/users/@me/badges", s.authed(s.handleMyBadgesPut))
	mux.Handle("GET /api/v1/moderation/badges", s.staff(rankAdmin, s.handleModBadges))
	mux.Handle("POST /api/v1/moderation/badges", s.staff(rankAdmin, s.handleModBadgeCreate))
	mux.Handle("POST /api/v1/moderation/badges/image", s.staff(rankAdmin, s.handleModBadgeImage))
	mux.Handle("GET /api/v1/moderation/badges/{badge}/holders", s.staff(rankAdmin, s.handleModBadgeHolders))
	mux.Handle("PUT /api/v1/moderation/badges/{badge}/holders/{uid}", s.staff(rankAdmin, s.handleModBadgeHolderPut))
	mux.Handle("PATCH /api/v1/moderation/badges/{id}", s.staff(rankAdmin, s.handleModBadgeEdit))
	mux.Handle("DELETE /api/v1/moderation/badges/{id}", s.staff(rankAdmin, s.handleModBadgeDelete))
	mux.Handle("GET /api/v1/moderation/users/{id}/badges", s.staff(rankAdmin, s.handleModUserBadges))
	mux.Handle("PUT /api/v1/moderation/users/{id}/badges", s.staff(rankAdmin, s.handleModUserBadgesPut))
}

// profileBadges is what a profile's answer carries; a failure shows none
// rather than failing the profile.
func (s *Server) profileBadges(r *http.Request, u *store.User) []store.Badge {
	list, err := s.Store.ProfileBadges(r.Context(), u)
	if err != nil {
		s.Log.Warn("badges: profile", "user", u.ID.String(), "err", err)
		return []store.Badge{}
	}
	return list
}

func (s *Server) handleMyBadges(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.MyBadges(r.Context(), auth.UserFrom(r.Context()))
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"badges": list})
}

func (s *Server) handleMyBadgesPut(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if ok, retry := s.limProfile.Allow("badges:" + u.ID.String()); !ok {
		w.Header().Set("Retry-After", retryAfter(retry))
		WriteError(w, ErrRateLimited)
		return
	}
	var p store.BadgePrefs
	if err := ReadJSON(w, r, &p, 16<<10); err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.SetBadgePrefs(r.Context(), u.ID, p); err != nil {
		WriteError(w, err)
		return
	}
	s.handleMyBadges(w, r)
}

// --- staff ------------------------------------------------------------------

func (s *Server) handleModBadges(w http.ResponseWriter, r *http.Request) {
	defs, err := s.Store.BadgeDefs(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	contributors, err := s.Store.ContributorCount(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"badges": defs, "contributors": contributors})
}

type badgeDefBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       int    `json:"color"`
	// Color2 is a gradient's second colour; missing or -1 is one colour.
	Color2 *int   `json:"color2"`
	Glyph  string `json:"glyph"`
	Shape  string `json:"shape"`
	// ImageID is a picture put up with POST /moderation/badges/image, drawn
	// instead of the icon; missing or "0" is none.
	ImageID ids.ID `json:"image_id"`
}

// badgeSide is the largest an event badge's picture is kept.
const badgeSide = 128

// readBadgeDef reads and checks an event badge's name, description and look.
func (s *Server) readBadgeDef(w http.ResponseWriter, r *http.Request) (*store.BadgeDef, bool) {
	var b badgeDefBody
	if err := ReadJSON(w, r, &b, 4<<10); err != nil {
		WriteError(w, err)
		return nil, false
	}
	bad := func(code, msg string) (*store.BadgeDef, bool) {
		WriteError(w, E(http.StatusBadRequest, code, msg))
		return nil, false
	}
	name, ok := textutil.CleanName(b.Name, 4*store.MaxBadgeName)
	if !ok || name == "" || utf8.RuneCountInString(name) > store.MaxBadgeName {
		return bad("bad_name", "A badge needs a name of 1 to 32 characters.")
	}
	desc, ok := textutil.CleanName(b.Description, 4*store.MaxBadgeDesc)
	if !ok || utf8.RuneCountInString(desc) > store.MaxBadgeDesc {
		return bad("bad_description", "A badge's description can be at most 120 characters.")
	}
	if !store.ValidBadgeIcon(b.Icon) {
		return bad("bad_icon", "Pick an icon for the badge.")
	}
	color2 := -1
	if b.Color2 != nil {
		color2 = *b.Color2
	}
	if b.Color < 0 || b.Color > 0xFFFFFF || color2 < -1 || color2 > 0xFFFFFF {
		return bad("bad_color", "That colour is not valid.")
	}
	glyph := b.Glyph
	if glyph == "" {
		glyph = "light"
	}
	if glyph != "light" && glyph != "dark" {
		return bad("bad_glyph", "The icon is either light or dark.")
	}
	shape := b.Shape
	if shape == "" {
		shape = "circle"
	}
	if !store.BadgeShapes[shape] {
		return bad("bad_shape", "That shape is not one on offer.")
	}
	if b.ImageID != 0 {
		blob, err := s.Store.BlobByID(r.Context(), b.ImageID)
		if err != nil || blob.Kind != "badge" || blob.Encrypted {
			return bad("bad_image", "That picture has gone. Put it up again.")
		}
	}
	return &store.BadgeDef{Name: name, Description: desc, Icon: b.Icon, Color: b.Color, Color2: color2, Glyph: glyph, Shape: shape, ImageID: b.ImageID}, true
}

// handleModBadgeImage takes a picture for an event badge, cuts it square and
// keeps it small. Nothing names it until a badge is saved with it, so one
// never saved is collected.
func (s *Server) handleModBadgeImage(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	lim := s.limitsFor("badge")
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, lim.MaxBytes+1))
	if err != nil || int64(len(data)) > lim.MaxBytes {
		WriteError(w, media.ErrTooLarge)
		return
	}
	if width, height, err := media.ImageDims(bytes.NewReader(data)); err != nil {
		WriteError(w, media.ErrUnsupported)
		return
	} else if err := lim.Check(width, height); err != nil {
		WriteError(w, err)
		return
	}
	out, _, err := media.Crop(data, media.CropSpec{Rect: cropFromQuery(r), MaxW: badgeSide, MaxH: badgeSide})
	if err != nil {
		WriteError(w, cropError(err))
		return
	}
	b, err := s.Media.PutPublic(r.Context(), "badge", bytes.NewReader(out), media.Limits{MaxBytes: int64(len(out)) + 1, MaxW: badgeSide, MaxH: badgeSide}, me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	s.releaseBlob(r.Context(), b.ID)
	WriteJSON(w, http.StatusOK, map[string]any{"id": b.ID, "image": "/media/badge/" + b.ID.String(), "animated": b.Animated})
}

// keepBadgeImage moves a badge's hold from the picture it had to the one it
// has now.
func (s *Server) keepBadgeImage(r *http.Request, was, now ids.ID) error {
	if was == now {
		return nil
	}
	if now != 0 {
		if err := s.Store.AddBlobRef(r.Context(), now, 1); err != nil {
			return err
		}
	}
	if was != 0 {
		s.releaseBlob(r.Context(), was)
	}
	return nil
}

func (s *Server) handleModBadgeCreate(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	d, ok := s.readBadgeDef(w, r)
	if !ok {
		return
	}
	d.CreatedBy = me.ID
	if err := s.keepBadgeImage(r, 0, d.ImageID); err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.CreateBadgeDef(r.Context(), d); err != nil {
		s.keepBadgeImage(r, d.ImageID, 0)
		WriteError(w, err)
		return
	}
	s.logBadge(r, store.LogBadgeCreate, nil, map[string]any{"name": d.Name})
	WriteJSON(w, http.StatusOK, d)
}

func (s *Server) handleModBadgeEdit(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	was, err := s.Store.BadgeDef(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	d, ok := s.readBadgeDef(w, r)
	if !ok {
		return
	}
	d.ID, d.CreatedBy, d.CreatedAt = id, was.CreatedBy, was.CreatedAt
	if d.ImageID != was.ImageID && d.ImageID != 0 {
		if err := s.Store.AddBlobRef(r.Context(), d.ImageID, 1); err != nil {
			WriteError(w, err)
			return
		}
	}
	if err := s.Store.UpdateBadgeDef(r.Context(), d); err != nil {
		if d.ImageID != was.ImageID && d.ImageID != 0 {
			s.releaseBlob(r.Context(), d.ImageID)
		}
		WriteError(w, err)
		return
	}
	if d.ImageID != was.ImageID && was.ImageID != 0 {
		s.releaseBlob(r.Context(), was.ImageID)
	}
	s.logBadge(r, store.LogBadgeEdit, nil, map[string]any{"name": d.Name, "from": was.Name})
	WriteJSON(w, http.StatusOK, d)
}

func (s *Server) handleModBadgeDelete(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	was, err := s.Store.BadgeDef(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.DeleteBadgeDef(r.Context(), id); err != nil {
		WriteError(w, err)
		return
	}
	if was.ImageID != 0 {
		s.releaseBlob(r.Context(), was.ImageID)
	}
	s.logBadge(r, store.LogBadgeDelete, nil, map[string]any{"name": was.Name})
	w.WriteHeader(http.StatusNoContent)
}

// holderJSON is one holder with what the list shows of them.
type holderJSON struct {
	store.BadgeHolder
	User *store.PublicUser `json:"user,omitempty"`
}

func (s *Server) holdersJSON(r *http.Request, list []store.BadgeHolder) ([]holderJSON, error) {
	want := make([]ids.ID, 0, len(list))
	for _, h := range list {
		want = append(want, h.UserID)
	}
	users, err := s.Store.UsersByIDs(r.Context(), want)
	if err != nil {
		return nil, err
	}
	out := make([]holderJSON, 0, len(list))
	for _, h := range list {
		j := holderJSON{BadgeHolder: h}
		if u := users[h.UserID]; u != nil {
			p := u.Public()
			j.User = &p
		}
		out = append(out, j)
	}
	return out, nil
}

// knownBadge reports whether a badge id is one the holders routes take.
func knownBadge(b string) bool {
	switch b {
	case store.BadgeStaff, store.BadgePlan:
		return true
	}
	return store.OverridableBadges[b] > 0 || store.IsGrantableBadge(b)
}

// handleModBadgeHolders lists who holds a badge, earned or given, a page at
// a time (?offset, ?limit up to 100), and who staff took an earned one from.
func (s *Server) handleModBadgeHolders(w http.ResponseWriter, r *http.Request) {
	badge := r.PathValue("badge")
	if !knownBadge(badge) {
		WriteError(w, ErrNotFound)
		return
	}
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	list, taken, total, err := s.Store.BadgeHolders(r.Context(), badge, offset, limit)
	if err != nil {
		WriteError(w, err)
		return
	}
	held, err := s.holdersJSON(r, list)
	if err != nil {
		WriteError(w, err)
		return
	}
	gone, err := s.holdersJSON(r, taken)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"holders": held, "taken": gone, "total": total})
}

// handleModBadgeHolderPut gives one badge to one account or takes it away:
// {"state": "give"|"take"|"auto", "level": n}. An earned badge given or taken
// stays so until set back to "auto".
func (s *Server) handleModBadgeHolderPut(w http.ResponseWriter, r *http.Request) {
	badge := r.PathValue("badge")
	uid, err := ids.Parse(r.PathValue("uid"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	u, err := s.Store.UserByID(r.Context(), uid)
	if err != nil {
		WriteError(w, err)
		return
	}
	if u.Flags&(store.NotPeople|store.FlagBot|store.FlagDeleted) != 0 {
		WriteError(w, E(http.StatusBadRequest, "no_badges", "This account cannot hold badges."))
		return
	}
	var body struct {
		State string `json:"state"`
		Level int    `json:"level"`
	}
	if err := ReadJSON(w, r, &body, 1<<10); err != nil {
		WriteError(w, err)
		return
	}
	changed, err := s.Store.SetBadgeHolder(r.Context(), badge, u.ID, auth.UserFrom(r.Context()).ID, body.State, body.Level)
	if errors.Is(err, store.ErrBadBadge) {
		WriteError(w, E(http.StatusBadRequest, "bad_badge", "Staff cannot change that badge that way."))
		return
	}
	if err != nil {
		WriteError(w, err)
		return
	}
	if changed {
		s.logBadgeChange(r, u, []store.BadgeChange{{Badge: badge, State: body.State, Level: body.Level}})
	}
	w.WriteHeader(http.StatusNoContent)
}

// logBadgeChange logs changes in the shape the panel's log reads: what was
// given, what was taken, and which earned badges follow the account again.
func (s *Server) logBadgeChange(r *http.Request, u *store.User, changes []store.BadgeChange) {
	added, removed, auto := []string{}, []string{}, []string{}
	for _, c := range changes {
		name := s.badgeNames(r, []string{c.Badge})[0]
		switch c.State {
		case "give":
			added = append(added, name)
		case "take":
			removed = append(removed, name)
		default:
			auto = append(auto, name)
		}
	}
	details := map[string]any{"added": added, "removed": removed}
	if len(auto) > 0 {
		details["automatic"] = auto
	}
	s.logBadge(r, store.LogBadgeChange, u, details)
}

// badgeTarget is the account a staff badge route names, refusing bots,
// Nexa's own accounts and deleted ones, which hold no badges.
func (s *Server) badgeTarget(w http.ResponseWriter, r *http.Request) *store.User {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return nil
	}
	u, err := s.Store.UserByID(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return nil
	}
	if u.Flags&(store.NotPeople|store.FlagBot|store.FlagDeleted) != 0 {
		WriteError(w, E(http.StatusBadRequest, "no_badges", "This account cannot hold badges."))
		return nil
	}
	return u
}

func (s *Server) handleModUserBadges(w http.ResponseWriter, r *http.Request) {
	u := s.badgeTarget(w, r)
	if u == nil {
		return
	}
	s.writeUserBadges(w, r, u)
}

// writeUserBadges answers what staff gave an account by hand, how they set
// its earned badges, and what it earned of each.
func (s *Server) writeUserBadges(w http.ResponseWriter, r *http.Request, u *store.User) {
	granted, err := s.Store.GrantedBadges(r.Context(), u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	overrides, err := s.Store.BadgeOverrides(r.Context(), u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	earned, err := s.Store.EarnedLevels(r.Context(), u)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"granted": granted, "overrides": overrides, "earned": earned})
}

func (s *Server) handleModUserBadgesPut(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	u := s.badgeTarget(w, r)
	if u == nil {
		return
	}
	var body struct {
		Badges []string `json:"badges"`
		// Overrides, when sent, are exactly how staff set the account's
		// earned badges (0 taken away, above 0 given at that level); older
		// apps leave it out and change none.
		Overrides map[string]int `json:"overrides"`
	}
	if err := ReadJSON(w, r, &body, 16<<10); err != nil {
		WriteError(w, err)
		return
	}
	if len(body.Badges) > 64 {
		WriteError(w, E(http.StatusBadRequest, "too_many_badges", "That is too many badges."))
		return
	}
	for _, b := range body.Badges {
		if !store.IsGrantableBadge(b) {
			WriteError(w, E(http.StatusBadRequest, "bad_badge", "Staff cannot give that badge."))
			return
		}
	}
	for b := range body.Overrides {
		if store.OverridableBadges[b] == 0 {
			WriteError(w, E(http.StatusBadRequest, "bad_badge", "Staff cannot change that badge."))
			return
		}
	}
	added, removed, err := s.Store.SetGrantedBadges(r.Context(), u.ID, me.ID, body.Badges)
	if err != nil {
		WriteError(w, err)
		return
	}
	var changes []store.BadgeChange
	if body.Overrides != nil {
		if changes, err = s.Store.SetBadgeOverrides(r.Context(), u.ID, me.ID, body.Overrides); err != nil {
			WriteError(w, err)
			return
		}
	}
	for _, b := range added {
		changes = append(changes, store.BadgeChange{Badge: b, State: "give"})
	}
	for _, b := range removed {
		changes = append(changes, store.BadgeChange{Badge: b, State: "take"})
	}
	if len(changes) > 0 {
		s.logBadgeChange(r, u, changes)
	}
	s.writeUserBadges(w, r, u)
}

// badgeNames names given badges for the log: Contributor by its id (the
// panel translates it), an event badge by its name now, since it can go.
func (s *Server) badgeNames(r *http.Request, list []string) []string {
	out := []string{}
	for _, b := range list {
		if n, ok := strings.CutPrefix(b, store.EventBadgePrefix); ok {
			if id, err := ids.Parse(n); err == nil {
				if d, err := s.Store.BadgeDef(r.Context(), id); err == nil {
					out = append(out, d.Name)
					continue
				}
			}
		}
		out = append(out, b)
	}
	return out
}

func (s *Server) logBadge(r *http.Request, action string, target *store.User, details map[string]any) {
	me := auth.UserFrom(r.Context())
	e := &store.ModLogEntry{Action: action, ModeratorID: me.ID, ModeratorUsername: me.Username, Details: mustJSON(details)}
	if target != nil {
		e.TargetID, e.TargetUsername = &target.ID, target.Username
	}
	if err := s.Store.AppendModLog(r.Context(), e); err != nil {
		s.Log.Warn("badges: log", "action", action, "err", err)
		return
	}
	s.emitLog(r.Context(), e)
}
