package install

import (
	"os"
	"path/filepath"
)

// serverTarget builds the command an MCP client runs to start this server:
// the absolute path to the running executable. Credentials are passed in env.
func serverTarget(env map[string]string) (ServerTarget, error) {
	exe, err := os.Executable()
	if err != nil {
		return ServerTarget{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return ServerTarget{Command: exe, Env: env}, nil
}
