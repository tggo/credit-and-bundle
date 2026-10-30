package app_test

// Field-path diff and the plain-text form of a spec.

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// issue is one difference: where it is, what the spec wants and what the run produced.
type issue struct {
	Path      string
	Want, Got any
}

func (i issue) String() string {
	return fmt.Sprintf("%s: want %v, got %v", i.Path, show(i.Want), show(i.Got))
}

func show(v any) string {
	if v == nil {
		return "nothing"
	}
	return fmt.Sprintf("%+v", v)
}

// diff walks structs, slices and maps and reports leaf differences by path, e.g. "Ledger[1].Delta".
func diff(path string, want, got any) []issue {
	return walk(path, reflect.ValueOf(want), reflect.ValueOf(got))
}

func walk(path string, w, g reflect.Value) []issue {
	switch w.Kind() {
	case reflect.Struct:
		var out []issue
		for i := 0; i < w.NumField(); i++ {
			out = append(out, walk(join(path, w.Type().Field(i).Name), w.Field(i), g.Field(i))...)
		}
		return out
	case reflect.Slice:
		var out []issue
		for i := 0; i < max(w.Len(), g.Len()); i++ {
			p := fmt.Sprintf("%s[%d]", path, i)
			switch {
			case i >= g.Len():
				out = append(out, issue{p, w.Index(i).Interface(), nil})
			case i >= w.Len():
				out = append(out, issue{p, nil, g.Index(i).Interface()})
			default:
				out = append(out, walk(p, w.Index(i), g.Index(i))...)
			}
		}
		return out
	case reflect.Map:
		var out []issue
		for _, k := range w.MapKeys() {
			p := fmt.Sprintf("%s[%q]", path, k.Interface())
			gv := g.MapIndex(k)
			if !gv.IsValid() {
				out = append(out, issue{p, w.MapIndex(k).Interface(), nil})
				continue
			}
			out = append(out, walk(p, w.MapIndex(k), gv)...)
		}
		return out
	default:
		if !reflect.DeepEqual(w.Interface(), g.Interface()) {
			return []issue{{path, w.Interface(), g.Interface()}}
		}
		return nil
	}
}

func join(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}

// text is the spec as a paragraph a product owner can read; failed specs carry their differences.
func (sp spec) text(issues []issue) string {
	var b strings.Builder
	mark := "✓"
	if len(issues) > 0 {
		mark = "✗"
	}
	fmt.Fprintf(&b, "## %s %s\n\n", mark, sp.Name)
	for i, g := range sp.Given {
		lead := "And"
		if i == 0 {
			lead = "Given"
		}
		fmt.Fprintf(&b, "- %s %s\n", lead, g)
	}
	fmt.Fprintf(&b, "- **When** %s\n", sp.When)
	th := sp.Then
	if th.Status != 0 {
		fmt.Fprintf(&b, "- **Then** the response is %d\n", th.Status)
	}
	if len(th.Ledger) == 0 {
		b.WriteString("- the ledger gets no new entries\n")
	}
	for _, x := range th.Ledger {
		fmt.Fprintf(&b, "- ledger: %s %+d on %q", x.Type, x.Delta, x.Lot)
		if x.Charge != "" {
			fmt.Fprintf(&b, " for %s", x.Charge)
		}
		b.WriteString("\n")
	}
	labels := make([]string, 0, len(th.Lots))
	for l := range th.Lots {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	for _, l := range labels {
		fmt.Fprintf(&b, "- %q has %d left\n", l, th.Lots[l])
	}
	for _, is := range issues {
		fmt.Fprintf(&b, "- ✗ `%s`\n", is)
	}
	b.WriteString("\n")
	return b.String()
}
