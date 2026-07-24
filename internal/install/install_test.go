package install

import (
	"os"
	"path/filepath"
	"testing"
)

// Builds a ClientDef whose config path points at `file`, for the given kind.
func def(kind, file, topKey string) ClientDef {
	return ClientDef{
		ID:   "test-" + kind,
		Name: "Test " + kind,
		Install: InstallKind{
			Kind:   kind,
			TopKey: topKey,
			pathFn: func() string { return file },
		},
	}
}

func writeTemp(t *testing.T, dir, name, content string) string {
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

// JSON: present/absent/missing-file all reported correctly.
func TestIsRegisteredJSON(t *testing.T) {
	dir := t.TempDir()
	present := writeTemp(t, dir, "has.json", `{"mcpServers":{"favro":{"command":"x"}}}`)
	absent := writeTemp(t, dir, "none.json", `{"mcpServers":{"other":{"command":"x"}}}`)

	if !isCurrentlyRegistered(def("file-json", present, "mcpServers"), "favro") {
		t.Error("expected favro to be registered in has.json")
	}
	if isCurrentlyRegistered(def("file-json", absent, "mcpServers"), "favro") {
		t.Error("favro should not be reported as registered in none.json")
	}
	// Missing file -> not registered (no panic).
	if isCurrentlyRegistered(def("file-json", filepath.Join(dir, "nope.json"), "mcpServers"), "favro") {
		t.Error("missing file should report not registered")
	}
}

// JSON: BOM-prefixed file still detects registration.
func TestIsRegisteredJSONBOM(t *testing.T) {
	dir := t.TempDir()
	f := writeTemp(t, dir, "bom.json", "\xEF\xBB\xBF"+`{"mcpServers":{"favro":{"command":"x"}}}`)
	if !isCurrentlyRegistered(def("file-json", f, "mcpServers"), "favro") {
		t.Error("BOM-prefixed file should still detect registration")
	}
}

// TOML: present/absent reported correctly.
func TestIsRegisteredTOML(t *testing.T) {
	dir := t.TempDir()
	present := writeTemp(t, dir, "has.toml", "[mcp_servers.favro]\ncommand = \"x\"\n")
	absent := writeTemp(t, dir, "none.toml", "[mcp_servers.other]\ncommand = \"x\"\n")

	if !isCurrentlyRegistered(def("file-toml", present, ""), "favro") {
		t.Error("expected favro registered in has.toml")
	}
	if isCurrentlyRegistered(def("file-toml", absent, ""), "favro") {
		t.Error("favro should not be reported as registered in none.toml")
	}
}

// YAML list: present/absent reported correctly (Continue shape).
func TestIsRegisteredYAMLList(t *testing.T) {
	dir := t.TempDir()
	present := writeTemp(t, dir, "has.yaml", "mcpServers:\n  - name: favro\n    command: x\n  - name: other\n    command: y\n")
	absent := writeTemp(t, dir, "none.yaml", "mcpServers:\n  - name: other\n    command: y\n")

	if !isCurrentlyRegistered(def("file-yaml-list", present, ""), "favro") {
		t.Error("expected favro registered in has.yaml")
	}
	if isCurrentlyRegistered(def("file-yaml-list", absent, ""), "favro") {
		t.Error("favro should not be reported as registered in none.yaml")
	}
}

// Command kind: assume registered (can't introspect). This guarantees a
// deselected detected command-client still gets a removal attempt.
func TestIsRegisteredCommandAssumesTrue(t *testing.T) {
	c := ClientDef{Install: InstallKind{Kind: "command"}}
	if !isCurrentlyRegistered(c, "favro") {
		t.Error("command kind should assume registered so removal is attempted")
	}
}

// Unknown kind: never reported as registered.
func TestIsRegisteredUnknownKind(t *testing.T) {
	c := ClientDef{Install: InstallKind{Kind: "something-else"}}
	if isCurrentlyRegistered(c, "favro") {
		t.Error("unknown kind should not be reported as registered")
	}
}
