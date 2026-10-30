// Package archcheck enforces the domain layout: internal/<domain>/{iface.go, models, storages, services}.
// Cross-domain calls go through the caller's own iface.go and the other domain's models, never its storages or services.
package archcheck

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "bundle-credit-test/internal/"

var domains = map[string]bool{"catalog": true, "customer": true, "license": true, "credits": true, "billing": true, "board": true}

// pkgImports maps "credits/services" → internal imports of that package (without the module prefix).
func pkgImports(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	root := ".."
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		rel = filepath.ToSlash(rel)
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if strings.HasPrefix(p, module) {
				out[rel] = append(out[rel], strings.TrimPrefix(p, module))
			}
		}
		if _, ok := out[rel]; !ok {
			out[rel] = nil
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// split returns domain and layer: "credits/services" → ("credits", "services"); "credits" → ("credits", "").
func split(pkg string) (string, string) {
	d, l, _ := strings.Cut(pkg, "/")
	return d, l
}

func allowed(from, to string) (bool, string) {
	fd, fl := split(from)
	td, tl := split(to)
	switch {
	case fd == "platform":
		return td == "platform", "platform imports only platform"
	case fd == "app" || fd == "web":
		return td != "archcheck", ""
	case !domains[fd]:
		return true, ""
	}
	if td == "app" || td == "web" {
		return false, "a domain never imports the app or web layer"
	}
	if td == "platform" {
		return fl != "models", "models import nothing"
	}
	if fl == "models" {
		return false, "models import nothing"
	}
	same := fd == td
	switch fl {
	case "storages":
		return same && tl == "models", "storages import only their own models and platform"
	case "", "services":
		if same {
			return tl == "" || tl == "models", "services use their own iface and models, never their own storages"
		}
		return tl == "models", "another domain is reached through its models and our iface, never its root, storages or services"
	}
	return false, "unknown layer " + fl
}

func TestLayerRules(t *testing.T) {
	for from, imps := range pkgImports(t) {
		for _, to := range imps {
			if ok, why := allowed(from, to); !ok {
				t.Errorf("%s imports %s: %s", from, to, why)
			}
		}
	}
}

func TestDomainsHaveIface(t *testing.T) {
	for d := range domains {
		if _, err := os.Stat(filepath.Join("..", d, "iface.go")); err != nil {
			t.Errorf("domain %s has no iface.go", d)
		}
	}
}

func TestNoUnknownTopLevelPackages(t *testing.T) {
	entries, _ := os.ReadDir("..")
	for _, e := range entries {
		if e.IsDir() && !domains[e.Name()] && !map[string]bool{"platform": true, "app": true, "web": true, "archcheck": true}[e.Name()] {
			t.Errorf("internal/%s is not a known domain; add it to archcheck or fold it into one", e.Name())
		}
	}
}

func TestAllowedTable(t *testing.T) {
	for _, c := range []struct {
		from, to string
		ok       bool
	}{
		{"credits/services", "credits", true},
		{"credits/services", "credits/models", true},
		{"credits/services", "credits/storages", false},
		{"billing/services", "credits/models", true},
		{"billing/services", "credits/services", false},
		{"billing", "credits", false},
		{"credits/models", "platform/db", false},
		{"license/storages", "license/models", true},
		{"license/storages", "credits/models", false},
		{"platform/db", "credits/models", false},
		{"credits", "app", false},
	} {
		if ok, _ := allowed(c.from, c.to); ok != c.ok {
			t.Errorf("allowed(%s → %s) = %v, want %v", c.from, c.to, ok, c.ok)
		}
	}
}
