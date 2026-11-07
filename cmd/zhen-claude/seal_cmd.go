// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"unheaded/pkg/champion"
	"unheaded/pkg/gungnir"
)

// runSealCmd is `zhen-claude seal-runbooks`: a person's act after reading a
// runbook change. It seals runbooks/MANIFEST.sha256 with the ML-DSA-65 seed in
// -key (unheaded-keygen -algo mldsa65 runbook-manifest), so live runs of what
// the manifest lists can start again (pkg/champion/runbookseal.go).
func runSealCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("zhen-claude seal-runbooks", flag.ContinueOnError)
	fs.SetOutput(stderr)
	key := fs.String("key", "", "the ML-DSA-65 seed file (0600)")
	root := fs.String("project-root", ".", "the root holding runbooks/")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *key == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: zhen-claude seal-runbooks -key FILE [-project-root DIR]")
		return 2
	}
	priv, err := gungnir.LoadSeedFile(*key)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "zhen-claude seal-runbooks: %v\n", err)
		return 1
	}
	path := filepath.Join(*root, "runbooks", champion.RunbookManifestFile)
	manifest, err := os.ReadFile(path) // #nosec G304 -- the operator's own -project-root
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "zhen-claude seal-runbooks: %v\n", err)
		return 1
	}
	seal, err := champion.SealRunbookManifest(manifest, priv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "zhen-claude seal-runbooks: %v\n", err)
		return 1
	}
	if err := os.WriteFile(path+champion.RunbookSealSuffix, seal, 0o644); err != nil { // #nosec G306 -- a signature, public by nature
		_, _ = fmt.Fprintf(stderr, "zhen-claude seal-runbooks: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "sealed %s\n", path)
	return 0
}
