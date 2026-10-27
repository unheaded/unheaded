// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import "testing"

// FuzzTrustLabel: no byte string other than the three listed labels may
// be trusted; the three listed labels are always trusted.
func FuzzTrustLabel(f *testing.F) {
	for _, s := range []string{"canonical", "local", "direct-user", "external", "", "Canonical", "local\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, label string) {
		tc := ToolCall{Name: "write_file", Args: map[string]any{"path": "/x"}, Justification: []Reference{{SourceTrust: label}}}
		got := tc.HasUntrustedJustification()
		want := !TrustedSourceLabels[label]
		if got != want {
			t.Fatalf("label %q: untrusted=%v want %v", label, got, want)
		}
		// A trusted reference never launders an untrusted one, whatever
		// the order.
		mixed := ToolCall{Name: "write_file", Justification: []Reference{{SourceTrust: "canonical"}, {SourceTrust: label}}}
		if mixed.HasUntrustedJustification() != want {
			t.Fatalf("chain [canonical, %q]: untrusted=%v want %v", label, !want, want)
		}
	})
}
