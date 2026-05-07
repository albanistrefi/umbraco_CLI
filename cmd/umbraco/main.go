package main

import (
	"fmt"
	"os"

	"umbraco-cli/internal/cli"
)

func main() {
	root, err := cli.NewRootCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize runtime: %v\n", err)
		os.Exit(1)
	}
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
