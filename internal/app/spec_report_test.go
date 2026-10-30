package app_test

// Running spec suites and writing their outputs: -specdoc (plain text) and -specreport (HTML with query plans).

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	specDoc    = flag.String("specdoc", "", "write the specifications as plain text to this file")
	specReport = flag.String("specreport", "", "write the specifications, their SQL and query plans as HTML to this file")

	reportMu     sync.Mutex
	reportSuites []suite // every suite of this run; the files are rewritten after each one
)

type suite struct {
	Title   string
	Results []result
}

// runSpecs checks every spec as a subtest, then rewrites the requested outputs with all suites so far.
func runSpecs(t *testing.T, title string, specs []spec) {
	s := suite{Title: title}
	for _, sp := range specs {
		t.Run(sp.Name, func(t *testing.T) {
			r := sp.check(t)
			for _, is := range r.Issues {
				t.Errorf("%s", is)
			}
			for _, q := range r.Queries {
				if q.Err != "" {
					t.Errorf("query failed: %s: %s", q.Err, q.SQL)
				}
			}
			s.Results = append(s.Results, r)
		})
	}
	reportMu.Lock()
	defer reportMu.Unlock()
	reportSuites = append(reportSuites, s)
	if *specDoc != "" {
		var b strings.Builder
		for _, su := range reportSuites {
			fmt.Fprintf(&b, "# %s\n\n", su.Title)
			for _, r := range su.Results {
				b.WriteString(r.Spec.text(r.Issues))
			}
		}
		write(t, *specDoc, b.String())
	}
	if *specReport != "" {
		var b strings.Builder
		if err := reportTmpl.Execute(&b, newReport(reportSuites)); err != nil {
			t.Fatalf("specreport: %v", err)
		}
		write(t, *specReport, b.String())
	}
}

func write(t *testing.T, path, body string) {
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// report is what the HTML page shows: totals, every table access across all specs, then each spec.
type report struct {
	At                              string
	Specs, Failed, Queries, Finding int
	Tables                          []tableUse
	Suites                          []suite
}

// tableUse sums how a table was reached across all specs.
type tableUse struct {
	Table            string
	Growing          bool
	Indexes          []string
	SeqScans         int
	Filters          []string
	IndexHits, Total int
}

func newReport(suites []suite) report {
	rep := report{At: time.Now().Format("2006-01-02 15:04"), Suites: suites}
	byT := map[string]*tableUse{}
	var order []string
	for _, su := range suites {
		for _, r := range su.Results {
			rep.Specs++
			if len(r.Issues) > 0 {
				rep.Failed++
			}
			for _, q := range r.Queries {
				rep.Queries++
				rep.Finding += q.Findings()
				for _, sc := range q.Scans {
					if sc.Table == "" {
						continue
					}
					u, ok := byT[sc.Table]
					if !ok {
						u = &tableUse{Table: sc.Table, Growing: growing[sc.Table]}
						byT[sc.Table] = u
						order = append(order, sc.Table)
					}
					u.Total++
					if sc.Seq() {
						u.SeqScans++
						u.Filters = addOnce(u.Filters, sc.Filter)
					} else {
						u.IndexHits++
						u.Indexes = addOnce(u.Indexes, sc.Index)
					}
				}
			}
		}
	}
	for _, name := range order {
		rep.Tables = append(rep.Tables, *byT[name])
	}
	return rep
}

func addOnce(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func (r result) Text() string { return r.Spec.text(r.Issues) }
