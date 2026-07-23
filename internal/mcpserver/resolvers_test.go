package mcpserver

import (
	"errors"
	"testing"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

type item struct{ id, name string }

func TestMatchByName(t *testing.T) {
	getID := func(i item) string { return i.id }
	getName := func(i item) string { return i.name }

	// single match
	r, err := matchByName([]item{{"1", "Alpha"}, {"2", "Beta"}}, getID, getName, "thing", "Beta")
	if err != nil || r.id != "2" {
		t.Errorf("single match: got %v %v", r, err)
	}
	// no match -> notFoundError
	if _, err := matchByName([]item{{"1", "Alpha"}}, getID, getName, "thing", "Zeta"); err == nil {
		t.Error("expected notFoundError")
	}
	// multiple matches -> ambiguousError
	if _, err := matchByName([]item{{"1", "Dupe"}, {"2", "Dupe"}}, getID, getName, "thing", "Dupe"); err == nil {
		t.Error("expected ambiguousError")
	}
}

func TestParseSequentialID(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"#123", 123, true},
		{"123", 123, true},
		{"Ref-123", 123, true},
		{"thi-1825", 1825, true},
		{"abc", 0, false},
		{"", 0, false},
		{"#0", 0, false},
	}
	for _, c := range cases {
		got, ok := parseSequentialID(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parseSequentialID(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIsResolveMiss(t *testing.T) {
	if !isResolveMiss(&favro.NotFoundError{APIError: &favro.APIError{Status: 404}}) {
		t.Error("404 should be a resolve miss")
	}
	if !isResolveMiss(&favro.AuthError{APIError: &favro.APIError{Status: 403}}) {
		t.Error("403 should be a resolve miss")
	}
	if isResolveMiss(&favro.APIError{Status: 500}) {
		t.Error("500 should NOT be a resolve miss")
	}
	if isResolveMiss(errors.New("other")) {
		t.Error("generic error should NOT be a resolve miss")
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		err      error
		wantKind string
	}{
		{&favro.NotFoundError{APIError: &favro.APIError{Status: 404}}, "not_found"},
		{&favro.AuthError{APIError: &favro.APIError{Status: 401}}, "authentication"},
		{&favro.AuthError{APIError: &favro.APIError{Status: 403}}, "forbidden"},
		{&favro.RateLimitError{APIError: &favro.APIError{Status: 429}}, "rate_limited"},
		{&favro.APIError{Status: 500}, "api_error"},
		{&notFoundError{entityType: "card", identifier: "x"}, "not_found"},
		{&ambiguousError{entityType: "card", name: "x"}, "ambiguous"},
	}
	for _, c := range cases {
		kind, _ := classifyError(c.err)
		if kind != c.wantKind {
			t.Errorf("classifyError(%v) kind=%s want %s", c.err, kind, c.wantKind)
		}
	}
}
