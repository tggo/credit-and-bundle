package web

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"

	"bundle-credit-test/internal/platform/plantuml"
)

// Editable PlantUML diagrams: the default source ships in diagrams/*.puml; an edited source lives in boards.diagrams.
// Rendering goes to the self-hosted plantuml-server; without it the build-time SVGs are shown read-only.

//go:embed diagrams/*.puml diagrams/*.iuml
var pumlFS embed.FS

var diagramIDs = map[string]bool{"states": true, "grants": true, "charge": true, "domains": true, "migration": true, "purchase": true, "flow_current": true, "flow_changed": true}

func defaultSource(id string) string {
	b, _ := pumlFS.ReadFile("diagrams/" + id + ".puml")
	return string(b)
}

// withSkin inlines the shared skin: the server cannot read our !include files.
func withSkin(src string) string {
	skin, _ := pumlFS.ReadFile("diagrams/skin.iuml")
	return strings.Replace(src, "!include skin.iuml", string(skin), 1)
}

var lineRe = regexp.MustCompile(`line \d+`)

var svgCache sync.Map // sha256(source) → svg

func (s *Server) source(ctx context.Context, id string) (string, int64, bool) {
	if src, rev, err := s.a.Boards.DiagramSource(ctx, id); err == nil {
		return src, rev, false
	}
	return defaultSource(id), 0, true
}

func (s *Server) render(ctx context.Context, src string) ([]byte, error) {
	key := sha256.Sum256([]byte(src))
	if v, ok := svgCache.Load(key); ok {
		return v.([]byte), nil
	}
	svg, err := s.a.PlantUML.Render(ctx, withSkin(src))
	if err == nil {
		svgCache.Store(key, svg)
	}
	var se plantuml.ErrSyntax
	if errors.As(err, &se) && strings.Contains(src, "!include skin.iuml") {
		// the skin was inlined: map the server's line back to the editor's line
		skin, _ := pumlFS.ReadFile("diagrams/skin.iuml")
		shift := strings.Count(strings.TrimRight(string(skin), "\n"), "\n")
		se.Msg = lineRe.ReplaceAllStringFunc(se.Msg, func(m string) string {
			n, _ := strconv.Atoi(strings.TrimPrefix(m, "line "))
			return "line " + strconv.Itoa(n-shift)
		})
		return nil, se
	}
	return svg, err
}

func (s *Server) diagramRoutes(r fiber.Router) {
	valid := func(c *fiber.Ctx) (string, bool) { id := c.Params("id"); return id, diagramIDs[id] }

	r.Get("/:id", func(c *fiber.Ctx) error {
		id, ok := valid(c)
		if !ok {
			return c.SendStatus(404)
		}
		src, rev, def := s.source(c.Context(), id)
		skin, _ := pumlFS.ReadFile("diagrams/skin.iuml")
		return c.JSON(fiber.Map{"id": id, "source": src, "rev": rev, "default": def, "editable": s.a.PlantUML.URL != "",
			"skinShift": strings.Count(string(skin), "\n")})
	})
	// SVG of the current source; falls back to the build-time render when the server is not there.
	r.Get("/:id/svg", func(c *fiber.Ctx) error {
		id, ok := valid(c)
		if !ok {
			return c.SendStatus(404)
		}
		src, _, _ := s.source(c.Context(), id)
		svg, err := s.render(c.Context(), src)
		if err != nil {
			svg = []byte(diagram(id))
		}
		c.Type("svg")
		return c.Send(svg)
	})
	// Preview renders without saving; a syntax error comes back as 422 with PlantUML's message.
	r.Post("/:id/preview", func(c *fiber.Ctx) error {
		svg, err := s.render(c.Context(), string(c.Body()))
		var se plantuml.ErrSyntax
		if errors.As(err, &se) {
			return c.Status(422).SendString(se.Msg)
		}
		if err != nil {
			return c.Status(503).SendString(err.Error())
		}
		c.Type("svg")
		return c.Send(svg)
	})
	r.Put("/:id", func(c *fiber.Ctx) error {
		id, ok := valid(c)
		if !ok {
			return c.SendStatus(404)
		}
		src := string(c.Body())
		if _, err := s.render(c.Context(), src); err != nil {
			return c.Status(422).SendString(err.Error())
		}
		rev, err := s.a.Boards.SaveDiagram(c.Context(), id, src)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		s.pubDiagram(id, rev, c.Get("X-Client"))
		return c.JSON(fiber.Map{"rev": rev})
	})
	r.Delete("/:id", func(c *fiber.Ctx) error {
		id, ok := valid(c)
		if !ok {
			return c.SendStatus(404)
		}
		_ = s.a.Boards.DeleteDiagram(c.Context(), id)
		s.pubDiagram(id, 0, c.Get("X-Client"))
		return c.SendStatus(204)
	})
}

func (s *Server) pubDiagram(id string, rev int64, client string) {
	b, _ := json.Marshal(map[string]any{"type": "diagram", "id": id, "rev": rev, "client": client})
	s.a.Hub.Pub("diagram", string(b))
}
