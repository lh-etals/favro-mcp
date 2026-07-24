package install

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/mcpserver"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// Options controls install/uninstall behaviour.
type Options struct {
	DryRun  bool
	Yes     bool
	Name    string
	Toolset string // "", "read", "write", "delete", or "custom"

	// Credentials written into each client's env block (instead of the login
	// store). If empty, the server reads `favro-mcp login` creds at runtime.
	Email string
	Token string
}

// ApplyResult is the outcome of registering with one client.
type ApplyResult struct {
	Status string // "ok" | "noop" | "skipped" | "failed"
	Detail string
}

func safeDetect(c ClientDef) bool {
	defer func() { _ = recover() }()
	return c.Detect()
}

func mapWrite(r WriteResult, file string, dryRun bool) ApplyResult {
	switch r {
	case writeSkippedUnparseable:
		return ApplyResult{Status: "skipped", Detail: file + " is not valid; left untouched"}
	case writeNoop:
		return ApplyResult{Status: "noop"}
	default:
		d := file
		if dryRun {
			d = "would write " + file
		}
		return ApplyResult{Status: "ok", Detail: d}
	}
}

func applyClient(c ClientDef, name string, e ServerTarget, dryRun bool) ApplyResult {
	inst := c.Install
	switch inst.Kind {
	case "file-json":
		file := inst.path()
		if file == "" {
			return ApplyResult{Status: "skipped", Detail: "not supported on this platform"}
		}
		r, err := upsertJSONServer(file, inst.TopKey, name, e, dryRun, inst.entryFn)
		if err != nil {
			return ApplyResult{Status: "failed", Detail: err.Error()}
		}
		return mapWrite(r, file, dryRun)
	case "file-toml":
		file := inst.path()
		if file == "" {
			return ApplyResult{Status: "skipped", Detail: "not supported on this platform"}
		}
		r, err := upsertTomlServer(file, name, e, dryRun)
		if err != nil {
			return ApplyResult{Status: "failed", Detail: err.Error()}
		}
		return mapWrite(r, file, dryRun)
	case "file-yaml-list":
		file := inst.path()
		if file == "" {
			return ApplyResult{Status: "skipped", Detail: "not supported on this platform"}
		}
		r, err := upsertYamlServerList(file, name, e, dryRun)
		if err != nil {
			return ApplyResult{Status: "failed", Detail: err.Error()}
		}
		return mapWrite(r, file, dryRun)
	case "command":
		args := inst.buildArgs(name, e)
		bin := inst.Bin
		if inst.resolveBin != nil {
			if p := inst.resolveBin(); p != "" {
				bin = p
			}
		}
		if dryRun {
			return ApplyResult{Status: "ok", Detail: "would run: " + bin + " " + strings.Join(args, " ")}
		}
		cmd := exec.Command(bin, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			detail := firstLine(err.Error())
			if s := strings.TrimSpace(stderr.String()); s != "" {
				detail = firstLine(s)
			}
			return ApplyResult{Status: "failed", Detail: detail}
		}
		return ApplyResult{Status: "ok"}
	}
	return ApplyResult{Status: "skipped", Detail: "unknown install kind"}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// isCurrentlyRegistered reports whether `name` is already present in this
// client's config (or, for command-style clients where we can't tell, assumes
// it is so the installer never skips a legitimate removal). Used to diff the
// user's new selection against the prior state and unregister deselected
// clients.
func isCurrentlyRegistered(c ClientDef, serverName string) bool {
	inst := c.Install
	switch inst.Kind {
	case "file-json":
		file := inst.path()
		if file == "" {
			return false
		}
		_, data, err := readJSONTolerant(file)
		if err != nil || data == nil {
			return false
		}
		servers, _ := data[inst.TopKey].(map[string]any)
		_, exists := servers[serverName]
		return exists
	case "file-toml":
		file := inst.path()
		if file == "" {
			return false
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return false
		}
		raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
		if strings.TrimSpace(string(raw)) == "" {
			return false
		}
		doc := map[string]any{}
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return false
		}
		servers, _ := doc["mcp_servers"].(map[string]any)
		_, exists := servers[serverName]
		return exists
	case "file-yaml-list":
		file := inst.path()
		if file == "" {
			return false
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return false
		}
		rootMap := map[string]any{}
		if err := yaml.Unmarshal(raw, &rootMap); err != nil {
			return false
		}
		list, _ := rootMap["mcpServers"].([]any)
		for _, it := range list {
			if m, ok := it.(map[string]any); ok {
				if n, _ := m["name"].(string); n == serverName {
					return true
				}
			}
		}
		return false
	case "command":
		// Can't easily inspect external CLI state; assume registered so a
		// deselected detected client still gets a removal attempt.
		return true
	}
	return false
}

func describe(r ApplyResult) string {
	switch r.Status {
	case "ok":
		if r.Detail != "" {
			return "registered -> " + r.Detail
		}
		return "registered"
	case "noop":
		return "already registered (no change)"
	case "skipped":
		return "skipped: " + r.Detail
	case "failed":
		return "failed: " + r.Detail
	}
	return ""
}

// RunInstall detects installed MCP clients and registers this server with the
// ones the user chooses. It is idempotent and non-destructive.
//
// On a real terminal (no --yes) the entire interactive flow - toolset, client
// selection, optional custom tools, optional login, applying, summary - runs as
// a single tea.Program so the screen evolves in place instead of appending a
// new block per step. In unattended mode (--yes or no TTY) it applies sensible
// defaults and prints results directly.
func RunInstall(opts Options) error {
	name := opts.Name
	if name == "" {
		name = "favro"
	}

	// Credentials are managed centrally by `favro-mcp login` (read by the
	// server at runtime), so client configs do not embed secrets by default.
	// We only embed them when explicitly provided via flags or FAVRO_* env.
	email := opts.Email
	if email == "" {
		email = os.Getenv("FAVRO_EMAIL")
	}
	token := opts.Token
	if token == "" {
		token = os.Getenv("FAVRO_API_TOKEN")
	}
	embedCreds := email != "" && token != ""

	var detected, others []ClientDef
	for _, c := range Clients {
		if safeDetect(c) {
			detected = append(detected, c)
		} else {
			others = append(others, c)
		}
	}

	if !opts.Yes && isTTY() {
		return runInteractive(opts, name, email, token, embedCreds, detected, others)
	}
	return runUnattended(opts, name, email, token, embedCreds, detected, others)
}

// runInteractive drives the single-program installer UI.
func runInteractive(opts Options, name, email, token string, embedCreds bool, detected, others []ClientDef) error {
	if len(detected) == 0 && len(others) == 0 {
		fmt.Println("No supported clients known for this platform.")
		return nil
	}
	// Snapshot each detected client's current registration state BEFORE the
	// prompt so deselected-but-registered clients can be unregistered.
	wasRegistered := map[string]bool{}
	for _, c := range detected {
		if isCurrentlyRegistered(c, name) {
			wasRegistered[c.ID] = true
		}
	}
	needLogin := !embedCreds && !credentials.Exists()
	m := newInstallModel(opts, name, email, token, embedCreds, needLogin, detected, others, wasRegistered)
	return runForm(m)
}

// runUnattended applies defaults (Read+Write toolset, all detected clients)
// without any prompts. Used for --yes or when stdin/stdout is not a TTY.
func runUnattended(opts Options, name, email, token string, embedCreds bool, detected, others []ClientDef) error {
	env := map[string]string{}
	switch opts.Toolset {
	case mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete:
		env["FAVRO_TOOLSET"] = opts.Toolset
	case "custom":
		if opts.Yes {
			return fmt.Errorf("--toolset=custom requires interactive selection (omit --yes)")
		}
		// Non-interactive custom selection isn't possible; default to Read+Write.
		env["FAVRO_TOOLSET"] = mcpserver.TierWrite
	case "":
		env["FAVRO_TOOLSET"] = mcpserver.TierWrite
	default:
		return fmt.Errorf("unknown --toolset %q (use read, write, delete, or custom)", opts.Toolset)
	}

	if embedCreds {
		env["FAVRO_EMAIL"] = email
		env["FAVRO_API_TOKEN"] = token
	} else if !credentials.Exists() {
		if !opts.Yes {
			// Non-interactive, no creds, no --yes: nothing more we can do.
			return ErrCancelled
		}
		fmt.Println("Note: Favro credentials not set. Run `favro-mcp login` (or export FAVRO_EMAIL/FAVRO_API_TOKEN) so the server can authenticate.")
		fmt.Println()
	}

	target, err := serverTarget(env)
	if err != nil {
		return err
	}

	if len(detected) == 0 {
		fmt.Println("No supported clients detected. Re-run without --yes to pick manually.")
		return nil
	}

	detectedSet := map[string]bool{}
	for _, c := range detected {
		detectedSet[c.ID] = true
	}

	fmt.Printf("favro-mcp installer - registering server %q\n", name)
	fmt.Printf("  command: %s\n\n", target.Command)
	if opts.DryRun {
		fmt.Print("Dry run - no files will be changed.\n\n")
	}
	for _, c := range Clients {
		if !detectedSet[c.ID] {
			continue
		}
		r := applyClient(c, name, target, opts.DryRun)
		tail := ""
		if c.ReloadHint != "" {
			tail = " -> " + c.ReloadHint
		}
		fmt.Printf("  %s: %s%s\n", c.Name, describe(r), tail)
	}
	fmt.Println("\nDone.")
	fmt.Println("Run `favro-mcp configure` anytime to change the toolset, clients, or re-login.")
	return nil
}

// interactiveLogin prompts for email + token (hidden on a TTY) via the TUI,
// verifies them against the Favro API, and saves them only on success. Loops
// on failure. Used by RunApp's standalone "Log in to Favro" entry (the
// installer flow folds login into its own single-program UI instead).
func interactiveLogin(prefillEmail string) error {
	if !isTTY() {
		return ErrCancelled
	}
	for {
		p := tea.NewProgram(newLoginModel(prefillEmail))
		out, err := p.Run()
		if err != nil {
			return err
		}
		m := out.(loginModel)
		if m.cancel {
			return ErrCancelled
		}
		email := strings.TrimSpace(m.email.Value())
		token := strings.TrimSpace(m.token.Value())
		if email == "" || token == "" {
			return ErrCancelled
		}
		if err := verifyAndSaveCreds(email, token); err != nil {
			fmt.Printf("\nVerification failed: %v\nPlease try again.\n\n", err)
			prefillEmail = email
			continue
		}
		fmt.Println("Credentials verified and saved.")
		return nil
	}
}

// verifyAndSaveCreds checks the credentials against the live Favro API and, on
// success, persists them. Invalid credentials are never saved.
func verifyAndSaveCreds(email, token string) error {
	if _, err := favro.NewClient(email, token, "").GetOrganizations(); err != nil {
		return err
	}
	return credentials.Save(email, token)
}

// RunUninstall removes this server from the MCP clients the user chooses.
func RunUninstall(opts Options) error {
	name := opts.Name
	if name == "" {
		name = "favro"
	}
	var detected []ClientDef
	for _, c := range Clients {
		if safeDetect(c) {
			detected = append(detected, c)
		}
	}
	if len(detected) == 0 {
		fmt.Println("No supported clients detected.")
		return nil
	}

	var chosen []ClientDef
	if opts.Yes {
		chosen = detected
	} else {
		rows := make([]multiRow, len(detected))
		for i, c := range detected {
			rows[i] = multiRow{id: c.ID, label: c.Name, checked: true}
		}
		out, err := runMultiSelect(fmt.Sprintf("Remove %q from which clients?", name), rows)
		if err != nil {
			return err
		}
		idSet := map[string]bool{}
		for _, r := range out {
			if r.checked {
				idSet[r.id] = true
			}
		}
		for _, c := range Clients {
			if idSet[c.ID] {
				chosen = append(chosen, c)
			}
		}
	}
	if len(chosen) == 0 {
		fmt.Println("Nothing selected; no changes made.")
		return nil
	}

	if opts.DryRun {
		fmt.Print("Dry run - no files will be changed.\n\n")
	}
	for _, c := range chosen {
		r := applyRemove(c, name, opts.DryRun)
		fmt.Printf("  %s: %s\n", c.Name, describeRemove(r, opts.DryRun))
	}
	fmt.Println("\nDone.")
	return nil
}

func applyRemove(c ClientDef, name string, dryRun bool) ApplyResult {
	inst := c.Install
	switch inst.Kind {
	case "file-json":
		file := inst.path()
		if file == "" {
			return ApplyResult{Status: "skipped", Detail: "not supported on this platform"}
		}
		r, err := removeJSONServer(file, inst.TopKey, name, dryRun)
		if err != nil {
			return ApplyResult{Status: "failed", Detail: err.Error()}
		}
		return mapWrite(r, file, dryRun)
	case "file-toml":
		file := inst.path()
		if file == "" {
			return ApplyResult{Status: "skipped", Detail: "not supported on this platform"}
		}
		r, err := removeTomlServer(file, name, dryRun)
		if err != nil {
			return ApplyResult{Status: "failed", Detail: err.Error()}
		}
		return mapWrite(r, file, dryRun)
	case "file-yaml-list":
		file := inst.path()
		if file == "" {
			return ApplyResult{Status: "skipped", Detail: "not supported on this platform"}
		}
		r, err := removeYamlServerList(file, name, dryRun)
		if err != nil {
			return ApplyResult{Status: "failed", Detail: err.Error()}
		}
		return mapWrite(r, file, dryRun)
	case "command":
		if inst.removeArgs == nil {
			return ApplyResult{Status: "skipped", Detail: "no automated removal for this client"}
		}
		args := inst.removeArgs(name)
		bin := inst.Bin
		if inst.resolveBin != nil {
			if p := inst.resolveBin(); p != "" {
				bin = p
			}
		}
		if dryRun {
			return ApplyResult{Status: "ok", Detail: "would run: " + bin + " " + strings.Join(args, " ")}
		}
		cmd := exec.Command(bin, args...)
		var rerr bytes.Buffer
		cmd.Stderr = &rerr
		if err := cmd.Run(); err != nil {
			detail := firstLine(err.Error())
			if se := strings.TrimSpace(rerr.String()); se != "" {
				detail = firstLine(se)
			}
			return ApplyResult{Status: "failed", Detail: detail}
		}
		return ApplyResult{Status: "ok"}
	}
	return ApplyResult{Status: "skipped", Detail: "unknown install kind"}
}

func describeRemove(r ApplyResult, dryRun bool) string {
	if r.Status == "ok" {
		if dryRun {
			return "would remove"
		}
		return "removed"
	}
	if r.Status == "noop" {
		return "not registered (nothing to remove)"
	}
	return describe(r)
}
