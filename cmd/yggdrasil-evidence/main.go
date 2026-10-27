// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Command yggdrasil-evidence validates, signs, and verifies Yggdrasil
// signed-manifest evidence packs (task #68, ADR-105 decisions 5-7).
//
// Subcommands:
//
//	validate          manifest.yaml against schema v1 (rules mirrored in Go,
//	                  drift-guarded against nix/yggdrasil/evidence-pack/schema/)
//	sign              gungnir evidence seal over manifest.yaml or
//	                  image.iso.sha256, or a hex custody-entry signature
//	pubkey            derive build.pubkey from the build key seed
//	verify-signature  both seals against build.pubkey
//	verify-iso        an ISO against the signed image.iso.sha256 and manifest
//	verify-gates      CI gates, recomputed from the pack's attachments
//	verify-custody    hash-chained, signed, monotonic chain_of_custody
//	verify            all of the above, each stage reported
//	diff              two packs; exit 3 if custody history was rewritten
//	cask sign|verify  Sealed Cask Binding Rune seal (pkg/sealedcask)
//
// Packs are read in memory with size caps and no extraction (pack.go). Runbooks
// at nix/yggdrasil/evidence-pack/runbooks/ use these subcommands.
//
// Exit codes: 0 ok, 1 verification failed, 2 usage or unreadable input,
// 3 (diff) custody rewritten.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main without os.Exit, so every subcommand is testable in-process.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stderr)
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "validate":
		return cmdValidate(rest, stdout, stderr)
	case "sign":
		return cmdSign(rest, stdout, stderr)
	case "pubkey":
		return cmdPubkey(rest, stdout, stderr)
	case "verify":
		return cmdVerify(rest, stdout, stderr)
	case "verify-signature":
		return cmdVerifySignature(rest, stdout, stderr)
	case "verify-iso":
		return cmdVerifyISO(rest, stdout, stderr)
	case "verify-gates":
		return cmdVerifyGates(rest, stdout, stderr)
	case "verify-custody":
		return cmdVerifyCustody(rest, stdout, stderr)
	case "diff":
		return cmdDiff(rest, stdout, stderr)
	case "cask":
		return cmdCask(rest, stdout, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "yggdrasil-evidence: unknown subcommand %q\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `yggdrasil-evidence — Validate, sign, and verify Yggdrasil signed-manifest evidence packs.

Usage:
  yggdrasil-evidence <subcommand> [flags]

Subcommands:
  validate           Validate manifest.yaml against schema
  sign               Sign manifest.yaml, image.iso.sha256 or a custody entry (ML-DSA-65 build key)
  pubkey             Derive build.pubkey from the build key seed (generates nothing)
  verify             Full evidence-pack verification (sig + ISO + gates)
  verify-signature   Just check the manifest signature
  verify-iso         Just check the ISO hash matches the manifest
  verify-gates       Just check the CI gates from the manifest
  verify-custody     Just check the chain-of-custody log
  diff               Diff two evidence packs
  cask sign          Seal a Sealed Cask's BINDING_RUNE.sha256 with the build key
  cask verify        Verify a Sealed Cask (also: the standalone cask-verify)

Exit codes: 0 ok, 1 verification failed, 2 usage or unreadable input.

See: nix/yggdrasil/evidence-pack/README.md for the full spec.
See: nix/yggdrasil/evidence-pack/runbooks/ for operator runbooks.`)
}

// newFlags returns a FlagSet that reports to stderr and never exits.
func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("validate", stderr)
	schema := fs.String("schema", "", "optional: refuse to run unless this schema file matches the rules built into this binary")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "validate: expected one positional arg (manifest.yaml path)")
		return exitUsage
	}
	if *schema != "" {
		b, err := os.ReadFile(*schema)
		if err != nil {
			fmt.Fprintf(stderr, "validate: %v\n", err)
			return exitUsage
		}
		if drift := schemaDrift(b); len(drift) > 0 {
			fmt.Fprintf(stderr, "validate: %s does not match the schema v1 rules built into this binary:\n", *schema)
			for _, d := range drift {
				fmt.Fprintln(stderr, "  "+d)
			}
			return exitUsage
		}
	}
	path := fs.Arg(0)
	data, err := readBounded(path, maxManifestBytes)
	if err != nil {
		fmt.Fprintf(stderr, "validate: %v\n", err)
		return exitUsage
	}
	if _, err := ValidateManifest(data); err != nil {
		fmt.Fprintf(stderr, "validate: %s: INVALID\n%v\n", path, err)
		return exitFail
	}
	fmt.Fprintf(stdout, "validate: %s: OK (schema v1)\n", path)
	return exitOK
}

// readBounded reads a regular file of at most limit bytes.
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-named input file
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, limit)
	}
	return b, nil
}
