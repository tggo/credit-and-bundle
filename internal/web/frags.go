package web

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	billmodels "bundle-credit-test/internal/billing/models"
	catmodels "bundle-credit-test/internal/catalog/models"
	credmodels "bundle-credit-test/internal/credits/models"
	custmodels "bundle-credit-test/internal/customer/models"
)

type appView struct {
	catmodels.App
	Licensed         bool
	LicenseFrom      string
	Available, Total int64
	Sources          []credmodels.BalanceSource
}

type stateData struct {
	Customer string
	Me       custmodels.Customer
	Account  string
	Subs     []billmodels.SubView
	Apps     []appView
	Lots     []credmodels.Lot
	Ledger   []credmodels.LedgerEntry
	Charges  []credmodels.ChargeRow
	Total    int64
}

func (s *Server) fragState(c *fiber.Ctx) error {
	ctx := c.Context()
	me, err := s.customer(c)
	if err != nil {
		return c.SendString("unknown customer")
	}
	now := s.a.Clock.Now(ctx)
	d := stateData{Customer: me.Handle, Me: me}
	w, err := s.a.Credits.Wallet(ctx, me.ID)
	if err != nil {
		return c.SendString(err.Error())
	}
	d.Account, d.Lots, d.Ledger, d.Charges = w.Status, w.Lots, w.Ledger, w.Charges
	d.Subs, _ = s.a.Billing.Subscriptions(ctx, me.ID)
	apps, _ := s.a.Catalog.Apps(ctx)
	for _, a := range apps {
		v := appView{App: a, LicenseFrom: s.a.Licenses.ActiveFrom(ctx, me.ID, a.ID, now)}
		v.Licensed = v.LicenseFrom != ""
		if b, err := s.a.Credits.Balance(ctx, me.ID, a.ID); err == nil {
			v.Available, v.Total = b.Available, b.Total
			for _, src := range b.BySource {
				if src.AppliesHere {
					v.Sources = append(v.Sources, src)
				}
			}
			d.Total = b.Total
		}
		d.Apps = append(d.Apps, v)
	}
	return render(c, "state", d)
}

func (s *Server) fragWebhooks(c *fiber.Ctx) error {
	evs, err := s.a.Billing.Webhooks(c.Context())
	if err != nil {
		return c.SendString(err.Error())
	}
	subs, _ := s.a.Billing.Subscriptions(c.Context(), "")
	return render(c, "webhooks_frag", fiber.Map{"Events": evs, "Subs": subs})
}

func (s *Server) fragReconcile(c *fiber.Ctx) error {
	rows, n, err := s.a.Billing.Reconcile(c.Context())
	if err != nil {
		return c.SendString(err.Error())
	}
	prov, ours := s.a.Billing.NetTotals(c.Context())
	return render(c, "reconcile_frag", fiber.Map{"Rows": rows, "Checked": n, "Provider": prov, "Ours": ours})
}

func (s *Server) fragClock(c *fiber.Ctx) error {
	return c.SendString(s.a.Clock.Now(c.Context()).Format("Mon Jan 02 2006 15:04"))
}

// fragFlow renders the Mermaid sankey of the credit flow: lots on the left, where their credits went on the right.
func (s *Server) fragFlow(c *fiber.Ctx) error {
	me, err := s.customer(c)
	if err != nil {
		return c.SendString("unknown customer")
	}
	flows, err := s.a.Credits.Flows(c.Context(), me.ID)
	if err != nil {
		return c.SendString(err.Error())
	}
	clean := strings.NewReplacer(",", " ", "\"", "", "\n", " ", "·", "", "#", "", "+", " plus")
	var out []string
	for _, f := range flows {
		if f.Credits > 0 {
			out = append(out, fmt.Sprintf("\"%s\",\"%s\",%d", strings.Join(strings.Fields(clean.Replace(f.Lot)), " "), clean.Replace(f.Dest), f.Credits))
		}
	}
	return render(c, "flow_frag", fiber.Map{"Rows": out})
}
