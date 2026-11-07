package discordsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultAPI is Discord's REST API.
const DefaultAPI = "https://discord.com/api/v10"

// APIError is a REST call Discord refused.
type APIError struct {
	Status int
	Code   int    // Discord's JSON error code, when it sent one
	OAuth  string // OAuth's error ("invalid_grant"), when it is that
	Msg    string
}

func (e *APIError) Error() string {
	if e.OAuth != "" {
		return fmt.Sprintf("discord answered %d: %s", e.Status, e.OAuth)
	}
	return fmt.Sprintf("discord answered %d: %s (code %d)", e.Status, e.Msg, e.Code)
}

// rest calls Discord's REST API as the bot or as a person.
type rest struct {
	base   string
	client *http.Client
	bot    string // the bot token
	id     string // the application's client id
	secret string // and its secret
}

// Tokens are what OAuth hands over for a person.
type Tokens struct {
	Access    string
	Refresh   string
	ExpiresAt time.Time
	Scope     string
}

// User is a Discord account as /users/@me describes it.
type User struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Avatar     string `json:"avatar"`
}

// do sends one request and decodes a JSON answer into out, waiting out one
// rate limit if Discord asks for a short wait.
func (r *rest) do(ctx context.Context, method, path string, auth string, body io.Reader, contentType string, out any) (int, error) {
	var buf []byte
	if body != nil {
		b, err := io.ReadAll(body)
		if err != nil {
			return 0, err
		}
		buf = b
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, r.base+path, bytes.NewReader(buf))
		if err != nil {
			return 0, err
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("User-Agent", "DiscordBot (https://nexa.pm, 1) Nexa")
		res, err := r.client.Do(req)
		if err != nil {
			return 0, err
		}
		raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if err != nil {
			return res.StatusCode, err
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			var rl struct {
				RetryAfter float64 `json:"retry_after"`
			}
			_ = json.Unmarshal(raw, &rl)
			wait := time.Duration(rl.RetryAfter * float64(time.Second))
			if wait <= 0 {
				if s, err := strconv.ParseFloat(res.Header.Get("Retry-After"), 64); err == nil {
					wait = time.Duration(s * float64(time.Second))
				}
			}
			if wait > 0 && wait <= 15*time.Second {
				select {
				case <-ctx.Done():
					return res.StatusCode, ctx.Err()
				case <-time.After(wait):
				}
				continue
			}
		}
		if res.StatusCode >= 300 {
			e := &APIError{Status: res.StatusCode}
			var j struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Error   string `json:"error"`
			}
			if json.Unmarshal(raw, &j) == nil {
				e.Code, e.Msg, e.OAuth = j.Code, j.Message, j.Error
			}
			if e.Msg == "" && e.OAuth == "" {
				e.Msg = strings.TrimSpace(string(raw))
				if len(e.Msg) > 200 {
					e.Msg = e.Msg[:200]
				}
			}
			return res.StatusCode, e
		}
		if out != nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, out); err != nil {
				return res.StatusCode, fmt.Errorf("discord sent something unexpected for %s: %w", path, err)
			}
		}
		return res.StatusCode, nil
	}
}

func (r *rest) botAuth() string { return "Bot " + r.bot }

// gatewayURL is where the bot connects for a fresh session, waiting first if
// it has used up the day's session starts.
func (r *rest) gatewayURL(ctx context.Context) (string, error) {
	var g struct {
		URL   string `json:"url"`
		Limit struct {
			Remaining  int   `json:"remaining"`
			ResetAfter int64 `json:"reset_after"`
		} `json:"session_start_limit"`
	}
	if _, err := r.do(ctx, http.MethodGet, "/gateway/bot", r.botAuth(), nil, "", &g); err != nil {
		return "", err
	}
	if g.URL == "" {
		return "", errors.New("discord gave no gateway address")
	}
	if g.Limit.Remaining == 0 && g.Limit.ResetAfter > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(g.Limit.ResetAfter) * time.Millisecond):
		}
	}
	return g.URL, nil
}

func (r *rest) token(ctx context.Context, form url.Values) (*Tokens, error) {
	var t struct {
		Access    string `json:"access_token"`
		Refresh   string `json:"refresh_token"`
		ExpiresIn int64  `json:"expires_in"`
		Scope     string `json:"scope"`
	}
	form.Set("client_id", r.id)
	form.Set("client_secret", r.secret)
	if _, err := r.do(ctx, http.MethodPost, "/oauth2/token", "", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", &t); err != nil {
		return nil, err
	}
	if t.Access == "" {
		return nil, errors.New("discord sent no access token")
	}
	return &Tokens{Access: t.Access, Refresh: t.Refresh, Scope: t.Scope, ExpiresAt: time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)}, nil
}

// exchange turns the code Discord sent back with the person into their tokens.
func (r *rest) exchange(ctx context.Context, code, redirect string) (*Tokens, error) {
	return r.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}})
}

// refresh swaps a refresh token for new tokens.
func (r *rest) refresh(ctx context.Context, refresh string) (*Tokens, error) {
	return r.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
}

// revoke tells Discord a token is no longer wanted, and every token issued with it goes too.
func (r *rest) revoke(ctx context.Context, token string) error {
	form := url.Values{"token": {token}, "token_type_hint": {"refresh_token"}, "client_id": {r.id}, "client_secret": {r.secret}}
	_, err := r.do(ctx, http.MethodPost, "/oauth2/token/revoke", "", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", nil)
	return err
}

// redirects are the callbacks listed for the application under OAuth2,
// Redirects in the Developer Portal, which the bot may read of its own
// application. Discord opens its page for linking only with one of these.
func (r *rest) redirects(ctx context.Context) ([]string, error) {
	var a struct {
		Redirects *[]string `json:"redirect_uris"`
	}
	if _, err := r.do(ctx, http.MethodGet, "/applications/@me", r.botAuth(), nil, "", &a); err != nil {
		return nil, err
	}
	if a.Redirects == nil {
		return nil, errors.New("discord did not say which redirects are listed")
	}
	return *a.Redirects, nil
}

// me is the Discord account an access token belongs to.
func (r *rest) me(ctx context.Context, access string) (*User, error) {
	var u User
	if _, err := r.do(ctx, http.MethodGet, "/users/@me", "Bearer "+access, nil, "", &u); err != nil {
		return nil, err
	}
	if u.ID == "" {
		return nil, errors.New("discord did not say who this is")
	}
	return &u, nil
}

// addMember puts a person in the guild with their own access token (the
// guilds.join scope), as the bot. Someone already in it is left as they are.
func (r *rest) addMember(ctx context.Context, guildID, userID, access string) error {
	body, _ := json.Marshal(map[string]string{"access_token": access})
	_, err := r.do(ctx, http.MethodPut, "/guilds/"+guildID+"/members/"+userID, r.botAuth(), bytes.NewReader(body), "application/json", nil)
	return err
}

// removeMember takes a person out of the guild, as the bot.
func (r *rest) removeMember(ctx context.Context, guildID, userID string) error {
	_, err := r.do(ctx, http.MethodDelete, "/guilds/"+guildID+"/members/"+userID, r.botAuth(), nil, "", nil)
	return err
}

// guildOwner is who owns the guild; they cannot be removed from it.
func (r *rest) guildOwner(ctx context.Context, guildID string) (string, error) {
	var g struct {
		OwnerID string `json:"owner_id"`
	}
	if _, err := r.do(ctx, http.MethodGet, "/guilds/"+guildID, r.botAuth(), nil, "", &g); err != nil {
		return "", err
	}
	return g.OwnerID, nil
}
