package install

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	detectharness "github.com/sairaph/detect-harness"
)

func testHarnesses() []Harness {
	return []Harness{
		{ID: "claude-code", Name: "Claude Code", State: detectharness.Detected},
		{ID: "cursor", Name: "Cursor", State: detectharness.Detected, Configured: true},
		{ID: "zed", Name: "Zed", State: detectharness.NotDetected},
		{ID: "windsurf", Name: "Windsurf", State: detectharness.Unavailable},
	}
}

func adopted(t *testing.T, opts Options) *model {
	t.Helper()
	m := &model{opts: opts}
	m.adopt(testHarnesses())
	return m
}

func TestAdoptSelectsDetectedAndConfigured(t *testing.T) {
	m := adopted(t, Options{})
	if !m.selected["claude-code"] || !m.selected["cursor"] {
		t.Fatalf("detected and configured harnesses should start selected: %v", m.selected)
	}
	if m.selected["zed"] || m.selected["windsurf"] {
		t.Fatalf("undetected harnesses should not start selected: %v", m.selected)
	}
	if m.step != stepToolset {
		t.Fatalf("with no --toolset the flow opens on the toolset question, got step %d", m.step)
	}
	if m.toolsetCursor != 1 {
		t.Fatalf("toolset cursor should open on Read + Write, got %d", m.toolsetCursor)
	}
}

func TestToolsetFlagSkipsToolsetScreen(t *testing.T) {
	m := adopted(t, Options{Toolset: "read"})
	if m.step != stepHarnesses {
		t.Fatalf("--toolset=read should skip straight to the client list, got step %d", m.step)
	}
	if m.toolsetChoice != "read" {
		t.Fatalf("toolsetChoice = %q, want read", m.toolsetChoice)
	}

	m = adopted(t, Options{Toolset: "custom"})
	if m.step != stepCustomTools {
		t.Fatalf("--toolset=custom should open the custom tool list, got step %d", m.step)
	}
}

func TestToolsetEnterOnCustomOpensToolList(t *testing.T) {
	m := adopted(t, Options{})
	m.toolsetCursor = 3
	m.toolsetKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != stepCustomTools {
		t.Fatalf("enter on Custom should open the tool list, got step %d", m.step)
	}
	m.toolsKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.step != stepToolset {
		t.Fatalf("esc in the tool list goes back to the toolset question, got step %d", m.step)
	}
}

func TestComputeEnvToolsets(t *testing.T) {
	m := &model{toolsetChoice: "read"}
	if env := m.computeEnv(); env["FAVRO_TOOLSET"] != "read" {
		t.Fatalf("read toolset env = %v", env)
	}

	m = &model{toolsetChoice: "custom", toolRows: []toolRow{
		{id: "list_boards", checked: true},
		{id: "create_card", checked: true},
		{id: "delete_card", checked: false},
	}}
	env := m.computeEnv()
	if env["FAVRO_TOOLS"] != "list_boards,create_card" {
		t.Fatalf("custom env = %v", env)
	}
	if _, ok := env["FAVRO_TOOLSET"]; ok {
		t.Fatalf("FAVRO_TOOLS and FAVRO_TOOLSET are mutually exclusive: %v", env)
	}

	// Nothing checked falls back to the recommended tier rather than
	// registering a server with no tools.
	m = &model{toolsetChoice: "custom", toolRows: []toolRow{{id: "list_boards"}}}
	if env := m.computeEnv(); env["FAVRO_TOOLSET"] != "write" {
		t.Fatalf("empty custom selection should fall back to write: %v", env)
	}
}

func TestComputeEnvEmbedsCredsOnlyWhenProvided(t *testing.T) {
	m := &model{toolsetChoice: "write", embedCreds: true, email: "a@b.c", token: "tok"}
	env := m.computeEnv()
	if env["FAVRO_EMAIL"] != "a@b.c" || env["FAVRO_API_TOKEN"] != "tok" {
		t.Fatalf("flag-provided creds should be embedded: %v", env)
	}
	m.embedCreds = false
	if env := m.computeEnv(); env["FAVRO_EMAIL"] != "" {
		t.Fatalf("creds must not be embedded without embedCreds: %v", env)
	}
}

func TestCursorSkipsUnselectableRows(t *testing.T) {
	m := adopted(t, Options{Toolset: "write"})
	m.showAll = true
	m.cursor = m.firstSelectable()
	seen := map[int]bool{}
	for range m.harnesses {
		if !m.harnesses[m.cursor].Selectable() {
			t.Fatalf("cursor landed on unselectable row %d (%s)", m.cursor, m.harnesses[m.cursor].Name)
		}
		seen[m.cursor] = true
		m.moveCursor(1)
	}
	if len(seen) != 3 {
		t.Fatalf("cursor should walk the 3 selectable rows, walked %d", len(seen))
	}
}

func TestVisibleKeepsSelectedHiddenRows(t *testing.T) {
	m := adopted(t, Options{Toolset: "write"})
	m.showAll = true
	// Select the undetected Zed, then hide the undetected group.
	m.cursor = 2
	m.toggle()
	m.showAll = false
	visible := m.visible()
	found := false
	for _, index := range visible {
		if m.harnesses[index].ID == "zed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a selected row must stay visible after v hides its group: %v", visible)
	}
}

func TestToggleAllOnlyTouchesVisible(t *testing.T) {
	m := adopted(t, Options{Toolset: "write"})
	m.selected["claude-code"] = false
	m.toggleAll()
	if !m.selected["claude-code"] || !m.selected["cursor"] {
		t.Fatalf("toggleAll should select every visible selectable row: %v", m.selected)
	}
	if m.selected["zed"] {
		t.Fatalf("toggleAll must not select rows hidden behind v: %v", m.selected)
	}
}

func TestAppliedMsgSettlesExceptDryRun(t *testing.T) {
	m := adopted(t, Options{Toolset: "write"})
	m.signedIn = true
	m.Update(appliedMsg{})
	if !m.settled {
		t.Fatal("a real apply is the point of no return")
	}
	if m.step != stepDone {
		t.Fatalf("signed in after apply should land on the summary, got step %d", m.step)
	}

	m = adopted(t, Options{Toolset: "write", DryRun: true})
	m.signedIn = false
	m.Update(appliedMsg{})
	if m.settled {
		t.Fatal("a dry run changed nothing and must stay cancellable")
	}
	if m.step != stepLogin {
		t.Fatalf("not signed in after apply should offer the sign-in, got step %d", m.step)
	}
}

func TestVerifyFailureReturnsToEmail(t *testing.T) {
	m := adopted(t, Options{Toolset: "write"})
	m.settled = true
	m.step = stepVerifying
	m.email, m.token = "a@b.c", "bad"
	m.Update(verifiedMsg{err: errFake("401 unauthorized")})
	if m.step != stepEmail {
		t.Fatalf("a failed verification is recoverable and returns to email, got step %d", m.step)
	}
	if !strings.Contains(m.message, "verification failed") {
		t.Fatalf("message = %q", m.message)
	}
	if !m.settled {
		t.Fatal("a failed login must not unsettle the finished client configuration")
	}
	if m.input != "a@b.c" {
		t.Fatalf("the email should be editable rather than retyped, input = %q", m.input)
	}
}

func TestEscDuringLoginKeepsSettledWork(t *testing.T) {
	m := adopted(t, Options{Toolset: "write"})
	m.settled = true
	m.step = stepEmail
	m.inputKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.step != stepDone {
		t.Fatalf("esc skips the sign-in and lands on the summary, got step %d", m.step)
	}
	if m.cancel {
		t.Fatal("skipping login is not a cancel")
	}
}

func TestApplyingTakesNoKeys(t *testing.T) {
	m := adopted(t, Options{Toolset: "write"})
	for _, step := range []step{stepApplying, stepRemoving, stepVerifying} {
		m.step = step
		m.cancel = false
		m.key(tea.KeyMsg{Type: tea.KeyCtrlC})
		if m.cancel {
			t.Fatalf("step %d must ignore ctrl+c mid-write", step)
		}
	}
}

func TestUnattendedEnv(t *testing.T) {
	if _, err := unattendedEnv(Options{Toolset: "custom", Yes: true}); err == nil {
		t.Fatal("--toolset=custom --yes cannot be honoured and must error")
	}
	env, err := unattendedEnv(Options{})
	if err != nil || env["FAVRO_TOOLSET"] != "write" {
		t.Fatalf("default unattended toolset should be write: %v, %v", env, err)
	}
	env, _ = unattendedEnv(Options{Toolset: "delete", Email: "a@b.c", Token: "tok"})
	if env["FAVRO_TOOLSET"] != "delete" || env["FAVRO_EMAIL"] != "a@b.c" {
		t.Fatalf("unattended env = %v", env)
	}
}

func TestSummarise(t *testing.T) {
	cases := []struct {
		result   detectharness.Result
		enabling bool
		dryRun   bool
		want     string
	}{
		{detectharness.Result{State: detectharness.Applied}, true, false, "registered"},
		{detectharness.Result{State: detectharness.Applied, Action: "update"}, true, false, "replaced an existing entry"},
		{detectharness.Result{State: detectharness.Applied}, true, true, "would register"},
		{detectharness.Result{State: detectharness.Applied}, false, false, "removed"},
		{detectharness.Result{State: detectharness.Applied}, false, true, "would remove"},
		{detectharness.Result{State: detectharness.ApplyNoop}, true, false, "already correct"},
		{detectharness.Result{State: detectharness.ApplySkipped, Reason: "no config"}, true, false, "skipped: no config"},
		{detectharness.Result{State: detectharness.ApplyConflict}, true, false, "conflict: another server is registered under this name"},
		{detectharness.Result{State: detectharness.ApplyFailed, Reason: "denied"}, true, false, "failed: denied"},
	}
	for _, c := range cases {
		if got := summarise(c.result, c.enabling, c.dryRun); got != c.want {
			t.Errorf("summarise(%v, %v, %v) = %q, want %q", c.result.State, c.enabling, c.dryRun, got, c.want)
		}
	}
}

func TestToolsetLabel(t *testing.T) {
	m := &model{toolsetChoice: "write"}
	if got := m.toolsetLabel(); got != "Read + Write" {
		t.Fatalf("toolsetLabel = %q", got)
	}
	m = &model{toolsetChoice: "custom", toolRows: []toolRow{
		{checked: true}, {checked: true}, {checked: false},
	}}
	if got := m.toolsetLabel(); got != "Custom (2 tools)" {
		t.Fatalf("toolsetLabel = %q", got)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }
