package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lh-etals/favro-mcp/internal/credentials"
)

// Options controls install/uninstall behaviour.
type Options struct {
	DryRun bool
	Yes    bool
	Name   string

	// Credentials written into each client's env block. If empty, the installer
	// reads them from flags/env/stdin and falls back to placeholders.
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
		r, err := upsertJSONServer(file, inst.TopKey, name, e, dryRun)
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
		if dryRun {
			return ApplyResult{Status: "ok", Detail: "would run: " + inst.Bin + " " + strings.Join(args, " ")}
		}
		cmd := exec.Command(inst.Bin, args...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			return ApplyResult{Status: "failed", Detail: firstLine(err.Error())}
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
func RunInstall(opts Options) error {
	name := opts.Name
	if name == "" {
		name = "favro"
	}

	// Credentials are managed centrally by `favro-mcp login` (read by the
	// server at runtime), so client configs do not embed secrets by default.
	// We only embed them when explicitly provided via flags or FAVRO_* env.
	env := map[string]string{}
	email := opts.Email
	token := opts.Token
	if email == "" {
		email = os.Getenv("FAVRO_EMAIL")
	}
	if token == "" {
		token = os.Getenv("FAVRO_API_TOKEN")
	}
	if email != "" && token != "" {
		env["FAVRO_EMAIL"] = email
		env["FAVRO_API_TOKEN"] = token
	} else if !credentials.Exists() {
		fmt.Println("Note: Favro credentials not set. Run `favro-mcp login` (or export FAVRO_EMAIL/FAVRO_API_TOKEN) so the server can authenticate.")
		fmt.Println()
	}

	target, err := serverTarget(env)
	if err != nil {
		return err
	}

	var detected, others []ClientDef
	for _, c := range Clients {
		if safeDetect(c) {
			detected = append(detected, c)
		} else {
			others = append(others, c)
		}
	}

	fmt.Printf("favro-mcp installer - registering server %q\n", name)
	fmt.Printf("  command: %s\n\n", target.Command)

	var chosen []ClientDef
	if opts.Yes {
		chosen = detected
		if len(chosen) == 0 {
			fmt.Println("No supported clients detected. Re-run without --yes to pick manually.")
			return nil
		}
	} else {
		if len(detected) == 0 && len(others) == 0 {
			fmt.Println("No supported clients known for this platform.")
			return nil
		}
		choices := make([]choice, 0, len(detected)+len(others))
		for _, c := range detected {
			choices = append(choices, choice{id: c.ID, label: c.Name + " (detected)", checked: true})
		}
		for _, c := range others {
			choices = append(choices, choice{id: c.ID, label: c.Name, checked: false})
		}
		ids, err := multiSelect("Select clients to register favro-mcp with:", choices)
		if err != nil {
			return err
		}
		idSet := map[string]bool{}
		for _, id := range ids {
			idSet[id] = true
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
		r := applyClient(c, name, target, opts.DryRun)
		tail := ""
		if c.ReloadHint != "" {
			tail = " -> " + c.ReloadHint
		}
		fmt.Printf("  %s: %s%s\n", c.Name, describe(r), tail)
	}
	fmt.Println("\nDone.")
	return nil
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
		choices := make([]choice, 0, len(detected))
		for _, c := range detected {
			choices = append(choices, choice{id: c.ID, label: c.Name, checked: true})
		}
		ids, err := multiSelect(fmt.Sprintf("Remove %q from which clients?", name), choices)
		if err != nil {
			return err
		}
		idSet := map[string]bool{}
		for _, id := range ids {
			idSet[id] = true
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
		if dryRun {
			return ApplyResult{Status: "ok", Detail: "would run: " + inst.Bin + " " + strings.Join(args, " ")}
		}
		if err := exec.Command(inst.Bin, args...).Run(); err != nil {
			return ApplyResult{Status: "failed", Detail: firstLine(err.Error())}
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
