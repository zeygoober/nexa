// Package rpcapps looks up the public face of a Discord Rich Presence
// application: the name and icon its activity shows under, and the art assets
// its images name. chatd asks Discord once per application and keeps the
// answer, so nobody using Nexa contacts Discord for it themselves.
package rpcapps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ErrUnknown means Discord has no such application (or the id is not one).
var ErrUnknown = errors.New("rpcapps: unknown application")

// ErrBusy means Discord was not asked just now: it asked to slow down, or new
// applications are being looked up faster than Pace lets out. It is not
// remembered, so a later lookup asks again.
var ErrBusy = errors.New("rpcapps: busy, try again later")

var appIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)

// App is what Discord says publicly about one application.
type App struct {
	ID      string
	Name    string
	IconURL string // empty when the application has no icon
	cdn     string
	assets  map[string]string // lower-cased asset name -> asset id
}

// AssetURL is the CDN address of one of the application's art assets, given
// its name or id, or "" when it has none by that name.
func (a *App) AssetURL(key string) string {
	if a == nil || key == "" {
		return ""
	}
	id := key
	if !isDigits(key) {
		id = a.assets[strings.ToLower(key)]
		if id == "" {
			return ""
		}
	}
	return fmt.Sprintf("%s/app-assets/%s/%s.png", a.cdn, a.ID, id)
}

// Cache fetches applications from Discord's public API and keeps them.
type Cache struct {
	Base      string // Discord's API root
	CDN       string // Discord's CDN root
	Client    *http.Client
	UserAgent string
	TTL       time.Duration // how long a found application is kept
	FailTTL   time.Duration // how long a failure is kept before asking again
	Max       int           // applications kept at most
	// Pace spaces out requests to Discord across every lookup. Discord blocks
	// an address that keeps being refused, and this server's address is also
	// the address of whoever runs it and their own Discord. nil is no pacing.
	Pace *rate.Limiter

	now         func() time.Time
	mu          sync.Mutex
	entries     map[string]*entry
	pausedUntil time.Time // Discord answered 429: nothing is asked before this
}

type entry struct {
	app  *App
	err  error
	at   time.Time
	done chan struct{}
}

// New returns a cache that asks Discord's production API.
func New() *Cache {
	return &Cache{
		Base:      "https://discord.com/api/v10",
		CDN:       "https://cdn.discordapp.com",
		Client:    &http.Client{Timeout: 5 * time.Second},
		UserAgent: "Nexa (https://nexa.pm, rich presence)",
		TTL:       24 * time.Hour,
		FailTTL:   10 * time.Minute,
		Max:       5000,
		Pace:      rate.NewLimiter(rate.Every(time.Second), 20),
		now:       time.Now,
		entries:   map[string]*entry{},
	}
}

// Lookup returns the application with that id. Lookups of one application
// that overlap share a single request.
func (c *Cache) Lookup(ctx context.Context, id string) (*App, error) {
	if !appIDPattern.MatchString(id) {
		return nil, ErrUnknown
	}
	c.mu.Lock()
	if e, ok := c.entries[id]; ok {
		select {
		case <-e.done:
			ttl := c.TTL
			if e.err != nil {
				ttl = c.FailTTL
			}
			if !errors.Is(e.err, ErrBusy) && c.now().Sub(e.at) < ttl {
				c.mu.Unlock()
				return e.app, e.err
			}
		default:
			c.mu.Unlock()
			return c.wait(ctx, e)
		}
	}
	e := &entry{done: make(chan struct{})}
	c.entries[id] = e
	c.evictLocked()
	c.mu.Unlock()

	// the fetch is not tied to this caller, whose give-up must not poison the cache for everyone
	go func() {
		fctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		app, err := c.fetch(fctx, id)
		c.mu.Lock()
		e.app, e.err, e.at = app, err, c.now()
		c.mu.Unlock()
		close(e.done)
	}()
	return c.wait(ctx, e)
}

func (c *Cache) wait(ctx context.Context, e *entry) (*App, error) {
	select {
	case <-e.done:
		c.mu.Lock()
		defer c.mu.Unlock()
		return e.app, e.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// evictLocked drops the oldest finished entries once there are too many.
func (c *Cache) evictLocked() {
	for len(c.entries) > c.Max {
		var oldest string
		var at time.Time
		for id, e := range c.entries {
			select {
			case <-e.done:
			default:
				continue
			}
			if oldest == "" || e.at.Before(at) {
				oldest, at = id, e.at
			}
		}
		if oldest == "" {
			return
		}
		delete(c.entries, oldest)
	}
}

func (c *Cache) fetch(ctx context.Context, id string) (*App, error) {
	var info struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	status, err := c.getJSON(ctx, "/applications/"+id+"/rpc", &info)
	if err != nil {
		if status == http.StatusNotFound || status == http.StatusBadRequest {
			return nil, ErrUnknown
		}
		return nil, err
	}
	app := &App{ID: id, Name: info.Name, cdn: c.CDN, assets: map[string]string{}}
	if info.Icon != "" {
		app.IconURL = fmt.Sprintf("%s/app-icons/%s/%s.png?size=128", c.CDN, id, info.Icon)
	}
	var assets []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	// an application without art, or a hiccup fetching it, still has a name;
	// art that was only put off is asked for again with the name
	if _, err := c.getJSON(ctx, "/oauth2/applications/"+id+"/assets", &assets); err == nil {
		for _, a := range assets {
			if isDigits(a.ID) && a.Name != "" {
				app.assets[strings.ToLower(a.Name)] = a.ID
			}
		}
	} else if errors.Is(err, ErrBusy) {
		return nil, ErrBusy
	}
	return app, nil
}

// permit waits for a turn to ask Discord, or says ErrBusy when none comes in time.
func (c *Cache) permit(ctx context.Context) error {
	c.mu.Lock()
	paused := c.now().Before(c.pausedUntil)
	c.mu.Unlock()
	if paused {
		return ErrBusy
	}
	if c.Pace != nil && c.Pace.Wait(ctx) != nil {
		return ErrBusy
	}
	return nil
}

// pause stops every lookup for as long as Discord's Retry-After says: a
// minute when it says nothing usable, an hour at most.
func (c *Cache) pause(retryAfter string) {
	d := time.Minute
	if s, err := strconv.ParseFloat(strings.TrimSpace(retryAfter), 64); err == nil && s > 0 && s < 86400 {
		d = time.Duration(s * float64(time.Second))
	}
	d = min(max(d, time.Second), time.Hour)
	c.mu.Lock()
	if until := c.now().Add(d); until.After(c.pausedUntil) {
		c.pausedUntil = until
	}
	c.mu.Unlock()
}

func (c *Cache) getJSON(ctx context.Context, path string, out any) (int, error) {
	if err := c.permit(ctx); err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := c.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		c.pause(res.Header.Get("Retry-After"))
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
		return res.StatusCode, ErrBusy
	}
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
		return res.StatusCode, fmt.Errorf("rpcapps: %s answered %d", path, res.StatusCode)
	}
	return res.StatusCode, json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// TestApp builds an App without asking Discord, for tests elsewhere.
func TestApp(id, name, iconURL string, assets map[string]string, cdn string) *App {
	a := &App{ID: id, Name: name, IconURL: iconURL, cdn: cdn, assets: map[string]string{}}
	for k, v := range assets {
		a.assets[strings.ToLower(k)] = v
	}
	return a
}
