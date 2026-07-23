package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type noArgs struct{}

func registerOrganizations(srv *mcp.Server, s *Server) {
	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "list_organizations",
		Description: "List all organizations accessible to the authenticated user. Returns each organization's ID, name, and member count. Use set_organization to select one as the active organization.",
	}, s.listOrganizations)

	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "get_current_organization",
		Description: "Get details of the currently selected organization. Returns a message if none is selected.",
	}, s.getCurrentOrganization)

	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "set_organization",
		Description: "Select an organization (by ID or name) as the active organization for subsequent operations.",
	}, s.setOrganization)
}

func (s *Server) listOrganizations(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	orgs, err := client.GetOrganizations()
	if err != nil {
		return jsonResult(nil, err)
	}
	out := make([]map[string]any, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, map[string]any{
			"organization_id": o.OrganizationID,
			"name":            o.Name,
			"member_count":    len(o.SharedToUsers),
		})
	}
	return jsonResult(map[string]any{"organizations": out}, nil)
}

func (s *Server) getCurrentOrganization(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	if s.session.Org() == "" {
		return jsonResult(map[string]any{"message": "No organization selected. Use set_organization tool first."}, nil)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	org, err := client.GetOrganization(s.session.Org())
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"organization_id": org.OrganizationID,
		"name":            org.Name,
		"member_count":    len(org.SharedToUsers),
	}, nil)
}

type setOrganizationArgs struct {
	Org string `json:"org" jsonschema:"Organization ID or name"`
}

func (s *Server) setOrganization(_ context.Context, _ *mcp.CallToolRequest, args setOrganizationArgs) (*mcp.CallToolResult, any, error) {
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	org, err := NewResolver(client).Organization(args.Org)
	if err != nil {
		return jsonResult(nil, err)
	}
	s.session.SetOrg(org.OrganizationID)
	return jsonResult(map[string]any{
		"message":         "Selected organization: " + org.Name,
		"organization_id": org.OrganizationID,
		"name":            org.Name,
	}, nil)
}
