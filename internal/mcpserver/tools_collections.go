package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCollections(srv *mcp.Server, s *Server) {
	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "list_collections",
		Description: "List all collections (folders) in the organization. Collections are folders that contain boards. If you are looking for a board but cannot find it with list_boards, it may be inside a collection. Use the collection name or ID with list_boards(collection=...) to see boards inside that collection.",
	}, s.listCollections)
}

func (s *Server) listCollections(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	collections, err := client.GetCollections(false)
	if err != nil {
		return jsonResult(nil, err)
	}
	out := make([]map[string]any, 0, len(collections))
	for _, c := range collections {
		out = append(out, map[string]any{
			"collection_id": c.CollectionID,
			"name":          c.Name,
			"archived":      c.Archived,
		})
	}
	return jsonResult(map[string]any{"collections": out}, nil)
}
