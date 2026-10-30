// Package web is the HTTP layer: server-rendered pages, htmx fragments, simulator actions, the credits API, boards and SSE.
package web

import (
	"io/fs"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"

	"bundle-credit-test/internal/app"
)

type Server struct {
	a *app.App
}

func New(a *app.App) *Server { return &Server{a: a} }

func (s *Server) Routes() *fiber.App {
	f := fiber.New(fiber.Config{DisableStartupMessage: true})
	sub, _ := fs.Sub(staticFS, "static")
	f.Use("/static", filesystem.New(filesystem.Config{Root: http.FS(sub)}))

	for path, page := range map[string]string{"/": "sim", "/webhooks": "webhooks", "/reconcile": "reconcile",
		"/diagrams": "diagrams", "/board": "board", "/data": "data", "/api": "api",
		"/tables": "tables", "/migration": "migration", "/testing": "testing"} {
		page := page
		f.Get(path, func(c *fiber.Ctx) error { return s.page(c, page) })
	}
	f.Get("/events", s.sse)
	f.Get("/api/schema/comments", s.schemaComments)

	f.Get("/frag/state", s.fragState)
	f.Get("/frag/webhooks", s.fragWebhooks)
	f.Get("/frag/reconcile", s.fragReconcile)
	f.Get("/frag/flow", s.fragFlow)
	f.Get("/frag/clock", s.fragClock)
	f.Get("/frag/tables", s.fragTables)
	f.Get("/frag/table", s.fragTable)

	s.simRoutes(f.Group("/sim"))
	s.apiRoutes(f.Group("/api/v1/credits"))
	s.boardRoutes(f.Group("/api/boards"))
	s.diagramRoutes(f.Group("/api/diagrams"))
	return f
}
