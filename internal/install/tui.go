package install

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/lh-etals/favro-mcp/internal/mcpserver"
)

// ErrCancelled is returned when the user aborts an interactive prompt, or when
// a prompt is invoked in a non-interactive context (no TTY).
var ErrCancelled = fmt.Errorf("cancelled")

var sharedReader = bufio.NewReader(os.Stdin)

func readLine() string {
	line, _ := sharedReader.ReadString('\n')
	return strings.TrimSpace(line)
}

// isTTY reports whether stdin AND stdout are interactive terminals. The
// bubbletea prompts require both; otherwise we fall back to plain text prompts
// (or return ErrCancelled so the caller can use sensible defaults).
func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// --- shared styles -----------------------------------------------------------

var (
	styleTitle  = lipgloss.NewStyle().Bold(true)
	styleCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("14")) // cyan
	styleOn     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	styleOff    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // bright-black / grey
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleHint   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan-ish
	styleFooter = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// ===========================================================================
// Toolset single-select
// ===========================================================================

// selectModel is an arrow-key single-choice list. It renders in place via
// bubbletea's inline (no-alt-screen) renderer.
type selectModel struct {
	title   string
	options []string
	hint    string // optional subtitle
	footer  string
	cursor  int
	cancel  bool
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case "enter":
		return m, tea.Quit
	}
	return m, nil
}

func (m selectModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.title))
	b.WriteString("\n\n")
	for i, o := range m.options {
		cursor := " "
		if i == m.cursor {
			cursor = styleCursor.Render(">")
		}
		dot := " "
		label := o
		if i == m.cursor {
			dot = styleOn.Render("●")
		}
		if i == 1 && strings.Contains(m.title, "Toolset") {
			label += styleHint.Render("  (recommended)")
		}
		b.WriteString(fmt.Sprintf(" %s %s %s\n", cursor, dot, label))
	}
	b.WriteString("\n" + styleFooter.Render(m.footer))
	return b.String()
}

// promptToolset runs the toolset single-select and returns one of
// TierRead/TierWrite/TierDelete or "custom". On a non-TTY or cancel it returns
// ErrCancelled.
func promptToolset() (string, error) {
	if !isTTY() {
		return "", ErrCancelled
	}
	m := selectModel{
		title:   "Toolset - which tools should the server expose?",
		options: []string{"Read-only", "Read + Write", "Read + Write + Delete", "Custom (toggle each tool)"},
		cursor:  1,
		footer:  "up/down move . enter select . q cancel",
	}
	p := tea.NewProgram(m)
	out, err := p.Run()
	if err != nil {
		return "", err
	}
	r := out.(selectModel)
	if r.cancel {
		return "", ErrCancelled
	}
	vals := []string{mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete, "custom"}
	return vals[r.cursor], nil
}

// ===========================================================================
// Generic multi-select (tools, uninstall)
// ===========================================================================

// multiRow is one toggleable line in a multiModel.
type multiRow struct {
	id      string
	label   string
	hint    string
	checked bool
}

// multiModel is an arrow-key multi-select. Every row is selectable.
type multiModel struct {
	title  string
	footer string
	rows   []multiRow
	cursor int
	cancel bool
}

func (m multiModel) Init() tea.Cmd { return nil }

func (m multiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case " ":
		if m.cursor >= 0 && m.cursor < len(m.rows) {
			m.rows[m.cursor].checked = !m.rows[m.cursor].checked
		}
	case "enter":
		return m, tea.Quit
	}
	return m, nil
}

func (m multiModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.title))
	b.WriteString("\n\n")
	for i, r := range m.rows {
		cursor := " "
		if i == m.cursor {
			cursor = styleCursor.Render(">")
		}
		mark := styleOff.Render("[ ]")
		if r.checked {
			mark = styleOn.Render("[x]")
		}
		label := r.label
		if r.hint != "" {
			label = fmt.Sprintf("%-26s %s", r.label, styleDim.Render("("+r.hint+")"))
		}
		b.WriteString(fmt.Sprintf(" %s %s %s\n", cursor, mark, label))
	}
	b.WriteString("\n" + styleFooter.Render(m.footer))
	return b.String()
}

// runMultiSelect runs a multiModel and returns the final rows. On non-TTY or
// cancel, returns ErrCancelled.
func runMultiSelect(title string, rows []multiRow) ([]multiRow, error) {
	if !isTTY() {
		return nil, ErrCancelled
	}
	if len(rows) == 0 {
		return nil, nil
	}
	m := multiModel{
		title:  title,
		footer: "up/down move . space toggle . enter confirm . q cancel",
		rows:   rows,
	}
	p := tea.NewProgram(m)
	out, err := p.Run()
	if err != nil {
		return nil, err
	}
	r := out.(multiModel)
	if r.cancel {
		return nil, ErrCancelled
	}
	return r.rows, nil
}

// promptTools is the Custom toolset tool toggle. Read+Write tools are
// pre-checked, Delete is off. Returns the selected tool names.
func promptTools(catalog []mcpserver.ToolInfo) ([]string, error) {
	if !isTTY() {
		return nil, ErrCancelled
	}
	rows := make([]multiRow, len(catalog))
	for i, t := range catalog {
		rows[i] = multiRow{
			id:      t.Name,
			label:   t.Name,
			hint:    t.Tier,
			checked: t.Tier == mcpserver.TierRead || t.Tier == mcpserver.TierWrite,
		}
	}
	out, err := runMultiSelect("Toggle tools", rows)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, r := range out {
		if r.checked {
			ids = append(ids, r.id)
		}
	}
	return ids, nil
}

// ===========================================================================
// Client multi-select (detected / not-detected with v toggle)
// ===========================================================================

// clientRow is one row in clientModel. Detected rows are selectable; others
// are visible only when "show all" is toggled on, and the cursor skips them.
type clientRow struct {
	client   ClientDef
	detected bool
	checked  bool
}

// clientModel is the main client-selection screen.
type clientModel struct {
	title   string
	footer  string
	rows    []clientRow
	cursor  int // index into rows; always points at a detected row
	showAll bool
	cancel  bool
}

func newClientModel(detected, others []ClientDef) clientModel {
	rows := make([]clientRow, 0, len(detected)+len(others))
	for _, c := range detected {
		rows = append(rows, clientRow{client: c, detected: true, checked: true})
	}
	for _, c := range others {
		rows = append(rows, clientRow{client: c, detected: false, checked: false})
	}
	m := clientModel{
		title:  "AI Clients",
		footer: "up/down move . space toggle . v show all . enter confirm . q cancel",
		rows:   rows,
	}
	if len(detected) > 0 {
		m.cursor = 0 // first detected row
	}
	return m
}

// detectedIdxs returns the indices of detected rows.
func (m clientModel) detectedIdxs() []int {
	out := make([]int, 0, len(m.rows))
	for i, r := range m.rows {
		if r.detected {
			out = append(out, i)
		}
	}
	return out
}

func (m clientModel) Init() tea.Cmd { return nil }

func (m clientModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "ctrl+c", "q", "esc":
		m.cancel = true
		return m, tea.Quit
	case "v":
		m.showAll = !m.showAll
	case "up", "k":
		idxs := m.detectedIdxs()
		if len(idxs) == 0 {
			return m, nil
		}
		pos := indexOf(idxs, m.cursor)
		if pos < 0 {
			m.cursor = idxs[0]
		} else if pos > 0 {
			m.cursor = idxs[pos-1]
		} else {
			m.cursor = idxs[len(idxs)-1] // wrap
		}
	case "down", "j":
		idxs := m.detectedIdxs()
		if len(idxs) == 0 {
			return m, nil
		}
		pos := indexOf(idxs, m.cursor)
		if pos < 0 {
			m.cursor = idxs[0]
		} else if pos < len(idxs)-1 {
			m.cursor = idxs[pos+1]
		} else {
			m.cursor = idxs[0] // wrap
		}
	case " ":
		if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].detected {
			m.rows[m.cursor].checked = !m.rows[m.cursor].checked
		}
	case "enter":
		return m, tea.Quit
	}
	return m, nil
}

func (m clientModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.title))
	b.WriteString("\n\n")
	for i, r := range m.rows {
		if !r.detected && !m.showAll {
			continue
		}
		cursor := " "
		if i == m.cursor && r.detected {
			cursor = styleCursor.Render(">")
		}
		var mark, label string
		if r.detected {
			if r.checked {
				mark = styleOn.Render("[x]")
			} else {
				mark = styleOff.Render("[ ]")
			}
			label = fmt.Sprintf("%-22s %s", r.client.Name, styleDim.Render("(detected)"))
		} else {
			mark = styleDim.Render("[ ]")
			label = styleDim.Render(fmt.Sprintf("%-22s (not detected)", r.client.Name))
		}
		b.WriteString(fmt.Sprintf(" %s %s %s\n", cursor, mark, label))
	}
	b.WriteString("\n" + styleFooter.Render(m.footer))
	return b.String()
}

func indexOf(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// promptClients runs the client multi-select. Detected clients are pre-checked
// and selectable; others are hidden until the user presses 'v' and remain
// non-selectable. Returns the IDs of checked clients (or nil for "none
// detected" so the caller can fall back to all detected).
func promptClients(detected, others []ClientDef) ([]string, error) {
	if !isTTY() {
		return nil, ErrCancelled
	}
	if len(detected) == 0 {
		fmt.Printf("\n  AI Clients\n\n")
		fmt.Printf("  No clients detected. You can install one (Claude Desktop,\n")
		fmt.Printf("  Cursor, VS Code, ...) and re-run `favro-mcp configure`.\n\n")
		if len(others) > 0 {
			fmt.Printf("  Known but not detected:\n")
			for _, c := range others {
				fmt.Printf("    - %s\n", c.Name)
			}
			fmt.Println()
		}
		return nil, nil
	}
	p := tea.NewProgram(newClientModel(detected, others))
	out, err := p.Run()
	if err != nil {
		return nil, err
	}
	m := out.(clientModel)
	if m.cancel {
		return nil, ErrCancelled
	}
	var ids []string
	for _, r := range m.rows {
		if r.detected && r.checked {
			ids = append(ids, r.client.ID)
		}
	}
	return ids, nil
}

// ===========================================================================
// Login (two text inputs, tab to switch, enter to submit)
// ===========================================================================

type loginModel struct {
	title   string
	footer  string
	email   textinput.Model
	token   textinput.Model
	focused int // 0 = email, 1 = token
	cancel  bool
}

func newLoginModel(prefill string) loginModel {
	e := textinput.New()
	e.Prompt = "  Email:  "
	e.Placeholder = "you@example.com"
	e.CharLimit = 200
	if prefill != "" {
		e.SetValue(prefill)
	}
	e.Focus()

	t := textinput.New()
	t.Prompt = "  Token:  "
	t.Placeholder = "Favro API token"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 400

	return loginModel{
		title:   "Log in to Favro",
		footer:  "tab switch fields . enter submit . esc cancel",
		email:   e,
		token:   t,
		focused: 0,
	}
}

func (m loginModel) Init() tea.Cmd { return textinput.Blink }

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c", "esc":
			m.cancel = true
			return m, tea.Quit
		case "tab", "shift+tab":
			if m.focused == 0 {
				m.focused = 1
				m.email.Blur()
				m.token.Focus()
			} else {
				m.focused = 0
				m.token.Blur()
				m.email.Focus()
			}
			return m, textinput.Blink
		case "enter":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	if m.focused == 0 {
		m.email, cmd = m.email.Update(msg)
	} else {
		m.token, cmd = m.token.Update(msg)
	}
	return m, cmd
}

func (m loginModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.title))
	b.WriteString("\n\n")
	b.WriteString(m.email.View() + "\n\n")
	b.WriteString(m.token.View() + "\n\n")
	b.WriteString(styleFooter.Render(m.footer))
	return b.String()
}

// promptLogin runs the login TUI and returns trimmed email + token. An empty
// field (or cancel) returns ErrCancelled.
func promptLogin(prefillEmail string) (email, token string, err error) {
	if !isTTY() {
		return "", "", ErrCancelled
	}
	p := tea.NewProgram(newLoginModel(prefillEmail))
	out, err := p.Run()
	if err != nil {
		return "", "", err
	}
	m := out.(loginModel)
	if m.cancel {
		return "", "", ErrCancelled
	}
	email = strings.TrimSpace(m.email.Value())
	token = strings.TrimSpace(m.token.Value())
	if email == "" || token == "" {
		return "", "", ErrCancelled
	}
	return email, token, nil
}

// ===========================================================================
// RunApp - top-level menu for bare `favro-mcp`
// ===========================================================================

// RunApp is the interactive CLI app launched by bare `favro-mcp` on a real
// terminal. It shows an arrow-key menu and loops until the user quits.
func RunApp() {
	for {
		m := selectModel{
			title:   "favro-mcp",
			options: []string{"Configure AI clients", "Log in to Favro", "Quit"},
			footer:  "up/down move . enter select . q quit",
		}
		p := tea.NewProgram(m)
		out, err := p.Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, "  Error:", err)
			return
		}
		r := out.(selectModel)
		if r.cancel || r.cursor == 2 {
			return
		}
		switch r.cursor {
		case 0:
			if err := RunInstall(Options{}); err != nil && !errors.Is(err, ErrCancelled) {
				fmt.Fprintln(os.Stderr, "  Error:", err)
			}
		case 1:
			if err := interactiveLogin(""); err != nil && !errors.Is(err, ErrCancelled) {
				fmt.Fprintln(os.Stderr, "  Error:", err)
			}
		}
	}
}
