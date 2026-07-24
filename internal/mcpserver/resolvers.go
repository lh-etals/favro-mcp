package mcpserver

import (
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/favro/resolver"
)

// Resolver, its error types, and the helpers around them live in
// internal/favro/resolver so both the MCP server and the CLI can share them
// without one depending on the other. The declarations below are thin aliases
// / wrappers that keep the existing mcpserver API working.

// Exported aliases (stable, public-ish surface used by callers and tests).
type Resolver = resolver.Resolver
type NotFound = resolver.NotFound
type Ambiguous = resolver.Ambiguous

// Unexported aliases kept so classifyError (errors.As against *notFoundError /
// *ambiguousError) and existing struct-literal type references keep working.
type notFoundError = resolver.NotFound
type ambiguousError = resolver.Ambiguous

// NewResolver constructs a Resolver backed by the given Favro client.
func NewResolver(c *favro.Client) *Resolver { return resolver.New(c) }

// IsResolveMiss reports whether err means "this ID is unusable; try by name".
func IsResolveMiss(err error) bool { return resolver.IsMiss(err) }

func isResolveMiss(err error) bool { return resolver.IsMiss(err) }
func isNotFound(err error) bool    { return resolver.IsNotFound(err) }

func parseSequentialID(identifier string) (int, bool) {
	return resolver.ParseSequentialID(identifier)
}

// matchByName is a thin generic wrapper over resolver.MatchByName so existing
// callers/tests in this package keep type-checking.
func matchByName[T any](items []T, getID func(T) string, getName func(T) string, entityType, identifier string) (*T, error) {
	return resolver.MatchByName(items, getID, getName, entityType, identifier)
}

// newNotFoundError / newAmbiguousError construct the (aliased) resolver errors
// for call sites that previously built struct literals directly.
func newNotFoundError(entityType, identifier string) *notFoundError {
	return resolver.NewNotFound(entityType, identifier)
}

func newAmbiguousError(entityType, name string, matches []resolver.MatchInfo) *ambiguousError {
	return resolver.NewAmbiguous(entityType, name, matches)
}
