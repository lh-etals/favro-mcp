// Package app implements the interactive CLI app shell that a bare
// `favro-mcp` invocation opens on a real terminal. It owns the top-level
// arrow-key menu whose contents depend on login state and dispatches into
// installer / data / login flows.
package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/install"
	"golang.org/x/term"
)

// isTerminal reports whether stdin AND stdout are interactive TTYs (bubbletea
// inline mode requires both).
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// RunApp is the interactive CLI app. Shows a state-dependent main menu:
//   - Logged out: Log in, Configure AI clients, Quit
//   - Logged in:  Browse boards, List cards, View card detail, Users, Tags,
//     Switch organization, Configure AI clients, Log out, Quit
//
// It uses bubbletea inline mode (no alt-screen) and loops until the user quits.
func RunApp() {
	if !isTerminal() {
		fmt.Fprintln(os.Stderr, "favro-mcp: not a terminal. Run `favro-mcp mcp` to start the MCP server.")
		return
	}
	for {
		loggedIn := credentials.Exists()
		items := buildMenu(loggedIn)
		labels := make([]string, len(items))
		for i, it := range items {
			labels[i] = it.label
		}
		title := "favro-mcp"
		if loggedIn {
			if s, err := NewSession(); err == nil {
				title = "favro-mcp  " + s.email
			}
		}
		m := selectModel{
			title:   title,
			options: labels,
			footer:  "up/down move . enter select . q quit",
		}
		p := tea.NewProgram(m)
		out, err := p.Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, "  Error:", err)
			return
		}
		r := out.(selectModel)
		if r.cancel || r.cursor < 0 || r.cursor >= len(items) {
			return
		}
		action := items[r.cursor].action
		if action == nil { // Quit
			return
		}
		action()
	}
}

// menuItem pairs a menu label with the action that runs when it is selected. A
// nil action means "quit the app".
type menuItem struct {
	label  string
	action func()
}

func buildMenu(loggedIn bool) []menuItem {
	if !loggedIn {
		return []menuItem{
			{label: "Log in to Favro", action: runLoginFlow},
			{label: "Configure AI clients", action: runConfigure},
			{label: "Quit", action: nil},
		}
	}
	return []menuItem{
		{label: "Browse boards", action: runBrowseBoards},
		{label: "List cards", action: runListCards},
		{label: "View card detail", action: runViewCardDetail},
		{label: "Users", action: runUsers},
		{label: "Tags", action: runTags},
		{label: "Switch organization", action: runSwitchOrg},
		{label: "Configure AI clients", action: runConfigure},
		{label: "Log out", action: showAccountInfo},
		{label: "Quit", action: nil},
	}
}

// --- actions ---------------------------------------------------------------

func runConfigure() {
	fmt.Println()
	if err := install.RunInstall(install.Options{}); err != nil && !errors.Is(err, install.ErrCancelled) {
		fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
	}
	fmt.Println()
}

// showAccountInfo doubles as the "Log out" entry: rather than deleting stored
// credentials, it shows the active account and hints at `favro-mcp login` to
// switch.
func showAccountInfo() {
	fmt.Println()
	s, err := NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Not logged in."))
		fmt.Println()
		return
	}
	fmt.Printf("  Logged in as: %s\n", s.email)
	if id := s.OrgID(); id != "" {
		fmt.Printf("  Active org:   %s\n", id)
	} else {
		fmt.Printf("  Active org:   %s\n", styleDim.Render("(none selected)"))
	}
	fmt.Println(styleDim.Render("  Run `favro-mcp login` to switch accounts."))
	fmt.Println()
}

// runSwitchOrg lists the user's organizations and sets the active one.
func runSwitchOrg() {
	fmt.Println()
	s, err := NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Not logged in."))
		fmt.Println()
		return
	}
	orgs, err := s.Client().GetOrganizations()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Failed to load organizations: "+err.Error()))
		fmt.Println()
		return
	}
	if len(orgs) == 0 {
		fmt.Println(styleDim.Render("  Your account has no organizations."))
		fmt.Println()
		return
	}
	if len(orgs) == 1 {
		o := orgs[0]
		s.SetOrg(o.OrganizationID)
		fmt.Printf("  Only one organization: %s\n\n", styleSuccess.Render(o.Name))
		return
	}
	labels := make([]string, len(orgs))
	cur := s.OrgID()
	for i, o := range orgs {
		label := o.Name
		if o.OrganizationID == cur {
			label += " " + styleSuccess.Render("(active)")
		}
		labels[i] = label
	}
	m := selectModel{
		title:   "Switch organization",
		options: labels,
		footer:  "up/down move . enter select . q cancel",
	}
	p := tea.NewProgram(m)
	out, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
		fmt.Println()
		return
	}
	r := out.(selectModel)
	if r.cancel {
		fmt.Println()
		return
	}
	if r.cursor >= 0 && r.cursor < len(orgs) {
		s.SetOrg(orgs[r.cursor].OrganizationID)
		fmt.Printf("  Active organization: %s\n\n", styleSuccess.Render(orgs[r.cursor].Name))
		return
	}
	fmt.Println()
}

// runLoginFlow prompts for email + token via the TUI, verifies them against
// the live Favro API, and persists them only on success. Loops on failure
// until cancelled.
func runLoginFlow() {
	fmt.Println()
	prefill := ""
	for {
		p := tea.NewProgram(newLoginModel(prefill))
		out, err := p.Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
			fmt.Println()
			return
		}
		m := out.(loginModel)
		if m.cancel {
			fmt.Println()
			return
		}
		email := strings.TrimSpace(m.email.Value())
		token := strings.TrimSpace(m.token.Value())
		if email == "" || token == "" {
			fmt.Println()
			return
		}
		if _, err := favro.NewClient(email, token, "").GetOrganizations(); err != nil {
			fmt.Printf("\n  %s\n  %s\n\n",
				styleError.Render("Verification failed: "+err.Error()),
				styleDim.Render("Please try again."))
			prefill = email
			continue
		}
		if err := credentials.Save(email, token); err != nil {
			fmt.Fprintln(os.Stderr, styleError.Render("  Failed to save credentials: "+err.Error()))
			fmt.Println()
			return
		}
		fmt.Printf("  %s\n\n", styleSuccess.Render("Signed in as "+email))
		return
	}
}

// ===========================================================================
// TUI models (bubbletea inline mode - no alt-screen)
// ===========================================================================

// selectModel is an arrow-key single-choice list rendered inline.
type selectModel struct {
	title   string
	options []string
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
		if i == m.cursor {
			dot = styleSuccess.Render("●")
		}
		b.WriteString(fmt.Sprintf(" %s %s %s\n", cursor, dot, o))
	}
	b.WriteString("\n" + styleFooter.Render(m.footer))
	return b.String()
}

// loginModel is the email + token form used by runLoginFlow.
type loginModel struct {
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

	return loginModel{email: e, token: t, focused: 0}
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
	b.WriteString(styleTitle.Render("Log in to Favro"))
	b.WriteString("\n\n")
	b.WriteString(m.email.View() + "\n\n")
	b.WriteString(m.token.View() + "\n\n")
	b.WriteString(styleFooter.Render("tab switch fields . enter submit . esc cancel"))
	return b.String()
}
