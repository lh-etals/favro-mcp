package app

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// runViewCardDetail is the "View card detail" menu entry. It prompts for a card
// identifier (sequential ID like #123, card ID, or name) and shows the detail
// screen.
func runViewCardDetail() {
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
	showCardDetail(s, nil)
}

// showCardDetail resolves and renders a single card. When preselected is nil
// the user is prompted for a card identifier; otherwise the given card is
// shown directly (used by the "List cards" screen's detail action).
func showCardDetail(s *Session, preselected *favro.Card) {
	card := preselected
	if card == nil {
		p := tea.NewProgram(newCardPromptModel())
		out, err := p.Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
			fmt.Println()
			return
		}
		r := out.(cardPromptModel)
		if r.cancel {
			fmt.Println()
			return
		}
		identifier := strings.TrimSpace(r.input.Value())
		if identifier == "" {
			fmt.Println()
			return
		}
		c, err := s.Resolver().Card(identifier, s.BoardID())
		if err != nil {
			fmt.Fprintln(os.Stderr, styleError.Render("  "+err.Error()))
			fmt.Println()
			return
		}
		card = c
	}

	text, err := buildCardDetailText(s.Client(), *card)
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  "+err.Error()))
		fmt.Println()
		return
	}
	title := fmt.Sprintf("Card detail  #%d", card.SequentialID)
	p := tea.NewProgram(newScrollTextModel(title, text))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
	}
	fmt.Println()
}

// tlData pairs a tasklist with its tasks while building card detail.
type tlData struct {
	tl    favro.TaskList
	tasks []favro.Task
}

// buildCardDetailText fetches the card's tasklists, comments, and resolves
// board/column/lane/user names to produce the human-readable detail block.
func buildCardDetailText(client *favro.Client, c favro.Card) (string, error) {
	tasklists, err := client.GetTasklists(c.CardCommonID)
	if err != nil {
		return "", err
	}
	tls := make([]tlData, 0, len(tasklists))
	for _, tl := range tasklists {
		tasks, err := client.GetTasks(c.CardCommonID, tl.TaskListID)
		if err != nil {
			return "", err
		}
		tls = append(tls, tlData{tl: tl, tasks: tasks})
	}

	comments, err := client.GetComments(c.CardCommonID)
	if err != nil {
		return "", err
	}

	// Resolve board / lane / column names (best-effort, fall back to IDs).
	cardBoard := strOr(c.WidgetCommonID)
	boardName, laneName, colName := cardBoard, strOr(c.LaneID), strOr(c.ColumnID)
	if w, err := client.GetWidget(cardBoard); err == nil && w != nil {
		boardName = w.Name
		for _, l := range w.Lanes {
			if l.LaneID == strOr(c.LaneID) {
				laneName = l.Name
				break
			}
		}
	}
	if cols, err := client.GetColumns(cardBoard); err == nil {
		for _, col := range cols {
			if col.ColumnID == strOr(c.ColumnID) {
				colName = col.Name
				break
			}
		}
	}

	// Cache user names for assignments + comments.
	userNames := map[string]string{}
	if users, err := client.GetUsers(); err == nil {
		for _, u := range users {
			userNames[u.UserID] = u.Name
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s\n", c.SequentialID, c.Name)
	fmt.Fprintf(&b, "Board: %s  Column: %s", boardName, colName)
	if strOr(c.LaneID) != "" {
		fmt.Fprintf(&b, "  Lane: %s", laneName)
	}
	b.WriteString("\n")
	status := "active"
	if c.Archived {
		status = "archived"
	}
	if strOr(c.DueDate) != "" {
		fmt.Fprintf(&b, "Status: %s  Due: %s\n", status, strOr(c.DueDate))
	} else {
		fmt.Fprintf(&b, "Status: %s\n", status)
	}
	if len(c.Assignments) > 0 {
		names := make([]string, 0, len(c.Assignments))
		for _, a := range c.Assignments {
			if n := userNames[a.UserID]; n != "" {
				names = append(names, n)
			} else {
				names = append(names, a.UserID)
			}
		}
		fmt.Fprintf(&b, "Assigned: %s\n", strings.Join(names, ", "))
	}
	if len(c.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(c.Tags, ", "))
	}

	// Strip the trailing tasklist checkbox lines Favro auto-appends to the
	// detailed description before showing it.
	desc := stripTasklistCheckboxes(strOr(c.DetailedDescription), tls)
	if desc != "" {
		fmt.Fprintf(&b, "\nDescription:\n%s\n", desc)
	}
	if len(tls) > 0 {
		b.WriteString("\nChecklists:\n")
		for _, d := range tls {
			done := 0
			for _, t := range d.tasks {
				if t.Completed {
					done++
				}
			}
			fmt.Fprintf(&b, "- %s (%d/%d)\n", d.tl.Name, done, len(d.tasks))
			for _, t := range d.tasks {
				box := "[ ]"
				if t.Completed {
					box = "[x]"
				}
				fmt.Fprintf(&b, "  %s %s\n", box, t.Name)
			}
		}
	}
	if len(comments) > 0 {
		b.WriteString("\nComments:\n")
		for _, cm := range comments {
			who := cm.UserID
			if n := userNames[cm.UserID]; n != "" {
				who = n
			}
			fmt.Fprintf(&b, "- %s (%s): %q\n", who, cm.Created, cm.Comment)
		}
	}
	return b.String(), nil
}

// stripTasklistCheckboxes removes the trailing tasklist checkbox lines that
// Favro auto-appends to a card's detailedDescription. It is the inline
// equivalent of mcpserver.stripTasklistFromDescription, reimplemented here so
// the app package does not need to import the MCP server.
func stripTasklistCheckboxes(description string, tasklists []tlData) string {
	if description == "" || len(tasklists) == 0 {
		return description
	}
	lines := strings.Split(strings.TrimRight(description, "\n"), "\n")

	checkboxPatterns := map[string]struct{}{}
	tasklistNames := map[string]struct{}{}
	for _, tl := range tasklists {
		tasklistNames[tl.tl.Name] = struct{}{}
		for _, t := range tl.tasks {
			checkboxPatterns["\u2610 "+t.Name] = struct{}{} // ☐
			checkboxPatterns["\u2611 "+t.Name] = struct{}{} // ☑
		}
	}

	for len(lines) > 0 {
		line := strings.TrimSpace(lines[len(lines)-1])
		if line == "" {
			lines = lines[:len(lines)-1]
			continue
		}
		if _, ok := checkboxPatterns[line]; ok {
			lines = lines[:len(lines)-1]
			continue
		}
		if _, ok := tasklistNames[line]; ok {
			lines = lines[:len(lines)-1]
			continue
		}
		break
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// strOr dereferences a *string, returning "" for nil. Mirrors the helper in
// the mcpserver package; kept local so the app stays self-contained.
func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// --- card identifier prompt -------------------------------------------------

// cardPromptModel is a single textinput used to ask for a card identifier.
type cardPromptModel struct {
	input  textinput.Model
	cancel bool
}

func newCardPromptModel() cardPromptModel {
	ti := textinput.New()
	ti.Prompt = "  Card: "
	ti.Placeholder = "#123, card ID, or name"
	ti.CharLimit = 200
	ti.Focus()
	return cardPromptModel{input: ti}
}

func (m cardPromptModel) Init() tea.Cmd { return textinput.Blink }

func (m cardPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c", "esc":
			m.cancel = true
			return m, tea.Quit
		case "enter":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m cardPromptModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("View card detail"))
	b.WriteString("\n\n")
	b.WriteString(m.input.View() + "\n\n")
	b.WriteString(styleFooter.Render("enter submit . esc cancel"))
	return b.String()
}
