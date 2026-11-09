package store

import (
	"context"
	"encoding/json"
	"time"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

const (
	// ActionServerManage is a case telling someone what the owner of Nexa did
	// to a server: its owner, a member removed or banned, or its new owner.
	ActionServerManage = "server_manage"
	// LogOwnerServerPrefix starts every moderation log action of the owner's
	// server management, and the panel's name for the action follows it.
	LogOwnerServerPrefix  = "owner_server_"
	LogOwnerServerBegin   = LogOwnerServerPrefix + "begin"
	LogOwnerServerView    = LogOwnerServerPrefix + "view"
	LogOwnerServerRefused = LogOwnerServerPrefix + "refused"
)

// OwnerManage is what is recorded about an action of the owner's server
// management once it has been made. The moderation log cannot be changed once
// written (a database trigger holds that), so the attempt is a line of its own
// (AppendModLog, before the change, and an action that cannot log that does not
// go ahead), and Log is the line with how it came out.
type OwnerManage struct {
	Log   *ModLogEntry
	Audit *AuditEntry
	Cases []*ModCase
}

// ownerRecordTries is how many times a committed change's record is attempted.
const ownerRecordTries = 3

// OwnerManageRecord writes the server's audit entry, every notice and the
// outcome line at once, after the change has committed, so that the server
// hears only of what happened.
func (s *Store) OwnerManageRecord(ctx context.Context, m *OwnerManage, changes json.RawMessage) error {
	if m.Audit.ID.IsZero() {
		m.Audit.ID = ids.New()
	}
	if m.Audit.CreatedAt == 0 {
		m.Audit.CreatedAt = nowMs()
	}
	if len(changes) == 0 {
		changes = json.RawMessage("{}")
	}
	m.Audit.Changes = changes
	var err error
	for try := range ownerRecordTries {
		if try > 0 {
			time.Sleep(time.Duration(try) * 100 * time.Millisecond)
		}
		err = s.Tx(ctx, func(tx *db.Tx) error {
			if _, err := tx.ExecContext(ctx, `INSERT INTO audit_log (id, server_id, actor_id, action, target_type, target_id, changes, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				int64(m.Audit.ID), int64(m.Audit.ServerID), int64(m.Audit.ActorID), m.Audit.Action, m.Audit.TargetType, nullID(m.Audit.TargetID), string(m.Audit.Changes), m.Audit.Reason, m.Audit.CreatedAt); err != nil {
				return err
			}
			for _, c := range m.Cases {
				if err := openCaseTx(ctx, tx, c, nil); err != nil {
					return err
				}
			}
			if len(m.Cases) > 0 {
				id, num := m.Cases[0].ID, m.Cases[0].Number
				m.Log.CaseID, m.Log.CaseNumber = &id, &num
			}
			return appendModLogTx(ctx, tx, m.Log)
		})
		if err == nil {
			return nil
		}
	}
	m.Log.CaseID, m.Log.CaseNumber = nil, nil
	return err
}

// PrivilegedMembers lists the members of a server who hold a staff rank or are
// apps, with the ones named in also: everyone who must not gain access to a
// channel through the owner's server management.
func (s *Store) PrivilegedMembers(ctx context.Context, serverID ids.ID, also []ids.ID) (map[ids.ID]bool, error) {
	rows, err := s.db.R.QueryContext(ctx, `SELECT u.id FROM server_members m JOIN users u ON u.id = m.user_id WHERE m.server_id = ? AND u.flags & ? != 0`,
		int64(serverID), int64(StaffFlags|FlagBot))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.ID]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[ids.ID(id)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range also {
		if ok, _ := s.IsMember(ctx, serverID, id); ok {
			out[id] = true
		}
	}
	return out, nil
}
