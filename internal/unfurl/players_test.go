package unfurl

import (
	"net/url"
	"strings"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestPlayerForKnownVideos(t *testing.T) {
	cases := []struct{ link, site, contains string }{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "YouTube", "youtube-nocookie.com/embed/dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ?t=10", "YouTube", "youtube-nocookie.com/embed/dQw4w9WgXcQ"},
		{"https://vimeo.com/123456789", "Vimeo", "player.vimeo.com/video/123456789"},
		{"https://vimeo.com/123456789/abc123", "Vimeo", "h=abc123"},
		{"https://www.twitch.tv/videos/1234567", "Twitch", "player.twitch.tv/?video=1234567"},
		{"https://clips.twitch.tv/SomeClipSlug", "Twitch", "clips.twitch.tv/embed?clip=SomeClipSlug"},
		{"https://www.twitch.tv/someone/clip/SomeClipSlug", "Twitch", "clip=SomeClipSlug"},
		{"https://streamable.com/abc12", "Streamable", "streamable.com/e/abc12"},
		{"https://www.dailymotion.com/video/x8abcde", "Dailymotion", "dailymotion.com/embed/video/x8abcde"},
		{"https://www.loom.com/share/0123456789abcdef0123", "Loom", "loom.com/embed/0123456789abcdef0123"},
	}
	for _, c := range cases {
		p := PlayerFor(mustURL(t, c.link), "chat.evilion.xyz")
		if p == nil {
			t.Errorf("%s: no player", c.link)
			continue
		}
		if p.Site != c.site || !strings.Contains(p.URL, c.contains) {
			t.Errorf("%s: got %+v, want site %q containing %q", c.link, p, c.site, c.contains)
		}
		if !strings.HasPrefix(p.URL, "https://") {
			t.Errorf("%s: player must be https, got %q", c.link, p.URL)
		}
	}
}

// Anything that is not one of the known video players gets no frame at all:
// a preview never runs a page someone linked to.
func TestPlayerForRefusesEverythingElse(t *testing.T) {
	for _, link := range []string{
		"https://example.com/video.html",
		"https://evil.example/?url=https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtube.com.evil.example/watch?v=dQw4w9WgXcQ",
		"https://vimeo.com/notanumber",
		"https://vimeo.com/123456789/../../evil",
		"https://twitch.tv/someone",
		"https://streamable.com/e/abc12/../../x",
		"javascript:alert(1)",
		"https://open.spotify.com/track/abc",
		"https://docs.google.com/document/d/abc/edit",
	} {
		if p := PlayerFor(mustURL(t, link), "chat.evilion.xyz"); p != nil {
			t.Errorf("%s: unexpected player %+v", link, p)
		}
	}
}

// Twitch refuses to open without being told where it is embedded, so no host
// means no Twitch player rather than a broken one.
func TestTwitchNeedsAParent(t *testing.T) {
	if p := PlayerFor(mustURL(t, "https://www.twitch.tv/videos/1234567"), ""); p != nil {
		t.Errorf("unexpected player %+v", p)
	}
}

// Nexa at several addresses names each of them, so the player opens at any.
func TestTwitchIsToldEveryAddress(t *testing.T) {
	p := PlayerFor(mustURL(t, "https://www.twitch.tv/videos/1234567"), "nexa.pm", "", "mathtest.lol", "chat.evilion.xyz")
	if p == nil {
		t.Fatal("no player")
	}
	if !strings.HasSuffix(p.URL, "&parent=nexa.pm&parent=mathtest.lol&parent=chat.evilion.xyz") {
		t.Errorf("parents in %s", p.URL)
	}
	if p := PlayerFor(mustURL(t, "https://www.twitch.tv/videos/1234567")); p != nil {
		t.Errorf("a player with no parent at all: %+v", p)
	}
}
