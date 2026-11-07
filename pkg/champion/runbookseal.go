// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
)

// The manifest sits in the tree it vouches for, so its bytes alone prove
// nothing about who wrote them (H3). A live run also needs them sealed: a
// gungnir evidence seal, MANIFEST.sha256.seal beside the manifest, made with a
// key this process never holds and verified with the public half, which must
// live outside anything the project root reaches.
//
//	unheaded-keygen -algo mldsa65 runbook-manifest
//	scripts/runbook-manifest.sh --write
//	scripts/runbook-manifest.sh --seal /path/to/runbook-manifest.key
//
// Editing a runbook unseals every live run until it is sealed again. A dry run
// runs nothing and needs no seal.
const (
	// RunbookSealSuffix follows RunbookManifestFile in the seal's name.
	RunbookSealSuffix = ".seal"
	// RunbookManifestKeyEnv overrides DefaultRunbookManifestKey.
	RunbookManifestKeyEnv = "ZHEN_RUNBOOK_MANIFEST_KEY"
	// DefaultRunbookManifestKey is where unheaded-keygen puts the public half.
	DefaultRunbookManifestKey = "/etc/unheaded/keys/runbook-manifest.pub"

	runbookSealPurpose = "runbook-manifest"
)

// ErrRunbookManifestUnsealed means no good seal from the configured key covers
// the manifest's bytes.
var ErrRunbookManifestUnsealed = errors.New("runbook manifest is not sealed by the configured key")

// SealRunbookManifest seals manifest bytes with key.
func SealRunbookManifest(manifest []byte, key *mldsa65.PrivateKey) ([]byte, error) {
	signer, err := gungnir.NewSignerWithID(key)
	if err != nil {
		return nil, err
	}
	seal, err := signer.SignEvidence(manifest, runbookSealPurpose)
	if err != nil {
		return nil, err
	}
	return json.Marshal(seal)
}

// manifestKey loads the public key, refusing one the sandbox could rewrite: a
// path under a root, as given or as resolved, or a file others may write.
func (c *Champion) manifestKey() (*mldsa65.PublicKey, error) {
	path := os.Getenv(RunbookManifestKeyEnv)
	if path == "" {
		path = DefaultRunbookManifestKey
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	for _, root := range append([]string{c.config.ProjectRoot}, c.config.AllowedPaths...) {
		for _, p := range []string{path, resolved} {
			if _, inside := underRoot(p, root); inside {
				return nil, fmt.Errorf("key %s is inside the sandbox", path)
			}
		}
	}
	dir, err := os.OpenRoot(filepath.Dir(resolved))
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }() // read-only
	f, err := dir.Open(filepath.Base(resolved))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only
	// Judged on the open file, so what passes is what is read.
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("key %s is writable by others (%v)", path, fi.Mode().Perm())
	}
	raw, err := io.ReadAll(io.LimitReader(f, mldsa65.PublicKeySize+1))
	if err != nil {
		return nil, err
	}
	var pub mldsa65.PublicKey
	if err := pub.UnmarshalBinary(raw); err != nil {
		return nil, fmt.Errorf("key %s: %w", path, err)
	}
	return &pub, nil
}

// checkManifestSeal verifies the seal over exactly the bytes the caller read.
func (c *Champion) checkManifestSeal(runbooksDir string, manifest []byte) error {
	pub, err := c.manifestKey()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRunbookManifestUnsealed, err)
	}
	dir, err := os.OpenRoot(runbooksDir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRunbookManifestUnsealed, err)
	}
	defer func() { _ = dir.Close() }() // read-only
	f, err := dir.Open(RunbookManifestFile + RunbookSealSuffix)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRunbookManifestUnsealed, err)
	}
	defer func() { _ = f.Close() }() // read-only
	raw, err := io.ReadAll(io.LimitReader(f, gungnir.MaxEvidenceSealBytes+1))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRunbookManifestUnsealed, err)
	}
	seal, err := gungnir.ParseEvidenceSeal(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRunbookManifestUnsealed, err)
	}
	if err := gungnir.VerifyEvidence(manifest, seal, pub, runbookSealPurpose); err != nil {
		return fmt.Errorf("%w: %v", ErrRunbookManifestUnsealed, err)
	}
	return nil
}
