package web

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// Read-only Postgres viewer (the Data page): table list with row counts, rows of one table, live via SSE.

type tableInfo struct {
	Comment string
	Name    string
	Rows    int64
	Service string
}

// service groups tables the way the design doc does.
var tableService = map[string]string{
	"applications": "Application", "plans": "Catalog", "offerings": "Catalog", "offering_items": "Catalog",
	"credit_packages": "Catalog", "action_prices": "Catalog", "customers": "Customer",
	"subscriptions": "Billing (simulated)", "payments": "Billing (simulated)", "webhook_events": "Billing (simulated)",
	"provider_txns": "Payment provider (mock)", "licenses": "License",
	"credit_accounts": "Credits", "credit_lots": "Credits", "lot_scope_apps": "Credits",
	"credit_charges": "Credits", "credit_ledger": "Credits", "sim_clock": "Simulator",
}

func (s *Server) tables(c *fiber.Ctx) ([]tableInfo, error) {
	rows, err := s.a.Pool.Query(c.Context(), `SELECT table_name FROM information_schema.tables
		WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make([]tableInfo, 0, len(names))
	for _, n := range names {
		var cnt int64
		_ = s.a.Pool.QueryRow(c.Context(), `SELECT count(*) FROM `+pgx.Identifier{n}.Sanitize()).Scan(&cnt)
		var com string
		_ = s.a.Pool.QueryRow(c.Context(), `SELECT COALESCE(obj_description(to_regclass($1), 'pg_class'), '')`, "public."+n).Scan(&com)
		out = append(out, tableInfo{Name: n, Rows: cnt, Service: tableService[n], Comment: com})
	}
	rank := map[string]int{"Catalog": 0, "Credits": 1, "License": 2, "Customer": 3, "Application": 4,
		"Billing (simulated)": 5, "Payment provider (mock)": 6, "Simulator": 7}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank[out[i].Service], rank[out[j].Service]
		if out[i].Service == "" {
			ri = 99
		}
		if out[j].Service == "" {
			rj = 99
		}
		return ri < rj
	})
	return out, nil
}

type tableView struct {
	Head    []colInfo
	Schema  tableSchema
	View    string // rows | structure
	Name    string
	Cols    []string
	Rows    [][]string
	Total   int64
	Limit   int
	Order   string
	Filter  string
	Missing bool
}

func (s *Server) fragTable(c *fiber.Ctx) error {
	ctx := c.Context()
	name := c.Query("t")
	ts, err := s.tables(c)
	if err != nil {
		return c.SendString(err.Error())
	}
	known := false
	for _, t := range ts {
		known = known || t.Name == name
	}
	if !known {
		return render(c, "table_frag", tableView{Name: name, Missing: true})
	}
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	id := pgx.Identifier{name}.Sanitize()

	// newest first when the table has a time or serial column
	var order, orderCol string
	for _, col := range []string{"created_at", "received_at", "id"} {
		var ok bool
		_ = s.a.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, name, col).Scan(&ok)
		if ok {
			order, orderCol = " ORDER BY "+pgx.Identifier{col}.Sanitize()+" DESC", col
			break
		}
	}
	// optional customer filter on tables that carry customer_id or account_id
	where, args := "", []any{}
	if cust := c.Query("c"); cust != "" && c.Query("only") == "1" {
		var hasCust, hasAcc bool
		_ = s.a.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name='customer_id')`, name).Scan(&hasCust)
		_ = s.a.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name='account_id')`, name).Scan(&hasAcc)
		switch {
		case hasCust:
			where, args = " WHERE customer_id = (SELECT id FROM customers WHERE handle = $1)", []any{cust}
		case hasAcc:
			where, args = " WHERE account_id = (SELECT a.id FROM credit_accounts a JOIN customers c ON c.id = a.customer_id WHERE c.handle = $1)", []any{cust}
		}
	}
	sch, err := s.tableSchema(ctx, name)
	if err != nil {
		return c.SendString(err.Error())
	}
	v := tableView{Schema: sch, View: c.Query("view", "rows"), Name: name, Limit: limit, Order: orderCol, Filter: strings.TrimPrefix(where, " WHERE ")}
	_ = s.a.Pool.QueryRow(ctx, `SELECT count(*) FROM `+id+where, args...).Scan(&v.Total)

	rows, err := s.a.Pool.Query(ctx, `SELECT * FROM `+id+where+order+` LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		return c.SendString(err.Error())
	}
	defer rows.Close()
	for _, f := range rows.FieldDescriptions() {
		v.Cols = append(v.Cols, f.Name)
		if c := sch.colByName[f.Name]; c != nil {
			v.Head = append(v.Head, *c)
		} else {
			v.Head = append(v.Head, colInfo{Name: f.Name})
		}
	}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return c.SendString(err.Error())
		}
		row := make([]string, len(vals))
		for i, x := range vals {
			row[i] = cell(x)
		}
		v.Rows = append(v.Rows, row)
	}
	return render(c, "table_frag", v)
}

func cell(x any) string {
	switch t := x.(type) {
	case nil:
		return "∅"
	case time.Time:
		return t.UTC().Format("2006-01-02 15:04:05")
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", t[0:4], t[4:6], t[6:8], t[8:10], t[10:16])
	case map[string]any, []any:
		b, _ := json.Marshal(t)
		return string(b)
	case []byte:
		return string(t)
	}
	return fmt.Sprint(x)
}

func (s *Server) fragTables(c *fiber.Ctx) error {
	ts, err := s.tables(c)
	if err != nil {
		return c.SendString(err.Error())
	}
	return render(c, "tables_frag", fiber.Map{"Tables": ts, "Sel": c.Query("t"), "Customer": c.Query("c")})
}
