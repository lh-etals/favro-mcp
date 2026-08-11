// Package app implements the interactive CLI app shell that a bare
// `favro-mcp` invocation opens on a real terminal. The ENTIRE app runs as ONE
// tea.Program in the alternate screen buffer so it owns the full terminal
// window (like lazygit / gh-dash); a single viewport drives all scrolling.
package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/cellbuf"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/install"
)

// isTerminal reports whether stdin AND stdout are interactive TTYs (bubbletea
// inline mode requires both). One implementation for the whole program, owned
// by the install package.
func isTerminal() bool {
	return install.Interactive(os.Stdin, os.Stdout)
}

// RunApp is the interactive CLI app. It builds a single appModel and runs it
// in one tea.Program in the alternate screen buffer so the app owns the full
// terminal window (like lazygit / gh-dash). When the user picks "Configure AI
// clients" the program quits with reconfigure=true; RunApp then launches the
// installer (which runs inline in the regular buffer) and re-opens the app
// afterwards.
func RunApp() {
	if !isTerminal() {
		fmt.Fprintln(os.Stderr, "favro-mcp: not a terminal. Run `favro-mcp mcp` to start the MCP server.")
		return
	}
	m := newAppModel()
	for {
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "  Error:", err)
			return
		}
		if !m.reconfigure {
			return
		}
		// Hand the TTY off to the installer, then resume the app. A non-zero
		// exit already explained itself on the installer's own screen.
		m.reconfigure = false
		install.Run(context.Background(), install.Options{
			Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		})
		// Refresh login state in case the installer wrote new creds.
		m.session, _ = NewSession()
		m.step = stepMenu
		m.err = ""
		m.rebuildMenu()
	}
}

// ===========================================================================
// Unified appModel
// ===========================================================================

// step identifies one screen of the unified appModel state machine.
type step int

const (
	stepMenu step = iota
	stepLoading
	stepLogin
	stepVerifying
	stepSwitchOrg
	stepBoardsPickCollection
	stepBoardsPickBoard
	stepCards
	stepCardPrompt // entering a card identifier (no preselected card)
	stepCardDetail
	stepUsers
	stepTags
	stepScroll // generic scroll fallback (currently used for account info)
)

// appModel owns every screen of the interactive app. All sub-models are
// embedded fields so their state survives step transitions; the active one is
// selected by `step` in Update/View. A single viewport drives all scrollable
// content (lists + detail text); it is resized by WindowSizeMsg.
type appModel struct {
	step         step
	reconfigure  bool // set when user picked Configure; RunApp re-runs installer after exit
	loadingLabel string

	session *Session // nil when not logged in

	// Reusable sub-models.
	selectModel selectModel     // menu, switch-org, collection/board/card pickers
	loginModel  loginModel      // email + token form
	cardPrompt  cardPromptModel // single textinput for card identifier
	scroll      scrollTextModel // card detail / users / tags / account info

	// Full-window viewport (alt-screen). Width/Height are the terminal size;
	// viewport.Height is sized to leave room for the header and footer chrome.
	width    int
	height   int
	viewport viewport.Model
	// lastSyncedStep tracks step transitions so the viewport scrolls back to
	// the top whenever a new screen is entered.
	lastSyncedStep step

	// Data caches filled by tea.Cmds.
	orgs        []favro.Organization
	collections []favro.Collection
	boards      []favro.Widget
	cards       []favro.Card
	page        int
	totalPages  int
	pendingCard *favro.Card // card chosen from the list, awaiting detail load

	// err holds a transient error rendered inside the current step (cleared on
	// the next navigation key).
	err string
}

func newAppModel() *appModel {
	m := &appModel{step: stepMenu}
	m.session, _ = NewSession() // nil if not logged in
	m.viewport = viewport.New(80, 20)
	m.viewport.MouseWheelEnabled = true
	m.rebuildMenu()
	return m
}

// rebuildMenu resets selectModel to show the main menu for the current login
// state.
func (m *appModel) rebuildMenu() {
	loggedIn := m.session != nil
	title := "favro-mcp"
	if loggedIn {
		title = "favro-mcp  " + m.session.email
	}
	m.selectModel = selectModel{
		title:   title,
		options: menuLabels(loggedIn),
		footer:  "up/down move . pgup/pgdn scroll . enter select . q quit",
	}
}

func menuLabels(loggedIn bool) []string {
	items := buildMenu(loggedIn)
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.label
	}
	return out
}

func (m *appModel) Init() tea.Cmd { return nil }

// Update is the public tea.Model entry point. It dispatches to updateInner
// for the real work, then re-syncs the shared viewport so the rendered body
// always reflects the latest model state.
func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.updateInner(msg)
	if a, ok := nm.(*appModel); ok {
		a.syncViewport()
		return a, cmd
	}
	return nm, cmd
}

// updateInner is the step-aware dispatcher. Returns the new model (always a
// *appModel in practice) plus a command.
func (m *appModel) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Ctrl+c always quits immediately, regardless of step.
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// WindowSizeMsg resizes the shared viewport (and is also forwarded to
	// textinput sub-models by the per-step handlers below).
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		m.viewport.Width = ws.Width
		vh := ws.Height - 4 // leave 4 lines for header + footer chrome
		if vh < 3 {
			vh = 3
		}
		m.viewport.Height = vh
	}

	// Async results land here regardless of step.
	switch msg := msg.(type) {
	case orgsLoadedMsg:
		return m.onOrgsLoaded(msg)
	case collectionsLoadedMsg:
		return m.onCollectionsLoaded(msg)
	case boardsLoadedMsg:
		return m.onBoardsLoaded(msg)
	case cardsLoadedMsg:
		return m.onCardsLoaded(msg)
	case usersLoadedMsg:
		return m.onUsersLoaded(msg)
	case tagsLoadedMsg:
		return m.onTagsLoaded(msg)
	case cardResolvedMsg:
		return m.onCardResolved(msg)
	case cardDetailLoadedMsg:
		return m.onCardDetailLoaded(msg)
	case credsVerifiedMsg:
		return m.onCredsVerified(msg)
	}

	// Steps with textinputs need non-key messages (blink, window-size).
	switch m.step {
	case stepLogin:
		return m.updateLogin(msg)
	case stepCardPrompt:
		return m.updateCardPrompt(msg)
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch m.step {
	case stepMenu:
		return m.updateMenu(k)
	case stepSwitchOrg:
		return m.updateSwitchOrg(k)
	case stepBoardsPickCollection:
		return m.updatePickCollection(k)
	case stepBoardsPickBoard:
		return m.updatePickBoard(k)
	case stepCards:
		return m.updateCards(k)
	case stepCardDetail, stepUsers, stepTags, stepScroll:
		return m.updateScroll(k)
	case stepLoading, stepVerifying:
		// Wait for the async result; ignore keys (ctrl+c handled above).
		return m, nil
	}
	return m, nil
}

// isListStep reports whether the current step renders a selectModel whose
// highlighted row (cursor) should auto-scroll into view.
func isListStep(s step) bool {
	switch s {
	case stepMenu, stepSwitchOrg, stepBoardsPickCollection, stepBoardsPickBoard, stepCards:
		return true
	}
	return false
}

// --- menu step --------------------------------------------------------------

func (m *appModel) updateMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key dismisses a stale menu error.
	m.err = ""
	switch k.String() {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "pgdown", "g", "G":
		m.handleViewportKey(k.String())
	case "enter":
		return m.chooseMenu()
	}
	return m, nil
}

func (m *appModel) chooseMenu() (tea.Model, tea.Cmd) {
	items := buildMenu(m.session != nil)
	if m.selectModel.cursor < 0 || m.selectModel.cursor >= len(items) {
		return m, tea.Quit
	}
	switch items[m.selectModel.cursor].choice {
	case menuQuit:
		return m, tea.Quit
	case menuConfigure:
		// Installer has its own TUI; hand the TTY off after quitting ours.
		m.reconfigure = true
		return m, tea.Quit
	case menuLogin:
		m.step = stepLogin
		m.loginModel = newLoginModel("")
		m.loginModel.email.Focus()
		m.loginModel.token.Blur()
		return m, textinput.Blink
	case menuBrowseBoards:
		if !m.requireOrgOrError() {
			return m, nil
		}
		m.step = stepLoading
		m.loadingLabel = "Loading collections..."
		return m, loadCollectionsCmd(m.session)
	case menuListCards:
		if !m.requireOrgOrError() {
			return m, nil
		}
		if m.session.BoardID() == "" {
			m.err = "No board selected. Use Browse boards first."
			return m, nil
		}
		m.step = stepLoading
		m.loadingLabel = "Loading cards..."
		m.page = 0
		return m, loadCardsCmd(m.session, m.session.BoardID(), 0)
	case menuViewCardDetail:
		if !m.requireOrgOrError() {
			return m, nil
		}
		m.step = stepCardPrompt
		m.cardPrompt = newCardPromptModel()
		m.pendingCard = nil
		return m, textinput.Blink
	case menuUsers:
		if !m.requireOrgOrError() {
			return m, nil
		}
		m.step = stepLoading
		m.loadingLabel = "Loading users..."
		return m, loadUsersCmd(m.session)
	case menuTags:
		if !m.requireOrgOrError() {
			return m, nil
		}
		m.step = stepLoading
		m.loadingLabel = "Loading tags..."
		return m, loadTagsCmd(m.session)
	case menuSwitchOrg:
		m.step = stepLoading
		m.loadingLabel = "Loading organizations..."
		return m, loadOrgsCmd(m.session)
	case menuLogout:
		// "Log out" actually shows the active account info.
		text, title := buildAccountInfoText(m.session)
		m.scroll = newScrollTextModel(title, text)
		m.step = stepScroll
		return m, nil
	}
	return m, nil
}

// requireOrgOrError ensures an active organization is set on the session,
// auto-selecting when the account has exactly one. On failure it sets m.err
// (rendered in the menu) and returns false.
func (m *appModel) requireOrgOrError() bool {
	if _, err := m.session.RequireOrg(); err != nil {
		m.err = err.Error()
		return false
	}
	return true
}

func (m *appModel) backToMenu() (tea.Model, tea.Cmd) {
	m.err = ""
	m.step = stepMenu
	m.rebuildMenu()
	return m, nil
}

// --- login step -------------------------------------------------------------

func (m *appModel) updateLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return m.backToMenu()
		case "enter":
			email := strings.TrimSpace(m.loginModel.email.Value())
			token := strings.TrimSpace(m.loginModel.token.Value())
			if email == "" || token == "" {
				m.loginModel.errText = "email and token are required"
				return m, textinput.Blink
			}
			m.loginModel.errText = ""
			m.step = stepVerifying
			return m, verifyCredsCmd(email, token)
		}
	}
	var cmd tea.Cmd
	m.loginModel, cmd = m.loginModel.update(msg)
	return m, cmd
}

// --- card identifier prompt step -------------------------------------------

func (m *appModel) updateCardPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return m.backToMenu()
		case "enter":
			identifier := strings.TrimSpace(m.cardPrompt.input.Value())
			if identifier == "" {
				return m.backToMenu()
			}
			m.step = stepLoading
			m.loadingLabel = "Resolving card..."
			return m, resolveCardCmd(m.session, identifier, m.session.BoardID())
		}
	}
	var cmd tea.Cmd
	m.cardPrompt, cmd = m.cardPrompt.update(msg)
	return m, cmd
}

// --- switch-org step --------------------------------------------------------

func (m *appModel) updateSwitchOrg(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "esc":
		return m.backToMenu()
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "pgdown", "g", "G":
		m.handleViewportKey(k.String())
	case "enter":
		if m.selectModel.cursor >= 0 && m.selectModel.cursor < len(m.orgs) {
			chosen := m.orgs[m.selectModel.cursor]
			m.session.SetOrg(chosen.OrganizationID)
			return m.backToMenu()
		}
		return m.backToMenu()
	}
	return m, nil
}

// --- boards steps -----------------------------------------------------------

func (m *appModel) updatePickCollection(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "esc":
		return m.backToMenu()
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "pgdown", "g", "G":
		m.handleViewportKey(k.String())
	case "enter":
		collectionID := resolveCollectionChoice(m.collections, m.selectModel.cursor)
		m.step = stepLoading
		m.loadingLabel = "Loading boards..."
		return m, loadBoardsCmd(m.session, collectionID)
	}
	return m, nil
}

func (m *appModel) updatePickBoard(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "esc":
		// Up one level: back to the collection picker.
		m.step = stepLoading
		m.loadingLabel = "Loading collections..."
		return m, loadCollectionsCmd(m.session)
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "pgdown", "g", "G":
		m.handleViewportKey(k.String())
	case "enter":
		if m.selectModel.cursor >= 0 && m.selectModel.cursor < len(m.boards) {
			chosen := m.boards[m.selectModel.cursor]
			m.session.SetBoard(chosen.WidgetCommonID)
			return m.backToMenu()
		}
		return m.backToMenu()
	}
	return m, nil
}

// --- cards step -------------------------------------------------------------

func (m *appModel) updateCards(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "esc":
		return m.backToMenu()
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "pgdown", "g", "G":
		m.handleViewportKey(k.String())
	case "n":
		if m.page < m.totalPages-1 {
			m.page++
			m.step = stepLoading
			m.loadingLabel = "Loading cards..."
			return m, loadCardsCmd(m.session, m.session.BoardID(), m.page)
		}
	case "p":
		if m.page > 0 {
			m.page--
			m.step = stepLoading
			m.loadingLabel = "Loading cards..."
			return m, loadCardsCmd(m.session, m.session.BoardID(), m.page)
		}
	case "enter":
		if m.selectModel.cursor >= 0 && m.selectModel.cursor < len(m.cards) {
			c := m.cards[m.selectModel.cursor]
			m.pendingCard = &c
			m.step = stepLoading
			m.loadingLabel = "Loading card detail..."
			return m, loadCardDetailCmd(m.session, c)
		}
	}
	return m, nil
}

// --- scroll step (card detail / users / tags / account) --------------------

func (m *appModel) updateScroll(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "esc":
		// After viewing a card from the list, q returns to the list; from a
		// top-level entry it returns to the menu. Distinguish by pendingCard.
		if m.pendingCard != nil && m.session != nil && m.session.BoardID() != "" {
			// Coming from list-cards detail view -> reload the cards page.
			m.step = stepLoading
			m.loadingLabel = "Loading cards..."
			m.pendingCard = nil
			return m, loadCardsCmd(m.session, m.session.BoardID(), m.page)
		}
		m.pendingCard = nil
		return m.backToMenu()
	case "up", "k", "down", "j", "pgup", "pgdown", "g", "G":
		m.handleViewportKey(k.String())
	}
	return m, nil
}

// handleViewportKey dispatches scroll-only keys to the shared viewport.
func (m *appModel) handleViewportKey(key string) {
	switch key {
	case "up", "k":
		m.viewport.LineUp(1)
	case "down", "j":
		m.viewport.LineDown(1)
	case "pgup":
		m.viewport.PageUp()
	case "pgdown":
		m.viewport.PageDown()
	case "g":
		m.viewport.GotoTop()
	case "G":
		m.viewport.GotoBottom()
	}
}

// --- async result handlers --------------------------------------------------

func (m *appModel) onOrgsLoaded(msg orgsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = "Failed to load organizations: " + msg.err.Error()
		return m.backToMenu()
	}
	m.orgs = msg.orgs
	if len(m.orgs) == 0 {
		m.err = "Your account has no organizations."
		return m.backToMenu()
	}
	if len(m.orgs) == 1 {
		m.session.SetOrg(m.orgs[0].OrganizationID)
		m.err = ""
		return m.backToMenu()
	}
	cur := m.session.OrgID()
	labels := make([]string, len(m.orgs))
	for i, o := range m.orgs {
		label := o.Name
		if o.OrganizationID == cur {
			label += " " + styleSuccess.Render("(active)")
		}
		labels[i] = label
	}
	m.selectModel = selectModel{
		title:   "Switch organization",
		options: labels,
		footer:  "up/down move . enter select . q cancel",
	}
	m.step = stepSwitchOrg
	return m, nil
}

func (m *appModel) onCollectionsLoaded(msg collectionsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = "Failed to load collections: " + msg.err.Error()
		return m.backToMenu()
	}
	m.collections = msg.collections
	if len(m.collections) == 0 {
		// Skip the picker; load boards with no collection filter.
		m.step = stepLoading
		m.loadingLabel = "Loading boards..."
		return m, loadBoardsCmd(m.session, "")
	}
	m.selectModel = selectModel{
		title:   "Browse boards",
		options: collectionSelectOptions(m.collections),
		footer:  "up/down move . enter select . q cancel",
	}
	m.step = stepBoardsPickCollection
	return m, nil
}

func (m *appModel) onBoardsLoaded(msg boardsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = "Failed to load boards: " + msg.err.Error()
		return m.backToMenu()
	}
	m.boards = msg.boards
	if len(m.boards) == 0 {
		m.err = "No boards found."
		return m.backToMenu()
	}
	m.selectModel = selectModel{
		title:   "Browse boards  pick a board",
		options: boardSelectOptions(m.boards),
		footer:  "up/down move . enter select . q cancel",
	}
	m.step = stepBoardsPickBoard
	return m, nil
}

func (m *appModel) onCardsLoaded(msg cardsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = "Failed to load cards: " + msg.err.Error()
		return m.backToMenu()
	}
	if len(msg.cards) == 0 {
		m.err = "No cards on this board."
		return m.backToMenu()
	}
	m.cards = msg.cards
	m.totalPages = msg.total
	title := fmt.Sprintf("List cards  page %d/%d", m.page+1, m.totalPages)
	footer := "up/down move . enter detail . n next . p prev . q back"
	if m.totalPages <= 1 {
		footer = "up/down move . enter detail . q back"
	}
	m.selectModel = selectModel{
		title:   title,
		options: cardListOptions(m.cards),
		footer:  footer,
	}
	m.step = stepCards
	return m, nil
}

func (m *appModel) onUsersLoaded(msg usersLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = "Failed to load users: " + msg.err.Error()
		return m.backToMenu()
	}
	if len(msg.users) == 0 {
		m.err = "No users found."
		return m.backToMenu()
	}
	m.scroll = newScrollTextModel(
		fmt.Sprintf("Users  %d", len(msg.users)),
		usersTableText(msg.users),
	)
	m.step = stepUsers
	return m, nil
}

func (m *appModel) onTagsLoaded(msg tagsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = "Failed to load tags: " + msg.err.Error()
		return m.backToMenu()
	}
	if len(msg.tags) == 0 {
		m.err = "No tags found."
		return m.backToMenu()
	}
	m.scroll = newScrollTextModel(
		fmt.Sprintf("Tags  %d", len(msg.tags)),
		tagsTableText(msg.tags),
	)
	m.step = stepTags
	return m, nil
}

func (m *appModel) onCardResolved(msg cardResolvedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err.Error()
		// Back to the prompt for another attempt.
		m.step = stepCardPrompt
		return m, textinput.Blink
	}
	m.pendingCard = msg.card
	m.step = stepLoading
	m.loadingLabel = "Loading card detail..."
	return m, loadCardDetailCmd(m.session, *msg.card)
}

func (m *appModel) onCardDetailLoaded(msg cardDetailLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err.Error()
		if m.pendingCard == nil {
			// Came from the prompt path.
			m.step = stepCardPrompt
			return m, textinput.Blink
		}
		return m.backToMenu()
	}
	m.scroll = newScrollTextModel(msg.title, msg.text)
	m.step = stepCardDetail
	return m, nil
}

func (m *appModel) onCredsVerified(msg credsVerifiedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.loginModel.errText = fmt.Sprintf("verification failed: %v", msg.err)
		m.loginModel.token.SetValue("")
		m.step = stepLogin
		m.loginModel.email.Focus()
		m.loginModel.token.Blur()
		return m, textinput.Blink
	}
	email := strings.TrimSpace(m.loginModel.email.Value())
	token := strings.TrimSpace(m.loginModel.token.Value())
	if err := credentials.Save(email, token); err != nil {
		m.err = "Failed to save credentials: " + err.Error()
		return m.backToMenu()
	}
	m.session, _ = NewSession()
	return m.backToMenu()
}

// ===========================================================================
// View
// ===========================================================================

func (m *appModel) View() string {
	// Wait for the first WindowSizeMsg before painting so the viewport is
	// sized correctly for the alt-screen.
	if m.height == 0 {
		return ""
	}

	var middle string
	switch m.step {
	case stepLogin:
		// Forms render their own content (no viewport scrolling); still
		// framed by the shared header/footer for a consistent chrome.
		middle = m.loginModel.View()
	case stepCardPrompt:
		middle = m.cardPrompt.View()
	default:
		middle = m.viewport.View()
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(middle)
	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

// renderHeader returns the single styled title line shown above the viewport.
func (m *appModel) renderHeader() string {
	return styleTitle.Render(m.headerText())
}

func (m *appModel) headerText() string {
	const app = "Favro MCP"
	screen := m.screenName()
	if screen == "" {
		return app
	}
	return app + "  " + screen
}

// screenName returns the per-step label appended to the app name in the header.
func (m *appModel) screenName() string {
	switch m.step {
	case stepMenu:
		if m.session != nil {
			return "Menu  " + m.session.email
		}
		return "Menu"
	case stepSwitchOrg:
		return "Switch organization"
	case stepBoardsPickCollection:
		return "Browse boards"
	case stepBoardsPickBoard:
		return "Pick a board"
	case stepCards:
		if m.totalPages > 0 {
			return fmt.Sprintf("List cards  page %d/%d", m.page+1, m.totalPages)
		}
		return "List cards"
	case stepCardDetail, stepUsers, stepTags, stepScroll:
		return m.scroll.title
	case stepLoading:
		return ""
	case stepVerifying:
		return "Log in"
	case stepLogin:
		return "Log in"
	case stepCardPrompt:
		return "View card detail"
	}
	return ""
}

// renderFooter returns the single styled key-hint line below the viewport.
func (m *appModel) renderFooter() string {
	return styleFooter.Render(m.footerText())
}

func (m *appModel) footerText() string {
	switch m.step {
	case stepMenu:
		return m.selectModel.footer
	case stepSwitchOrg, stepBoardsPickCollection, stepBoardsPickBoard:
		return "up/down move . pgup/pgdn scroll . enter select . q cancel"
	case stepCards:
		if m.totalPages > 1 {
			return "up/down move . pgup/pgdn scroll . enter detail . n next . p prev . q back"
		}
		return "up/down move . pgup/pgdn scroll . enter detail . q back"
	case stepCardDetail, stepUsers, stepTags, stepScroll:
		return "up/down scroll . pgup/pgdn . g top . G bottom . q back"
	case stepLoading:
		return "loading... (ctrl+c to cancel)"
	case stepVerifying:
		return ""
	case stepLogin:
		return "tab switch fields . enter submit . esc cancel"
	case stepCardPrompt:
		return "enter submit . esc cancel"
	}
	return ""
}

// bodyString returns the inner content the viewport (or form area) should show
// for the current step. For list and scroll steps this is fed to the viewport.
func (m *appModel) bodyString() string {
	switch m.step {
	case stepMenu:
		body := m.selectModel.View()
		if m.err != "" {
			body += "\n" + styleError.Render(m.err) + "\n"
		}
		return body
	case stepSwitchOrg, stepBoardsPickCollection, stepBoardsPickBoard, stepCards:
		return m.selectModel.View()
	case stepCardDetail, stepUsers, stepTags, stepScroll:
		return m.scroll.content
	case stepLoading:
		return m.loadingLabel
	case stepVerifying:
		return "Verifying credentials..."
	}
	return ""
}

// syncViewport feeds the current screen body into the shared viewport.
// Called by Update after every state change. When the step changes the
// viewport is reset to the top so lists do not open scrolled to the bottom
// of the previous screen. NOTE: this does not auto-follow the cursor; that
// only happens in moveCursor so explicit pgup/pgdown scrolls are respected.
func (m *appModel) syncViewport() {
	if m.height == 0 {
		return
	}
	body := m.bodyString()
	// Wrap long lines (e.g. card detail paragraphs) to the viewport width so
	// nothing is clipped at the right edge.
	m.viewport.SetContent(cellbuf.Wrap(body, m.viewport.Width, ""))
	if m.lastSyncedStep != m.step {
		m.viewport.GotoTop()
		m.lastSyncedStep = m.step
	}
}

// moveCursor adjusts the selectModel cursor by delta (clamped to the option
// range) and scrolls the viewport so the highlighted row stays visible. Used
// by all list steps for up/down/j/k keys.
func (m *appModel) moveCursor(delta int) {
	n := len(m.selectModel.options)
	if n == 0 {
		return
	}
	m.selectModel.cursor += delta
	if m.selectModel.cursor < 0 {
		m.selectModel.cursor = 0
	}
	if m.selectModel.cursor > n-1 {
		m.selectModel.cursor = n - 1
	}
	m.followCursor()
}

// followCursor scrolls the viewport to keep the highlighted list row on
// screen. Each selectModel option currently occupies exactly one rendered
// line, so cursor N maps to viewport line N.
func (m *appModel) followCursor() {
	if !isListStep(m.step) {
		return
	}
	cursor := m.selectModel.cursor
	top := m.viewport.YOffset
	bottom := m.viewport.YOffset + m.viewport.Height - 1
	switch {
	case cursor < top:
		m.viewport.SetYOffset(cursor)
	case cursor > bottom:
		m.viewport.SetYOffset(cursor - m.viewport.Height + 1)
	}
}

// ===========================================================================
// Menu definition
// ===========================================================================

// menuChoice identifies a menu entry's behavior. Replaces the old func()
// actions so the model can transition between steps itself (no tea.Program
// re-creation).
type menuChoice int

const (
	menuQuit menuChoice = iota
	menuLogin
	menuConfigure
	menuBrowseBoards
	menuListCards
	menuViewCardDetail
	menuUsers
	menuTags
	menuSwitchOrg
	menuLogout
)

// menuItem pairs a menu label with the choice that fires when it is selected.
type menuItem struct {
	label  string
	choice menuChoice
}

// buildMenu returns the menu entries for the current login state.
func buildMenu(loggedIn bool) []menuItem {
	if !loggedIn {
		return []menuItem{
			{label: "Log in to Favro", choice: menuLogin},
			{label: "Configure AI clients", choice: menuConfigure},
			{label: "Quit", choice: menuQuit},
		}
	}
	return []menuItem{
		{label: "Browse boards", choice: menuBrowseBoards},
		{label: "List cards", choice: menuListCards},
		{label: "View card detail", choice: menuViewCardDetail},
		{label: "Users", choice: menuUsers},
		{label: "Tags", choice: menuTags},
		{label: "Switch organization", choice: menuSwitchOrg},
		{label: "Configure AI clients", choice: menuConfigure},
		{label: "Log out", choice: menuLogout},
		{label: "Quit", choice: menuQuit},
	}
}

// buildAccountInfoText renders the active account / org block shown by the
// "Log out" entry (which actually just displays the account).
func buildAccountInfoText(s *Session) (string, string) {
	var b strings.Builder
	b.WriteString("Logged in as: " + s.email + "\n")
	if id := s.OrgID(); id != "" {
		b.WriteString("Active org:   " + id + "\n")
	} else {
		b.WriteString("Active org:   (none selected)\n")
	}
	b.WriteString("\nRun `favro-mcp login` to switch accounts.\n")
	return b.String(), "favro-mcp  " + s.email
}

// ===========================================================================
// Async tea.Cmds and their result messages
// ===========================================================================

// Each loader snapshots the session fields it needs before going off the
// main loop, so the goroutine never races with the model mutating session
// state on the main loop.

type orgsLoadedMsg struct {
	orgs []favro.Organization
	err  error
}

func loadOrgsCmd(s *Session) tea.Cmd {
	client := s.Client()
	return func() tea.Msg {
		orgs, err := client.GetOrganizations()
		return orgsLoadedMsg{orgs: orgs, err: err}
	}
}

type collectionsLoadedMsg struct {
	collections []favro.Collection
	err         error
}

func loadCollectionsCmd(s *Session) tea.Cmd {
	client := s.Client()
	return func() tea.Msg {
		cols, err := client.GetCollections(false)
		return collectionsLoadedMsg{collections: cols, err: err}
	}
}

type boardsLoadedMsg struct {
	boards []favro.Widget
	err    error
}

func loadBoardsCmd(s *Session, collectionID string) tea.Cmd {
	client := s.Client()
	return func() tea.Msg {
		boards, err := client.GetWidgets(collectionID, false)
		return boardsLoadedMsg{boards: boards, err: err}
	}
}

type cardsLoadedMsg struct {
	cards []favro.Card
	total int
	err   error
}

func loadCardsCmd(s *Session, boardID string, page int) tea.Cmd {
	client := s.Client()
	filter := favro.CardFilter{WidgetCommonID: boardID, Unique: true}
	return func() tea.Msg {
		result, err := client.GetCardsPage(filter, page, "")
		return cardsLoadedMsg{cards: result.Cards, total: result.Pages, err: err}
	}
}

type usersLoadedMsg struct {
	users []favro.User
	err   error
}

func loadUsersCmd(s *Session) tea.Cmd {
	client := s.Client()
	return func() tea.Msg {
		users, err := client.GetUsers()
		return usersLoadedMsg{users: users, err: err}
	}
}

type tagsLoadedMsg struct {
	tags []favro.Tag
	err  error
}

func loadTagsCmd(s *Session) tea.Cmd {
	client := s.Client()
	return func() tea.Msg {
		tags, err := client.GetTags()
		return tagsLoadedMsg{tags: tags, err: err}
	}
}

type cardResolvedMsg struct {
	card *favro.Card
	err  error
}

func resolveCardCmd(s *Session, identifier, boardID string) tea.Cmd {
	r := s.Resolver()
	return func() tea.Msg {
		c, err := r.Card(identifier, boardID)
		if err != nil {
			return cardResolvedMsg{err: err}
		}
		return cardResolvedMsg{card: c}
	}
}

type cardDetailLoadedMsg struct {
	text  string
	title string
	err   error
}

func loadCardDetailCmd(s *Session, c favro.Card) tea.Cmd {
	client := s.Client()
	seq := c.SequentialID
	return func() tea.Msg {
		text, err := buildCardDetailText(client, c)
		if err != nil {
			return cardDetailLoadedMsg{err: err}
		}
		return cardDetailLoadedMsg{
			text:  text,
			title: fmt.Sprintf("Card detail  #%d", seq),
		}
	}
}

type credsVerifiedMsg struct{ err error }

func verifyCredsCmd(email, token string) tea.Cmd {
	return func() tea.Msg {
		_, err := favro.NewClient(email, token, "").GetOrganizations()
		return credsVerifiedMsg{err: err}
	}
}

// ===========================================================================
// selectModel: a plain arrow-key single-choice list (no Init/Update of its
// own; the appModel drives it directly).
// ===========================================================================

type selectModel struct {
	title   string
	options []string
	footer  string
	cursor  int
}

// View renders the option lines only (no title/footer chrome; those live on
// appModel so they can frame the viewport). Each option is exactly one line
// so the appModel can map cursor N to viewport line N for auto-scrolling.
func (m selectModel) View() string {
	if len(m.options) == 0 {
		return ""
	}
	var b strings.Builder
	for i, o := range m.options {
		cursor := " "
		if i == m.cursor {
			cursor = styleCursor.Render(">")
		}
		dot := " "
		if i == m.cursor {
			dot = styleSuccess.Render("●")
		}
		fmt.Fprintf(&b, " %s %s %s\n", cursor, dot, o)
	}
	return b.String()
}

// ===========================================================================
// loginModel: email + token form. Owned by appModel; appModel intercepts
// submit/cancel and otherwise forwards messages here for typing + blink.
// ===========================================================================

type loginModel struct {
	email   textinput.Model
	token   textinput.Model
	focused int // 0 = email, 1 = token
	errText string
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

// update forwards a message to the focused textinput (for typing and the
// cursor-blink animation) and handles Tab to switch focus.
func (m loginModel) update(msg tea.Msg) (loginModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
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
	b.WriteString("\n\nCredentials are verified then saved.\n\n")
	b.WriteString(m.email.View() + "\n\n")
	b.WriteString(m.token.View() + "\n\n")
	if m.errText != "" {
		b.WriteString(styleError.Render(m.errText) + "\n\n")
	}
	b.WriteString(styleFooter.Render("tab switch fields . enter submit . esc cancel"))
	return b.String()
}
