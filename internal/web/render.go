package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

//go:embed templates/*.html
var tplFS embed.FS

//go:embed static
var staticFS embed.FS

//go:embed diagrams/*.svg
var diagFS embed.FS

// diagram returns a PlantUML SVG rendered at build time (diagrams/*.puml → *.svg), ready to inline.
func diagram(name string) template.HTML {
	b, err := diagFS.ReadFile("diagrams/" + name + ".svg")
	if err != nil {
		return template.HTML("<p>missing diagram " + template.HTMLEscapeString(name) + "</p>")
	}
	s := string(b)
	if i := strings.Index(s, "<svg"); i > 0 {
		s = s[i:]
	}
	s = strings.Replace(s, `preserveAspectRatio="none"`, `preserveAspectRatio="xMidYMid meet"`, 1)
	return template.HTML(s)
}

// assetVersion is the process start time; it goes on every /static URL.
var assetVersion = strconv.FormatInt(time.Now().Unix(), 36)

var tpl = template.Must(template.New("").Funcs(template.FuncMap{
	"money": func(m int64) string { return fmt.Sprintf("$%.2f", float64(m)/100) },
	"dt":    func(t time.Time) string { return t.Format("Jan 02 15:04") },
	"day":   func(t time.Time) string { return t.Format("Jan 02 2006") },
	"short": func(s string) string {
		if len(s) > 8 {
			return s[:8]
		}
		return s
	},
	"pct":     func(f float64) string { return fmt.Sprintf("%.0f%%", f*100) },
	"divf":    func(a, b int64) float64 { return float64(a) * 100 / float64(b) },
	"diagram": diagram,
}).ParseFS(tplFS, "templates/*.html"))

func render(c *fiber.Ctx, name string, data any) error {
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, name, data); err != nil {
		return c.Status(500).SendString(err.Error())
	}
	c.Type("html", "utf-8")
	return c.Send(buf.Bytes())
}

// logKind colours a log line by HTTP status.
func logKind(code int) string {
	switch {
	case code < 300:
		return "ok"
	case code < 500 && code != 402:
		return "warn"
	}
	return "err"
}
