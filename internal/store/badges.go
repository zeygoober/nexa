package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// Profile badges. Most are earned and worked out here when a profile is
// opened, from what the account already is or did; staff give Contributor and
// the event badges they make by hand (user_badges). Each person orders their
// own and hides any but their staff rank (badge_prefs). Bots, Nexa's own
// accounts and deleted accounts have none.
const (
	BadgeStaff        = "staff"         // a staff rank; drawn from the account's flags
	BadgePlan         = "plan"          // Supporter or Pro; drawn from the account's plan
	BadgeOG           = "og"            // the account was made before OGCutoff
	BadgeYears        = "years"         // whole years on Nexa, from one
	BadgeBugHunter    = "bug_hunter"    // staff resolved their bug reports; Level by ReportTiers
	BadgeIdeas        = "ideas"         // staff resolved their suggestions; Level by ReportTiers
	BadgeAppDeveloper = "app_developer" // one of their apps' bots is in AppDeveloperServers servers they do not own
	BadgeGiftGiver    = "gift_giver"    // they paid for a plan someone else got; Level by GiftTiers
	BadgeContributor  = "contributor"   // given by staff to people whose code Nexa merged
	EventBadgePrefix  = "event:"        // given by staff: "event:<badge_defs id>"
)

// OGCutoff is the start of 2026-11-02 in Alaska (UTC-9): accounts made
// before it, the day badges arrived, are OG.
const OGCutoff int64 = 1793610000000

// AppDeveloperServers is how many servers, not the developer's own, one of
// their bots must be in: a test app in their own server does not count.
const AppDeveloperServers = 5

// ReportTiers are the resolved bug reports (Bug Hunter) or suggestions
// (Ideas) each of the five tiers needs.
var ReportTiers = [5]int{1, 3, 10, 25, 50}

// GiftTiers are the paid gifts Gift Giver's bronze, silver and gold need.
var GiftTiers = [3]int{1, 5, 20}

// defaultBadgeOrder is where a badge goes when its holder has not placed it.
var defaultBadgeOrder = []string{BadgeStaff, BadgePlan, BadgeOG, BadgeYears, BadgeContributor, BadgeBugHunter, BadgeIdeas, BadgeAppDeveloper, BadgeGiftGiver}

// Badge is one badge on a profile. Staff and plan carry nothing more: the
// clients draw them from the account itself.
type Badge struct {
	ID string `json:"id"`
	// Level is Gift Giver's tier (1 bronze, 2 silver, 3 gold), Bug Hunter's
	// and Ideas' (1 to 5, by ReportTiers) or the years on Nexa.
	Level int `json:"level,omitempty"`
	// Name, Description (its tooltip), Icon (a Lucide icon's name), Color and
	// Color2 (a gradient's second colour), Glyph ("dark" for a dark icon),
	// Shape (missing for a circle) and Image (a picture drawn instead of the
	// icon) are an event badge's.
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       *int   `json:"color,omitempty"`
	Color2      *int   `json:"color2,omitempty"`
	Glyph       string `json:"glyph,omitempty"`
	Shape       string `json:"shape,omitempty"`
	Image       string `json:"image,omitempty"`
	// Hidden is set only in a person's own list.
	Hidden bool `json:"hidden,omitempty"`
}

// BadgeDef is a badge staff made for an event.
type BadgeDef struct {
	ID          ids.ID `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       int    `json:"color"`
	Color2      int    `json:"color2"` // -1 for one colour
	Glyph       string `json:"glyph"`  // "light" or "dark"
	Shape       string `json:"shape"`  // one of BadgeShapes
	ImageID     ids.ID `json:"image_id,omitempty"`
	CreatedBy   ids.ID `json:"created_by"`
	CreatedAt   int64  `json:"created_at"`
	Holders     int    `json:"holders"`
}

// BadgeShapes are the shapes an event badge can take.
var BadgeShapes = map[string]bool{"circle": true, "rounded": true, "shield": true, "hexagon": true, "star": true, "diamond": true, "seal": true, "ribbon": true}

// badge is how this badge sits on a profile.
func (d *BadgeDef) badge() Badge {
	color := d.Color
	b := Badge{ID: EventBadgePrefix + d.ID.String(), Name: d.Name, Description: d.Description, Icon: d.Icon, Color: &color}
	if d.Color2 >= 0 {
		c2 := d.Color2
		b.Color2 = &c2
	}
	if d.Glyph == "dark" {
		b.Glyph = "dark"
	}
	if d.Shape != "circle" && BadgeShapes[d.Shape] {
		b.Shape = d.Shape
	}
	if d.ImageID != 0 {
		b.Image = "/media/badge/" + d.ImageID.String()
	}
	return b
}

// OverridableBadges are the earned badges staff can give or take regardless of
// what the account earned, each with its highest level.
var OverridableBadges = map[string]int{BadgeOG: 1, BadgeYears: 50, BadgeBugHunter: 5, BadgeIdeas: 5, BadgeAppDeveloper: 1, BadgeGiftGiver: 3}

// Limits on what staff make and what a person's preferences hold.
const (
	MaxBadgeName   = 32
	MaxBadgeDesc   = 120
	maxBadgePrefs  = 128
	maxBadgeIDText = 40
)

var badgeIconRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidBadgeIcon reports whether an icon is shaped like a Lucide icon's name.
// Clients draw a plain award for a name they do not know.
func ValidBadgeIcon(icon string) bool { return len(icon) <= 48 && badgeIconRE.MatchString(icon) }

// IsGrantableBadge reports whether staff can give a badge id by hand.
func IsGrantableBadge(id string) bool {
	if id == BadgeContributor {
		return true
	}
	n, ok := strings.CutPrefix(id, EventBadgePrefix)
	if !ok {
		return false
	}
	_, err := strconv.ParseInt(n, 10, 64)
	return err == nil
}

// validBadgeID reports whether a preference can name a badge id at all.
func validBadgeID(id string) bool {
	if len(id) == 0 || len(id) > maxBadgeIDText {
		return false
	}
	for _, d := range defaultBadgeOrder {
		if id == d {
			return true
		}
	}
	return IsGrantableBadge(id)
}

// yearsSince counts the whole years from a time in milliseconds to now.
func yearsSince(createdMs int64, now time.Time) int {
	start := time.UnixMilli(createdMs).UTC()
	n := now.UTC().Year() - start.Year()
	if n > 0 && start.AddDate(n, 0, 0).After(now.UTC()) {
		n--
	}
	if n < 0 {
		return 0
	}
	return n
}

// earnedBadges works out every badge an account holds, in the default order.
func (s *Store) earnedBadges(ctx context.Context, u *User, now time.Time) ([]Badge, error) {
	if u == nil || u.Flags&(NotPeople|FlagBot|FlagDeleted) != 0 {
		return nil, nil
	}
	var out []Badge
	if u.Flags&StaffFlags != 0 {
		out = append(out, Badge{ID: BadgeStaff})
	}
	if u.Plan == "supporter" || u.Plan == "pro" {
		out = append(out, Badge{ID: BadgePlan})
	}
	levels, err := s.naturalLevels(ctx, u, now)
	if err != nil {
		return nil, err
	}
	over, err := s.BadgeOverrides(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for b, l := range over {
		levels[b] = l
	}
	add := func(id string, withLevel bool) {
		if l := levels[id]; l > 0 {
			b := Badge{ID: id}
			if withLevel {
				b.Level = l
			}
			out = append(out, b)
		}
	}
	add(BadgeOG, false)
	add(BadgeYears, true)
	granted, err := s.grantedBadges(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if granted[BadgeContributor] {
		out = append(out, Badge{ID: BadgeContributor})
	}
	add(BadgeBugHunter, true)
	add(BadgeIdeas, true)
	add(BadgeAppDeveloper, false)
	add(BadgeGiftGiver, true)
	// event badges, oldest given first
	var events []ids.ID
	for id := range granted {
		if n, ok := strings.CutPrefix(id, EventBadgePrefix); ok {
			if v, err := strconv.ParseInt(n, 10, 64); err == nil {
				events = append(events, ids.ID(v))
			}
		}
	}
	if len(events) > 0 {
		defs, err := s.badgeDefsByID(ctx, events)
		if err != nil {
			return nil, err
		}
		sort.Slice(events, func(i, j int) bool { return events[i] < events[j] })
		for _, id := range events {
			if d := defs[id]; d != nil {
				out = append(out, d.badge())
			}
		}
	}
	return out, nil
}

// naturalLevels is what an account earned of each overridable badge, before
// any override: 1 for a badge without levels, 0 for one it does not hold.
func (s *Store) naturalLevels(ctx context.Context, u *User, now time.Time) (map[string]int, error) {
	out := map[string]int{}
	if u.CreatedAt < OGCutoff {
		out[BadgeOG] = 1
	}
	if y := yearsSince(u.CreatedAt, now); y >= 1 {
		out[BadgeYears] = y
	}
	for _, k := range []struct{ kind, badge string }{{BugKindBug, BadgeBugHunter}, {BugKindSuggestion, BadgeIdeas}} {
		var n int
		if err := s.db.R.QueryRowContext(ctx, `SELECT count(*) FROM bug_reports WHERE user_id = ? AND status = ? AND kind = ?`, int64(u.ID), BugResolved, k.kind).Scan(&n); err != nil {
			return nil, err
		}
		if level := tierOf(n, ReportTiers[:]); level > 0 {
			out[k.badge] = level
		}
	}
	var servers int
	if err := s.db.R.QueryRowContext(ctx, `SELECT count(DISTINCT m.server_id) FROM applications a JOIN server_members m ON m.user_id = a.id JOIN servers v ON v.id = m.server_id WHERE a.owner_id = ? AND v.owner_id != ?`, int64(u.ID), int64(u.ID)).Scan(&servers); err != nil {
		return nil, err
	}
	if servers >= AppDeveloperServers {
		out[BadgeAppDeveloper] = 1
	}
	var gifts int
	if err := s.db.R.QueryRowContext(ctx, `SELECT count(*) FROM plan_gifts WHERE from_id = ? AND to_id != ? AND session_id NOT LIKE '`+ownerGiftClaim+`%'`, int64(u.ID), int64(u.ID)).Scan(&gifts); err != nil {
		return nil, err
	}
	if level := giftLevel(gifts); level > 0 {
		out[BadgeGiftGiver] = level
	}
	return out, nil
}

// EarnedLevels is what an account earned of each overridable badge, overrides
// left out, for the staff panel to show beside them.
func (s *Store) EarnedLevels(ctx context.Context, u *User) (map[string]int, error) {
	if u == nil || u.Flags&(NotPeople|FlagBot|FlagDeleted) != 0 {
		return map[string]int{}, nil
	}
	return s.naturalLevels(ctx, u, time.Now())
}

// BadgeOverrides is what staff set of an account's earned badges: 0 taken
// away, above 0 given at that level.
func (s *Store) BadgeOverrides(ctx context.Context, uid ids.ID) (map[string]int, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT badge, level FROM badge_overrides WHERE user_id = ?`, int64(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var b string
		var l int
		if err := rows.Scan(&b, &l); err != nil {
			return nil, err
		}
		out[b] = l
	}
	return out, rows.Err()
}

// BadgeChange is one change staff made to an account's badges, for the log.
type BadgeChange struct {
	Badge string `json:"badge"`
	// State is "give", "take" or "auto" (an override cleared).
	State string `json:"state"`
	Level int    `json:"level,omitempty"`
}

// ErrBadBadge is a badge that cannot be changed that way.
var ErrBadBadge = errors.New("bad badge")

// clampBadgeLevel holds a given level to what the badge has.
func clampBadgeLevel(badge string, level int) int {
	top := OverridableBadges[badge]
	if level < 1 {
		level = 1
	}
	if level > top {
		level = top
	}
	return level
}

func setOverrideTx(ctx context.Context, tx *db.Tx, uid, by ids.ID, badge string, level int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO badge_overrides (user_id, badge, level, set_by, set_at) VALUES (?,?,?,?,?)
		ON CONFLICT(user_id, badge) DO UPDATE SET level = excluded.level, set_by = excluded.set_by, set_at = excluded.set_at`, int64(uid), badge, level, int64(by), nowMs())
	return err
}

// SetBadgeOverrides makes the overrides on an account's earned badges exactly
// these (0 taken away, above 0 given at that level; a badge left out follows
// what the account earned), answering what changed.
func (s *Store) SetBadgeOverrides(ctx context.Context, uid, by ids.ID, want map[string]int) ([]BadgeChange, error) {
	for b := range want {
		if _, ok := OverridableBadges[b]; !ok {
			return nil, ErrBadBadge
		}
	}
	var changes []BadgeChange
	err := s.Tx(ctx, func(tx *db.Tx) error {
		changes = nil
		rows, err := tx.QueryContext(ctx, `SELECT badge, level FROM badge_overrides WHERE user_id = ?`, int64(uid))
		if err != nil {
			return err
		}
		have := map[string]int{}
		for rows.Next() {
			var b string
			var l int
			if err := rows.Scan(&b, &l); err != nil {
				rows.Close()
				return err
			}
			have[b] = l
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for b := range have {
			if _, keep := want[b]; !keep {
				if _, err := tx.ExecContext(ctx, `DELETE FROM badge_overrides WHERE user_id = ? AND badge = ?`, int64(uid), b); err != nil {
					return err
				}
				changes = append(changes, BadgeChange{Badge: b, State: "auto"})
			}
		}
		for b, l := range want {
			if l != 0 {
				l = clampBadgeLevel(b, l)
			}
			if old, ok := have[b]; ok && old == l {
				continue
			}
			if err := setOverrideTx(ctx, tx, uid, by, b, l); err != nil {
				return err
			}
			if l == 0 {
				changes = append(changes, BadgeChange{Badge: b, State: "take"})
			} else {
				changes = append(changes, BadgeChange{Badge: b, State: "give", Level: l})
			}
		}
		sort.Slice(changes, func(i, j int) bool { return changes[i].Badge < changes[j].Badge })
		return nil
	})
	return changes, err
}

// SetBadgeHolder gives ("give", at level for a badge with levels) or takes
// ("take") one badge of one account; "auto" lets an earned badge follow what
// the account earned again. It answers whether anything changed. Staff ranks
// and plans are not given this way (ErrBadBadge); an event badge that no
// longer exists is ErrNotFound.
func (s *Store) SetBadgeHolder(ctx context.Context, badge string, uid, by ids.ID, state string, level int) (bool, error) {
	changed := false
	switch {
	case IsGrantableBadge(badge):
		if state != "give" && state != "take" {
			return false, ErrBadBadge
		}
		err := s.Tx(ctx, func(tx *db.Tx) error {
			if n, ok := strings.CutPrefix(badge, EventBadgePrefix); ok && state == "give" {
				v, err := strconv.ParseInt(n, 10, 64)
				if err != nil {
					return ErrNotFound
				}
				var one int
				if err := tx.QueryRowContext(ctx, `SELECT 1 FROM badge_defs WHERE id = ?`, v).Scan(&one); err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return ErrNotFound
					}
					return err
				}
			}
			var res sql.Result
			var err error
			if state == "give" {
				res, err = tx.ExecContext(ctx, `INSERT INTO user_badges (user_id, badge, granted_by, granted_at) VALUES (?,?,?,?) ON CONFLICT DO NOTHING`, int64(uid), badge, int64(by), nowMs())
			} else {
				res, err = tx.ExecContext(ctx, `DELETE FROM user_badges WHERE user_id = ? AND badge = ?`, int64(uid), badge)
			}
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			changed = n > 0
			return nil
		})
		return changed, err
	case OverridableBadges[badge] > 0:
		err := s.Tx(ctx, func(tx *db.Tx) error {
			var old int
			err := tx.QueryRowContext(ctx, `SELECT level FROM badge_overrides WHERE user_id = ? AND badge = ?`, int64(uid), badge).Scan(&old)
			had := err == nil
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			switch state {
			case "auto":
				if !had {
					return nil
				}
				_, err = tx.ExecContext(ctx, `DELETE FROM badge_overrides WHERE user_id = ? AND badge = ?`, int64(uid), badge)
			case "give", "take":
				l := 0
				if state == "give" {
					l = clampBadgeLevel(badge, level)
				}
				if had && old == l {
					return nil
				}
				err = setOverrideTx(ctx, tx, uid, by, badge, l)
			default:
				return ErrBadBadge
			}
			changed = err == nil
			return err
		})
		return changed, err
	}
	return false, ErrBadBadge
}

// BadgeHolder is one account holding a badge, or one staff took it from.
type BadgeHolder struct {
	UserID ids.ID `json:"user_id"`
	Level  int    `json:"level,omitempty"`
	// Source is "earned", "given" (by hand, or an override giving an earned
	// badge) or, for a list of staff ranks and plans, "account".
	Source string `json:"source"`
	// Overridden marks an earned badge staff set by hand.
	Overridden bool `json:"overridden,omitempty"`
}

// BadgeHolders lists who holds a badge, ordered by account id, a page at a
// time, with the total; taken lists the accounts staff took an earned badge
// from (all of them: there are few).
func (s *Store) BadgeHolders(ctx context.Context, badge string, offset, limit int) (list, taken []BadgeHolder, total int, err error) {
	held := map[ids.ID]BadgeHolder{}
	people := ` AND u.flags & ? = 0`
	notPeople := int64(NotPeople | FlagBot | FlagDeleted)
	collect := func(q string, args ...any) error {
		rows, err := s.db.R.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				return err
			}
			held[ids.ID(id)] = BadgeHolder{UserID: ids.ID(id), Level: n, Source: "earned"}
		}
		return rows.Err()
	}
	asTier := func(tiers []int) {
		for id, h := range held {
			if l := tierOf(h.Level, tiers); l > 0 {
				h.Level = l
				held[id] = h
			} else {
				delete(held, id)
			}
		}
	}
	switch {
	case badge == BadgeStaff:
		err = collect(`SELECT u.id, 0 FROM users u WHERE u.flags & ? != 0`+people, int64(StaffFlags), notPeople)
	case badge == BadgePlan:
		err = collect(`SELECT u.id, 0 FROM users u WHERE u.plan IN ('supporter', 'pro')`+people, notPeople)
	case badge == BadgeOG:
		err = collect(`SELECT u.id, 0 FROM users u WHERE u.created_at < ?`+people, OGCutoff, notPeople)
	case badge == BadgeYears:
		cut := time.Now().AddDate(-1, 0, 0).UnixMilli()
		var rows *db.Rows
		rows, err = s.db.R.QueryContext(ctx, `SELECT u.id, u.created_at FROM users u WHERE u.created_at <= ?`+people, cut, notPeople)
		if err == nil {
			now := time.Now()
			for rows.Next() {
				var id, at int64
				if err = rows.Scan(&id, &at); err != nil {
					break
				}
				if y := yearsSince(at, now); y >= 1 {
					held[ids.ID(id)] = BadgeHolder{UserID: ids.ID(id), Level: y, Source: "earned"}
				}
			}
			rows.Close()
			if err == nil {
				err = rows.Err()
			}
		}
	case badge == BadgeBugHunter || badge == BadgeIdeas:
		kind := BugKindBug
		if badge == BadgeIdeas {
			kind = BugKindSuggestion
		}
		err = collect(`SELECT u.id, count(*) FROM bug_reports b JOIN users u ON u.id = b.user_id WHERE b.status = ? AND b.kind = ?`+people+` GROUP BY u.id`, BugResolved, kind, notPeople)
		asTier(ReportTiers[:])
	case badge == BadgeAppDeveloper:
		err = collect(`SELECT u.id, count(DISTINCT m.server_id) FROM applications a JOIN users u ON u.id = a.owner_id JOIN server_members m ON m.user_id = a.id JOIN servers v ON v.id = m.server_id WHERE v.owner_id != a.owner_id`+people+` GROUP BY u.id HAVING count(DISTINCT m.server_id) >= ?`, notPeople, AppDeveloperServers)
		for id, h := range held {
			h.Level = 0
			held[id] = h
		}
	case badge == BadgeGiftGiver:
		err = collect(`SELECT u.id, count(*) FROM plan_gifts g JOIN users u ON u.id = g.from_id WHERE g.to_id != g.from_id AND g.session_id NOT LIKE '`+ownerGiftClaim+`%'`+people+` GROUP BY u.id`, notPeople)
		asTier(GiftTiers[:])
	case IsGrantableBadge(badge):
		err = collect(`SELECT u.id, 0 FROM user_badges g JOIN users u ON u.id = g.user_id WHERE g.badge = ?`+people, badge, notPeople)
		for id, h := range held {
			h.Source = "given"
			held[id] = h
		}
	default:
		return nil, nil, 0, ErrBadBadge
	}
	if err != nil {
		return nil, nil, 0, err
	}
	if top := OverridableBadges[badge]; top > 0 {
		rows, err := s.db.R.QueryContext(ctx, `SELECT u.id, o.level FROM badge_overrides o JOIN users u ON u.id = o.user_id WHERE o.badge = ?`+people, badge, notPeople)
		if err != nil {
			return nil, nil, 0, err
		}
		for rows.Next() {
			var id int64
			var l int
			if err := rows.Scan(&id, &l); err != nil {
				rows.Close()
				return nil, nil, 0, err
			}
			uid := ids.ID(id)
			if l == 0 {
				delete(held, uid)
				taken = append(taken, BadgeHolder{UserID: uid, Source: "earned", Overridden: true})
				continue
			}
			if top == 1 {
				l = 0
			}
			held[uid] = BadgeHolder{UserID: uid, Level: l, Source: "given", Overridden: true}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, nil, 0, err
		}
	}
	all := make([]BadgeHolder, 0, len(held))
	for _, h := range held {
		all = append(all, h)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].UserID < all[j].UserID })
	sort.Slice(taken, func(i, j int) bool { return taken[i].UserID < taken[j].UserID })
	total = len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	if taken == nil {
		taken = []BadgeHolder{}
	}
	return all[offset:end], taken, total, nil
}

func giftLevel(gifts int) int { return tierOf(gifts, GiftTiers[:]) }

// tierOf is the highest tier whose need n meets, 0 for none.
func tierOf(n int, tiers []int) int {
	level := 0
	for i, need := range tiers {
		if n >= need {
			level = i + 1
		}
	}
	return level
}

// grantedBadges is the set of badges staff gave an account.
func (s *Store) grantedBadges(ctx context.Context, uid ids.ID) (map[string]bool, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT badge FROM user_badges WHERE user_id = ?`, int64(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		out[b] = true
	}
	return out, rows.Err()
}

// GrantedBadges lists the badges staff gave an account, sorted.
func (s *Store) GrantedBadges(ctx context.Context, uid ids.ID) ([]string, error) {
	set, err := s.grantedBadges(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for b := range set {
		out = append(out, b)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) badgeDefsByID(ctx context.Context, list []ids.ID) (map[ids.ID]*BadgeDef, error) {
	out := map[ids.ID]*BadgeDef{}
	if len(list) == 0 {
		return out, nil
	}
	args := make([]any, len(list))
	for i, id := range list {
		args[i] = int64(id)
	}
	rows, err := s.db.R.QueryContext(ctx, `SELECT id, name, description, icon, color, color2, glyph, shape, image_id, created_by, created_at FROM badge_defs WHERE id IN (?`+strings.Repeat(",?", len(list)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		d, err := scanBadgeDef(rows)
		if err != nil {
			return nil, err
		}
		out[d.ID] = d
	}
	return out, rows.Err()
}

func scanBadgeDef(sc interface{ Scan(...any) error }) (*BadgeDef, error) {
	var d BadgeDef
	var id, img, by int64
	if err := sc.Scan(&id, &d.Name, &d.Description, &d.Icon, &d.Color, &d.Color2, &d.Glyph, &d.Shape, &img, &by, &d.CreatedAt); err != nil {
		return nil, err
	}
	d.ID, d.ImageID, d.CreatedBy = ids.ID(id), ids.ID(img), ids.ID(by)
	return &d, nil
}

// BadgePrefs is the order someone put their badges in and the ones they hid.
type BadgePrefs struct {
	Order  []string `json:"order"`
	Hidden []string `json:"hidden"`
}

func (s *Store) badgePrefs(ctx context.Context, uid ids.ID) (BadgePrefs, error) {
	var shown, hidden string
	p := BadgePrefs{Order: []string{}, Hidden: []string{}}
	err := s.db.R.QueryRowContext(ctx, `SELECT shown, hidden FROM badge_prefs WHERE user_id = ?`, int64(uid)).Scan(&shown, &hidden)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal([]byte(shown), &p.Order)
	_ = json.Unmarshal([]byte(hidden), &p.Hidden)
	return p, nil
}

// arrange puts badges in their holder's order (the ones they placed first,
// the rest after in the default order) and marks the ones they hid. A staff
// rank is never hidden.
func arrange(list []Badge, p BadgePrefs) []Badge {
	at := map[string]int{}
	for i, id := range p.Order {
		if _, seen := at[id]; !seen {
			at[id] = i
		}
	}
	hidden := map[string]bool{}
	for _, id := range p.Hidden {
		hidden[id] = id != BadgeStaff
	}
	out := make([]Badge, len(list))
	copy(out, list)
	sort.SliceStable(out, func(i, j int) bool {
		a, aok := at[out[i].ID]
		b, bok := at[out[j].ID]
		switch {
		case aok && bok:
			return a < b
		case aok != bok:
			return aok
		}
		return false
	})
	for i := range out {
		out[i].Hidden = hidden[out[i].ID]
	}
	return out
}

// ProfileBadges is what everyone sees on an account's profile: its badges in
// its order, without the ones it hid.
func (s *Store) ProfileBadges(ctx context.Context, u *User) ([]Badge, error) {
	list, err := s.MyBadges(ctx, u)
	if err != nil {
		return nil, err
	}
	out := []Badge{}
	for _, b := range list {
		if !b.Hidden {
			out = append(out, b)
		}
	}
	return out, nil
}

// MyBadges is an account's own view of its badges: every one, in its order,
// the hidden ones marked.
func (s *Store) MyBadges(ctx context.Context, u *User) ([]Badge, error) {
	list, err := s.earnedBadges(ctx, u, time.Now())
	if err != nil || len(list) == 0 {
		return []Badge{}, err
	}
	p, err := s.badgePrefs(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return arrange(list, p), nil
}

// SetBadgePrefs keeps the order and hidden set someone chose. Unknown ids are
// dropped; ids of badges they do not hold yet are kept, so a badge staff take
// away and give back keeps its place.
func (s *Store) SetBadgePrefs(ctx context.Context, uid ids.ID, p BadgePrefs) error {
	clean := func(list []string) []string {
		out, seen := []string{}, map[string]bool{}
		for _, id := range list {
			if validBadgeID(id) && !seen[id] && len(out) < maxBadgePrefs {
				seen[id] = true
				out = append(out, id)
			}
		}
		return out
	}
	order, err := json.Marshal(clean(p.Order))
	if err != nil {
		return err
	}
	hidden := []string{}
	for _, id := range clean(p.Hidden) {
		if id != BadgeStaff {
			hidden = append(hidden, id)
		}
	}
	hid, err := json.Marshal(hidden)
	if err != nil {
		return err
	}
	return s.Tx(ctx, func(tx *db.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM badge_prefs WHERE user_id = ?`, int64(uid)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO badge_prefs (user_id, shown, hidden) VALUES (?, ?, ?)`, int64(uid), string(order), string(hid))
		return err
	})
}

// --- what staff do ----------------------------------------------------------

// BadgeDefs lists the event badges, newest first, with how many hold each.
func (s *Store) BadgeDefs(ctx context.Context) ([]*BadgeDef, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT id, name, description, icon, color, color2, glyph, shape, image_id, created_by, created_at FROM badge_defs ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	out := []*BadgeDef{}
	for rows.Next() {
		d, err := scanBadgeDef(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	counts, err := s.badgeHolders(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range out {
		d.Holders = counts[EventBadgePrefix+d.ID.String()]
	}
	return out, nil
}

// badgeHolders counts the accounts holding each badge staff give.
func (s *Store) badgeHolders(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT badge, count(*) FROM user_badges GROUP BY badge`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var b string
		var n int
		if err := rows.Scan(&b, &n); err != nil {
			return nil, err
		}
		out[b] = n
	}
	return out, rows.Err()
}

// ContributorCount is how many accounts hold Contributor.
func (s *Store) ContributorCount(ctx context.Context) (int, error) {
	counts, err := s.badgeHolders(ctx)
	return counts[BadgeContributor], err
}

// BadgeDef returns one event badge.
func (s *Store) BadgeDef(ctx context.Context, id ids.ID) (*BadgeDef, error) {
	d, err := scanBadgeDef(s.db.R.QueryRowContext(ctx, `SELECT id, name, description, icon, color, color2, glyph, shape, image_id, created_by, created_at FROM badge_defs WHERE id = ?`, int64(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// CreateBadgeDef makes an event badge.
func (s *Store) CreateBadgeDef(ctx context.Context, d *BadgeDef) error {
	d.ID, d.CreatedAt = ids.New(), nowMs()
	_, err := s.db.W.ExecContext(ctx, `INSERT INTO badge_defs (id, name, description, icon, color, color2, glyph, shape, image_id, created_by, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		int64(d.ID), d.Name, d.Description, d.Icon, d.Color, d.Color2, d.Glyph, d.Shape, int64(d.ImageID), int64(d.CreatedBy), d.CreatedAt)
	return err
}

// UpdateBadgeDef changes how an event badge looks and what it says.
func (s *Store) UpdateBadgeDef(ctx context.Context, d *BadgeDef) error {
	res, err := s.db.W.ExecContext(ctx, `UPDATE badge_defs SET name = ?, description = ?, icon = ?, color = ?, color2 = ?, glyph = ?, shape = ?, image_id = ? WHERE id = ?`,
		d.Name, d.Description, d.Icon, d.Color, d.Color2, d.Glyph, d.Shape, int64(d.ImageID), int64(d.ID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBadgeDef deletes an event badge and takes it from everyone holding it.
func (s *Store) DeleteBadgeDef(ctx context.Context, id ids.ID) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM badge_defs WHERE id = ?`, int64(id))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM user_badges WHERE badge = ?`, EventBadgePrefix+id.String())
		return err
	})
}

// SetGrantedBadges makes the badges staff gave an account exactly these,
// answering what was added and what was taken. Event badges that no longer
// exist are refused with ErrNotFound.
func (s *Store) SetGrantedBadges(ctx context.Context, uid, by ids.ID, want []string) (added, removed []string, err error) {
	err = s.Tx(ctx, func(tx *db.Tx) error {
		added, removed = nil, nil
		wanted := map[string]bool{}
		for _, b := range want {
			if n, ok := strings.CutPrefix(b, EventBadgePrefix); ok {
				v, err := strconv.ParseInt(n, 10, 64)
				if err != nil {
					return ErrNotFound
				}
				var one int
				if err := tx.QueryRowContext(ctx, `SELECT 1 FROM badge_defs WHERE id = ?`, v).Scan(&one); err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return ErrNotFound
					}
					return err
				}
			}
			wanted[b] = true
		}
		rows, err := tx.QueryContext(ctx, `SELECT badge FROM user_badges WHERE user_id = ?`, int64(uid))
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				rows.Close()
				return err
			}
			have[b] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for b := range have {
			if !wanted[b] {
				if _, err := tx.ExecContext(ctx, `DELETE FROM user_badges WHERE user_id = ? AND badge = ?`, int64(uid), b); err != nil {
					return err
				}
				removed = append(removed, b)
			}
		}
		now := nowMs()
		for b := range wanted {
			if !have[b] {
				if _, err := tx.ExecContext(ctx, `INSERT INTO user_badges (user_id, badge, granted_by, granted_at) VALUES (?,?,?,?)`, int64(uid), b, int64(by), now); err != nil {
					return err
				}
				added = append(added, b)
			}
		}
		sort.Strings(added)
		sort.Strings(removed)
		return nil
	})
	return added, removed, err
}
