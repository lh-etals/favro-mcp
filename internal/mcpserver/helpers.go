package mcpserver

import (
	"errors"
	"strconv"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

var errNoBoard = errors.New("no board specified and no current board selected")

// strOr dereferences a *string, returning "" for nil.
func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func itoa(n int) string { return strconv.Itoa(n) }

// client builds a Favro client from env + active org.
func (s *Server) client() (*favro.Client, error) {
	return s.session.newClient()
}

// requireOrg returns the active org ID (auto-selecting when there is exactly one).
func (s *Server) requireOrg() (string, error) {
	return s.session.requireOrg()
}

// resolveBoardArg returns the board ID to operate on: an explicitly provided
// board (resolved by ID/name), or the current board when none is given.
func (s *Server) resolveBoardArg(client *favro.Client, board string) (string, error) {
	bid := s.session.effectiveBoard(board)
	if board != "" {
		w, err := NewResolver(client).Board(board)
		if err != nil {
			return "", err
		}
		bid = w.WidgetCommonID
	}
	return bid, nil
}
