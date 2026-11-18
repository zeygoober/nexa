package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/perms"
)

// ServerTemplateCodeLen is how long a Nexa template code is.
const ServerTemplateCodeLen = 12

// ServerTemplate is a server's template link: a snapshot of its channels,
// roles and permissions that anyone with the code can make a new server from.
type ServerTemplate struct {
	Code        string
	ServerID    ids.ID
	Name        string
	Description string
	Spec        TemplateSpec
	UsageCount  int
	CreatedAt   int64
	SyncedAt    int64
}

const serverTemplateCols = `code, server_id, name, description, spec, usage_count, created_at, synced_at`

func scanServerTemplate(row interface{ Scan(...any) error }) (*ServerTemplate, error) {
	var t ServerTemplate
	var sid int64
	var spec string
	if err := row.Scan(&t.Code, &sid, &t.Name, &t.Description, &spec, &t.UsageCount, &t.CreatedAt, &t.SyncedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.ServerID = ids.ID(sid)
	if err := json.Unmarshal([]byte(spec), &t.Spec); err != nil {
		return nil, err
	}
	return &t, nil
}

// ServerTemplateOf is a server's template, or ErrNotFound.
func (s *Store) ServerTemplateOf(ctx context.Context, serverID ids.ID) (*ServerTemplate, error) {
	return scanServerTemplate(s.db.R.QueryRowContext(ctx, `SELECT `+serverTemplateCols+` FROM server_templates WHERE server_id = ?`, int64(serverID)))
}

// ServerTemplateByCode is the template a code names, or ErrNotFound.
func (s *Store) ServerTemplateByCode(ctx context.Context, code string) (*ServerTemplate, error) {
	return scanServerTemplate(s.db.R.QueryRowContext(ctx, `SELECT `+serverTemplateCols+` FROM server_templates WHERE code = ?`, code))
}

// SnapshotServer is a server as a template: its custom roles (never a bot's
// own), its categories and channels with their role overwrites (never one for
// a single member), and the settings a new server starts with. Messages,
// members, emoji, stickers and pictures are not part of it, and neither is a
// channel glyph or forum emoji that names this server's own emoji.
func (s *Store) SnapshotServer(ctx context.Context, serverID ids.ID) (TemplateSpec, error) {
	srv, err := s.ServerByID(ctx, serverID)
	if err != nil {
		return TemplateSpec{}, err
	}
	roles, err := s.Roles(ctx, serverID)
	if err != nil {
		return TemplateSpec{}, err
	}
	chans, err := s.ServerChannels(ctx, serverID)
	if err != nil {
		return TemplateSpec{}, err
	}
	ows, err := s.OverwritesForServer(ctx, serverID)
	if err != nil {
		return TemplateSpec{}, err
	}
	spec := TemplateSpec{
		Name:                 srv.Name,
		Description:          srv.Description,
		DefaultNotifications: srv.DefaultNotifications,
		EveryonePerms:        perms.DefaultEveryone,
		Roles:                []TemplateRole{},
		Channels:             []TemplateChannel{},
		SystemChannelIndex:   -1,
	}
	roleIdx := map[ids.ID]int{serverID: -1}
	for _, r := range roles {
		if r.ID == serverID {
			spec.EveryonePerms = r.Permissions & perms.All
			continue
		}
		if r.ManagedBy != nil || len(spec.Roles) >= MaxServerRoles-1 {
			continue
		}
		roleIdx[r.ID] = len(spec.Roles)
		spec.Roles = append(spec.Roles, TemplateRole{
			Name: r.Name, Color: r.Color, Color2: r.Color2, Color3: r.Color3,
			Hoist: r.Hoist, Mentionable: r.Mentionable, Permissions: r.Permissions & perms.All, IconEmoji: r.IconEmoji,
		})
	}
	own := func(emoji string) string {
		// a custom emoji is this server's, which the new one will not have
		if strings.Contains(emoji, ":") {
			return ""
		}
		return emoji
	}
	convert := func(list []Overwrite) []TemplateOverwrite {
		out := []TemplateOverwrite{}
		for _, ow := range list {
			idx, ok := roleIdx[ow.TargetID]
			if ow.TargetType != "role" || !ok {
				continue
			}
			out = append(out, TemplateOverwrite{RoleIndex: idx, Allow: ow.Allow & perms.All, Deny: ow.Deny & perms.All})
		}
		return out
	}
	chanIdx := map[ids.ID]int{}
	add := func(c *Channel) {
		if len(spec.Channels) >= MaxServerChannels {
			return
		}
		tc := TemplateChannel{
			Type: c.Type, Name: c.Name, Topic: c.Topic, Position: c.Position, ParentIndex: -1,
			NSFW: c.NSFW, SlowmodeSeconds: c.SlowmodeSeconds, UserLimit: c.UserLimit,
			DefaultAutoArchive: c.DefaultAutoArchive, URL: c.URL, Overwrites: convert(ows[c.ID]),
		}
		if !strings.HasPrefix(c.Glyph, "custom:") {
			tc.Glyph = c.Glyph
		}
		if c.ParentID != nil {
			if idx, ok := chanIdx[*c.ParentID]; ok {
				tc.ParentIndex = idx
			}
		}
		if c.Forum != nil {
			f := *c.Forum
			f.PinnedPost = nil
			f.DefaultReaction = own(f.DefaultReaction)
			f.Tags = make([]ForumTag, 0, len(c.Forum.Tags))
			for _, t := range c.Forum.Tags {
				f.Tags = append(f.Tags, ForumTag{Name: t.Name, Emoji: own(t.Emoji), Moderated: t.Moderated})
			}
			tc.Forum = &f
		}
		chanIdx[c.ID] = len(spec.Channels)
		spec.Channels = append(spec.Channels, tc)
	}
	// categories first, so every channel finds its parent's index
	for _, c := range chans {
		if c.Type == "category" {
			add(c)
		}
	}
	for _, c := range chans {
		if c.Type != "category" {
			add(c)
		}
	}
	if srv.SystemChannelID != nil {
		if idx, ok := chanIdx[*srv.SystemChannelID]; ok {
			spec.SystemChannelIndex = idx
		}
	}
	return spec, nil
}

// CreateServerTemplate takes a server's snapshot under a new code. A server
// has one template, so a second is ErrConflict.
func (s *Store) CreateServerTemplate(ctx context.Context, serverID ids.ID, name, description string) (*ServerTemplate, error) {
	spec, err := s.SnapshotServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	now := nowMs()
	t := &ServerTemplate{Code: randomCode(ServerTemplateCodeLen), ServerID: serverID, Name: name, Description: description, Spec: spec, CreatedAt: now, SyncedAt: now}
	err = s.Tx(ctx, func(tx *db.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM server_templates WHERE server_id = ?`, int64(serverID)).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO server_templates (`+serverTemplateCols+`) VALUES (?,?,?,?,?,0,?,?)`,
			t.Code, int64(serverID), name, description, string(raw), now, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateServerTemplate renames a server's template, describes it again, or
// (sync) takes the server's snapshot again under the same code.
func (s *Store) UpdateServerTemplate(ctx context.Context, serverID ids.ID, name, description *string, sync bool) (*ServerTemplate, error) {
	var sets []string
	var args []any
	if name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *name)
	}
	if description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *description)
	}
	if sync {
		spec, err := s.SnapshotServer(ctx, serverID)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "spec = ?", "synced_at = ?")
		args = append(args, string(raw), nowMs())
	}
	if len(sets) > 0 {
		args = append(args, int64(serverID))
		res, err := s.db.W.ExecContext(ctx, `UPDATE server_templates SET `+strings.Join(sets, ", ")+` WHERE server_id = ?`, args...)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
	}
	return s.ServerTemplateOf(ctx, serverID)
}

// DeleteServerTemplate removes a server's template, so its link stops working.
func (s *Store) DeleteServerTemplate(ctx context.Context, serverID ids.ID) error {
	res, err := s.db.W.ExecContext(ctx, `DELETE FROM server_templates WHERE server_id = ?`, int64(serverID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountServerTemplateUse counts one more server made from a template.
func (s *Store) CountServerTemplateUse(ctx context.Context, code string) error {
	_, err := s.db.W.ExecContext(ctx, `UPDATE server_templates SET usage_count = usage_count + 1 WHERE code = ?`, code)
	return err
}
