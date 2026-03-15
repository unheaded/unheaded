// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package routing provides traffic routing for the service mesh.
// THE HAUBERK - Request matching for the Unheaded Kingdom.
package routing

import (
	"regexp"
	"strings"
)

// MatchType represents the type of match.
type MatchType int

const (
	// MatchExact requires exact string match.
	MatchExact MatchType = iota
	// MatchPrefix requires prefix match.
	MatchPrefix
	// MatchRegex requires regex match.
	MatchRegex
	// MatchPresent only checks for presence.
	MatchPresent
)

// Match defines a request match condition.
type Match struct {
	// URI matches the request path.
	URI *StringMatch
	// Method matches the HTTP method.
	Method *StringMatch
	// Headers matches request headers.
	Headers map[string]*StringMatch
	// SourceLabels matches source service labels.
	SourceLabels map[string]string
	// SourceNamespace matches source namespace.
	SourceNamespace *StringMatch
}

// Matches checks if a request matches this condition.
func (m *Match) Matches(req *Request) bool {
	// Check URI
	if m.URI != nil && !m.URI.Matches(req.Path) {
		return false
	}

	// Check method
	if m.Method != nil && !m.Method.Matches(req.Method) {
		return false
	}

	// Check headers
	for name, sm := range m.Headers {
		value, exists := req.Headers[name]
		if sm.Type == MatchPresent {
			if !exists {
				return false
			}
		} else if !sm.Matches(value) {
			return false
		}
	}

	// Check source namespace
	if m.SourceNamespace != nil && !m.SourceNamespace.Matches(req.SourceNamespace) {
		return false
	}

	return true
}

// StringMatch defines string matching criteria.
type StringMatch struct {
	// Type is the match type.
	Type MatchType
	// Value is the match value.
	Value string
	// regex is compiled for regex matches.
	regex *regexp.Regexp
}

// NewExactMatch creates an exact string match.
func NewExactMatch(value string) *StringMatch {
	return &StringMatch{
		Type:  MatchExact,
		Value: value,
	}
}

// NewPrefixMatch creates a prefix match.
func NewPrefixMatch(prefix string) *StringMatch {
	return &StringMatch{
		Type:  MatchPrefix,
		Value: prefix,
	}
}

// NewRegexMatch creates a regex match.
func NewRegexMatch(pattern string) (*StringMatch, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	return &StringMatch{
		Type:  MatchRegex,
		Value: pattern,
		regex: re,
	}, nil
}

// NewPresentMatch creates a presence check match.
func NewPresentMatch() *StringMatch {
	return &StringMatch{
		Type: MatchPresent,
	}
}

// Matches checks if a string matches.
func (sm *StringMatch) Matches(s string) bool {
	switch sm.Type {
	case MatchExact:
		return s == sm.Value
	case MatchPrefix:
		return strings.HasPrefix(s, sm.Value)
	case MatchRegex:
		if sm.regex == nil {
			// Try to compile if not already compiled
			re, err := regexp.Compile(sm.Value)
			if err != nil {
				return false
			}
			sm.regex = re
		}
		return sm.regex.MatchString(s)
	case MatchPresent:
		return s != ""
	default:
		return false
	}
}

// MatchBuilder helps build match conditions.
type MatchBuilder struct {
	match *Match
}

// NewMatchBuilder creates a new match builder.
func NewMatchBuilder() *MatchBuilder {
	return &MatchBuilder{
		match: &Match{
			Headers: make(map[string]*StringMatch),
		},
	}
}

// URI sets URI match.
func (b *MatchBuilder) URI(sm *StringMatch) *MatchBuilder {
	b.match.URI = sm
	return b
}

// URIExact sets exact URI match.
func (b *MatchBuilder) URIExact(path string) *MatchBuilder {
	b.match.URI = NewExactMatch(path)
	return b
}

// URIPrefix sets prefix URI match.
func (b *MatchBuilder) URIPrefix(prefix string) *MatchBuilder {
	b.match.URI = NewPrefixMatch(prefix)
	return b
}

// Method sets method match.
func (b *MatchBuilder) Method(method string) *MatchBuilder {
	b.match.Method = NewExactMatch(method)
	return b
}

// Header adds a header match.
func (b *MatchBuilder) Header(name string, sm *StringMatch) *MatchBuilder {
	b.match.Headers[name] = sm
	return b
}

// HeaderExact adds an exact header match.
func (b *MatchBuilder) HeaderExact(name, value string) *MatchBuilder {
	b.match.Headers[name] = NewExactMatch(value)
	return b
}

// HeaderPresent adds a header presence check.
func (b *MatchBuilder) HeaderPresent(name string) *MatchBuilder {
	b.match.Headers[name] = NewPresentMatch()
	return b
}

// SourceNamespace sets source namespace match.
func (b *MatchBuilder) SourceNamespace(ns string) *MatchBuilder {
	b.match.SourceNamespace = NewExactMatch(ns)
	return b
}

// Build returns the built match.
func (b *MatchBuilder) Build() *Match {
	return b.match
}

// PathMatcher provides path matching utilities.
type PathMatcher struct {
	segments []pathSegment
}

type pathSegment struct {
	value    string
	isParam  bool
	isWild   bool
}

// NewPathMatcher creates a new path matcher from a pattern.
// Pattern format: /users/:id/posts/* where :id is a parameter and * is wildcard
func NewPathMatcher(pattern string) *PathMatcher {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	segments := make([]pathSegment, len(parts))

	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			segments[i] = pathSegment{value: part[1:], isParam: true}
		} else if part == "*" {
			segments[i] = pathSegment{isWild: true}
		} else {
			segments[i] = pathSegment{value: part}
		}
	}

	return &PathMatcher{segments: segments}
}

// Match checks if a path matches and extracts parameters.
func (pm *PathMatcher) Match(path string) (bool, map[string]string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	params := make(map[string]string)

	if len(parts) != len(pm.segments) {
		// Check for trailing wildcard
		if len(pm.segments) > 0 && pm.segments[len(pm.segments)-1].isWild {
			if len(parts) < len(pm.segments)-1 {
				return false, nil
			}
		} else {
			return false, nil
		}
	}

	for i, seg := range pm.segments {
		if i >= len(parts) {
			break
		}

		if seg.isWild {
			// Wildcard matches remaining path
			params["*"] = strings.Join(parts[i:], "/")
			return true, params
		}

		if seg.isParam {
			params[seg.value] = parts[i]
		} else if seg.value != parts[i] {
			return false, nil
		}
	}

	return true, params
}
