package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// Discovery: servers their owners chose to show to everyone on Nexa, with
// nothing asked of them first. Each has a category, up to five tags and a
// language, and is found by search, filtered by those and by size, and
// joined straight from the list. Its channels stay end-to-end encrypted: a
// newcomer's keys come from the members' apps, as with an invite that does
// not carry one.

// DiscoveryCategories are the categories a listed server can be in.
var DiscoveryCategories = []string{"gaming", "music", "entertainment", "creative", "science_tech", "education", "anime", "sports", "community", "other"}

// discoveryLanguages are the languages a listing can name: the ones the app speaks.
var discoveryLanguages = []string{"en", "ar", "de", "es", "fr", "hi", "ja", "ko", "pt-BR", "ru", "zh-CN"}

const (
	maxDiscoveryTags = 5
	maxTagLen        = 20
)

var discoveryTagChars = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} \-]*$`)

func (s *Server) discoveryRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/discovery", s.authed(s.handleDiscover))
	mux.Handle("GET /api/v1/discovery/categories", s.authed(s.handleDiscoveryCategories))
	mux.Handle("GET /api/v1/discovery/{id}", s.authed(s.handleDiscoveryServer))
	mux.Handle("POST /api/v1/discovery/{id}/join", s.userLimited(s.limJoin, s.handleDiscoveryJoin))
}

// cleanDiscoveryTags checks and tidies a listing's tags: lower case, single
// spaces, no repeats, at most five of two to twenty letters.
func cleanDiscoveryTags(raw []string) ([]string, bool) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range raw {
		t = strings.ToLower(strings.Join(strings.Fields(t), " "))
		if t == "" || t == "nsfw" {
			continue
		}
		n := len([]rune(t))
		if n < 2 || n > maxTagLen || !discoveryTagChars.MatchString(t) {
			return nil, false
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out, len(out) <= maxDiscoveryTags
}

// discoveryPatch reads a server's Discovery settings from a PATCH body into
// p, answering 400 itself and returning false when one is wrong.
func (s *Server) discoveryPatch(w http.ResponseWriter, raw map[string]json.RawMessage, p *store.ServerPatch, changes map[string]any) bool {
	if v, ok := raw["discoverable"]; ok {
		var on bool
		if json.Unmarshal(v, &on) != nil {
			WriteError(w, E(http.StatusBadRequest, "invalid_value", "discoverable must be true or false."))
			return false
		}
		p.Discoverable = &on
		changes["discoverable"] = on
	}
	if v, ok := raw["discovery_category"]; ok {
		var cat string
		if json.Unmarshal(v, &cat) != nil || (cat != "" && !OneOf(cat, DiscoveryCategories...)) {
			WriteError(w, E(http.StatusBadRequest, "invalid_category", "Pick one of Discovery's categories."))
			return false
		}
		p.DiscoveryCategory = &cat
		changes["discovery_category"] = cat
	}
	if v, ok := raw["discovery_tags"]; ok {
		var list []string
		if json.Unmarshal(v, &list) != nil {
			WriteError(w, E(http.StatusBadRequest, "invalid_tags", "Tags are a list of words."))
			return false
		}
		clean, ok := cleanDiscoveryTags(list)
		if !ok {
			WriteError(w, E(http.StatusBadRequest, "invalid_tags", "Up to 5 tags, each 2 to 20 letters, numbers, spaces or dashes."))
			return false
		}
		p.DiscoveryTags = &clean
		changes["discovery_tags"] = clean
	}
	if v, ok := raw["discovery_language"]; ok {
		var lang string
		if json.Unmarshal(v, &lang) != nil || (lang != "" && !OneOf(lang, discoveryLanguages...)) {
			WriteError(w, E(http.StatusBadRequest, "invalid_language", "Pick one of the languages Nexa speaks."))
			return false
		}
		p.DiscoveryLanguage = &lang
		changes["discovery_language"] = lang
	}
	return true
}

// discoveryCard is a listed server as Discovery shows it.
type discoveryCard struct {
	ID          ids.ID           `json:"id"`
	Name        string           `json:"name"`
	IconID      *ids.ID          `json:"icon_id"`
	BannerID    *ids.ID          `json:"banner_id"`
	Description string           `json:"description"`
	Category    string           `json:"category"`
	Tags        []string         `json:"tags"`
	Language    string           `json:"language"`
	MemberCount int              `json:"member_count"`
	OnlineCount int              `json:"online_count"`
	CreatedAt   int64            `json:"created_at"`
	ListedAt    *int64           `json:"listed_at"`
	Tag         *store.ServerTag `json:"tag"`
	Member      bool             `json:"member"`
	// JoinsPaused says the server takes nobody new right now.
	JoinsPaused bool `json:"joins_paused"`
	NSFW        bool `json:"nsfw"`
	NSFWContent bool `json:"nsfw_content"`
}

func (s *Server) card(r *http.Request, l *store.Listed, me ids.ID) discoveryCard {
	_, online := s.counts(r.Context(), l.ID)
	member, _ := s.Store.IsMember(r.Context(), l.ID, me)
	c := discoveryCard{ID: l.ID, Name: l.Name, IconID: l.IconID, BannerID: l.BannerID, Description: l.Description, Category: l.DiscoveryCategory,
		Tags: l.DiscoveryTags, Language: l.DiscoveryLanguage, MemberCount: l.MemberCount, OnlineCount: online, CreatedAt: l.CreatedAt,
		ListedAt: l.DiscoverySince, Member: member, JoinsPaused: joinsPaused(l.Server)}
	c.NSFW = l.NSFW
	c.NSFWContent, _ = s.Store.ServerAgeRestricted(r.Context(), l.ID)
	// NSFW is a reserved, derived Discovery tag, not one of the five custom tags.
	c.Tags = make([]string, 0, len(l.DiscoveryTags)+1)
	for _, tag := range l.DiscoveryTags {
		if !strings.EqualFold(tag, "nsfw") {
			c.Tags = append(c.Tags, tag)
		}
	}
	if c.NSFWContent {
		c.Tags = append(c.Tags, "nsfw")
	}
	if l.Tag != nil && !c.NSFW {
		// what it looks like, never the invite it keeps
		c.Tag = &store.ServerTag{Text: l.Tag.Text, Badge: l.Tag.Badge, Color: l.Tag.Color, Roles: []ids.ID{}}
	}
	return c
}

// joinsPaused reports whether a server has paused its invites.
func joinsPaused(srv *store.Server) bool {
	return srv.InvitesPausedUntil != nil && *srv.InvitesPausedUntil > time.Now().UnixMilli()
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	q := r.URL.Query()
	atoi := func(k string) int {
		n, _ := strconv.Atoi(q.Get(k))
		return n
	}
	dq := store.DiscoveryQuery{ShowNSFW: q.Get("nsfw") == "1", NSFWOnly: q.Get("nsfw_only") == "1", Query: q.Get("q"), Category: q.Get("category"), Tag: q.Get("tag"), Language: q.Get("language"),
		MinMembers: atoi("min_members"), MaxMembers: atoi("max_members"), Sort: q.Get("sort"), Offset: atoi("offset"), Limit: atoi("limit")}
	if dq.Limit <= 0 || dq.Limit > 48 {
		dq.Limit = 24
	}
	// one more than asked, to say whether there is another page
	want := dq.Limit
	dq.Limit++
	list, err := s.Store.DiscoverServers(r.Context(), dq)
	if err != nil {
		WriteError(w, err)
		return
	}
	more := len(list) > want
	if more {
		list = list[:want]
	}
	out := make([]discoveryCard, 0, len(list))
	for _, l := range list {
		out = append(out, s.card(r, l, me.ID))
	}
	resp := map[string]any{"servers": out, "has_more": more}
	if more {
		resp["next_offset"] = dq.Offset + want
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDiscoveryCategories(w http.ResponseWriter, r *http.Request) {
	counts, err := s.Store.DiscoveryCategoryCounts(r.Context(), r.URL.Query().Get("nsfw") == "1")
	if err != nil {
		WriteError(w, err)
		return
	}
	tags, err := s.Store.PopularDiscoveryTags(r.Context(), 20, r.URL.Query().Get("nsfw") == "1")
	if err != nil {
		WriteError(w, err)
		return
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	WriteJSON(w, http.StatusOK, map[string]any{"categories": DiscoveryCategories, "counts": counts, "total": total, "tags": tags, "languages": discoveryLanguages})
}

// listedFor loads the listed server a Discovery route names.
func (s *Server) listedFor(w http.ResponseWriter, r *http.Request) *store.Listed {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return nil
	}
	l, err := s.Store.ListedServer(r.Context(), id)
	if err != nil {
		WriteError(w, E(http.StatusNotFound, "not_listed", "That server is not on Discovery any more."))
		return nil
	}
	return l
}

func (s *Server) handleDiscoveryServer(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	l := s.listedFor(w, r)
	if l == nil {
		return
	}
	if !s.requireNSFWConsent(w, r, l.ID) {
		return
	}
	resp := map[string]any{"server": s.card(r, l, me.ID)}
	// a taste of how the server talks: its own emoji, the newest first
	if emojis, err := s.Store.ServerEmojis(r.Context(), l.ID); err == nil {
		shown := []map[string]any{}
		for i := len(emojis) - 1; i >= 0 && len(shown) < 8; i-- {
			e := emojis[i]
			shown = append(shown, map[string]any{"id": e.ID, "blob_id": e.BlobID, "name": e.Name, "animated": e.Animated})
		}
		resp["emojis"] = shown
	}
	if owner, err := s.Store.UserByID(r.Context(), l.OwnerID); err == nil {
		resp["owner"] = owner.Public()
	}
	WriteJSON(w, http.StatusOK, resp)
}

// handleDiscoveryJoin joins a listed server, as an invite would.
func (s *Server) handleDiscoveryJoin(w http.ResponseWriter, r *http.Request) {
	if !s.requireCaptcha(w, r, "join") {
		return
	}
	me := auth.UserFrom(r.Context())
	l := s.listedFor(w, r)
	if l == nil {
		return
	}
	srv := l.Server
	if !s.requireNSFWConsent(w, r, srv.ID) {
		return
	}
	s.forgetCounts(srv.ID)
	member, err := s.Store.Join(r.Context(), srv.ID, me.ID, store.JoinInfo{Method: "discovery"})
	switch {
	case errors.Is(err, store.ErrBanned):
		WriteError(w, E(http.StatusForbidden, "banned", "You are banned from this server."))
		return
	case errors.Is(err, store.ErrJoinsPaused):
		WriteError(w, errJoinsPaused)
		return
	case err != nil && err != store.ErrAlreadyExists:
		WriteError(w, err)
		return
	}
	status := http.StatusOK
	if err == nil {
		status = http.StatusCreated
		s.joined(r, srv, me, member)
	}
	view, users, err := s.serverView(r.Context(), srv, me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if status == http.StatusCreated {
		s.Bcast.EmitUsers([]ids.ID{me.ID}, "server_create", serverCreateEvent{view, users})
	}
	WriteJSON(w, status, map[string]any{"server": view, "users": users})
}

var errJoinsPaused = E(http.StatusForbidden, "joins_paused", "This server isn't taking new members right now.")
