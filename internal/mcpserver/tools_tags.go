package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTags(srv *mcp.Server, s *Server) {
	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "list_tags",
		Description: "List all tags in the organization with their IDs, names, and colors. Use the tag name or ID with tag_card to add/remove tags from cards.",
	}, s.listTags)
}

func (s *Server) listTags(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	tags, err := client.GetTags()
	if err != nil {
		return jsonResult(nil, err)
	}
	out := make([]map[string]any, 0, len(tags))
	for _, t := range tags {
		color := ""
		if t.Color != nil {
			color = *t.Color
		}
		out = append(out, map[string]any{
			"tag_id": t.TagID,
			"name":   t.Name,
			"color":  color,
		})
	}
	return jsonResult(map[string]any{"tags": out, "count": len(out)}, nil)
}
