// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/champion"
)

// H3: what `seal-runbooks` writes is what the gate accepts, and it needs the
// seed, not the public key, so nothing the daemon holds can make one.
func TestSealRunbooks_TheGateAcceptsWhatItWrites(t *testing.T) {
	root := t.TempDir()
	body := []byte("metadata:\n  name: x\nsteps: []\n")
	sum := sha256.Sum256(body)
	for p, b := range map[string][]byte{
		"runbooks/x.yaml":          body,
		"runbooks/MANIFEST.sha256": []byte(hex.EncodeToString(sum[:]) + "  x.yaml\n"),
		"scripts/run-runbook.py":   []byte("print('ran')\n"),
	} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var seed [mldsa65.SeedSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		t.Fatal(err)
	}
	keys := t.TempDir()
	seedPath := filepath.Join(keys, "runbook-manifest.key")
	if err := os.WriteFile(seedPath, seed[:], 0o600); err != nil {
		t.Fatal(err)
	}
	pub, _ := mldsa65.NewKeyFromSeed(&seed)
	pubPath := filepath.Join(keys, "runbook-manifest.pub")
	if err := os.WriteFile(pubPath, pub.Bytes(), 0o444); err != nil {
		t.Fatal(err)
	}
	t.Setenv(champion.RunbookManifestKeyEnv, pubPath)

	c, err := champion.New(champion.Config{ProjectRoot: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RunbookExecute(context.Background(), "x", false); !errors.Is(err, champion.ErrRunbookManifestUnsealed) {
		t.Fatalf("before sealing: %v", err)
	}
	var out, errOut bytes.Buffer
	if rc := run([]string{"seal-runbooks", "-key", seedPath, "-project-root", root}, nil, &out, &errOut); rc != 0 {
		t.Fatalf("rc %d: %s", rc, errOut.String())
	}
	if _, err := c.RunbookExecute(context.Background(), "x", false); errors.Is(err, champion.ErrRunbookManifestUnsealed) {
		t.Fatalf("after sealing: %v", err)
	}
	if rc := run([]string{"seal-runbooks", "-key", pubPath, "-project-root", root}, nil, &out, &errOut); rc == 0 {
		t.Fatal("the public key sealed something")
	}
}
