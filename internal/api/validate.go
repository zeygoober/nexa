package api

import (
	"regexp"
	"strings"

	"evilion.xyz/chat/internal/textutil"
)

var usernameRe = regexp.MustCompile(`^[a-z0-9_.]{2,32}$`)

// shortNameRe is the one-character username a gift link gives: a letter or a
// number, and never available any other way.
var shortNameRe = regexp.MustCompile(`^[a-z0-9]$`)

// ShortUsername reports whether a name is a one-character one.
func ShortUsername(s string) bool { return shortNameRe.MatchString(s) }

// ValidUsername reports whether s is an acceptable unique username.
func ValidUsername(s string) bool {
	if !usernameRe.MatchString(s) {
		return false
	}
	if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") || strings.Contains(s, "..") {
		return false
	}
	return true
}

// CleanText trims, strips control characters (keeping \n and \t) and
// enforces a maximum rune count. ok is false when the result is too long.
// It lives in textutil so the gateway applies the same rules.
func CleanText(s string, max int) (out string, ok bool) { return textutil.CleanText(s, max) }

// CleanDisplayName returns a one-line display name of 1–32 runes, with
// nothing in it that shows as nothing: a name made only of invisible
// characters is no name.
func CleanDisplayName(s string) (string, bool) {
	out, ok := textutil.CleanName(s, 32)
	if !ok || out == "" {
		return "", false
	}
	return out, true
}

// OneOf reports whether v is in allowed.
func OneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
