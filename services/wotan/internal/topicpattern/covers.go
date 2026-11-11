// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package topicpattern

import "strings"

// Covers reports whether every topic that matches pattern also matches
// allow: the test a subscription pattern must pass against an allow-list
// entry (FND-043). It is decided on the patterns themselves, never by
// enumerating topics, so a subscription to "tasks.#" is refused by an
// allowance of "tasks.*" (depth differs) and by "tasks.created" (one topic),
// while "tasks.created" is covered by "tasks.*", "tasks.#" and "#".
//
// Rules, segment by segment, dots apart:
//   - "*" or "#" alone as allow covers every valid pattern.
//   - An allow segment "#" (last) covers whatever remains, if anything
//     remains: Match requires at least one segment there.
//   - An allow segment "*" covers one pattern segment that is a literal or
//     "*"; it does not cover "#", which can span several.
//   - A literal allow segment covers only the same literal.
//   - Both must end together; a pattern with segments left over, or an
//     allow with literal or "*" segments left over, is not covered.
//
// Empty or invalid strings are never covered.
func Covers(allow, pattern string) bool {
	if !ValidPattern(allow) || !ValidPattern(pattern) {
		return false
	}
	if allow == "*" || allow == "#" {
		return true
	}
	as := strings.Split(allow, ".")
	ps := strings.Split(pattern, ".")
	i := 0
	for ; i < len(as); i++ {
		a := as[i]
		if a == "#" {
			// Only a trailing "#" is a multi-level wildcard; elsewhere
			// Match would never accept it, so nothing is covered.
			return i == len(as)-1 && i < len(ps)
		}
		if i >= len(ps) {
			return false
		}
		p := ps[i]
		switch {
		case p == "#":
			return false
		case a == "*":
			// one segment, literal or "*"
		case a != p:
			return false
		}
	}
	return i == len(ps)
}
