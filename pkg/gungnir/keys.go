// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package gungnir

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

// Key files and longer-lived seals, for signed compliance snapshots
// (ADR-099 §2). A private key is stored as its 32-byte seed and loaded only
// from a file no one but its owner can read. Where the file lives (TPM,
// sealed storage, an offline machine) is the operator's decision, per
// ADR-043 condition #4; nothing here generates or commits a key.

// ErrKeyFilePerms refuses a private key file group or others can read.
var ErrKeyFilePerms = errors.New("gungnir: private key file is readable by group or others")

// KeyID names a public key: the first 16 hex digits of its SHA-256.
func KeyID(pub *mldsa65.PublicKey) string {
	h := sha256.Sum256(pub.Bytes())
	return hex.EncodeToString(h[:8])
}

// NewSignerWithID is NewSigner with the key's real ID instead of "dev-spike".
func NewSignerWithID(priv *mldsa65.PrivateKey) (*Signer, error) {
	s, err := NewSigner(priv)
	if err != nil {
		return nil, err
	}
	s.keyID = KeyID(s.pub)
	return s, nil
}

// SignFor seals payload with an expiry ttl from now (Sign keeps its one
// hour). ttl must be positive and at most 31 days: a snapshot must be
// re-issued, not trusted forever.
func (s *Signer) SignFor(payload []byte, ttl time.Duration) (*GungnirSeal, error) {
	if ttl <= 0 || ttl > 31*24*time.Hour {
		return nil, fmt.Errorf("gungnir: ttl %s outside (0, 31 days]", ttl)
	}
	return s.sign(payload, ttl)
}

// LoadSeedFile reads a private key seed file (32 bytes), refusing one that
// group or others can read and anything but a regular file. The checks run on
// the open file, not the path, so the bytes read are the bytes checked (no
// swap between stat and read), and a FIFO at the path cannot block the open
// (LICH-012 L12a, 2026-10-30).
func LoadSeedFile(path string) (*mldsa65.PrivateKey, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0) // #nosec G304 -- operator-named key file, checked below
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("gungnir: %s is not a regular file", path)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: %s is %v", ErrKeyFilePerms, path, fi.Mode().Perm())
	}
	b, err := io.ReadAll(io.LimitReader(f, mldsa65.SeedSize+1))
	if err != nil {
		return nil, err
	}
	defer clear(b)
	if len(b) != mldsa65.SeedSize {
		if len(b) > mldsa65.SeedSize {
			return nil, fmt.Errorf("gungnir: %s is larger than a %d-byte seed", path, mldsa65.SeedSize)
		}
		return nil, fmt.Errorf("gungnir: %s is %d bytes, want a %d-byte seed", path, len(b), mldsa65.SeedSize)
	}
	var seed [mldsa65.SeedSize]byte
	copy(seed[:], b)
	_, priv := mldsa65.NewKeyFromSeed(&seed)
	clear(seed[:])
	return priv, nil
}

// LoadPublicKeyFile reads a packed ML-DSA-65 public key.
func LoadPublicKeyFile(path string) (*mldsa65.PublicKey, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- operator-named public key file
	if err != nil {
		return nil, err
	}
	var pub mldsa65.PublicKey
	if err := pub.UnmarshalBinary(b); err != nil {
		return nil, fmt.Errorf("gungnir: %s: %w", path, err)
	}
	return &pub, nil
}
