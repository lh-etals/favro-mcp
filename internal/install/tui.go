package install

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/mcpserver"
	detectharness "github.com/sairaph/detect-harness"
)

// The installer is one program whose screen evolves in place, in the shape
// sana-mcp and interactive-terminal-mcp use: a bold header, one section line,
// the content, and a footer. Printing each step instead appends a new block per
// question and leaves the finished ones stranded above the current one.
var (
	styleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	styleOn     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleOff    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleError  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleHint   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleFooter = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// Options carries everything Run needs, streams included, so the whole flow is
// wired through one seam and there is exactly one TTY decision.
type Options struct {
	Uninstall bool
	DryRun    bool
	Yes       bool
	Name      string
	Toolset   string // "", "read", "write", "delete", or "custom"

	// Credentials written into each client's env block (instead of the login
	// store). If empty, the server reads `favro-mcp login` creds at runtime.
	Email string
	Token string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// step is one screen of the installer.
type step int

const (
	// stepDetecting is first because probing every client walks the filesystem
	// and takes a visible moment. Without a screen for it the installer prints
	// its download bar and then shows nothing at all.
	stepDetecting step = iota
	stepToolset
	stepCustomTools // reachable only from Custom in stepToolset
	stepHarnesses
	stepApplying
	stepLogin
	stepEmail
	stepToken
	stepVerifying
	stepDone
	// stepPlan and stepRemoving belong to uninstall, which is the same program
	// with a different first screen.
	stepPlan
	stepRemoving
)

// header matches the installer script's own output: a leading blank line and a
// two-space indent, so the download and the setup read as one thing rather than
// two programs taking turns.
func header() string {
	return "\n" + styleTitle.Render("  favro-mcp setup")
}

func uninstallHeader() string {
	return "\n" + styleTitle.Render("  favro-mcp uninstall")
}

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

// toolRow is one toggleable line in the custom-tools step.
type toolRow struct {
	id      string
	tier    string
	checked bool
}

type model struct {
	ctx  context.Context
	opts Options
	name string
	// installer carries DefaultEnv for detection and is rebuilt with the chosen
	// env just before applying.
	installer *Installer

	step  step
	frame int

	// toolset
	toolsetCursor int
	toolsetChoice string
	toolRows      []toolRow
	toolCursor    int

	// clients
	harnesses []Harness
	selected  map[detectharness.ID]bool
	cursor    int
	showAll   bool
	results   []detectharness.Result

	// choices, used by every yes/no screen
	choice int

	// login
	signedIn bool
	email    string
	token    string
	// embedCreds means credentials came in via flags or FAVRO_* env and are
	// written into each client's env block instead of the login store.
	embedCreds bool
	input      string

	// unreachable is the PATH line to add when the installed command will not
	// resolve from a new shell. The script exports the directory for its own
	// child processes, so the process PATH proves nothing here.
	unreachable string

	// uninstall
	registered []detectharness.ID
	removed    bool

	// settled marks the point of no return: client configuration written.
	// Nothing after it may be described as a run that changed nothing.
	settled bool
	message string
	failure string
	cancel  bool
}

type detectedMsg []Harness
type appliedMsg []detectharness.Result
type removedMsg []detectharness.Result
type spinMsg struct{}
type verifiedMsg struct{ err error }

// spinFrames are deliberately plain. The installer is the first thing a person
// sees, sometimes in a console whose font has no braille glyphs, and a row of
// replacement boxes is a worse first impression than a character that draws.
var spinFrames = []string{"-", "\\", "|", "/"}

func (m *model) spinner() string { return spinFrames[m.frame%len(spinFrames)] }

// spinning is every step whose view draws a spinner.
func (m *model) spinning() bool {
	switch m.step {
	case stepDetecting, stepApplying, stepRemoving, stepVerifying:
		return true
	}
	return false
}

func spin() tea.Cmd {
	return tea.Tick(110*time.Millisecond, func(time.Time) tea.Msg { return spinMsg{} })
}

func (m *model) Init() tea.Cmd {
	if m.step == stepPlan {
		return nil
	}
	return tea.Batch(spin(), m.detect())
}

func (m *model) detect() tea.Cmd {
	return func() tea.Msg { return detectedMsg(m.installer.Detect(m.ctx)) }
}

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case spinMsg:
		// Every step that draws a spinner has to keep asking for frames, or it
		// renders one frozen character that reads as a stall.
		if !m.spinning() {
			return m, nil
		}
		m.frame++
		return m, spin()

	case detectedMsg:
		m.adopt(message)
		return m, nil

	case appliedMsg:
		m.results = message
		// Client configuration has been written, so this is the point of no
		// return - except on a dry run, which changed nothing and must still be
		// free to cancel out of.
		if !m.opts.DryRun {
			m.settled = true
		}
		m.unreachable = pathHint()
		if m.signedIn {
			m.step = stepDone
			return m, nil
		}
		m.step, m.choice, m.message = stepLogin, 0, ""
		return m, nil

	case verifiedMsg:
		if message.err != nil {
			// Wrong credentials or an unreachable Favro is recoverable: it
			// belongs on the screen the user is on, not in the fatal-error path.
			m.message = "verification failed: " + message.err.Error()
			m.step, m.input, m.token = stepEmail, m.email, ""
			return m, nil
		}
		if err := credentials.Save(m.email, m.token); err != nil {
			m.failure = "failed to save credentials: " + err.Error()
			return m, tea.Quit
		}
		m.signedIn, m.message = true, ""
		m.step = stepDone
		return m, nil

	case removedMsg:
		m.settled = !m.opts.DryRun
		m.results, m.removed = message, true
		m.step = stepDone
		return m, nil

	case tea.KeyMsg:
		return m.key(message)
	}
	return m, nil
}

func (m *model) key(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Applying, removing, and verifying take no keys at all, ctrl+c included.
	// Each is a few seconds of writes or one network call, and interrupting one
	// leaves half the clients registered under a message saying nothing
	// happened.
	if m.step == stepApplying || m.step == stepRemoving || m.step == stepVerifying {
		return m, nil
	}
	if key.String() == "ctrl+c" {
		m.cancel = true
		return m, tea.Quit
	}
	switch m.step {
	case stepDetecting:
		// Nothing has been written yet, so leaving is free.
		if name := key.String(); name == "q" || name == "esc" {
			m.cancel = true
			return m, tea.Quit
		}
	case stepToolset:
		return m.toolsetKey(key)
	case stepCustomTools:
		return m.toolsKey(key)
	case stepHarnesses:
		return m.harnessKey(key)
	case stepLogin:
		return m.chooseKey(key, 2, m.confirmSignIn)
	case stepEmail, stepToken:
		return m.inputKey(key)
	case stepPlan:
		return m.chooseKey(key, 2, m.confirmUninstall)
	case stepDone:
		switch key.String() {
		case "enter", "esc", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

// chooseKey drives a two-option list. confirm is called with the chosen index.
func (m *model) chooseKey(key tea.KeyMsg, options int, confirm func(int) tea.Cmd) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		m.choice = (m.choice - 1 + options) % options
	case "down", "j":
		m.choice = (m.choice + 1) % options
	case "enter":
		return m, confirm(m.choice)
	case "esc", "q":
		return m, confirm(options - 1)
	}
	return m, nil
}

func (m *model) toolsetKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		m.toolsetCursor = (m.toolsetCursor - 1 + len(toolsetOptions)) % len(toolsetOptions)
	case "down", "j":
		m.toolsetCursor = (m.toolsetCursor + 1) % len(toolsetOptions)
	case "enter":
		m.toolsetChoice = toolsetValues[m.toolsetCursor]
		if m.toolsetChoice == "custom" {
			m.step = stepCustomTools
			return m, nil
		}
		m.step = stepHarnesses
	case "esc", "q":
		m.cancel = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) toolsKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		m.toolCursor = (m.toolCursor - 1 + len(m.toolRows)) % len(m.toolRows)
	case "down", "j":
		m.toolCursor = (m.toolCursor + 1) % len(m.toolRows)
	case " ":
		m.toolRows[m.toolCursor].checked = !m.toolRows[m.toolCursor].checked
	case "a":
		anyUnchecked := false
		for _, row := range m.toolRows {
			if !row.checked {
				anyUnchecked = true
				break
			}
		}
		for index := range m.toolRows {
			m.toolRows[index].checked = anyUnchecked
		}
	case "enter":
		m.step = stepHarnesses
	case "esc":
		// A sub-screen of the toolset question, so esc goes back to it.
		m.step = stepToolset
	case "q":
		m.cancel = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) harnessKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "v":
		m.showAll = !m.showAll
		if !m.onVisibleRow() || !m.harnesses[m.cursor].Selectable() {
			m.cursor = m.firstSelectable()
		}
	case " ":
		m.toggle()
	case "a":
		m.toggleAll()
	case "enter":
		m.step = stepApplying
		return m, tea.Batch(spin(), m.apply())
	case "esc", "q":
		m.cancel = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) inputKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEnter:
		value := strings.TrimSpace(m.input)
		if value == "" {
			return m, nil
		}
		if m.step == stepEmail {
			m.email, m.input, m.message = value, "", ""
			m.step = stepToken
			return m, nil
		}
		m.token, m.input = value, ""
		m.step = stepVerifying
		return m, tea.Batch(spin(), verifyCmd(m.ctx, m.email, m.token))
	case tea.KeyEsc:
		m.step = stepDone
	case tea.KeyBackspace:
		if runes := []rune(m.input); len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		m.input += string(key.Runes)
	}
	return m, nil
}

// adopt takes the detection result and sets up the selection it implies, then
// opens on the toolset question unless a --toolset flag already answered it.
func (m *model) adopt(harnesses []Harness) {
	m.harnesses = harnesses
	m.selected = map[detectharness.ID]bool{}
	for _, harness := range harnesses {
		m.selected[harness.ID] = harness.Configured ||
			(harness.State == detectharness.Detected && harness.Selectable())
	}
	m.cursor = m.firstSelectable()
	switch m.opts.Toolset {
	case mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete:
		m.toolsetChoice = m.opts.Toolset
		m.step = stepHarnesses
	case "custom":
		m.toolsetChoice = "custom"
		m.step = stepCustomTools
	default:
		m.step = stepToolset
		m.toolsetCursor = 1 // open on the recommended Read + Write
	}
}

func (m *model) confirmSignIn(choice int) tea.Cmd {
	if choice != 0 {
		m.step = stepDone
		return nil
	}
	m.step, m.input, m.message = stepEmail, m.email, ""
	return nil
}

// computeEnv builds the env block written into each client config from the
// chosen toolset / tools and (only when explicitly provided) credentials.
func (m *model) computeEnv() map[string]string {
	env := map[string]string{}
	switch m.toolsetChoice {
	case "custom":
		var ids []string
		for _, row := range m.toolRows {
			if row.checked {
				ids = append(ids, row.id)
			}
		}
		if len(ids) == 0 {
			env["FAVRO_TOOLSET"] = mcpserver.TierWrite
		} else {
			env["FAVRO_TOOLS"] = strings.Join(ids, ",")
		}
	case mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete:
		env["FAVRO_TOOLSET"] = m.toolsetChoice
	default:
		env["FAVRO_TOOLSET"] = mcpserver.TierWrite
	}
	if m.embedCreds {
		env["FAVRO_EMAIL"] = m.email
		env["FAVRO_API_TOKEN"] = m.token
	}
	return env
}

// toolsetLabel names the chosen toolset for the summary grid.
func (m *model) toolsetLabel() string {
	if m.toolsetChoice == "custom" {
		count := 0
		for _, row := range m.toolRows {
			if row.checked {
				count++
			}
		}
		if count == 0 {
			return toolsetOptions[1]
		}
		return fmt.Sprintf("Custom (%d tools)", count)
	}
	for index, value := range toolsetValues {
		if value == m.toolsetChoice {
			return toolsetOptions[index]
		}
	}
	return toolsetOptions[1]
}

func (m *model) apply() tea.Cmd {
	env := m.computeEnv()
	dryRun := m.opts.DryRun
	name := m.name
	var present, absent []detectharness.ID
	for _, harness := range m.harnesses {
		if !harness.Selectable() {
			continue
		}
		if m.selected[harness.ID] {
			present = append(present, harness.ID)
		} else if harness.Configured {
			absent = append(absent, harness.ID)
		}
	}
	return func() tea.Msg {
		// The detection installer carries the default env; the one that writes
		// carries the env the user just chose.
		installer, err := NewInstaller(name, env)
		if err != nil {
			installer = m.installer
		}
		if dryRun {
			results := installer.PlanResults(m.ctx, present, detectharness.Present)
			return appliedMsg(append(results,
				installer.PlanResults(m.ctx, absent, detectharness.Absent)...))
		}
		results := installer.Apply(m.ctx, present, detectharness.Present)
		return appliedMsg(append(results, installer.Apply(m.ctx, absent, detectharness.Absent)...))
	}
}

// verifyCmd checks the credentials against the live Favro API off the main
// loop, so the screen shows a spinner instead of freezing.
func verifyCmd(_ context.Context, email, token string) tea.Cmd {
	return func() tea.Msg {
		_, err := favro.NewClient(email, token, "").GetOrganizations()
		return verifiedMsg{err: err}
	}
}

func (m *model) confirmUninstall(choice int) tea.Cmd {
	if choice != 0 {
		m.cancel = true
		return tea.Quit
	}
	m.step = stepRemoving
	registered, dryRun := m.registered, m.opts.DryRun
	return tea.Batch(spin(), func() tea.Msg {
		if dryRun {
			return removedMsg(m.installer.PlanResults(m.ctx, registered, detectharness.Absent))
		}
		return removedMsg(m.installer.Apply(m.ctx, registered, detectharness.Absent))
	})
}

// --- selection helpers ------------------------------------------------------

// visible is which harnesses earn a line: the ones that are here, the ones
// already registered, and anything currently selected. A list of thirteen
// clients, eleven of them "not detected", tells the user nothing they asked
// for.
//
// Selected rows stay on screen even after v hides their group. Letting them
// disappear meant a client revealed with v, selected, and then hidden again was
// still registered - a config file written for software the user could no
// longer see chosen, and could not unpick.
func (m *model) visible() []int {
	var indices []int
	for index, harness := range m.harnesses {
		if harness.State == detectharness.Detected || harness.Configured ||
			m.showAll || m.selected[harness.ID] {
			indices = append(indices, index)
		}
	}
	return indices
}

// firstSelectable is where the cursor opens.
//
// It has to be a row the cursor is actually drawn on: the pointer is only
// rendered for a selectable harness, so starting on an unselectable one opens a
// list with no visible cursor at all.
func (m *model) firstSelectable() int {
	for _, index := range m.visible() {
		if m.harnesses[index].Selectable() {
			return index
		}
	}
	return m.firstVisible()
}

func (m *model) firstVisible() int {
	if indices := m.visible(); len(indices) > 0 {
		return indices[0]
	}
	return 0
}

// onVisibleRow reports whether the cursor is on a row that is actually drawn.
//
// With nothing detected the list is empty, and a cursor of zero still pointed
// at a harness - so space on the "No AI clients detected" screen selected and
// then registered a client that was never on screen.
func (m *model) onVisibleRow() bool {
	for _, index := range m.visible() {
		if index == m.cursor {
			return true
		}
	}
	return false
}

// moveCursor walks the rows a pointer is drawn on.
//
// The pointer is only rendered for a selectable row, so stepping onto one that
// cannot be configured left the list with no cursor visible anywhere.
func (m *model) moveCursor(direction int) {
	var indices []int
	for _, index := range m.visible() {
		if m.harnesses[index].Selectable() {
			indices = append(indices, index)
		}
	}
	if len(indices) == 0 {
		return
	}
	position := 0
	for index, value := range indices {
		if value == m.cursor {
			position = index
		}
	}
	position += direction
	if position < 0 {
		position = len(indices) - 1
	}
	if position >= len(indices) {
		position = 0
	}
	m.cursor = indices[position]
}

func (m *model) toggle() {
	if m.cursor >= len(m.harnesses) || !m.onVisibleRow() {
		return
	}
	harness := m.harnesses[m.cursor]
	if !harness.Selectable() {
		m.message = harness.Name + " could not be inspected, so it cannot be changed."
		return
	}
	m.selected[harness.ID] = !m.selected[harness.ID]
	m.message = ""
	// A row shown only because it was selected leaves the list when it is not.
	// Leaving the cursor on it made the next keypress do nothing at all.
	if !m.onVisibleRow() {
		m.cursor = m.firstSelectable()
	}
}

// toggleAll turns every visible row on, or clears them if they are all on.
//
// Visible, not every harness. Selecting rows hidden behind v wrote real
// configuration files for software the user does not have and cannot see, under
// a message announcing that it had selected the detected ones.
func (m *model) toggleAll() {
	indices := m.visible()
	anyUnselected := false
	for _, index := range indices {
		harness := m.harnesses[index]
		if harness.Selectable() && !m.selected[harness.ID] {
			anyUnselected = true
			break
		}
	}
	for _, index := range indices {
		harness := m.harnesses[index]
		if harness.Selectable() {
			m.selected[harness.ID] = anyUnselected
		}
	}
	selectable := 0
	for _, index := range indices {
		if m.harnesses[index].Selectable() {
			selectable++
		}
	}
	switch {
	case anyUnselected:
		m.message = "Selected every client shown."
	case selectable > 0:
		m.message = "Cleared every client shown."
	default:
		// Nothing on screen could be changed, so nothing is claimed.
		m.message = ""
	}
}

// connected counts the clients this run left registered.
func (m *model) connected() int {
	count := 0
	for _, result := range m.results {
		if result.Desired == detectharness.Present &&
			(result.State == detectharness.Applied || result.State == detectharness.ApplyNoop) {
			count++
		}
	}
	return count
}

// reloadHint is one client and what has to be done to it. The client's name
// stays attached: a bare instruction to restart something, with nothing saying
// what, is not advice anyone can follow.
type reloadHint struct {
	client string
	action string
}

func (m *model) reloadHints() []reloadHint {
	byID := make(map[detectharness.ID]string, len(m.harnesses))
	for _, harness := range m.harnesses {
		byID[harness.ID] = harness.ReloadHint
	}
	var hints []reloadHint
	for _, result := range m.results {
		if result.Desired != detectharness.Present || result.State != detectharness.Applied {
			continue
		}
		if action := byID[result.HarnessID]; action != "" {
			hints = append(hints, reloadHint{client: result.Name, action: action})
		}
	}
	return hints
}

// summarise renders one client's outcome in at most a few words. A dry run
// speaks in the conditional so a preview is never mistaken for a change.
func summarise(result detectharness.Result, enabling, dryRun bool) string {
	switch result.State {
	case detectharness.Applied:
		if enabling {
			// An update rewrote an entry that was already there under this
			// name. Usually that is this program's own older registration, but
			// it can be someone else's server - and silently calling that
			// "registered" is the one case where the user needs the difference.
			if result.Action == "update" {
				if dryRun {
					return "would replace an existing entry"
				}
				return "replaced an existing entry"
			}
			if dryRun {
				return "would register"
			}
			return "registered"
		}
		if dryRun {
			return "would remove"
		}
		return "removed"
	case detectharness.ApplyNoop:
		return "already correct"
	case detectharness.ApplySkipped:
		return "skipped: " + result.Reason
	case detectharness.ApplyConflict:
		return "conflict: another server is registered under this name"
	case detectharness.ApplyFailed:
		return "failed: " + result.Reason
	default:
		return string(result.State)
	}
}

// Run executes configure, install, or uninstall and returns the process exit
// code.
func Run(ctx context.Context, opts Options) int {
	name := opts.Name
	if name == "" {
		name = ServerName
	}
	switch opts.Toolset {
	case "", mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete, "custom":
	default:
		fmt.Fprintf(opts.Stderr,
			"favro-mcp: unknown --toolset %q (use read, write, delete, or custom)\n", opts.Toolset)
		return 1
	}

	installer, err := NewInstaller(name, DefaultEnv())
	if err != nil {
		fmt.Fprintln(opts.Stderr, "favro-mcp:", err)
		return 1
	}
	// No terminal means no questions can be asked, so nothing is asked.
	if opts.Yes || !interactive(opts) {
		return runUnattended(ctx, installer, name, opts)
	}

	state := &model{ctx: ctx, opts: opts, name: name, installer: installer}
	state.embedCreds = opts.Email != "" && opts.Token != ""
	state.email, state.token = opts.Email, opts.Token
	if state.embedCreds || credentials.Exists() {
		state.signedIn = true
		if !state.embedCreds {
			if email, _, err := credentials.Load(); err == nil {
				state.email = email
			}
		}
	}

	// Custom-tool toggles (read+write pre-checked, delete off).
	for _, info := range mcpserver.ToolCatalog() {
		state.toolRows = append(state.toolRows, toolRow{
			id:      info.Name,
			tier:    info.Tier,
			checked: info.Tier == mcpserver.TierRead || info.Tier == mcpserver.TierWrite,
		})
	}

	if opts.Uninstall {
		for _, harness := range installer.Detect(ctx) {
			if harness.Configured {
				state.registered = append(state.registered, harness.ID)
				state.harnesses = append(state.harnesses, harness)
			}
		}
		if len(state.registered) == 0 {
			fmt.Fprintln(opts.Stdout, "  favro-mcp is not registered with any client.")
			return 0
		}
		state.step, state.choice = stepPlan, 1
	}

	program := tea.NewProgram(state, tea.WithContext(ctx),
		tea.WithInput(opts.Stdin), tea.WithOutput(opts.Stdout))
	if _, err := program.Run(); err != nil && !state.settled {
		// Only before the point of no return. A signal arriving while the
		// summary of a finished setup is on screen is not a failed setup, and
		// reporting it as one makes the install script announce that configure
		// did not complete.
		fmt.Fprintln(opts.Stderr, "favro-mcp:", err)
		return 1
	}

	if state.failure != "" {
		fmt.Fprintln(opts.Stderr, "favro-mcp:", state.failure)
		return 1
	}
	if state.cancel && !state.settled {
		fmt.Fprintln(opts.Stdout, "  Cancelled; no changes were made.")
		// Zero, so the installer script does not follow a deliberate cancel
		// with "configure did not complete".
		return 0
	}
	for _, result := range state.results {
		if result.State == detectharness.ApplyFailed || result.State == detectharness.ApplyConflict {
			return 1
		}
	}
	return 0
}
