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

// runLogin obtains Favro credentials (via flags or interactive prompt), verifies
// them against the live API, and only then persists them. Invalid credentials
// are never saved. In unattended mode (both --email and --token given) an
// invalid attempt exits with a non-zero status; interactively it re-prompts.
func runLogin(args []string) {
	fs := flag.NewFlagSet("favro-mcp login", flag.ExitOnError)
	email := fs.String("email", "", "Favro email (else prompted)")
	token := fs.String("token", "", "Favro API token (else prompted, hidden)")
	_ = fs.Parse(args)

	verify := func(e, t string) (int, error) {
		orgs, err := favro.NewClient(e, t, "").GetOrganizations()
		if err != nil {
			return 0, err
		}
		return len(orgs), nil
	}

	// Unattended: both credentials supplied via flags - verify once, save or fail.
	if *email != "" && *token != "" {
		n, err := verify(*email, *token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Credentials invalid: %v\nNot saved.\n", err)
			os.Exit(1)
		}
		if err := credentials.Save(*email, *token); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("Credentials verified - %d organization(s) accessible.\nSaved to ~/.favro-mcp/credentials.json\n", n)
		return
	}

	// Interactive: prompt until valid (or cancelled).
	for {
		e, t, err := credentials.Prompt()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		n, err := verify(e, t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Verification failed (%v). Credentials not saved - try again.\n", err)
			continue
		}
		if err := credentials.Save(e, t); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("Credentials verified - %d organization(s) accessible.\nSaved to ~/.favro-mcp/credentials.json\n", n)
		return
	}
}

// runInstaller parses the install/uninstall flags and delegates to the
// installer (ported from sana-mcp's install module).
func runInstaller(uninstall bool, args []string) {
	fs := flag.NewFlagSet("favro-mcp", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show what would change without writing anything")
	yes := fs.Bool("yes", false, "register with all detected clients, no prompts")
	name := fs.String("name", "favro", "server name written into client configs")
	toolset := fs.String("toolset", "", "toolset to expose: read, write, delete, or custom")
	email := fs.String("email", "", "Favro email (else FAVRO_EMAIL env or `favro-mcp login`)")
	token := fs.String("token", "", "Favro API token (else FAVRO_API_TOKEN env or `favro-mcp login`)")
	_ = fs.Parse(args)

	opts := install.Options{DryRun: *dryRun, Yes: *yes, Name: *name, Toolset: *toolset, Email: *email, Token: *token}
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
