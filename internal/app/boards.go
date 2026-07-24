package app

import (
	"fmt"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// collectionSelectOptions builds the option labels for the collection picker
// inside Browse boards. The first entry is always "all boards / no
// collection"; subsequent entries are the collection names in order.
func collectionSelectOptions(cols []favro.Collection) []string {
	out := make([]string, 0, len(cols)+1)
	out = append(out, "(All boards / no collection)")
	for _, c := range cols {
		out = append(out, c.Name)
	}
	return out
}

// resolveCollectionChoice maps the picker cursor back to a collection ID.
// Cursor 0 (or out-of-range) means "all boards", returned as "".
func resolveCollectionChoice(cols []favro.Collection, cursor int) string {
	if cursor <= 0 || cursor > len(cols) {
		return ""
	}
	return cols[cursor-1].CollectionID
}

// boardSelectOptions builds selectModel option labels for the board picker.
func boardSelectOptions(boards []favro.Widget) []string {
	out := make([]string, len(boards))
	for i, b := range boards {
		out[i] = formatBoardLabel(b)
	}
	return out
}

// formatBoardLabel renders a widget (board) as a single menu line, including
// its type as a dim suffix when present.
func formatBoardLabel(b favro.Widget) string {
	if b.Type == "" {
		return b.Name
	}
	return fmt.Sprintf("%s  %s", b.Name, styleDim.Render("("+b.Type+")"))
}
