package web

import (
	"html/template"

	"github.com/gofiber/fiber/v2"

	catmodels "bundle-credit-test/internal/catalog/models"
	custmodels "bundle-credit-test/internal/customer/models"
)

type pageData struct {
	Page      string
	Customer  string // handle, used in URLs
	Me        custmodels.Customer
	Customers []custmodels.Customer
	Offerings []catmodels.Offering
	Contents  map[string][]string
	Apps      []catmodels.App
	Clock     string
	LogHTML   template.HTML
	NoSSE     bool // for headless screenshots
	Board     string
	V         string // asset version: static URLs change with every build, so caches never serve old files
}

// customer resolves the ?c= handle (default alice) to the customer row.
func (s *Server) customer(c *fiber.Ctx) (custmodels.Customer, error) {
	return s.a.Customers.ByHandle(c.Context(), c.Query("c", "alice"))
}

func (s *Server) page(c *fiber.Ctx, name string) error {
	ctx := c.Context()
	d := pageData{Page: name, Customer: c.Query("c", "alice"), Clock: s.a.Clock.Now(ctx).Format("Mon Jan 02 2006 15:04"),
		LogHTML: template.HTML(s.a.Hub.LogHTML()), NoSSE: c.Query("nosse") != "", Board: c.Query("b", "changed"), V: assetVersion}
	d.Me, _ = s.customer(c)
	d.Customers, _ = s.a.Customers.List(ctx)
	d.Offerings, _ = s.a.Catalog.Offerings(ctx)
	d.Contents, _ = s.a.Catalog.Contents(ctx)
	d.Apps, _ = s.a.Catalog.Apps(ctx)
	return render(c, name, d)
}
