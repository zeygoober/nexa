package api

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A player's token is good only for its own site, until it expires, and only
// with the secret that signed it.
func TestFrameTokens(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.Secret = []byte("secret one")
	good := srv.frameSign(spotifySite, time.Now().Add(time.Hour).Unix())
	if !srv.frameTokenOK(spotifySite, good) {
		t.Fatal("a fresh token refused")
	}
	if srv.frameTokenOK(soundcloudSite, good) {
		t.Fatal("one site's token opened another")
	}
	if srv.frameTokenOK(spotifySite, srv.frameSign(spotifySite, time.Now().Add(-time.Minute).Unix())) {
		t.Fatal("an expired token taken")
	}
	exp, _, _ := strings.Cut(good, ".")
	n, _ := strconv.ParseInt(exp, 36, 64)
	if srv.frameTokenOK(spotifySite, strconv.FormatInt(n+3600, 36)+good[len(exp):]) {
		t.Fatal("a token with its expiry pushed back taken")
	}
	for _, bad := range []string{"", "x", "abc.def", good + "x"} {
		if srv.frameTokenOK(spotifySite, bad) {
			t.Fatalf("%q taken", bad)
		}
	}
	srv.Secret = []byte("secret two")
	if srv.frameTokenOK(spotifySite, good) {
		t.Fatal("a token signed with another secret taken")
	}
}

// The site's addresses in a page, its stylesheets and its inline scripts point
// at the proxy; anyone else's are left for the policy to refuse.
func TestFrameRewrite(t *testing.T) {
	base, _ := url.Parse("https://open.spotify.com/embed/track/abc")
	page := `<html><head><link href="/static/x.css" rel="stylesheet"><script nonce="n" src="https://encore.scdn.co/x/base.js"></script>` +
		`<style>@font-face{src:url(//encore.scdn.co/fonts/a.woff2)}</style></head>` +
		`<body><img src="https://i.scdn.co/image/abc"><a href="https://evil.example/">x</a>` +
		`<script>var c={"u":"https:\/\/api.spotify.com\/v1"}</script></body></html>`
	out := string(spotifySite.injectShim(spotifySite.rewriteHTML([]byte(page), "/sp/T", base), "/sp/T", "https://chat.example/"))
	for _, want := range []string{
		`href="/sp/T/p/open.spotify.com/static/x.css"`,
		`<script crossorigin="anonymous" nonce="n" src="/sp/T/p/encore.scdn.co/x/base.js"`,
		`url(/sp/T/p/encore.scdn.co/fonts/a.woff2)`,
		`src="/sp/T/p/i.scdn.co/image/abc"`,
		`href="https://evil.example/"`,
		`\/sp\/T\/p\/api.spotify.com\/v1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rewritten page lacks %s\n%s", want, out)
		}
	}
	// the shim comes before anything of the site's runs
	if head := strings.Index(out, "<head>"); !strings.HasPrefix(out[head+len("<head>"):], "<script>(function(C){") {
		t.Errorf("shim not first in head:\n%s", out)
	}
	css := string(spotifySite.rewriteCSS([]byte(`a{background:url("https://i.scdn.co/a.png")} b{background:url(/img/b.png)} c{background:url(data:image/png;base64,AA)}`), "/sp/T", base))
	for _, want := range []string{`url("/sp/T/p/i.scdn.co/a.png")`, `url(/sp/T/p/open.spotify.com/img/b.png)`, `url(data:image/png;base64,AA)`} {
		if !strings.Contains(css, want) {
			t.Errorf("rewritten css lacks %s: %s", want, css)
		}
	}
}
