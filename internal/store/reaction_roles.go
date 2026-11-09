package store

import (
	"context"
	"database/sql"
	"errors"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// ReactionRole says that reacting to a message with Emoji gives Role.
type ReactionRole struct {
	Emoji  string `json:"emoji"`
	RoleID ids.ID `json:"role_id"`
}

// fillReactionRoles adds to each message the reactions on it that give a role.
func (s *Store) fillReactionRoles(ctx context.Context, byID map[ids.ID]*Message, ph string, list []ids.ID) error {
	rows, err := s.db.R.QueryContext(ctx, `SELECT message_id, emoji, role_id FROM reaction_roles WHERE message_id IN (`+ph+`) ORDER BY created_at`, idArgs(list)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var mid, role int64
		var emoji string
		if err := rows.Scan(&mid, &emoji, &role); err != nil {
			return err
		}
		if m := byID[ids.ID(mid)]; m != nil {
			m.ReactionRoles = append(m.ReactionRoles, ReactionRole{Emoji: emoji, RoleID: ids.ID(role)})
		}
	}
	return rows.Err()
}

// ReactionRolesOf lists what a message's reactions give.
func (s *Store) ReactionRolesOf(ctx context.Context, msgID ids.ID) ([]ReactionRole, error) {
	m := &Message{ID: msgID}
	if err := s.fillReactionRoles(ctx, map[ids.ID]*Message{msgID: m}, "?", []ids.ID{msgID}); err != nil {
		return nil, err
	}
	if m.ReactionRoles == nil {
		return []ReactionRole{}, nil
	}
	return m.ReactionRoles, nil
}

// SetReactionRole makes a reaction give a role, replacing what it gave.
func (s *Store) SetReactionRole(ctx context.Context, serverID, msgID ids.ID, emoji string, roleID ids.ID) error {
	return s.Tx(ctx, func(tx *db.Tx) error {
		if err := tx.Serialise(ctx, "reactions", int64(msgID)); err != nil {
			return err
		}
		var others int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM reaction_roles WHERE message_id = ? AND emoji <> ?`, int64(msgID), emoji).Scan(&others); err != nil {
			return err
		}
		if others >= MaxReactionEmoji {
			return ErrLimit
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO reaction_roles (message_id, emoji, server_id, role_id, created_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (message_id, emoji) DO UPDATE SET role_id = excluded.role_id`,
			int64(msgID), emoji, int64(serverID), int64(roleID), nowMs())
		return err
	})
}

// RemoveReactionRole makes a reaction give nothing; the role it gave, or
// ErrNotFound when it gave none.
func (s *Store) RemoveReactionRole(ctx context.Context, msgID ids.ID, emoji string) (ids.ID, error) {
	var role int64
	err := s.Tx(ctx, func(tx *db.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT role_id FROM reaction_roles WHERE message_id = ? AND emoji = ?`, int64(msgID), emoji).Scan(&role)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM reaction_roles WHERE message_id = ? AND emoji = ?`, int64(msgID), emoji)
		return err
	})
	return ids.ID(role), err
}
