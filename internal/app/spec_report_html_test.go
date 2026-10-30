package app_test

import "html/template"

// reportTmpl is the self-contained HTML of -specreport.
var reportTmpl = template.Must(template.New("r").Funcs(template.FuncMap{
	"steps": func(r result) []string {
		var out []string
		for _, g := range r.Spec.Given {
			out = append(out, g.String())
		}
		return out
	},
}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Specs and query plans</title>
<style>
:root{--bg:#0f1115;--card:#171a21;--line:#262b36;--fg:#e6e8ee;--mute:#9aa3b2;--acc:#7aa2ff;--ok:#5fd39a;--warn:#f2b45a;--bad:#ff7a7a}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.55 -apple-system,Inter,sans-serif}
main{max-width:1100px;margin:0 auto;padding:28px 16px 60px}h1{margin:0 0 4px}h2{margin:36px 0 10px}
.sub{color:var(--mute)}.kpis{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:10px;margin:18px 0}
.kpi{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:12px 14px}.kpi b{font-size:26px;display:block}
table{width:100%;border-collapse:collapse;font-size:14px}th,td{text-align:left;padding:7px 9px;border-bottom:1px solid var(--line);vertical-align:top}
th{color:var(--mute);font-weight:600}code{font:12.5px ui-monospace,Menlo,monospace;color:#cfd4de}
.ok{color:var(--ok)}.bad{color:var(--bad)}.warn{color:var(--warn)}.mute{color:var(--mute)}
.spec{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:14px 16px;margin:12px 0}
.spec.fail{border-color:var(--bad)}.spec h3{margin:0 0 8px;font-size:16px}
.gwt{display:grid;grid-template-columns:70px 1fr;gap:2px 10px;margin-bottom:10px}.gwt span:nth-child(odd){color:var(--mute)}
details{margin-top:6px}summary{cursor:pointer;color:var(--acc)}
.pill{display:inline-block;font-size:12px;padding:1px 8px;border-radius:99px;border:1px solid var(--line);margin:1px 3px 1px 0}
.pill.idx{color:var(--ok);border-color:#2f5a45}.pill.seq{color:var(--bad);border-color:#6a3030}.pill.ref{color:var(--warn);border-color:#5a4a25}
.sql{white-space:pre-wrap;word-break:break-word}
.legend{font-size:13.5px;color:var(--mute);background:var(--card);border:1px solid var(--line);border-radius:10px;padding:10px 14px}
</style></head><body><main>
<h1>Specifications and query plans</h1>
<div class="sub">bundle-credit-test · real Postgres · generated {{.At}}</div>
<div class="kpis">
<div class="kpi"><b class="{{if .Failed}}bad{{else}}ok{{end}}">{{.Specs}}</b>specs, {{.Failed}} failed</div>
<div class="kpi"><b>{{.Queries}}</b>distinct SQL statements in When steps</div>
<div class="kpi"><b class="{{if .Finding}}bad{{else}}ok{{end}}">{{.Finding}}</b>seq scans on growing tables</div>
<div class="kpi"><b>{{len .Tables}}</b>tables touched</div>
</div>
<div class="legend">Every statement a When step ran is explained again with <code>enable_seqscan = off</code>, so
Postgres uses any index that can serve it. A <span class="bad">Seq Scan</span> left in that plan means
<b>no index fits the query</b>: harmless on small reference tables (<span class="warn">reference</span>), a finding
on tables that grow with traffic. Data: the spec's own rows; plans use the first arguments seen.</div>

<h2>Tables</h2>
<table><tr><th>Table</th><th>Accesses</th><th>Indexes used</th><th>Seq scans (no fitting index)</th></tr>
{{range .Tables}}<tr><td><code>{{.Table}}</code> {{if .Growing}}<span class="pill">growing</span>{{else}}<span class="pill ref">reference</span>{{end}}</td>
<td>{{.Total}}</td><td>{{range .Indexes}}<span class="pill idx">{{.}}</span>{{else}}<span class="mute">—</span>{{end}}</td>
<td>{{if .SeqScans}}<b class="{{if .Growing}}bad{{else}}warn{{end}}">{{.SeqScans}}</b>{{range .Filters}}<br><code>{{.}}</code>{{end}}{{else}}<span class="ok">0</span>{{end}}</td></tr>
{{end}}</table>

{{range .Suites}}<h2>{{.Title}}</h2>
{{range .Results}}<div class="spec {{if .Issues}}fail{{end}}">
<h3>{{if .Issues}}<span class="bad">✗</span>{{else}}<span class="ok">✓</span>{{end}} {{.Spec.Name}}</h3>
<div class="gwt">{{range $i, $g := steps .}}<span>{{if $i}}and{{else}}Given{{end}}</span><span>{{$g}}</span>{{end}}
<span>When</span><span><b>{{.Spec.When}}</b></span>
<span>Then</span><span>{{if .Spec.Then.Status}}response {{.Spec.Then.Status}}{{end}}
{{range .Spec.Then.Ledger}}<br>ledger: {{.Type}} {{.Delta}} on “{{.Lot}}”{{if .Charge}} for {{.Charge}}{{end}}{{end}}
{{if not .Spec.Then.Ledger}}<br>no new ledger entries{{end}}
{{range $k, $v := .Spec.Then.Lots}}<br>“{{$k}}” has {{$v}} left{{end}}</span></div>
{{range .Issues}}<div class="bad"><code>{{.}}</code></div>{{end}}
<details><summary>{{len .Queries}} SQL statements</summary>
<table><tr><th>SQL</th><th>×</th><th>Plan</th></tr>
{{range .Queries}}<tr><td class="sql"><code>{{.SQL}}</code>{{if .Err}}<div class="bad">{{.Err}}</div>{{end}}</td><td>{{.Count}}</td>
<td>{{range .Scans}}{{if .Seq}}<span class="pill {{if .Finding}}seq{{else}}ref{{end}}">Seq Scan {{.Table}}</span>{{if .Filter}}<br><code>{{.Filter}}</code>{{end}}
{{else if .Index}}<span class="pill idx">{{.Table}} → {{.Index}}</span>{{else}}<span class="pill">{{.Node}} {{.Table}}</span>{{end}}<br>{{else}}<span class="mute">no table scan</span>{{end}}</td></tr>
{{end}}</table></details>
</div>{{end}}{{end}}
</main></body></html>`))
