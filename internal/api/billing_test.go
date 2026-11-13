package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"evilion.xyz/chat/internal/whop"
)

const testWebhookSecret = "ws_test"

// fakeWhop plays the parts of Whop's API billing.go calls.
type fakeWhop struct {
	mu        sync.Mutex
	mems      map[string]map[string]any
	checkouts []map[string]any
	canceled  []string
}

func newFakeWhop(t *testing.T) (*fakeWhop, *httptest.Server) {
	f := &fakeWhop{mems: map[string]map[string]any{}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if v := r.Header.Get("Api-Version-Date"); v != whop.APIVersionDate {
			t.Errorf("Api-Version-Date %q", v)
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/variants/"):
			id := strings.TrimPrefix(r.URL.Path, "/variants/")
			price := map[string]float64{"plan_sup_month": 5, "plan_sup_year": 48, "plan_pro_month": 10, "plan_pro_year": 96}[id]
			json.NewEncoder(w).Encode(map[string]any{"id": id, "billing_type": "renewal", "initial_price": price, "renewal_price": price, "currency": "usd"})
		case r.Method == "POST" && r.URL.Path == "/checkout_configurations":
			f.checkouts = append(f.checkouts, in)
			id := fmt.Sprintf("ch_%d", len(f.checkouts))
			json.NewEncoder(w).Encode(map[string]any{"id": id, "purchase_url": "https://whop.test/checkout/" + id})
		case strings.HasPrefix(r.URL.Path, "/memberships/"):
			id, cancel := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/memberships/"), "/cancel")
			m, ok := f.mems[id]
			if !ok {
				w.WriteHeader(404)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "not_found", "message": "no such membership"}})
				return
			}
			if cancel && r.Method == "POST" {
				f.canceled = append(f.canceled, id)
				m["status"] = "canceled"
			}
			json.NewEncoder(w).Encode(m)
		default:
			t.Errorf("unexpected Whop call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(ts.Close)
	return f, ts
}

func (f *fakeWhop) setMem(id, variant, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mems[id] = map[string]any{
		"id": id, "user_id": "user_" + id, "variant_id": variant, "status": status, "cancel_at_period_end": false,
		"current_period_end": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
		"manage_url":         "https://whop.test/manage/" + id,
	}
}

func (f *fakeWhop) last() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkouts[len(f.checkouts)-1]
}

func (f *fakeWhop) lastID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fmt.Sprintf("ch_%d", len(f.checkouts))
}

// sendWebhook posts a signed event the way Whop does.
func sendWebhook(t *testing.T, ts *httptest.Server, typ string, data map[string]any, secret string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"id": "msg_1", "type": typ, "data": data})
	now := time.Now().Unix()
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/billing/webhook", bytes.NewReader(body))
	req.Header.Set("webhook-id", "msg_1")
	req.Header.Set("webhook-timestamp", fmt.Sprint(now))
	req.Header.Set("webhook-signature", whop.Sign(body, secret, "msg_1", now))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// paid sends the payment.succeeded of a checkout.
func paid(t *testing.T, ts *httptest.Server, payID, checkoutID, membershipID string) {
	t.Helper()
	data := map[string]any{"id": payID, "status": "paid", "checkout_configuration_id": checkoutID, "membership_id": membershipID}
	if code := sendWebhook(t, ts, "payment.succeeded", data, testWebhookSecret); code != 200 {
		t.Fatalf("payment webhook: %d", code)
	}
}

func billingServer(t *testing.T) (*Server, *httptest.Server, *fakeWhop) {
	srv, ts := newTestServer(t)
	f, fs := newFakeWhop(t)
	c := &srv.Cfg
	c.WhopAPIKey, c.WhopWebhookSecret, c.WhopAPIURL, c.WhopProductID = "apik_test", testWebhookSecret, fs.URL, "prod_nexa"
	c.WhopSupporterMonthly, c.WhopSupporterYearly, c.WhopProMonthly, c.WhopProYearly = "plan_sup_month", "plan_sup_year", "plan_pro_month", "plan_pro_year"
	return srv, ts, f
}

// subscribe buys a plan through a checkout and its payment.
func subscribe(t *testing.T, ts *httptest.Server, f *fakeWhop, tok, plan, period, mem, variant string) {
	t.Helper()
	if res, body := do(t, ts, "POST", "/api/v1/billing/checkout", tok, map[string]any{"plan": plan, "period": period}); res.StatusCode != 200 {
		t.Fatalf("checkout: %d %v", res.StatusCode, body)
	}
	f.setMem(mem, variant, "active")
	paid(t, ts, "pay_"+mem, f.lastID(), mem)
}

func TestBillingCheckoutWebhookAndPlan(t *testing.T) {
	srv, ts, f := billingServer(t)
	tok, uid := register(t, ts, "payer")

	res, body := do(t, ts, "POST", "/api/v1/billing/checkout", tok, map[string]any{"plan": "pro", "period": "year"})
	if res.StatusCode != 200 || body["url"] != "https://whop.test/checkout/ch_1" {
		t.Fatalf("checkout: %d %v", res.StatusCode, body)
	}
	got := f.last()
	if got["variant_id"] != "plan_pro_year" || got["mode"] != "payment" || got["metadata"].(map[string]any)["user_id"] != uid {
		t.Fatalf("checkout body: %v", got)
	}

	// a forged event changes nothing
	f.setMem("mem_1", "plan_pro_year", "active")
	if code := sendWebhook(t, ts, "payment.succeeded", map[string]any{"id": "pay_1", "checkout_configuration_id": "ch_1", "membership_id": "mem_1"}, "ws_wrong"); code != 400 {
		t.Fatalf("forged webhook: %d", code)
	}
	// a membership alone names only Whop's user, so it goes on nobody
	if code := sendWebhook(t, ts, "membership.activated", map[string]any{"id": "mem_1"}, testWebhookSecret); code != 200 {
		t.Fatalf("membership webhook: %d", code)
	}
	// a payment for a checkout this server did not make buys nothing
	f.setMem("mem_x", "plan_pro_year", "active")
	paid(t, ts, "pay_x", "ch_someone_elses", "mem_x")
	if u, _ := srv.Store.UserByID(t.Context(), modID(uid)); u.Plan != "" {
		t.Fatalf("plan before the payment: %q", u.Plan)
	}

	paid(t, ts, "pay_1", "ch_1", "mem_1")
	_, me := do(t, ts, "GET", "/api/v1/users/@me", tok, nil)
	if me["plan"] != "pro" {
		t.Fatalf("plan: %v", me["plan"])
	}
	_, bill := do(t, ts, "GET", "/api/v1/billing", tok, nil)
	if bill["plan"] != "pro" || bill["status"] != "active" || bill["manageable"] != true || bill["period"] != "year" {
		t.Fatalf("billing: %v", bill)
	}
	if p := bill["prices"].(map[string]any)["pro"].(map[string]any)["year"].(map[string]any); p["amount"].(float64) != 9600 || p["currency"] != "usd" {
		t.Fatalf("prices: %v", bill["prices"])
	}
	if lim := bill["limits"].(map[string]any); lim["boosts"].(float64) != 2 || lim["file_max"].(float64) != float64(4<<30) {
		t.Fatalf("limits: %v", lim)
	}

	// buying again goes through the membership's own page
	if res, body := do(t, ts, "POST", "/api/v1/billing/checkout", tok, map[string]any{"plan": "supporter", "period": "month"}); res.StatusCode != 409 || errCode(body) != "already_subscribed" {
		t.Fatalf("second checkout: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "POST", "/api/v1/billing/portal", tok, nil); res.StatusCode != 200 || body["url"] != "https://whop.test/manage/mem_1" {
		t.Fatalf("portal: %d %v", res.StatusCode, body)
	}

	// boosts: two, on servers the booster is in
	s1 := createServer(t, ts, tok, "one")["id"].(string)
	s2 := createServer(t, ts, tok, "two")["id"].(string)
	s3 := createServer(t, ts, tok, "three")["id"].(string)
	for _, sid := range []string{s1, s2} {
		if res, body := do(t, ts, "POST", "/api/v1/billing/boosts", tok, map[string]any{"server_id": sid}); res.StatusCode != 204 {
			t.Fatalf("boost: %d %v", res.StatusCode, body)
		}
	}
	if res, body := do(t, ts, "POST", "/api/v1/billing/boosts", tok, map[string]any{"server_id": s3}); res.StatusCode != 409 || errCode(body) != "no_boosts_left" {
		t.Fatalf("third boost: %d %v", res.StatusCode, body)
	}
	if srv1, _ := srv.Store.ServerByID(t.Context(), modID(s1)); srv1.Boosts != 1 {
		t.Fatalf("boosts on the server: %d", srv1.Boosts)
	}

	// the membership ends: back to free, boosts gone
	f.setMem("mem_1", "plan_pro_year", "expired")
	if code := sendWebhook(t, ts, "membership.deactivated", map[string]any{"id": "mem_1"}, testWebhookSecret); code != 200 {
		t.Fatalf("webhook: %d", code)
	}
	if u, _ := srv.Store.UserByID(t.Context(), modID(uid)); u.Plan != "" {
		t.Fatalf("plan after the end: %q", u.Plan)
	}
	if srv1, _ := srv.Store.ServerByID(t.Context(), modID(s1)); srv1.Boosts != 0 {
		t.Fatalf("boosts after the end: %d", srv1.Boosts)
	}
	if res, body := do(t, ts, "POST", "/api/v1/billing/boosts", tok, map[string]any{"server_id": s1}); res.StatusCode != 403 || errCode(body) != "plan_required" {
		t.Fatalf("free boost: %d %v", res.StatusCode, body)
	}
}

func TestBillingOlderSubscriptionEndingKeepsNewer(t *testing.T) {
	srv, ts, f := billingServer(t)
	tok, uid := register(t, ts, "twice")
	subscribe(t, ts, f, tok, "pro", "month", "mem_old", "plan_pro_month")
	f.setMem("mem_old", "plan_pro_month", "past_due")
	sendWebhook(t, ts, "membership.updated", map[string]any{"id": "mem_old"}, testWebhookSecret)
	// a second plan, bought while the first was still on
	if _, err := srv.Store.DB().W.Exec(`UPDATE users SET plan = '' WHERE id = ?`, int64(modID(uid))); err != nil {
		t.Fatal(err)
	}
	subscribe(t, ts, f, tok, "supporter", "month", "mem_new", "plan_sup_month")
	f.setMem("mem_old", "plan_pro_month", "canceled")
	if _, err := srv.Store.DB().W.Exec(`UPDATE billing SET subscription_id = 'mem_new'`); err != nil {
		t.Fatal(err)
	}
	paid(t, ts, "pay_late", "ch_none", "mem_old") // the old one's last word
	if u, _ := srv.Store.UserByID(t.Context(), modID(uid)); u.Plan != "supporter" {
		t.Fatalf("plan: %q", u.Plan)
	}
}

func TestPlanCaps(t *testing.T) {
	srv, ts, f := billingServer(t)
	tok, uid := register(t, ts, "capped")
	big := int64(1500) << 20
	if res, body := do(t, ts, "POST", "/api/v1/uploads", tok, map[string]any{"size": big}); res.StatusCode != 413 || errCode(body) != "too_large" {
		t.Fatalf("free 1.5 GB: %d %v", res.StatusCode, body)
	}
	// a boosted server's channel takes it
	sid := createServer(t, ts, tok, "boosted")["id"].(string)
	if _, err := srv.Store.DB().W.Exec(`INSERT INTO server_boosts (id, user_id, server_id, created_at) VALUES (1, ?, ?, 0)`, int64(modID(uid)), int64(modID(sid))); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.DB().W.Exec(`UPDATE servers SET boosts = 1 WHERE id = ?`, int64(modID(sid))); err != nil {
		t.Fatal(err)
	}
	_, chans := do(t, ts, "GET", "/api/v1/servers/"+sid+"/channels", tok, nil)
	var chID string
	for _, c := range chans["_list"].([]any) {
		if c.(map[string]any)["type"] == "text" {
			chID = c.(map[string]any)["id"].(string)
		}
	}
	if res, body := do(t, ts, "POST", "/api/v1/uploads", tok, map[string]any{"size": big, "channel_id": chID}); res.StatusCode != 201 {
		t.Fatalf("boosted 1.5 GB: %d %v", res.StatusCode, body)
	}

	// a paid name effect is for plans
	style := map[string]any{"name_style": map[string]any{"effect": "neon", "colors": []int{0xff00aa}}}
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, style); res.StatusCode != 403 || errCode(body) != "plan_required" {
		t.Fatalf("free neon: %d %v", res.StatusCode, body)
	}
	subscribe(t, ts, f, tok, "supporter", "month", "mem_c", "plan_sup_month")
	if res, body := do(t, ts, "PATCH", "/api/v1/users/@me", tok, style); res.StatusCode != 200 {
		t.Fatalf("supporter neon: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "POST", "/api/v1/uploads", tok, map[string]any{"size": big}); res.StatusCode != 201 {
		t.Fatalf("supporter 1.5 GB: %d %v", res.StatusCode, body)
	}
}

func TestMonthlyUploads(t *testing.T) {
	srv, ts := newTestServer(t)
	tok, uid := register(t, ts, "hoarder")
	now := time.Now().UnixMilli()
	// 5 GB put up today fills the free plan's 30 days
	if _, err := srv.Store.DB().W.Exec(`INSERT INTO blobs (id, sha256, size, mime, kind, encrypted, ref_count, owner_id, created_at) VALUES (99, 'x', ?, 'application/octet-stream', 'attachment', 1, 1, ?, ?)`, int64(5)<<30, int64(modID(uid)), now); err != nil {
		t.Fatal(err)
	}
	if res, body := do(t, ts, "POST", "/api/v1/uploads", tok, map[string]any{"size": 1 << 20}); res.StatusCode != 507 || errCode(body) != "upload_month_full" {
		t.Fatalf("over the month: %d %v", res.StatusCode, body)
	}
	_, bill := do(t, ts, "GET", "/api/v1/billing", tok, nil)
	if bill["month_used"].(float64) != float64(int64(5)<<30) || int64(bill["month_frees_at"].(float64)) != now+(30*24*time.Hour).Milliseconds() {
		t.Fatalf("billing: %v", bill)
	}
	// once it is 31 days old it no longer counts
	if _, err := srv.Store.DB().W.Exec(`UPDATE blobs SET created_at = ? WHERE id = 99`, now-(31*24*time.Hour).Milliseconds()); err != nil {
		t.Fatal(err)
	}
	if res, body := do(t, ts, "POST", "/api/v1/uploads", tok, map[string]any{"size": 1 << 20}); res.StatusCode != 201 {
		t.Fatalf("after 30 days: %d %v", res.StatusCode, body)
	}
}

func TestGiftAPlan(t *testing.T) {
	srv, ts, f := billingServer(t)
	tokA, idA := register(t, ts, "giver")
	_, idB := register(t, ts, "lucky")
	if res, body := do(t, ts, "POST", "/api/v1/billing/gift", tokA, map[string]any{"to": idA, "plan": "supporter", "period": "month"}); res.StatusCode != 400 || errCode(body) != "gift_self" {
		t.Fatalf("a gift to yourself: %d %v", res.StatusCode, body)
	}
	gift := func() string {
		t.Helper()
		if res, body := do(t, ts, "POST", "/api/v1/billing/gift", tokA, map[string]any{"to": idB, "plan": "supporter", "period": "month"}); res.StatusCode != 200 {
			t.Fatalf("gift checkout: %d %v", res.StatusCode, body)
		}
		return f.lastID()
	}
	first := gift()
	v, _ := f.last()["variant"].(map[string]any)
	if v["initial_price"] != 5.0 || v["billing_type"] != "one_time" || v["product_id"] != "prod_nexa" || f.last()["metadata"].(map[string]any)["gift_to"] != idB {
		t.Fatalf("gift checkout body: %v", f.last())
	}
	second := gift()
	// a payment not yet through gives nothing
	if code := sendWebhook(t, ts, "payment.succeeded", map[string]any{"id": "pay_open", "status": "open", "checkout_configuration_id": first}, testWebhookSecret); code != 200 {
		t.Fatalf("unpaid: %d", code)
	}
	paid(t, ts, "pay_1", first, "")
	paid(t, ts, "pay_1", first, "")  // Whop sending it twice gives it once
	paid(t, ts, "pay_2", second, "") // a second month starts when the first ends
	if u, _ := srv.Store.UserByID(t.Context(), modID(idB)); u.Plan != "supporter" {
		t.Fatalf("plan: %q", u.Plan)
	}
	gifts, _ := srv.Store.GiftsFor(t.Context(), modID(idB))
	if len(gifts) != 2 || gifts[1].StartsAt != gifts[0].EndsAt || gifts[0].FromName == "" {
		t.Fatalf("gifts: %+v", gifts)
	}
	// once both have ended, the plan goes with them
	if _, err := srv.Store.DB().W.Exec(`UPDATE plan_gifts SET starts_at = 0, ends_at = ?`, time.Now().Add(-time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if lapsed, err := srv.Store.RecomputePlans(t.Context(), 0); err != nil || len(lapsed) != 1 {
		t.Fatalf("recompute: %v %v", lapsed, err)
	}
	if u, _ := srv.Store.UserByID(t.Context(), modID(idB)); u.Plan != "" {
		t.Fatalf("plan after the gift: %q", u.Plan)
	}
}

func TestGiftLink(t *testing.T) {
	srv, ts, f := billingServer(t)
	tokA, _ := register(t, ts, "linkgiver")
	tokB, idB := register(t, ts, "claimer")
	tokC, _ := register(t, ts, "toolate")
	if res, body := do(t, ts, "POST", "/api/v1/billing/gift", tokA, map[string]any{"link": true, "plan": "pro", "period": "year"}); res.StatusCode != 200 {
		t.Fatalf("link checkout: %d %v", res.StatusCode, body)
	}
	meta := f.last()["metadata"].(map[string]any)
	if meta["gift_link"] != "1" || meta["gift_to"] != nil || f.last()["variant"].(map[string]any)["initial_price"] != 96.0 {
		t.Fatalf("link checkout body: %v", f.last())
	}
	for range 2 { // sent twice, made once
		paid(t, ts, "pay_link", f.lastID(), "")
	}
	_, bill := do(t, ts, "GET", "/api/v1/billing", tokA, nil)
	links, _ := bill["gift_links"].([]any)
	if len(links) != 1 {
		t.Fatalf("gift links: %v", bill["gift_links"])
	}
	code := links[0].(map[string]any)["code"].(string)
	if res, body := do(t, ts, "POST", "/api/v1/gifts/"+code+"/claim", tokA, nil); res.StatusCode != 400 || errCode(body) != "gift_self" {
		t.Fatalf("claiming your own: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "GET", "/api/v1/gifts/"+code, tokB, nil); res.StatusCode != 200 || body["plan"] != "pro" || body["claimed"] != false || body["days"].(float64) != 365 {
		t.Fatalf("preview: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "POST", "/api/v1/gifts/"+code+"/claim", tokB, nil); res.StatusCode != 200 || body["claimed_by_me"] != true {
		t.Fatalf("claim: %d %v", res.StatusCode, body)
	}
	if u, _ := srv.Store.UserByID(t.Context(), modID(idB)); u.Plan != "pro" {
		t.Fatalf("plan: %q", u.Plan)
	}
	if res, body := do(t, ts, "POST", "/api/v1/gifts/"+code+"/claim", tokC, nil); res.StatusCode != 410 || errCode(body) != "gift_claimed" {
		t.Fatalf("second claim: %d %v", res.StatusCode, body)
	}
	if res, body := do(t, ts, "GET", "/api/v1/gifts/nope", tokC, nil); res.StatusCode != 404 || errCode(body) != "unknown_gift" {
		t.Fatalf("unknown: %d %v", res.StatusCode, body)
	}
	_, bill = do(t, ts, "GET", "/api/v1/billing", tokA, nil)
	if l := bill["gift_links"].([]any)[0].(map[string]any); l["claimed"] != true || l["claimed_name"] == "" || l["claimed_by"] != idB {
		t.Fatalf("buyer's view: %v", l)
	}
}

// A big MAX_UPLOAD_BYTES (the live server's is 15 GB) sizes webhook, app and
// note files only: it once lifted every plan to the same size.
func TestPlansKeepTheirOwnFileSizes(t *testing.T) {
	srv, ts := newTestServer(t)
	srv.Cfg.MaxUploadBytes = 15 << 30
	_, info := do(t, ts, "GET", "/api/v1/instance", "", nil)
	plans := info["plans"].(map[string]any)
	for plan, want := range map[string]float64{"free": 1 << 30, "supporter": 2 << 30, "pro": 4 << 30} {
		if got := plans[plan].(map[string]any)["file_max"].(float64); got != want {
			t.Errorf("%s file_max %v, want %v", plan, got, want)
		}
	}
	if got := plans["boost_file_max"].(float64); got != 2<<30 {
		t.Errorf("boost_file_max %v", got)
	}
	if got := info["max_upload_bytes"].(float64); got != 1<<30 {
		t.Errorf("max_upload_bytes %v, want the free plan's", got)
	}
}

// Pay what you want: the split, the bounds, the checkout Whop is asked for,
// and the time going on once, Pro first and Supporter after it.
func TestPayWhatYouWant(t *testing.T) {
	for _, c := range []struct{ amount, pro, sup int64 }{{100, 0, 0}, {499, 0, 0}, {500, 0, 1}, {999, 0, 1}, {1000, 1, 0}, {1500, 1, 1}, {2499, 2, 0}, {9600, 9, 1}} {
		if pro, sup := pwywSplit(c.amount, 500, 1000); pro != c.pro || sup != c.sup {
			t.Errorf("%d cents: %d Pro, %d Supporter", c.amount, pro, sup)
		}
	}
	srv, ts, f := billingServer(t)
	tok, id := register(t, ts, "tipper")
	for _, amount := range []int{99, 100_000_000} {
		if res, body := do(t, ts, "POST", "/api/v1/billing/pay-what-you-want", tok, map[string]any{"amount": amount}); res.StatusCode != 400 || errCode(body) != "invalid_amount" {
			t.Fatalf("%d: %d %v", amount, res.StatusCode, body)
		}
	}
	if res, body := do(t, ts, "POST", "/api/v1/billing/pay-what-you-want", tok, map[string]any{"amount": 2550}); res.StatusCode != 200 || body["pro_months"].(float64) != 2 {
		t.Fatalf("checkout: %d %v", res.StatusCode, body)
	}
	got := f.last()
	if v := got["variant"].(map[string]any); v["initial_price"] != 25.5 || v["billing_type"] != "one_time" || got["metadata"].(map[string]any)["pro_days"] != "60" {
		t.Fatalf("checkout body: %v", got)
	}
	for range 2 { // Whop sending it twice gives it once
		paid(t, ts, "pay_pwyw", f.lastID(), "")
	}
	if u, _ := srv.Store.UserByID(t.Context(), modID(id)); u.Plan != "pro" {
		t.Fatalf("plan: %q", u.Plan)
	}
	gifts, _ := srv.Store.GiftsFor(t.Context(), modID(id))
	if len(gifts) != 2 || gifts[0].Plan != "pro" || gifts[1].Plan != "supporter" || gifts[1].StartsAt != gifts[0].EndsAt || gifts[0].EndsAt-gifts[0].StartsAt != 60*24*3600*1000 {
		t.Fatalf("gifts: %+v", gifts)
	}
}
