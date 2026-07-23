package favro

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mock builds an httptest server whose handler decides the response from the
// method/path/query, and points favro.BaseURL at it for the test.
func mock(t *testing.T, handle func(method, path, query string) (int, any)) (*httptest.Server, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := handle(r.Method, r.URL.Path, r.URL.RawQuery)
		if body == nil {
			w.WriteHeader(status)
			return
		}
		b, _ := json.Marshal(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(b)
	}))
	restore := WithBaseURL(srv.URL + "/api/v1")
	return srv, func() { restore(); srv.Close() }
}

func TestPaginateAll(t *testing.T) {
	_, cleanup := mock(t, func(m, p, q string) (int, any) {
		if m == "GET" && p == "/api/v1/widgets" {
			if !strings.Contains(q, "requestid=") {
				return 200, map[string]any{"requestId": "req1", "pages": 2, "entities": []any{
					map[string]any{"name": "A", "widgetCommonId": "idA"},
				}}
			}
			return 200, map[string]any{"entities": []any{
				map[string]any{"name": "B", "widgetCommonId": "idB"},
			}}
		}
		return 404, nil
	})
	defer cleanup()
	c := NewClient("e", "t", "")
	items, err := c.paginateAll("/widgets", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("got %d entities, want 2", len(items))
	}
}

func TestPaginateAllMissingRequestId(t *testing.T) {
	_, cleanup := mock(t, func(m, p, q string) (int, any) {
		// pages>1 but no requestId -> must error rather than silently truncate.
		return 200, map[string]any{"pages": 3, "entities": []any{map[string]any{"x": 1}}}
	})
	defer cleanup()
	c := NewClient("e", "t", "")
	if _, err := c.paginateAll("/widgets", nil); err == nil {
		t.Fatal("expected error when pages>1 but no requestId")
	}
}

func TestCardsMarkdownFallback(t *testing.T) {
	_, cleanup := mock(t, func(m, p, q string) (int, any) {
		if m == "GET" && p == "/api/v1/cards" {
			if strings.Contains(strings.ToLower(q), "descriptionformat=markdown") {
				return 500, map[string]any{"message": "server error"}
			}
			return 200, map[string]any{"entities": []any{map[string]any{"cardId": "c1", "name": "ok"}}}
		}
		return 404, nil
	})
	defer cleanup()
	c := NewClient("e", "t", "")
	cards, err := c.GetCards(CardFilter{Unique: true})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if len(cards) != 1 || cards[0].Name != "ok" {
		t.Errorf("unexpected cards: %+v", cards)
	}
}

func TestErrorDecoding(t *testing.T) {
	_, cleanup := mock(t, func(m, p, q string) (int, any) {
		switch {
		case p == "/api/v1/organizations":
			return 401, map[string]any{"message": "bad creds"}
		case strings.HasPrefix(p, "/api/v1/widgets/"):
			return 403, map[string]any{"message": "denied"}
		case strings.HasPrefix(p, "/api/v1/tags/"):
			return 404, map[string]any{"message": "gone"}
		case p == "/api/v1/odd":
			return 200, map[string]any{"message": "soft error"} // single-key message
		default:
			return 404, nil
		}
	})
	defer cleanup()
	c := NewClient("e", "t", "")

	var auth *AuthError
	if _, err := c.GetOrganizations(); !errors.As(err, &auth) || auth.Status != 401 {
		t.Errorf("expected 401 AuthError, got %T %v", err, err)
	}
	var forbidden *AuthError
	if _, err := c.GetWidget("x"); !errors.As(err, &forbidden) || forbidden.Status != 403 {
		t.Errorf("expected 403 AuthError, got %T %v", err, err)
	}
	var nf *NotFoundError
	if _, err := c.GetTag("x"); !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T %v", err, err)
	}
	var api *APIError
	if _, err := c.get("/odd", nil, true); !errors.As(err, &api) || api.Status != 200 {
		t.Errorf("expected 200 soft APIError, got %T %v", err, err)
	}
}

func TestArrayResponse(t *testing.T) {
	_, cleanup := mock(t, func(m, p, q string) (int, any) {
		// DELETE /cards?everywhere=true returns a bare array; client must not error.
		if m == "DELETE" && strings.HasPrefix(p, "/api/v1/cards/") {
			return 200, []any{map[string]any{"cardId": "c1"}, map[string]any{"cardId": "c2"}}
		}
		return 404, nil
	})
	defer cleanup()
	c := NewClient("e", "t", "")
	if err := c.DeleteCard("c1", true); err != nil {
		t.Errorf("array response should not error, got %v", err)
	}
}
