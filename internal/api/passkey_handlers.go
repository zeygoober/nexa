package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
	"evilion.xyz/chat/internal/webauthn"
)

// Passkeys sign an account in and open its messages, in one step. The app
// asks the passkey for its PRF output, makes a key from it and wraps the
// identity key with that; the server keeps the wrapped copy beside the
// passkey's public key and hands it back only to a sign-in the passkey signed.
// A passkey that cannot give a PRF output is refused by the app before it gets
// here, since it could sign in but never read anything.
//
// A passkey belongs to the address it was made on (its RP ID): one made at
// nexa.pm is offered there and in the Android app, one made at another of
// Nexa's addresses only there.

const (
	passkeyTTL      = 5 * time.Minute
	maxPasskeyName  = 64
	maxPasskeyBlob  = 4 << 10
	passkeyNotFound = "That passkey is not on any Nexa account. It may have been removed."
)

func (s *Server) passkeyRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/auth/passkeys/options", s.authed(s.handlePasskeyOptions))
	mux.Handle("POST /api/v1/auth/passkeys", s.authed(s.handlePasskeyCreate))
	mux.Handle("GET /api/v1/auth/passkeys", s.authed(s.handlePasskeyList))
	mux.Handle("PATCH /api/v1/auth/passkeys/{id}", s.authed(s.handlePasskeyRename))
	mux.Handle("DELETE /api/v1/auth/passkeys/{id}", s.authed(s.handlePasskeyDelete))
	mux.Handle("POST /api/v1/auth/passkey/begin", ipLimited(s.limAuthIP, s.handlePasskeyBegin))
	mux.Handle("POST /api/v1/auth/passkey/finish", ipLimited(s.limAuthIP, s.handlePasskeyFinish))
}

// spentChallenges keeps the challenges already answered until they would have
// expired anyway, so one answer signs in once. Per process, like the captcha
// batches.
type spentChallenges struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func (c *spentChallenges) spend(ch string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.seen == nil {
		c.seen = map[string]time.Time{}
	}
	for k, exp := range c.seen {
		if now.After(exp) {
			delete(c.seen, k)
		}
	}
	if _, ok := c.seen[ch]; ok {
		return false
	}
	c.seen[ch] = now.Add(passkeyTTL + time.Minute)
	return true
}

// passkeyRP is the relying party a request makes passkeys for: the address
// its page is at when that is one of Nexa's working addresses, otherwise (the
// Android app, which sends no Origin) Nexa's own.
func (s *Server) passkeyRP(r *http.Request) string {
	if o := r.Header.Get("Origin"); o != "" {
		if addr, kind := s.originAddress(o); kind == ourPublic || kind == ourAlt {
			if u, err := url.Parse(addr); err == nil {
				return strings.ToLower(u.Hostname())
			}
		}
	}
	u, _ := url.Parse(s.Cfg.PublicURL)
	return strings.ToLower(u.Hostname())
}

// passkeyOriginOK says whether the origin a passkey's answer names may speak
// for rpID: one of Nexa's pages on that host, or the published Android app
// for Nexa's own address.
func (s *Server) passkeyOriginOK(origin, rpID string) bool {
	if strings.HasPrefix(origin, "android:apk-key-hash:") {
		pub, _ := url.Parse(s.Cfg.PublicURL)
		app := s.androidOrigin()
		return app != "" && origin == app && strings.EqualFold(pub.Hostname(), rpID)
	}
	addr, kind := s.originAddress(origin)
	if kind != ourPublic && kind != ourAlt {
		return false
	}
	u, err := url.Parse(addr)
	return err == nil && strings.EqualFold(u.Hostname(), rpID)
}

// androidOrigin is how Android names the published app to a passkey: the
// SHA-256 of its signing certificate, base64url.
func (s *Server) androidOrigin() string {
	rel, _ := s.androidManifest()
	if rel.SigningSHA256 == "" {
		return ""
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(rel.SigningSHA256, ":", ""))
	if err != nil {
		return ""
	}
	return "android:apk-key-hash:" + base64.RawURLEncoding.EncodeToString(raw)
}

func newChallenge() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

// handlePasskeyOptions starts adding a passkey, after the password: what the
// app passes to navigator.credentials.create (or Android's Credential
// Manager), and a ticket that carries the challenge to the next step.
func (s *Server) handlePasskeyOptions(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		AuthKey []byte `json:"auth_key"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if !s.requireCaptcha(w, r, "passkey") {
		return
	}
	if err := s.requirePassword(w, u, req.AuthKey, "Your password is wrong."); err != nil {
		WriteError(w, err)
		return
	}
	have, err := s.Store.PasskeysByUser(r.Context(), u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	if len(have) >= store.MaxPasskeys {
		WriteError(w, E(http.StatusBadRequest, "too_many_passkeys", "You have as many passkeys as an account can keep. Remove one first."))
		return
	}
	rp := s.passkeyRP(r)
	exclude := []map[string]any{}
	for _, p := range have {
		if p.RPID == rp {
			exclude = append(exclude, map[string]any{"type": "public-key", "id": base64.RawURLEncoding.EncodeToString(p.CredentialID)})
		}
	}
	challenge := newChallenge()
	params := []map[string]any{}
	for _, a := range webauthn.Algs {
		params = append(params, map[string]any{"type": "public-key", "alg": a})
	}
	tk := s.signTicket(ticket{Purpose: "passkey-create", User: u.ID.String(), Exp: time.Now().Add(passkeyTTL).Unix(), Secret: rp + " " + base64.RawURLEncoding.EncodeToString(challenge)})
	WriteJSON(w, http.StatusOK, map[string]any{
		"ticket": tk,
		// the creation options in WebAuthn's JSON form (binary as base64url)
		"public_key": map[string]any{
			"challenge":          base64.RawURLEncoding.EncodeToString(challenge),
			"rp":                 map[string]any{"id": rp, "name": "Nexa"},
			"user":               map[string]any{"id": base64.RawURLEncoding.EncodeToString([]byte(u.ID.String())), "name": u.Username, "displayName": u.DisplayName},
			"pubKeyCredParams":   params,
			"excludeCredentials": exclude,
			"authenticatorSelection": map[string]any{
				"residentKey": "required", "requireResidentKey": true, "userVerification": "required",
			},
			"attestation": "none",
			"timeout":     int(passkeyTTL / time.Millisecond),
		},
	})
}

func (s *Server) handlePasskeyCreate(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		Ticket            string `json:"ticket"`
		Name              string `json:"name"`
		ClientDataJSON    []byte `json:"client_data_json"`
		AttestationObject []byte `json:"attestation_object"`
		EncPrivateKey     []byte `json:"enc_private_key"`
	}
	if err := ReadJSON(w, r, &req, 32<<10); err != nil {
		WriteError(w, err)
		return
	}
	tk, ok := s.readTicket(req.Ticket, "passkey-create")
	rp, ch, _ := strings.Cut(tk.Secret, " ")
	challenge, _ := base64.RawURLEncoding.DecodeString(ch)
	if !ok || tk.User != u.ID.String() || len(challenge) == 0 {
		WriteError(w, E(http.StatusBadRequest, "passkey_expired", "That took too long. Start adding the passkey again."))
		return
	}
	bad := func() {
		WriteError(w, E(http.StatusBadRequest, "passkey_invalid", "The passkey's answer could not be checked."))
	}
	if len(req.EncPrivateKey) == 0 || len(req.EncPrivateKey) > maxWrappedKey || len(req.AttestationObject) > maxPasskeyBlob || len(req.ClientDataJSON) > maxPasskeyBlob {
		bad()
		return
	}
	origin, err := webauthn.CheckClientData(req.ClientDataJSON, "webauthn.create", challenge)
	if err != nil || !s.passkeyOriginOK(origin, rp) {
		bad()
		return
	}
	raw, err := webauthn.AuthDataFromAttestation(req.AttestationObject)
	if err != nil {
		bad()
		return
	}
	ad, err := webauthn.ParseAuthData(raw, rp)
	if err != nil || ad.PublicKey == nil || !s.spentPasskey.spend(ch) {
		bad()
		return
	}
	spki, err := webauthn.MarshalKey(ad.PublicKey)
	if err != nil {
		bad()
		return
	}
	p := &store.Passkey{UserID: u.ID, CredentialID: ad.CredentialID, PublicKey: spki, Alg: ad.Alg, SignCount: int64(ad.SignCount),
		RPID: rp, Name: clipRunes(strings.TrimSpace(req.Name), maxPasskeyName), EncPrivateKey: req.EncPrivateKey}
	switch err := s.Store.CreatePasskey(r.Context(), p); {
	case errors.Is(err, store.ErrConflict):
		WriteError(w, E(http.StatusConflict, "passkey_exists", "That passkey is already on an account."))
		return
	case errors.Is(err, store.ErrLimit):
		WriteError(w, E(http.StatusBadRequest, "too_many_passkeys", "You have as many passkeys as an account can keep. Remove one first."))
		return
	case err != nil:
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, p)
}

func (s *Server) handlePasskeyList(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.PasskeysByUser(r.Context(), auth.UserFrom(r.Context()).ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, list)
}

func (s *Server) handlePasskeyRename(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if err := s.Store.RenamePasskey(r.Context(), auth.UserFrom(r.Context()).ID, id, clipRunes(strings.TrimSpace(req.Name), maxPasskeyName)); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	if err := s.Store.DeletePasskey(r.Context(), auth.UserFrom(r.Context()).ID, id); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePasskeyBegin is the challenge for a sign-in. No username: the
// passkey says whose it is.
func (s *Server) handlePasskeyBegin(w http.ResponseWriter, r *http.Request) {
	challenge := newChallenge()
	rp := s.passkeyRP(r)
	tk := s.signTicket(ticket{Purpose: "passkey-get", Exp: time.Now().Add(passkeyTTL).Unix(), Secret: base64.RawURLEncoding.EncodeToString(challenge)})
	WriteJSON(w, http.StatusOK, map[string]any{
		"ticket": tk,
		"public_key": map[string]any{
			"challenge":        base64.RawURLEncoding.EncodeToString(challenge),
			"rpId":             rp,
			"allowCredentials": []any{},
			"userVerification": "required",
			"timeout":          int(passkeyTTL / time.Millisecond),
		},
	})
}

func (s *Server) handlePasskeyFinish(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req struct {
		Ticket            string `json:"ticket"`
		CredentialID      []byte `json:"credential_id"`
		ClientDataJSON    []byte `json:"client_data_json"`
		AuthenticatorData []byte `json:"authenticator_data"`
		Signature         []byte `json:"signature"`
	}
	if err := ReadJSON(w, r, &req, 32<<10); err != nil {
		WriteError(w, err)
		return
	}
	fail := func(code, msg string) {
		if d := time.Since(start); d < minLoginTime {
			time.Sleep(minLoginTime - d)
		}
		WriteError(w, E(http.StatusUnauthorized, code, msg))
	}
	tk, ok := s.readTicket(req.Ticket, "passkey-get")
	challenge, _ := base64.RawURLEncoding.DecodeString(tk.Secret)
	if !ok || len(challenge) == 0 {
		fail("passkey_expired", "That took too long. Try the passkey again.")
		return
	}
	p, err := s.Store.PasskeyByCredential(r.Context(), req.CredentialID)
	if errors.Is(err, store.ErrNotFound) || len(req.CredentialID) == 0 {
		fail("passkey_unknown", passkeyNotFound)
		return
	}
	if err != nil {
		WriteError(w, err)
		return
	}
	origin, err := webauthn.CheckClientData(req.ClientDataJSON, "webauthn.get", challenge)
	if err != nil || !s.passkeyOriginOK(origin, p.RPID) {
		fail("passkey_invalid", "The passkey's answer could not be checked.")
		return
	}
	ad, err := webauthn.ParseAuthData(req.AuthenticatorData, p.RPID)
	if err != nil || webauthn.VerifySignature(p.PublicKey, p.Alg, req.AuthenticatorData, req.ClientDataJSON, req.Signature) != nil {
		fail("passkey_invalid", "The passkey's answer could not be checked.")
		return
	}
	// a counter that went back means a copy of the passkey is in use somewhere
	if (p.SignCount != 0 || ad.SignCount != 0) && int64(ad.SignCount) <= p.SignCount {
		fail("passkey_invalid", "The passkey's answer could not be checked.")
		return
	}
	if !s.spentPasskey.spend(tk.Secret) {
		fail("passkey_expired", "That took too long. Try the passkey again.")
		return
	}
	u, err := s.Store.UserByID(r.Context(), p.UserID)
	if err != nil || u.Flags&(store.FlagDeleted|store.FlagDisabled|store.NotPeople) != 0 {
		fail("passkey_unknown", passkeyNotFound)
		return
	}
	if ok, err := s.Store.UsePasskey(r.Context(), p.ID, p.SignCount, int64(ad.SignCount)); err != nil || !ok {
		fail("passkey_invalid", "The passkey's answer could not be checked.")
		return
	}
	// a passkey that verified its person is already a second factor
	token, err := s.newSession(r, u.ID)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"user":            s.meView(r.Context(), u),
		"token":           token,
		"public_key":      u.PublicKey,
		"enc_private_key": p.EncPrivateKey,
	})
}
