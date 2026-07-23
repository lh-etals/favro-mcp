package mcpserver

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// mapKeys returns the top-level keys of a parsed frontmatter mapping node, in
// document order.
func mapKeys(t *testing.T, doc *yaml.Node) []string {
	t.Helper()
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		t.Fatalf("not a document node: %+v", doc)
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		t.Fatalf("not a mapping node: kind=%v", m.Kind)
	}
	keys := []string{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		keys = append(keys, m.Content[i].Value)
	}
	return keys
}

func TestFrontmatterValidAndOrdered(t *testing.T) {
	cases := []struct {
		name       string
		rendered   string
		wantKeys   []string // top-level order
		wantInBody string
	}{
		{
			name: "list_boards",
			rendered: rendered{
				front: listBoardsFront{
					Collection: "Internal Tasks (AiApp)",
					Boards: []boardRow{
						{Name: "Tasks", ID: "255a", Type: "board"},
						{Name: "Archive", ID: "2b66", Type: "board"},
					},
				},
				body: "2 board(s) in Internal Tasks (AiApp).",
			}.String(),
			wantKeys:   []string{"collection", "boards"},
			wantInBody: "2 board(s)",
		},
		{
			name: "list_cards",
			rendered: rendered{
				front: listCardsFront{
					Board: "Tasks", Page: 1, Pages: 3, Query: "login",
					Cards: []cardRow{{Seq: 3709, Name: "Fix login redirect", ID: "c7e4", Tags: []string{"bug"}}},
				},
				body: "1 card(s) (page 1/3). Next: page=2.",
			}.String(),
			wantKeys:   []string{"board", "page", "pages", "query", "cards"},
			wantInBody: "1 card(s)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			front, body, err := parseFrontmatter(tc.rendered)
			if err != nil {
				t.Fatalf("frontmatter invalid: %v\n%s", err, tc.rendered)
			}
			got := mapKeys(t, front)
			if !equal(got, tc.wantKeys) {
				t.Errorf("key order = %v, want %v", got, tc.wantKeys)
			}
			if !strings.Contains(body, tc.wantInBody) {
				t.Errorf("body %q missing %q", body, tc.wantInBody)
			}
		})
	}
}

// omitempty: list_boards without collection must not emit a collection key.
func TestFrontmatterOmitsEmpty(t *testing.T) {
	s := rendered{front: listBoardsFront{Boards: []boardRow{{Name: "X", ID: "1"}}}}.String()
	front, _, err := parseFrontmatter(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range mapKeys(t, front) {
		if k == "collection" {
			t.Errorf("collection should be omitted when empty; output:\n%s", s)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
