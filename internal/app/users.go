package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// usersTableText renders the organization's members as a fixed-width table
// suitable for the scroll view. Used by the interactive Users screen.
func usersTableText(users []favro.User) string {
	sorted := make([]favro.User, len(users))
	copy(sorted, users)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	const (
		nameW  = 22
		emailW = 26
	)
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s\n",
		padRight("Name", nameW), padRight("Email", emailW), "Role")
	fmt.Fprintln(&b, strings.Repeat("-", nameW+emailW+6))
	for _, u := range sorted {
		role := userRole(u)
		fmt.Fprintf(&b, "%s  %s  %s\n",
			padRight(u.Name, nameW), padRight(u.Email, emailW), role)
	}
	return b.String()
}

// userRole returns the role label for a user, falling back to a dash when the
// organization role is unset. Shared with the one-shot `list-users` command.
func userRole(u favro.User) string {
	if u.OrganizationRole != nil && *u.OrganizationRole != "" {
		return *u.OrganizationRole
	}
	return "-"
}
