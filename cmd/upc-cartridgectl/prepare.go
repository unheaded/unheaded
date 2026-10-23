// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/cartridge"
	"unheaded/pkg/gungnir"
)

// Refusals (ADR-108 threat model: only signed, validated images load).
var (
	ErrUnsigned = errors.New("cartridge has no seal (use --lab to load unsigned images in a lab)")
	ErrNoKey    = errors.New("no pinned public key (--pubkey) to verify the seal")
	ErrSeal     = errors.New("seal is not a Gungnir seal")
	ErrInvalid  = errors.New("cartridge refused by the validator")
)

// Validator checks an image's exact bytes (the real one pipes them to
// `cart validate -`).
type Validator func(raw []byte) error

// Prepared is an image that passed every check, carrying the very bytes the
// checks ran on.
type Prepared struct {
	Raw    []byte
	Image  *cartridge.Image
	SHA256 string
	KeyID  string
	Lab    bool
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Prepare runs the load-time checks on raw, in order, all on the same
// in-memory bytes the caller will write to the maps (no file is re-read
// between a check and the write, so nothing can be swapped in between):
//  1. the Gungnir ML-DSA-65 seal verifies against the pinned key (expiry
//     included) — or, with lab, an unsigned image is accepted and marked;
//  2. the validator (`cart validate -`) accepts the bytes;
//  3. the bytes decode as an ABI v0 image.
func Prepare(raw, sealJSON []byte, pub *mldsa65.PublicKey, lab bool, validate Validator) (*Prepared, error) {
	p := &Prepared{Raw: raw, SHA256: sha256Hex(raw)}
	switch {
	case sealJSON == nil && lab:
		p.Lab = true
	case sealJSON == nil:
		return nil, ErrUnsigned
	case pub == nil:
		return nil, ErrNoKey
	default:
		var seal gungnir.GungnirSeal
		if err := json.Unmarshal(sealJSON, &seal); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSeal, err)
		}
		if err := gungnir.Verify(raw, &seal, pub); err != nil {
			return nil, fmt.Errorf("seal: %w", err)
		}
		p.KeyID = gungnir.KeyID(pub)
	}
	if err := validate(raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	img, err := cartridge.DecodeImage(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	p.Image = img
	return p, nil
}

// cartValidator runs `cart validate -` with raw on stdin.
func cartValidator(cartBin string) Validator {
	return func(raw []byte) error {
		cmd := exec.Command(cartBin, "validate", "-") // #nosec G204 -- operator-configured tool path, fixed arguments
		cmd.Stdin = bytes.NewReader(raw)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s", bytes.TrimSpace(out))
		}
		return nil
	}
}
