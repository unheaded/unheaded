// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// crypto-inventory regenerates the repository's cryptographic inventory
// (CBOM, CycloneDX 1.6) and checks it for staleness (ADR-105).
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	_ = stdout
	fmt.Fprintln(stderr, "usage: crypto-inventory <scan|check|summary|collect-host> [flags]")
	return 2
}
