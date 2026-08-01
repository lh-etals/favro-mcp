package install

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/mcpserver"
	detectharness "github.com/sairaph/detect-harness"
	"golang.org/x/term"
)

// Without a terminal there is nobody to answer a question, so this path asks
// none and prints plain indented lines.
//
// It exists because piping the installer through a shell is the normal install
// route, and a container, a CI job or a harness invoking `favro-mcp configure`
// all arrive here. Starting a full-screen program with no controllable input
// leaves it drawing frames at a pipe until something kills it.

// Interactive reports whether both streams are terminals. It is the one TTY
// check in this program; main and the app shell call it too.
func Interactive(stdin io.Reader, stdout io.Writer) bool {
	in, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	out, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}

func interactive(opts Options) bool {
	return Interactive(opts.Stdin, opts.Stdout)
}

// unattendedEnv resolves the env block without a user to ask. A custom toolset
// cannot be picked blind, so --toolset=custom with --yes is refused rather
// than silently widened.
func unattendedEnv(opts Options) (map[string]string, error) {
	env := map[string]string{}
	switch opts.Toolset {
	case mcpserver.TierRead, mcpserver.TierWrite, mcpserver.TierDelete:
		env["FAVRO_TOOLSET"] = opts.Toolset
	case "custom":
		if opts.Yes {
			return nil, fmt.Errorf("--toolset=custom requires interactive selection (omit --yes)")
		}
		env["FAVRO_TOOLSET"] = mcpserver.TierWrite
	default:
		env["FAVRO_TOOLSET"] = mcpserver.TierWrite
	}
	if opts.Email != "" && opts.Token != "" {
		env["FAVRO_EMAIL"] = opts.Email
		env["FAVRO_API_TOKEN"] = opts.Token
	}
	return env, nil
}

// runUnattended registers every detected client, or removes every registration.
func runUnattended(ctx context.Context, probe *Installer, name string, opts Options) int {
	// A blank line first: this output follows the install script's progress
	// bar, and the two should not run together.
	fmt.Fprintln(opts.Stdout)

	remove := opts.Uninstall
	installer := probe
	if !remove {
		env, err := unattendedEnv(opts)
		if err != nil {
			fmt.Fprintln(opts.Stderr, "favro-mcp:", err)
			return 1
		}
		rebuilt, err := NewInstaller(name, env)
		if err != nil {
			fmt.Fprintln(opts.Stderr, "favro-mcp:", err)
			return 1
		}
		installer = rebuilt
	}

	var ids []detectharness.ID
	byID := map[detectharness.ID]Harness{}
	for _, harness := range probe.Detect(ctx) {
		byID[harness.ID] = harness
		switch {
		case remove && harness.Configured:
			ids = append(ids, harness.ID)
		case !remove && harness.Selectable() && harness.State == detectharness.Detected:
			// Registering with a client that is not installed writes a config
			// file for software the user does not have.
			ids = append(ids, harness.ID)
		}
	}

	desired := detectharness.Present
	if remove {
		desired = detectharness.Absent
	}
	var results []detectharness.Result
	if opts.DryRun {
		results = installer.PlanResults(ctx, ids, desired)
	} else {
		results = installer.Apply(ctx, ids, desired)
	}

	if len(results) == 0 {
		if remove {
			fmt.Fprintln(opts.Stdout, "  favro-mcp was not registered with any client.")
		} else {
			fmt.Fprintln(opts.Stdout,
				"  No AI clients were detected. Install one, then run `favro-mcp configure`.")
		}
	}

	failures := 0
	changed := false
	for _, result := range results {
		fmt.Fprintf(opts.Stdout, "  %-22s %s\n", result.Name, summarise(result, !remove, opts.DryRun))
		if result.State == detectharness.ApplyFailed || result.State == detectharness.ApplyConflict {
			failures++
		}
		if result.State == detectharness.Applied {
			changed = true
		}
	}
	if changed && !remove && !opts.DryRun {
		fmt.Fprintln(opts.Stdout, "\n  Restart the affected clients so they pick up the change:")
		for _, result := range results {
			hint := byID[result.HarnessID].ReloadHint
			if result.State == detectharness.Applied && hint != "" {
				fmt.Fprintf(opts.Stdout, "  %-22s %s\n", result.Name, hint)
			}
		}
	}

	embedCreds := opts.Email != "" && opts.Token != ""
	if !remove && !embedCreds && !credentials.Exists() {
		fmt.Fprintln(opts.Stdout,
			"\n  Not signed in to Favro yet. Run `favro-mcp login` so the server can authenticate.")
	}

	if failures > 0 {
		return 1
	}
	return 0
}
