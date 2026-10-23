// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build privacy_bite

package privacy

// biteNoop under the privacy_bite tag disables redaction. Never build a
// binary with this tag: it exists so the tests can be seen failing.
const biteNoop = true
