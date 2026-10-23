// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// crypto-inventory regenerates the repository's cryptographic inventory
// (CBOM, CycloneDX 1.6) and checks it for staleness (ADR-105).
//
//	go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json
//	go run ./cmd/crypto-inventory check            # exit 1 when the CBOM is stale
//	go run ./cmd/crypto-inventory summary          # pq-status x count
//	go run ./cmd/crypto-inventory lint --baseline docs/policy/pq-kex-baseline.txt
//	go run ./cmd/crypto-inventory lint --print     # today's violations, no baseline
//	go run ./cmd/crypto-inventory lint --explain services/wotan/cmd/wotan/main.go
//	go run ./cmd/crypto-inventory collect-host --host east --ssh govan@east --out compliance/crypto/hosts/east.json
//	go run ./cmd/crypto-inventory collect-host --host west --local --out compliance/crypto/hosts/west.json
//
// scan and check read the tree only (git ls-files: tracked plus untracked,
// not ignored) and the checked-in host facts; they never connect anywhere.
// collect-host runs read-only commands and writes hostnames, never
// addresses or key material.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultCBOM     = "compliance/crypto/cbom.cdx.json"
	defaultBaseline = "docs/policy/pq-kex-baseline.txt"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `usage: crypto-inventory <command> [flags]

commands:
  scan          regenerate the CBOM (--out PATH, default stdout)
  check         exit 1 when the checked-in CBOM differs from a regeneration
  summary       count components by pq-status
  collect-host  write one host's read-only crypto facts (--host, --local | --ssh, --out)
  lint          exit 1 on classical-only key exchange not in the shrink-only baseline
                (--baseline FILE, --update refuses additions, --print, --explain PATH)
`

// run dispatches a subcommand; exit codes: 0 ok, 1 finding, 2 usage/error.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "repository root (any directory inside it)")
	switch cmd {
	case "scan":
		out := fs.String("out", "-", "output file, - for stdout")
		if fs.Parse(rest) != nil || fs.NArg() != 0 {
			return 2
		}
		return cmdScan(*root, *out, stdout, stderr)
	case "check":
		cbom := fs.String("cbom", defaultCBOM, "checked-in CBOM, relative to the repository root")
		if fs.Parse(rest) != nil || fs.NArg() != 0 {
			return 2
		}
		return cmdCheck(*root, *cbom, stdout, stderr)
	case "summary":
		if fs.Parse(rest) != nil || fs.NArg() != 0 {
			return 2
		}
		return cmdSummary(*root, stdout, stderr)
	case "collect-host":
		host := fs.String("host", "", "short host name recorded in the file (e.g. east)")
		local := fs.Bool("local", false, "collect from this machine")
		sshTarget := fs.String("ssh", "", "collect over ssh from user@host (BatchMode)")
		out := fs.String("out", "", "output file")
		if fs.Parse(rest) != nil || fs.NArg() != 0 {
			return 2
		}
		return cmdCollectHost(*host, *local, *sshTarget, *out, stdout, stderr)
	case "lint":
		baseline := fs.String("baseline", defaultBaseline, "shrink-only baseline, relative to the repository root")
		update := fs.Bool("update", false, "rewrite the baseline from the tree; refuses additions")
		print := fs.Bool("print", false, "print today's violations and exit 0")
		explain := fs.String("explain", "", "show what each scanner recorded for one repo-relative path")
		if fs.Parse(rest) != nil || fs.NArg() != 0 {
			return 2
		}
		return cmdLint(*root, *baseline, *print, *update, *explain, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", cmd, usage)
		return 2
	}
}

func cmdScan(root, out string, stdout, stderr io.Writer) int {
	tr, err := gitTree(root)
	if err != nil {
		fmt.Fprintln(stderr, "scan:", err)
		return 2
	}
	b, _, err := generate(tr)
	if err != nil {
		fmt.Fprintln(stderr, "scan:", err)
		return 2
	}
	if out == "-" {
		_, err = stdout.Write(b)
	} else {
		err = os.WriteFile(out, b, 0o644) // #nosec G306 -- checked-in inventory, no secrets
	}
	if err != nil {
		fmt.Fprintln(stderr, "scan:", err)
		return 2
	}
	return 0
}

func cmdCheck(root, cbom string, stdout, stderr io.Writer) int {
	tr, err := gitTree(root)
	if err != nil {
		fmt.Fprintln(stderr, "check:", err)
		return 2
	}
	want, _, err := generate(tr)
	if err != nil {
		fmt.Fprintln(stderr, "check:", err)
		return 2
	}
	p := cbom
	if !filepath.IsAbs(p) {
		p = filepath.Join(tr.Root, filepath.FromSlash(cbom))
	}
	have, err := os.ReadFile(p) // #nosec G304 -- operator-chosen inventory path
	if err != nil {
		fmt.Fprintf(stderr, "[FAIL] %s: %v\n  Generate it: go run ./cmd/crypto-inventory scan --out %s\n", cbom, err, cbom)
		return 1
	}
	if err := compareCBOM(want, have); err != nil {
		if errors.Is(err, errStale) {
			fmt.Fprintf(stderr, "[FAIL] %s: %v\n  Regenerate: go run ./cmd/crypto-inventory scan --out %s\n", cbom, err, cbom)
			return 1
		}
		fmt.Fprintln(stderr, "check:", err)
		return 2
	}
	fmt.Fprintf(stdout, "[PASS] %s matches the tree\n", cbom)
	return 0
}

func cmdSummary(root string, stdout, stderr io.Writer) int {
	tr, err := gitTree(root)
	if err != nil {
		fmt.Fprintln(stderr, "summary:", err)
		return 2
	}
	_, assets, err := generate(tr)
	if err != nil {
		fmt.Fprintln(stderr, "summary:", err)
		return 2
	}
	fmt.Fprint(stdout, summarize(assets))
	return 0
}

func cmdCollectHost(host string, local bool, sshTarget, out string, stdout, stderr io.Writer) int {
	if host == "" || out == "" || local == (sshTarget != "") {
		fmt.Fprintln(stderr, "collect-host: need --host, --out and exactly one of --local / --ssh")
		return 2
	}
	var r Runner = localRunner{}
	if sshTarget != "" {
		if !reSSHTarget.MatchString(sshTarget) {
			fmt.Fprintf(stderr, "collect-host: bad --ssh target %q (want user@host)\n", sshTarget)
			return 2
		}
		r = sshRunner{target: sshTarget}
	}
	if err := writeHostFacts(host, r, out); err != nil {
		fmt.Fprintln(stderr, "collect-host:", err)
		return 2
	}
	fmt.Fprintf(stdout, "wrote %s\n", out)
	return 0
}
