// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package doom

import (
	"errors"
	"os"
)

// ResolveHMACKey returns the ROM HMAC key: flagValue if set, else the
// UPC_HMAC_KEY environment variable. There is no default (S-ASCEND-2 Step
// 169): a key shipped in the binary is a public key, and a missing key must
// stop the command rather than sign with something everyone knows.
func ResolveHMACKey(flagValue string) ([]byte, error) {
	if flagValue != "" {
		return []byte(flagValue), nil
	}
	if v := os.Getenv("UPC_HMAC_KEY"); v != "" {
		return []byte(v), nil
	}
	return nil, errors.New("UPC_HMAC_KEY is required; credentials are not stored in this repo (or pass --hmac-key)")
}
