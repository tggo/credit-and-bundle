package services

import (
	"context"
	"fmt"
	"time"

	"bundle-credit-test/internal/billing/models"
	credmodels "bundle-credit-test/internal/credits/models"
	licmodels "bundle-credit-test/internal/license/models"
	"bundle-credit-test/internal/platform/db"
)

// grant turns one paid period into rights: a license per plan item, a lot per credit item.
// Credits that come with apps are usable only in those apps; a pure credit pack works everywhere.
func (s *Service) grant(ctx context.Context, q db.Q, sub *models.Subscription, payID string, ev models.Event, now time.Time) error {
	o, err := s.catalog.Offering(ctx, q, sub.OfferingID)
	if err != nil {
		return err
	}
	items, err := s.catalog.Items(ctx, q, sub.OfferingID)
	if err != nil {
		return err
	}
	var apps []string
	for _, it := range items {
		if it.AppID != nil {
			apps = append(apps, *it.AppID)
		}
	}
	for _, it := range items {
		if it.Type == "plan" {
			if err := s.lic.Upsert(ctx, q, licmodels.Grant{SubscriptionID: sub.ID, CustomerID: sub.CustomerID,
				OfferingItemID: it.ID, AppID: *it.AppID, Due: ev.PeriodEnd}); err != nil {
				return err
			}
			continue
		}
		g := credmodels.Grant{CustomerID: sub.CustomerID, PaymentID: payID, OfferingItemID: it.ID, SubscriptionID: sub.ID,
			Label: fmt.Sprintf("%s · %s", o.Name, ev.PeriodStart.Format("Jan 02")), Scope: "platform", Source: "pack",
			Credits: *it.Credits, Expires: ev.PeriodEnd, At: now}
		switch {
		case it.PackApp != nil: // in-app pack: scoped to its own app, whatever else the offering contains
			g.Scope, g.Apps = "bundle", []string{*it.PackApp}
		case len(apps) > 0:
			g.Scope, g.Source, g.Apps = "bundle", "subscription", apps
			if o.Kind == "bundle" {
				g.Source = "bundle"
			}
		}
		if err := s.credits.GrantLot(ctx, q, g); err != nil {
			return err
		}
	}
	return nil
}
