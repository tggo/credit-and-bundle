package web

import (
	"bundle-credit-test/internal/app"
	"context"
	"crypto/subtle"
	"fmt"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"

	custmodels "bundle-credit-test/internal/customer/models"
)

// Simulator actions. All return 204; the pages refresh through SSE.
func (s *Server) simRoutes(r fiber.Router) {
	r.Post("/reset", s.reset)
	r.Post("/buy", s.act(func(ctx context.Context, c *fiber.Ctx, me custmodels.Customer) error {
		return s.a.Billing.Purchase(ctx, me.ID, me.Handle, c.FormValue("o"), false)
	}))
	r.Post("/advance", s.act(func(ctx context.Context, c *fiber.Ctx, _ custmodels.Customer) error {
		d, err := strconv.Atoi(c.FormValue("days"))
		if err != nil || d < 1 || d > 400 { // the shared clock only moves forward, at most about a year at once
			return fmt.Errorf("days must be 1..400")
		}
		if err := s.a.Billing.Advance(ctx, d); err != nil {
			return err
		}
		s.a.Sweep(ctx)
		return nil
	}))
	r.Post("/cancel", s.act(func(ctx context.Context, c *fiber.Ctx, _ custmodels.Customer) error {
		return s.a.Billing.Cancel(ctx, c.FormValue("sub"))
	}))
	r.Post("/refund", s.act(func(ctx context.Context, c *fiber.Ctx, _ custmodels.Customer) error {
		return s.a.Billing.Refund(ctx, c.FormValue("sub"))
	}))
	r.Post("/charge", s.act(s.simCharge))
	r.Post("/race", s.act(s.simRace))
	r.Post("/retry", s.act(s.simRetry))
	r.Post("/flag", s.act(s.simFlag))
	r.Post("/wh/dup", s.act(func(ctx context.Context, _ *fiber.Ctx, me custmodels.Customer) error {
		return s.a.Billing.DuplicatePaid(ctx, me.ID)
	}))
	r.Post("/wh/out-of-order", s.act(func(ctx context.Context, c *fiber.Ctx, me custmodels.Customer) error {
		s.a.Hub.Log("info", "scenario: invoice.paid arrives before subscription.created")
		return s.a.Billing.Purchase(ctx, me.ID, me.Handle, c.FormValue("o"), true)
	}))
	r.Post("/wh/late-created", s.act(func(ctx context.Context, _ *fiber.Ctx, me custmodels.Customer) error {
		return s.a.Billing.LateCreated(ctx, me.ID)
	}))
	r.Post("/wh/past-due", s.act(func(ctx context.Context, _ *fiber.Ctx, me custmodels.Customer) error {
		return s.a.Billing.PastDue(ctx, me.ID)
	}))
	r.Post("/recon/phantom", s.act(func(ctx context.Context, _ *fiber.Ctx, _ custmodels.Customer) error { return s.a.Billing.Phantom(ctx) }))
	r.Post("/recon/silent-refund", s.act(func(ctx context.Context, _ *fiber.Ctx, _ custmodels.Customer) error {
		return s.a.Billing.SilentRefund(ctx)
	}))
	r.Post("/recon/fix", s.act(func(ctx context.Context, c *fiber.Ctx, _ custmodels.Customer) error {
		return s.a.Billing.FixRecon(ctx, c.FormValue("ref"))
	}))
}

// act resolves the customer (form field c, a handle), runs fn, logs an error and tells the pages to refresh.
func (s *Server) act(fn func(context.Context, *fiber.Ctx, custmodels.Customer) error) fiber.Handler {
	return func(c *fiber.Ctx) error {
		me, _ := s.a.Customers.ByHandle(c.Context(), c.FormValue("c", "alice"))
		if err := fn(context.Background(), c, me); err != nil {
			s.a.Hub.Log("err", "%v", err)
		}
		s.a.Hub.Changed()
		return c.SendStatus(204)
	}
}

// reset is guarded by RESET_PASSWORD on the public deploy; the browser asks for it via hx-prompt.
func (s *Server) reset(c *fiber.Ctx) error {
	// Without RESET_PASSWORD only a local run may reset: a deploy that lost its .env stays closed.
	want := os.Getenv("RESET_PASSWORD")
	if want == "" && !c.IsFromLocal() ||
		want != "" && subtle.ConstantTimeCompare([]byte(c.Get("HX-Prompt")), []byte(want)) != 1 {
		s.a.Hub.Log("warn", "reset refused: wrong password")
		return c.SendStatus(403)
	}
	if err := s.a.Reset(context.Background()); err != nil {
		s.a.Hub.Log("err", "%v", err)
	}
	s.a.Hub.Log("info", "reset: data back to the seed, new ids — open pages reload")
	// Seeds get fresh uuids, so buttons on already open pages point at ids that no longer exist.
	s.a.Hub.Pub("reload", "1")
	return c.SendStatus(204)
}

func (s *Server) simFlag(_ context.Context, c *fiber.Ctx, _ custmodels.Customer) error {
	f := s.a.Faults
	switch c.FormValue("f") {
	case "fail":
		f.FailAI.Add(1)
		s.a.Hub.Log("warn", "next AI call will fail with provider 5xx")
	case "block":
		f.BlockAI.Add(1)
		s.a.Hub.Log("warn", "next AI call will be blocked by content policy")
	case "crash":
		f.CrashAI.Add(1)
		s.a.Hub.Log("warn", "next AI call: the process dies after the debit — the charge stays in_progress; the sweeper returns the credits after %s", app.ChargeTimeout)
	case "lose":
		f.LoseWebhook.Add(1)
		s.a.Hub.Log("warn", "next invoice.paid webhook will be lost")
	}
	return nil
}
