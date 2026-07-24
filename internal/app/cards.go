package app

import (
	"fmt"
	"strings"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// cardListOptions builds selectModel option labels for one page of cards.
func cardListOptions(cards []favro.Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = formatCardLabel(c)
	}
	return out
}

// formatCardLabel renders a card as "#<seq> <name> [tag1, tag2]".
func formatCardLabel(c favro.Card) string {
	label := fmt.Sprintf("#%d %s", c.SequentialID, c.Name)
	if len(c.Tags) > 0 {
		label += " [" + strings.Join(c.Tags, ", ") + "]"
	}
	return label
}
