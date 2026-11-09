// Package lastfm asks Last.fm what a person is listening to. Last.fm's API
// terms decide what the apps may do with the answer: no artwork, and every
// display credits Last.fm and links to its pages.
package lastfm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBase = "https://ws.audioscrobbler.com/2.0/"

// What Last.fm can answer instead of a listening history.
var (
	ErrNotFound    = errors.New("lastfm: no such user")
	ErrPrivate     = errors.New("lastfm: listening history is private")
	ErrRateLimited = errors.New("lastfm: rate limited")
	ErrUnavailable = errors.New("lastfm: unavailable")
)

// Client is safe to share. Its key is the instance's own.
type Client struct {
	key, base string
	http      *http.Client
}

// New makes a client; base and transport may be left empty.
func New(key, base string, transport http.RoundTripper) *Client {
	if base == "" {
		base = defaultBase
	}
	return &Client{key: key, base: base, http: &http.Client{Timeout: 10 * time.Second, Transport: transport}}
}

// Track is the song a person is listening to.
type Track struct {
	Name, Artist, URL string
}

// Recent is the answer about one account: its name as Last.fm writes it, and
// what is playing, nil when nothing is.
type Recent struct {
	User    string
	Playing *Track
}

// Recent asks what a Last.fm user is listening to now.
func (c *Client) Recent(ctx context.Context, user string) (*Recent, error) {
	q := url.Values{"method": {"user.getrecenttracks"}, "user": {user}, "api_key": {c.key}, "format": {"json"}, "limit": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Nexa")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, ErrUnavailable
	}
	return parseRecent(body, res.StatusCode)
}

func parseRecent(body []byte, status int) (*Recent, error) {
	var answer struct {
		Error  int `json:"error"`
		Recent struct {
			Track json.RawMessage `json:"track"`
			Attr  struct {
				User string `json:"user"`
			} `json:"@attr"`
		} `json:"recenttracks"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, ErrUnavailable
	}
	switch answer.Error {
	case 0:
	case 6:
		return nil, ErrNotFound
	case 17:
		return nil, ErrPrivate
	case 29:
		return nil, ErrRateLimited
	default:
		return nil, ErrUnavailable
	}
	if status != http.StatusOK || answer.Recent.Attr.User == "" {
		return nil, ErrUnavailable
	}
	out := &Recent{User: answer.Recent.Attr.User}
	for _, t := range tracks(answer.Recent.Track) {
		if t.Attr.NowPlaying == "true" && t.Name != "" {
			out.Playing = &Track{Name: t.Name, Artist: t.Artist.Text, URL: ownURL(t.URL)}
			break
		}
	}
	return out, nil
}

type wireTrack struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Artist struct {
		Text string `json:"#text"`
	} `json:"artist"`
	Attr struct {
		NowPlaying string `json:"nowplaying"`
	} `json:"@attr"`
}

// tracks reads Last.fm's list, which is a lone object when it holds one song.
func tracks(raw json.RawMessage) []wireTrack {
	var list []wireTrack
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var one wireTrack
	if json.Unmarshal(raw, &one) == nil {
		return []wireTrack{one}
	}
	return nil
}

// ownURL keeps an address only if it is one of Last.fm's own pages.
func ownURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "www.last.fm" {
		return ""
	}
	return raw
}

// ArtistURL is the page of a track's artist, from the track's own address
// (https://www.last.fm/music/Artist/_/Song).
func ArtistURL(trackURL string) string {
	if before, _, ok := strings.Cut(trackURL, "/_/"); ok {
		return before
	}
	return ""
}
