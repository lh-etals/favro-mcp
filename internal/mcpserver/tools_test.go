package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mockFavro starts an httptest server that maps "METHOD /path" (path without the
// /api/v1 prefix) to a response body (status 200), points favro.BaseURL at it,
// and sets dummy credentials. Returns a cleanup func.
func mockFavro(t *testing.T, routes map[string]any) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		body, ok := routes[r.Method+" "+path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s, ok := body.(statusBody); ok {
			w.WriteHeader(s.status)
			b, _ := json.Marshal(s.body)
			w.Write(b)
			return
		}
		b, _ := json.Marshal(body)
		w.Write(b)
	}))
	restore := favro.WithBaseURL(srv.URL + "/api/v1")
	t.Setenv("FAVRO_EMAIL", "e")
	t.Setenv("FAVRO_API_TOKEN", "t")
	return func() { restore(); srv.Close() }
}

type statusBody struct {
	status int
	body   any
}

func cardBody() any {
	return map[string]any{
		"cardId": "c1", "cardCommonId": "cc1", "sequentialId": 42, "name": "Fix login",
		"widgetCommonId": "b1", "columnId": "col1",
		"assignments":            []any{map[string]any{"userId": "u1"}},
		"detailedDescription":    "steps to reproduce",
	}
}

func TestToolListCards(t *testing.T) {
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
		"GET /widgets/b1":    map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"},
		"GET /cards": map[string]any{
			"requestId": "r1", "pages": 2,
			"entities": []any{
				map[string]any{"cardId": "c1", "sequentialId": 42, "name": "Fix login", "columnId": "col1"},
				map[string]any{"cardId": "c2", "sequentialId": 43, "name": "Add docs", "columnId": "col1"},
			},
		},
	})
	defer cleanup()

	s := NewServer()
	res, _, err := s.listCards(context.Background(), &mcp.CallToolRequest{}, listCardsArgs{Board: "b1"})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"board: Tasks", "pages: 2", "Fix login", "seq: 42"} {
		if !strings.Contains(out, want) {
			t.Errorf("list_cards output missing %q:\n%s", want, out)
		}
	}
}

func TestToolListCardsQuery(t *testing.T) {
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
		"GET /widgets/b1":    map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"},
		"GET /cards": map[string]any{"entities": []any{
			map[string]any{"cardId": "c1", "sequentialId": 42, "name": "Fix login"},
			map[string]any{"cardId": "c2", "sequentialId": 43, "name": "Add docs"},
		}},
	})
	defer cleanup()

	s := NewServer()
	q := "login"
	res, _, err := s.listCards(context.Background(), &mcp.CallToolRequest{}, listCardsArgs{Board: "b1", Query: &q})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(out, "query: login") || !strings.Contains(out, "Fix login") {
		t.Errorf("query frontmatter/result missing:\n%s", out)
	}
	if strings.Contains(out, "Add docs") {
		t.Errorf("non-matching card should be filtered out:\n%s", out)
	}
}

func TestToolGetCardDetails(t *testing.T) {
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
		"GET /widgets/b1":    map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"},
		"GET /cards/c1":      cardBody(),
		"GET /columns":       map[string]any{"entities": []any{map[string]any{"columnId": "col1", "name": "Done"}}},
		"GET /users":         map[string]any{"entities": []any{map[string]any{"userId": "u1", "name": "Jane Doe"}}},
		"GET /tags":          map[string]any{"entities": []any{}},
		"GET /tasklists":     map[string]any{"entities": []any{}},
		"GET /comments":      map[string]any{"entities": []any{}},
	})
	defer cleanup()

	s := NewServer()
	res, _, err := s.getCardDetails(context.Background(), &mcp.CallToolRequest{}, getCardDetailsArgs{Card: "c1", Board: strPtr("b1")})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	// frontmatter: title + resolved parent names
	for _, want := range []string{"title: Fix login", "board: Tasks", "card_id: c1", "seq: 42"} {
		if !strings.Contains(out, want) {
			t.Errorf("card detail frontmatter missing %q:\n%s", want, out)
		}
	}
	// body: name resolution (user_id -> name) + description
	if !strings.Contains(out, "**Assigned:** Jane Doe") {
		t.Errorf("expected assignee name resolved to Jane Doe:\n%s", out)
	}
	if !strings.Contains(out, "## Description") || !strings.Contains(out, "steps to reproduce") {
		t.Errorf("expected description section:\n%s", out)
	}
}

func TestToolErrorRendering(t *testing.T) {
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
		"GET /widgets/b1":    map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"},
		"GET /cards/c1":      statusBody{404, map[string]any{"message": "not found"}},
	})
	defer cleanup()

	s := NewServer()
	res, _, err := s.getCardDetails(context.Background(), &mcp.CallToolRequest{}, getCardDetailsArgs{Card: "c1", Board: strPtr("b1")})
	if err != nil {
		t.Fatalf("handler returned Go error instead of structured result: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true")
	}
	out := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(out, "kind: not_found") || !strings.Contains(out, "status: 404") {
		t.Errorf("error frontmatter wrong:\n%s", out)
	}
}

func strPtr(s string) *string { return &s }
