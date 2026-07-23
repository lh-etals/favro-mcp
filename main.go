package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lh-etals/favro-mcp/internal/credentials"
	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/lh-etals/favro-mcp/internal/install"
	"github.com/lh-etals/favro-mcp/internal/mcpserver"
)

func main() {
	// Subcommands: login (credentials), install/uninstall (register with AI
	// clients). Any other invocation runs the stdio MCP server.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login":
			runLogin(os.Args[2:])
			return
		case "install":
			runInstaller(false, os.Args[2:])
			return
		case "uninstall":
			runInstaller(true, os.Args[2:])
			return
		}
	}

	// Run the MCP server over stdio.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mcpserver.NewServer().Run(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// runLogin prompts for (or accepts via flags) the Favro email + API token,
// stores them centrally, and verifies them against the Favro API.
func runLogin(args []string) {
	fs := flag.NewFlagSet("favro-mcp login", flag.ExitOnError)
	email := fs.String("email", "", "Favro email (else prompted)")
	token := fs.String("token", "", "Favro API token (else prompted, hidden)")
	_ = fs.Parse(args)

	e, t := *email, *token
	if e == "" || t == "" {
		var err error
		e, t, err = credentials.PromptAndSave()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else if err := credentials.Save(e, t); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Verify against the live API so a typo doesn't silently break the server.
	client := favro.NewClient(e, t, "")
	if orgs, err := client.GetOrganizations(); err != nil {
		fmt.Fprintf(os.Stderr, "Credentials saved to ~/.favro-mcp/credentials.json, but verification failed: %v\n", err)
	} else {
		fmt.Printf("Credentials verified - %d organization(s) accessible.\nSaved to ~/.favro-mcp/credentials.json\n", len(orgs))
	}
}

// runInstaller parses the install/uninstall flags and delegates to the
// installer (ported from sana-mcp's install module).
func runInstaller(uninstall bool, args []string) {
	fs := flag.NewFlagSet("favro-mcp", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show what would change without writing anything")
	yes := fs.Bool("yes", false, "register with all detected clients, no prompts")
	name := fs.String("name", "favro", "server name written into client configs")
	email := fs.String("email", "", "Favro email (else FAVRO_EMAIL env or prompt)")
	token := fs.String("token", "", "Favro API token (else FAVRO_API_TOKEN env or prompt)")
	_ = fs.Parse(args)

	opts := install.Options{DryRun: *dryRun, Yes: *yes, Name: *name, Email: *email, Token: *token}
	var err error
	if uninstall {
		err = install.RunUninstall(opts)
	} else {
		err = install.RunInstall(opts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
