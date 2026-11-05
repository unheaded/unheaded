// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package store

import "unheaded/pkg/secrets"

// NotifyWatchers lets a test drive the send path directly.
func (s *VaultStore) NotifyWatchers(path string, secret *secrets.Secret) {
	s.notifyWatchers(path, secret)
}
