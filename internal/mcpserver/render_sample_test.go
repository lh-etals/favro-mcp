package mcpserver

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSampleFormats(t *testing.T) {
	// list_boards
	lb := rendered{
		front: map[string]any{
			"collection": "Internal Tasks (AiApp)",
			"boards": []boardRow{
				{Name: "Tasks", ID: "255a5d9c834f8dcf14b732e1", Type: "board"},
				{Name: "Archive", ID: "2b66d04e2b1f52442db579f8", Type: "board"},
			},
		},
		body: "2 board(s) in Internal Tasks (AiApp).",
	}
	t.Log("=== list_boards ===\n" + lb.String())

	// list_cards (with query)
	lc := rendered{
		front: map[string]any{
			"board": "Tasks",
			"page":  1,
			"pages": 3,
			"query": "login",
			"cards": []cardRow{
				{Seq: 3709, Name: "Fix login redirect", ID: "c7e476131c11a9c14fc28d15", Column: "f2df3d3aaa19b133e2469e6f", Tags: []string{"bug"}},
				{Seq: 3700, Name: "Login rate limit", ID: "062abbc3884d5818023c6cae", Column: "a1bc0000000000000000aaaa"},
			},
		},
		body: "2 card(s) (page 1/3). Next: page=2.",
	}
	t.Log("=== list_cards ===\n" + lc.String())

	// create_card message
	t.Log("=== create_card ===\n" + mdMessage("Created card **#3709 Fix login redirect**.", map[string]any{
		"card_id":        "c7e476131c11a9c14fc28d15",
		"card_common_id": "d1a20000000000000000b1b2",
		"sequential_id":  3709,
		"board":          "255a5d9c834f8dcf14b732e1",
	}))

	// error (not_found via resolver)
	ec := errorResult(&notFoundError{entityType: "card", identifier: "zzz-nonexistent"}).Content[0].(*mcp.TextContent)
	t.Log("=== error (not_found) ===\n" + ec.Text)
}
