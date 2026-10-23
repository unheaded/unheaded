// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build !privacy_bite

package privacy

// biteNoop is true only under the privacy_bite build tag, which turns the
// redactor into a no-op so the tests can be seen failing (Step 112).
const biteNoop = false
