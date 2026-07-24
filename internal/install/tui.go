package install

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/mcpserver"
)

// ErrCancelled is returned when the user aborts an interactive prompt, or when
// a prompt is invoked in a non-interactive context (no TTY).
var ErrCancelled = fmt.Errorf("cancelled")

// isTTY reports whether stdin AND stdout are interactive terminals. The
// bubbletea UI requires both; otherwise we fall back to unattended defaults
// (or return ErrCancelled so the caller can react).
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
	styleHint   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleFooter = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // red
)

func indexOf(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// ===========================================================================
// Unified installer model: the WHOLE interactive flow (toolset -> clients ->
// custom tools -> login -> verifying -> configuring -> done) runs as ONE
// tea.Program, so bubbletea's inline renderer redraws a single screen in place
// instead of appending a new block per step.
// ===========================================================================

type step int

const (
	stepToolset step = iota
	stepClients
	stepCustomTools  // only if Custom chosen
	stepLogin        // only if creds missing
	stepVerifying    // network check while submitting login
	stepConfiguring  // file I/O + exec while applying changes
	stepDone
)

var toolsetOptions = []string{
	"Read-only",
	"Read + Write",
	"Read + Write + Delete",
	"Custom (toggle each tool)",
}

var toolsetValues = []string{
	mcpserver.TierRead,
	mcpserver.TierWrite,
	mcpserver.TierDelete,
	"custom",
}

// clientRow is one row in the clients step. Detected rows are selectable;
// others are visible only when "show all" is toggled on and the cursor skips
// them.
type clientRow struct {
	client   ClientDef
	detected bool
	checked  bool
}

// toolRow is one toggleable line in the custom-tools step.
type toolRow struct {
	id      string
	label   string
	hint    string
	checked bool
}

// applyResult pairs a client with its install/remove outcome during applying.
type applyResult struct {
	client ClientDef
	res    ApplyResult
	action string // "install" | "remove"
}

type installModel struct {
	step   step
	cancel bool

	// toolset
	toolsetCursor int
	toolsetChoice string // resolved once the toolset step is done

	// clients
	clientRows   []clientRow
	clientCursor int
	showAll      bool

	// custom tools
	toolRows   []toolRow
	toolCursor int

	// login
	emailTI    textinput.Model
	tokenTI    textinput.Model
	loginFocus int // 0 = email, 1 = token
	loginError string

	// applying / done
	applyResults []applyResult
	applyError   string

	// resolved data
	opts          Options
	name          string
	email         string // creds to embed (from flags/env) when embedCreds
	token         string
	embedCreds    bool
	needLogin     bool
	detected      []ClientDef
	others        []ClientDef
	wasRegistered map[string]bool
}

func newInstallModel(
	opts Options,
	name, email, token string,
	embedCreds, needLogin bool,
	detected, others []ClientDef,
	wasRegistered map[string]bool,
) *installModel {
	m := &installModel{
		opts:          opts,
		name:          name,
		email:         email,
		token:         token,
		embedCreds:    embedCreds,
		needLogin:     needLogin,
		detected:      detected,
		others:        others,
		wasRegistered: wasRegistered,
	}

	// Clients: detected (pre-checked) then others (hidden until 'v').
	rows := make([]clientRow, 0, len(detected)+len(others))
	for _, c := range detected {
		rows = append(rows, clientRow{client: c, detected: true, checked: true})
	}
	for _, c := range others {
		rows = append(rows, clientRow{client: c, detected: false, checked: false})
	}
	m.clientRows = rows
	if len(detected) > 0 {
		m.clientCursor = 0 // first detected row (kept valid by moveClient)
	}

	// Login inputs (only used if the login step runs).
	e := textinput.New()
	e.Prompt = "  Email:  "
	e.Placeholder = "you@example.com"
	e.CharLimit = 200
	if email != "" {
		e.SetValue(email)
	}
	m.emailTI = e
	t := textinput.New()
	t.Prompt = "  Token:  "
	t.Placeholder = "Favro API token"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 400
	m.tokenTI = t

	// Custom-tool toggles (read+write pre-checked, delete off).
	cat := mcpserver.ToolCatalog()
	tr := make([]toolRow, 0, len(cat))
	for _, info := range cat {
		tr = append(tr, toolRow{
			id:      info.Name,
			label:   info.Name,
			hint:    info.Tier,
			checked: info.Tier == mcpserver.TierRead || info.Tier == mcpserver.TierWrite,
		})
	}
	m.toolRows = tr

	// Starting step: pick the toolset unless it was pre-resolved by a flag.
	switch opts.Toolset {
	case mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete:
		m.toolsetChoice = opts.Toolset
		m.step = stepClients
	case "custom":
		m.toolsetChoice = "custom"
		m.step = stepClients
	default:
		m.step = stepToolset
		m.toolsetCursor = 1 // highlight Read + Write
	}
	return m
}

func (m *installModel) Init() tea.Cmd { return nil }

func (m *installModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
		m.cancel = true
		return m, tea.Quit
	}
	// Async results from verifyCredsCmd / startApply land here:
	switch msg := msg.(type) {
	case credsVerifiedMsg:
		if msg.err != nil {
			m.loginError = fmt.Sprintf("verification failed: %v", msg.err)
			m.tokenTI.SetValue("")
			m.step = stepLogin
			m.emailTI.Focus()
			return m, textinput.Blink
		}
		if err := credentials.Save(m.email, m.token); err != nil {
			m.applyError = fmt.Sprintf("failed to save credentials: %v", err)
			m.step = stepDone
			return m, nil
		}
		m.needLogin = false
		m.step = stepConfiguring
		return m, m.startApply()
	case applyDoneMsg:
		m.applyResults = msg.results
		m.applyError = msg.err
		m.step = stepDone
		return m, nil
	}
	// The login step must also handle non-key messages (cursor blink).
	if m.step == stepLogin {
		return m.updateLogin(msg)
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch m.step {
	case stepToolset:
		return m, m.updateToolset(k)
	case stepClients:
		return m, m.updateClients(k)
	case stepCustomTools:
		return m, m.updateTools(k)
	case stepDone:
		switch k.String() {
		case "enter", "q", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *installModel) updateToolset(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "q", "esc":
		m.cancel = true
		return tea.Quit
	case "up", "k":
		if m.toolsetCursor > 0 {
			m.toolsetCursor--
		}
	case "down", "j":
		if m.toolsetCursor < len(toolsetOptions)-1 {
			m.toolsetCursor++
		}
	case "enter":
		m.toolsetChoice = toolsetValues[m.toolsetCursor]
		m.step = stepClients
	}
	return nil
}

func (m *installModel) updateClients(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "q", "esc":
		m.cancel = true
		return tea.Quit
	case "v":
		m.showAll = !m.showAll
	case "up", "k":
		m.moveClient(-1)
	case "down", "j":
		m.moveClient(1)
	case " ":
		m.toggleClient()
	case "enter":
		if m.toolsetChoice == "custom" {
			m.step = stepCustomTools
			return nil
		}
		return m.toLoginOrApply()
	}
	return nil
}

func (m *installModel) updateTools(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "q", "esc":
		m.cancel = true
		return tea.Quit
	case "up", "k":
		if m.toolCursor > 0 {
			m.toolCursor--
		}
	case "down", "j":
		if m.toolCursor < len(m.toolRows)-1 {
			m.toolCursor++
		}
	case " ":
		if m.toolCursor >= 0 && m.toolCursor < len(m.toolRows) {
			m.toolRows[m.toolCursor].checked = !m.toolRows[m.toolCursor].checked
		}
	case "enter":
		return m.toLoginOrApply()
	}
	return nil
}

func (m *installModel) updateLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.cancel = true
			return m, tea.Quit
		case "tab", "shift+tab":
			if m.loginFocus == 0 {
				m.loginFocus = 1
				m.emailTI.Blur()
				m.tokenTI.Focus()
			} else {
				m.loginFocus = 0
				m.tokenTI.Blur()
				m.emailTI.Focus()
			}
			return m, textinput.Blink
		case "enter":
			return m, m.submitLogin()
		}
	}
	var cmd tea.Cmd
	if m.loginFocus == 0 {
		m.emailTI, cmd = m.emailTI.Update(msg)
	} else {
		m.tokenTI, cmd = m.tokenTI.Update(msg)
	}
	return m, cmd
}

// submitLogin captures the entered credentials and kicks off an async
// verification against the Favro API. The UI flips to stepVerifying while the
// network call runs; the result is handled in Update via credsVerifiedMsg.
func (m *installModel) submitLogin() tea.Cmd {
	email := strings.TrimSpace(m.emailTI.Value())
	token := strings.TrimSpace(m.tokenTI.Value())
	if email == "" || token == "" {
		m.loginError = "email and token are required"
		return nil
	}
	m.email = email
	m.token = token
	m.loginError = ""
	m.step = stepVerifying
	return verifyCredsCmd(email, token)
}

// toLoginOrApply advances from clients/custom-tools to the login step (if creds
// are still missing) or straight to the async apply pipeline.
func (m *installModel) toLoginOrApply() tea.Cmd {
	if m.needLogin {
		m.step = stepLogin
		m.loginError = ""
		m.loginFocus = 0
		m.emailTI.Focus()
		m.tokenTI.Blur()
		return textinput.Blink
	}
	m.step = stepConfiguring
	return m.startApply()
}

func (m *installModel) moveClient(dir int) {
	idxs := m.detectedIdxs()
	if len(idxs) == 0 {
		return
	}
	pos := indexOf(idxs, m.clientCursor)
	if pos < 0 {
		m.clientCursor = idxs[0]
		return
	}
	pos += dir
	if pos < 0 {
		pos = len(idxs) - 1
	} else if pos >= len(idxs) {
		pos = 0
	}
	m.clientCursor = idxs[pos]
}

func (m *installModel) toggleClient() {
	if m.clientCursor >= 0 && m.clientCursor < len(m.clientRows) && m.clientRows[m.clientCursor].detected {
		m.clientRows[m.clientCursor].checked = !m.clientRows[m.clientCursor].checked
	}
}

func (m installModel) detectedIdxs() []int {
	out := make([]int, 0, len(m.clientRows))
	for i, r := range m.clientRows {
		if r.detected {
			out = append(out, i)
		}
	}
	return out
}

// computeEnv builds the env block written into each client config from the
// chosen toolset / tools and (only when explicitly provided) credentials.
func (m *installModel) computeEnv() map[string]string {
	env := map[string]string{}
	switch m.toolsetChoice {
	case "custom":
		var ids []string
		for _, r := range m.toolRows {
			if r.checked {
				ids = append(ids, r.id)
			}
		}
		if len(ids) == 0 {
			env["FAVRO_TOOLSET"] = mcpserver.TierWrite
		} else {
			env["FAVRO_TOOLS"] = strings.Join(ids, ",")
		}
	case mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete:
		env["FAVRO_TOOLSET"] = m.toolsetChoice
	}
	if m.embedCreds {
		env["FAVRO_EMAIL"] = m.email
		env["FAVRO_API_TOKEN"] = m.token
	}
	return env
}

// credsVerifiedMsg is returned by verifyCredsCmd with the verification outcome.
type credsVerifiedMsg struct{ err error }

// verifyCredsCmd checks the given credentials against the live Favro API and
// reports success/failure via credsVerifiedMsg. Runs off the main loop so the
// UI can show a "Verifying..." state instead of freezing.
func verifyCredsCmd(email, token string) tea.Cmd {
	return func() tea.Msg {
		_, err := favro.NewClient(email, token, "").GetOrganizations()
		return credsVerifiedMsg{err: err}
	}
}

// applyDoneMsg carries the results (or fatal error) of an apply run.
type applyDoneMsg struct {
	results []applyResult
	err     string
}

// startApply returns a tea.Cmd that runs every selected client's install (and
// unregisters deselected previously-registered clients) off the main loop and
// reports results via applyDoneMsg. Inputs are snapshotted into locals so the
// goroutine never races with the model's state.
func (m *installModel) startApply() tea.Cmd {
	chosenSet := map[string]bool{}
	for _, r := range m.clientRows {
		if r.detected && r.checked {
			chosenSet[r.client.ID] = true
		}
	}
	detectedSet := map[string]bool{}
	for _, c := range m.detected {
		detectedSet[c.ID] = true
	}
	env := m.computeEnv()
	name := m.name
	dryRun := m.opts.DryRun
	wasRegistered := m.wasRegistered
	return func() tea.Msg {
		target, err := serverTarget(env)
		if err != nil {
			return applyDoneMsg{err: err.Error()}
		}
		var results []applyResult
		for _, c := range Clients {
			if !detectedSet[c.ID] {
				continue
			}
			switch {
			case chosenSet[c.ID]:
				r := applyClient(c, name, target, dryRun)
				results = append(results, applyResult{client: c, res: r, action: "install"})
			case wasRegistered[c.ID]:
				r := applyRemove(c, name, dryRun)
				results = append(results, applyResult{client: c, res: r, action: "remove"})
			}
		}
		return applyDoneMsg{results: results}
	}
}

func (m installModel) resultLine(r applyResult) string {
	if r.action == "remove" {
		return describeRemove(r.res, m.opts.DryRun)
	}
	return describe(r.res)
}

// --- views ------------------------------------------------------------------

func (m *installModel) View() string {
	switch m.step {
	case stepToolset:
		return m.viewToolset()
	case stepClients:
		return m.viewClients()
	case stepCustomTools:
		return m.viewTools()
	case stepLogin:
		return m.viewLogin()
	case stepVerifying:
		return styleTitle.Render("favro-mcp installer") + "\n\nVerifying credentials..."
	case stepConfiguring:
		return styleTitle.Render("favro-mcp installer") + "\n\nConfiguring clients..."
	case stepDone:
		return m.viewDone()
	}
	return ""
}

func (m *installModel) viewToolset() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("favro-mcp installer"))
	b.WriteString("\nToolset - which tools should the server expose?\n\n")
	for i, o := range toolsetOptions {
		cursor := " "
		if i == m.toolsetCursor {
			cursor = styleCursor.Render(">")
		}
		dot := " "
		if i == m.toolsetCursor {
			dot = styleOn.Render("●")
		}
		label := o
		if o == "Read + Write" {
			label += styleHint.Render("  (recommended)")
		}
		b.WriteString(fmt.Sprintf(" %s %s %s\n", cursor, dot, label))
	}
	b.WriteString("\n" + styleFooter.Render("up/down move . enter select . q cancel"))
	return b.String()
}

func (m *installModel) viewClients() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("favro-mcp installer"))
	b.WriteString("\nAI Clients - register with which clients?\n\n")
	anyShown := false
	for i, r := range m.clientRows {
		if !r.detected && !m.showAll {
			continue
		}
		anyShown = true
		cursor := " "
		if i == m.clientCursor && r.detected {
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
	if !anyShown {
		b.WriteString(styleDim.Render("  No clients detected. Install one (Claude Desktop,\n  Cursor, VS Code, ...) and re-run configure.\n"))
	}
	if len(m.others) > 0 {
		hint := "press v to show other known clients"
		if m.showAll {
			hint = "press v to hide non-detected"
		}
		b.WriteString("\n" + styleDim.Render(hint))
	}
	b.WriteString("\n\n" + styleFooter.Render("up/down move . space toggle . v show all . enter confirm . q cancel"))
	return b.String()
}

func (m *installModel) viewTools() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("favro-mcp installer"))
	b.WriteString("\nCustom tools - toggle each tool (read+write pre-checked).\n\n")
	for i, r := range m.toolRows {
		cursor := " "
		if i == m.toolCursor {
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
	b.WriteString("\n" + styleFooter.Render("up/down move . space toggle . enter confirm . q cancel"))
	return b.String()
}

func (m *installModel) viewLogin() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("favro-mcp installer"))
	b.WriteString("\nLog in to Favro - credentials are verified then saved.\n\n")
	b.WriteString(m.emailTI.View() + "\n\n")
	b.WriteString(m.tokenTI.View() + "\n\n")
	if m.loginError != "" {
		b.WriteString(styleErr.Render(m.loginError) + "\n\n")
	}
	b.WriteString(styleFooter.Render("tab switch fields . enter submit . esc cancel"))
	return b.String()
}

func (m *installModel) viewDone() string {
	var b strings.Builder
	if m.applyError != "" {
		b.WriteString(styleErr.Render("Error: " + m.applyError) + "\n\n")
	} else if len(m.applyResults) == 0 {
		b.WriteString("Nothing selected; no changes made.\n\n")
	} else {
		if m.opts.DryRun {
			b.WriteString(styleDim.Render("Dry run - no files were changed.") + "\n")
		}
		allNoop := len(m.applyResults) > 0
		for _, r := range m.applyResults {
			if r.res.Status != "noop" {
				allNoop = false
				break
			}
		}
		if allNoop {
			b.WriteString("All selected clients are already configured. No changes needed.\n")
		} else {
			for _, r := range m.applyResults {
				b.WriteString(fmt.Sprintf("  %s: %s\n", r.client.Name, m.resultLine(r)))
			}
		}
		b.WriteString("\n")
		if !allNoop {
			b.WriteString("Restart your AI clients for changes to take effect.\n\n")
		}
	}
	b.WriteString(styleFooter.Render("Run favro-mcp to configure, login, or interact with Favro from the console."))
	b.WriteString("\n" + styleFooter.Render("enter to exit"))
	return b.String()
}

// runForm runs the entire interactive installer as a single tea.Program so the
// screen evolves in place instead of appending a new block per step. Returns
// ErrCancelled on a non-TTY or if the user aborts. The model is mutated in
// place; the caller reads results from it after this returns.
func runForm(m *installModel) error {
	if !isTTY() {
		return ErrCancelled
	}
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return err
	}
	if m.cancel {
		return ErrCancelled
	}
	return nil
}

// ===========================================================================
// Standalone login model (used by RunApp's "Log in to Favro" menu entry; the
// installer flow uses installModel's stepLogin instead).
// ===========================================================================

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

// ===========================================================================
// Generic single-select (RunApp menu) and multi-select (RunUninstall)
// ===========================================================================

// selectModel is an arrow-key single-choice list. It renders in place via
// bubbletea's inline (no-alt-screen) renderer.
type selectModel struct {
	title   string
	options []string
	hint    string
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
		b.WriteString(fmt.Sprintf(" %s %s %s\n", cursor, dot, label))
	}
	b.WriteString("\n" + styleFooter.Render(m.footer))
	return b.String()
}

// multiRow is one toggleable line in a multiModel.
type multiRow struct {
	id      string
	label   string
	hint    string
	checked bool
}

// multiModel is an arrow-key multi-select. Used by RunUninstall.
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
