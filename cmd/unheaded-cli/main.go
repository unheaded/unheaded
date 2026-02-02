// Package main is THE GAUNTLETS - the CLI tool for the Unheaded Kingdom.
// This is the hands of the Kingdom, commanding all infrastructure operations.
package main

import (
	"fmt"
	"os"

	"unheaded/cmd/unheaded-cli/cmd"
)

// Version information - set at build time
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	// Set version info for commands
	cmd.SetVersionInfo(Version, Commit, BuildTime)

	// Create and execute root command
	root := cmd.NewRootCommand()
	if err := root.Execute(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
