package app

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// runListCards shows the cards on the active board, paginated. Esc/q returns
// to the main menu.
func runListCards() {
	fmt.Println()
	s, err := NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Not logged in."))
		fmt.Println()
		return
	}
	if s.BoardID() == "" {
		fmt.Fprintln(os.Stderr, styleError.Render("  No board selected. Use Browse boards first."))
		fmt.Println()
		return
	}
	if _, err := s.RequireOrg(); err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  "+err.Error()))
		fmt.Println()
		return
	}

	page := 0
	for {
		cards, total, err := s.Client().GetCardsPage(favro.CardFilter{
			WidgetCommonID: s.BoardID(),
			Unique:         true,
		}, page)
		if err != nil {
			fmt.Fprintln(os.Stderr, styleError.Render("  Failed to load cards: "+err.Error()))
			fmt.Println()
			return
		}
		if len(cards) == 0 {
			fmt.Println(styleDim.Render("  No cards on this board."))
			fmt.Println()
			return
		}

		labels := make([]string, len(cards))
		for i, c := range cards {
			labels[i] = formatCardLabel(c)
		}
		title := fmt.Sprintf("List cards  page %d/%d", page+1, total)
		footer := "up/down move . enter detail . n next . p prev . q back"
		if total <= 1 {
			footer = "up/down move . enter detail . q back"
		}
		m := cardsModel{
			selectModel: selectModel{title: title, options: labels, footer: footer},
			page:        page,
			total:       total,
		}
		p := tea.NewProgram(m)
		out, err := p.Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
			fmt.Println()
			return
		}
		r := out.(cardsModel)
		if r.cancel {
			fmt.Println()
			return
		}
		switch r.action {
		case "next":
			if page < total-1 {
				page++
			}
			continue
		case "prev":
			if page > 0 {
				page--
			}
			continue
		case "detail":
			if r.cursor >= 0 && r.cursor < len(cards) {
				c := cards[r.cursor]
				fmt.Printf("  Card: %s (#%d)\n", styleSuccess.Render(c.Name), c.SequentialID)
			}
			fmt.Println()
			return
		}
		fmt.Println()
		return
	}
}

// formatCardLabel renders a card as "#<seq> <name> [tag1, tag2]".
func formatCardLabel(c favro.Card) string {
	label := fmt.Sprintf("#%d %s", c.SequentialID, c.Name)
	if len(c.Tags) > 0 {
		label += " [" + strings.Join(c.Tags, ", ") + "]"
	}
	return label
}

// cardsModel extends selectModel with n/p pagination actions. View/Init are
// inherited from selectModel; Update is overridden to capture the extra keys.
type cardsModel struct {
	selectModel
	action string // "next", "prev", "detail", or "" (no-op)
	page   int
	total  int
}

func (m cardsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "ctrl+c", "q", "esc":
		m.cancel = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case "n":
		if m.page < m.total-1 {
			m.action = "next"
			return m, tea.Quit
		}
	case "p":
		if m.page > 0 {
			m.action = "prev"
			return m, tea.Quit
		}
	case "enter":
		m.action = "detail"
		return m, tea.Quit
	}
	return m, nil
}
