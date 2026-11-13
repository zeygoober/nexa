package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/totp"
)

// enableTOTP walks a registered user through authenticator setup and returns
// the shared secret plus the backup codes the server handed back.
func enableTOTP(t *testing.T, ts *httptest.Server, token string) (string, []string) {
	t.Helper()
	res, body := do(t, ts, "POST", "/api/v1/auth/mfa/totp/setup", token, map[string]any{"auth_key": b64(key32)})
	if res.StatusCode != 200 {
		t.Fatalf("setup: %d %v", res.StatusCode, body)
	}
	secret, _ := body["secret"].(string)
	ticket, _ := body["ticket"].(string)
	if secret == "" || ticket == "" {
		t.Fatalf("setup gave no secret or ticket: %v", body)
	}
	code, err := totp.Code(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/totp/enable", token, map[string]any{"ticket": ticket, "code": code})
	if res.StatusCode != 200 {
		t.Fatalf("enable: %d %v", res.StatusCode, body)
	}
	raw, _ := body["backup_codes"].([]any)
	codes := make([]string, 0, len(raw))
	for _, c := range raw {
		codes = append(codes, c.(string))
	}
	if len(codes) != backupCount {
		t.Fatalf("want %d backup codes, got %d", backupCount, len(codes))
	}
	return secret, codes
}

func TestTOTPSetupNeedsPasswordAndCode(t *testing.T) {
	_, ts := newTestServer(t)
	token, _ := register(t, ts, "alice")

	res, body := do(t, ts, "POST", "/api/v1/auth/mfa/totp/setup", token, map[string]any{"auth_key": b64(key32b)})
	if res.StatusCode != 401 || errCode(body) != "invalid_credentials" {
		t.Fatalf("wrong password should be refused: %d %v", res.StatusCode, body)
	}

	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/totp/setup", token, map[string]any{"auth_key": b64(key32)})
	if res.StatusCode != 200 {
		t.Fatalf("setup: %d %v", res.StatusCode, body)
	}
	secret := body["secret"].(string)
	ticket := body["ticket"].(string)
	if body["uri"].(string) == "" || body["formatted"].(string) == "" {
		t.Errorf("setup should return something scannable and something typeable: %v", body)
	}

	// Nothing is stored until a code confirms it.
	res, body = do(t, ts, "GET", "/api/v1/auth/mfa", token, nil)
	if res.StatusCode != 200 || body["enabled"] != false {
		t.Fatalf("an abandoned setup must not enable anything: %d %v", res.StatusCode, body)
	}

	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/totp/enable", token, map[string]any{"ticket": ticket, "code": "000000"})
	if res.StatusCode != 400 || errCode(body) != "invalid_code" {
		t.Fatalf("a wrong code should not enable: %d %v", res.StatusCode, body)
	}

	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/totp/enable", token, map[string]any{"ticket": ticket[:len(ticket)-2] + "xy", "code": "000000"})
	if res.StatusCode != 400 || errCode(body) != "setup_expired" {
		t.Fatalf("a tampered ticket should be refused: %d %v", res.StatusCode, body)
	}

	code, _ := totp.Code(secret, time.Now())
	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/totp/enable", token, map[string]any{"ticket": ticket, "code": code})
	if res.StatusCode != 200 || body["enabled"] != true {
		t.Fatalf("enable: %d %v", res.StatusCode, body)
	}

	res, body = do(t, ts, "GET", "/api/v1/users/@me", token, nil)
	if res.StatusCode != 200 || body["mfa_enabled"] != true || body["backup_codes_left"].(float64) != float64(backupCount) {
		t.Fatalf("me should report the second factor: %d %v", res.StatusCode, body)
	}
}

// The reply to the enable request is the only copy of the backup codes anyone
// gets. A repeat of that request — a double submit, a retry after a proxy gave
// up — used to hand out a fresh set and quietly void the list the person had
// just written down, leaving them holding codes that no longer work.
func TestEnablingTOTPTwiceKeepsTheFirstBackupCodes(t *testing.T) {
	_, ts := newTestServer(t)
	token, _ := register(t, ts, "alice")

	res, body := do(t, ts, "POST", "/api/v1/auth/mfa/totp/setup", token, map[string]any{"auth_key": b64(key32)})
	if res.StatusCode != 200 {
		t.Fatalf("setup: %d %v", res.StatusCode, body)
	}
	secret, ticket := body["secret"].(string), body["ticket"].(string)

	code, err := totp.Code(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/totp/enable", token, map[string]any{"ticket": ticket, "code": code})
	if res.StatusCode != 200 {
		t.Fatalf("enable: %d %v", res.StatusCode, body)
	}
	first, _ := body["backup_codes"].([]any)
	if len(first) != backupCount {
		t.Fatalf("want %d backup codes, got %d", backupCount, len(first))
	}

	// the same request again, as a retry would send it
	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/totp/enable", token, map[string]any{"ticket": ticket, "code": code})
	if res.StatusCode != 409 || errCode(body) != "already_enabled" {
		t.Fatalf("a repeat must be refused, not served a new set: %d %v", res.StatusCode, body)
	}
	if _, ok := body["backup_codes"]; ok {
		t.Fatalf("a refused repeat must not hand out codes: %v", body)
	}

	// and the set handed out the first time is still the set that counts
	res, body = do(t, ts, "GET", "/api/v1/users/@me", token, nil)
	if res.StatusCode != 200 || body["backup_codes_left"].(float64) != float64(backupCount) {
		t.Fatalf("the first set should be untouched: %d %v", res.StatusCode, body)
	}
}

func TestLoginRequiresSecondFactor(t *testing.T) {
	srv, ts := newTestServer(t)
	token, _ := register(t, ts, "alice")
	secret, backups := enableTOTP(t, ts, token)

	res, body := do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "alice", "auth_key": b64(key32)})
	if res.StatusCode != 200 || body["mfa_required"] != true {
		t.Fatalf("login should stop at the second factor: %d %v", res.StatusCode, body)
	}
	if _, leaked := body["token"]; leaked {
		t.Fatal("the session token must not be issued before the code")
	}
	if _, leaked := body["enc_private_key"]; leaked {
		t.Fatal("the wrapped key must not be handed out before the code")
	}
	ticket := body["ticket"].(string)

	res, body = do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "code": "123456"})
	if res.StatusCode != 401 || errCode(body) != "invalid_code" {
		t.Fatalf("a wrong code should fail: %d %v", res.StatusCode, body)
	}
	srv.limAuthUsr.Reset("mfa:" + userIDOf(t, ts, token))

	// Setting it up spent the current step, so the next one is what an app
	// would be showing by the time anybody signs in. It is inside the skew
	// window, so the server takes it.
	code, _ := totp.CodeAt(secret, totp.Step(time.Now())+1)
	res, body = do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "code": code})
	if res.StatusCode != 200 || body["token"] == nil || body["enc_private_key"] == nil {
		t.Fatalf("the right code should sign in: %d %v", res.StatusCode, body)
	}

	// The same code cannot be used twice, even inside its own thirty seconds.
	res, body = do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "alice", "auth_key": b64(key32)})
	if res.StatusCode != 200 || body["mfa_required"] != true {
		t.Fatalf("second login: %d %v", res.StatusCode, body)
	}
	ticket = body["ticket"].(string)
	res, body = do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "code": code})
	if res.StatusCode != 401 || errCode(body) != "invalid_code" {
		t.Fatalf("a replayed code must be refused: %d %v", res.StatusCode, body)
	}
	srv.limAuthUsr.Reset("mfa:" + userIDOf(t, ts, token))

	// A backup code gets you in once.
	res, body = do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "backup_code": backups[0]})
	if res.StatusCode != 200 || body["token"] == nil {
		t.Fatalf("a backup code should sign in: %d %v", res.StatusCode, body)
	}
	res, body = do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "alice", "auth_key": b64(key32)})
	ticket = body["ticket"].(string)
	res, body = do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "backup_code": backups[0]})
	if res.StatusCode != 401 {
		t.Fatalf("a spent backup code must be refused: %d %v", res.StatusCode, body)
	}
	srv.limAuthUsr.Reset("mfa:" + userIDOf(t, ts, token))

	// Lower case with the dashes stripped is the same code.
	res, body = do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "alice", "auth_key": b64(key32)})
	ticket = body["ticket"].(string)
	messy := ""
	for _, r := range backups[1] {
		if r != '-' {
			messy += string(r | 0x20)
		}
	}
	res, body = do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "backup_code": messy})
	if res.StatusCode != 200 {
		t.Fatalf("a lower-case backup code should still work: %d %v", res.StatusCode, body)
	}

	res, body = do(t, ts, "GET", "/api/v1/auth/mfa", token, nil)
	if res.StatusCode != 200 || body["backup_codes_left"].(float64) != float64(backupCount-2) {
		t.Fatalf("two codes should be spent: %v", body)
	}
}

// A phone that lost the answer to its second step asks again with the same
// code (2026-11-11: a backup code was spent and nobody was signed in). Only
// that ticket gets in again; the spent code stays spent for any other.
func TestLostSecondStepAnswerCanBeAskedAgain(t *testing.T) {
	srv, ts := newTestServer(t)
	token, _ := register(t, ts, "alice")
	_, backups := enableTOTP(t, ts, token)
	login := func() string {
		_, body := do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "alice", "auth_key": b64(key32)})
		return body["ticket"].(string)
	}
	ticket := login()
	for i := range 2 {
		res, body := do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "backup_code": backups[0]})
		if res.StatusCode != 200 || body["token"] == nil {
			t.Fatalf("try %d with the same ticket and code should sign in: %d %v", i+1, res.StatusCode, body)
		}
	}
	// another code on that ticket is not a repeat
	res, body := do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "code": "123456"})
	if res.StatusCode != 401 || errCode(body) != "invalid_code" {
		t.Fatalf("a different code on a used ticket must be refused: %d %v", res.StatusCode, body)
	}
	srv.limAuthUsr.Reset("mfa:" + userIDOf(t, ts, token))
	// the spent code on a new ticket is refused
	res, body = do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": login(), "backup_code": backups[0]})
	if res.StatusCode != 401 || errCode(body) != "invalid_code" {
		t.Fatalf("a spent backup code on another ticket must be refused: %d %v", res.StatusCode, body)
	}
	res, body = do(t, ts, "GET", "/api/v1/auth/mfa", token, nil)
	if body["backup_codes_left"].(float64) != float64(backupCount-1) {
		t.Fatalf("one code should be spent: %v", body)
	}
}

func mustParseID(t *testing.T, s string) ids.ID {
	t.Helper()
	id, err := ids.Parse(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return id
}

func userIDOf(t *testing.T, ts *httptest.Server, token string) string {
	t.Helper()
	_, body := do(t, ts, "GET", "/api/v1/users/@me", token, nil)
	id, _ := body["id"].(string)
	return id
}

func TestMFADisableAndRegenerate(t *testing.T) {
	_, ts := newTestServer(t)
	token, _ := register(t, ts, "alice")
	_, first := enableTOTP(t, ts, token)

	res, body := do(t, ts, "POST", "/api/v1/auth/mfa/backup", token, map[string]any{"auth_key": b64(key32b)})
	if res.StatusCode != 401 {
		t.Fatalf("regenerating needs the password: %d %v", res.StatusCode, body)
	}
	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/backup", token, map[string]any{"auth_key": b64(key32)})
	if res.StatusCode != 200 {
		t.Fatalf("regenerate: %d %v", res.StatusCode, body)
	}
	fresh := body["backup_codes"].([]any)
	if len(fresh) != backupCount || fresh[0].(string) == first[0] {
		t.Fatalf("regenerate should replace the set: %v", body)
	}
	// The old set stops working.
	_, body = do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "alice", "auth_key": b64(key32)})
	ticket := body["ticket"].(string)
	res, body = do(t, ts, "POST", "/api/v1/auth/login/mfa", "", map[string]any{"ticket": ticket, "backup_code": first[0]})
	if res.StatusCode != 401 {
		t.Fatalf("a replaced backup code must be refused: %d %v", res.StatusCode, body)
	}

	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/disable", token, map[string]any{"auth_key": b64(key32b)})
	if res.StatusCode != 401 {
		t.Fatalf("disabling needs the password: %d %v", res.StatusCode, body)
	}
	res, body = do(t, ts, "POST", "/api/v1/auth/mfa/disable", token, map[string]any{"auth_key": b64(key32)})
	if res.StatusCode != 200 || body["enabled"] != false {
		t.Fatalf("disable: %d %v", res.StatusCode, body)
	}
	res, body = do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "alice", "auth_key": b64(key32)})
	if res.StatusCode != 200 || body["token"] == nil {
		t.Fatalf("login should be back to one step: %d %v", res.StatusCode, body)
	}
	res, body = do(t, ts, "GET", "/api/v1/auth/mfa", token, nil)
	if body["backup_codes_left"].(float64) != 0 {
		t.Fatalf("disabling should drop the backup codes: %v", body)
	}
}

func TestSealedSecretAndTicketsAreOpaque(t *testing.T) {
	srv, ts := newTestServer(t)
	token, id := register(t, ts, "alice")
	secret, _ := enableTOTP(t, ts, token)

	uid := mustParseID(t, id)
	m, err := srv.Store.MFAFor(t.Context(), uid)
	if err != nil || m == nil {
		t.Fatalf("stored row: %v %v", m, err)
	}
	if string(m.Secret) == secret {
		t.Fatal("the shared secret is stored in the clear")
	}
	got, err := srv.openSecret(m.Secret)
	if err != nil || got != secret {
		t.Fatalf("unseal: %q %v", got, err)
	}

	// A ticket minted for one purpose is not accepted for another.
	tk := srv.signTicket(ticket{Purpose: "totp-setup", User: id, Exp: time.Now().Add(time.Minute).Unix(), Secret: secret})
	if _, ok := srv.readTicket(tk, "login-mfa"); ok {
		t.Error("a setup ticket must not pass as a login ticket")
	}
	expired := srv.signTicket(ticket{Purpose: "login-mfa", User: id, Exp: time.Now().Add(-time.Second).Unix()})
	if _, ok := srv.readTicket(expired, "login-mfa"); ok {
		t.Error("an expired ticket must not pass")
	}
}

func TestRecoveryKeyClearsSecondFactor(t *testing.T) {
	_, ts := newTestServer(t)
	token, _ := register(t, ts, "alice")
	enableTOTP(t, ts, token)

	_, begin := do(t, ts, "POST", "/api/v1/auth/recover/begin", "", map[string]any{"username": "alice", "recovery_auth_key": b64(key32b)})
	res, body := do(t, ts, "POST", "/api/v1/auth/recover/finish", "", map[string]any{
		"username": "alice", "recovery_auth_key": b64(key32b), "ticket": begin["ticket"],
		"new_auth_key": b64(key32), "new_kdf_salt": b64(salt16),
		"new_kdf_params":      map[string]int{"m": 65536, "t": 3, "p": 1, "v": 1},
		"new_enc_private_key": b64(wrapped),
	})
	if res.StatusCode != 200 || body["token"] == nil {
		t.Fatalf("recover: %d %v", res.StatusCode, body)
	}
	user := body["user"].(map[string]any)
	if user["mfa_enabled"] != false {
		t.Fatalf("recovery should clear the authenticator: %v", user)
	}
	res, body = do(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"username": "alice", "auth_key": b64(key32)})
	if res.StatusCode != 200 || body["token"] == nil {
		t.Fatalf("login after recovery should be one step: %d %v", res.StatusCode, body)
	}
}
