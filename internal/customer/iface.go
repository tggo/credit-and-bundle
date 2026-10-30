// Package customer owns the customers table.
package customer

import (
	"context"

	"bundle-credit-test/internal/customer/models"
)

type Store interface {
	ByHandle(ctx context.Context, handle string) (models.Customer, error)
	ByID(ctx context.Context, id string) (models.Customer, error)
	List(ctx context.Context) ([]models.Customer, error)
}
