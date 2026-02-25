// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the Business Source License 1.1 (BSL 1.1).
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

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
