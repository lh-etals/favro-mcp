package install

import (
	"encoding/json"
	"path/filepath"
	"runtime"
)

// ServerTarget is the command an MCP client runs to start this server.
type ServerTarget struct {
	Command string
	Args    []string
	Env     map[string]string
}

// InstallKind describes how a client's config is written.
type InstallKind struct {
	Kind     string // "file-json" | "file-toml" | "file-yaml-list" | "command"
	pathFn   func() string
	TopKey   string // for file-json
	entryFn  func(ServerTarget) map[string]any // custom file-json entry shape (nil = default)
	Bin      string
	resolveBin func() string // full path to the CLI (PATH + install dirs); falls back to Bin
	buildArgs func(name string, e ServerTarget) []string
	removeArgs func(name string) []string
}

// ClientDef is one supported MCP client.
type ClientDef struct {
	ID         string
	Name       string
	Detect     func() bool
	Install    InstallKind
	ReloadHint string
}

// --- per-client config paths ----------------------------------------------

func claudeDesktopConfig() string {
	if runtime.GOOS == "darwin" {
		return appSupport("Claude", "claude_desktop_config.json")
	}
	if runtime.GOOS == "windows" {
		a := appData()
		if a == "" {
			return ""
		}
		return filepath.Join(a, "Claude", "claude_desktop_config.json")
	}
	return "" // no official Linux build
}

func cursorConfig() string      { return home(".cursor", "mcp.json") }
func codexConfig() string       { return home(".codex", "config.toml") }
func geminiConfig() string      { return home(".gemini", "settings.json") }
func windsurfConfig() string    { return home(".codeium", "windsurf", "mcp_config.json") }
func clineConfig() string       { return home(".cline", "data", "settings", "cline_mcp_settings.json") }

func amazonQConfig() string   { return home(".aws", "amazonq", "mcp.json") }
func opencodeConfig() string  { return xdgConfig("opencode", "opencode.json") }
func continueConfig() string { return home(".continue", "config.yaml") }

func rooConfig() string {
	var base string
	switch runtime.GOOS {
	case "darwin":
		base = appSupport("Code", "User", "globalStorage")
	case "windows":
		a := appData()
		if a == "" {
			return ""
		}
		base = filepath.Join(a, "Code", "User", "globalStorage")
	default:
		base = xdgConfig("Code", "User", "globalStorage")
	}
	return filepath.Join(base, "rooveterinaryinc.roo-cline", "mcp_settings.json")
}

func zedConfig() string {
	if runtime.GOOS == "darwin" {
		return appSupport("zed", "settings.json")
	}
	if runtime.GOOS == "windows" {
		a := appData()
		if a == "" {
			return ""
		}
		return filepath.Join(a, "Zed", "settings.json")
	}
	return xdgConfig("zed", "settings.json")
}

// --- the registry ----------------------------------------------------------

// Clients is the ordered registry of supported MCP clients.
var Clients = []ClientDef{
	{
		ID:   "claude-desktop",
		Name: "Claude Desktop",
		Detect: func() bool {
			lad := localAppData()
			return exists(claudeDesktopConfig()) ||
				appBundle("Claude.app") ||
				(lad != "" && exists(filepath.Join(lad, "AnthropicClaude")))
		},
		Install:    InstallKind{Kind: "file-json", pathFn: claudeDesktopConfig, TopKey: "mcpServers"},
		ReloadHint: "quit and restart Claude Desktop",
	},
	{
		ID:   "claude-code",
		Name: "Claude Code (CLI)",
		Detect: func() bool {
			// PATH, the config dir/file, OR an install not on PATH.
			return whichIn("claude", claudeBins()...) != "" ||
				exists(home(".claude.json")) || exists(home(".claude"))
		},
		Install: InstallKind{
			Kind:       "command",
			Bin:        "claude",
			resolveBin: func() string { return whichIn("claude", claudeBins()...) },
			buildArgs: func(name string, e ServerTarget) []string {
				args := []string{"mcp", "add", name, "-s", "user"}
				args = append(args, envFlagArgs(e)...)
				args = append(args, "--", e.Command)
				args = append(args, e.Args...)
				return args
			},
			removeArgs: func(name string) []string { return []string{"mcp", "remove", name, "-s", "user"} },
		},
		ReloadHint: "restart Claude Code sessions",
	},
	{
		ID:   "cursor",
		Name: "Cursor",
		Detect: func() bool {
			lad := localAppData()
			return exists(home(".cursor")) ||
				appBundle("Cursor.app") ||
				(lad != "" && exists(filepath.Join(lad, "Programs", "cursor")))
		},
		Install:    InstallKind{Kind: "file-json", pathFn: cursorConfig, TopKey: "mcpServers"},
		ReloadHint: "restart Cursor",
	},
	{
		ID:   "codex",
		Name: "Codex CLI",
		Detect: func() bool {
			return which("codex") != "" || exists(home(".codex"))
		},
		Install:    InstallKind{Kind: "file-toml", pathFn: codexConfig},
		ReloadHint: "restart Codex sessions",
	},
	{
		ID:   "gemini-cli",
		Name: "Gemini CLI",
		Detect: func() bool {
			return which("gemini") != "" || exists(home(".gemini"))
		},
		Install:    InstallKind{Kind: "file-json", pathFn: geminiConfig, TopKey: "mcpServers"},
		ReloadHint: "restart Gemini CLI sessions",
	},
	{
		ID:   "windsurf",
		Name: "Windsurf",
		Detect: func() bool {
			lad := localAppData()
			return exists(home(".codeium", "windsurf")) ||
				appBundle("Windsurf.app") ||
				(lad != "" && exists(filepath.Join(lad, "Programs", "Windsurf")))
		},
		Install:    InstallKind{Kind: "file-json", pathFn: windsurfConfig, TopKey: "mcpServers"},
		ReloadHint: "Windsurf reloads the affected server automatically",
	},
	{
		ID:   "zed",
		Name: "Zed",
		Detect: func() bool {
			return which("zed") != "" || exists(zedConfig()) || appBundle("Zed.app")
		},
		Install:    InstallKind{Kind: "file-json", pathFn: zedConfig, TopKey: "context_servers"},
		ReloadHint: "Zed reloads settings automatically",
	},
	{
		ID:   "cline",
		Name: "Cline",
		Detect: func() bool {
			return exists(home(".cline")) || hasVscodeExt("saoudrizwan.claude-dev-")
		},
		Install:    InstallKind{Kind: "file-json", pathFn: clineConfig, TopKey: "mcpServers"},
		ReloadHint: "restart the server in Cline's MCP panel",
	},
	{
		ID:   "roo-code",
		Name: "Roo Code",
		Detect: func() bool {
			return exists(rooConfig()) || hasVscodeExt("rooveterinaryinc.roo-cline-")
		},
		Install:    InstallKind{Kind: "file-json", pathFn: rooConfig, TopKey: "mcpServers"},
		ReloadHint: "restart the server in Roo's MCP panel",
	},
	{
		ID:   "amazon-q",
		Name: "Amazon Q Developer CLI",
		Detect: func() bool {
			return which("q") != "" || which("qchat") != "" || exists(home(".aws", "amazonq"))
		},
		Install:    InstallKind{Kind: "file-json", pathFn: amazonQConfig, TopKey: "mcpServers"},
		ReloadHint: "restart Amazon Q sessions",
	},
	{
		ID:   "continue",
		Name: "Continue",
		Detect: func() bool {
			return exists(home(".continue")) || hasVscodeExt("continue.continue-")
		},
		Install:    InstallKind{Kind: "file-yaml-list", pathFn: continueConfig},
		ReloadHint: "reload Continue config",
	},
	{
		ID:   "opencode",
		Name: "OpenCode",
		Detect: func() bool {
			return whichIn("opencode", npmGlobalBins()...) != "" ||
				exists(xdgConfig("opencode")) || exists(home(".config", "opencode"))
		},
		Install: InstallKind{
			Kind:    "file-json",
			pathFn:  opencodeConfig,
			TopKey:  "mcp",
			entryFn: opencodeEntry,
		},
		ReloadHint: "restart OpenCode",
	},
	{
		ID:   "vscode",
		Name: "VS Code",
		Detect: func() bool {
			return whichIn("code", vscodeBins()...) != "" ||
				appBundle("Visual Studio Code.app") ||
				appBundle("Visual Studio Code - Insiders.app") ||
				hasVscodeExt("ms-vscode.cpptools") // VS Code family present if any ext dir exists-ish
		},
		Install: InstallKind{
			Kind:       "command",
			Bin:        "code",
			resolveBin: func() string { return whichIn("code", vscodeBins()...) },
			buildArgs: func(name string, e ServerTarget) []string {
				entry := map[string]any{"name": name, "type": "stdio", "command": e.Command}
				if len(e.Args) > 0 {
					entry["args"] = e.Args
				}
				if len(e.Env) > 0 {
					entry["env"] = e.Env
				}
				b, _ := json.Marshal(entry)
				return []string{"--add-mcp", string(b)}
			},
		},
		ReloadHint: "reload the VS Code window (MCP: List Servers)",
	},
}

func (k InstallKind) path() string {
	if k.pathFn == nil {
		return ""
	}
	return k.pathFn()
}
