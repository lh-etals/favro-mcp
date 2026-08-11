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
		"assignments":         []any{map[string]any{"userId": "u1"}},
		"detailedDescription": "steps to reproduce",
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

func fullCardBody() any {
	return map[string]any{
		"cardId": "c1", "cardCommonId": "cc1", "sequentialId": 42, "name": "Fix login",
		"widgetCommonId": "b1", "columnId": "col1", "laneId": "lane1",
		"tags":      []any{"tag1"},
		"startDate": "2025-01-01T00:00:00.000Z",
		"dueDate":   "2025-02-01T00:00:00.000Z",
		"createdAt": "2024-12-01T00:00:00.000Z",
		"assignments": []any{
			map[string]any{"userId": "u1", "completed": true},
		},
		"customFields": []any{
			map[string]any{"customFieldId": "cf1", "value": "High"},
		},
		"detailedDescription": "steps to reproduce",
	}
}

func TestToolListCardsDetailSummaryUnchanged(t *testing.T) {
	// Same fixture as TestToolListCardsDetailFull, but no detail arg: output
	// must carry none of the full-mode fields, guarding that detail is opt-in.
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
		"GET /widgets/b1":    map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"},
		"GET /cards":         map[string]any{"entities": []any{fullCardBody()}},
	})
	defer cleanup()

	s := NewServer()
	res, _, err := s.listCards(context.Background(), &mcp.CallToolRequest{}, listCardsArgs{Board: "b1"})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	for _, unwanted := range []string{"card_common_id", "column_name", "custom_fields", "description_hash", "created_at"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("summary (default) output should not contain %q:\n%s", unwanted, out)
		}
	}
}

func TestToolListCardsDetailFull(t *testing.T) {
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
		"GET /widgets/b1":    map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"},
		"GET /cards":         map[string]any{"entities": []any{fullCardBody()}},
		"GET /columns":       map[string]any{"entities": []any{map[string]any{"columnId": "col1", "name": "Done"}}},
	})
	defer cleanup()

	s := NewServer()
	detail := "full"
	res, _, err := s.listCards(context.Background(), &mcp.CallToolRequest{}, listCardsArgs{Board: "b1", Detail: &detail})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	wantHash := contentHash("steps to reproduce")
	for _, want := range []string{
		"card_common_id: cc1", "column_name: Done", "lane: lane1",
		"user_id: u1", "completed: true",
		"id: cf1", "value: High",
		`start_date: "2025-01-01T00:00:00.000Z"`, `due_date: "2025-02-01T00:00:00.000Z"`,
		`created_at: "2024-12-01T00:00:00.000Z"`,
		"description_hash: " + wantHash,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail=full output missing %q:\n%s", want, out)
		}
	}
}

func TestToolListCardsInvalidDetail(t *testing.T) {
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
	})
	defer cleanup()

	s := NewServer()
	detail := "everything"
	res, _, err := s.listCards(context.Background(), &mcp.CallToolRequest{}, listCardsArgs{Board: "b1", Detail: &detail})
	if err != nil {
		t.Fatalf("handler returned Go error instead of structured result: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for an invalid detail value")
	}
}

// TestToolListCardsRequestID drives the tool against a query-aware server
// (mockFavro ignores the query string, so it can't tell page 0 apart from a
// requestId-carrying page 1) to prove request_id round-trips: the page-0 call
// echoes one, and supplying it back on page 1 reaches Favro rather than being
// silently dropped.
func TestToolListCardsRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		q := r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		var body any
		switch {
		case r.Method == "GET" && path == "/organizations":
			body = map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}}
		case r.Method == "GET" && path == "/widgets/b1":
			body = map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"}
		case r.Method == "GET" && path == "/cards" && strings.Contains(q, "requestId=reqZ") && strings.Contains(q, "page=1"):
			// The one query shape a caller reusing the page-0 requestId produces.
			body = map[string]any{"entities": []any{map[string]any{"cardId": "c2", "sequentialId": 2, "name": "B"}}}
		case r.Method == "GET" && path == "/cards" && !strings.Contains(q, "requestId="):
			body = map[string]any{"requestId": "reqA", "pages": 2, "entities": []any{
				map[string]any{"cardId": "c1", "sequentialId": 1, "name": "A"},
			}}
		default:
			// Any other shape (e.g. a caller that ignored the supplied request_id
			// and re-minted its own) is a shape this test wants to fail loudly on.
			http.NotFound(w, r)
			return
		}
		b, _ := json.Marshal(body)
		w.Write(b)
	}))
	defer srv.Close()
	restore := favro.WithBaseURL(srv.URL + "/api/v1")
	defer restore()
	t.Setenv("FAVRO_EMAIL", "e")
	t.Setenv("FAVRO_API_TOKEN", "t")

	s := NewServer()
	res, _, err := s.listCards(context.Background(), &mcp.CallToolRequest{}, listCardsArgs{Board: "b1", Page: 0})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(out, "request_id: reqA") {
		t.Fatalf("page 0 output should echo the requestId Favro minted:\n%s", out)
	}

	reqID := "reqZ" // deliberately not "reqA", so this only passes if the tool
	// actually threads the caller's value through rather than re-minting one.
	res, _, err = s.listCards(context.Background(), &mcp.CallToolRequest{},
		listCardsArgs{Board: "b1", Page: 1, RequestID: &reqID})
	if err != nil {
		t.Fatal(err)
	}
	out = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(out, "name: B") {
		t.Errorf("page 1 fetched with the supplied request_id should return card B:\n%s", out)
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

func TestToolGetCardDetailsSince(t *testing.T) {
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
		"GET /widgets/b1":    map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"},
		"GET /cards/c1":      cardBody(),
		"GET /columns":       map[string]any{"entities": []any{map[string]any{"columnId": "col1", "name": "Done"}}},
		"GET /users":         map[string]any{"entities": []any{map[string]any{"userId": "u1", "name": "Jane Doe"}}},
		"GET /tags":          map[string]any{"entities": []any{}},
		"GET /tasklists":     map[string]any{"entities": []any{}},
		"GET /comments": map[string]any{"entities": []any{
			map[string]any{"commentId": "cm1", "userId": "u1", "comment": "old one", "created": "2025-01-01T00:00:00.000Z"},
			map[string]any{"commentId": "cm2", "userId": "u1", "comment": "edited later", "created": "2025-01-02T00:00:00.000Z", "lastUpdated": "2025-03-01T00:00:00.000Z"},
			map[string]any{"commentId": "cm3", "userId": "u1", "comment": "recent", "created": "2025-06-01T00:00:00.000Z"},
		}},
	})
	defer cleanup()

	s := NewServer()

	// No since: all three comments, and the edited one shows both timestamps.
	res, _, err := s.getCardDetails(context.Background(), &mcp.CallToolRequest{}, getCardDetailsArgs{Card: "c1", Board: strPtr("b1")})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"old one", "edited later", "recent", "created 2025-01-02T00:00:00.000Z, edited 2025-03-01T00:00:00.000Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q with no since filter:\n%s", want, out)
		}
	}

	// since=2025-02-15: keeps the edited-after-that-date comment and the newer
	// one, drops the untouched-since-January one.
	since := "2025-02-15T00:00:00.000Z"
	res, _, err = s.getCardDetails(context.Background(), &mcp.CallToolRequest{}, getCardDetailsArgs{Card: "c1", Board: strPtr("b1"), Since: &since})
	if err != nil {
		t.Fatal(err)
	}
	out = res.Content[0].(*mcp.TextContent).Text
	if strings.Contains(out, "old one") {
		t.Errorf("comment untouched since before the cutoff should be filtered out:\n%s", out)
	}
	if !strings.Contains(out, "edited later") || !strings.Contains(out, "recent") {
		t.Errorf("expected comments at/after the cutoff to survive:\n%s", out)
	}
}

func TestToolGetCardDetailsSinceInvalid(t *testing.T) {
	cleanup := mockFavro(t, map[string]any{
		"GET /organizations": map[string]any{"entities": []any{map[string]any{"organizationId": "o1", "name": "Org"}}},
		"GET /widgets/b1":    map[string]any{"widgetCommonId": "b1", "name": "Tasks", "type": "board"},
		"GET /cards/c1":      cardBody(),
		"GET /tasklists":     map[string]any{"entities": []any{}},
		"GET /comments":      map[string]any{"entities": []any{}},
	})
	defer cleanup()

	s := NewServer()
	since := "not-a-date"
	res, _, err := s.getCardDetails(context.Background(), &mcp.CallToolRequest{}, getCardDetailsArgs{Card: "c1", Board: strPtr("b1"), Since: &since})
	if err != nil {
		t.Fatalf("handler returned Go error instead of structured result: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for an unparseable since value")
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
