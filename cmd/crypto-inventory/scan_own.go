// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Cryptographic primitives written in this repository instead of taken from
// a library (backlog row D-34). There are few, each for a stated reason, and
// each is a row in the CBOM so an export or security review finds it there.
const (
	kindOwnPrimitive = "own-primitive"
	ruleOwnPrimitive = "own-primitive-unlisted" // SHA-256 round constants in a file the table does not list
)

// sha256RoundConstant matches the first SHA-256 round constant (FIPS 180-4
// section 4.2.2) as a hex literal, in either case. Any implementation of
// SHA-256 has to spell it. The pattern is written in two halves so that this
// file does not hold the literal and trip its own lint.
var sha256RoundConstant = regexp.MustCompile(`(?i)0x428a` + `2f98`)

// ownPrimitive is one listed implementation. marker finds the line the row
// points at; TestOwnPrimitiveTableMatchesRepo fails when the file is gone or
// no longer holds it.
type ownPrimitive struct {
	path, name, primitive, param, detail string
	marker                               *regexp.Regexp
}

// ownPrimitives is the table. A new entry is a decision, not a formality:
// say why no library fits, and tell whoever keeps the export record.
var ownPrimitives = []ownPrimitive{
	{
		path: "crates/upc-api/src/surface_hash.rs", name: "SHA-256 (hand-written, FIPS 180-4)",
		primitive: primHash, param: "SHA-256", marker: sha256RoundConstant,
		detail: "the algorithm is the standard's and the implementation is this repository's: the eBPF workspace has no hashing crate",
	},
	{
		path: "services/sophia/hkdf_derive.go", name: "HKDF-SHA-256 (hand-written on crypto/hmac, RFC 5869)",
		primitive: primKDF, param: "HKDF-SHA-256", marker: regexp.MustCompile(`(?m)^func hkdfExtract\(`),
		detail: "extract and expand written here on the standard library's HMAC",
	},
}

func ownListed(rel string) bool {
	for _, p := range ownPrimitives {
		if p.path == rel {
			return true
		}
	}
	return false
}

// sourceExts are the files the tripwire reads. Prose, data and generated
// documents may quote the constant; code may not hold it unlisted.
var sourceExts = map[string]bool{
	".go": true, ".rs": true, ".c": true, ".h": true, ".cc": true, ".cpp": true, ".hpp": true,
	".py": true, ".js": true, ".ts": true, ".zig": true, ".s": true, ".S": true, ".asm": true,
}

// isShippedSource reports a source file that is not a test: Go _test.go
// files and Rust tests, benches and examples may carry test vectors.
func isShippedSource(rel string) bool {
	if !sourceExts[path.Ext(rel)] {
		return false
	}
	if strings.HasSuffix(rel, "_test.go") {
		return false
	}
	if strings.HasSuffix(rel, ".rs") {
		p := "/" + rel
		if strings.Contains(p, "/tests/") || strings.Contains(p, "/benches/") || strings.Contains(p, "/examples/") {
			return false
		}
	}
	return true
}

// scanOwnPrimitives emits one row per listed implementation present in the
// tree, and one row per unlisted source file that holds the SHA-256 round
// constants. The second kind breaks ruleOwnPrimitive.
func scanOwnPrimitives(t *Tree) ([]Asset, error) {
	var out []Asset
	base := Asset{
		Kind: kindOwnPrimitive, Scanner: "source", AssetType: assetAlgorithm, NISTLevel: -1,
		PQStatus: pqNA, LinkState: linkEnabled, Owner: ownerFirstParty,
	}
	for _, p := range ownPrimitives {
		if !t.Has(p.path) {
			continue
		}
		data, err := t.Read(p.path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p.path, err)
		}
		loc := p.marker.FindIndex(data)
		if loc == nil {
			continue // the table test names a stale entry
		}
		a := base
		a.Path, a.Line = p.path, lineOf(data, loc[0])
		a.Name, a.Primitive, a.ParamSet, a.Detail = p.name, p.primitive, p.param, p.detail
		out = append(out, a)
	}
	for _, rel := range t.Match(isShippedSource) {
		if ownListed(rel) {
			continue
		}
		data, err := t.Read(rel)
		if errors.Is(err, errTooLarge) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		loc := sha256RoundConstant.FindIndex(data)
		if loc == nil {
			continue
		}
		a := base
		a.Path, a.Line = rel, lineOf(data, loc[0])
		a.Name, a.Primitive, a.ParamSet = "SHA-256 round constants in a file that is not in the own-primitive table", primHash, "SHA-256"
		a.Detail = "a hand-written SHA-256, or a copy of one: use a library, or list the file in ownPrimitives with its reason"
		out = append(out, a)
	}
	return out, nil
}
