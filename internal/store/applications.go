package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// MaxApplicationsPerOwner is how many applications one account may own.
const MaxApplicationsPerOwner = 25

// InstallParams are the scopes and permissions an install asks for by
// default; Permissions is a decimal string of permission bits.
type InstallParams struct {
	Scopes      []string `json:"scopes"`
	Permissions string   `json:"permissions"`
}

// IntegrationTypeConfig is one way an application can be installed (to a
// server, "0", or to an account, "1") and what that asks for by default.
type IntegrationTypeConfig struct {
	OAuth2InstallParams *InstallParams `json:"oauth2_install_params,omitempty"`
}

// Application is a program with a bot, made in Settings, Developer Portal.
// Its id is its bot's account id.
type Application struct {
	ID                  ids.ID
	OwnerID             ids.ID
	Name                string
	Description         string
	IconID              *ids.ID
	CoverID             *ids.ID
	Tags                []string
	TermsURL            string
	PrivacyURL          string
	InteractionsURL     string
	RoleConnectionsURL  string
	CustomInstallURL    string
	InstallParams       *InstallParams
	IntegrationTypes    map[string]IntegrationTypeConfig
	RedirectURIs        []string
	Flags               int64
	BotPublic           bool
	Discoverable        bool `json:"-"`
	DiscoveryHidden     bool `json:"-"`
	BotRequireCodeGrant bool
	TokenHash           string
	SecretHash          string
	VerifyKey           string
	SignKeySealed       []byte
	BotKeySealed        []byte
	EventWebhooksURL    string
	EventWebhooksStatus int
	EventWebhooksTypes  []string
	CreatedAt           int64
	UpdatedAt           int64
}

const applicationCols = `id, owner_id, name, description, icon_id, cover_id, tags, terms_url, privacy_url, interactions_url, role_connections_url, custom_install_url, install_params, integration_types, redirect_uris, flags, bot_public, bot_require_code_grant, token_hash, secret_hash, verify_key, sign_key_sealed, bot_key_sealed, event_webhooks_url, event_webhooks_status, event_webhooks_types, created_at, updated_at, discoverable, discovery_hidden`

func scanApplication(r rowScanner) (*Application, error) {
	var a Application
	var icon, cover sql.NullInt64
	var tags, integration, redirects, eventTypes string
	var install sql.NullString
	var public, codeGrant, discoverable, discoveryHidden int64
	if err := r.Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &icon, &cover, &tags, &a.TermsURL, &a.PrivacyURL, &a.InteractionsURL, &a.RoleConnectionsURL,
		&a.CustomInstallURL, &install, &integration, &redirects, &a.Flags, &public, &codeGrant, &a.TokenHash, &a.SecretHash, &a.VerifyKey,
		&a.SignKeySealed, &a.BotKeySealed, &a.EventWebhooksURL, &a.EventWebhooksStatus, &eventTypes, &a.CreatedAt, &a.UpdatedAt, &discoverable, &discoveryHidden); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.IconID, a.CoverID = idPtr(icon), idPtr(cover)
	a.BotPublic, a.BotRequireCodeGrant = public == 1, codeGrant == 1
	a.Discoverable, a.DiscoveryHidden = discoverable == 1, discoveryHidden == 1
	a.Tags = jsonStrings(tags)
	a.RedirectURIs = jsonStrings(redirects)
	a.EventWebhooksTypes = jsonStrings(eventTypes)
	if install.Valid && install.String != "" {
		var p InstallParams
		if json.Unmarshal([]byte(install.String), &p) == nil {
			a.InstallParams = &p
		}
	}
	a.IntegrationTypes = map[string]IntegrationTypeConfig{}
	_ = json.Unmarshal([]byte(integration), &a.IntegrationTypes)
	return &a, nil
}

func jsonStrings(s string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func stringsJSON(list []string) string {
	if list == nil {
		list = []string{}
	}
	b, _ := json.Marshal(list)
	return string(b)
}

func (s *Store) queryApplications(ctx context.Context, q string, args ...any) ([]*Application, error) {
	rows, err := s.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Application{}
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateApplication stores an application with its bot account in one go,
// refusing with ErrTooMany once the owner has as many as they may, and with
// ErrUsernameTaken when the bot's username is someone else's.
func (s *Store) CreateApplication(ctx context.Context, a *Application, bot *User) error {
	now := nowMs()
	if bot.ID.IsZero() {
		bot.ID = ids.New()
	}
	bot.CreatedAt, bot.UpdatedAt = now, now
	if bot.PresencePref == "" {
		bot.PresencePref = "online"
	}
	if bot.DMPrivacy == "" {
		bot.DMPrivacy = DMEveryone
	}
	a.ID, a.CreatedAt, a.UpdatedAt = bot.ID, now, now
	if a.IntegrationTypes == nil {
		a.IntegrationTypes = map[string]IntegrationTypeConfig{"0": {}}
	}
	integration, err := json.Marshal(a.IntegrationTypes)
	if err != nil {
		return err
	}
	var install any
	if a.InstallParams != nil {
		b, err := json.Marshal(a.InstallParams)
		if err != nil {
			return err
		}
		install = string(b)
	}
	return s.Tx(ctx, func(tx *db.Tx) error {
		if err := tx.Serialise(ctx, "applications", int64(a.OwnerID)); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM applications WHERE owner_id = ?`, int64(a.OwnerID)).Scan(&n); err != nil {
			return err
		}
		if n >= MaxApplicationsPerOwner {
			return ErrTooMany
		}
		if held, err := usernameHeldTx(ctx, tx, bot.Username); err != nil || held {
			if err == nil {
				err = ErrUsernameTaken
			}
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO users (`+userCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0) ON CONFLICT(username) DO NOTHING`,
			int64(bot.ID), bot.Username, bot.DisplayName, bot.AuthHash, bot.KDFSalt, bot.KDFParams, bot.PublicKey, bot.EncPrivateKey, nil, nil,
			nullID(bot.AvatarID), nullID(bot.BannerID), bot.AboutMe, "", nil, nil, "", "", nil, bot.PresencePref, bot.DMPrivacy, bot.Flags, bot.CreatedAt, bot.UpdatedAt, 0,
			"", nil, "", "", nil, "", "")
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrUsernameTaken
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO applications (`+applicationCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			int64(a.ID), int64(a.OwnerID), a.Name, a.Description, nullID(a.IconID), nullID(a.CoverID), stringsJSON(a.Tags), a.TermsURL, a.PrivacyURL,
			a.InteractionsURL, a.RoleConnectionsURL, a.CustomInstallURL, install, string(integration), stringsJSON(a.RedirectURIs), a.Flags,
			boolInt(a.BotPublic), boolInt(a.BotRequireCodeGrant), a.TokenHash, a.SecretHash, a.VerifyKey, a.SignKeySealed, a.BotKeySealed,
			a.EventWebhooksURL, a.EventWebhooksStatus, stringsJSON(a.EventWebhooksTypes), a.CreatedAt, a.UpdatedAt, boolInt(a.Discoverable), boolInt(a.DiscoveryHidden))
		return err
	})
}

// ApplicationByID loads one application.
func (s *Store) ApplicationByID(ctx context.Context, id ids.ID) (*Application, error) {
	return scanApplication(s.db.R.QueryRowContext(ctx, `SELECT `+applicationCols+` FROM applications WHERE id = ?`, int64(id)))
}

// ApplicationByToken finds the application whose bot token hashes to hash.
func (s *Store) ApplicationByToken(ctx context.Context, hash string) (*Application, error) {
	if hash == "" {
		return nil, ErrNotFound
	}
	return scanApplication(s.db.R.QueryRowContext(ctx, `SELECT `+applicationCols+` FROM applications WHERE token_hash = ?`, hash))
}

// ApplicationsByOwner lists an account's applications, oldest first.
func (s *Store) ApplicationsByOwner(ctx context.Context, owner ids.ID) ([]*Application, error) {
	return s.queryApplications(ctx, `SELECT `+applicationCols+` FROM applications WHERE owner_id = ? ORDER BY id`, int64(owner))
}

// ApplicationsByIDs loads the listed applications that exist.
func (s *Store) ApplicationsByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*Application, error) {
	out := map[ids.ID]*Application{}
	err := chunked(list, func(part []ids.ID) error {
		apps, err := s.queryApplications(ctx, `SELECT `+applicationCols+` FROM applications WHERE id IN (`+placeholders(len(part))+`)`, idArgs(part)...)
		if err != nil {
			return err
		}
		for _, a := range apps {
			out[a.ID] = a
		}
		return nil
	})
	return out, err
}

// ApplicationPatch is an optional set of application changes. For the double
// pointers a pointer to nil clears the value.
type ApplicationPatch struct {
	Name                *string
	Description         *string
	IconID              **ids.ID
	CoverID             **ids.ID
	Tags                *[]string
	TermsURL            *string
	PrivacyURL          *string
	InteractionsURL     *string
	RoleConnectionsURL  *string
	CustomInstallURL    *string
	InstallParams       **InstallParams
	IntegrationTypes    *map[string]IntegrationTypeConfig
	RedirectURIs        *[]string
	BotPublic           *bool
	BotRequireCodeGrant *bool
	Discoverable        *bool
	DiscoveryHidden     *bool
	EventWebhooksURL    *string
	EventWebhooksStatus *int
	EventWebhooksTypes  *[]string
}

// UpdateApplication applies p. Pictures are swapped with SwapImage, which
// keeps their references right; p's IconID and CoverID are for callers that
// already did so.
func (s *Store) UpdateApplication(ctx context.Context, id ids.ID, p ApplicationPatch) error {
	var sets []string
	var args []any
	set := func(col string, v any) { sets = append(sets, col+" = ?"); args = append(args, v) }
	if p.Name != nil {
		set("name", *p.Name)
	}
	if p.Discoverable != nil {
		set("discoverable", boolInt(*p.Discoverable))
	}
	if p.DiscoveryHidden != nil {
		set("discovery_hidden", boolInt(*p.DiscoveryHidden))
	}
	if p.Description != nil {
		set("description", *p.Description)
	}
	if p.IconID != nil {
		set("icon_id", nullID(*p.IconID))
	}
	if p.CoverID != nil {
		set("cover_id", nullID(*p.CoverID))
	}
	if p.Tags != nil {
		set("tags", stringsJSON(*p.Tags))
	}
	if p.TermsURL != nil {
		set("terms_url", *p.TermsURL)
	}
	if p.PrivacyURL != nil {
		set("privacy_url", *p.PrivacyURL)
	}
	if p.InteractionsURL != nil {
		set("interactions_url", *p.InteractionsURL)
	}
	if p.RoleConnectionsURL != nil {
		set("role_connections_url", *p.RoleConnectionsURL)
	}
	if p.CustomInstallURL != nil {
		set("custom_install_url", *p.CustomInstallURL)
	}
	if p.InstallParams != nil {
		if *p.InstallParams == nil {
			set("install_params", nil)
		} else {
			b, err := json.Marshal(*p.InstallParams)
			if err != nil {
				return err
			}
			set("install_params", string(b))
		}
	}
	if p.IntegrationTypes != nil {
		b, err := json.Marshal(*p.IntegrationTypes)
		if err != nil {
			return err
		}
		set("integration_types", string(b))
	}
	if p.RedirectURIs != nil {
		set("redirect_uris", stringsJSON(*p.RedirectURIs))
	}
	if p.BotPublic != nil {
		set("bot_public", boolInt(*p.BotPublic))
	}
	if p.BotRequireCodeGrant != nil {
		set("bot_require_code_grant", boolInt(*p.BotRequireCodeGrant))
	}
	if p.EventWebhooksURL != nil {
		set("event_webhooks_url", *p.EventWebhooksURL)
	}
	if p.EventWebhooksStatus != nil {
		set("event_webhooks_status", *p.EventWebhooksStatus)
	}
	if p.EventWebhooksTypes != nil {
		set("event_webhooks_types", stringsJSON(*p.EventWebhooksTypes))
	}
	if len(sets) == 0 {
		return nil
	}
	set("updated_at", nowMs())
	args = append(args, int64(id))
	res, err := s.db.W.ExecContext(ctx, `UPDATE applications SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetApplicationToken replaces the bot's token (its hash).
func (s *Store) SetApplicationToken(ctx context.Context, id ids.ID, hash string) error {
	return s.execOne(ctx, `UPDATE applications SET token_hash = ?, updated_at = ? WHERE id = ?`, hash, nowMs(), int64(id))
}

// SetApplicationSecret replaces the OAuth2 client secret (its hash).
func (s *Store) SetApplicationSecret(ctx context.Context, id ids.ID, hash string) error {
	return s.execOne(ctx, `UPDATE applications SET secret_hash = ?, updated_at = ? WHERE id = ?`, hash, nowMs(), int64(id))
}

// SetApplicationFlags replaces an application's flags, and marks its bot as
// one that reads every message it can see when they hold the Message Content
// intent, which changes whose keys it is entitled to.
func (s *Store) SetApplicationFlags(ctx context.Context, id ids.ID, flags int64, readsAll bool) error {
	defer s.everythingChanged()
	return s.Tx(ctx, func(tx *db.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE applications SET flags = ?, updated_at = ? WHERE id = ?`, flags, nowMs(), int64(id))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		q := `UPDATE users SET flags = flags & ? WHERE id = ?`
		arg := ^FlagBotReadsAll
		if readsAll {
			q, arg = `UPDATE users SET flags = flags | ? WHERE id = ?`, FlagBotReadsAll
		}
		_, err = tx.ExecContext(ctx, q, arg, int64(id))
		return err
	})
}

// DeleteApplicationRow removes an application and everything hanging off it
// (commands, installs, tokens, interactions, emoji, its bot's sealed copies).
// Its bot's account stays, as a deleted account, for the messages it wrote;
// the caller takes the bot out of its servers first.
func (s *Store) DeleteApplicationRow(ctx context.Context, id ids.ID) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		if err := releaseBlobRefs(ctx, tx, `SELECT icon_id FROM applications WHERE id = ? UNION ALL SELECT cover_id FROM applications WHERE id = ? UNION ALL SELECT blob_id FROM application_emojis WHERE application_id = ?`, int64(id), int64(id), int64(id)); err != nil {
			return err
		}
		for _, q := range []string{`DELETE FROM bot_copies WHERE bot_id = ?`, `DELETE FROM ephemeral_messages WHERE application_id = ?`, `DELETE FROM applications WHERE id = ?`} {
			if _, err := tx.ExecContext(ctx, q, int64(id)); err != nil {
				return err
			}
		}
		return nil
	})
}

// execOne runs an update that must touch exactly one row.
func (s *Store) execOne(ctx context.Context, q string, args ...any) error {
	res, err := s.db.W.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- emoji ---------------------------------------------------------------------

// MaxApplicationEmojis is how many emoji one application may have.
const MaxApplicationEmojis = 2000

// ApplicationEmoji is an emoji an application uploaded for its messages.
type ApplicationEmoji struct {
	ApplicationID ids.ID
	Name          string
	BlobID        ids.ID
	Animated      bool
	CreatedAt     int64
}

func scanAppEmoji(r rowScanner) (*ApplicationEmoji, error) {
	var e ApplicationEmoji
	var animated int64
	if err := r.Scan(&e.ApplicationID, &e.Name, &e.BlobID, &animated, &e.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e.Animated = animated == 1
	return &e, nil
}

const appEmojiCols = `application_id, name, blob_id, animated, created_at`

// ApplicationEmojis lists an application's emoji by name.
func (s *Store) ApplicationEmojis(ctx context.Context, app ids.ID) ([]*ApplicationEmoji, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT `+appEmojiCols+` FROM application_emojis WHERE application_id = ? ORDER BY name`, int64(app))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ApplicationEmoji{}
	for rows.Next() {
		e, err := scanAppEmoji(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ApplicationEmojiByBlob finds an application's emoji by its picture.
func (s *Store) ApplicationEmojiByBlob(ctx context.Context, app, blob ids.ID) (*ApplicationEmoji, error) {
	return scanAppEmoji(s.db.R.QueryRowContext(ctx, `SELECT `+appEmojiCols+` FROM application_emojis WHERE application_id = ? AND blob_id = ? ORDER BY name LIMIT 1`, int64(app), int64(blob)))
}

// CreateApplicationEmoji stores an emoji; the caller holds the blob's
// reference. ErrAlreadyExists for a name in use, ErrTooMany past the cap.
func (s *Store) CreateApplicationEmoji(ctx context.Context, e *ApplicationEmoji) error {
	e.CreatedAt = nowMs()
	return s.Tx(ctx, func(tx *db.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM application_emojis WHERE application_id = ?`, int64(e.ApplicationID)).Scan(&n); err != nil {
			return err
		}
		if n >= MaxApplicationEmojis {
			return ErrTooMany
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO application_emojis (`+appEmojiCols+`) VALUES (?,?,?,?,?)`, int64(e.ApplicationID), e.Name, int64(e.BlobID), boolInt(e.Animated), e.CreatedAt)
		if err != nil && db.IsUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return err
	})
}

// RenameApplicationEmoji renames the emoji with the given picture.
func (s *Store) RenameApplicationEmoji(ctx context.Context, app, blob ids.ID, name string) error {
	res, err := s.db.W.ExecContext(ctx, `UPDATE application_emojis SET name = ? WHERE application_id = ? AND blob_id = ?`, name, int64(app), int64(blob))
	if err != nil {
		if db.IsUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteApplicationEmoji removes an emoji and gives back its picture's reference.
func (s *Store) DeleteApplicationEmoji(ctx context.Context, app, blob ids.ID) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM application_emojis WHERE application_id = ? AND blob_id = ?`, int64(app), int64(blob))
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `UPDATE blobs SET ref_count = `+refFloor(tx.Dialect(), "ref_count - ?")+` WHERE id = ?`, n, int64(blob))
		return err
	})
}

// --- commands --------------------------------------------------------------------

// MaxCommands is how many commands of one kind an application may have in one
// place: slash commands, then user and message commands.
const (
	MaxSlashCommands   = 100
	MaxContextCommands = 15
)

// Command is an application command: a slash command, or an entry in the
// user or message menu. Data holds the rest as the bot defined it.
type Command struct {
	ID            ids.ID
	ApplicationID ids.ID
	GuildID       *ids.ID
	Type          int
	Name          string
	Data          json.RawMessage
	Version       ids.ID
	CreatedAt     int64
}

const commandCols = `id, application_id, guild_id, type, name, data, version, created_at`

func scanCommand(r rowScanner) (*Command, error) {
	var c Command
	var guild sql.NullInt64
	var data string
	if err := r.Scan(&c.ID, &c.ApplicationID, &guild, &c.Type, &c.Name, &data, &c.Version, &c.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.GuildID = idPtr(guild)
	c.Data = json.RawMessage(data)
	return &c, nil
}

func (s *Store) queryCommands(ctx context.Context, q string, args ...any) ([]*Command, error) {
	rows, err := s.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Command{}
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Commands lists an application's global commands (guild nil) or its
// commands in one server.
func (s *Store) Commands(ctx context.Context, app ids.ID, guild *ids.ID) ([]*Command, error) {
	if guild == nil {
		return s.queryCommands(ctx, `SELECT `+commandCols+` FROM application_commands WHERE application_id = ? AND guild_id IS NULL ORDER BY id`, int64(app))
	}
	return s.queryCommands(ctx, `SELECT `+commandCols+` FROM application_commands WHERE application_id = ? AND guild_id = ? ORDER BY id`, int64(app), int64(*guild))
}

// CommandByID loads one command.
func (s *Store) CommandByID(ctx context.Context, id ids.ID) (*Command, error) {
	return scanCommand(s.db.R.QueryRowContext(ctx, `SELECT `+commandCols+` FROM application_commands WHERE id = ?`, int64(id)))
}

// CommandsOf lists the commands the given applications offer in a server:
// their global ones and those made for that server (guild zero for global
// ones only).
func (s *Store) CommandsOf(ctx context.Context, apps []ids.ID, guild ids.ID) ([]*Command, error) {
	out := []*Command{}
	err := chunked(apps, func(part []ids.ID) error {
		args := idArgs(part)
		q := `SELECT ` + commandCols + ` FROM application_commands WHERE application_id IN (` + placeholders(len(part)) + `) AND (guild_id IS NULL`
		if !guild.IsZero() {
			q += ` OR guild_id = ?`
			args = append(args, int64(guild))
		}
		list, err := s.queryCommands(ctx, q+`) ORDER BY application_id, id`, args...)
		if err != nil {
			return err
		}
		out = append(out, list...)
		return nil
	})
	return out, err
}

func commandLimit(typ int) int {
	if typ == 1 {
		return MaxSlashCommands
	}
	return MaxContextCommands
}

// PutCommand creates a command, or replaces the one of the same kind and name
// in the same place (keeping its id), as creating one does. ErrTooMany when
// that kind is full there.
func (s *Store) PutCommand(ctx context.Context, c *Command) (created bool, err error) {
	err = s.Tx(ctx, func(tx *db.Tx) error {
		created, err = putCommandTx(ctx, tx, c)
		return err
	})
	return created, err
}

func putCommandTx(ctx context.Context, tx *db.Tx, c *Command) (bool, error) {
	guildArg := nullID(c.GuildID)
	where := `application_id = ? AND type = ? AND name = ? AND guild_id IS NULL`
	args := []any{int64(c.ApplicationID), c.Type, c.Name}
	if c.GuildID != nil {
		where = `application_id = ? AND type = ? AND name = ? AND guild_id = ?`
		args = append(args, int64(*c.GuildID))
	}
	var existing int64
	var created int64
	err := tx.QueryRowContext(ctx, `SELECT id, created_at FROM application_commands WHERE `+where, args...).Scan(&existing, &created)
	c.Version = ids.New()
	if err == nil {
		c.ID, c.CreatedAt = ids.ID(existing), created
		_, err = tx.ExecContext(ctx, `UPDATE application_commands SET data = ?, version = ? WHERE id = ?`, string(c.Data), int64(c.Version), existing)
		return false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	countWhere := `application_id = ? AND type = ? AND guild_id IS NULL`
	countArgs := []any{int64(c.ApplicationID), c.Type}
	if c.GuildID != nil {
		countWhere = `application_id = ? AND type = ? AND guild_id = ?`
		countArgs = append(countArgs, int64(*c.GuildID))
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM application_commands WHERE `+countWhere, countArgs...).Scan(&n); err != nil {
		return false, err
	}
	if n >= commandLimit(c.Type) {
		return false, ErrTooMany
	}
	if c.ID.IsZero() {
		c.ID = ids.New()
	}
	c.CreatedAt = nowMs()
	_, err = tx.ExecContext(ctx, `INSERT INTO application_commands (`+commandCols+`) VALUES (?,?,?,?,?,?,?,?)`,
		int64(c.ID), int64(c.ApplicationID), guildArg, c.Type, c.Name, string(c.Data), int64(c.Version), c.CreatedAt)
	return err == nil, err
}

// UpdateCommand replaces a command's name and data. ErrAlreadyExists when
// another of its kind in the same place has the new name.
func (s *Store) UpdateCommand(ctx context.Context, c *Command) error {
	c.Version = ids.New()
	res, err := s.db.W.ExecContext(ctx, `UPDATE application_commands SET name = ?, data = ?, version = ? WHERE id = ?`, c.Name, string(c.Data), int64(c.Version), int64(c.ID))
	if err != nil {
		if db.IsUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCommand removes a command and the permissions set on it.
func (s *Store) DeleteCommand(ctx context.Context, c *Command) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM application_commands WHERE id = ?`, int64(c.ID))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM application_command_permissions WHERE application_id = ? AND command_id = ?`, int64(c.ApplicationID), int64(c.ID))
		return err
	})
}

// ReplaceCommands makes an application's commands in one place exactly list:
// commands of a kind and name it had already keep their ids, the rest are
// new, and those not in list go. Those go first, so the limits apply to list
// alone, as the protocol's bulk overwrite does.
func (s *Store) ReplaceCommands(ctx context.Context, app ids.ID, guild *ids.ID, list []*Command) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		if err := tx.Serialise(ctx, "commands", int64(app)); err != nil {
			return err
		}
		type key struct {
			typ  int
			name string
		}
		keep := map[key]bool{}
		for _, c := range list {
			keep[key{c.Type, c.Name}] = true
		}
		q := `SELECT id, type, name FROM application_commands WHERE application_id = ? AND guild_id IS NULL`
		args := []any{int64(app)}
		if guild != nil {
			q = `SELECT id, type, name FROM application_commands WHERE application_id = ? AND guild_id = ?`
			args = append(args, int64(*guild))
		}
		rows, err := tx.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		var gone []int64
		for rows.Next() {
			var id int64
			var k key
			if err := rows.Scan(&id, &k.typ, &k.name); err != nil {
				rows.Close()
				return err
			}
			if !keep[k] {
				gone = append(gone, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range gone {
			if _, err := tx.ExecContext(ctx, `DELETE FROM application_commands WHERE id = ?`, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM application_command_permissions WHERE application_id = ? AND command_id = ?`, int64(app), id); err != nil {
				return err
			}
		}
		for _, c := range list {
			c.ApplicationID, c.GuildID = app, guild
			if _, err := putCommandTx(ctx, tx, c); err != nil {
				return err
			}
		}
		return nil
	})
}

// CommandPermissions is who may use one command (or, with CommandID the
// application's id, all of its commands) in one server.
type CommandPermissions struct {
	ApplicationID ids.ID
	GuildID       ids.ID
	CommandID     ids.ID
	Permissions   json.RawMessage
}

// CommandPermissionsIn lists every permission set an application has in a server.
func (s *Store) CommandPermissionsIn(ctx context.Context, app, guild ids.ID) ([]*CommandPermissions, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT application_id, guild_id, command_id, permissions FROM application_command_permissions WHERE application_id = ? AND guild_id = ? ORDER BY command_id`, int64(app), int64(guild))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*CommandPermissions{}
	for rows.Next() {
		var p CommandPermissions
		var raw string
		if err := rows.Scan(&p.ApplicationID, &p.GuildID, &p.CommandID, &raw); err != nil {
			return nil, err
		}
		p.Permissions = json.RawMessage(raw)
		out = append(out, &p)
	}
	return out, rows.Err()
}

// SetCommandPermissions replaces the permissions of one command (or the
// whole application's) in a server; an empty list removes them.
func (s *Store) SetCommandPermissions(ctx context.Context, p *CommandPermissions) error {
	if len(p.Permissions) == 0 || string(p.Permissions) == "[]" || string(p.Permissions) == "null" {
		_, err := s.db.W.ExecContext(ctx, `DELETE FROM application_command_permissions WHERE application_id = ? AND guild_id = ? AND command_id = ?`, int64(p.ApplicationID), int64(p.GuildID), int64(p.CommandID))
		return err
	}
	_, err := s.db.W.ExecContext(ctx, `INSERT INTO application_command_permissions (application_id, guild_id, command_id, permissions) VALUES (?,?,?,?)
		ON CONFLICT(application_id, guild_id, command_id) DO UPDATE SET permissions = excluded.permissions`, int64(p.ApplicationID), int64(p.GuildID), int64(p.CommandID), string(p.Permissions))
	return err
}

// --- installs ----------------------------------------------------------------------

// GuildInstall is an application installed in a server.
type GuildInstall struct {
	ApplicationID ids.ID
	GuildID       ids.ID
	InstalledBy   *ids.ID
	Scopes        []string
	CreatedAt     int64
}

// AddGuildInstall records an application installed in a server, or adds the
// scopes to an install already there.
func (s *Store) AddGuildInstall(ctx context.Context, app, guild, by ids.ID, scopes []string) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		var raw string
		err := tx.QueryRowContext(ctx, `SELECT scopes FROM application_guild_installs WHERE application_id = ? AND guild_id = ?`, int64(app), int64(guild)).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, `INSERT INTO application_guild_installs (application_id, guild_id, installed_by, scopes, created_at) VALUES (?,?,?,?,?)`,
				int64(app), int64(guild), int64(by), stringsJSON(scopes), nowMs())
			return err
		}
		if err != nil {
			return err
		}
		merged := mergeScopes(jsonStrings(raw), scopes)
		_, err = tx.ExecContext(ctx, `UPDATE application_guild_installs SET scopes = ? WHERE application_id = ? AND guild_id = ?`, stringsJSON(merged), int64(app), int64(guild))
		return err
	})
}

func mergeScopes(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, list := range [][]string{a, b} {
		for _, sc := range list {
			if !seen[sc] {
				seen[sc] = true
				out = append(out, sc)
			}
		}
	}
	return out
}

// RemoveGuildInstall takes an application out of a server, with the
// permissions its managers set on its commands there.
func (s *Store) RemoveGuildInstall(ctx context.Context, app, guild ids.ID) (bool, error) {
	var removed bool
	err := s.Tx(ctx, func(tx *db.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM application_guild_installs WHERE application_id = ? AND guild_id = ?`, int64(app), int64(guild))
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		removed = n > 0
		if _, err := tx.ExecContext(ctx, `DELETE FROM application_command_permissions WHERE application_id = ? AND guild_id = ?`, int64(app), int64(guild)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM application_commands WHERE application_id = ? AND guild_id = ?`, int64(app), int64(guild))
		return err
	})
	return removed, err
}

func (s *Store) queryGuildInstalls(ctx context.Context, q string, args ...any) ([]*GuildInstall, error) {
	rows, err := s.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*GuildInstall{}
	for rows.Next() {
		var g GuildInstall
		var by sql.NullInt64
		var scopes string
		if err := rows.Scan(&g.ApplicationID, &g.GuildID, &by, &scopes, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.InstalledBy, g.Scopes = idPtr(by), jsonStrings(scopes)
		out = append(out, &g)
	}
	return out, rows.Err()
}

// GuildInstalls lists the applications installed in a server.
func (s *Store) GuildInstalls(ctx context.Context, guild ids.ID) ([]*GuildInstall, error) {
	return s.queryGuildInstalls(ctx, `SELECT application_id, guild_id, installed_by, scopes, created_at FROM application_guild_installs WHERE guild_id = ? ORDER BY created_at`, int64(guild))
}

// GuildInstall loads one application's install in a server.
func (s *Store) GuildInstall(ctx context.Context, app, guild ids.ID) (*GuildInstall, error) {
	list, err := s.queryGuildInstalls(ctx, `SELECT application_id, guild_id, installed_by, scopes, created_at FROM application_guild_installs WHERE application_id = ? AND guild_id = ?`, int64(app), int64(guild))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return list[0], nil
}

// ApplicationGuildCount is how many servers an application is installed in.
func (s *Store) ApplicationGuildCount(ctx context.Context, app ids.ID) (int, error) {
	var n int
	err := s.db.R.QueryRowContext(ctx, `SELECT count(*) FROM application_guild_installs WHERE application_id = ?`, int64(app)).Scan(&n)
	return n, err
}

// UserInstall is an application installed on an account.
type UserInstall struct {
	ApplicationID ids.ID
	UserID        ids.ID
	Scopes        []string
	CreatedAt     int64
}

// AddUserInstall records an application installed on an account.
func (s *Store) AddUserInstall(ctx context.Context, app, user ids.ID, scopes []string) error {
	_, err := s.db.W.ExecContext(ctx, `INSERT INTO application_user_installs (application_id, user_id, scopes, created_at) VALUES (?,?,?,?)
		ON CONFLICT(application_id, user_id) DO UPDATE SET scopes = excluded.scopes`, int64(app), int64(user), stringsJSON(scopes), nowMs())
	return err
}

// RemoveUserInstall takes an application off an account.
func (s *Store) RemoveUserInstall(ctx context.Context, app, user ids.ID) (bool, error) {
	res, err := s.db.W.ExecContext(ctx, `DELETE FROM application_user_installs WHERE application_id = ? AND user_id = ?`, int64(app), int64(user))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UserInstalls lists the applications installed on an account.
func (s *Store) UserInstalls(ctx context.Context, user ids.ID) ([]*UserInstall, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT application_id, user_id, scopes, created_at FROM application_user_installs WHERE user_id = ? ORDER BY created_at`, int64(user))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*UserInstall{}
	for rows.Next() {
		var u UserInstall
		var scopes string
		if err := rows.Scan(&u.ApplicationID, &u.UserID, &scopes, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.Scopes = jsonStrings(scopes)
		out = append(out, &u)
	}
	return out, rows.Err()
}

// HasUserInstall reports whether an application is installed on an account.
func (s *Store) HasUserInstall(ctx context.Context, app, user ids.ID) (bool, error) {
	var n int
	err := s.db.R.QueryRowContext(ctx, `SELECT count(*) FROM application_user_installs WHERE application_id = ? AND user_id = ?`, int64(app), int64(user)).Scan(&n)
	return n > 0, err
}

// ApplicationUserCount is how many accounts an application is installed on.
func (s *Store) ApplicationUserCount(ctx context.Context, app ids.ID) (int, error) {
	var n int
	err := s.db.R.QueryRowContext(ctx, `SELECT count(*) FROM application_user_installs WHERE application_id = ?`, int64(app)).Scan(&n)
	return n, err
}

// BotServers lists the servers a bot is a member of, oldest membership first.
func (s *Store) BotServers(ctx context.Context, bot ids.ID) ([]ids.ID, error) {
	return s.serversOf(ctx, bot)
}

// ApplicationGuilds lists the servers an application is installed in.
func (s *Store) ApplicationGuilds(ctx context.Context, app ids.ID) ([]ids.ID, error) {
	return s.idList(ctx, `SELECT guild_id FROM application_guild_installs WHERE application_id = ? ORDER BY guild_id`, int64(app))
}

// ApplicationUsers lists the accounts an application is installed on.
func (s *Store) ApplicationUsers(ctx context.Context, app ids.ID) ([]ids.ID, error) {
	return s.idList(ctx, `SELECT user_id FROM application_user_installs WHERE application_id = ? ORDER BY user_id`, int64(app))
}
