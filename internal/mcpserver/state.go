package mcpserver

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
)

// State holds the mutable session selection (active organization and board).
// An MCP stdio server serves a single client, so one shared value is enough.
type State struct {
	mu             sync.Mutex
	currentOrgID   string
	currentBoardID string
}

func (s *State) Org() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentOrgID
}

func (s *State) SetOrg(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentOrgID = id
}

func (s *State) Board() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentBoardID
}

func (s *State) SetBoard(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentBoardID = id
}

// Session wires session state to the Favro client.
type Session struct {
	state *State
}

func NewSession() *Session {
	return &Session{state: &State{}}
}

// Delegating accessors keep tool code readable.
func (s *Session) Org() string              { return s.state.Org() }
func (s *Session) SetOrg(id string)         { s.state.SetOrg(id) }
func (s *Session) Board() string            { return s.state.Board() }
func (s *Session) SetBoard(id string)       { s.state.SetBoard(id) }
func (s *Session) effectiveBoard(board string) string {
	if board != "" {
		return board
	}
	return s.state.Board()
}

// newClient builds a Favro client from credentials: FAVRO_EMAIL/FAVRO_API_TOKEN
// env vars take precedence, otherwise the central store written by
// `favro-mcp login` is used.
func (s *Session) newClient() (*favro.Client, error) {
	email := os.Getenv("FAVRO_EMAIL")
	token := os.Getenv("FAVRO_API_TOKEN")
	if email == "" || token == "" {
		if e, t, err := credentials.Load(); err == nil {
			if email == "" {
				email = e
			}
			if token == "" {
				token = t
			}
		}
	}
	if email == "" || token == "" {
		return nil, errors.New("Favro credentials not configured. Run `favro-mcp login`, or set FAVRO_EMAIL and FAVRO_API_TOKEN")
	}
	return favro.NewClient(email, token, s.state.Org()), nil
}

// requireOrg returns the active org ID, auto-selecting when the account has
// exactly one organization. Mirrors the Python require_org() behaviour.
func (s *Session) requireOrg() (string, error) {
	if id := s.state.Org(); id != "" {
		return id, nil
	}
	client, err := s.newClient()
	if err != nil {
		return "", err
	}
	orgs, err := client.GetOrganizations()
	if err != nil {
		return "", err
	}
	if len(orgs) == 0 {
		return "", errors.New("no organizations found for this account")
	}
	if len(orgs) == 1 {
		s.state.SetOrg(orgs[0].OrganizationID)
		return orgs[0].OrganizationID, nil
	}
	names := make([]string, 0, len(orgs))
	for _, o := range orgs {
		names = append(names, o.Name)
	}
	return "", fmt.Errorf(
		"multiple organizations available (%s). Use the set_organization tool to select one, or call list_organizations to see them",
		strings.Join(names, ", "),
	)
}
