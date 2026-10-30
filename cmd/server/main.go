// Command server runs the Credits & Bundles simulator: pages, credits API, boards, SSE.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"bundle-credit-test/internal/app"
	"bundle-credit-test/internal/web"
)

func main() {
	dsn := getenv("DATABASE_URL", "postgres://bc:bc@localhost:55433/bundlecredit?sslmode=disable&timezone=UTC")
	addr := getenv("ADDR", ":8095")
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	for i := 0; i < 30 && pool.Ping(ctx) != nil; i++ {
		time.Sleep(time.Second)
	}
	a := app.New(pool, getenv("PLANTUML_URL", "http://localhost:18095"))
	if err := a.Start(ctx, os.Getenv("KEEP_DATA") != ""); err != nil {
		log.Fatalf("start: %v", err)
	}
	go a.RunSweeper(ctx, 15*time.Second)
	log.Printf("listening on http://localhost%s", addr)
	log.Fatal(web.New(a).Routes().Listen(addr))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
