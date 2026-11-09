package api

import (
	"context"
	"time"

	"evilion.xyz/chat/internal/ids"
)

const (
	callCountEvery = time.Minute
	// how many counts of call time are held in memory before they are written
	callCountsPerWrite = 5
)

// StartBadgeProgress adds up, until ctx ends, the time people spend in calls
// with someone else, for the Open Mic badge. Time is added once a minute and
// written every few, so a crash loses at most those few minutes.
func (s *Server) StartBadgeProgress(ctx context.Context) {
	go func() {
		tick := time.NewTicker(callCountEvery)
		defer tick.Stop()
		pending := map[ids.ID]int64{}
		write := func(c context.Context) {
			if err := s.Store.AddCallSeconds(c, pending); err != nil {
				if c.Err() == nil {
					s.Log.Warn("call time for badges", "err", err)
				}
				return // kept, for the next write
			}
			clear(pending)
		}
		last, counts := time.Now(), 0
		for {
			select {
			case <-ctx.Done():
				done, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				write(done)
				cancel()
				return
			case now := <-tick.C:
				// a machine that slept did not spend that time in a call
				gone := int64(min(now.Sub(last), 2*callCountEvery) / time.Second)
				last = now
				for _, uid := range s.Voice.InCallTogether() {
					pending[uid] += gone
				}
				if counts++; counts >= callCountsPerWrite {
					counts = 0
					write(ctx)
				}
			}
		}
	}()
}
