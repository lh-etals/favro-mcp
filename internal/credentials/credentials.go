// Package credentials stores the user's Favro API credentials in a single
// 0o600 file under ~/.favro-mcp/credentials.json, written by `favro-mcp login`
// and read by the server at runtime (env vars override it).
package credentials

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

type stored struct {
	Email string `json:"email"`
	Token string `json:"api_token"`
}

// Dir returns the per-user data directory (~/.favro-mcp).
func Dir() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".favro-mcp"), nil
}

// Path returns the credentials file location.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "credentials.json"), nil
}

// Exists reports whether a credentials file is present.
func Exists() bool {
	p, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Load reads the stored credentials. A missing file is a non-nil error.
func Load() (email, token string, err error) {
	p, err := Path()
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", "", err
	}
	var s stored
	if err := json.Unmarshal(b, &s); err != nil {
		return "", "", fmt.Errorf("invalid credentials file: %w", err)
	}
	return s.Email, s.Token, nil
}

// Save writes the credentials with mode 0o600 (dir 0o700).
func Save(email, token string) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(stored{Email: email, Token: token}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// Prompt interactively asks for the Favro email and API token (input hidden
// when on a TTY) and returns them. It does NOT save - the caller verifies
// against the API first, then calls Save only on success.
func Prompt() (email, token string, err error) {
	r := bufio.NewReader(os.Stdin)
	fmt.Print("Favro email: ")
	line, _ := r.ReadString('\n')
	email = strings.TrimSpace(line)

	fmt.Print("Favro API token (input hidden): ")
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		if err != nil {
			return "", "", err
		}
		token = strings.TrimSpace(string(b))
		fmt.Println()
	} else {
		line, _ := r.ReadString('\n')
		token = strings.TrimSpace(line)
	}
	if email == "" || token == "" {
		return "", "", fmt.Errorf("email and token are required")
	}
	return email, token, nil
}
