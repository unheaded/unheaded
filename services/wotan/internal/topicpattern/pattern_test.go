// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package topicpattern

import "testing"

// Match's rules are covered through the grpc package's wrapper tests.
func TestIsPattern(t *testing.T) {
	for s, want := range map[string]bool{
		"alerts.critical": false, "alerts.*": true, "alerts.#": true, "*": true, "a-b_c.1": false,
	} {
		if got := IsPattern(s); got != want {
			t.Errorf("IsPattern(%q) = %v, want %v", s, got, want)
		}
	}
}
