package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
	"evilion.xyz/chat/internal/totp"
)

const (
	// How long the client has to finish each step.
	mfaLoginTTL = 5 * time.Minute
	mfaSetupTTL = 15 * time.Minute
	backupCount = 10
	backupChars = "23456789ABCDEFGHJKMNPQRSTUVWXYZ" // no 0/O/1/I/L, which people mistype
	backupLen   = 12
)

func (s *Server) mfaRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/auth/login/mfa", ipLimited(s.limAuthIP, s.handleLoginMFA))
	mux.Handle("GET /api/v1/auth/mfa", s.authed(s.handleMFAStatus))
	mux.Handle("POST /api/v1/auth/mfa/totp/setup", s.authed(s.handleTOTPSetup))
	mux.Handle("POST /api/v1/auth/mfa/totp/enable", s.authed(s.handleTOTPEnable))
	mux.Handle("POST /api/v1/auth/mfa/disable", s.authed(s.handleMFADisable))
	mux.Handle("POST /api/v1/auth/mfa/backup", s.authed(s.handleBackupCodes))
}

// derivedKey is a purpose-separated key off the server secret, so the same
// secret can seal TOTP seeds, sign tickets and hash backup codes without any
// one of them weakening another.
func (s *Server) derivedKey(purpose string) []byte {
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(purpose))
	return mac.Sum(nil)
}

// sealSecret encrypts a TOTP seed for storage. Only this box can open it.
func (s *Server) sealSecret(plain string) ([]byte, error) {
	block, err := aes.NewCipher(s.derivedKey("mfa-seal-v1"))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plain), nil), nil
}

func (s *Server) openSecret(sealed []byte) (string, error) {
	block, err := aes.NewCipher(s.derivedKey("mfa-seal-v1"))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("sealed secret is truncated")
	}
	out, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ticket is a short-lived, signed note to self: it carries a half-finished
// login or setup between two requests without any server-side state.
type ticket struct {
	Purpose string `json:"p"`
	User    string `json:"u"`
	Exp     int64  `json:"e"`
	Secret  string `json:"s,omitempty"`
	// Nonce makes a sign-in's ticket its own: without it two in the same
	// second were the same ticket, and a repeat (mfaReplays) has to name one
	Nonce string `json:"n,omitempty"`
}

func (s *Server) signTicket(t ticket) string {
	body, _ := json.Marshal(t)
	enc := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, s.derivedKey("mfa-ticket-v1"))
	mac.Write([]byte(enc))
	return enc + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) readTicket(raw, purpose string) (ticket, bool) {
	var t ticket
	enc, sig, ok := strings.Cut(raw, ".")
	if !ok || enc == "" {
		return t, false
	}
	mac := hmac.New(sha256.New, s.derivedKey("mfa-ticket-v1"))
	mac.Write([]byte(enc))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return t, false
	}
	body, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil || json.Unmarshal(body, &t) != nil {
		return ticket{}, false
	}
	if t.Purpose != purpose || t.Exp < time.Now().Unix() {
		return ticket{}, false
	}
	return t, true
}

// newBackupCodes returns readable single-use codes and their stored hashes.
func (s *Server) newBackupCodes(user ids.ID) ([]string, []string, error) {
	codes := make([]string, 0, backupCount)
	hashes := make([]string, 0, backupCount)
	buf := make([]byte, backupLen)
	for i := 0; i < backupCount; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, err
		}
		var b strings.Builder
		for j, v := range buf {
			if j > 0 && j%4 == 0 {
				b.WriteByte('-')
			}
			// len(backupChars) is 31, a prime, so the modulo bias over 256 is
			// negligible and every code still has ~59 bits behind it.
			b.WriteByte(backupChars[int(v)%len(backupChars)])
		}
		code := b.String()
		codes = append(codes, code)
		hashes = append(hashes, s.hashBackupCode(user, code))
	}
	return codes, hashes, nil
}

func normalizeBackupCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "", "\t", "").Replace(strings.TrimSpace(code)))
}

// hashBackupCode is keyed by the server secret, so the stored digests are
// useless to anyone who only has the database file.
func (s *Server) hashBackupCode(user ids.ID, code string) string {
	mac := hmac.New(sha256.New, s.derivedKey("mfa-backup-v1"))
	mac.Write([]byte(user.String()))
	mac.Write([]byte{0})
	mac.Write([]byte(normalizeBackupCode(code)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) issuer() string {
	host := strings.TrimPrefix(strings.TrimPrefix(s.Cfg.PublicURL, "https://"), "http://")
	if i := strings.IndexAny(host, "/:"); i > 0 {
		host = host[:i]
	}
	if host == "" {
		host = "evilion chat"
	}
	return host
}

// requirePassword re-checks the account password before a change that needs
// it — the second factor, the password itself, deleting the account — so a
// borrowed session cannot lock the owner out. wrong is what a wrong password
// is told. The checks are metered per account: each is an Argon2 hash.
func (s *Server) requirePassword(w http.ResponseWriter, u *store.User, authKey []byte, wrong string) error {
	if ok, retry := s.limPass.Allow("pw:" + u.ID.String()); !ok {
		w.Header().Set("Retry-After", retryAfter(retry))
		return ErrRateLimited
	}
	ok, err := auth.VerifyAuthKey(authKey, u.AuthHash)
	if err != nil || !ok {
		return E(http.StatusUnauthorized, "invalid_credentials", wrong)
	}
	return nil
}

func (s *Server) handleMFAStatus(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	m, err := s.Store.MFAFor(r.Context(), u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	// A failed read used to report zero, and someone told they have no backup
	// codes left makes new ones — which throws away the ten they are holding.
	left, err := s.Store.BackupCodesLeft(r.Context(), u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	enabled := m != nil && m.Enabled
	resp := map[string]any{"enabled": enabled, "backup_codes_left": left}
	if enabled {
		resp["enabled_at"] = m.CreatedAt
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		AuthKey []byte `json:"auth_key"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if !s.requireCaptcha(w, r, "mfa_setup") {
		return
	}
	if err := s.requirePassword(w, u, req.AuthKey, "Your password is wrong."); err != nil {
		WriteError(w, err)
		return
	}
	secret, err := totp.NewSecret()
	if err != nil {
		WriteError(w, err)
		return
	}
	// Nothing is written until a code proves the app has the same secret, so an
	// abandoned setup leaves no trace.
	tk := s.signTicket(ticket{Purpose: "totp-setup", User: u.ID.String(), Exp: time.Now().Add(mfaSetupTTL).Unix(), Secret: secret})
	WriteJSON(w, http.StatusOK, map[string]any{
		"secret":    secret,
		"formatted": totp.Pretty(secret),
		"uri":       totp.URI(s.issuer(), u.Username, secret),
		"ticket":    tk,
	})
}

func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	tk, ok := s.readTicket(req.Ticket, "totp-setup")
	if !ok || tk.User != u.ID.String() || tk.Secret == "" {
		WriteError(w, E(http.StatusBadRequest, "setup_expired", "That setup timed out. Start again to get a fresh code."))
		return
	}
	if ok, retry := s.limAuthUsr.Allow("mfa:" + u.ID.String()); !ok {
		w.Header().Set("Retry-After", retryAfter(retry))
		WriteError(w, ErrRateLimited)
		return
	}
	step, valid := totp.Verify(tk.Secret, req.Code, time.Now(), totp.CodeSkew)
	if !valid {
		WriteError(w, E(http.StatusBadRequest, "invalid_code", "That code is wrong. Check your authenticator and try the next one."))
		return
	}
	sealed, err := s.sealSecret(tk.Secret)
	if err != nil {
		WriteError(w, err)
		return
	}
	codes, hashes, err := s.newBackupCodes(u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	// One change or none, and only once: this response is the only copy of the
	// backup codes, so a repeat of the request must not replace the list the
	// person has already written down.
	if err := s.Store.FinishTOTPSetup(r.Context(), u.ID, sealed, step, hashes); err != nil {
		if errors.Is(err, store.ErrMFAAlreadyOn) {
			WriteError(w, E(http.StatusConflict, "already_enabled", "Two-factor is already on. Turn it off first, or make new backup codes from Security."))
			return
		}
		WriteError(w, err)
		return
	}
	s.limAuthUsr.Reset("mfa:" + u.ID.String())
	WriteJSON(w, http.StatusOK, map[string]any{"enabled": true, "backup_codes": codes})
}

func (s *Server) handleMFADisable(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		AuthKey []byte `json:"auth_key"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	// turning two-factor off is what a stolen session would want most
	if !s.requireCaptcha(w, r, "mfa_disable") {
		return
	}
	if err := s.requirePassword(w, u, req.AuthKey, "Your password is wrong."); err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.DisableMFA(r.Context(), u.ID); err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

func (s *Server) handleBackupCodes(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		AuthKey []byte `json:"auth_key"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if !s.requireCaptcha(w, r, "mfa_backup") {
		return
	}
	if err := s.requirePassword(w, u, req.AuthKey, "Your password is wrong."); err != nil {
		WriteError(w, err)
		return
	}
	on, err := s.Store.MFAEnabled(r.Context(), u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if !on {
		WriteError(w, E(http.StatusBadRequest, "mfa_disabled", "Turn on an authenticator first."))
		return
	}
	codes, hashes, err := s.newBackupCodes(u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.ReplaceBackupCodes(r.Context(), u.ID, hashes); err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

// mfaChallenge is the reply to a password that was right but is not enough.
func (s *Server) mfaChallenge(w http.ResponseWriter, u *store.User, start time.Time) {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	tk := s.signTicket(ticket{Purpose: "login-mfa", User: u.ID.String(), Exp: time.Now().Add(mfaLoginTTL).Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonce)})
	// This has no request to hang off — the caller does not pass one — so it
	// gets a deadline of its own rather than none at all. The count is only
	// shown beside the code box, so a failure leaves it unsaid rather than
	// failing the sign-in.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	left, err := s.Store.BackupCodesLeft(ctx, u.ID)
	cancel()
	if err != nil {
		s.Log.Warn("backup codes left", "user", u.ID.String(), "err", err)
	}
	if d := time.Since(start); d < minLoginTime {
		time.Sleep(minLoginTime - d)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "ticket": tk, "backup_codes_left": left})
}

// mfaReplays holds, for each sign-in ticket whose second step passed, which
// code passed it, until the ticket lapses. A phone that loses the answer
// (Safari's "Load failed" when someone switches apps, a dropped connection)
// still spent the code, so asking again with it was refused and the person
// was locked out of a code they never got to use, a backup code gone for
// good. The same ticket with the same code gets a session again instead.
// Only whoever passed the password step holds the ticket, and a code never
// counts for any other ticket, so a code is still good for one sign-in.
// In memory, per process, as chatd is one.
type mfaReplays struct {
	mu   sync.Mutex
	done map[[32]byte]mfaReplay
}

type mfaReplay struct {
	proof [32]byte
	until time.Time
}

func mfaReplayKey(ticket string) [32]byte { return sha256.Sum256([]byte(ticket)) }

// mfaProof is what a request presented, so a repeat can be told from another code.
func (s *Server) mfaProof(uid ids.ID, code, backup string) [32]byte {
	if strings.TrimSpace(backup) != "" {
		return sha256.Sum256([]byte("backup\x00" + s.hashBackupCode(uid, backup)))
	}
	return sha256.Sum256([]byte("totp\x00" + strings.Join(strings.Fields(code), "")))
}

func (m *mfaReplays) remember(ticket string, proof [32]byte, until time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if m.done == nil {
		m.done = map[[32]byte]mfaReplay{}
	}
	for k, v := range m.done {
		if now.After(v.until) {
			delete(m.done, k)
		}
	}
	m.done[mfaReplayKey(ticket)] = mfaReplay{proof: proof, until: until}
}

func (m *mfaReplays) repeated(ticket string, proof [32]byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.done[mfaReplayKey(ticket)]
	return ok && time.Now().Before(v.until) && subtle.ConstantTimeCompare(v.proof[:], proof[:]) == 1
}

func (s *Server) handleLoginMFA(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
		Backup string `json:"backup_code"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	tk, ok := s.readTicket(req.Ticket, "login-mfa")
	if !ok {
		WriteError(w, E(http.StatusUnauthorized, "mfa_expired", "That sign-in timed out. Enter your password again."))
		return
	}
	uid, err := ids.Parse(tk.User)
	if err != nil {
		WriteError(w, E(http.StatusUnauthorized, "mfa_expired", "That sign-in timed out. Enter your password again."))
		return
	}
	if ok, retry := s.limAuthUsr.Allow("mfa:" + tk.User); !ok {
		w.Header().Set("Retry-After", retryAfter(retry))
		WriteError(w, ErrRateLimited)
		return
	}
	ctx := r.Context()
	u, err := s.Store.UserByID(ctx, uid)
	if err != nil || u.Flags&(store.FlagDeleted|store.FlagDisabled) != 0 {
		WriteError(w, E(http.StatusUnauthorized, "invalid_credentials", "Username or password is wrong."))
		return
	}
	m, err := s.Store.MFAFor(ctx, uid)
	if err != nil {
		WriteError(w, err)
		return
	}
	if m == nil || !m.Enabled {
		WriteError(w, E(http.StatusBadRequest, "mfa_disabled", "This account no longer uses an authenticator. Sign in again."))
		return
	}
	fail := func() {
		if d := time.Since(start); d < minLoginTime {
			time.Sleep(minLoginTime - d)
		}
		WriteError(w, E(http.StatusUnauthorized, "invalid_code", "That code is wrong or has already been used."))
	}
	passed := false
	switch {
	case strings.TrimSpace(req.Backup) != "":
		spent, err := s.Store.SpendBackupCode(ctx, uid, s.hashBackupCode(uid, req.Backup))
		if err != nil {
			WriteError(w, err)
			return
		}
		passed = spent
	default:
		secret, err := s.openSecret(m.Secret)
		if err != nil {
			WriteError(w, err)
			return
		}
		step, valid := totp.Verify(secret, req.Code, time.Now(), totp.CodeSkew)
		if valid {
			// A code is good once: the counter it used is burned immediately.
			fresh, err := s.Store.SpendTOTPStep(ctx, uid, step)
			if err != nil {
				WriteError(w, err)
				return
			}
			passed = fresh
		}
	}
	proof := s.mfaProof(uid, req.Code, req.Backup)
	if !passed {
		// the same code again on the same ticket: the answer that signed in was lost on the way
		// (a ticket from before nonces could be another sign-in's, so it never repeats)
		if tk.Nonce == "" || !s.mfaAnswered.repeated(req.Ticket, proof) {
			fail()
			return
		}
	} else if tk.Nonce != "" {
		s.mfaAnswered.remember(req.Ticket, proof, time.Unix(tk.Exp, 0))
	}
	token, err := s.newSession(r, uid)
	if err != nil {
		WriteError(w, err)
		return
	}
	s.limAuthUsr.Reset("mfa:" + tk.User)
	s.limAuthUsr.Reset("login:" + u.Username)
	if d := time.Since(start); d < minLoginTime {
		time.Sleep(minLoginTime - d)
	}
	resp := keyBundle(u)
	resp["user"] = s.meView(ctx, u)
	resp["token"] = token
	WriteJSON(w, http.StatusOK, resp)
}
