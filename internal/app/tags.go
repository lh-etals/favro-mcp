package app

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// runTags lists the organization's tags as a formatted, scrollable table.
// Esc/q returns to the main menu.
func runTags() {
	fmt.Println()
	s, err := NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Not logged in."))
		fmt.Println()
		return
	}
	if _, err := s.RequireOrg(); err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  "+err.Error()))
		fmt.Println()
		return
	}
	tags, err := s.Client().GetTags()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Failed to load tags: "+err.Error()))
		fmt.Println()
		return
	}
	if len(tags) == 0 {
		fmt.Println(styleDim.Render("  No tags found."))
		fmt.Println()
		return
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})

	const nameW = 24
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", padRight("Tag Name", nameW), "Color")
	fmt.Fprintln(&b, strings.Repeat("-", nameW+8))
	for _, t := range tags {
		color := "-"
		if t.Color != nil && *t.Color != "" {
			color = *t.Color
		}
		fmt.Fprintf(&b, "%s  %s\n", padRight(t.Name, nameW), color)
	}

	p := tea.NewProgram(newScrollTextModel(
		fmt.Sprintf("Tags  %d", len(tags)), b.String()))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
	}
	fmt.Println()
}
