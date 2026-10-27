// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"os"
	"strings"

	"unheaded/services/wotan/internal/api"
)

// adminKeyEnv names the environment variable that carries the admin API key.
// It is read from the environment only, never a flag: flags show in ps.
const adminKeyEnv = "WOTAN_ADMIN_API_KEY"

var errAdminKeyUnset = errors.New(adminKeyEnv + " unset: set it or drop -admin")

// adminKeyFromEnv returns the admin key when admin endpoints are enabled.
// Enabled without a key is an error: the endpoints would refuse every
// request forever, which is what happened before S-CLEAN-SLATE.
func adminKeyFromEnv(enabled bool) (string, error) {
	if !enabled {
		return "", nil
	}
	key := strings.TrimSpace(os.Getenv(adminKeyEnv))
	if key == "" {
		return "", errAdminKeyUnset
	}
	return key, nil
}

// configureAdmin applies the admin key to s, or fails when -admin is on
// and no key is set. The handlers still refuse on an empty key (defence in
// depth), so a nil error with admin off leaves the endpoints unregistered
// and the key empty.
func configureAdmin(s *api.Server, enabled bool) error {
	key, err := adminKeyFromEnv(enabled)
	if err != nil {
		return err
	}
	if key != "" {
		s.SetAdminAPIKey(key)
	}
	return nil
}
