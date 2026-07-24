// Package resolver resolves Favro entity IDs/names (and sequential card IDs)
// to concrete favro model objects. It is shared by both the MCP server and the
// CLI so neither needs to depend on the other.
package resolver

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// Resolver errors -----------------------------------------------------------

// NotFound is returned when an entity cannot be located by ID or name.
type NotFound struct {
	entityType string
	identifier string
}

// NewNotFound constructs a NotFound error.
func NewNotFound(entityType, identifier string) *NotFound {
	return &NotFound{entityType: entityType, identifier: identifier}
}

func (e *NotFound) Error() string {
	return fmt.Sprintf("%s not found: %s", e.entityType, e.identifier)
}

// MatchInfo describes a single candidate when several entities share a name.
type MatchInfo struct {
	id   string
	desc string
}

// NewMatchInfo constructs a MatchInfo.
func NewMatchInfo(id, desc string) MatchInfo { return MatchInfo{id: id, desc: desc} }

// Ambiguous is returned when multiple entities match a name.
type Ambiguous struct {
	entityType string
	name       string
	matches    []MatchInfo
}

// NewAmbiguous constructs an Ambiguous error.
func NewAmbiguous(entityType, name string, matches []MatchInfo) *Ambiguous {
	return &Ambiguous{entityType: entityType, name: name, matches: matches}
}

func (e *Ambiguous) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "multiple %ss match '%s':", e.entityType, e.name)
	for _, m := range e.matches {
		fmt.Fprintf(&b, "\n  - %s: %s", m.id, m.desc)
	}
	b.WriteString("\nPlease use one of the IDs above instead.")
	return b.String()
}

// IsNotFound reports whether err is a Favro NotFoundError.
func IsNotFound(err error) bool {
	var nfe *favro.NotFoundError
	return errors.As(err, &nfe)
}

// IsMiss reports whether the "try as an ID" fast-path should give up and
// fall through to a name search. Favro returns 404 for some endpoints and 403
// (forbidden) for others when an ID is absent/inaccessible, so both mean "this
// string is not a usable ID for me - try matching by name".
func IsMiss(err error) bool {
	var nfe *favro.NotFoundError
	if errors.As(err, &nfe) {
		return true
	}
	var auth *favro.AuthError
	if errors.As(err, &auth) && auth.Status == 403 {
		return true
	}
	return false
}

// Resolver wraps a Favro client and resolves IDs/names to entities.
type Resolver struct {
	c *favro.Client
}

// New constructs a Resolver backed by the given Favro client.
func New(c *favro.Client) *Resolver { return &Resolver{c: c} }

// MatchByName returns the single item whose name equals identifier
// (case-insensitive), or a NotFound / Ambiguous error.
func MatchByName[T any](items []T, getID func(T) string, getName func(T) string, entityType, identifier string) (*T, error) {
	var matches []T
	lower := strings.ToLower(identifier)
	for _, it := range items {
		if strings.ToLower(getName(it)) == lower {
			matches = append(matches, it)
		}
	}
	if len(matches) == 0 {
		return nil, NewNotFound(entityType, identifier)
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}
	mi := make([]MatchInfo, 0, len(matches))
	for _, m := range matches {
		mi = append(mi, MatchInfo{id: getID(m), desc: getName(m)})
	}
	return nil, NewAmbiguous(entityType, identifier, mi)
}

// Organization --------------------------------------------------------------

func (r *Resolver) Organization(idOrName string) (*favro.Organization, error) {
	if org, err := r.c.GetOrganization(idOrName); err == nil && org != nil {
		return org, nil
	} else if err != nil && !IsMiss(err) {
		return nil, err
	}
	orgs, err := r.c.GetOrganizations()
	if err != nil {
		return nil, err
	}
	return MatchByName(orgs, func(o favro.Organization) string { return o.OrganizationID }, func(o favro.Organization) string { return o.Name }, "organization", idOrName)
}

// Board (Widget) ------------------------------------------------------------

func (r *Resolver) Board(idOrName string) (*favro.Widget, error) {
	if w, err := r.c.GetWidget(idOrName); err == nil && w != nil {
		return w, nil
	} else if err != nil && !IsMiss(err) {
		return nil, err
	}
	boards, err := r.c.GetWidgets("", false)
	if err != nil {
		return nil, err
	}
	return MatchByName(boards, func(w favro.Widget) string { return w.WidgetCommonID }, func(w favro.Widget) string { return w.Name }, "board", idOrName)
}

// Tag -----------------------------------------------------------------------

func (r *Resolver) Tag(idOrName string) (*favro.Tag, error) {
	if t, err := r.c.GetTag(idOrName); err == nil && t != nil {
		return t, nil
	} else if err != nil && !IsMiss(err) {
		return nil, err
	}
	tags, err := r.c.GetTags()
	if err != nil {
		return nil, err
	}
	return MatchByName(tags, func(t favro.Tag) string { return t.TagID }, func(t favro.Tag) string { return t.Name }, "tag", idOrName)
}

// Column (name resolution requires a board) ---------------------------------

func (r *Resolver) Column(idOrName, boardID string) (*favro.Column, error) {
	if col, err := r.c.GetColumn(idOrName); err == nil && col != nil {
		return col, nil
	} else if err != nil && !IsMiss(err) {
		return nil, err
	}
	if boardID == "" {
		return nil, errors.New("board is required to resolve columns by name")
	}
	cols, err := r.c.GetColumns(boardID)
	if err != nil {
		return nil, err
	}
	return MatchByName(cols, func(c favro.Column) string { return c.ColumnID }, func(c favro.Column) string { return c.Name }, "column", idOrName)
}

// Lane (no by-id endpoint; both ID and name matched against a board's lanes) -

func (r *Resolver) Lane(idOrName, boardID string) (*favro.Lane, error) {
	if boardID == "" {
		return nil, errors.New("board is required to resolve lanes")
	}
	lanes, err := r.c.GetLanes(boardID)
	if err != nil {
		return nil, err
	}
	// Exact ID match first.
	for i := range lanes {
		if lanes[i].LaneID == idOrName {
			return &lanes[i], nil
		}
	}
	return MatchByName(lanes, func(l favro.Lane) string { return l.LaneID }, func(l favro.Lane) string { return l.Name }, "lane", idOrName)
}

// User (ID, name, or email) -------------------------------------------------

func (r *Resolver) User(idOrNameOrEmail string) (*favro.User, error) {
	// Try base resolution: ID then name.
	if u, err := r.resolveUserByNameOrID(idOrNameOrEmail); err == nil {
		return u, nil
	} else if _, amb := err.(*Ambiguous); amb {
		return nil, err
	} else if _, nf := err.(*NotFound); !nf {
		return nil, err
	}
	// Fall back to email match.
	users, err := r.c.GetUsers()
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(idOrNameOrEmail)
	var matches []favro.User
	for _, u := range users {
		if strings.ToLower(u.Email) == lower {
			matches = append(matches, u)
		}
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}
	if len(matches) > 1 {
		mi := make([]MatchInfo, 0, len(matches))
		for _, u := range matches {
			mi = append(mi, MatchInfo{id: u.UserID, desc: fmt.Sprintf("%s (%s)", u.Name, u.Email)})
		}
		return nil, NewAmbiguous("user", idOrNameOrEmail, mi)
	}
	return nil, NewNotFound("user", idOrNameOrEmail)
}

func (r *Resolver) resolveUserByNameOrID(idOrName string) (*favro.User, error) {
	if u, err := r.c.GetUser(idOrName); err == nil && u != nil {
		return u, nil
	} else if err != nil && !IsMiss(err) {
		return nil, err
	}
	users, err := r.c.GetUsers()
	if err != nil {
		return nil, err
	}
	return MatchByName(users, func(u favro.User) string { return u.UserID }, func(u favro.User) string { return u.Name }, "user", idOrName)
}

// Card (ID, sequential #123, or name; name needs a board) -------------------

var seqIDRe = regexp.MustCompile(`^(?:#|[a-zA-Z]+-)?(\d+)$`)

// ParseSequentialID extracts a positive integer from identifiers like "#123",
// "123", or "Ref-123". The bool result is false when the string is not a valid
// sequential ID (or is non-positive).
func ParseSequentialID(identifier string) (int, bool) {
	m := seqIDRe.FindStringSubmatch(strings.TrimSpace(identifier))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func (r *Resolver) Card(idOrName, boardID string) (*favro.Card, error) {
	// Sequential ID (#123, 123, Ref-123): use the API's cardSequentialId filter
	// with unique=false so multi-board instances come back distinctly.
	if seq, ok := ParseSequentialID(idOrName); ok {
		board := ""
		cards, err := r.c.GetCards(favro.CardFilter{
			WidgetCommonID:   board,
			CardSequentialID: &seq,
			Unique:           false,
		})
		if err != nil {
			return nil, err
		}
		if boardID != "" {
			board = boardID
			filtered := cards[:0]
			for _, c := range cards {
				if c.WidgetCommonID != nil && *c.WidgetCommonID == board {
					filtered = append(filtered, c)
				}
			}
			cards = filtered
		}
		if len(cards) == 0 {
			return nil, NewNotFound("card", idOrName)
		}
		if len(cards) == 1 {
			return &cards[0], nil
		}
		mi := make([]MatchInfo, 0, len(cards))
		for _, c := range cards {
			b := ""
			if c.WidgetCommonID != nil {
				b = *c.WidgetCommonID
			}
			mi = append(mi, MatchInfo{id: c.CardID, desc: fmt.Sprintf("#%d on board %s: %s", c.SequentialID, b, c.Name)})
		}
		return nil, NewAmbiguous("card", idOrName, mi)
	}

	// Direct ID lookup.
	if c, err := r.c.GetCard(idOrName); err == nil && c != nil {
		return c, nil
	} else if err != nil && !IsMiss(err) {
		return nil, err
	}

	// Name lookup requires a board.
	if boardID == "" {
		return nil, fmt.Errorf("card '%s' not found by ID. To search by name, provide the board argument", idOrName)
	}
	cards, err := r.c.GetCards(favro.CardFilter{WidgetCommonID: boardID, Unique: true})
	if err != nil {
		return nil, err
	}
	return MatchByName(cards, func(c favro.Card) string { return c.CardID }, func(c favro.Card) string { return c.Name }, "card", idOrName)
}
