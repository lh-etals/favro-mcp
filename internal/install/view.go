package install

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	detectharness "github.com/sairaph/detect-harness"
)

// Every screen is header, one section line, the content, and a footer. Keeping
// that shape is what makes the steps read as one program rather than a sequence
// of unrelated questions.

func (m *model) View() string {
	switch m.step {
	case stepDetecting:
		return section(header(), "") + "  " + m.spinner() + " Initialising, looking for AI clients...\n"
	case stepToolset:
		return m.viewToolset()
	case stepCustomTools:
		return m.viewTools()
	case stepHarnesses:
		return m.viewHarnesses()
	case stepApplying:
		return section(header(), "") + "  " + m.spinner() + " Registering...\n"
	case stepLogin:
		return m.viewLogin()
	case stepEmail, stepToken:
		return m.viewInput()
	case stepVerifying:
		return section(header(), "") + "  " + m.spinner() + " Verifying credentials with Favro...\n"
	case stepPlan:
		return m.viewPlan()
	case stepRemoving:
		return section(uninstallHeader(), "") + "  " + m.spinner() + " Removing...\n"
	}
	if m.step == stepDone && m.removed {
		return m.viewRemoved()
	}
	return m.viewDone()
}

// section writes a header and, when there is one, the line naming what this
// screen is for. One shape helper, so no screen hand-rolls its own spacing.
func section(head, title string) string {
	if title == "" {
		return head + "\n\n"
	}
	return head + "\n\n  " + title + "\n\n"
}

// choices renders a short action list with the cursor on one of them.
//
// A cursor and nothing else: the filled dot means "this is the stored value" in
// a settings list, and putting one beside Cancel says the opposite of what the
// row does.
func choices(cursor int, options ...string) string {
	var out strings.Builder
	for index, option := range options {
		pointer := " "
		if index == cursor {
			pointer = styleCursor.Render(">")
		}
		fmt.Fprintf(&out, " %s %s\n", pointer, option)
	}
	return out.String()
}

func footer(hints string) string {
	return "\n" + styleFooter.Render("  "+hints)
}

// shortPath writes a path the way a person would say it. An absolute path under
// the home directory is most of a line of noise before the part that matters.
func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	return strings.ReplaceAll(path, home+string(filepath.Separator),
		"~"+string(filepath.Separator))
}

// pathHint returns the export line to add when the installed command will not
// resolve from a new shell.
//
// The startup files are consulted rather than the process PATH: the install
// script exports the directory for its own child processes, so this process
// can always find the binary even when no future shell will. $HOME rather than
// a tilde, because the line is meant to be pasted and a tilde inside double
// quotes is not a home directory.
func pathHint() string {
	if runtime.GOOS == "windows" {
		// The Windows installer writes the persisted user PATH itself and tells
		// the user to open a new window; there is no startup file to check.
		return ""
	}
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	directory := filepath.Dir(executable)
	if directory != filepath.Join(home, ".favro-mcp", "bin") {
		// Run from a build tree or a hand-picked location; PATH advice about
		// the managed install directory would be wrong either way.
		return ""
	}
	for _, name := range []string{".zshrc", ".bashrc", ".profile", ".bash_profile"} {
		if data, err := os.ReadFile(filepath.Join(home, name)); err == nil &&
			strings.Contains(string(data), directory) {
			return ""
		}
	}
	return `export PATH="$HOME/.favro-mcp/bin:$PATH"`
}

func (m *model) viewToolset() string {
	var out strings.Builder
	out.WriteString(section(header(), "Toolset — which tools should the server expose?"))
	for index, option := range toolsetOptions {
		pointer := " "
		if index == m.toolsetCursor {
			pointer = styleCursor.Render(">")
		}
		label := option
		if index == 1 {
			label += styleHint.Render("  (recommended)")
		}
		fmt.Fprintf(&out, " %s %s\n", pointer, label)
	}
	return out.String() + footer("↑↓ move · enter select · q cancel")
}

func (m *model) viewTools() string {
	var out strings.Builder
	out.WriteString(section(header(), "Custom tools — toggle each tool the server may use."))
	for index, row := range m.toolRows {
		pointer := " "
		if index == m.toolCursor {
			pointer = styleCursor.Render(">")
		}
		mark := styleOff.Render("○")
		if row.checked {
			mark = styleOn.Render("●")
		}
		line := fmt.Sprintf("%-26s %s", row.id, styleDim.Render("("+row.tier+")"))
		fmt.Fprintf(&out, " %s %s %s\n", pointer, mark, line)
	}
	return out.String() + footer("↑↓ move · space toggle · a all/none · enter continue · esc back")
}

func (m *model) viewHarnesses() string {
	var out strings.Builder
	out.WriteString(section(header(), "AI clients — which should be able to use Favro?"))

	indices := m.visible()
	if len(indices) == 0 {
		out.WriteString(styleDim.Render(
			"  No AI clients detected. Install one, then run\n  `favro-mcp configure`.\n"))
	}
	for _, index := range indices {
		harness := m.harnesses[index]
		cursor := " "
		if index == m.cursor && harness.Selectable() {
			cursor = styleCursor.Render(">")
		}
		mark := styleOff.Render("○")
		if !harness.Selectable() {
			mark = styleDim.Render("·")
		} else if m.selected[harness.ID] {
			mark = styleOn.Render("●")
		}
		line := fmt.Sprintf("%-22s %s", harness.Name, styleDim.Render(harness.StatusText()))
		if !harness.Selectable() {
			line = styleDim.Render(fmt.Sprintf("%-22s ", harness.Name)) +
				styleHint.Render(harness.StatusText())
		}
		fmt.Fprintf(&out, " %s %s %s\n", cursor, mark, line)
	}

	hidden := len(m.harnesses) - len(indices)
	if hidden > 0 && !m.showAll {
		out.WriteString("\n" + styleDim.Render(
			fmt.Sprintf("  press v to show %d client(s) that are not installed", hidden)))
	} else if m.showAll {
		out.WriteString("\n" + styleDim.Render("  press v to hide clients that are not installed"))
	}
	if m.message != "" {
		out.WriteString("\n\n  " + m.message)
	}
	return out.String() + "\n" + footer(
		"↑↓ move · space toggle · a all/none · v show all · enter continue · q cancel")
}

func (m *model) viewLogin() string {
	var out strings.Builder
	out.WriteString(section(header(), "Favro account — sign in now so the server can authenticate?"))
	out.WriteString(choices(m.choice, "Sign in", "Skip for now"))
	if m.message != "" {
		out.WriteString("\n" + styleError.Render("  "+m.message) + "\n")
	}
	return out.String() + footer("↑↓ move · enter select · esc skip")
}

func (m *model) viewInput() string {
	title := "Favro account — your email address"
	if m.step == stepToken {
		title = "Favro account — your API token"
	}
	var out strings.Builder
	out.WriteString(section(header(), title))
	if m.step == stepToken {
		out.WriteString(styleDim.Render("  Create one in Favro under My profile > API tokens.") + "\n\n")
		fmt.Fprintf(&out, "  %s_\n", strings.Repeat("*", len([]rune(m.input))))
	} else {
		fmt.Fprintf(&out, "  %s_\n", m.input)
	}
	if m.message != "" {
		out.WriteString("\n" + styleError.Render("  "+m.message) + "\n")
	}
	return out.String() + footer("enter confirm · esc skip")
}

func (m *model) viewDone() string {
	var out strings.Builder
	out.WriteString(section(header(), ""))

	if m.opts.DryRun {
		out.WriteString(styleDim.Render("  Dry run — no files were changed.") + "\n\n")
	}
	fmt.Fprintf(&out, "  %-22s %d connected\n", "AI clients", m.connected())
	if m.signedIn && m.email != "" {
		fmt.Fprintf(&out, "  %-22s %s\n", "Favro account", m.email)
	} else if m.signedIn {
		fmt.Fprintf(&out, "  %-22s %s\n", "Favro account", "signed in")
	} else {
		fmt.Fprintf(&out, "  %-22s %s\n", "Favro account", styleHint.Render("not signed in"))
	}
	fmt.Fprintf(&out, "  %-22s %s\n", "Toolset", m.toolsetLabel())

	// Every client, not just the failures. A registration removed at the user's
	// request produced no line at all, so the one outcome they had just chosen
	// was the one they could not see.
	if len(m.results) > 0 {
		out.WriteString("\n")
	}
	for _, result := range m.results {
		enabling := result.Desired == detectharness.Present
		summary := summarise(result, enabling, m.opts.DryRun)
		if result.State == detectharness.ApplyFailed || result.State == detectharness.ApplyConflict {
			summary = styleError.Render(summary)
		}
		fmt.Fprintf(&out, "  %-22s %s\n", result.Name, summary)
	}

	if hints := m.reloadHints(); len(hints) > 0 {
		out.WriteString("\n  Restart the affected clients so they pick up the change:\n")
		for _, hint := range hints {
			fmt.Fprintf(&out, "  %-22s %s\n", hint.client, styleDim.Render(hint.action))
		}
	}

	if m.unreachable != "" {
		out.WriteString("\n" + styleHint.Render("  favro-mcp is not on your PATH yet.") + "\n")
		out.WriteString(styleDim.Render("  Add this to your shell profile, or open a new terminal:\n    "+
			m.unreachable) + "\n")
	}

	next := "  Run `favro-mcp` to browse your boards,"
	if !m.signedIn {
		next = "  Run `favro-mcp login` to sign in,"
	}
	out.WriteString("\n" + styleDim.Render(next+
		"\n  or `favro-mcp configure` to change any of this later."))
	return out.String() + "\n" + footer("enter to finish")
}

// --- uninstall ---------------------------------------------------------------

func (m *model) viewPlan() string {
	var out strings.Builder
	out.WriteString(section(uninstallHeader(), "This removes:"))

	names := make([]string, 0, len(m.harnesses))
	for _, harness := range m.harnesses {
		names = append(names, harness.Name)
	}
	fmt.Fprintf(&out, "  %-22s %s\n", "AI clients", styleDim.Render(strings.Join(names, ", ")))
	out.WriteString("\n")
	out.WriteString(choices(m.choice, "Remove everything", "Cancel"))
	return out.String() + footer("↑↓ move · enter select · esc cancel")
}

func (m *model) viewRemoved() string {
	var out strings.Builder
	out.WriteString(section(uninstallHeader(), ""))

	if m.opts.DryRun {
		out.WriteString(styleDim.Render("  Dry run — no files were changed.") + "\n\n")
	}
	for _, result := range m.results {
		summary := summarise(result, false, m.opts.DryRun)
		if result.State == detectharness.ApplyFailed || result.State == detectharness.ApplyConflict {
			summary = styleError.Render(summary)
		}
		fmt.Fprintf(&out, "  %-22s %s\n", result.Name, summary)
	}
	out.WriteString("\n" + styleDim.Render(
		"  The binary and `~/.favro-mcp` are untouched; delete them yourself\n"+
			"  if you want them gone."))
	return out.String() + "\n" + footer("enter to finish")
}
