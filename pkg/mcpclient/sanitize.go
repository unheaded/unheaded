// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package mcpclient

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// replacement stands in for every rune SanitizeText refuses to pass through.
// It is visible, so a reader can see that something was removed.
const replacement = '\uFFFD'

// SanitizeText makes untrusted server text safe to put in a model's context
// or print on a terminal. It keeps newline and tab, turns CRLF into LF, and
// replaces with U+FFFD:
//
//   - invalid UTF-8;
//   - C0 controls (including ESC, so no terminal escape sequences survive),
//     DEL, a lone CR, and C1 controls (U+0080-U+009F, 8-bit CSI among them);
//   - format characters (Unicode category Cf): bidi overrides and isolates
//     (Trojan Source), zero-width characters, BOM, soft hyphen, and the tag
//     block U+E0000-U+E007F, which carries invisible text a model can read;
//   - the line and paragraph separators U+2028 and U+2029.
//
// Ordinary printable text in any script passes through unchanged.
func SanitizeText(s string) string {
	if isClean(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size <= 1:
			b.WriteRune(replacement)
		case r == '\r' && i+1 < len(s) && s[i+1] == '\n':
			// CRLF: drop the CR, the LF is written next iteration.
		case allowed(r):
			b.WriteRune(r)
		default:
			b.WriteRune(replacement)
		}
		i += size
	}
	return b.String()
}

// isClean is the fast path: most tool output is plain text.
func isClean(s string) bool {
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c < 0x20 && c != '\n' && c != '\t' || c == 0x7f {
				return false
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 || !allowed(r) {
			return false
		}
		i += size
	}
	return true
}

func allowed(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return true
	case unicode.IsControl(r): // C0, DEL, C1
		return false
	case unicode.Is(unicode.Cf, r):
		return false
	case r == '\u2028' || r == '\u2029':
		return false
	}
	return true
}

// truncate cuts s to at most limit bytes without splitting a rune and says
// so. limit <= 0 means no limit.
func truncate(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n[truncated by mcpclient]"
}
