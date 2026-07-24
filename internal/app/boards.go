package app

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// runBrowseBoards walks the user through Collections -> Boards and sets the
// selected board as the active one on the session. Esc/q at either step
// returns to the main menu.
func runBrowseBoards() {
	fmt.Println()
	s, err := NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Not logged in."))
		fmt.Println()
		return
	}
	if _, err := s.RequireOrg(); err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  " + err.Error()))
		fmt.Println()
		return
	}

	collectionID, ok := pickCollection(s)
	if !ok {
		return
	}
	chosen, ok := pickBoard(s, collectionID)
	if !ok {
		return
	}
	s.SetBoard(chosen.WidgetCommonID)
	fmt.Printf("  Selected board: %s\n\n", styleSuccess.Render(chosen.Name))
}

// pickCollection shows the collection chooser. It returns the chosen
// collection id (empty for "all boards") and a flag that is false if the user
// cancelled or there was an error.
func pickCollection(s *Session) (string, bool) {
	collections, err := s.Client().GetCollections(false)
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Failed to load collections: "+err.Error()))
		fmt.Println()
		return "", false
	}
	if len(collections) == 0 {
		return "", true
	}
	labels := make([]string, 0, len(collections)+1)
	labels = append(labels, "(All boards / no collection)")
	for _, c := range collections {
		labels = append(labels, c.Name)
	}
	m := selectModel{
		title:   "Browse boards",
		options: labels,
		footer:  "up/down move . enter select . q cancel",
	}
	p := tea.NewProgram(m)
	out, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
		fmt.Println()
		return "", false
	}
	r := out.(selectModel)
	if r.cancel {
		fmt.Println()
		return "", false
	}
	if r.cursor <= 0 || r.cursor > len(collections) {
		return "", true
	}
	return collections[r.cursor-1].CollectionID, true
}

// pickBoard shows the board chooser for a collection. It returns the chosen
// board and a flag that is false if the user cancelled or there was nothing to
// show.
func pickBoard(s *Session, collectionID string) (favro.Widget, bool) {
	boards, err := s.Client().GetWidgets(collectionID, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Failed to load boards: "+err.Error()))
		fmt.Println()
		return favro.Widget{}, false
	}
	if len(boards) == 0 {
		fmt.Println(styleDim.Render("  No boards found."))
		fmt.Println()
		return favro.Widget{}, false
	}
	labels := make([]string, len(boards))
	for i, b := range boards {
		labels[i] = formatBoardLabel(b)
	}
	m := selectModel{
		title:   "Browse boards  pick a board",
		options: labels,
		footer:  "up/down move . enter select . q cancel",
	}
	p := tea.NewProgram(m)
	out, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
		fmt.Println()
		return favro.Widget{}, false
	}
	r := out.(selectModel)
	if r.cancel {
		fmt.Println()
		return favro.Widget{}, false
	}
	if r.cursor < 0 || r.cursor >= len(boards) {
		fmt.Println()
		return favro.Widget{}, false
	}
	return boards[r.cursor], true
}

// formatBoardLabel renders a widget (board) as a single menu line, including
// its type as a dim suffix when present.
func formatBoardLabel(b favro.Widget) string {
	if b.Type == "" {
		return b.Name
	}
	return fmt.Sprintf("%s  %s", b.Name, styleDim.Render("("+b.Type+")"))
}
