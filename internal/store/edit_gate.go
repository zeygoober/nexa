package store

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"evilion.xyz/chat/internal/ids"
)

// ErrEditsBusy says a server's edit gate stayed closed for all of editWait.
var ErrEditsBusy = errors.New("server edits busy")

// editWait bounds a wait at the gate, so that a holder that never lets go, or
// a write made without its holder's context, fails rather than hangs.
const editWait = 10 * time.Second

// editGates is one gate per server between the owner's server management and
// everything else that can change who sees what in that server. An action
// reads the server, decides the change opens nothing to anyone guarded and
// makes it, and no role, overwrite, channel or ownership write of that server
// may land anywhere in between.
//
// Ordinary writes share a gate with each other; an action holds it alone. The
// order is always gate then database writer, and a gated write never starts
// inside a transaction (on SQLite that would already wait on itself). It lives
// in this process, which is what the deployment is: one chatd.
type editGates struct {
	mu   sync.Mutex
	open map[ids.ID]*editGate
}

type editGate struct {
	users            int // holding or waiting; at zero the entry goes
	sharing          int
	alone            bool
	waitingToBeAlone int
	freed            chan struct{} // closed and replaced whenever anything lets go
}

func (g *editGate) wake() {
	close(g.freed)
	g.freed = make(chan struct{})
}

func (g *editGates) take(ctx context.Context, server ids.ID, alone bool) error {
	g.mu.Lock()
	if g.open == nil {
		g.open = map[ids.ID]*editGate{}
	}
	e := g.open[server]
	if e == nil {
		e = &editGate{freed: make(chan struct{})}
		g.open[server] = e
	}
	e.users++
	if alone {
		e.waitingToBeAlone++
	}
	timer := time.NewTimer(editWait)
	defer timer.Stop()
	for {
		// an action waiting goes first, so that a stream of edits cannot hold it off
		if !e.alone && ((alone && e.sharing == 0) || (!alone && e.waitingToBeAlone == 0)) {
			if alone {
				e.waitingToBeAlone--
				e.alone = true
			} else {
				e.sharing++
			}
			g.mu.Unlock()
			return nil
		}
		freed := e.freed
		g.mu.Unlock()
		var err error
		select {
		case <-freed:
		case <-ctx.Done():
			err = ctx.Err()
		case <-timer.C:
			err = ErrEditsBusy
		}
		g.mu.Lock()
		if err != nil {
			if alone {
				e.waitingToBeAlone--
				e.wake()
			}
			g.leave(server, e)
			g.mu.Unlock()
			return err
		}
	}
}

func (g *editGates) give(server ids.ID, alone bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.open[server]
	if alone {
		e.alone = false
	} else {
		e.sharing--
	}
	e.wake()
	g.leave(server, e)
}

func (g *editGates) leave(server ids.ID, e *editGate) {
	if e.users--; e.users == 0 {
		delete(g.open, server)
	}
}

// editLease is in the context of whoever holds a server's gate, so that the
// holder's own writes go straight through. It dies with the hold, so a context
// that outlives it (a goroutine's) asks at the gate like anyone else.
type editLease struct {
	server ids.ID
	live   atomic.Bool
}

type editLeaseKey struct{}

func holdsEdits(ctx context.Context, server ids.ID) bool {
	l, _ := ctx.Value(editLeaseKey{}).(*editLease)
	return l != nil && l.server == server && l.live.Load()
}

func (s *Store) holdEdits(ctx context.Context, server ids.ID, alone bool) (context.Context, func(), error) {
	if server.IsZero() || holdsEdits(ctx, server) {
		return ctx, func() {}, nil
	}
	if err := s.edits.take(ctx, server, alone); err != nil {
		return ctx, nil, err
	}
	l := &editLease{server: server}
	l.live.Store(true)
	return context.WithValue(ctx, editLeaseKey{}, l), sync.OnceFunc(func() {
		l.live.Store(false)
		s.edits.give(server, alone)
	}), nil
}

// LockServerEdits holds a server's gate alone, for an action that has to read
// the server and change it with nothing landing in between. Pass the context
// it returns to everything the action does, and call the function it returns
// when the action is finished and recorded.
func (s *Store) LockServerEdits(ctx context.Context, server ids.ID) (context.Context, func(), error) {
	return s.holdEdits(ctx, server, true)
}

// ShareServerEdits is the ordinary side of the gate, taken by every write that
// can change who sees what in a server. Taken at the very top of the function,
// so it is let go after the permission cache has heard of the write.
func (s *Store) ShareServerEdits(ctx context.Context, server ids.ID) (context.Context, func(), error) {
	return s.holdEdits(ctx, server, false)
}

// shareChannelEdits is ShareServerEdits for a channel. A channel of no server
// has no gate.
func (s *Store) shareChannelEdits(ctx context.Context, channel ids.ID) (context.Context, func(), error) {
	server, err := channelServer(ctx, s.db.R, channel)
	if err != nil {
		return ctx, nil, err
	}
	return s.ShareServerEdits(ctx, server)
}

// shareRoleEdits is ShareServerEdits for a role. A role that does not exist
// has no gate, and the write that follows answers for it.
func (s *Store) shareRoleEdits(ctx context.Context, role ids.ID) (context.Context, func(), error) {
	var server ids.ID
	err := s.db.R.QueryRowContext(ctx, `SELECT server_id FROM roles WHERE id = ?`, int64(role)).Scan(&server)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ctx, nil, err
	}
	return s.ShareServerEdits(ctx, server)
}
