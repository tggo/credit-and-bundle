// Package ids makes short random references for external ids (provider refs, license keys, idempotency keys).
package ids

import (
	"crypto/rand"
	"encoding/hex"
)

func New(prefix string) string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
