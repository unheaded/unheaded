// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Asset kinds the Rust scanner emits.
const (
	kindRustProvider = "rust-provider"  // rustls::crypto::<provider>::default_provider()
	kindRustBuilder  = "rust-builder"   // a rustls config built on the process-default provider
	kindRustDep      = "rust-tls-dep"   // a TLS-bearing dependency line in Cargo.toml
	kindRustLock     = "rust-lock"      // providers and rustls version resolved in a Cargo.lock
	kindRustTonicTLS = "rust-tonic-tls" // tonic ClientTlsConfig/ServerTlsConfig in use
)

var (
	reRingProvider = regexp.MustCompile(`\bcrypto::ring::default_provider\s*\(`)
	reAwsProvider  = regexp.MustCompile(`\bcrypto::aws_lc_rs::default_provider\s*\(`)
	reDefaultBuild = regexp.MustCompile(`\b(ServerConfig|ClientConfig)::builder(_with_protocol_versions)?\s*\(`)
	reTonicTLS     = regexp.MustCompile(`\b(ClientTlsConfig|ServerTlsConfig)\b`)
	reHTTPS        = regexp.MustCompile(`https://`)
)

// cargoDep is a dependency as written in Cargo.toml: a version string or a
// table with features.
type cargoDep struct {
	features        []string
	defaultFeatures bool
}

func (d cargoDep) has(f ...string) bool {
	for _, have := range d.features {
		for _, want := range f {
			if have == want {
				return true
			}
		}
	}
	return false
}

// crate is one Cargo.toml and the sources under it.
type crate struct {
	dir      string // "" for the repository root
	manifest string
	deps     map[string]cargoDep
	src      []string
}

// rustlsHasHybridDefault reports whether this rustls version's aws-lc-rs
// provider offers X25519MLKEM768 first by default (0.23.27 and later).
func rustlsHasHybridDefault(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	n := make([]int, 3)
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return false
		}
		n[i] = x
	}
	switch {
	case n[0] > 0:
		return true
	case n[1] != 23:
		return n[1] > 23
	default:
		return n[2] >= 27
	}
}

// versionLess compares dotted numeric versions; a non-numeric part sorts
// as text so the order stays total and deterministic.
func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, ex := strconv.Atoi(pa[i])
		y, ey := strconv.Atoi(pb[i])
		if ex == nil && ey == nil {
			if x != y {
				return x < y
			}
			continue
		}
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

// scanRust inventories Cargo manifests, lockfiles and Rust sources.
func scanRust(t *Tree) ([]Asset, error) {
	var out []Asset
	lockRustls := map[string]string{} // lockfile dir -> rustls version
	for _, lock := range t.Match(func(f string) bool { return path.Base(f) == "Cargo.lock" }) {
		assets, ver, err := scanCargoLock(t, lock)
		if err != nil {
			return nil, err
		}
		lockRustls[dirOf(lock)] = ver
		out = append(out, assets...)
	}
	crates, err := loadCrates(t)
	if err != nil {
		return nil, err
	}
	for _, c := range crates {
		ver := nearestLockVersion(lockRustls, c.dir)
		assets, err := scanCrate(t, c, ver)
		if err != nil {
			return nil, err
		}
		out = append(out, assets...)
	}
	return out, nil
}

func dirOf(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}

// nearestLockVersion walks up from dir to the closest lockfile's rustls.
func nearestLockVersion(locks map[string]string, dir string) string {
	for {
		if v, ok := locks[dir]; ok {
			return v
		}
		if dir == "" {
			return ""
		}
		dir = dirOf(dir)
	}
}

func scanCargoLock(t *Tree, rel string) ([]Asset, string, error) {
	data, err := t.Read(rel)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", rel, err)
	}
	var lock struct {
		Package []struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if _, err := toml.Decode(string(data), &lock); err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", rel, err)
	}
	have := map[string]string{}
	for _, p := range lock.Package {
		// Several versions of one crate can coexist; keep the highest.
		if prev, ok := have[p.Name]; !ok || versionLess(prev, p.Version) {
			have[p.Name] = p.Version
		}
	}
	var out []Asset
	rustlsVer, ok := have["rustls"]
	if ok {
		var providers []string
		for _, p := range []string{"aws-lc-rs", "ring"} {
			if _, ok := have[p]; ok {
				providers = append(providers, p)
			}
		}
		detail := "providers compiled: " + strings.Join(providers, ", ")
		if len(providers) == 0 {
			detail = "no rustls crypto provider in the lockfile"
		}
		if rustlsHasHybridDefault(rustlsVer) {
			detail += "; aws-lc-rs default_provider offers X25519MLKEM768 first at this version"
		}
		out = append(out, Asset{
			Path: rel, Kind: kindRustLock, Name: "rustls " + rustlsVer,
			Scanner: "rust", AssetType: assetProtocol, Protocol: protoTLS, Version: "1.2-1.3",
			PQStatus: pqNA, LinkState: linkEnabled, Owner: ownerFirstParty,
			Detail: detail + "; the provider call sites decide",
		})
	}
	for _, name := range []string{"pqcrypto-mlkem", "ml-kem"} {
		if v, ok := have[name]; ok {
			out = append(out, Asset{
				Path: rel, Kind: kindKEMImport, Name: "ML-KEM (" + name + " " + v + ")",
				Scanner: "rust", AssetType: assetAlgorithm, Primitive: primKEM, ParamSet: "ML-KEM",
				NISTLevel: -1, PQStatus: pqNA, LinkState: linkEnabled, Owner: ownerFirstParty,
				Detail: "KEM implementation in the lockfile; how it is combined is not visible here",
			})
		}
	}
	return out, rustlsVer, nil
}

// loadCrates parses every Cargo.toml and assigns each .rs source to its
// nearest manifest. Integration tests, benches and examples are skipped.
func loadCrates(t *Tree) ([]*crate, error) {
	byDir := map[string]*crate{}
	var dirs []string
	for _, m := range t.Match(func(f string) bool { return path.Base(f) == "Cargo.toml" }) {
		data, err := t.Read(m)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", m, err)
		}
		deps, err := parseCargoDeps(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", m, err)
		}
		c := &crate{dir: dirOf(m), manifest: m, deps: deps}
		byDir[c.dir] = c
		dirs = append(dirs, c.dir)
	}
	for _, f := range t.Match(func(f string) bool { return strings.HasSuffix(f, ".rs") }) {
		if strings.Contains("/"+f, "/tests/") || strings.Contains("/"+f, "/benches/") || strings.Contains("/"+f, "/examples/") {
			continue
		}
		for d := dirOf(f); ; d = dirOf(d) {
			if c, ok := byDir[d]; ok {
				c.src = append(c.src, f)
				break
			}
			if d == "" {
				break
			}
		}
	}
	sort.Strings(dirs)
	out := make([]*crate, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, byDir[d])
	}
	return out, nil
}

// parseCargoDeps reads [dependencies] (not dev-/build-dependencies).
func parseCargoDeps(data []byte) (map[string]cargoDep, error) {
	var m struct {
		Dependencies map[string]any `toml:"dependencies"`
	}
	if _, err := toml.Decode(string(data), &m); err != nil {
		return nil, err
	}
	deps := map[string]cargoDep{}
	for name, v := range m.Dependencies {
		d := cargoDep{defaultFeatures: true}
		if tbl, ok := v.(map[string]any); ok {
			if df, ok := tbl["default-features"].(bool); ok {
				d.defaultFeatures = df
			}
			if fs, ok := tbl["features"].([]any); ok {
				for _, f := range fs {
					if s, ok := f.(string); ok {
						d.features = append(d.features, s)
					}
				}
			}
		}
		deps[name] = d
	}
	return deps, nil
}

// manifestLine finds the 1-based line declaring dependency name.
func manifestLine(data []byte, name string) int {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*=`)
	if loc := re.FindIndex(data); loc != nil {
		return lineOf(data, loc[0])
	}
	return 0
}

// crateProviderStatus classifies the process-default provider of a crate
// from its direct rustls features.
func crateProviderStatus(c *crate, rustlsVer string) (string, string) {
	d, ok := c.deps["rustls"]
	if !ok {
		return pqUnknown, "no direct rustls dependency; provider comes from elsewhere"
	}
	aws := d.has("aws_lc_rs", "aws-lc-rs") || d.defaultFeatures
	ring := d.has("ring")
	switch {
	case ring && !aws:
		return pqClassical, "rustls built with the ring provider only; ring has no ML-KEM"
	case aws && !ring:
		if rustlsVer != "" && !rustlsHasHybridDefault(rustlsVer) {
			return pqClassical, "aws-lc-rs provider, but rustls " + rustlsVer + " predates the X25519MLKEM768 default"
		}
		return pqHybrid, "aws-lc-rs provider: X25519MLKEM768 first by default"
	case aws && ring:
		return pqNA, "aws-lc-rs and ring both compiled; the provider call sites decide, and a process-default builder panics until one is installed"
	default:
		return pqUnknown, "rustls without a provider feature"
	}
}

func scanCrate(t *Tree, c *crate, rustlsVer string) ([]Asset, error) {
	var out []Asset
	manifest, err := t.Read(c.manifest)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", c.manifest, err)
	}
	usesTonicTLS, usesHTTPS := false, false
	type hit struct {
		rel  string
		line int
		text string
	}
	var hits []hit
	for _, f := range c.src {
		data, err := t.Read(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		for i, ln := range bytes.Split(data, []byte{'\n'}) {
			s := string(ln)
			if trimmed := strings.TrimSpace(s); strings.HasPrefix(trimmed, "//") {
				continue
			}
			if reTonicTLS.MatchString(s) {
				usesTonicTLS = true
			}
			if reHTTPS.MatchString(s) {
				usesHTTPS = true
			}
			if reRingProvider.MatchString(s) || reAwsProvider.MatchString(s) || reDefaultBuild.MatchString(s) || reTonicTLS.MatchString(s) {
				hits = append(hits, hit{f, i + 1, s})
			}
		}
	}
	add := func(a Asset) {
		a.Scanner = "rust"
		a.Owner = ownerFirstParty
		if a.LinkState == "" {
			a.LinkState = linkEnabled
		}
		if a.AssetType == "" {
			a.AssetType = assetProtocol
			a.Protocol = protoTLS
		}
		out = append(out, a)
	}
	defStatus, defDetail := crateProviderStatus(c, rustlsVer)
	tonicSeen := map[string]bool{}
	for _, h := range hits {
		switch {
		case reRingProvider.MatchString(h.text):
			add(Asset{Path: h.rel, Line: h.line, Kind: kindRustProvider, Name: "rustls ring provider",
				PQStatus: pqClassical, Detail: "ring has no ML-KEM: X25519/P-256/P-384 only"})
		case reAwsProvider.MatchString(h.text):
			st, det := pqHybrid, "aws-lc-rs provider: X25519MLKEM768 first by default"
			if rustlsVer != "" && !rustlsHasHybridDefault(rustlsVer) {
				st, det = pqClassical, "aws-lc-rs provider, but rustls "+rustlsVer+" predates the hybrid default"
			}
			add(Asset{Path: h.rel, Line: h.line, Kind: kindRustProvider, Name: "rustls aws-lc-rs provider",
				PQStatus: st, Detail: det})
		case reDefaultBuild.MatchString(h.text):
			add(Asset{Path: h.rel, Line: h.line, Kind: kindRustBuilder, Name: "rustls config on the process-default provider",
				PQStatus: defStatus, Detail: defDetail})
		case reTonicTLS.MatchString(h.text):
			if tonicSeen[h.rel] {
				continue
			}
			tonicSeen[h.rel] = true
			add(Asset{Path: h.rel, Line: h.line, Kind: kindRustTonicTLS, Name: "tonic TLS",
				PQStatus: pqClassical, Detail: "tonic 0.12 tls builds on rustls with ring"})
		}
	}
	names := make([]string, 0, len(c.deps))
	for n := range c.deps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		d := c.deps[name]
		line := manifestLine(manifest, name)
		switch name {
		case "rustls":
			st, det := crateProviderStatus(c, rustlsVer)
			add(Asset{Path: c.manifest, Line: line, Kind: kindRustDep, Name: "rustls dependency",
				PQStatus: st, Detail: det + featureNote(d)})
		case "tonic":
			if !d.has("tls", "tls-ring", "tls-aws-lc", "tls-roots", "tls-native-roots", "tls-webpki-roots") {
				continue
			}
			a := Asset{Path: c.manifest, Line: line, Kind: kindRustDep, Name: "tonic TLS feature",
				PQStatus: pqClassical, Detail: "tonic 0.12 tls builds on rustls with ring" + featureNote(d)}
			if !usesTonicTLS {
				a.LinkState = linkUnusedFeature
				a.Detail += "; no ClientTlsConfig/ServerTlsConfig in the crate"
			}
			add(a)
		case "reqwest":
			if !d.has("rustls-tls", "rustls-tls-webpki-roots", "rustls-tls-native-roots", "rustls-tls-manual-roots") {
				continue
			}
			a := Asset{Path: c.manifest, Line: line, Kind: kindRustDep, Name: "reqwest rustls TLS feature",
				PQStatus: pqClassical, Detail: "reqwest 0.12 rustls-tls builds on rustls with ring" + featureNote(d)}
			if !usesHTTPS {
				a.LinkState = linkUnusedFeature
				a.Detail += "; the crate only uses http:// URLs"
			}
			add(a)
		}
	}
	return out, nil
}

func featureNote(d cargoDep) string {
	if len(d.features) == 0 && d.defaultFeatures {
		return " (default features)"
	}
	note := " (features: " + strings.Join(d.features, ", ")
	if !d.defaultFeatures {
		note += "; default-features = false"
	}
	return note + ")"
}
