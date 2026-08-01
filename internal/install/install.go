// Package install registers the MCP server with the AI harnesses on this
// machine, in the shape shared with sana-mcp and interactive-terminal-mcp.
//
// Detection and config writing are detect-harness's job. This package decides
// what to show, what to ask, and what to do with the answers.
package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lh-etals/favro-mcp/internal/mcpserver"
	detectharness "github.com/sairaph/detect-harness"
)

// ServerName is the default entry name written into every harness
// configuration, overridable with --name.
const ServerName = "favro"

// Harness is one detected AI client, in the shape the UI needs.
type Harness struct {
	ID   detectharness.ID
	Name string
	// State is present, absent, or unavailable. Unavailable stays distinct from
	// absent so a permission error is never reported as "not installed".
	State       detectharness.DetectionState
	Configured  bool
	ConfigError string
	ReloadHint  string
}

// Selectable reports whether the user may toggle this harness. An environment
// that could not be inspected is shown but cannot be chosen, because writing to
// it would be guesswork.
func (h Harness) Selectable() bool {
	return h.State != detectharness.Unavailable && h.ConfigError == ""
}

// StatusText renders the harness state for a person, in at most two words.
//
// "cannot inspect" is its own answer. Reporting an environment that could not
// be read as "not detected" states the one thing the detection did not
// establish, and contradicted the message the same row gives when it is
// selected.
func (h Harness) StatusText() string {
	switch {
	case h.Configured:
		return "configured"
	case h.State == detectharness.Detected:
		return "detected"
	case !h.Selectable():
		return "cannot inspect"
	default:
		return "not detected"
	}
}

// Installer wraps detect-harness with this project's server definition.
type Installer struct {
	installer *detectharness.Installer
}

// DefaultEnv is the env block written when nothing narrows the toolset: the
// recommended Read + Write tier, no embedded credentials.
func DefaultEnv() map[string]string {
	return map[string]string{"FAVRO_TOOLSET": mcpserver.TierWrite}
}

// NewInstaller builds an installer that registers this binary under name with
// the given env block. The toolset is chosen mid-flow, so the installer used
// for detection carries DefaultEnv and is rebuilt with the real env just
// before applying; construction is cheap and does no I/O.
func NewInstaller(name string, env map[string]string) (*Installer, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve this binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	// The registered command runs the MCP server explicitly rather than relying
	// on the bare-invocation fallback, so a harness that allocates a TTY still
	// gets a server rather than the interactive application.
	inner, err := detectharness.New(detectharness.StdioServer{
		Name:    name,
		Command: executable,
		Args:    []string{"mcp"},
		Env:     env,
	})
	if err != nil {
		return nil, err
	}
	return &Installer{installer: inner}, nil
}

// Detect probes the machine and returns the harnesses worth showing, detected
// and configured ones first, then the rest alphabetically.
//
// Whether a harness is already configured is answered by planning the
// registration and seeing which harnesses report no work to do. Detecting a
// config file only proves the client exists, not that this server is in it.
// The comparison includes the env block, so a registration carrying a
// different toolset shows as "detected" and applying reports it replaced.
func (i *Installer) Detect(ctx context.Context) []Harness {
	detections := i.installer.Detect(ctx)

	ids := make([]detectharness.ID, 0, len(detections))
	for _, detection := range detections {
		ids = append(ids, detection.ID)
	}
	registered := map[detectharness.ID]bool{}
	if plan := i.installer.Plan(ctx, ids, detectharness.Present, planOptions()); plan != nil {
		for _, change := range plan.Changes() {
			registered[change.HarnessID] = change.State == detectharness.ChangeNoop
		}
	}

	harnesses := make([]Harness, 0, len(detections))
	for _, detection := range detections {
		harnesses = append(harnesses, Harness{
			ID:          detection.ID,
			Name:        detection.Name,
			State:       detection.State,
			Configured:  registered[detection.ID],
			ConfigError: detection.ConfigError,
			ReloadHint:  detection.ReloadHint,
		})
	}
	sort.SliceStable(harnesses, func(a, b int) bool {
		left, right := harnesses[a], harnesses[b]
		if (left.State == detectharness.Detected) != (right.State == detectharness.Detected) {
			return left.State == detectharness.Detected
		}
		return left.Name < right.Name
	})
	return harnesses
}

// planOptions replaces an existing entry of the same name rather than refusing.
//
// The entry is called "favro". A same-name entry that does not match is this
// program's own earlier registration - versions before the detect-harness
// rewrite wrote their own config entries, and upgrading from them is the case
// this exists to serve. Refusing meant the upgrade reported "another server is
// registered under this name", exited non-zero, and could never succeed
// however many times it was run.
func planOptions() detectharness.PlanOptions {
	return detectharness.PlanOptions{ConflictPolicy: detectharness.ConflictReplace}
}

// Apply registers or removes the server for the given harnesses.
func (i *Installer) Apply(ctx context.Context, ids []detectharness.ID, desired detectharness.DesiredState) []detectharness.Result {
	if len(ids) == 0 {
		return nil
	}
	return i.installer.Ensure(ctx, ids, desired, planOptions())
}

// PlanResults previews what Apply would do, in Apply's own result shape, so a
// dry run renders through the same summary vocabulary as a real one.
func (i *Installer) PlanResults(ctx context.Context, ids []detectharness.ID, desired detectharness.DesiredState) []detectharness.Result {
	if len(ids) == 0 {
		return nil
	}
	plan := i.installer.Plan(ctx, ids, desired, planOptions())
	changes := plan.Changes()
	results := make([]detectharness.Result, 0, len(changes))
	for _, change := range changes {
		state := detectharness.ApplyState("")
		switch change.State {
		case detectharness.ChangeReady:
			state = detectharness.Applied
		case detectharness.ChangeNoop:
			state = detectharness.ApplyNoop
		case detectharness.ChangeConflict:
			state = detectharness.ApplyConflict
		case detectharness.ChangeUnavailable:
			state = detectharness.ApplySkipped
		}
		results = append(results, detectharness.Result{
			HarnessID: change.HarnessID,
			Name:      change.Name,
			Path:      change.Path,
			Desired:   change.Desired,
			State:     state,
			Action:    change.Action,
			Reason:    change.Reason,
		})
	}
	return results
}
