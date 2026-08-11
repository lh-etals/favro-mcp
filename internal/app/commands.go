package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/favro/resolver"
)

// OneShotCommands maps every accepted one-shot command name (including
// aliases) to its implementation. main.go consults this map after the built-in
// subcommand switch (mcp / login / install / uninstall) and before falling
// through to the bare-invocation path (interactive app or MCP server).
var oneShotCommands = map[string]func([]string){
	"list-boards": runListBoardsCmd,
	"boards":      runListBoardsCmd,
	"list-cards":  runListCardsCmd,
	"cards":       runListCardsCmd,
	"get-card":    runGetCardCmd,
	"card":        runGetCardCmd,
	"add-comment": runAddCommentCmd,
	"comment":     runAddCommentCmd,
	"create-card": runCreateCardCmd,
	"list-users":  runListUsersCmd,
	"users":       runListUsersCmd,
	"list-tags":   runListTagsCmd,
	"tags":        runListTagsCmd,
}

// IsOneShot reports whether name is a recognized one-shot command.
func IsOneShot(name string) bool {
	_, ok := oneShotCommands[name]
	return ok
}

// RunOneShot dispatches to the named one-shot command. The caller must first
// confirm the command exists with IsOneShot. Args are normalized so flags may
// appear after positional arguments (e.g. `get-card 123 --board X`), which the
// stdlib flag package does not handle on its own.
func RunOneShot(name string, args []string) {
	oneShotCommands[name](reorderFlags(args))
}

// reorderFlags moves flag arguments (and their values) ahead of positional
// arguments so flag.Parse sees them. All one-shot flags are non-boolean
// (string/int), so a "-"-prefixed arg consumes the next arg as its value
// unless it is of the form --name=value or is a help flag. A bare "--" marks
// everything after it as positional.
func reorderFlags(args []string) []string {
	var flags, positional []string
	afterSep := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if afterSep {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			afterSep = true
			continue
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		if strings.Contains(a, "=") || a == "-h" || a == "--help" || a == "-help" {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

// --- shared helpers ---------------------------------------------------------

// dial loads credentials (env vars override the saved file), resolves the
// organization, and returns a client bound to it. Any failure prints a helpful
// message to stderr and exits the process.
func dial(orgArg string) *favro.Client {
	email, token, err := loadCredentials()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Not logged in. Run 'favro-mcp login' first.")
		os.Exit(1)
	}
	bare := favro.NewClient(email, token, "")
	orgID, err := resolveOrg(bare, orgArg)
	if err != nil {
		fail(err)
	}
	return favro.NewClient(email, token, orgID)
}

// loadCredentials prefers FAVRO_EMAIL / FAVRO_API_TOKEN env vars and falls back
// to the saved credentials file.
func loadCredentials() (email, token string, err error) {
	email = os.Getenv("FAVRO_EMAIL")
	token = os.Getenv("FAVRO_API_TOKEN")
	if email != "" && token != "" {
		return email, token, nil
	}
	e, t, err := credentials.Load()
	if err != nil {
		return "", "", err
	}
	if email == "" {
		email = e
	}
	if token == "" {
		token = t
	}
	return email, token, nil
}

// resolveOrg returns the organization ID to scope requests with. An explicit
// --org is matched by ID then name against the (header-less) organizations
// list; otherwise the single org is auto-selected. Listing orgs avoids the
// direct-by-id lookup, which 401s when no organizationId header is set yet.
func resolveOrg(client *favro.Client, orgArg string) (string, error) {
	orgs, err := client.GetOrganizations()
	if err != nil {
		return "", err
	}
	if orgArg == "" {
		switch len(orgs) {
		case 0:
			return "", errors.New("your account has no organizations")
		case 1:
			return orgs[0].OrganizationID, nil
		}
		var b strings.Builder
		b.WriteString("multiple organizations available; use --org <id-or-name>")
		for _, o := range orgs {
			fmt.Fprintf(&b, "\n  %s  %s", o.OrganizationID, o.Name)
		}
		return "", errors.New(b.String())
	}
	// Explicit org: exact ID match first.
	for _, o := range orgs {
		if o.OrganizationID == orgArg {
			return o.OrganizationID, nil
		}
	}
	// Then name match (handles ambiguity via MatchByName).
	m, err := resolver.MatchByName(orgs,
		func(o favro.Organization) string { return o.OrganizationID },
		func(o favro.Organization) string { return o.Name },
		"organization", orgArg)
	if err != nil {
		return "", err
	}
	return m.OrganizationID, nil
}

// fail prints err to stderr (mapping auth failures to a login hint) and exits 1.
func fail(err error) {
	var auth *favro.AuthError
	if errors.As(err, &auth) {
		fmt.Fprintln(os.Stderr, "Not logged in. Run 'favro-mcp login' first.")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// usage prints msg to stderr and exits 2 (the conventional usage-error code).
func usage(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
}

// resolveBoardID resolves a board name-or-ID to its widgetCommonID. An empty
// input returns "" (callers use that to mean "no board").
func resolveBoardID(r *resolver.Resolver, board string) (string, error) {
	if board == "" {
		return "", nil
	}
	b, err := r.Board(board)
	if err != nil {
		return "", err
	}
	return b.WidgetCommonID, nil
}

// resolveCollectionID resolves a collection name-or-ID to its ID. Empty input
// returns "".
func resolveCollectionID(client *favro.Client, nameOrID string) (string, error) {
	if nameOrID == "" {
		return "", nil
	}
	if c, err := client.GetCollection(nameOrID); err == nil && c != nil {
		return c.CollectionID, nil
	} else if err != nil && !resolver.IsMiss(err) {
		return "", err
	}
	cols, err := client.GetCollections(false)
	if err != nil {
		return "", err
	}
	m, err := resolver.MatchByName(cols,
		func(c favro.Collection) string { return c.CollectionID },
		func(c favro.Collection) string { return c.Name },
		"collection", nameOrID)
	if err != nil {
		return "", err
	}
	return m.CollectionID, nil
}

// columnCount returns the number of columns on a board, or "-" on error.
func columnCount(client *favro.Client, widgetCommonID string) string {
	cols, err := client.GetColumns(widgetCommonID)
	if err != nil {
		return "-"
	}
	return fmt.Sprintf("%d", len(cols))
}

// --- list-boards -----------------------------------------------------------

func runListBoardsCmd(args []string) {
	fs := flag.NewFlagSet("favro-mcp list-boards", flag.ExitOnError)
	org := fs.String("org", "", "organization ID or name (auto-selected if you have one)")
	collection := fs.String("collection", "", "collection name or ID")
	_ = fs.Parse(args)

	client := dial(*org)
	collectionID, err := resolveCollectionID(client, *collection)
	if err != nil {
		fail(err)
	}
	boards, err := client.GetWidgets(collectionID, false)
	if err != nil {
		fail(err)
	}
	if len(boards) == 0 {
		fmt.Println("No boards found.")
		return
	}
	sort.Slice(boards, func(i, j int) bool { return boards[i].Name < boards[j].Name })

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tType\tColumns")
	for _, b := range boards {
		fmt.Fprintf(w, "%s\t%s\t%s\n", b.Name, b.Type, columnCount(client, b.WidgetCommonID))
	}
	w.Flush()
}

// --- list-cards ------------------------------------------------------------

func runListCardsCmd(args []string) {
	fs := flag.NewFlagSet("favro-mcp list-cards", flag.ExitOnError)
	org := fs.String("org", "", "organization ID or name (auto-selected if you have one)")
	board := fs.String("board", "", "board name or ID (required)")
	column := fs.String("column", "", "column name to filter by")
	page := fs.Int("page", 1, "page number, 1-indexed")
	_ = fs.Parse(args)

	if *board == "" {
		usage("usage: favro-mcp list-cards --board <name-or-id> [--column name] [--page N] [--org id]")
	}
	if *page < 1 {
		usage("--page must be >= 1")
	}
	client := dial(*org)
	r := resolver.New(client)
	boardID, err := resolveBoardID(r, *board)
	if err != nil {
		fail(err)
	}
	f := favro.CardFilter{WidgetCommonID: boardID, Unique: true}
	if *column != "" {
		col, err := r.Column(*column, boardID)
		if err != nil {
			fail(err)
		}
		f.ColumnID = col.ColumnID
	}
	result, err := client.GetCardsPage(f, *page-1, "")
	if err != nil {
		fail(err)
	}
	cards, total := result.Cards, result.Pages
	if len(cards) == 0 {
		fmt.Println("No cards found.")
		return
	}
	// Build column id->name lookup for display (best-effort).
	colNames := map[string]string{}
	if cols, err := client.GetColumns(boardID); err == nil {
		for _, c := range cols {
			colNames[c.ColumnID] = c.Name
		}
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, c := range cards {
		col := ""
		if c.ColumnID != nil {
			col = colNames[*c.ColumnID]
			if col == "" {
				col = *c.ColumnID
			}
		}
		tags := "[]"
		if len(c.Tags) > 0 {
			tags = "[" + strings.Join(c.Tags, ", ") + "]"
		}
		fmt.Fprintf(w, "#%d\t%s\t%s\t%s\n", c.SequentialID, c.Name, tags, col)
	}
	w.Flush()
	fmt.Printf("Page %d/%d\n", *page, total)
}

// --- get-card --------------------------------------------------------------

func runGetCardCmd(args []string) {
	fs := flag.NewFlagSet("favro-mcp get-card", flag.ExitOnError)
	org := fs.String("org", "", "organization ID or name (auto-selected if you have one)")
	board := fs.String("board", "", "board name or ID (needed to look up a card by name)")
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) < 1 {
		usage("usage: favro-mcp get-card <card-id-or-name-or-#seq> [--board name] [--org id]")
	}
	client := dial(*org)
	r := resolver.New(client)
	boardID, err := resolveBoardID(r, *board)
	if err != nil {
		fail(err)
	}
	card, err := r.Card(rest[0], boardID)
	if err != nil {
		fail(err)
	}
	text, err := buildCardDetailText(client, *card)
	if err != nil {
		fail(err)
	}
	fmt.Print(text)
}

// --- add-comment -----------------------------------------------------------

func runAddCommentCmd(args []string) {
	fs := flag.NewFlagSet("favro-mcp add-comment", flag.ExitOnError)
	org := fs.String("org", "", "organization ID or name (auto-selected if you have one)")
	board := fs.String("board", "", "board name or ID (needed to look up a card by name)")
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) < 2 {
		usage("usage: favro-mcp add-comment <card> <comment> [--board name] [--org id]")
	}
	client := dial(*org)
	r := resolver.New(client)
	boardID, err := resolveBoardID(r, *board)
	if err != nil {
		fail(err)
	}
	card, err := r.Card(rest[0], boardID)
	if err != nil {
		fail(err)
	}
	if _, err := client.CreateComment(card.CardCommonID, rest[1]); err != nil {
		fail(err)
	}
	fmt.Printf("Comment added to #%d %s\n", card.SequentialID, card.Name)
}

// --- create-card -----------------------------------------------------------

func runCreateCardCmd(args []string) {
	fs := flag.NewFlagSet("favro-mcp create-card", flag.ExitOnError)
	org := fs.String("org", "", "organization ID or name (auto-selected if you have one)")
	name := fs.String("name", "", "card title (required)")
	board := fs.String("board", "", "board name or ID (required)")
	column := fs.String("column", "", "column name to place the card in")
	description := fs.String("description", "", "detailed description (Favro supports a subset of Markdown)")
	_ = fs.Parse(args)
	if *name == "" || *board == "" {
		usage("usage: favro-mcp create-card --name <title> --board <name> [--column name] [--description text] [--org id]")
	}
	client := dial(*org)
	r := resolver.New(client)
	b, err := r.Board(*board)
	if err != nil {
		fail(err)
	}
	var columnID string
	if *column != "" {
		col, err := r.Column(*column, b.WidgetCommonID)
		if err != nil {
			fail(err)
		}
		columnID = col.ColumnID
	}
	// Prime the description field with a space when content is provided: Favro
	// only parses markdown on update if the field already has content, and a
	// board template may overwrite the description sent at creation.
	var primed *string
	if *description != "" {
		space := " "
		primed = &space
	}
	card, err := client.CreateCard(favro.CreateCardOpts{
		Name: *name, WidgetCommonID: b.WidgetCommonID, ColumnID: columnID,
		DetailedDescription: primed,
	})
	if err != nil {
		fail(err)
	}
	if *description != "" {
		desc := *description
		card, err = client.UpdateCard(favro.UpdateCardOpts{CardID: card.CardID, DetailedDescription: &desc})
		if err != nil {
			fail(err)
		}
	}
	fmt.Printf("Created #%d %s on board %s\n", card.SequentialID, card.Name, b.Name)
}

// --- list-users ------------------------------------------------------------

func runListUsersCmd(args []string) {
	fs := flag.NewFlagSet("favro-mcp list-users", flag.ExitOnError)
	org := fs.String("org", "", "organization ID or name (auto-selected if you have one)")
	_ = fs.Parse(args)

	client := dial(*org)
	users, err := client.GetUsers()
	if err != nil {
		fail(err)
	}
	if len(users) == 0 {
		fmt.Println("No users found.")
		return
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tEmail\tRole")
	for _, u := range users {
		fmt.Fprintf(w, "%s\t%s\t%s\n", u.Name, u.Email, userRole(u))
	}
	w.Flush()
}

// --- list-tags -------------------------------------------------------------

func runListTagsCmd(args []string) {
	fs := flag.NewFlagSet("favro-mcp list-tags", flag.ExitOnError)
	org := fs.String("org", "", "organization ID or name (auto-selected if you have one)")
	_ = fs.Parse(args)

	client := dial(*org)
	tags, err := client.GetTags()
	if err != nil {
		fail(err)
	}
	if len(tags) == 0 {
		fmt.Println("No tags found.")
		return
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tColor")
	for _, t := range tags {
		color := "-"
		if t.Color != nil && *t.Color != "" {
			color = *t.Color
		}
		fmt.Fprintf(w, "%s\t%s\n", t.Name, color)
	}
	w.Flush()
}
