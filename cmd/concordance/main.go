// Command concordance is the entry point for the documentation retrieval project.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const topUsage = `Usage: concordance version
       concordance search [flags] TARGET "QUERY"
       concordance target show [flags] NAME
       concordance target activate [flags] NAME

Search is for trusted operators with a configured PostgreSQL service and R2 access.
Use search --help or target activate --help for settings and admission limits.
Imports are prepared and published through internal APIs; there is no import command.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "concordance:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")) {
		return writeCommand(out, []byte(topUsage))
	}
	if len(args) == 1 && args[0] == "version" {
		return writeCommand(out, []byte("concordance dev\n"))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	switch args[0] {
	case "search":
		return runSearch(ctx, args[1:], out, os.Getenv, connectSearch)
	case "target":
		return runTarget(ctx, args[1:], out, os.Getenv, connectSearch)
	}
	return errors.New("unknown command or arguments; use concordance --help")
}
