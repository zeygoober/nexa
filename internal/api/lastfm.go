package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/gateway"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/lastfm"
	"evilion.xyz/chat/internal/store"
)

var (
	// Last.fm's own rule for names.
	lastfmUsername = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{1,14}$`)
	errLastfmOff   = E(http.StatusServiceUnavailable, "lastfm_off", "Last.fm is not set up on this server.")
)

const (
	lastfmEvery   = 30 * time.Second
	lastfmPace    = 300 * time.Millisecond
	lastfmBackoff = 5 * time.Minute
)

// WireLastfm turns the integration on when this server has a key.
func (s *Server) WireLastfm(hub *gateway.Hub, key, base string) {
	if key == "" {
		return
	}
	var transport http.RoundTripper
	if s.Egress != nil {
		transport = s.Egress.Transport()
	}
	s.Lastfm = lastfm.New(key, base, transport)
	s.lastfmHub = hub
}

func (s *Server) lastfmRoutes(mux *http.ServeMux) {
	const path = "/api/v1/connections/lastfm"
	mux.Handle("GET "+path, s.authed(s.handleLastfmGet))
	mux.Handle("PUT "+path, s.authed(s.handleLastfmLink))
	mux.Handle("PATCH "+path, s.authed(s.handleLastfmShow))
	mux.Handle("DELETE "+path, s.authed(s.handleLastfmUnlink))
}

func lastfmJSON(available bool, l *store.LastfmLink) map[string]any {
	out := map[string]any{"available": available, "linked": nil}
	if l != nil {
		out["linked"] = map[string]any{"username": l.Username, "show": l.Show, "url": "https://www.last.fm/user/" + l.Username}
	}
	return out
}

func (s *Server) handleLastfmGet(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	l, err := s.Store.LastfmLinkOf(r.Context(), me.ID)
	if errors.Is(err, store.ErrNotFound) {
		l = nil
	} else if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, lastfmJSON(s.Lastfm != nil, l))
}

func (s *Server) handleLastfmLink(w http.ResponseWriter, r *http.Request) {
	if s.Lastfm == nil {
		WriteError(w, errLastfmOff)
		return
	}
	me := auth.UserFrom(r.Context())
	var req struct {
		Username string `json:"username"`
		Show     bool   `json:"show"`
	}
	if err := ReadJSON(w, r, &req, 1<<10); err != nil {
		WriteError(w, err)
		return
	}
	name := strings.TrimSpace(req.Username)
	if !lastfmUsername.MatchString(name) {
		WriteError(w, E(http.StatusBadRequest, "lastfm_not_found", "There is no Last.fm account with that name."))
		return
	}
	if ok, retry := s.limLastfm.Allow(me.ID.String()); !ok {
		w.Header().Set("Retry-After", retryAfter(retry))
		WriteError(w, ErrRateLimited)
		return
	}
	got, err := s.Lastfm.Recent(r.Context(), name)
	switch {
	case errors.Is(err, lastfm.ErrNotFound):
		WriteError(w, E(http.StatusBadRequest, "lastfm_not_found", "There is no Last.fm account with that name."))
		return
	case errors.Is(err, lastfm.ErrPrivate):
		WriteError(w, E(http.StatusBadRequest, "lastfm_private", "That account hides its listening. Turn off Hide Recent Listening in your Last.fm privacy settings first."))
		return
	case err != nil:
		WriteError(w, E(http.StatusBadGateway, "lastfm_failed", "Last.fm did not answer. Try again in a moment."))
		return
	}
	if err := s.Store.SetLastfmLink(r.Context(), me.ID, got.User, req.Show); err != nil {
		WriteError(w, err)
		return
	}
	if req.Show {
		s.showListening(me.ID, got.Playing)
	} else {
		s.showListening(me.ID, nil)
	}
	WriteJSON(w, http.StatusOK, lastfmJSON(true, &store.LastfmLink{UserID: me.ID, Username: got.User, Show: req.Show}))
}

func (s *Server) handleLastfmShow(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	var req struct {
		Show bool `json:"show"`
	}
	if err := ReadJSON(w, r, &req, 1<<10); err != nil {
		WriteError(w, err)
		return
	}
	l, err := s.Store.LastfmLinkOf(r.Context(), me.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.SetLastfmShow(r.Context(), me.ID, req.Show); err != nil {
		WriteError(w, err)
		return
	}
	if !req.Show {
		s.showListening(me.ID, nil)
	}
	l.Show = req.Show
	WriteJSON(w, http.StatusOK, lastfmJSON(s.Lastfm != nil, l))
}

func (s *Server) handleLastfmUnlink(w http.ResponseWriter, r *http.Request) {
	me := auth.UserFrom(r.Context())
	if err := s.Store.DeleteLastfmLink(r.Context(), me.ID); err != nil {
		WriteError(w, err)
		return
	}
	s.showListening(me.ID, nil)
	WriteJSON(w, http.StatusOK, lastfmJSON(s.Lastfm != nil, nil))
}

// showListening puts a song on someone's activity, or takes it off. No artwork:
// Last.fm's terms do not allow it.
func (s *Server) showListening(uid ids.ID, t *lastfm.Track) {
	if s.lastfmHub == nil {
		return
	}
	if t == nil {
		s.lastfmHub.SetListening(uid, nil)
		return
	}
	raw, err := json.Marshal(map[string]any{
		"name": "Last.fm", "type": 2,
		"details": t.Name, "details_url": t.URL,
		"state": t.Artist, "state_url": lastfm.ArtistURL(t.URL),
	})
	if err != nil {
		return
	}
	s.lastfmHub.SetListening(uid, raw)
}

// StartLastfm asks what each linked person who is online is listening to.
func (s *Server) StartLastfm(ctx context.Context) {
	if s.Lastfm == nil {
		return
	}
	go func() {
		tick := time.NewTicker(lastfmEvery)
		defer tick.Stop()
		for {
			wait := s.pollLastfm(ctx)
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			if wait > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
			}
		}
	}()
}

// pollLastfm goes through the links once, and says how long to stay away.
func (s *Server) pollLastfm(ctx context.Context) time.Duration {
	links, err := s.Store.LastfmShowing(ctx)
	if err != nil {
		s.Log.Warn("lastfm links", "err", err)
		return 0
	}
	for _, l := range links {
		if !s.lastfmHub.Online(l.UserID) {
			if s.lastfmHub.Listening(l.UserID) {
				s.showListening(l.UserID, nil)
			}
			continue
		}
		got, err := s.Lastfm.Recent(ctx, l.Username)
		switch {
		case errors.Is(err, lastfm.ErrRateLimited):
			s.Log.Warn("lastfm rate limited")
			return lastfmBackoff
		case errors.Is(err, lastfm.ErrNotFound), errors.Is(err, lastfm.ErrPrivate):
			s.showListening(l.UserID, nil)
		case err == nil:
			s.showListening(l.UserID, got.Playing)
		}
		// any other failure leaves what was showing: it says nothing about the song
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(lastfmPace):
		}
	}
	return 0
}
