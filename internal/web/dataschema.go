package web

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Table structure for the Data page, read from the Postgres catalog: columns, keys, references, indexes.

type colInfo struct {
	Name, Type, Default, Comment string
	NotNull                      bool
	PK, Unique                   bool
	FK                           *ref // this column references another table
	Checks                       []string
}

type ref struct{ Table, Column string }

type tableSchema struct {
	Comment   string
	Cols      []colInfo
	Checks    []string // table-level checks
	Uniques   []string // multi-column unique constraints
	RefBy     []ref    // other tables' columns that point here
	Indexes   []string
	DDL       string
	colByName map[string]*colInfo
}

func (s *Server) tableSchema(ctx context.Context, table string) (tableSchema, error) {
	var ts tableSchema
	ts.colByName = map[string]*colInfo{}
	rel := "public." + pgx.Identifier{table}.Sanitize()

	_ = s.a.Pool.QueryRow(ctx, `SELECT COALESCE(obj_description($1::regclass, 'pg_class'), '')`, rel).Scan(&ts.Comment)
	rows, err := s.a.Pool.Query(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull, COALESCE(pg_get_expr(d.adbin, d.adrelid), ''),
		COALESCE(col_description(a.attrelid, a.attnum), '')
		FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, rel)
	if err != nil {
		return ts, err
	}
	for rows.Next() {
		var c colInfo
		if err := rows.Scan(&c.Name, &c.Type, &c.NotNull, &c.Default, &c.Comment); err != nil {
			rows.Close()
			return ts, err
		}
		ts.Cols = append(ts.Cols, c)
	}
	rows.Close()
	for i := range ts.Cols {
		ts.colByName[ts.Cols[i].Name] = &ts.Cols[i]
	}

	// constraints of this table; column names resolved from conkey / confkey
	rows, err = s.a.Pool.Query(ctx, `SELECT c.contype::text, pg_get_constraintdef(c.oid),
		ARRAY(SELECT attname FROM pg_attribute WHERE attrelid = c.conrelid AND attnum = ANY(c.conkey) ORDER BY attnum),
		COALESCE(c.confrelid::regclass::text, ''),
		ARRAY(SELECT attname FROM pg_attribute WHERE attrelid = c.confrelid AND attnum = ANY(c.confkey) ORDER BY attnum)
		FROM pg_constraint c WHERE c.conrelid = $1::regclass ORDER BY c.contype, c.conname`, rel)
	if err != nil {
		return ts, err
	}
	for rows.Next() {
		var typ, def, refTable string
		var cols, refCols []string
		if err := rows.Scan(&typ, &def, &cols, &refTable, &refCols); err != nil {
			rows.Close()
			return ts, err
		}
		switch typ {
		case "p":
			for _, c := range cols {
				ts.colByName[c].PK = true
			}
		case "f":
			if len(cols) == 1 && len(refCols) == 1 {
				ts.colByName[cols[0]].FK = &ref{Table: strings.TrimPrefix(refTable, "public."), Column: refCols[0]}
			}
		case "u":
			if len(cols) == 1 {
				ts.colByName[cols[0]].Unique = true
			} else {
				ts.Uniques = append(ts.Uniques, "("+strings.Join(cols, ", ")+")")
			}
		case "c":
			if len(cols) == 1 {
				ts.colByName[cols[0]].Checks = append(ts.colByName[cols[0]].Checks, def)
			} else {
				ts.Checks = append(ts.Checks, def)
			}
		}
	}
	rows.Close()

	rows, err = s.a.Pool.Query(ctx, `SELECT c.conrelid::regclass::text,
		(SELECT attname FROM pg_attribute WHERE attrelid = c.conrelid AND attnum = c.conkey[1])
		FROM pg_constraint c WHERE c.contype = 'f' AND c.confrelid = $1::regclass ORDER BY 1`, rel)
	if err != nil {
		return ts, err
	}
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.Table, &r.Column); err != nil {
			rows.Close()
			return ts, err
		}
		r.Table = strings.TrimPrefix(r.Table, "public.")
		ts.RefBy = append(ts.RefBy, r)
	}
	rows.Close()

	rows, err = s.a.Pool.Query(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname='public' AND tablename=$1 ORDER BY indexname`, table)
	if err != nil {
		return ts, err
	}
	ts.Indexes, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return ts, err
	}
	ts.DDL, err = s.ddl(ctx, table, rel, ts)
	return ts, err
}

// ddl rebuilds CREATE TABLE from the catalog (columns + constraints as Postgres prints them).
func (s *Server) ddl(ctx context.Context, table, rel string, ts tableSchema) (string, error) {
	var b strings.Builder
	b.WriteString("CREATE TABLE " + table + " (\n")
	var lines []string
	for _, c := range ts.Cols {
		l := "  " + c.Name + " " + c.Type
		if c.NotNull {
			l += " NOT NULL"
		}
		if c.Default != "" {
			l += " DEFAULT " + c.Default
		}
		lines = append(lines, l)
	}
	rows, err := s.a.Pool.Query(ctx, `SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = $1::regclass
		ORDER BY CASE contype WHEN 'p' THEN 0 WHEN 'u' THEN 1 WHEN 'f' THEN 2 ELSE 3 END, conname`, rel)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return "", err
		}
		lines = append(lines, "  CONSTRAINT "+name+" "+def)
	}
	b.WriteString(strings.Join(lines, ",\n"))
	b.WriteString("\n);")
	return b.String(), rows.Err()
}

// schemaComments serves every table and column description (COMMENT ON) for the board's side panel,
// so the board explains a table with the same text as the Data page.
func (s *Server) schemaComments(c *fiber.Ctx) error {
	type tbl struct {
		Comment string            `json:"comment"`
		Cols    map[string]string `json:"cols"`
	}
	out := map[string]*tbl{}
	rows, err := s.a.Pool.Query(c.Context(), `SELECT c.relname, COALESCE(obj_description(c.oid, 'pg_class'), ''),
		COALESCE(a.attname, ''), COALESCE(col_description(c.oid, a.attnum), '')
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		LEFT JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		WHERE c.relkind = 'r'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var t, tc, col, cc string
		if err := rows.Scan(&t, &tc, &col, &cc); err != nil {
			return err
		}
		if out[t] == nil {
			out[t] = &tbl{Comment: tc, Cols: map[string]string{}}
		}
		if cc != "" {
			out[t].Cols[col] = cc
		}
	}
	return c.JSON(out)
}
