package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools wires every Favro tool onto the server.
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
