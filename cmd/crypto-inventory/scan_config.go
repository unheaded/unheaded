// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Asset kinds the config scanner emits.
const (
	kindPostgresImage = "postgres-image"
	kindPostgresConf  = "postgres-tls"
	kindProxyImage    = "proxy-image"
	kindSSHDKex       = "sshd-kex"
	kindWireGuard     = "wireguard"
	kindGoDirective   = "go-directive"
)

// OpenSSH hybrid key exchanges (ML-KEM or NTRU Prime, each with X25519).
var sshHybridKex = map[string]bool{
	"mlkem768x25519-sha256":              true,
	"sntrup761x25519-sha512":             true,
	"sntrup761x25519-sha512@openssh.com": true,
}

// OpenSSL/PostgreSQL hybrid group names (case-insensitive in OpenSSL).
var opensslHybridGroups = map[string]bool{
	"x25519mlkem768":     true,
	"secp256r1mlkem768":  true,
	"secp384r1mlkem1024": true,
}

var (
	rePostgresImage = regexp.MustCompile(`^\s*-?\s*image:\s*["']?(?:[\w.-]+/)*postgres:([^"'\s#]+)`)
	reTraefikImage  = regexp.MustCompile(`^\s*-?\s*image:\s*["']?(?:[\w.-]+/)*traefik:([^"'\s#]+)`)
	reKex           = regexp.MustCompile(`(?i)^\s*kexalgorithms(?:\s*=\s*|\s+)["']?([^"'\s#]+)`)
	reKexToken      = regexp.MustCompile(`^[A-Za-z0-9@._+^-]+$`)
	reWGPSK         = regexp.MustCompile(`^\s*(PresharedKey\s*=|presharedKeyFile\s*=)`)
	reWGPeer        = regexp.MustCompile(`^\s*\[Peer\]\s*$`)
	reGoDirective   = regexp.MustCompile(`^go\s+(\d+)\.(\d+)`)
	rePGSetting     = regexp.MustCompile(`^\s*(ssl|ssl_groups|ssl_ecdh_curve)(?:\s*=\s*|\s+)'?([^'#\s]*)'?`)
)

// classifySSHKex classifies an sshd KexAlgorithms list. A list relative to
// the compiled-in default (+, -, ^ prefixes) cannot be judged from text.
func classifySSHKex(list []string) string {
	if len(list) == 0 {
		return pqUnknown
	}
	hyb := 0
	for _, k := range list {
		if strings.HasPrefix(k, "+") || strings.HasPrefix(k, "-") || strings.HasPrefix(k, "^") {
			return pqUnknown
		}
		if sshHybridKex[k] {
			hyb++
		}
	}
	switch {
	case hyb == len(list):
		return pqHybridStrict
	case hyb > 0:
		return pqHybridOpp
	default:
		return pqClassical
	}
}

// classifyPGGroups classifies a PostgreSQL ssl_groups/ssl_ecdh_curve list
// on a server whose floor is TLS 1.3 (db/postgresql.conf).
func classifyPGGroups(groups []string) string {
	if len(groups) == 0 {
		return pqClassical // PG <= 17: prime256v1; PG 18 default X25519:prime256v1
	}
	hyb := 0
	for _, g := range groups {
		if opensslHybridGroups[strings.ToLower(g)] {
			hyb++
		}
	}
	switch {
	case hyb == len(groups):
		return pqHybridStrict
	case hyb > 0:
		return pqHybrid
	default:
		return pqClassical
	}
}

func inDirs(rel string, dirs ...string) bool {
	for _, d := range dirs {
		if strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

func isYAML(rel string) bool {
	return strings.HasSuffix(rel, ".yml") || strings.HasSuffix(rel, ".yaml")
}

func isPostgresConf(rel string) bool {
	base := path.Base(rel)
	if !strings.HasSuffix(base, ".conf") || strings.HasPrefix(base, "pg_hba") || strings.HasPrefix(base, "pg_ident") {
		return false
	}
	return strings.Contains(base, "postgresql") || strings.Contains(path.Dir(rel), "postgresql")
}

func isPatch(rel string) bool {
	return strings.HasSuffix(rel, ".patch") || strings.HasSuffix(rel, ".diff")
}

// godebugCarrier: where the plan looks for GODEBUG kill switches.
func godebugCarrier(rel string) bool {
	base := path.Base(rel)
	return isYAML(rel) || strings.HasSuffix(base, ".service") || strings.Contains(base, "Dockerfile") ||
		base == ".env.example" || strings.HasSuffix(base, ".env.example")
}

type lineFn func(rel string, n int, line string)

func eachLine(data []byte, rel string, fn lineFn) {
	for i, ln := range bytes.Split(data, []byte{'\n'}) {
		fn(rel, i+1, strings.TrimRight(string(ln), "\r"))
	}
}

// pgFacts summarises every tracked PostgreSQL conf for the image sites.
type pgFacts struct {
	tlsOn, hybrid bool
}

// scanConfig inventories compose/Kubernetes images, PostgreSQL TLS, sshd
// KEX lists, WireGuard peers, GODEBUG kill switches and go.mod directives.
func scanConfig(t *Tree) ([]Asset, error) {
	var out []Asset
	add := func(a Asset) {
		a.Scanner = "config"
		a.Owner = ownerFirstParty
		if a.LinkState == "" {
			a.LinkState = linkEnabled
		}
		if a.AssetType == "" {
			a.AssetType = assetProtocol
		}
		out = append(out, a)
	}
	read := func(rel string) ([]byte, error) {
		data, err := t.Read(rel)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		return data, nil
	}

	// PostgreSQL conf first: the image sites depend on it.
	var pg pgFacts
	for _, rel := range t.Match(isPostgresConf) {
		data, err := read(rel)
		if err != nil {
			return nil, err
		}
		var sslLine, groupLine int
		var sslOn bool
		var groups []string
		eachLine(data, rel, func(_ string, n int, line string) {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				return
			}
			m := rePGSetting.FindStringSubmatch(line)
			if m == nil {
				return
			}
			switch m[1] {
			case "ssl":
				sslOn = strings.EqualFold(m[2], "on") || strings.EqualFold(m[2], "true")
				sslLine = n
			case "ssl_groups", "ssl_ecdh_curve":
				groups = strings.Split(m[2], ":")
				groupLine = n
			}
		})
		if sslLine == 0 && groupLine == 0 {
			continue
		}
		if sslOn {
			pg.tlsOn = true
		}
		st := classifyPGGroups(groups)
		if st == pqHybrid || st == pqHybridStrict {
			pg.hybrid = true
		}
		line, detail := sslLine, "TLS on; no group list: PG <= 17 uses prime256v1, PG 18 X25519:prime256v1"
		if groupLine != 0 {
			line, detail = groupLine, "group list for PG 18 ssl_groups (or PG <= 17 ssl_ecdh_curve, one curve)"
		}
		if sslLine != 0 && !sslOn {
			detail = "TLS off"
		}
		add(Asset{Path: rel, Line: line, Kind: kindPostgresConf, Name: "PostgreSQL TLS settings",
			Protocol: protoTLS, Version: "1.3", Groups: groups, PQStatus: st, Detail: detail})
	}

	for _, rel := range t.Files {
		switch {
		case isYAML(rel):
			data, err := read(rel)
			if err != nil {
				return nil, err
			}
			eachLine(data, rel, func(rel string, n int, line string) {
				if m := rePostgresImage.FindStringSubmatch(line); m != nil {
					add(postgresImageAsset(rel, n, m[1], pg))
				}
				if m := reTraefikImage.FindStringSubmatch(line); m != nil {
					add(Asset{Path: rel, Line: n, Kind: kindProxyImage, Name: "Traefik edge (traefik:" + m[1] + ")",
						Protocol: protoTLS, PQStatus: pqUnknown, LinkState: linkDisabled,
						Detail: "edge TLS is off by decision (accepted finding, scripts/compliance/check-edge-tls.sh); when on, Traefik's Go runtime picks the groups"})
				}
			})
		}
		if inDirs(rel, "runbooks", "nix", "deploy", "scripts", "config") || strings.HasPrefix(path.Base(rel), "sshd_config") {
			data, err := read(rel)
			if err != nil {
				return nil, err
			}
			out = append(out, scanSSHDKex(rel, data)...)
			if a, ok := scanWireGuard(rel, data); ok {
				add(a)
			}
		}
		if godebugCarrier(rel) {
			data, err := read(rel)
			if err != nil {
				return nil, err
			}
			eachLine(data, rel, func(rel string, n int, line string) {
				if strings.Contains(line, "GODEBUG") && killSwitch.MatchString(line) {
					add(Asset{Path: rel, Line: n, Kind: kindGodebug, Name: "GODEBUG hybrid kill switch",
						Protocol: protoTLS, PQStatus: pqClassical, Detail: killSwitch.FindString(line)})
				}
			})
		}
		if path.Base(rel) == "go.mod" {
			data, err := read(rel)
			if err != nil {
				return nil, err
			}
			eachLine(data, rel, func(rel string, n int, line string) {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					return
				}
				if m := reGoDirective.FindStringSubmatch(line); m != nil {
					major, _ := strconv.Atoi(m[1])
					minor, _ := strconv.Atoi(m[2])
					a := Asset{Path: rel, Line: n, Kind: kindGoDirective, Name: "Go language version (go.mod)",
						Protocol: protoTLS, Version: m[1] + "." + m[2], PQStatus: pqHybrid,
						Detail: "go >= 1.24: crypto/tls defaults include X25519MLKEM768"}
					if major == 1 && minor < 24 {
						a.PQStatus = pqClassical
						a.Detail = "go < 1.24 turns the X25519MLKEM768 default off"
					}
					add(a)
				}
				if killSwitch.MatchString(line) {
					add(Asset{Path: rel, Line: n, Kind: kindGodebug, Name: "GODEBUG hybrid kill switch (go.mod)",
						Protocol: protoTLS, PQStatus: pqClassical, Detail: killSwitch.FindString(line)})
				}
			})
		}
	}
	return out, nil
}

func postgresImageAsset(rel string, n int, tag string, pg pgFacts) Asset {
	major := 0
	for _, r := range tag {
		if r < '0' || r > '9' {
			break
		}
		major = major*10 + int(r-'0')
		if major > 1000 {
			break
		}
	}
	a := Asset{Path: rel, Line: n, Kind: kindPostgresImage, Name: "PostgreSQL (postgres:" + tag + ")",
		Protocol: protoTLS, Version: "1.3"}
	switch {
	case major == 0:
		a.PQStatus, a.Detail = pqUnknown, "untagged or non-numeric version"
	case major < 18:
		a.PQStatus, a.Detail = pqClassical, "PostgreSQL <= 17 sets one ECDH curve (ssl_ecdh_curve, default prime256v1); hybrid needs PG 18 ssl_groups"
	case pg.hybrid:
		a.PQStatus, a.Detail = pqHybrid, "PG 18 with a hybrid group in ssl_groups"
	default:
		a.PQStatus, a.Detail = pqClassical, "PG 18 default ssl_groups (X25519:prime256v1) is classical"
	}
	if !pg.tlsOn {
		a.LinkState = linkDisabled
		a.Detail += "; no tracked postgresql conf turns ssl on"
	}
	return a
}

// scanSSHDKex finds KexAlgorithms settings. In a patch only added and
// context lines count; a removed line is not a setting.
func scanSSHDKex(rel string, data []byte) []Asset {
	var out []Asset
	patch := isPatch(rel)
	eachLine(data, rel, func(rel string, n int, line string) {
		if patch {
			switch {
			case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "-"):
				return
			case strings.HasPrefix(line, "+") || strings.HasPrefix(line, " "):
				line = line[1:]
			}
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return
		}
		m := reKex.FindStringSubmatch(line)
		if m == nil {
			return
		}
		var list []string
		for _, k := range strings.Split(m[1], ",") {
			if k == "" || !reKexToken.MatchString(k) {
				return // not an algorithm list (a variable, prose): skip
			}
			list = append(list, k)
		}
		out = append(out, Asset{Path: rel, Line: n, Kind: kindSSHDKex, Name: "sshd KexAlgorithms",
			Scanner: "config", AssetType: assetProtocol, Protocol: protoSSH, Groups: list,
			PQStatus: classifySSHKex(list), LinkState: linkEnabled, Owner: ownerFirstParty,
			Detail: "the client's order picks among these; only a hybrid-only list guarantees a hybrid"})
	})
	return out
}

// scanWireGuard reports one asset per file that configures WireGuard peers.
func scanWireGuard(rel string, data []byte) (Asset, bool) {
	pskLine, peerLine := 0, 0
	eachLine(data, rel, func(_ string, n int, line string) {
		if pskLine == 0 && reWGPSK.MatchString(line) {
			pskLine = n
		}
		if peerLine == 0 && reWGPeer.MatchString(line) {
			peerLine = n
		}
	})
	a := Asset{Path: rel, Kind: kindWireGuard, Name: "WireGuard peer", AssetType: assetProtocol, Protocol: protoOther}
	switch {
	case pskLine != 0:
		a.Line, a.PQStatus = pskLine, pqPSKStatic
		a.Detail = "a pre-shared key is mixed into the Noise handshake: post-quantum only while the PSK stays secret, and it is static until rotated"
	case peerLine != 0:
		a.Line, a.PQStatus = peerLine, pqClassical
		a.Detail = "Noise IK over X25519 with no pre-shared key"
	default:
		return Asset{}, false
	}
	return a, true
}
