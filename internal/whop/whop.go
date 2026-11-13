// Package whop is the little of Whop's API that plans need: reading a plan
// (Whop calls it a variant) for its price, making a checkout, reading and
// cancelling a membership, and checking a webhook's signature. Requests are
// JSON with the API key as a bearer token, pinned to one API version date so
// the objects read back keep their shape.
package whop

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIVersionDate is the version every request asks for (Api-Version-Date).
// From it, a plan is a variant (variant_id) and events name the account_id.
const APIVersionDate = "2026-08-14"

// Client talks to Whop with an API key.
type Client struct {
	Key     string
	BaseURL string // https://api.whop.com/api/v1 unless the sandbox or a test points elsewhere
	HTTP    *http.Client
}

// New is a client for the live API (or BaseURL when not empty).
func New(key, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.whop.com/api/v1"
	}
	return &Client{Key: key, BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Error is what Whop answered a request it refused with.
type Error struct {
	Status  int
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("whop: %d %s: %s", e.Status, e.Type, e.Message) }

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Api-Version-Date", APIVersionDate)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		var env struct {
			Error Error `json:"error"`
		}
		_ = json.Unmarshal(raw, &env)
		env.Error.Status = res.StatusCode
		if env.Error.Message == "" {
			env.Error.Message = strings.TrimSpace(string(raw[:min(len(raw), 300)]))
		}
		return &env.Error
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Cents turns an amount in dollars, as Whop writes prices, into cents.
func Cents(dollars float64) int64 { return int64(math.Round(dollars * 100)) }

// Dollars turns cents into the amount Whop takes.
func Dollars(cents int64) float64 { return float64(cents) / 100 }

// Variant is one of Whop's plans, as far as a price needs it.
type Variant struct {
	ID            string  `json:"id"`
	BillingType   string  `json:"billing_type"`   // renewal or one_time
	BillingPeriod int     `json:"billing_period"` // days between charges
	InitialPrice  float64 `json:"initial_price"`
	RenewalPrice  float64 `json:"renewal_price"`
	Currency      string  `json:"currency"`
}

// Price is what the variant charges each time, in cents: what it renews at,
// or what it costs once.
func (v Variant) Price() int64 {
	if v.BillingType == "renewal" && v.RenewalPrice > 0 {
		return Cents(v.RenewalPrice)
	}
	return Cents(v.InitialPrice)
}

// GetVariant reads one plan.
func (c *Client) GetVariant(ctx context.Context, id string) (*Variant, error) {
	var v Variant
	if err := c.do(ctx, http.MethodGet, "/variants/"+url.PathEscape(id), nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// InlineVariant is a one-time plan made for one checkout: a gift or an amount
// the buyer chose.
type InlineVariant struct {
	ProductID    string            `json:"product_id"`
	Title        string            `json:"title"`
	BillingType  string            `json:"billing_type"`
	InitialPrice float64           `json:"initial_price"`
	Currency     string            `json:"currency"`
	Visibility   string            `json:"visibility"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// CheckoutParams describe one checkout: a plan by id, or one made inline.
type CheckoutParams struct {
	AccountID   string
	VariantID   string
	Variant     *InlineVariant
	Metadata    map[string]string
	RedirectURL string
}

// Checkout is a checkout made: its id comes back on the payment.
type Checkout struct {
	ID          string `json:"id"`
	PurchaseURL string `json:"purchase_url"`
}

// CreateCheckout makes a checkout and returns where to send the buyer.
func (c *Client) CreateCheckout(ctx context.Context, p CheckoutParams) (*Checkout, error) {
	in := map[string]any{"mode": "payment", "redirect_url": p.RedirectURL}
	if p.AccountID != "" {
		in["account_id"] = p.AccountID
	}
	if p.VariantID != "" {
		in["variant_id"] = p.VariantID
	} else if p.Variant != nil {
		in["variant"] = p.Variant
	}
	if len(p.Metadata) > 0 {
		in["metadata"] = p.Metadata
	}
	var out Checkout
	if err := c.do(ctx, http.MethodPost, "/checkout_configurations", in, &out); err != nil {
		return nil, err
	}
	if out.ID == "" || out.PurchaseURL == "" {
		return nil, errors.New("whop: a checkout with no id or address")
	}
	return &out, nil
}

// Membership is someone's subscription to a plan, as far as Nexa needs it.
type Membership struct {
	ID                string            `json:"id"`
	Status            string            `json:"status"`
	UserID            string            `json:"user_id"`
	VariantID         string            `json:"variant_id"`
	CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
	CurrentPeriodEnd  string            `json:"current_period_end"`
	ManageURL         string            `json:"manage_url"`
	Metadata          map[string]string `json:"metadata"`
}

// PeriodEnd is when the membership renews or, not renewing, ends (ms), or 0.
func (m Membership) PeriodEnd() int64 {
	if t, err := time.Parse(time.RFC3339, m.CurrentPeriodEnd); err == nil {
		return t.UnixMilli()
	}
	if n, err := strconv.ParseInt(m.CurrentPeriodEnd, 10, 64); err == nil && n > 0 {
		return n * 1000
	}
	return 0
}

// GetMembership reads a membership as it is now.
func (c *Client) GetMembership(ctx context.Context, id string) (*Membership, error) {
	var m Membership
	if err := c.do(ctx, http.MethodGet, "/memberships/"+url.PathEscape(id), nil, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// CancelMembership ends a membership at once, or at the end of what is paid.
func (c *Client) CancelMembership(ctx context.Context, id string, atPeriodEnd bool) error {
	return c.do(ctx, http.MethodPost, "/memberships/"+url.PathEscape(id)+"/cancel", map[string]any{"cancel_at_period_end": atPeriodEnd}, nil)
}

// Event is a webhook's event, with its object left raw for the type to read.
type Event struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ErrBadSignature is a webhook whose signature does not check out.
var ErrBadSignature = errors.New("whop: bad webhook signature")

// Tolerance is how far a webhook's signed time may be from now.
const Tolerance = 5 * time.Minute

// keys are the HMAC keys a secret may stand for. Whop's helper hands its
// ws_ secret to a Standard Webhooks verifier base64-encoded, which decodes it
// straight back, so the key is the secret's own bytes; a whsec_ secret, the
// spec's own form, is the base64 after its prefix.
func keys(secret string) [][]byte {
	out := [][]byte{[]byte(secret)}
	if rest, ok := strings.CutPrefix(secret, "whsec_"); ok {
		if k, err := base64.StdEncoding.DecodeString(rest); err == nil {
			out = append(out, k)
		}
	}
	return out
}

// VerifyWebhook checks a Standard Webhooks signature (webhook-id,
// webhook-timestamp, and webhook-signature holding "v1,<base64 HMAC-SHA256 of
// id.timestamp.body>", several separated by spaces) and reads the event.
func VerifyWebhook(body []byte, id, timestamp, signature, secret string, now time.Time) (*Event, error) {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || id == "" || signature == "" || secret == "" {
		return nil, ErrBadSignature
	}
	if d := now.Sub(time.Unix(ts, 0)); d > Tolerance || d < -Tolerance {
		return nil, ErrBadSignature
	}
	ok := false
	for _, key := range keys(secret) {
		want := sign(body, key, id, ts)
		for _, part := range strings.Fields(signature) {
			ver, sig, found := strings.Cut(part, ",")
			if found && ver == "v1" && hmac.Equal([]byte(sig), []byte(want)) {
				ok = true
			}
		}
	}
	if !ok {
		return nil, ErrBadSignature
	}
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func sign(body, key []byte, id string, ts int64) string {
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s.%d.", id, ts)
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Sign is the v1 signature of a body, for tests that play Whop.
func Sign(body []byte, secret, id string, ts int64) string {
	return "v1," + sign(body, []byte(secret), id, ts)
}
