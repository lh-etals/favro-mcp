package app

import (
	"fmt"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/favro/resolver"
)

// Session holds the app's active scope (organization + board) on top of the
// user's stored credentials, and provides client/resolver accessors that
// automatically carry that scope.
type Session struct {
	email         string
	token         string
	activeOrgID   string
	activeBoardID string
}

// NewSession loads stored credentials. It returns an error if the user is not
// logged in (no credentials file).
func NewSession() (*Session, error) {
	email, token, err := credentials.Load()
	if err != nil {
		return nil, fmt.Errorf("not logged in: %w", err)
	}
	return &Session{email: email, token: token}, nil
}

// Email returns the authenticated user's email.
func (s *Session) Email() string { return s.email }

// Client returns a favro.Client bound to the currently active organization
// (empty if none has been chosen yet).
func (s *Session) Client() *favro.Client {
	return favro.NewClient(s.email, s.token, s.activeOrgID)
}

// Resolver returns a resolver wrapping the session's client.
func (s *Session) Resolver() *resolver.Resolver { return resolver.New(s.Client()) }

// RequireOrg returns the active organization ID, auto-selecting when the
// account has exactly one organization. It returns an error when no org is set
// and the account has zero or more than one organization.
func (s *Session) RequireOrg() (string, error) {
	if s.activeOrgID != "" {
		return s.activeOrgID, nil
	}
	orgs, err := s.Client().GetOrganizations()
	if err != nil {
		return "", err
	}
	if len(orgs) == 1 {
		s.activeOrgID = orgs[0].OrganizationID
		return s.activeOrgID, nil
	}
	if len(orgs) == 0 {
		return "", fmt.Errorf("your account has no organizations")
	}
	return "", fmt.Errorf("multiple organizations available; select one first")
}

func (s *Session) SetOrg(id string)   { s.activeOrgID = id }
func (s *Session) SetBoard(id string) { s.activeBoardID = id }
func (s *Session) BoardID() string    { return s.activeBoardID }
func (s *Session) OrgID() string      { return s.activeOrgID }
