package app

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// runUsers lists the organization's members as a formatted, scrollable table.
// Esc/q returns to the main menu.
func runUsers() {
	fmt.Println()
	s, err := NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Not logged in."))
		fmt.Println()
		return
	}
	if _, err := s.RequireOrg(); err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  "+err.Error()))
		fmt.Println()
		return
	}
	users, err := s.Client().GetUsers()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Failed to load users: "+err.Error()))
		fmt.Println()
		return
	}
	if len(users) == 0 {
		fmt.Println(styleDim.Render("  No users found."))
		fmt.Println()
		return
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Name < users[j].Name
	})

	const (
		nameW  = 22
		emailW = 26
	)
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s\n",
		padRight("Name", nameW), padRight("Email", emailW), "Role")
	fmt.Fprintln(&b, strings.Repeat("-", nameW+emailW+6))
	for _, u := range users {
		role := userRole(u)
		fmt.Fprintf(&b, "%s  %s  %s\n",
			padRight(u.Name, nameW), padRight(u.Email, emailW), role)
	}

	p := tea.NewProgram(newScrollTextModel(
		fmt.Sprintf("Users  %d", len(users)), b.String()))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, styleError.Render("  Error: "+err.Error()))
	}
	fmt.Println()
}

// userRole returns the role label for a user, falling back to a dash when the
// organization role is unset.
func userRole(u favro.User) string {
	if u.OrganizationRole != nil && *u.OrganizationRole != "" {
		return *u.OrganizationRole
	}
	return "-"
}
