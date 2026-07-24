package app

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/x/cellbuf"
	"golang.org/x/term"
)

// scrollTextModel renders a block of (possibly long) read-only text inside a
// scrollable viewport. Used by the card-detail, users, and tags screens. q/esc
// returns to the caller.
type scrollTextModel struct {
	title   string
	content string
	vp      viewport.Model
	cancel  bool
}

func newScrollTextModel(title, content string) scrollTextModel {
	w, h, _ := term.GetSize(int(os.Stdout.Fd()))
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	height := h - 4
	if height < 3 {
		height = 3
	}
	m := scrollTextModel{title: title, content: content}
	m.vp = viewport.New(w, height)
	m = m.applyContent(w)
	return m
}

// applyContent wraps the stored content to the given width and feeds it to the
// viewport so long lines are not clipped at the right edge.
func (m scrollTextModel) applyContent(width int) scrollTextModel {
	m.vp.SetContent(cellbuf.Wrap(m.content, width, ""))
	return m
}

func (m scrollTextModel) Init() tea.Cmd { return nil }

func (m scrollTextModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		h := msg.Height - 4
		if h < 3 {
			h = 3
		}
		m.vp.Height = h
		m = m.applyContent(msg.Width)
		return m, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c", "q", "esc":
			m.cancel = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m scrollTextModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.title))
	b.WriteString("\n\n")
	b.WriteString(m.vp.View())
	b.WriteString("\n")
	b.WriteString(styleFooter.Render("up/down scroll . q back"))
	return b.String()
}

// padRight left-justifies s in a field of the given width (byte width, like the
// rest of the app which assumes mostly-ASCII values).
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
