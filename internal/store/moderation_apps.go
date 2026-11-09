package store

import (
	"context"
	"strings"

	"evilion.xyz/chat/internal/ids"
)

// ModApp is an app as the staff's Bots tab lists it: who owns it, where it
// is added, and what its bot may read.
type ModApp struct {
	ID          ids.ID
	OwnerID     ids.ID
	Name        string
	Description string
	IconID      *ids.ID
	// Flags are the app's (its privileged intents); BotFlags its bot account's.
	Flags    int64
	BotFlags int64
	Public   bool
	// Discoverable: its developer put it on Discovery; DiscoveryHidden: staff took it off.
	Discoverable, DiscoveryHidden bool
	// Banned: its bot is under a ban in effect, so its token is refused.
	Banned    bool
	Servers   int
	Accounts  int
	CreatedAt int64
}

// ModAppFilter narrows and orders the Bots tab.
type ModAppFilter struct {
	// Query matches an app's name or description, its bot's username, or its owner's username.
	Query string
	// Sort is "newest" (the default), "oldest", "servers" or "name".
	Sort   string
	Offset int
	Limit  int
	Now    int64
}

// ModApps pages every app on the instance for the staff.
func (s *Store) ModApps(ctx context.Context, f ModAppFilter) ([]*ModApp, error) {
	if f.Limit <= 0 || f.Limit > 121 {
		f.Limit = 60
	}
	var where []string
	args := []any{f.Now}
	if q := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(f.Query), "@")); q != "" {
		if len(q) > maxSearchQuery {
			q = q[:maxSearchQuery]
		}
		like := "%" + escapeLike(strings.ToLower(q)) + "%"
		cond := s.db.Dialect.Lower("a.name") + ` LIKE ? ESCAPE '\' OR ` + s.db.Dialect.Lower("a.description") + ` LIKE ? ESCAPE '\' OR b.username LIKE ? ESCAPE '\' OR o.username LIKE ? ESCAPE '\'`
		args = append(args, like, like, like, like)
		if id, err := ids.Parse(q); err == nil {
			cond += ` OR a.id = ? OR a.owner_id = ?`
			args = append(args, int64(id), int64(id))
		}
		where = append(where, `(`+cond+`)`)
	}
	order := `a.id DESC`
	switch f.Sort {
	case "oldest":
		order = `a.id ASC`
	case "servers":
		order = `servers DESC, a.id DESC`
	case "name":
		order = s.db.Dialect.Lower("a.name") + ` ASC, a.id ASC`
	}
	q := `SELECT a.id, a.owner_id, a.name, a.description, a.icon_id, a.flags, b.flags, a.bot_public, a.created_at, a.discoverable, a.discovery_hidden,
		(SELECT count(*) FROM application_guild_installs g WHERE g.application_id = a.id) AS servers,
		(SELECT count(*) FROM application_user_installs u WHERE u.application_id = a.id),
		CASE WHEN EXISTS (SELECT 1 FROM mod_restrictions r WHERE r.user_id = a.id AND r.kind = 'ban' AND (r.expires_at IS NULL OR r.expires_at > ?)) THEN 1 ELSE 0 END
		FROM applications a JOIN users b ON b.id = a.id JOIN users o ON o.id = a.owner_id`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, f.Limit, f.Offset)
	rows, err := s.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ModApp
	for rows.Next() {
		var m ModApp
		var icon *int64
		var public, banned, listed, hidden int64
		if err := rows.Scan(&m.ID, &m.OwnerID, &m.Name, &m.Description, &icon, &m.Flags, &m.BotFlags, &public, &m.CreatedAt, &listed, &hidden, &m.Servers, &m.Accounts, &banned); err != nil {
			return nil, err
		}
		if icon != nil {
			id := ids.ID(*icon)
			m.IconID = &id
		}
		m.Public, m.Banned = public != 0, banned != 0
		m.Discoverable, m.DiscoveryHidden = listed != 0, hidden != 0
		out = append(out, &m)
	}
	return out, rows.Err()
}

// CountApps is how many apps there are.
func (s *Store) CountApps(ctx context.Context) (int, error) {
	var n int
	err := s.db.R.QueryRowContext(ctx, `SELECT count(*) FROM applications`).Scan(&n)
	return n, err
}
