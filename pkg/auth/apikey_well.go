// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"errors"
)

func openWellAPIKeyStore(context.Context, ...APIKeyStoreOption) (APIKeyStore, error) {
	return nil, errors.New("auth: AUTH_APIKEY_STORE=well is not available yet")
}
