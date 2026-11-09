package store

import (
	"context"
	"database/sql"
	"errors"

	"evilion.xyz/chat/internal/ids"
)

// LastfmLink is an account's Last.fm username. It is a public name typed in,
// not proof that the person owns it.
type LastfmLink struct {
	UserID   ids.ID
	Username string
	Show     bool
}

// LastfmLinkOf is an account's link, or ErrNotFound.
func (s *Store) LastfmLinkOf(ctx context.Context, uid ids.ID) (*LastfmLink, error) {
	l := &LastfmLink{UserID: uid}
	var show int
	err := s.db.R.QueryRowContext(ctx, `SELECT username, show_activity FROM lastfm_links WHERE user_id = ?`, int64(uid)).Scan(&l.Username, &show)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	l.Show = show != 0
	return l, nil
}

// SetLastfmLink links an account to a Last.fm username, replacing any link it has.
func (s *Store) SetLastfmLink(ctx context.Context, uid ids.ID, username string, show bool) error {
	_, err := s.db.W.ExecContext(ctx, `INSERT INTO lastfm_links (user_id, username, show_activity, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET username = excluded.username, show_activity = excluded.show_activity`,
		int64(uid), username, boolInt(show), nowMs())
	return err
}

// SetLastfmShow says whether a linked account's listening is shown.
func (s *Store) SetLastfmShow(ctx context.Context, uid ids.ID, show bool) error {
	_, err := s.db.W.ExecContext(ctx, `UPDATE lastfm_links SET show_activity = ? WHERE user_id = ?`, boolInt(show), int64(uid))
	return err
}

// DeleteLastfmLink removes an account's link.
func (s *Store) DeleteLastfmLink(ctx context.Context, uid ids.ID) error {
	_, err := s.db.W.ExecContext(ctx, `DELETE FROM lastfm_links WHERE user_id = ?`, int64(uid))
	return err
}

// LastfmShowing is every link whose listening is shown, for the poll.
func (s *Store) LastfmShowing(ctx context.Context) ([]LastfmLink, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT user_id, username FROM lastfm_links WHERE show_activity = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LastfmLink
	for rows.Next() {
		var uid int64
		var name string
		if err := rows.Scan(&uid, &name); err != nil {
			return nil, err
		}
		out = append(out, LastfmLink{UserID: ids.ID(uid), Username: name, Show: true})
	}
	return out, rows.Err()
}
