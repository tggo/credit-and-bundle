package web

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func (s *Server) sse(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	hub := s.a.Hub
	ch := hub.Sub()
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer hub.Unsub(ch)
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case m := <-ch:
				fmt.Fprintf(w, "event: %s\n", m.Event)
				for _, line := range strings.Split(m.Data, "\n") {
					fmt.Fprintf(w, "data: %s\n", line)
				}
				fmt.Fprint(w, "\n")
			case <-tick.C:
				fmt.Fprint(w, ": ping\n\n")
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	}))
	return nil
}
