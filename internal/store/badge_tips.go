package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"evilion.xyz/chat/internal/ids"
)

// The owner's own words for a built-in badge's tooltip, in every language in
// place of the apps' translations: the instance setting badge_tips, a JSON
// map from a Badge Index key (the entry's art: "og", "bug-3", "plan-pro",
// "mentor") to the text. A tiered badge has one per level, since each level
// is its own badge in the index. Event badges keep theirs in badge_defs.
const badgeTipsKey = "badge_tips"

// MaxBadgeTip is the longest tooltip in characters, as for event badges.
const MaxBadgeTip = 120

// badgeTipsFresh is how long the owner's words are used before they are read
// again; a change made here is seen at once.
const badgeTipsFresh = 30 * time.Second

// ErrBadgeTipKey is a description for a badge that does not exist.
var ErrBadgeTipKey = errors.New("no such badge")

// ErrBadgeTipLong is a description past MaxBadgeTip.
var ErrBadgeTipLong = errors.New("badge description too long")

type badgeTipCache struct {
	mu   sync.Mutex
	tips map[string]string
	at   time.Time
}

// badgeTips answers the owner's words; none when they cannot be read, so the
// apps' own show rather than a profile failing.
func (s *Store) badgeTips(ctx context.Context) map[string]string {
	c := &s.tips
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tips != nil && time.Since(c.at) < badgeTipsFresh {
		return c.tips
	}
	raw, err := s.InstanceSetting(ctx, badgeTipsKey)
	if err != nil {
		return c.tips
	}
	tips := map[string]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &tips)
	}
	c.tips, c.at = tips, time.Now()
	return tips
}

// tieredArt is the index art of each badge with levels, before "-<level>".
var tieredArt = map[string]string{
	BadgeBugHunter: "bug", BadgeIdeas: "ideas", BadgeAppDeveloper: "bot-maker", BadgeGiftGiver: "gift",
	BadgeFriendly: "friendly", BadgeHost: "host", BadgeRegular: "regular", BadgeOpenMic: "open-mic",
}

// badgeTipKey is the key a badge's words are kept under, for an account on
// plan; "" for staff ranks and event badges, which have none here.
func badgeTipKey(b Badge, plan string) string {
	switch {
	case b.ID == BadgeStaff || strings.HasPrefix(b.ID, EventBadgePrefix):
		return ""
	case b.ID == BadgePlan:
		if plan == "supporter" || plan == "pro" {
			return "plan-" + plan
		}
		return ""
	}
	if art, ok := tieredArt[b.ID]; ok {
		return art + "-" + strconv.Itoa(max(b.Level, 1))
	}
	return b.ID
}

// describeBadges puts the owner's words on the badges of an account on plan.
func (s *Store) describeBadges(ctx context.Context, list []Badge, plan string) {
	tips := s.badgeTips(ctx)
	if len(tips) == 0 {
		return
	}
	for i := range list {
		if t := tips[badgeTipKey(list[i], plan)]; t != "" {
			list[i].Description = t
		}
	}
}

// describeIndex puts the owner's words on what the index shows as held. A
// locked entry carries no badge, so its words are never sent.
func (s *Store) describeIndex(ctx context.Context, entries []BadgeIndexEntry) {
	tips := s.badgeTips(ctx)
	if len(tips) == 0 {
		return
	}
	for i := range entries {
		e := &entries[i]
		if e.Badge == nil || e.event != "" {
			continue
		}
		if t := tips[e.Art]; t != "" {
			e.Badge.Description = t
		}
	}
}

// SetBadgeTip keeps the owner's words for one badge by its index key; empty
// text goes back to the apps' own. An event badge's go to its definition.
func (s *Store) SetBadgeTip(ctx context.Context, key, text string) error {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > MaxBadgeTip {
		return ErrBadgeTipLong
	}
	if n, ok := strings.CutPrefix(key, EventBadgePrefix); ok {
		id, err := ids.Parse(n)
		if err != nil {
			return ErrBadgeTipKey
		}
		d, err := s.BadgeDef(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return ErrBadgeTipKey
		} else if err != nil {
			return err
		}
		d.Description = text
		return s.UpdateBadgeDef(ctx, d)
	}
	if _, ok := badgeIndexNames[key]; !ok {
		return ErrBadgeTipKey
	}
	c := &s.tips
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, err := s.InstanceSetting(ctx, badgeTipsKey)
	if err != nil {
		return err
	}
	tips := map[string]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &tips)
	}
	if text == "" {
		delete(tips, key)
	} else {
		tips[key] = text
	}
	value := ""
	if len(tips) > 0 {
		b, err := json.Marshal(tips)
		if err != nil {
			return err
		}
		value = string(b)
	}
	if err := s.SetInstanceSettings(ctx, [][2]string{{badgeTipsKey, value}}); err != nil {
		return err
	}
	c.tips, c.at = tips, time.Now()
	return nil
}
