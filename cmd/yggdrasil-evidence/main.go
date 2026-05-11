// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Command yggdrasil-evidence validates, signs, and verifies Yggdrasil
// signed-manifest evidence packs (task #68).
//
// Subcommands:
//   validate  — yamllint + JSON-Schema validate manifest.yaml against
//               nix/yggdrasil/evidence-pack/schema/manifest-v1.yaml
//   sign      — produce manifest.yaml.sig via cloudflare/circl ML-DSA-65
//   verify    — full evidence-pack verification (signature + ISO hash +
//               CI gates per the manifest)
//   diff      — diff two evidence packs (e.g. quarterly compliance review)
//
// Scaffold only — implementation lights up at task #65 (packer pipeline).
// The CLI surface is the contract; runbooks at
// nix/yggdrasil/evidence-pack/runbooks/ reference these subcommands.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "validate":
		cmdValidate(os.Args[2:])
	case "sign":
		cmdSign(os.Args[2:])
	case "verify":
		cmdVerify(os.Args[2:])
	case "verify-signature":
		cmdVerifySignature(os.Args[2:])
	case "verify-iso":
		cmdVerifyISO(os.Args[2:])
	case "verify-gates":
		cmdVerifyGates(os.Args[2:])
	case "verify-custody":
		cmdVerifyCustody(os.Args[2:])
	case "diff":
		cmdDiff(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "yggdrasil-evidence: unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `yggdrasil-evidence — Validate, sign, and verify Yggdrasil signed-manifest evidence packs.

Usage:
  yggdrasil-evidence <subcommand> [flags]

Subcommands:
  validate           Validate manifest.yaml against schema
  sign               Sign manifest.yaml with ML-DSA-65 build key
  verify             Full evidence-pack verification (sig + ISO + gates)
  verify-signature   Just check the manifest signature
  verify-iso         Just check the ISO hash matches the manifest
  verify-gates       Just check the CI gates from the manifest
  verify-custody     Just check the chain-of-custody log
  diff               Diff two evidence packs

See: nix/yggdrasil/evidence-pack/README.md for the full spec.
See: nix/yggdrasil/evidence-pack/runbooks/ for operator runbooks.`)
}

// --- subcommand stubs ---
//
// Each subcommand is wired with its flag-set and a TODO panic so the
// runbook contract is captured. Implementation lights up at task #65.

func cmdValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	schema := fs.String("schema", "nix/yggdrasil/evidence-pack/schema/manifest-v1.yaml", "Path to JSON Schema")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "validate: expected one positional arg (manifest.yaml path)")
		os.Exit(2)
	}
	manifest := fs.Arg(0)
	fmt.Fprintf(os.Stderr, "TODO(task #65): yamllint + JSON-Schema validate %s against %s\n", manifest, *schema)
	os.Exit(1)
}

func cmdSign(args []string) {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	key := fs.String("key", "/etc/yggdrasil/build.key", "ML-DSA-65 private key path")
	in := fs.String("in", "", "Input file to sign")
	out := fs.String("out", "", "Output signature file")
	_ = fs.Parse(args)
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "sign: --in and --out are required")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "TODO(task #65): sign %s with key %s -> %s via pkg/gungnir/\n", *in, *key, *out)
	os.Exit(1)
}

func cmdVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pubkey := fs.String("pubkey", "/etc/yggdrasil/root.pubkey", "ML-DSA-65 root public key path")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "verify: expected one positional arg (evidence-pack tarball)")
		os.Exit(2)
	}
	pack := fs.Arg(0)
	fmt.Fprintf(os.Stderr, "TODO(task #65): full verify %s with pubkey %s\n", pack, *pubkey)
	os.Exit(1)
}

func cmdVerifySignature(args []string) {
	fs := flag.NewFlagSet("verify-signature", flag.ExitOnError)
	pubkey := fs.String("pubkey", "/etc/yggdrasil/root.pubkey", "ML-DSA-65 public key path")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "verify-signature: expected one positional arg (evidence-pack tarball)")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "TODO(task #65): verify-signature %s with pubkey %s\n", fs.Arg(0), *pubkey)
	os.Exit(1)
}

func cmdVerifyISO(args []string) {
	fs := flag.NewFlagSet("verify-iso", flag.ExitOnError)
	iso := fs.String("iso", "", "Path to the ISO file to verify against")
	_ = fs.Parse(args)
	if *iso == "" || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "verify-iso: --iso and a positional evidence-pack arg are required")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "TODO(task #65): verify-iso %s against pack %s\n", *iso, fs.Arg(0))
	os.Exit(1)
}

func cmdVerifyGates(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "verify-gates: expected one positional arg (evidence-pack tarball)")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "TODO(task #65): verify-gates %s (CRITICAL/HIGH==0, CIS≥95%%, Lynis≥90, doctor exit==0)\n", args[0])
	os.Exit(1)
}

func cmdVerifyCustody(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "verify-custody: expected one positional arg (evidence-pack tarball)")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "TODO(task #65): verify-custody %s (monotonic timestamps, signature chain)\n", args[0])
	os.Exit(1)
}

func cmdDiff(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "diff: expected two positional args (old-pack, new-pack)")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "TODO(task #65): diff %s -> %s\n", args[0], args[1])
	os.Exit(1)
}
