package mcpserver

import (
	"context"
	"fmt"

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
	out := make([]userRow, 0, len(users))
	for _, u := range users {
		role := ""
		if u.OrganizationRole != nil {
			role = *u.OrganizationRole
		}
		out = append(out, userRow{Name: u.Name, ID: u.UserID, Email: u.Email, Role: role})
	}
	return textResult(rendered{front: listUsersFront{Users: out}, body: fmt.Sprintf("%d user(s).", len(out))}.String())
}

type listUsersFront struct {
	Users []userRow `yaml:"users"`
}

type userRow struct {
	Name  string `yaml:"name"`
	ID    string `yaml:"id"`
	Email string `yaml:"email,omitempty"`
	Role  string `yaml:"role,omitempty"`
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
	return textResult(rendered{
		front: listUsersFront{Users: []userRow{{Name: u.Name, ID: u.UserID, Email: u.Email, Role: role}}},
		body:  fmt.Sprintf("User **%s** (%s).", u.Name, u.Email),
	}.String())
}
