// Package install detects installed MCP-capable clients and registers this
// server with the ones the user chooses. Ported from sana-mcp's install module
// (clean-room: paths/formats/detection are public facts).
package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// --- path roots ------------------------------------------------------------

func home(parts ...string) string {
	h, err := os.UserHomeDir()
	if err != nil {
		h = "."
	}
	return filepath.Join(append([]string{h}, parts...)...)
}

// appSupport: macOS ~/Library/Application Support/...
func appSupport(parts ...string) string {
	return home(append([]string{"Library", "Application Support"}, parts...)...)
}

// appData: Windows %APPDATA% (Roaming); "" off-Windows or if unset.
func appData() string {
	if v := os.Getenv("APPDATA"); v != "" {
		return v
	}
	return ""
}

// localAppData: Windows %LOCALAPPDATA%; "" off-Windows or if unset.
func localAppData() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	return ""
}

// xdgConfig: XDG config dir (Linux/BSD), defaults to ~/.config.
func xdgConfig(parts ...string) string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = home(".config")
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

// --- presence --------------------------------------------------------------

func exists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// which resolves a binary on PATH (which on unix, where on Windows). Returns
// the absolute path or "" if not found.
func which(bin string) string {
	_, err := exec.LookPath(bin)
	if err == nil {
		if p, err := exec.LookPath(bin); err == nil {
			return p
		}
	}
	return ""
}

// appBundle reports whether a macOS .app bundle exists in /Applications.
func appBundle(name string) bool {
	return runtime.GOOS == "darwin" && exists(filepath.Join("/Applications", name))
}

// hasVscodeExt reports whether a VS Code-family extension with the given ID
// prefix is installed in any known extensions directory.
func hasVscodeExt(prefix string) bool {
	dirs := []string{
		home(".vscode", "extensions"),
		home(".vscode-insiders", "extensions"),
		home(".cursor", "extensions"),
		home(".windsurf", "extensions"),
		home(".vscodium", "extensions"),
	}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), prefix) {
				return true
			}
		}
	}
	return false
}
