// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

// cask sign | cask verify: the Sealed Cask's Binding Rune as a gungnir
// evidence seal (S-IRON-GATE Phase 13, ADR-010 amendment, pkg/sealedcask).
// The same build key that signs evidence packs signs casks, under its own
// purpose (sealed-cask-v1), so a cask seal never verifies as a pack seal.
// cmd/cask-verify is the standalone verifier for deploy hosts.

import (
	"fmt"
	"io"
	"os"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
	"unheaded/pkg/sealedcask"
)

func cmdCask(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "cask: expected `cask sign` or `cask verify`")
		return exitUsage
	}
	switch args[0] {
	case "sign":
		return cmdCaskSign(args[1:], stdout, stderr)
	case "verify":
		return cmdCaskVerify(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "cask: unknown subcommand %q (sign or verify)\n", args[0])
		return exitUsage
	}
}

func cmdCaskSign(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("cask sign", stderr)
	key := fs.String("key", "", "ML-DSA-65 build key seed (32 bytes, owner-only mode); required")
	manifest := fs.String("manifest", "", "the cask's "+sealedcask.ManifestName)
	out := fs.String("out", "", "signature file to write ("+sealedcask.SigName+")")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *key == "" || *manifest == "" || *out == "" {
		fmt.Fprintln(stderr, "cask sign: --key, --manifest and --out are required, no positional args")
		return exitUsage
	}
	data, err := readBounded(*manifest, sealedcask.DefaultLimits.MaxManifest)
	if err != nil {
		fmt.Fprintf(stderr, "cask sign: %v\n", err)
		return exitUsage
	}
	signer, err := loadSigner(*key)
	if err != nil {
		fmt.Fprintf(stderr, "cask sign: %v\n", err)
		return exitFail
	}
	seal, err := sealedcask.Sign(data, signer)
	if err != nil {
		fmt.Fprintf(stderr, "cask sign: refusing: %v\n", err)
		return exitFail
	}
	parsed, err := gungnir.ParseEvidenceSeal(seal)
	if err != nil {
		fmt.Fprintf(stderr, "cask sign: %v\n", err)
		return exitFail
	}
	if err := os.WriteFile(*out, seal, 0o644); err != nil { // #nosec G306 G703 -- public signature; operator-named path
		fmt.Fprintf(stderr, "cask sign: %v\n", err)
		return exitFail
	}
	fmt.Fprintf(stdout, "cask sign: %s (%s) -> %s, build key %s\n", *manifest, sealedcask.Purpose, *out, parsed.KeyId)
	return exitOK
}

func cmdCaskVerify(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("cask verify", stderr)
	cask := fs.String("cask", "", "the cask tarball")
	pubkey := fs.String("pubkey", defaultPubkey, "ML-DSA-65 build public key")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *cask == "" {
		fmt.Fprintln(stderr, "cask verify: --cask is required, no positional args")
		return exitUsage
	}
	pub, err := gungnir.LoadPublicKeyFile(*pubkey)
	if err != nil {
		fmt.Fprintf(stderr, "cask verify: public key: %v\n", err)
		return exitUsage
	}
	res, err := sealedcask.VerifyFile(*cask, []*mldsa65.PublicKey{pub}, sealedcask.DefaultLimits)
	if err != nil {
		fmt.Fprintf(stderr, "cask verify: %s: REFUSED: %v\n", *cask, err)
		return exitFail
	}
	fmt.Fprintf(stdout, "cask verify: %s: VERIFIED %s, %d files, Binding Rune sha256 %s, signed by build key %s\n",
		*cask, res.Root, res.Files, res.RuneSha256, res.KeyID)
	return exitOK
}
