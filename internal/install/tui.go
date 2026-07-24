package install

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/lh-etals/favro-mcp/internal/mcpserver"
)

// ErrCancelled is returned when the user aborts an interactive prompt.
var ErrCancelled = fmt.Errorf("cancelled")

// Styling ---------------------------------------------------------------------

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	subtleStyle = lipgloss.NewStyle().Faint(true)
)

// --- toolset select ----------------------------------------------------------

func runToolsetHuh() (string, error) {
	var choice string
	opts := []huh.Option[string]{
		huh.NewOption("Read-only", mcpserver.TierRead),
		huh.NewOption("Read + Write (recommended)", mcpserver.TierWrite),
		huh.NewOption("Read + Write + Delete", mcpserver.TierDelete),
		huh.NewOption("Custom (toggle each tool)", "custom"),
	}
	form := huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().
			Title("Toolset").
			Description("Which tools should the server expose?").
			Options(opts...).
			Value(&choice)),
	)
	if err := runForm(form); err != nil {
		return "", err
	}
	return choice, nil
}

// --- client multi-select -----------------------------------------------------

func runClientsHuh(detected, others []ClientDef) ([]string, error) {
	type row struct {
		value   string
		selected bool
	}
	rows := make([]row, 0, len(detected)+len(others))
	for _, c := range detected {
		rows = append(rows, row{value: c.ID, selected: true})
	}
	for _, c := range others {
		rows = append(rows, row{value: c.ID, selected: false})
	}
	opts := make([]huh.Option[string], 0, len(detected)+len(others))
	idx := 0
	idToName := map[string]string{}
	for _, c := range detected {
		idToName[c.ID] = c.Name + " (detected)"
		huhOpt := huh.NewOption(c.Name+" (detected)", c.ID)
		huhOpt = huhOpt.Selected(true)
		opts = append(opts, huhOpt)
		idx++
	}
	_ = idx
	for _, c := range others {
		idToName[c.ID] = c.Name
		opts = append(opts, huh.NewOption(c.Name, c.ID))
	}
	var selected []string
	form := huh.NewForm(
		huh.NewGroup(huh.NewMultiSelect[string]().
			Title("AI clients").
			Description("Select which clients to register with. Detected clients are pre-selected.").
			Options(opts...).
			Value(&selected)),
	)
	if err := runForm(form); err != nil {
		return nil, err
	}
	return selected, nil
}

// --- custom tools toggle -----------------------------------------------------

func runToolsHuh(catalog []mcpserver.ToolInfo) ([]string, error) {
	opts := make([]huh.Option[string], 0, len(catalog))
	for _, t := range catalog {
		label := fmt.Sprintf("%s (%s)", t.Name, t.Tier)
		opt := huh.NewOption(label, t.Name)
		if t.Tier == mcpserver.TierRead || t.Tier == mcpserver.TierWrite {
			opt = opt.Selected(true)
		}
		opts = append(opts, opt)
	}
	var selected []string
	form := huh.NewForm(
		huh.NewGroup(huh.NewMultiSelect[string]().
			Title("Tools").
			Description("Toggle individual tools on/off. Read+Write pre-selected.").
			Options(opts...).
			Value(&selected)),
	)
	if err := runForm(form); err != nil {
		return nil, err
	}
	return selected, nil
}

// --- login inputs ------------------------------------------------------------

func runLoginHuh(prefillEmail string) (email, token string, err error) {
	email = prefillEmail
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Favro email").
				Value(&email).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("email is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Favro API token").
				EchoMode(huh.EchoModePassword).
				Value(&token).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("token is required")
					}
					return nil
				}),
		),
	)
	if e := runForm(form); e != nil {
		return "", "", e
	}
	return email, token, nil
}

// --- helpers -----------------------------------------------------------------

// runForm runs a huh form with proper TTY detection. On non-TTY, returns
// ErrCancelled (the caller should fall back to flag/env values or print a hint).
func runForm(form *huh.Form) error {
	if !isTTY() {
		return ErrCancelled
	}
	return form.Run()
}

func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// --- interactive app (bare invocation) ---------------------------------------

// RunApp is the interactive CLI app launched by bare `favro-mcp`. It shows a
// main menu with navigation and configuration options.
func RunApp() {
	for {
		var action string
		form := huh.NewForm(
			huh.NewGroup(huh.NewSelect[string]().
				Title("favro-mcp").
				Description("What would you like to do?").
				Options(
					huh.NewOption("Configure AI clients", "configure"),
					huh.NewOption("Log in to Favro", "login"),
					huh.NewOption("Quit", "quit"),
				).
				Value(&action)),
		)
		if err := form.Run(); err != nil {
			return // user cancelled or pipe closed
		}
		switch action {
		case "quit":
			return
		case "configure":
			if err := RunInstall(Options{}); err != nil && err != ErrCancelled {
				fmt.Fprintln(os.Stderr, err)
			}
		case "login":
			if err := interactiveLogin(""); err != nil && err != ErrCancelled {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}
}
