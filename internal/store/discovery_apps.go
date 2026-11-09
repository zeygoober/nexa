package store

import (
	"context"
	"database/sql"
	"strings"

	"evilion.xyz/chat/internal/ids"
)

// ListedApp is an app as Discovery's Bots part shows it.
type ListedApp struct {
	ID          ids.ID   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IconID      *ids.ID  `json:"icon_id"`
	AvatarID    *ids.ID  `json:"avatar_id"`
	Tags        []string `json:"tags"`
	Servers     int      `json:"servers"`
	// ReadsAll: it has the Message Content intent, so it reads every message it can see.
	ReadsAll bool `json:"reads_all"`
}

// AppDiscoveryQuery is what the Bots part of Discovery asks for.
type AppDiscoveryQuery struct {
	Query  string
	Sort   string // "popular" (the default) or "newest"
	Offset int
	Limit  int
	Now    int64
}

// DiscoverApps lists the apps on Discovery: their developer listed them,
// they can be added by anyone, staff left them there, and neither the bot nor
// its owner has been banned, disabled or deleted.
func (s *Store) DiscoverApps(ctx context.Context, q AppDiscoveryQuery) ([]*ListedApp, error) {
	if q.Offset < 0 {
		q.Offset = 0
	}
	gone := int64(FlagDisabled | FlagDeleted)
	banned := `EXISTS (SELECT 1 FROM mod_restrictions r WHERE r.user_id = %s AND r.kind = 'ban' AND (r.expires_at IS NULL OR r.expires_at > ?))`
	where := []string{`a.discoverable = 1`, `a.discovery_hidden = 0`, `a.bot_public = 1`, `b.flags & ? = 0`, `o.flags & ? = 0`,
		`NOT ` + strings.Replace(banned, "%s", "a.id", 1), `NOT ` + strings.Replace(banned, "%s", "a.owner_id", 1)}
	args := []any{gone, gone, q.Now, q.Now}
	if term := strings.ToLower(strings.TrimSpace(q.Query)); term != "" {
		if len(term) > maxSearchQuery {
			term = term[:maxSearchQuery]
		}
		like := "%" + escapeLike(term) + "%"
		where = append(where, `(`+s.db.Dialect.Lower("a.name")+` LIKE ? ESCAPE '\' OR `+s.db.Dialect.Lower("a.description")+` LIKE ? ESCAPE '\' OR `+s.db.Dialect.Lower("a.tags")+` LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	order := `servers DESC, a.id DESC`
	if q.Sort == "newest" {
		order = `a.id DESC`
	}
	args = append(args, q.Limit, q.Offset)
	rows, err := s.db.R.QueryContext(ctx, `SELECT a.id, a.name, a.description, a.icon_id, b.avatar_id, a.tags, b.flags,
		(SELECT count(*) FROM application_guild_installs g WHERE g.application_id = a.id) AS servers
		FROM applications a JOIN users b ON b.id = a.id JOIN users o ON o.id = a.owner_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY `+order+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ListedApp{}
	for rows.Next() {
		var a ListedApp
		var icon, avatar sql.NullInt64
		var tags string
		var flags int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &icon, &avatar, &tags, &flags, &a.Servers); err != nil {
			return nil, err
		}
		a.IconID, a.AvatarID, a.Tags = idPtr(icon), idPtr(avatar), jsonStrings(tags)
		a.ReadsAll = flags&int64(FlagBotReadsAll) != 0
		out = append(out, &a)
	}
	return out, rows.Err()
}
