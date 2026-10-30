package app_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Drift guards: the schema, its descriptions and the model board must describe the same tables.

func TestEveryTableAndKeyColumnIsDescribed(t *testing.T) {
	e := newEnv(t)
	rows, err := e.a.Pool.Query(t.Context(), `SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND obj_description(c.oid, 'pg_class') IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		t.Errorf("table %s has no COMMENT ON TABLE (internal/platform/schema/comments.sql)", name)
	}
}

// simulatorOnly are tables of the simulation harness, not of the design; they are not on the model board.
var simulatorOnly = map[string]bool{"webhook_events": true, "provider_txns": true, "sim_clock": true}

func TestBoardShowsEveryDesignTable(t *testing.T) {
	e := newEnv(t)
	js, err := os.ReadFile("../web/static/board.js")
	if err != nil {
		t.Fatal(err)
	}
	// the changed-model definition: t("credit_lot", "CREDIT_LOT", ...)
	i := strings.Index(string(js), "changed: {")
	j := strings.Index(string(js), "charge: {")
	onBoard := map[string]bool{}
	for _, m := range regexp.MustCompile(`t\("([a-z_]+)"`).FindAllStringSubmatch(string(js)[i:j], -1) {
		onBoard[m[1]] = true
	}
	rows, err := e.a.Pool.Query(t.Context(), `SELECT tablename FROM pg_tables WHERE schemaname='public'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		if simulatorOnly[name] {
			continue
		}
		singular := strings.TrimSuffix(strings.TrimSuffix(name, "s"), "e") // credit_lots → credit_lot, action_prices → action_pric
		if !onBoard[strings.TrimSuffix(name, "s")] && !onBoard[name] && !onBoard[singular] && !onBoard[singular+"e"] {
			t.Errorf("table %s is not on the changed-model board (internal/web/static/board.js DEFS.changed)", name)
		}
	}
}

// Every column on the changed-model board must exist in the schema, every removed ("- ") column must not,
// and every schema column of a board table must be on the board. Catches a board that drifts from schema.sql.
func TestBoardFieldsMatchSchema(t *testing.T) {
	e := newEnv(t)
	js, err := os.ReadFile("../web/static/board.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	src = src[strings.Index(src, "changed: {"):strings.Index(src, "charge: {")]
	tableRe := regexp.MustCompile(`t\("([a-z_]+)", "[^"]*", "[a-z]+", \d+, \d+, \[([^\]]*)\]\)`)
	rowRe := regexp.MustCompile(`"([^"]*)"`)
	for _, m := range tableRe.FindAllStringSubmatch(src, -1) {
		table := dbTable(e, m[1])
		cols := map[string]bool{}
		rows, err := e.a.Pool.Query(t.Context(), `SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, table)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var c string
			_ = rows.Scan(&c)
			cols[c] = true
		}
		rows.Close()
		onBoard := map[string]bool{}
		for _, r := range rowRe.FindAllStringSubmatch(m[2], -1) {
			row := r[1]
			removed := strings.HasPrefix(row, "- ")
			f := strings.Fields(strings.TrimLeft(row, "+~- "))
			if len(f) < 2 {
				continue
			}
			name := f[1]
			onBoard[name] = true
			switch {
			case removed && cols[name]:
				t.Errorf("%s.%s is marked removed on the board but exists in the schema", table, name)
			case !removed && !cols[name] && !boardHidden[table+"."+name]:
				t.Errorf("%s.%s is on the board but not in the schema", table, name)
			}
		}
		for c := range cols {
			if !onBoard[c] && !boardHidden[table+"."+c] {
				t.Errorf("%s.%s is in the schema but not on the board", table, c)
			}
		}
	}
}

// Known differences between the design board and the MVP schema, each with its reason.
var boardHidden = map[string]bool{
	// readable seed codes and handles: simulator URLs and buttons, not part of the design
	"applications.code": true, "credit_packages.code": true, "offering_items.code": true, "offerings.code": true,
	"plans.code": true, "customers.handle": true,
	// the current model has vendor_id and status; the MVP keeps a vendor name and no application status
	"applications.vendor": true, "applications.vendor_id": true, "applications.status": true,
	// the current model has a customer status; the MVP has no customer lifecycle
	"customers.status": true,
}

func dbTable(e *env, id string) string {
	for _, c := range []string{id + "s", id + "es", id} {
		var ok bool
		_ = e.a.Pool.QueryRow(e.t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='public' AND tablename=$1)`, c).Scan(&ok)
		if ok {
			return c
		}
	}
	e.t.Fatalf("board table %s has no schema table", id)
	return ""
}

// The board draws the "one" end of a nullable FK as "zero or one". Its list of nullable FKs must equal the schema's.
func TestBoardCardinalityMatchesSchema(t *testing.T) {
	e := newEnv(t)
	js, err := os.ReadFile("../web/static/board.js")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`NULLABLE_FK = new Set\(\[([^\]]*)\]`).FindStringSubmatch(string(js))
	if m == nil {
		t.Fatal("NULLABLE_FK not found in board.js")
	}
	onBoard := map[string]bool{}
	for _, q := range regexp.MustCompile(`"([a-z_]+)\.([a-z_]+)"`).FindAllStringSubmatch(m[1], -1) {
		onBoard[dbTable(e, q[1])+"."+q[2]] = true
	}
	rows, err := e.a.Pool.Query(t.Context(), `SELECT cl.relname || '.' || a.attname FROM pg_constraint c
		JOIN pg_class cl ON cl.oid = c.conrelid JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		WHERE c.contype = 'f' AND NOT a.attnotnull`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	inSchema := map[string]bool{}
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		inSchema[k] = true
		if !onBoard[k] {
			t.Errorf("FK %s is nullable in the schema but drawn as exactly-one on the board (add it to NULLABLE_FK)", k)
		}
	}
	for k := range onBoard {
		if !inSchema[k] {
			t.Errorf("NULLABLE_FK lists %s, but the schema has it NOT NULL", k)
		}
	}
}
