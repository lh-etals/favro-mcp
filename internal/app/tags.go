package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// tagsTableText renders the organization's tags as a fixed-width table
// suitable for the scroll view. Used by the interactive Tags screen.
func tagsTableText(tags []favro.Tag) string {
	sorted := make([]favro.Tag, len(tags))
	copy(sorted, tags)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	const nameW = 24
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", padRight("Tag Name", nameW), "Color")
	fmt.Fprintln(&b, strings.Repeat("-", nameW+8))
	for _, t := range sorted {
		color := "-"
		if t.Color != nil && *t.Color != "" {
			color = *t.Color
		}
		fmt.Fprintf(&b, "%s  %s\n", padRight(t.Name, nameW), color)
	}
	return b.String()
}
