package web

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	catmodels "bundle-credit-test/internal/catalog/models"
	credmodels "bundle-credit-test/internal/credits/models"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Public credits API, as in the design doc. X-Customer-Id (customer uuid) stands in for the license token.
func (s *Server) apiRoutes(r fiber.Router) {
	r.Post("/charges", func(c *fiber.Ctx) error {
		var body struct {
			ApplicationID string `json:"application_id"`
			ActionCode    string `json:"action_code"`
			Input         any    `json:"input"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "bad_request"})
		}
		key := c.Get("Idempotency-Key")
		if key == "" {
			return c.Status(400).JSON(fiber.Map{"error": "idempotency_key_required"})
		}
		if !uuidRe.MatchString(c.Get("X-Customer-Id")) {
			return c.Status(400).JSON(fiber.Map{"error": "customer_required"})
		}
		if !uuidRe.MatchString(body.ApplicationID) {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_application_id"})
		}
		code, res := s.a.Credits.Charge(c.Context(), credmodels.ChargeRequest{CustomerID: c.Get("X-Customer-Id"),
			AppID: body.ApplicationID, Action: body.ActionCode, Key: key, Input: fmt.Sprint(body.Input)})
		s.a.Hub.Log(logKind(code), "API charge %s key=%s → %d %s", body.ActionCode, key, code, res.Status)
		s.a.Hub.Changed()
		return c.Status(code).JSON(res)
	})
	// application_id is optional: without it the answer is the whole wallet (the site's view), with it what this app
	// can spend. A malformed or unknown id is an error, never a silent "0 here": the app would show a wrong balance.
	r.Get("/balance", func(c *fiber.Ctx) error {
		cust, appID := c.Get("X-Customer-Id"), c.Query("application_id")
		if !uuidRe.MatchString(cust) {
			return c.Status(400).JSON(fiber.Map{"error": "customer_required"})
		}
		if appID != "" {
			if !uuidRe.MatchString(appID) {
				return c.Status(400).JSON(fiber.Map{"error": "invalid_application_id"})
			}
			apps, err := s.a.Catalog.Apps(c.Context())
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "internal"})
			}
			if !slices.ContainsFunc(apps, func(a catmodels.App) bool { return a.ID == appID }) {
				return c.Status(404).JSON(fiber.Map{"error": "unknown_application"})
			}
		}
		b, err := s.a.Credits.Balance(c.Context(), cust, appID)
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "unknown_customer"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "internal"})
		}
		return c.JSON(b)
	})
}
