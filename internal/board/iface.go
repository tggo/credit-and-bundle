// Package board owns the shared model boards: scenes saved with a revision, pushed to every open tab.
package board

import "context"

type Store interface {
	Get(ctx context.Context, id string) (scene []byte, rev int64, err error)
	Save(ctx context.Context, id string, scene []byte) (rev int64, changed bool, err error)
	Delete(ctx context.Context, id string) error

	// PlantUML sources edited on the Diagrams page; "" when the default is in use.
	DiagramSource(ctx context.Context, id string) (source string, rev int64, err error)
	SaveDiagram(ctx context.Context, id, source string) (rev int64, err error)
	DeleteDiagram(ctx context.Context, id string) error
}
