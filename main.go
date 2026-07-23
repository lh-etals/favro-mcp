package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lh-etals/favro-mcp/internal/install"
	"github.com/lh-etals/favro-mcp/internal/mcpserver"
)

func main() {
	// `install` and `uninstall` register favro-mcp with detected MCP clients.
	// Any other invocation runs the stdio MCP server.
	if len(os.Args) > 1 && (os.Args[1] == "install" || os.Args[1] == "uninstall") {
		runInstaller(os.Args[1] == "uninstall", os.Args[2:])
		return
	}

	// Run the MCP server over stdio.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mcpserver.NewServer().Run(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
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
