package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the server version (overridable via -ldflags at build time).
var Version = "0.7.0"

// Toolset tiers. They are cumulative: read ⊂ write ⊂ delete.
const (
	TierRead   = "read"
	TierWrite  = "write"
	TierDelete = "delete"
)

var tierRank = map[string]int{TierRead: 1, TierWrite: 2, TierDelete: 3}

// normalizeToolset returns a valid tier (default TierWrite) for the given value.
func normalizeToolset(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case TierRead, "r", "ro":
		return TierRead
	case TierDelete, "d", "all":
		return TierDelete
	case TierWrite, "w", "rw":
		return TierWrite
	default:
		return TierWrite
	}
}

// Server holds session state and the selected toolset, and registers tools.
type Server struct {
	session  *Session
	toolset  string          // used when allowlist is empty
	allowlist map[string]bool // non-empty => custom tool selection (FAVRO_TOOLS)
}

func NewServer() *Server {
	s := &Server{session: NewSession()}
	if tools := strings.TrimSpace(os.Getenv("FAVRO_TOOLS")); tools != "" {
		s.allowlist = map[string]bool{}
		for _, name := range strings.Split(tools, ",") {
			if n := strings.TrimSpace(name); n != "" {
				s.allowlist[n] = true
			}
		}
	} else {
		s.toolset = normalizeToolset(os.Getenv("FAVRO_TOOLSET"))
	}
	return s
}

// toolEnabled reports whether a tool (by name and tier) should be registered.
// An explicit allowlist (custom toolset) wins; otherwise the cumulative tier
// applies (read < write < delete).
func (s *Server) toolEnabled(name, tier string) bool {
	if len(s.allowlist) > 0 {
		return s.allowlist[name]
	}
	return tierRank[s.toolset] >= tierRank[tier]
}

// jsonResult serialises a value as indented JSON text content. Passing a
// non-nil error short-circuits to an error result.
func jsonResult(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	b, mErr := json.MarshalIndent(v, "", "  ")
	if mErr != nil {
		return nil, nil, fmt.Errorf("failed to encode result: %w", mErr)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}

// mc constructs the *mcp.Server and registers every tool.
func (s *Server) mc() *mcp.Server {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "favro-mcp", Version: Version},
		&mcp.ServerOptions{
			Instructions: "MCP server for Favro project management. Use set_organization/select first, then operate on boards and cards.",
		},
	)
	s.registerTools(srv)
	return srv
}

// Run starts the server on stdio.
func (s *Server) Run(ctx context.Context) error {
	return s.mc().Run(ctx, &mcp.StdioTransport{})
}
