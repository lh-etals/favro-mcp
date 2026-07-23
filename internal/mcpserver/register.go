package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// catalogCapture, when non-nil, makes addTool record each tool into it instead
// of registering. Used by ToolCatalog() to enumerate every tool from the single
// source of truth (the register* calls), avoiding a second hand-maintained list.
var catalogCapture []ToolInfo

// addTool registers a tool only if the server's toolset includes the given tier
// (or it is in the explicit allowlist). Tiers are cumulative
// (read < write < delete). When catalogCapture is set, it records the tool and
// does not touch srv (which may be nil).
func addTool[In any](
	s *Server,
	srv *mcp.Server,
	tier string,
	t *mcp.Tool,
	h func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error),
) {
	if catalogCapture != nil {
		catalogCapture = append(catalogCapture, ToolInfo{Name: t.Name, Tier: tier, Description: t.Description})
		return
	}
	if !s.toolEnabled(t.Name, tier) {
		return
	}
	mcp.AddTool(srv, t, h)
}

// registerTools wires every Favro tool onto the server, gated by the toolset.
func (s *Server) registerTools(srv *mcp.Server) {
	registerOrganizations(srv, s)
	registerCollections(srv, s)
	registerBoards(srv, s)
	registerColumns(srv, s)
	registerLanes(srv, s)
	registerTags(srv, s)
	registerUsers(srv, s)
	registerCards(srv, s)
}
