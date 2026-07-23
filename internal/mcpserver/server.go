package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the server version (overridable via -ldflags at build time).
var Version = "0.7.0"

// Server holds session state and registers all Favro MCP tools.
type Server struct {
	session *Session
}

func NewServer() *Server { return &Server{session: NewSession()} }

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
