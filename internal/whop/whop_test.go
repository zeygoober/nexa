package whop

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

// A webhook is what puts accounts on plans, so only Whop's signature, fresh,
// may pass.
func TestVerifyWebhook(t *testing.T) {
	body := []byte(`{"id":"msg_1","type":"payment.succeeded","data":{"id":"pay_1"}}`)
	now := time.Unix(1_800_000_000, 0)
	ts := fmt.Sprint(now.Unix())
	sig := Sign(body, "ws_secret", "msg_1", now.Unix())

	if ev, err := VerifyWebhook(body, "msg_1", ts, "v0,zzz "+sig, "ws_secret", now); err != nil || ev.Type != "payment.succeeded" {
		t.Fatalf("good signature: %v %v", ev, err)
	}
	for name, c := range map[string]struct{ id, ts, secret string }{
		"wrong secret":  {"msg_1", ts, "ws_other"},
		"another id":    {"msg_2", ts, "ws_secret"},
		"stale":         {"msg_1", fmt.Sprint(now.Add(-6 * time.Minute).Unix()), "ws_secret"},
		"no secret set": {"msg_1", ts, ""},
		"bad timestamp": {"msg_1", "soon", "ws_secret"},
	} {
		if _, err := VerifyWebhook(body, c.id, c.ts, sig, c.secret, now); err != ErrBadSignature {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := VerifyWebhook(append(body, ' '), "msg_1", ts, sig, "ws_secret", now); err != ErrBadSignature {
		t.Errorf("changed body: %v", err)
	}

	// the spec's own whsec_ form keys with its decoded bytes
	key := []byte("0123456789abcdef")
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "msg_1.%s.", ts)
	mac.Write(body)
	spec := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if _, err := VerifyWebhook(body, "msg_1", ts, spec, "whsec_"+base64.StdEncoding.EncodeToString(key), now); err != nil {
		t.Errorf("whsec_ secret: %v", err)
	}
}
