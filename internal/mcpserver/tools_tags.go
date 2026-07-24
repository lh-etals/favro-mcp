package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTags(srv *mcp.Server, s *Server) {
	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "list_tags",
		Description: "List all tags in the organization with their IDs, names, and colors. Use the tag name or ID with tag_card to add/remove tags from cards on Favro.",
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
	out := make([]tagRow, 0, len(tags))
	for _, t := range tags {
		color := ""
		if t.Color != nil {
			color = *t.Color
		}
		out = append(out, tagRow{Name: t.Name, ID: t.TagID, Color: color})
	}
	return textResult(rendered{front: listTagsFront{Tags: out}, body: fmt.Sprintf("%d tag(s).", len(out))}.String())
}

type listTagsFront struct {
	Tags []tagRow `yaml:"tags"`
}

type tagRow struct {
	Name  string `yaml:"name"`
	ID    string `yaml:"id"`
	Color string `yaml:"color,omitempty"`
}
