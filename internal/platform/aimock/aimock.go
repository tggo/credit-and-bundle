// Package aimock is the fake AI provider behind the gateway: a short delay, and failures on demand from faults.
package aimock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"bundle-credit-test/internal/platform/faults"
)

type Provider struct {
	Faults *faults.Faults
	Delay  time.Duration
}

var ErrUpstream = errors.New("provider 5xx")

func (p *Provider) Run(ctx context.Context, action, input string) ([]byte, bool, error) {
	select { // a caller's timeout ends the call, as with a real provider
	case <-time.After(p.Delay):
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	if faults.Take(&p.Faults.CrashAI) {
		return nil, false, faults.ErrCrash
	}
	if faults.Take(&p.Faults.FailAI) {
		return nil, false, ErrUpstream
	}
	if faults.Take(&p.Faults.BlockAI) {
		return []byte(`{"blocked":true,"reason":"content_policy"}`), true, nil
	}
	b, _ := json.Marshal(map[string]string{"output": fmt.Sprintf("%s done for %s", action, input)})
	return b, false, nil
}
