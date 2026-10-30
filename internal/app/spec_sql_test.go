package app_test

// Query recording and plan checks for the spec report.
// Every statement the When step ran is explained again with enable_seqscan=off: Postgres then uses any index
// that can serve the query, so a Seq Scan left in the plan means the table has no index for it. Test tables are
// tiny, so a plain EXPLAIN would choose Seq Scan anyway and hide a missing index.

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// growing are the tables that grow with traffic; a Seq Scan there is a finding. The rest are small reference data.
var growing = map[string]bool{"credit_ledger": true, "credit_lots": true, "credit_charges": true, "credit_accounts": true,
	"payments": true, "subscriptions": true, "licenses": true, "webhook_events": true, "provider_txns": true,
	"customers": true}

type query struct {
	SQL      string
	Count    int
	Duration time.Duration
	Err      string
	Scans    []scan
}

// scan is one table access in a plan.
type scan struct {
	Node, Table, Index, Filter string
}

func (s scan) Seq() bool     { return s.Node == "Seq Scan" }
func (s scan) Finding() bool { return s.Seq() && growing[s.Table] }

func (q query) Findings() int {
	n := 0
	for _, s := range q.Scans {
		if s.Finding() {
			n++
		}
	}
	return n
}

type traced struct {
	sql   string
	args  []any
	start time.Time
}

// tracer records statements while on is set; identical SQL text is grouped.
type tracer struct {
	on   atomic.Bool
	mu   sync.Mutex
	seen []*query
	args map[string][]any
	byQ  map[string]*query
}

type traceKey struct{}

func (t *tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if !t.on.Load() {
		return ctx
	}
	return context.WithValue(ctx, traceKey{}, traced{sql: d.SQL, args: d.Args, start: time.Now()})
}

var (
	spaces  = regexp.MustCompile(`\s+`)
	literal = regexp.MustCompile(`'[^']*'(::[a-z ]+)?`)
)

func (t *tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	tr, ok := ctx.Value(traceKey{}).(traced)
	if !ok {
		return
	}
	sql := strings.TrimSpace(spaces.ReplaceAllString(tr.sql, " "))
	if !explainable(sql) {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byQ == nil {
		t.byQ, t.args = map[string]*query{}, map[string][]any{}
	}
	q, ok := t.byQ[sql]
	if !ok {
		q = &query{SQL: sql}
		t.byQ[sql], t.args[sql] = q, tr.args
		t.seen = append(t.seen, q)
	}
	q.Count++
	q.Duration += time.Since(tr.start)
	if d.Err != nil && q.Err == "" {
		q.Err = d.Err.Error()
	}
}

func explainable(sql string) bool {
	head := strings.ToLower(strings.SplitN(sql, " ", 2)[0])
	return head == "select" || head == "insert" || head == "update" || head == "delete" || head == "with"
}

// explain plans every recorded statement with its first arguments, in a transaction that is rolled back.
func (t *tracer) explain(e *env) []query {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]query, 0, len(t.seen))
	for _, q := range t.seen {
		q.Scans = plan(e, q.SQL, t.args[q.SQL])
		out = append(out, *q)
	}
	return out
}

func plan(e *env, sql string, args []any) []scan {
	ctx := context.Background()
	tx, err := e.a.Pool.Begin(ctx)
	if err != nil {
		return []scan{{Node: "explain failed: " + err.Error()}}
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		return []scan{{Node: "explain failed: " + err.Error()}}
	}
	var raw []byte
	if err := tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		return []scan{{Node: "explain failed: " + err.Error()}}
	}
	var doc []struct{ Plan node }
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc) == 0 {
		return []scan{{Node: "explain unreadable"}}
	}
	var scans []scan
	doc[0].Plan.walk(&scans)
	return scans
}

type node struct {
	Type   string `json:"Node Type"`
	Rel    string `json:"Relation Name"`
	Index  string `json:"Index Name"`
	Filter string `json:"Filter"`
	Cond   string `json:"Index Cond"`
	Plans  []node `json:"Plans"`
}

func (n node) walk(out *[]scan) {
	if n.Rel != "" && n.Type != "ModifyTable" {
		f := n.Filter
		if f == "" {
			f = n.Cond
		}
		f = literal.ReplaceAllString(f, "?") // uuids and timestamps differ per run; the shape is what matters
		*out = append(*out, scan{Node: n.Type, Table: n.Rel, Index: n.Index, Filter: f})
	}
	for _, c := range n.Plans {
		c.walk(out)
	}
}
