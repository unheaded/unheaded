// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package mcpclient

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeText(t *testing.T) {
	const R = "\uFFFD"
	cases := []struct {
		name, in, want string
	}{
		{"plain", "hello world", "hello world"},
		{"keeps newline and tab", "a\n\tb", "a\n\tb"},
		{"non-latin passes", "Zhen \u771F \u2014 ok \u2713 \u00FCn\u00EFcode", "Zhen \u771F \u2014 ok \u2713 \u00FCn\u00EFcode"},
		{"crlf to lf", "a\r\nb", "a\nb"},
		{"lone cr", "abc\rxyz", "abc" + R + "xyz"},
		{"ansi escape", "\x1b[31mred\x1b[0m", R + "[31mred" + R + "[0m"},
		{"osc title", "\x1b]0;pwned\x07", R + "]0;pwned" + R},
		{"nul and bell", "a\x00b\x07", "a" + R + "b" + R},
		{"del", "a\x7fb", "a" + R + "b"},
		{"c1 csi", "a\u009b31mb", "a" + R + "31mb"},
		{"bidi override", "a\u202Eevil", "a" + R + "evil"},
		{"bidi isolate", "\u2066x\u2069", R + "x" + R},
		{"zero width", "a\u200Bb\u200Dc", "a" + R + "b" + R + "c"},
		{"bom", "\uFEFFhi", R + "hi"},
		{"tag block smuggling", "ok\U000E0049\U000E0047", "ok" + R + R},
		{"line separator", "a\u2028b\u2029c", "a" + R + "b" + R + "c"},
		{"invalid utf8", "a\xffb\xc3", "a" + R + "b" + R},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeText(tc.in)
			if got != tc.want {
				t.Fatalf("SanitizeText(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if again := SanitizeText(got); again != got {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func FuzzSanitizeText(f *testing.F) {
	f.Add("plain")
	f.Add("\x1b[2J\r\n\u202E\U000E0041\xff")
	f.Fuzz(func(t *testing.T, s string) {
		out := SanitizeText(s)
		if !utf8.ValidString(out) {
			t.Fatalf("invalid utf8 out of %q", s)
		}
		for _, r := range out {
			if r != replacement && !allowed(r) {
				t.Fatalf("disallowed rune %U survived %q", r, s)
			}
		}
		if strings.Contains(out, "\x1b") || strings.Contains(out, "\r") {
			t.Fatalf("escape or CR survived %q", s)
		}
	})
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("got %q", got)
	}
	if got := truncate("anything", 0); got != "anything" {
		t.Fatalf("got %q", got)
	}
	got := truncate("ab\u771Fcd", 3) // cut lands inside the 3-byte rune
	if !strings.HasPrefix(got, "ab\n") || !utf8.ValidString(got) {
		t.Fatalf("rune split: %q", got)
	}
	if !strings.HasSuffix(got, "[truncated by mcpclient]") {
		t.Fatalf("no marker: %q", got)
	}
}
