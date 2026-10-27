// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
)

// createMLDSA writes a new ML-DSA-65 key: the 32-byte seed gungnir.LoadSeedFile
// reads (0400, chowned like a service key) and the packed public key (0444).
// It prints the key id and the base64 public key Wotan's signed_prefixes
// takes; the seed is never printed.
func createMLDSA(o options, uid, gid int, stdout, stderr io.Writer) int {
	keyPath := filepath.Join(o.dir, o.name+".key")
	pubPath := filepath.Join(o.dir, o.name+".pub")
	if _, err := os.Lstat(keyPath); err == nil {
		return fail(stderr, refusal{fmt.Sprintf("key exists: %s (a signing key is never replaced; make one under a new name)", keyPath)})
	}
	if err := ensureDir(o.dir, keyDirMode); err != nil {
		return fail(stderr, err)
	}
	var seed [mldsa65.SeedSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return fail(stderr, fmt.Errorf("random seed: %w", err))
	}
	pub, _ := mldsa65.NewKeyFromSeed(&seed)
	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		return fail(stderr, err)
	}
	// Public half first: a .pub without its .key is derived data the next
	// run replaces; a .key without its .pub is never left behind.
	if err := writeAtomic(pubPath, pubBytes, pubMode); err != nil {
		return fail(stderr, err)
	}
	if err := writeAtomic(keyPath, seed[:], 0o400); err != nil {
		return fail(stderr, err)
	}
	clear(seed[:])
	if uid >= 0 {
		if err := os.Chown(keyPath, uid, gid); err != nil {
			return fail(stderr, fmt.Errorf("chown %s: %w", keyPath, err))
		}
	}
	fmt.Fprintf(stdout, "%s mldsa65 kid %s\n  key    %s (0400)\n  pub    %s\n  public %s\n",
		o.name, gungnir.KeyID(pub), keyPath, pubPath, base64.StdEncoding.EncodeToString(pubBytes))
	fmt.Fprintln(stdout, "  enrol: add the public line's base64 to the prefix's keys in Wotan's topics.signed_prefixes and restart Wotan")
	return exitOK
}

// printMLDSA prints an ML-DSA-65 key's id and base64 public key, from the
// .pub file or, failing that, from the seed.
func printMLDSA(o options, stdout, stderr io.Writer) int {
	pubPath := filepath.Join(o.dir, o.name+".pub")
	pub, err := gungnir.LoadPublicKeyFile(pubPath)
	if err != nil {
		var serr error
		if pub, serr = gungnir.PublicKeyFromSeed(filepath.Join(o.dir, o.name+".key")); serr != nil {
			return fail(stderr, errors.Join(err, serr))
		}
	}
	fmt.Fprintf(stdout, "%s mldsa65 kid %s\n  public %s\n", o.name, gungnir.KeyID(pub), base64.StdEncoding.EncodeToString(pub.Bytes()))
	return exitOK
}
