// Package clock is the simulated time: real time plus an offset kept in Postgres, so "+31 days" moves every process.
package clock

import (
	"context"
	"sync/atomic"
	"time"

	"bundle-credit-test/internal/platform/db"
)

type Clock struct {
	db   db.Q
	last atomic.Int64 // offset of the last successful read
}

func New(q db.Q) *Clock { return &Clock{db: q} }

// Now keeps the last known offset when the read fails: dropping to real time would revive expired lots.
func (c *Clock) Now(ctx context.Context) time.Time {
	var off int64
	if err := c.db.QueryRow(ctx, `SELECT offset_seconds FROM sim_clock WHERE id=1`).Scan(&off); err == nil {
		c.last.Store(off)
	}
	off = c.last.Load()
	return time.Now().UTC().Add(time.Duration(off) * time.Second).Truncate(time.Second)
}

func (c *Clock) Advance(ctx context.Context, days int) error {
	_, err := c.db.Exec(ctx, `UPDATE sim_clock SET offset_seconds = offset_seconds + $1 WHERE id=1`, days*86400)
	return err
}
