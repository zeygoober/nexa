package api

import (
	"net/http"
	"net/url"
	"strings"
)

// Nexa answers at more than one address: its own (PUBLIC_URL), others that
// work in full (ALT_URLS) and the ones it moved away from (MOVED_FROM). These
// say which of them a request, an Origin or a link is at.

type addressKind int

const (
	notOurs addressKind = iota
	ourPublic
	ourAlt
	ourMoved
)

// addressOf reports which of Nexa's addresses a host (with its port, if it
// has one) is, and that address as configured: "https://nexa.pm".
func (s *Server) addressOf(host string) (string, addressKind) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "", notOurs
	}
	for _, l := range []struct {
		urls []string
		kind addressKind
	}{{[]string{s.Cfg.PublicURL}, ourPublic}, {s.Cfg.AltURLs, ourAlt}, {s.Cfg.MovedFrom, ourMoved}} {
		for _, raw := range l.urls {
			if u, err := url.Parse(raw); err == nil && strings.EqualFold(u.Host, host) {
				return strings.TrimRight(raw, "/"), l.kind
			}
		}
	}
	return "", notOurs
}

// originAddress is addressOf for an Origin header or any other whole URL.
func (s *Server) originAddress(origin string) (string, addressKind) {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Host == "" {
		return "", notOurs
	}
	addr, kind := s.addressOf(u.Host)
	// the same host over the other scheme is someone else's page
	if kind != notOurs && !strings.HasPrefix(addr, u.Scheme+"://") {
		return "", notOurs
	}
	return addr, kind
}

// requestAddress is the address a request came in at, when it is one of
// Nexa's own: the host it asked for, as the tunnel passes it on.
func (s *Server) requestAddress(r *http.Request) (string, addressKind) {
	return s.addressOf(r.Host)
}

// hereOrPublic is the address to send someone back to: the one their
// request came in at when it is Nexa's, its own address otherwise.
func (s *Server) hereOrPublic(r *http.Request) string {
	if addr, kind := s.requestAddress(r); kind != notOurs {
		return addr
	}
	return strings.TrimRight(s.Cfg.PublicURL, "/")
}

// ourHostnames are the hostnames of every address, without ports: what
// Twitch's player and the captcha are told the pages run on.
func (s *Server) ourHostnames() []string {
	var out []string
	for _, raw := range s.Cfg.Origins() {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			out = append(out, strings.ToLower(u.Hostname()))
		}
	}
	return out
}
