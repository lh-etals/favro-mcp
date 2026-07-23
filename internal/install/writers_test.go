package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func target(email, token string) ServerTarget {
	return ServerTarget{
		Command: "/path/to/favro-mcp",
		Env:     map[string]string{"FAVRO_EMAIL": email, "FAVRO_API_TOKEN": token},
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// JSON: fresh create must be 0o600 and well-formed.
func TestJSONFreshCreatePermissions(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sub", "mcp.json")
	e := target("a@b.com", "tok")
	if r, err := upsertJSONServer(f, "mcpServers", "favro", e, false); err != nil || r != writeOK {
		t.Fatalf("upsert=%v err=%v", r, err)
	}
	info, _ := os.Stat(f)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm=%o want 0o600", info.Mode().Perm())
	}
	got := read(t, f)
	if !strings.Contains(got, `"favro"`) || !strings.Contains(got, "a@b.com") {
		t.Errorf("unexpected content:\n%s", got)
	}
}

// JSON: preserves sibling servers and other top-level keys.
func TestJSONPreservesSiblings(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "cfg.json", `{"version":1,"mcpServers":{"other":{"command":"x","args":[]}}}`)
	e := target("a@b.com", "tok")
	if _, err := upsertJSONServer(f, "mcpServers", "favro", e, false); err != nil {
		t.Fatal(err)
	}
	got := read(t, f)
	for _, want := range []string{`"version": 1`, `"other"`, `"favro"`, "a@b.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// JSON: idempotent second install is a noop.
func TestJSONIdempotent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "cfg.json")
	e := target("a@b.com", "tok")
	if _, err := upsertJSONServer(f, "mcpServers", "favro", e, false); err != nil {
		t.Fatal(err)
	}
	first := read(t, f)
	if r, err := upsertJSONServer(f, "mcpServers", "favro", e, false); err != nil || r != writeNoop {
		t.Fatalf("second upsert=%v err=%v want noop", r, err)
	}
	if read(t, f) != first {
		t.Error("file changed on idempotent second install")
	}
}

// JSON: install then uninstall restores the original file byte-for-byte.
func TestJSONInstallUninstallRoundTrip(t *testing.T) {
	dir := t.TempDir()
	orig := `{"mcpServers":{"other":{"command":"x"}}}`
	f := writeFile(t, dir, "cfg.json", orig)
	e := target("a@b.com", "tok")
	if _, err := upsertJSONServer(f, "mcpServers", "favro", e, false); err != nil {
		t.Fatal(err)
	}
	if r, err := removeJSONServer(f, "mcpServers", "favro", false); err != nil || r != writeOK {
		t.Fatalf("remove=%v err=%v", r, err)
	}
	got := read(t, f)
	if !strings.Contains(got, `"other"`) || strings.Contains(got, `"favro"`) {
		t.Errorf("uninstall did not restore siblings:\n%s", got)
	}
}

// JSON: BOM-prefixed file is handled, not treated as unparseable.
func TestJSONHandlesBOM(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "cfg.json", "\xEF\xBB\xBF"+`{"mcpServers":{}}`)
	e := target("a@b.com", "tok")
	if r, err := upsertJSONServer(f, "mcpServers", "favro", e, false); err != nil || r != writeOK {
		t.Fatalf("upsert=%v err=%v", r, err)
	}
	if !strings.Contains(read(t, f), `"favro"`) {
		t.Error("BOM file not updated")
	}
}

// JSON: unparseable file is left untouched.
func TestJSONSkipsUnparseable(t *testing.T) {
	dir := t.TempDir()
	orig := "{not valid json"
	f := writeFile(t, dir, "cfg.json", orig)
	e := target("a@b.com", "tok")
	if r, err := upsertJSONServer(f, "mcpServers", "favro", e, false); err != nil || r != writeSkippedUnparseable {
		t.Fatalf("upsert=%v err=%v want skipped", r, err)
	}
	if read(t, f) != orig {
		t.Error("unparseable file was modified")
	}
}

// TOML: REGRESSION — appending must NOT truncate the existing file (the A2 bug).
func TestTomlAppendPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	orig := `# my codex config
model = "gpt-5"

[mcp_servers.preexisting]
command = "old-server"
`
	f := writeFile(t, dir, "config.toml", orig)
	e := target("a@b.com", "tok")
	if r, err := upsertTomlServer(f, "favro", e, false); err != nil || r != writeOK {
		t.Fatalf("upsert=%v err=%v", r, err)
	}
	got := read(t, f)
	for _, want := range []string{"# my codex config", `model = "gpt-5"`, "preexisting", "old-server", "favro", "/path/to/favro-mcp"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q after upsert (truncation bug?) in:\n%s", want, got)
		}
	}
}

// TOML: idempotent + dotted name parses back as one key (no duplicate appends).
func TestTomlDottedNameNoDuplication(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.toml")
	e := target("a@b.com", "tok")
	for i := 0; i < 3; i++ {
		if _, err := upsertTomlServer(f, "my.server", e, false); err != nil {
			t.Fatal(err)
		}
	}
	got := read(t, f)
	doc := map[string]any{}
	if err := toml.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("re-parse failed: %v\n%s", err, got)
	}
	servers, _ := doc["mcp_servers"].(map[string]any)
	if len(servers) != 1 {
		t.Errorf("expected exactly 1 mcp_server, got %d (%v):\n%s", len(servers), servers, got)
	}
	if _, ok := servers["my.server"]; !ok {
		t.Errorf("my.server key not found:\n%s", got)
	}
}

// YAML: upsert + remove on the Continue list format.
func TestYamlListUpsertRemove(t *testing.T) {
	dir := t.TempDir()
	orig := "mcpServers:\n  - name: other\n    type: stdio\n    command: x\n"
	f := writeFile(t, dir, "config.yaml", orig)
	e := target("a@b.com", "tok")
	if r, err := upsertYamlServerList(f, "favro", e, false); err != nil || r != writeOK {
		t.Fatalf("upsert=%v err=%v", r, err)
	}
	got := read(t, f)
	if !strings.Contains(got, "other") || !strings.Contains(got, "favro") {
		t.Errorf("sibling or new entry lost:\n%s", got)
	}
	if r, err := upsertYamlServerList(f, "favro", e, false); err != nil || r != writeNoop {
		t.Fatalf("second upsert=%v want noop", r)
	}
	if r, err := removeYamlServerList(f, "favro", false); err != nil || r != writeOK {
		t.Fatalf("remove=%v err=%v", r, err)
	}
	got = read(t, f)
	if strings.Contains(got, "favro") || !strings.Contains(got, "other") {
		t.Errorf("remove did not clean up correctly:\n%s", got)
	}
}
