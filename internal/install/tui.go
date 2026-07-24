package install

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

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

// isTTY reports whether stdin is an interactive terminal. Prompt functions use
// this to bail out (returning ErrCancelled) instead of hanging when fed by a
// pipe, redirected file, or an MCP client's transport.
func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// promptSelect prints a numbered single-choice menu. Entering "0" or "q"
// cancels. Blank input selects defaultIdx. Returns (idx, ErrCancelled) on an
// explicit cancel.
func promptSelect(title string, options []string, defaultIdx int) (int, error) {
	fmt.Printf("\n  %s\n\n", title)
	for i, o := range options {
		marker := ""
		if i == defaultIdx {
			marker = " (recommended)"
		}
		fmt.Printf("    %d. %s%s\n", i+1, o, marker)
	}
	fmt.Printf("    0. Cancel\n")
	fmt.Printf("\n  Choose [0-%d] (default %d): ", len(options), defaultIdx+1)
	line := readLine()
	if line == "" {
		return defaultIdx, nil
	}
	if line == "q" || line == "0" {
		return 0, ErrCancelled
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(options) {
		fmt.Printf("  Invalid choice; using default.\n")
		return defaultIdx, nil
	}
	return n - 1, nil
}

// --- toolset select ----------------------------------------------------------

func promptToolset() (string, error) {
	if !isTTY() {
		return "", ErrCancelled
	}
	opts := []string{
		"Read-only",
		"Read + Write",
		"Read + Write + Delete",
		"Custom (toggle each tool)",
	}
	vals := []string{mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete, "custom"}
	idx, err := promptSelect("Toolset - which tools should the server expose?", opts, 1)
	if err != nil {
		return "", err
	}
	return vals[idx], nil
}

// --- client multi-select -----------------------------------------------------

func promptClients(detected, others []ClientDef) ([]string, error) {
	if !isTTY() {
		return nil, ErrCancelled
	}
	fmt.Printf("\n  AI Clients\n\n")
	if len(detected) == 0 {
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
	num := 0
	idByNum := map[int]string{}
	for _, c := range detected {
		num++
		idByNum[num] = c.ID
		fmt.Printf("    [x] %2d. %s (detected)\n", num, c.Name)
	}
	for _, c := range others {
		num++
		idByNum[num] = c.ID
		fmt.Printf("    [ ] %2d. %s\n", num, c.Name)
	}
	fmt.Printf("\n  Enter numbers to toggle (comma-separated, blank = keep defaults, 0 = cancel): ")
	line := readLine()
	if line == "0" || line == "q" {
		return nil, ErrCancelled
	}
	if line == "" {
		var out []string
		for _, c := range detected {
			out = append(out, c.ID)
		}
		return out, nil
	}
	// Build the toggle set: start from defaults (detected on), toggle user picks.
	selected := map[string]bool{}
	for _, c := range detected {
		selected[c.ID] = true
	}
	for _, p := range strings.Split(line, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err == nil {
			if id, ok := idByNum[n]; ok {
				selected[id] = !selected[id]
			}
		}
	}
	var out []string
	for _, c := range Clients {
		if selected[c.ID] {
			out = append(out, c.ID)
		}
	}
	return out, nil
}

// --- custom tools toggle -----------------------------------------------------

func promptTools(catalog []mcpserver.ToolInfo) ([]string, error) {
	if !isTTY() {
		return nil, ErrCancelled
	}
	fmt.Printf("\n  Toggle tools\n\n")
	num := 0
	nameByNum := map[int]string{}
	selected := map[string]bool{}
	for _, t := range catalog {
		num++
		nameByNum[num] = t.Name
		isOn := t.Tier == mcpserver.TierRead || t.Tier == mcpserver.TierWrite
		selected[t.Name] = isOn
		mark := "[ ]"
		if isOn {
			mark = "[x]"
		}
		fmt.Printf("    %s %2d. %-22s (%s)\n", mark, num, t.Name, t.Tier)
	}
	fmt.Printf("\n  Enter numbers to toggle (comma-separated, blank = keep defaults, 0 = cancel): ")
	line := readLine()
	if line == "0" || line == "q" {
		return nil, ErrCancelled
	}
	if line != "" {
		for _, p := range strings.Split(line, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err == nil {
				if name, ok := nameByNum[n]; ok {
					selected[name] = !selected[name]
				}
			}
		}
	}
	var out []string
	for _, t := range catalog {
		if selected[t.Name] {
			out = append(out, t.Name)
		}
	}
	return out, nil
}

// --- login inputs ------------------------------------------------------------

// promptLogin reads the Favro email (echoed) and API token (hidden on a TTY via
// term.ReadPassword). An empty email or token returns ErrCancelled.
func promptLogin(prefillEmail string) (email, token string, err error) {
	fmt.Printf("\n  Log in to Favro\n\n")
	fmt.Printf("  Email: ")
	email = readLine()
	if email == "" && prefillEmail != "" {
		email = prefillEmail
	}
	if email == "" {
		return "", "", ErrCancelled
	}
	fmt.Printf("  API token (hidden): ")
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, rerr := term.ReadPassword(fd)
		if rerr != nil {
			return "", "", rerr
		}
		token = strings.TrimSpace(string(b))
		fmt.Println()
	} else {
		token = readLine()
	}
	if token == "" {
		return "", "", ErrCancelled
	}
	return email, token, nil
}

// --- interactive app (bare invocation) ---------------------------------------

// RunApp is the interactive CLI app launched by bare `favro-mcp` on a real
// terminal. It shows a simple numbered menu.
func RunApp() {
	for {
		fmt.Print("\n  favro-mcp\n\n")
		fmt.Println("    1. Configure AI clients")
		fmt.Println("    2. Log in to Favro")
		fmt.Println("    3. Quit")
		fmt.Print("\n  Choose [1-3]: ")
		choice := readLine()
		switch choice {
		case "1":
			if err := RunInstall(Options{}); err != nil && !errors.Is(err, ErrCancelled) {
				fmt.Fprintln(os.Stderr, "  Error:", err)
			}
		case "2":
			if err := interactiveLogin(""); err != nil && !errors.Is(err, ErrCancelled) {
				fmt.Fprintln(os.Stderr, "  Error:", err)
			}
		case "3", "q", "quit", "":
			return
		default:
			fmt.Println("  Invalid choice.")
		}
	}
}
