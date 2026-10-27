// Command concordance is the entry point for the documentation retrieval project.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "concordance:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")) {
		_, err := fmt.Fprintln(out, "Usage: concordance version\n\nConcordance is a version-aware documentation retrieval project.\nFetching, indexing, searching and object storage are not implemented yet.")
		return err
	}
	if len(args) == 1 && args[0] == "version" {
		_, err := fmt.Fprintln(out, "concordance dev")
		return err
	}
	return errors.New("unknown command or arguments; use concordance --help")
}
