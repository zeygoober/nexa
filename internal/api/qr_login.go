package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// Signing in with a QR code: a device that is not signed in makes a one-time
// X25519 key and shows a code carrying the request's id and that public key.
// A signed-in phone scans it, is told which device is asking, and on Approve
// seals the account's identity key to the key in the code (never to anything
// the server says) and sends the sealed box here. The server makes a session
// for the waiting device and hands it the token with the box, which only that
// device can open. Requests live in memory for qrTTL, per process.

const (
	qrTTL       = 3 * time.Minute
	qrPollWait  = 25 * time.Second
	maxQRWaits  = 2000
	qrSealedMax = 256
)

type qrState string

const (
	qrPending  qrState = "pending"
	qrScanned  qrState = "scanned"
	qrApproved qrState = "approved"
	qrDenied   qrState = "denied"
	qrExpired  qrState = "expired"
)

type qrLogin struct {
	pollHash  [32]byte
	publicKey []byte
	userAgent string
	ip        string
	created   time.Time
	state     qrState
	user      ids.ID
	token     string
	sealed    []byte
	changed   chan struct{} // closed and replaced on every change
}

type qrLogins struct {
	mu sync.Mutex
	m  map[string]*qrLogin
}

func (q *qrLogins) sweep(now time.Time) {
	for id, l := range q.m {
		if now.Sub(l.created) > qrTTL {
			delete(q.m, id)
			close(l.changed)
		}
	}
}

// set moves a request on and wakes whoever waits on it.
func (l *qrLogin) set(st qrState) {
	l.state = st
	close(l.changed)
	l.changed = make(chan struct{})
}

func (s *Server) qrRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/auth/qr", ipLimited(s.limAuthIP, s.handleQRCreate))
	mux.Handle("GET /api/v1/auth/qr/{id}", ipLimited(s.limLookup, s.handleQRPoll))
	mux.Handle("POST /api/v1/auth/qr/{id}/scan", s.authed(s.handleQRScan))
	mux.Handle("POST /api/v1/auth/qr/{id}/approve", s.authed(s.handleQRApprove))
	mux.Handle("POST /api/v1/auth/qr/{id}/deny", s.authed(s.handleQRDeny))
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) handleQRCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey []byte `json:"public_key"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if len(req.PublicKey) != 32 {
		WriteError(w, E(http.StatusBadRequest, "invalid_key", "public_key must be 32 bytes"))
		return
	}
	id, poll := randomToken(16), randomToken(24)
	now := time.Now()
	s.qr.mu.Lock()
	defer s.qr.mu.Unlock()
	if s.qr.m == nil {
		s.qr.m = map[string]*qrLogin{}
	}
	s.qr.sweep(now)
	if len(s.qr.m) >= maxQRWaits {
		WriteError(w, ErrRateLimited)
		return
	}
	s.qr.m[id] = &qrLogin{pollHash: sha256.Sum256([]byte(poll)), publicKey: req.PublicKey, userAgent: clip(r.UserAgent(), maxUserAgent),
		ip: auth.ClientIP(r), created: now, state: qrPending, changed: make(chan struct{})}
	WriteJSON(w, http.StatusOK, map[string]any{"id": id, "poll": poll, "expires_at": now.Add(qrTTL).UnixMilli()})
}

// handleQRPoll is the waiting device's long poll: it answers as soon as the
// request is no longer in the state the device last saw, or after a while.
func (s *Server) handleQRPoll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	poll := sha256.Sum256([]byte(r.URL.Query().Get("poll")))
	seen := qrState(r.URL.Query().Get("seen"))
	deadline := time.NewTimer(qrPollWait)
	defer deadline.Stop()
	for {
		s.qr.mu.Lock()
		l := s.qr.m[id]
		if l == nil || subtle.ConstantTimeCompare(l.pollHash[:], poll[:]) != 1 {
			s.qr.mu.Unlock()
			WriteJSON(w, http.StatusOK, map[string]any{"state": qrExpired})
			return
		}
		if time.Since(l.created) > qrTTL {
			delete(s.qr.m, id)
			close(l.changed)
			s.qr.mu.Unlock()
			WriteJSON(w, http.StatusOK, map[string]any{"state": qrExpired})
			return
		}
		if l.state != seen {
			resp, done := s.qrView(r.Context(), l)
			if done {
				delete(s.qr.m, id)
			}
			s.qr.mu.Unlock()
			WriteJSON(w, http.StatusOK, resp)
			return
		}
		wait := l.changed
		s.qr.mu.Unlock()
		select {
		case <-wait:
		case <-deadline.C:
			WriteJSON(w, http.StatusOK, map[string]any{"state": seen})
			return
		case <-r.Context().Done():
			return
		}
	}
}

// qrView is what the waiting device is told, and whether the request is over.
func (s *Server) qrView(ctx context.Context, l *qrLogin) (map[string]any, bool) {
	out := map[string]any{"state": l.state}
	switch l.state {
	case qrScanned:
		// who is about to approve, so the device can say "check your phone, zey"
		if u, err := s.Store.UserByID(ctx, l.user); err == nil {
			out["user"] = u.Public()
		}
	case qrApproved:
		u, err := s.Store.UserByID(ctx, l.user)
		if err != nil {
			return map[string]any{"state": qrExpired}, true
		}
		out["user"] = s.meView(ctx, u)
		out["token"] = l.token
		out["public_key"] = u.PublicKey
		out["sealed_identity"] = l.sealed
		return out, true
	case qrDenied:
		return out, true
	}
	return out, false
}

// qrFor finds a live request for a signed-in person acting on it.
func (s *Server) qrFor(w http.ResponseWriter, r *http.Request) *qrLogin {
	l := s.qr.m[r.PathValue("id")]
	if l == nil || time.Since(l.created) > qrTTL || l.state == qrApproved || l.state == qrDenied {
		WriteError(w, E(http.StatusNotFound, "qr_expired", "That code has expired. Show a new one on the other device."))
		return nil
	}
	if l.state == qrScanned && l.user != auth.UserFrom(r.Context()).ID {
		WriteError(w, E(http.StatusConflict, "qr_taken", "Someone else already scanned that code."))
		return nil
	}
	return l
}

func (s *Server) handleQRScan(w http.ResponseWriter, r *http.Request) {
	s.qr.mu.Lock()
	defer s.qr.mu.Unlock()
	l := s.qrFor(w, r)
	if l == nil {
		return
	}
	if l.state == qrPending {
		l.user = auth.UserFrom(r.Context()).ID
		l.set(qrScanned)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"user_agent": l.userAgent, "created_at": l.created.UnixMilli()})
}

func (s *Server) handleQRApprove(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		SealedIdentity []byte `json:"sealed_identity"`
	}
	if err := ReadJSON(w, r, &req, 4<<10); err != nil {
		WriteError(w, err)
		return
	}
	if len(req.SealedIdentity) == 0 || len(req.SealedIdentity) > qrSealedMax {
		WriteError(w, E(http.StatusBadRequest, "invalid_box", "sealed_identity is missing or too long"))
		return
	}
	s.qr.mu.Lock()
	defer s.qr.mu.Unlock()
	l := s.qrFor(w, r)
	if l == nil {
		return
	}
	if l.state != qrScanned {
		WriteError(w, E(http.StatusConflict, "qr_not_scanned", "Scan the code first."))
		return
	}
	token, hash := auth.NewToken()
	se := &store.Session{UserID: u.ID, TokenHash: hash, UserAgent: l.userAgent, IP: l.ip}
	if err := s.Store.CreateSession(r.Context(), se); err != nil {
		WriteError(w, err)
		return
	}
	l.token, l.sealed = token, req.SealedIdentity
	l.set(qrApproved)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleQRDeny(w http.ResponseWriter, r *http.Request) {
	s.qr.mu.Lock()
	defer s.qr.mu.Unlock()
	l := s.qrFor(w, r)
	if l == nil {
		return
	}
	l.set(qrDenied)
	w.WriteHeader(http.StatusNoContent)
}
