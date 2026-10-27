// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import "unheaded/pkg/zhencapture"

// redactRow returns a copy of row with every string, at any depth, passed
// through the shared redactor (pkg/zhencapture/redact_patterns.json:
// secrets, tokens, keys, URL passwords, email addresses), and the number
// of replacements. Keys, numbers, booleans and nulls are kept. Nothing an
// archive class reads reaches retention_archive without this.
func redactRow(row map[string]any) (map[string]any, int) {
	n := 0
	out, _ := redactValue(row, &n).(map[string]any)
	return out, n
}

func redactValue(v any, n *int) any {
	switch x := v.(type) {
	case string:
		s, k := zhencapture.Redact(x)
		*n += k
		return s
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[k] = redactValue(e, n)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, e := range x {
			s[i] = redactValue(e, n)
		}
		return s
	default:
		return v
	}
}
