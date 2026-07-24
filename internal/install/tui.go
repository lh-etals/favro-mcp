package install

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/lh-etals/favro-mcp/internal/mcpserver"
)

// Styling ---------------------------------------------------------------------

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	subtleStyle  = lipgloss.NewStyle().Faint(true)
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	checkStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("36"))
	greyStyle    = lipgloss.NewStyle().Faint(true)
	detectedTag  = subtleStyle.Render("(detected)")
	footerStyle  = lipgloss.NewStyle().Faint(true).MarginTop(1)
)

func footer(shortcuts string) string {
	return footerStyle.Render("  " + shortcuts)
}

// quitKeyMsg lets a model carry its result out of the program.
type doneMsg struct{}

// --- clients multiselect -----------------------------------------------------

type clientItem struct {
	def      ClientDef
	detected bool
}

type clientsModel struct {
	items    []clientItem
	cursor   int // index into selectable (detected) items
	checked  map[string]bool
	showAll  bool
	cancelled bool
}

func newClientsModel(detected, others []ClientDef) clientsModel {
	items := make([]clientItem, 0, len(detected)+len(others))
	checked := map[string]bool{}
	for _, c := range detected {
		items = append(items, clientItem{def: c, detected: true})
		checked[c.ID] = true // detected are pre-selected
	}
	for _, c := range others {
		items = append(items, clientItem{def: c, detected: false})
	}
	return clientsModel{items: items, checked: checked}
}

// selectable returns indices of detected items.
func (m clientsModel) selectable() []int {
	out := []int{}
	for i, it := range m.items {
		if it.detected {
			out = append(out, i)
		}
	}
	return out
}

func (m clientsModel) Init() tea.Cmd { return nil }

func (m clientsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch strings.ToLower(msg.String()) {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			sel := m.selectable()
			if len(sel) > 0 {
				// find current position among selectable, move up
				curIdx := -1
				for i, idx := range sel {
					if idx == sel[m.cursor%len(sel)] {
						curIdx = i
						break
					}
				}
				if curIdx < 0 {
					curIdx = 0
				}
				if curIdx > 0 {
					curIdx--
				}
				m.cursor = curIdx
			}
		case "down", "j":
			sel := m.selectable()
			if len(sel) > 0 {
				n := len(sel)
				curIdx := m.cursor % n
				curIdx = (curIdx + 1) % n
				m.cursor = curIdx
			}
		case " ":
			sel := m.selectable()
			if len(sel) > 0 {
				idx := sel[m.cursor%len(sel)]
				id := m.items[idx].def.ID
				m.checked[id] = !m.checked[id]
			}
		case "v":
			m.showAll = !m.showAll
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m clientsModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("  Select AI clients to register with"))
	b.WriteString("\n\n")
	sel := m.selectable()
	curIdx := -1
	if len(sel) > 0 {
		curIdx = sel[m.cursor%len(sel)]
	}
	renderedDetected := map[int]bool{}
	for i, it := range m.items {
		if !it.detected && !m.showAll {
			continue
		}
		// cursor only on the current detected item
		cursor := " "
		if i == curIdx {
			cursor = cursorStyle.Render(">")
		}
		mark := "[ ]"
		if m.checked[it.def.ID] {
			mark = checkStyle.Render("[x]")
		}
		line := fmt.Sprintf("  %s %s %s", cursor, mark, it.def.Name)
		if it.detected {
			line += " " + detectedTag
		} else {
			// non-detected: greyed, not selectable
			line = greyStyle.Render("    " + it.def.Name + " " + subtleStyle.Render("(not detected)"))
			b.WriteString(line + "\n")
			continue
		}
		renderedDetected[i] = true
		b.WriteString(line + "\n")
	}
	if !m.showAll {
		b.WriteString(subtleStyle.Render("    + other clients not detected (press v to show)") + "\n")
	}
	hint := "↑/↓ move · space toggle · v show all · enter confirm · q cancel"
	b.WriteString(footer(hint))
	return b.String()
}

// selectedIDs returns the chosen client IDs after the program ends.
func (m clientsModel) selectedIDs() []string {
	out := []string{}
	for _, it := range m.items {
		if it.detected && m.checked[it.def.ID] {
			out = append(out, it.def.ID)
		}
	}
	return out
}

// runClientsTUI runs the client multi-select. Falls back to a numbered prompt
// when stdin is not a TTY.
func runClientsTUI(detected, others []ClientDef) ([]string, error) {
	if !isTTY() {
		return fallbackClientsSelect(detected, others), nil
	}
	m := newClientsModel(detected, others)
	p := tea.NewProgram(m, tea.WithAltScreen())
	res, err := p.Run()
	if err != nil {
		return nil, err
	}
	cm := res.(clientsModel)
	if cm.cancelled {
		return nil, fmt.Errorf("cancelled")
	}
	return cm.selectedIDs(), nil
}

// fallbackClientsSelect: numbered prompt for non-TTY (detected pre-selected).
func fallbackClientsSelect(detected, others []ClientDef) []string {
	fmt.Println("Select clients to register with (detected marked *):")
	idx := 0
	idByNum := map[int]string{}
	for _, c := range detected {
		idx++
		idByNum[idx] = c.ID
		fmt.Printf("  * %d. %s\n", idx, c.Name)
	}
	for _, c := range others {
		idx++
		idByNum[idx] = c.ID
		fmt.Printf("    %d. %s\n", idx, c.Name)
	}
	fmt.Print("Enter comma-separated numbers (blank = keep * rows): ")
	line := readLine()
	if strings.TrimSpace(line) == "" {
		var out []string
		for _, c := range detected {
			out = append(out, c.ID)
		}
		return out
	}
	var out []string
	for _, p := range strings.Split(line, ",") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &n); err == nil {
			if id, ok := idByNum[n]; ok {
				out = append(out, id)
			}
		}
	}
	return out
}

// --- toolset select ----------------------------------------------------------

type toolsetModel struct {
	choices   []string
	descs     []string
	cursor    int
	cancelled bool
}

var toolsetChoices = []string{"Read-only", "Read + Write", "Read + Write + Delete", "Custom"}
var toolsetDescs = []string{
	"list/get only. Safest; cannot change anything.",
	"also create/update/move cards, columns, tags, comments, attachments.",
	"full access, including deletes.",
	"toggle each tool on/off individually.",
}
var toolsetVals = []string{mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete, "custom"}

func newToolsetModel() toolsetModel { return toolsetModel{choices: toolsetChoices, descs: toolsetDescs, cursor: 1} }

func (m toolsetModel) Init() tea.Cmd { return nil }

func (m toolsetModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch strings.ToLower(k.String()) {
	case "ctrl+c", "q", "esc":
		m.cancelled = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.choices)-1 {
			m.cursor++
		}
	case "enter":
		return m, tea.Quit
	}
	return m, nil
}

func (m toolsetModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("  Choose the toolset the server exposes"))
	b.WriteString("\n\n")
	for i, c := range m.choices {
		cursor := " "
		if i == m.cursor {
			cursor = cursorStyle.Render(">")
		}
		line := fmt.Sprintf("  %s %s", cursor, c)
		if i == m.cursor {
			line += "  " + subtleStyle.Render(m.descs[i])
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(footer("↑/↓ move · enter select · q cancel"))
	return b.String()
}

// runToolsetTUI returns one of mcpserver.TierRead/mcpserver.TierWrite/mcpserver.TierDelete/"custom".
func runToolsetTUI() (string, error) {
	if !isTTY() {
		i := selectOne("Which toolset should the server expose?", toolsetChoices, 1)
		return toolsetVals[i], nil
	}
	m := newToolsetModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	res, err := p.Run()
	if err != nil {
		return "", err
	}
	mm := res.(toolsetModel)
	if mm.cancelled {
		return "", fmt.Errorf("cancelled")
	}
	return toolsetVals[mm.cursor], nil
}

// --- custom tools toggle -----------------------------------------------------

type toolsModel struct {
	names     []string
	descs     []string
	tiers     []string
	checked   []bool
	cursor    int
	cancelled bool
}

func newToolsModel(catalog []mcpserver.ToolInfo) toolsModel {
	names := make([]string, len(catalog))
	descs := make([]string, len(catalog))
	tiers := make([]string, len(catalog))
	checked := make([]bool, len(catalog))
	for i, t := range catalog {
		names[i] = t.Name
		descs[i] = t.Description
		tiers[i] = t.Tier
		// default to the write preset: read+write on, delete off
		checked[i] = t.Tier == mcpserver.TierRead || t.Tier == mcpserver.TierWrite
	}
	return toolsModel{names: names, descs: descs, tiers: tiers, checked: checked}
}

func (m toolsModel) Init() tea.Cmd { return nil }

func (m toolsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch strings.ToLower(k.String()) {
	case "ctrl+c", "q", "esc":
		m.cancelled = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.names)-1 {
			m.cursor++
		}
	case " ":
		m.checked[m.cursor] = !m.checked[m.cursor]
	case "enter":
		return m, tea.Quit
	}
	return m, nil
}

func (m toolsModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("  Toggle individual tools (space toggles)"))
	b.WriteString("\n\n")
	for i, n := range m.names {
		cursor := " "
		if i == m.cursor {
			cursor = cursorStyle.Render(">")
		}
		mark := "[ ]"
		if m.checked[i] {
			mark = checkStyle.Render("[x]")
		}
		b.WriteString(fmt.Sprintf("  %s %s %-20s %s\n", cursor, mark, n, subtleStyle.Render("("+m.tiers[i]+") "+truncate(m.descs[i], 50))))
	}
	b.WriteString(footer("↑/↓ move · space toggle · enter confirm · q cancel"))
	return b.String()
}

func (m toolsModel) selected() []string {
	out := []string{}
	for i, on := range m.checked {
		if on {
			out = append(out, m.names[i])
		}
	}
	return out
}

func runToolsTUI(catalog []mcpserver.ToolInfo) ([]string, error) {
	if !isTTY() {
		return fallbackToolsSelect(catalog), nil
	}
	m := newToolsModel(catalog)
	p := tea.NewProgram(m, tea.WithAltScreen())
	res, err := p.Run()
	if err != nil {
		return nil, err
	}
	mm := res.(toolsModel)
	if mm.cancelled {
		return nil, fmt.Errorf("cancelled")
	}
	return mm.selected(), nil
}

func fallbackToolsSelect(catalog []mcpserver.ToolInfo) []string {
	choices := make([]choice, 0, len(catalog))
	for _, t := range catalog {
		choices = append(choices, choice{id: t.Name, label: t.Name + " (" + t.Tier + ")", checked: t.Tier == mcpserver.TierRead || t.Tier == mcpserver.TierWrite})
	}
	ids, _ := multiSelect("Toggle tools:", choices)
	return ids
}

// --- login inputs ------------------------------------------------------------

type loginModel struct {
	emailTI  textinput.Model
	tokenTI  textinput.Model
	focus    int // 0=email, 1=token
	cancelled bool
}

func newLoginModel(email string) loginModel {
	e := textinput.New()
	e.Placeholder = "you@example.com"
	e.Prompt = "  Email:  "
	e.Width = 40
	e.SetValue(email)
	e.Focus()
	e.Cursor.Blink = false
	t := textinput.New()
	t.Placeholder = "hidden while typing"
	t.Prompt = "  Token:  "
	t.Width = 40
	t.EchoMode = textinput.EchoPassword
	t.Cursor.Blink = false
	return loginModel{emailTI: e, tokenTI: t}
}

func (m loginModel) Init() tea.Cmd { return nil }

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch strings.ToLower(msg.String()) {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "tab", "shift+tab":
			if m.focus == 0 {
				m.focus = 1
				m.tokenTI.Focus()
				m.emailTI.Blur()
			} else {
				m.focus = 0
				m.emailTI.Focus()
				m.tokenTI.Blur()
			}
		case "enter":
			if m.focus == 0 {
				m.focus = 1
				m.tokenTI.Focus()
				m.emailTI.Blur()
			} else {
				return m, tea.Quit
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	if m.focus == 0 {
		m.emailTI, cmd = m.emailTI.Update(msg)
	} else {
		m.tokenTI, cmd = m.tokenTI.Update(msg)
	}
	return m, cmd
}

func (m loginModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("  Log in to Favro"))
	b.WriteString("\n\n")
	b.WriteString(m.emailTI.View() + "\n")
	b.WriteString(m.tokenTI.View() + "\n")
	b.WriteString(footer("tab next field · enter submit · esc cancel"))
	return b.String()
}

// runLoginTUI collects email + token (token hidden). Returns empty ok=false if cancelled.
func runLoginTUI(prefillEmail string) (email, token string, ok bool, err error) {
	if !isTTY() {
		r := bufio.NewReader(os.Stdin)
		fmt.Print("Favro email: ")
		e, _ := r.ReadString('\n')
		fmt.Print("Favro API token: ")
		t, _ := r.ReadString('\n')
		return strings.TrimSpace(e), strings.TrimSpace(t), true, nil
	}
	m := newLoginModel(prefillEmail)
	p := tea.NewProgram(m, tea.WithAltScreen())
	res, err := p.Run()
	if err != nil {
		return "", "", false, err
	}
	mm := res.(loginModel)
	if mm.cancelled {
		return "", "", false, nil
	}
	return mm.emailTI.Value(), mm.tokenTI.Value(), true, nil
}

// --- helpers -----------------------------------------------------------------

func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func readLine() string {
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return line
}
