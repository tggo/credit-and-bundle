package web

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// Boards are shared: every save bumps rev and is pushed to all open tabs over SSE ("board" event).
func (s *Server) boardRoutes(r fiber.Router) {
	r.Get("/:id", func(c *fiber.Ctx) error {
		scene, rev, err := s.a.Boards.Get(c.Context(), c.Params("id"))
		if err != nil {
			return c.SendStatus(404)
		}
		c.Set("X-Rev", strconv.FormatInt(rev, 10))
		c.Type("json")
		return c.Send(scene)
	})
	r.Put("/:id", func(c *fiber.Ctx) error {
		id, body := c.Params("id"), c.Body()
		if !json.Valid(body) { // the scene is also pushed raw into the SSE event
			return c.Status(400).JSON(fiber.Map{"error": "invalid_scene"})
		}
		rev, changed, err := s.a.Boards.Save(c.Context(), id, body)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		c.Set("X-Rev", strconv.FormatInt(rev, 10))
		if !changed {
			return c.SendStatus(204)
		}
		s.a.Hub.Pub("board", fmt.Sprintf(`{"type":"scene","board":%q,"client":%q,"rev":%d,"scene":%s}`, id, c.Get("X-Client"), rev, body))
		c.Set("X-Rev", strconv.FormatInt(rev, 10))
		return c.SendStatus(204)
	})
	r.Delete("/:id", func(c *fiber.Ctx) error {
		_ = s.a.Boards.Delete(c.Context(), c.Params("id"))
		s.a.Hub.Pub("board", fmt.Sprintf(`{"type":"reset","board":%q,"client":%q}`, c.Params("id"), c.Get("X-Client")))
		return c.SendStatus(204)
	})
	// Presence: cursor position and name of each viewer, relayed as-is.
	r.Post("/:id/presence", func(c *fiber.Ctx) error {
		var p struct {
			Client, Name, Color string
			X, Y                float64
			Sel                 string
		}
		if err := c.BodyParser(&p); err != nil {
			return c.SendStatus(400)
		}
		b, _ := json.Marshal(map[string]any{"type": "cursor", "board": c.Params("id"), "client": p.Client, "name": p.Name,
			"color": p.Color, "x": p.X, "y": p.Y, "sel": p.Sel})
		s.a.Hub.Pub("board", string(b))
		return c.SendStatus(204)
	})
}
