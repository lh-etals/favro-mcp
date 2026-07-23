package mcpserver

import (
	"context"
	"fmt"

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
	rows := make([]orgRow, 0, len(orgs))
	for _, o := range orgs {
		rows = append(rows, orgRow{Name: o.Name, ID: o.OrganizationID, Members: len(o.SharedToUsers)})
	}
	return textResult(rendered{front: listOrgsFront{Organizations: rows}, body: fmt.Sprintf("%d organization(s).", len(rows))}.String())
}

// listOrgsFront is the ordered frontmatter for list_organizations.
type listOrgsFront struct {
	Organizations []orgRow `yaml:"organizations"`
}

type orgRow struct {
	Name    string `yaml:"name"`
	ID      string `yaml:"id"`
	Members int    `yaml:"members,omitempty"`
}

func (s *Server) getCurrentOrganization(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	if s.session.Org() == "" {
		return textResult("No organization selected. Use the `set_organization` tool first.")
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	org, err := client.GetOrganization(s.session.Org())
	if err != nil {
		return jsonResult(nil, err)
	}
	return textResult(rendered{front: listOrgsFront{Organizations: []orgRow{{Name: org.Name, ID: org.OrganizationID, Members: len(org.SharedToUsers)}}}, body: fmt.Sprintf("Active organization: **%s**.", org.Name)}.String())
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
	return textResult(mdMessage(fmt.Sprintf("Selected organization **%s**.", org.Name), map[string]any{"organization_id": org.OrganizationID}))
}
