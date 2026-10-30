// Package sse fans out server-sent events and keeps a short live log for the first render.
package sse

import (
	"fmt"
	"html"
	"strings"
	"sync"
	"time"
)

type sseMsg struct {
	Event string
	Data  string
}

// Hub fans out SSE messages and keeps a short event log for the initial render.
type Hub struct {
	mu   sync.Mutex
	subs map[chan sseMsg]struct{}
	logs []string // newest first, rendered <li>
}

func NewHub() *Hub { return &Hub{subs: map[chan sseMsg]struct{}{}} }

func (h *Hub) Sub() chan sseMsg {
	ch := make(chan sseMsg, 512)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsub(ch chan sseMsg) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *Hub) Pub(event, data string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- sseMsg{event, data}:
		default: // slow client: drop
		}
	}
}

// Changed tells every page to re-fetch its panels.
func (h *Hub) Changed() { h.Pub("changed", "1") }

// Log appends a line to the live log. kind: ok | warn | err | info.
func (h *Hub) Log(kind, format string, args ...any) {
	line := fmt.Sprintf(`<li class="log-%s"><time>%s</time> %s</li>`,
		kind, time.Now().Format("15:04:05.000"), html.EscapeString(fmt.Sprintf(format, args...)))
	h.mu.Lock()
	h.logs = append([]string{line}, h.logs...)
	if len(h.logs) > 300 {
		h.logs = h.logs[:300]
	}
	h.mu.Unlock()
	h.Pub("log", line)
}

func (h *Hub) LogHTML() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.logs, "\n")
}

func (h *Hub) ClearLog() {
	h.mu.Lock()
	h.logs = nil
	h.mu.Unlock()
}
