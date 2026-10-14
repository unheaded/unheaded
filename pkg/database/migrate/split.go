// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package migrate

import (
	"fmt"
	"strings"
)

// Split cuts SQL into statements at top-level semicolons. It knows enough
// of PostgreSQL's lexer not to cut inside string literals (a doubled
// quote escapes one; E-strings also take backslash escapes), quoted
// identifiers, $tag$ dollar quotes, -- comments and nested /* */ comments.
// Empty statements are dropped. It is used for notx files, whose statements
// must each be sent alone: several statements in one simple query run as
// one implicit transaction, which CREATE DATABASE refuses.
func Split(sql string) ([]string, error) {
	var out []string
	start, depth := 0, 0
	emit := func(end int) {
		if s := strings.TrimSpace(sql[start:end]); s != "" && !onlyComments(s) {
			out = append(out, s)
		}
		start = end + 1
	}
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case depth > 0:
			if c == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				depth--
				i++
			} else if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
				depth++
				i++
			}
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			depth = 1
			i++
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			nl := strings.IndexByte(sql[i:], '\n')
			if nl < 0 {
				i = len(sql)
			} else {
				i += nl
			}
		case c == '\'':
			backslash := i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') && (i < 2 || !isIdent(sql[i-2]))
			j := i + 1
			for ; j < len(sql); j++ {
				if backslash && sql[j] == '\\' {
					j++
					continue
				}
				if sql[j] == '\'' {
					if j+1 < len(sql) && sql[j+1] == '\'' {
						j++
						continue
					}
					break
				}
			}
			if j >= len(sql) {
				return nil, fmt.Errorf("unterminated string at byte %d", i)
			}
			i = j
		case c == '"':
			j := strings.IndexByte(sql[i+1:], '"')
			if j < 0 {
				return nil, fmt.Errorf("unterminated identifier at byte %d", i)
			}
			i += j + 1
		case c == '$' && (i == 0 || !isIdent(sql[i-1])):
			tag, ok := dollarTag(sql[i:])
			if !ok {
				continue
			}
			end := strings.Index(sql[i+len(tag):], tag)
			if end < 0 {
				return nil, fmt.Errorf("unterminated %s quote at byte %d", tag, i)
			}
			i += len(tag) + end + len(tag) - 1
		case c == ';':
			emit(i)
		}
	}
	if depth > 0 {
		return nil, fmt.Errorf("unterminated /* comment")
	}
	emit(len(sql))
	return out, nil
}

// dollarTag returns the opening $tag$ at the start of s, if s starts one.
func dollarTag(s string) (string, bool) {
	for j := 1; j < len(s); j++ {
		switch c := s[j]; {
		case c == '$':
			return s[:j+1], true
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || j > 1 && c >= '0' && c <= '9':
		default:
			return "", false
		}
	}
	return "", false
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// onlyComments reports whether s holds nothing but -- comments.
func onlyComments(s string) bool {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "--") {
			return false
		}
	}
	return true
}
