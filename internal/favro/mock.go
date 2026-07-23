package favro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
)

// idSegRe matches id-shaped path segments so a mock can match by template.
var idSegRe = regexp.MustCompile(`^[0-9a-f]{24}$|^[A-Za-z0-9]{16,}$`)

// Fixture is one recorded request/response pair (matches testdata/fixtures JSON).
type Fixture struct {
	Request struct {
		Method string `json:"method"`
		URL    string `json:"url"`
		Body   any    `json:"body"`
	} `json:"request"`
	Response struct {
		Status int `json:"status"`
		Body   any `json:"body"`
	} `json:"response"`
}

// LoadFixtures decodes fixture JSON files (concatenated or per-file) into a slice.
func LoadFixtures(files [][]byte) ([]Fixture, error) {
	var out []Fixture
	for _, raw := range files {
		var f Fixture
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("parse fixture: %w", err)
		}
		out = append(out, f)
	}
	return out, nil
}

// templatePath collapses id-shaped segments to {id} so requests with any id
// value match the recorded route shape.
func templatePath(p string) string {
	parts := strings.Split(strings.TrimPrefix(p, "/api/v1"), "/")
	for i, s := range parts {
		if idSegRe.MatchString(s) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

// MockServer returns an httptest.Server that replays the given fixtures by
// (method, path-template). The first matching fixture wins. Unmatched requests
// get 404. It also accepts custom handlers to override/add routes.
func MockServer(t interface {
	Helper()
	Logf(format string, args ...any)
	Fatalf(format string, args ...any)
}, fixtures []Fixture) *httptest.Server {
	t.Helper()
	type key struct{ method, tpl string }
	idx := map[key]Fixture{}
	for _, f := range fixtures {
		u, err := url.Parse(f.Request.URL)
		if err != nil {
			continue
		}
		k := key{strings.ToUpper(f.Request.Method), templatePath(u.Path)}
		if _, ok := idx[k]; !ok {
			idx[k] = f
		}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := key{strings.ToUpper(r.Method), templatePath(r.URL.Path)}
		f, ok := idx[k]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if f.Response.Status == 0 {
			f.Response.Status = 200
		}
		if f.Response.Body == nil {
			w.WriteHeader(f.Response.Status)
			return
		}
		b, err := json.Marshal(f.Response.Body)
		if err != nil {
			t.Fatalf("marshal mock body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.Response.Status)
		w.Write(b)
	}))
}

// WithBaseURL temporarily overrides BaseURL for the duration of a test.
func WithBaseURL(u string) (restore func()) {
	prev := BaseURL
	BaseURL = u
	return func() { BaseURL = prev }
}
