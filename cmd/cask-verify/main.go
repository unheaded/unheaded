// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Command cask-verify checks a Sealed Cask (ADR-010) before deployment: the
// Binding Rune (BINDING_RUNE.sha256) must carry a build-key ML-DSA-65 seal
// (BINDING_RUNE.sig, purpose sealed-cask-v1) that verifies with a trusted
// build.pubkey, and the cask's files must be exactly the rune's. The tarball
// is read as hostile input (pkg/sealedcask): links, devices, "..", absolute
// names, oversize members and trailing bytes are refused before anything is
// written.
//
//	cask-verify verify  --cask <cask.tar.gz> --pubkey <build.pubkey> [--pubkey <old.pubkey>]
//	cask-verify extract --cask <cask.tar.gz> --pubkey <build.pubkey> --dest <new dir>
//
// --pubkey may be repeated during a rotation overlap (both keys valid for a
// quarter, runbooks/security/build-key-ceremony.yaml). It is a standalone
// binary so a deploy host can verify without the Yggdrasil tooling; casks are
// signed with `yggdrasil-evidence cask sign`.
//
// Exit codes: 0 verified, 1 refused, 2 usage or unreadable key.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
	"unheaded/pkg/sealedcask"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "verify":
		return cmdVerify(args[1:], false, stdout, stderr)
	case "extract":
		return cmdVerify(args[1:], true, stdout, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "cask-verify: unknown subcommand %q\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `cask-verify: verify (and safely extract) a signed Sealed Cask.

Usage:
  cask-verify verify  --cask <cask.tar.gz> --pubkey <build.pubkey> [--pubkey <old.pubkey>]
  cask-verify extract --cask <cask.tar.gz> --pubkey <build.pubkey> --dest <new dir>

Exit codes: 0 verified, 1 refused, 2 usage or unreadable key.`)
}

// pubkeys is a repeatable --pubkey flag.
type pubkeys []string

func (p *pubkeys) String() string     { return strings.Join(*p, ",") }
func (p *pubkeys) Set(v string) error { *p = append(*p, v); return nil }

func cmdVerify(args []string, extract bool, stdout, stderr io.Writer) int {
	name := "verify"
	if extract {
		name = "extract"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cask := fs.String("cask", "", "the cask tarball")
	dest := fs.String("dest", "", "extract: directory to create (must not exist)")
	var keys pubkeys
	fs.Var(&keys, "pubkey", "trusted ML-DSA-65 build public key (repeatable)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *cask == "" || len(keys) == 0 || extract != (*dest != "") {
		fmt.Fprintf(stderr, "%s: --cask and --pubkey are required", name)
		if extract {
			fmt.Fprint(stderr, ", and --dest")
		} else {
			fmt.Fprint(stderr, " (--dest only with extract)")
		}
		fmt.Fprintln(stderr, "; no positional args")
		return exitUsage
	}
	trusted := make([]*mldsa65.PublicKey, 0, len(keys))
	for _, k := range keys {
		pub, err := gungnir.LoadPublicKeyFile(k)
		if err != nil {
			fmt.Fprintf(stderr, "%s: public key: %v\n", name, err)
			return exitUsage
		}
		trusted = append(trusted, pub)
	}
	var (
		res *sealedcask.Result
		err error
	)
	if extract {
		res, err = sealedcask.ExtractFile(*cask, *dest, trusted, sealedcask.DefaultLimits)
	} else {
		res, err = sealedcask.VerifyFile(*cask, trusted, sealedcask.DefaultLimits)
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: %s: REFUSED: %v\n", name, *cask, err)
		return exitFail
	}
	fmt.Fprintf(stdout, "%s: %s: VERIFIED %s, %d files, Binding Rune sha256 %s, signed by build key %s\n",
		name, *cask, res.Root, res.Files, res.RuneSha256, res.KeyID)
	if extract {
		fmt.Fprintf(stdout, "%s: extracted to %s\n", name, *dest)
	}
	return exitOK
}
