// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"errors"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go/config"
)

// ErrNoCredentials means none of the SDK's credential sources is set.
// Credentials are never stored in this repository.
var ErrNoCredentials = errors.New("no Anthropic credentials: set ANTHROPIC_API_KEY in the environment, or run `ant auth login` to create a profile (credentials are never stored in this repo)")

// CredentialSource names the credential the SDK will use, following the
// SDK's own resolution order (ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN,
// ANTHROPIC_PROFILE, workload identity federation, the active `ant auth
// login` profile). It never returns the credential itself. It fails when
// there is none, so a caller can stop before building a request instead
// of learning at send time.
func CredentialSource() (string, error) {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "env ANTHROPIC_API_KEY", nil
	}
	if os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" {
		return "env ANTHROPIC_AUTH_TOKEN", nil
	}
	if p := os.Getenv("ANTHROPIC_PROFILE"); p != "" {
		if _, err := config.LoadProfile(config.DefaultDir(), p); err != nil {
			return "", fmt.Errorf("ANTHROPIC_PROFILE=%q: %w", p, err)
		}
		return "profile " + p, nil
	}
	if os.Getenv("ANTHROPIC_FEDERATION_RULE_ID") != "" {
		// The SDK validates the rest of the federation variables itself.
		return "workload identity federation", nil
	}
	if _, err := config.LoadConfig(); err == nil {
		return "ant auth profile", nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("anthropic profile: %w", err)
	}
	return "", ErrNoCredentials
}
