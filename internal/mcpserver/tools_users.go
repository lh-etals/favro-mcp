package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerUsers(srv *mcp.Server, s *Server) {
	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "list_users",
		Description: "List all users in the current organization with their IDs, names, emails, and roles.",
	}, s.listUsers)
	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "get_user",
		Description: "Look up a user by ID, name, or email address. Useful for resolving user IDs returned in card details (assignments, comments) to human-readable user information.",
	}, s.getUser)
}

func (s *Server) listUsers(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	users, err := client.GetUsers()
	if err != nil {
		return jsonResult(nil, err)
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		role := ""
		if u.OrganizationRole != nil {
			role = *u.OrganizationRole
		}
		out = append(out, map[string]any{
			"user_id":            u.UserID,
			"name":               u.Name,
			"email":              u.Email,
			"organization_role":  role,
		})
	}
	return jsonResult(map[string]any{"users": out, "count": len(out)}, nil)
}

type getUserArgs struct {
	User string `json:"user" jsonschema:"User ID, name, or email address"`
}

func (s *Server) getUser(_ context.Context, _ *mcp.CallToolRequest, args getUserArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	u, err := NewResolver(client).User(args.User)
	if err != nil {
		return jsonResult(nil, err)
	}
	role := ""
	if u.OrganizationRole != nil {
		role = *u.OrganizationRole
	}
	return jsonResult(map[string]any{
		"user_id":            u.UserID,
		"name":               u.Name,
		"email":              u.Email,
		"organization_role":  role,
	}, nil)
}
