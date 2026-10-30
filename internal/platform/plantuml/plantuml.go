// Package plantuml renders PlantUML source to SVG through a self-hosted plantuml-server (docker, POST /svg).
package plantuml

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"
)

type Client struct {
	URL  string
	http *http.Client
}

func New(url string) *Client { return &Client{URL: url, http: &http.Client{Timeout: 15 * time.Second}} }

// ErrSyntax carries PlantUML's own message; the server answers 200 with an error image, so it is read from the SVG.
type ErrSyntax struct{ Msg string }

func (e ErrSyntax) Error() string { return e.Msg }

var errText = regexp.MustCompile(`>([^<]*(?:Syntax Error|error)[^<]*)<`)

func (c *Client) Render(ctx context.Context, src string) ([]byte, error) {
	if c == nil || c.URL == "" {
		return nil, errors.New("plantuml server not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/svg", bytes.NewBufferString(src))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	svg, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if e := res.Header.Get("X-PlantUML-Diagram-Error"); e != "" || res.StatusCode >= 300 {
		if e == "" {
			e = res.Status
		}
		if line := res.Header.Get("X-PlantUML-Diagram-Error-Line"); line != "" {
			e += " — line " + line // counted from @startuml, after the skin is inlined
		}
		return nil, ErrSyntax{Msg: e}
	}
	if bytes.Contains(svg, []byte("Syntax Error")) {
		msg := "syntax error"
		if m := errText.FindSubmatch(svg); m != nil {
			msg = string(m[1])
		}
		return svg, ErrSyntax{Msg: msg}
	}
	return svg, nil
}
