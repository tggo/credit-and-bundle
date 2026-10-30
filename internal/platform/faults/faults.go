// Package faults holds the simulator's failure switches: each counter makes the next N calls fail in a given way.
package faults

import (
	"errors"
	"sync/atomic"
)

// ErrCrash stands for the process dying mid-request: nothing after it runs, not even the refund.
var ErrCrash = errors.New("simulated crash after debit")

type Faults struct {
	FailAI      atomic.Int32 // next N AI calls fail with provider 5xx
	BlockAI     atomic.Int32 // next N AI calls are blocked by content policy
	LoseWebhook atomic.Int32 // next N invoice.paid webhooks are lost
	CrashAI     atomic.Int32 // next N charges: the process "dies" after the debit, the charge stays in_progress
}

// Take consumes one pending fault of a counter.
func Take(c *atomic.Int32) bool {
	for {
		v := c.Load()
		if v <= 0 {
			return false
		}
		if c.CompareAndSwap(v, v-1) {
			return true
		}
	}
}

func (f *Faults) Reset() {
	f.FailAI.Store(0)
	f.BlockAI.Store(0)
	f.LoseWebhook.Store(0)
	f.CrashAI.Store(0)
}
