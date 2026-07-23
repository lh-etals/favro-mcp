package favro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const BaseURL = "https://favro.com/api/v1"

// Errors --------------------------------------------------------------------

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message) }

type AuthError struct{ *APIError }

type NotFoundError struct{ *APIError }

type RateLimitError struct {
	*APIError
	ResetTime string
}

// Client --------------------------------------------------------------------

type Client struct {
	httpClient     *http.Client
	email          string
	token          string
	organizationID string
	baseURL        string
	backendID      string
}

func NewClient(email, token, organizationID string) *Client {
	return &Client{
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		email:          email,
		token:          token,
		organizationID: organizationID,
		baseURL:        BaseURL,
	}
}

func (c *Client) headers(includeOrg bool) http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json")
	if includeOrg && c.organizationID != "" {
		h.Set("organizationId", c.organizationID)
	}
	return h
}

// do performs a request and returns the decoded JSON body (or nil for 204).
func (c *Client) do(method, path string, params url.Values, body []byte, contentType string, includeOrg bool) (map[string]any, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, reader)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header = c.headers(includeOrg)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if bid := resp.Header.Get("X-Favro-Backend-Identifier"); bid != "" {
		c.backendID = bid
	}

	text, _ := io.ReadAll(resp.Body)
	switch {
	case resp.StatusCode == 401:
		return nil, &AuthError{&APIError{Status: 401, Message: "Invalid credentials"}}
	case resp.StatusCode == 403:
		return nil, &AuthError{&APIError{Status: 403, Message: "Access denied"}}
	case resp.StatusCode == 404:
		return nil, &NotFoundError{&APIError{Status: 404, Message: "Resource not found"}}
	case resp.StatusCode == 429:
		return nil, &RateLimitError{&APIError{Status: 429, Message: "Rate limit exceeded"}, resp.Header.Get("X-RateLimit-Reset")}
	case resp.StatusCode >= 400:
		return nil, &APIError{Status: resp.StatusCode, Message: string(text)}
	case resp.StatusCode == 204:
		return nil, nil
	}

	trimmed := bytes.TrimSpace(text)
	if len(trimmed) == 0 {
		return map[string]any{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(trimmed, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	if msg, ok := data["message"].(string); ok && len(data) == 1 {
		return nil, &APIError{Status: 200, Message: msg}
	}
	return data, nil
}

func (c *Client) get(path string, params url.Values, includeOrg bool) (map[string]any, error) {
	return c.do(http.MethodGet, path, params, nil, "", includeOrg)
}

func (c *Client) post(path string, body map[string]any, includeOrg bool) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.do(http.MethodPost, path, nil, b, "application/json", includeOrg)
}

func (c *Client) put(path string, body map[string]any, includeOrg bool) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.do(http.MethodPut, path, nil, b, "application/json", includeOrg)
}

func (c *Client) del(path string, params url.Values, includeOrg bool) (map[string]any, error) {
	return c.do(http.MethodDelete, path, params, nil, "", includeOrg)
}

func (c *Client) postBinary(path string, content []byte, params url.Values, includeOrg bool) (map[string]any, error) {
	return c.do(http.MethodPost, path, params, content, "application/octet-stream", includeOrg)
}

// Pagination helpers --------------------------------------------------------

func entitiesOf(data map[string]any) []map[string]any {
	raw, ok := data["entities"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func pagesOf(data map[string]any) int {
	switch v := data["pages"].(type) {
	case float64:
		if v == float64(int(v)) {
			return int(v)
		}
		return int(v)
	default:
		return 1
	}
}

// paginateAll fetches every page of a paginated endpoint.
func (c *Client) paginateAll(path string, params url.Values) ([]map[string]any, error) {
	if params == nil {
		params = url.Values{}
	}
	data, err := c.get(path, params, true)
	if err != nil {
		return nil, err
	}
	all := entitiesOf(data)

	reqID, _ := data["requestId"].(string)
	total := pagesOf(data)
	if reqID != "" {
		for page := 1; page < total; page++ {
			p := url.Values{}
			for k, vs := range params {
				p[k] = vs
			}
			p.Set("requestId", reqID)
			p.Set("page", strconv.Itoa(page))
			data, err = c.get(path, p, true)
			if err != nil {
				return nil, err
			}
			all = append(all, entitiesOf(data)...)
		}
	}
	return all, nil
}

// paginateSingle fetches a single page (0-indexed) and returns total pages.
func (c *Client) paginateSingle(path string, params url.Values, page int) ([]map[string]any, int, error) {
	if params == nil {
		params = url.Values{}
	}
	if page == 0 {
		data, err := c.get(path, params, true)
		if err != nil {
			return nil, 0, err
		}
		return entitiesOf(data), pagesOf(data), nil
	}
	first, err := c.get(path, params, true)
	if err != nil {
		return nil, 0, err
	}
	reqID, _ := first["requestId"].(string)
	total := pagesOf(first)
	if page >= total {
		return nil, total, nil
	}
	if reqID == "" {
		return nil, total, nil
	}
	p := url.Values{}
	for k, vs := range params {
		p[k] = vs
	}
	p.Set("requestId", reqID)
	p.Set("page", strconv.Itoa(page))
	data, err := c.get(path, p, true)
	if err != nil {
		return nil, total, err
	}
	return entitiesOf(data), total, nil
}

// decodeOne unmarshals a single response object into a typed value.
func decodeOne[T any](data map[string]any) (*T, error) {
	if data == nil {
		return nil, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// decodeMany unmarshals a slice of raw objects into typed values.
func decodeMany[T any](items []map[string]any) ([]T, error) {
	out := make([]T, 0, len(items))
	for _, m := range items {
		v, err := decodeOne[T](m)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}
