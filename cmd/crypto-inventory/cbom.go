// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CycloneDX 1.6 output: only the parts this inventory uses (ADR-105
// decision 4). Field names and enum spellings follow
// bom-1.6.schema.json; scripts/check-crypto-inventory.sh --schema
// validates the generated file against the published schema.

const (
	bomFormat   = "CycloneDX"
	specVersion = "1.6"
)

// Property names. Every component carries source, pq-status, link-state,
// owner and cnsa2; detail, kex-groups and finding are optional.
const (
	propSource    = "unheaded:source"
	propPQStatus  = "unheaded:pq-status"
	propLinkState = "unheaded:link-state"
	propOwner     = "unheaded:owner"
	propCNSA2     = "unheaded:cnsa2"
	propFinding   = "unheaded:finding"
	propDetail    = "unheaded:detail"
	propGroups    = "unheaded:kex-groups"
	propScanner   = "unheaded:scanner"
)

// pq-status values (plan Appendix C.5).
const (
	pqHybrid       = "hybrid"               // negotiates a hybrid with any modern peer
	pqHybridStrict = "hybrid-strict"        // hybrid or no handshake
	pqHybridOpp    = "hybrid-opportunistic" // hybrid only if the peer offers it (TLS 1.2 floor, mixed lists)
	pqOnly         = "pq-only"              // a pure ML-KEM group, not hybrid
	pqClassical    = "classical"
	pqSignature    = "pq-signature"
	pqClassicalSig = "classical-signature"
	pqPSKStatic    = "pq-psk-static" // WireGuard: a static PSK mixed into Noise
	pqUnknown      = "unknown"
	pqNA           = "n/a"
)

// link-state values.
const (
	linkEnabled       = "enabled"
	linkDisabled      = "disabled-by-config"
	linkUnusedFeature = "unused-feature"
)

// owner values.
const (
	ownerFirstParty = "first-party"
	ownerThirdParty = "third-party-peer"
)

// CycloneDX enums used here.
const (
	assetProtocol  = "protocol"
	assetAlgorithm = "algorithm"

	protoTLS   = "tls"
	protoSSH   = "ssh"
	protoOther = "other"

	primSignature = "signature"
	primKEM       = "kem"
	primKeyAgree  = "key-agree"
)

// Asset is one inventory entry before it becomes a CycloneDX component.
// Scanners fill it; Rules holds the PQ-KEX lint rules it violates (Phase 4).
type Asset struct {
	Path      string // repo-relative, slash-separated
	Line      int    // 1-based; 0 for whole-file facts
	Kind      string // short machine name: tls-config, sshd-kex, ...
	Name      string // human name
	Scanner   string // go, rust, config, host
	AssetType string // assetProtocol or assetAlgorithm
	Protocol  string // protoTLS, protoSSH, protoOther (protocol assets)
	Version   string // protocol version range, e.g. "1.2-1.3"
	Primitive string // algorithm assets
	ParamSet  string // algorithm assets: ML-DSA-65, ...
	NISTLevel int    // algorithm assets: 0..5; -1 = not stated
	Groups    []string
	PQStatus  string
	LinkState string
	Owner     string
	CNSA2     bool
	Detail    string
	Rules     []string // lint rule names this site violates
}

// Ref is the component's bom-ref: path:line:kind.
func (a Asset) Ref() string {
	return fmt.Sprintf("%s:%d:%s", a.Path, a.Line, a.Kind)
}

// Source is path:line, or the path alone for a whole-file fact.
func (a Asset) Source() string {
	if a.Line <= 0 {
		return a.Path
	}
	return fmt.Sprintf("%s:%d", a.Path, a.Line)
}

// BOM is the top-level CycloneDX document.
type BOM struct {
	BOMFormat   string      `json:"bomFormat"`
	SpecVersion string      `json:"specVersion"`
	Version     int         `json:"version"`
	Metadata    Metadata    `json:"metadata"`
	Components  []Component `json:"components"`
}

// Metadata names what the BOM describes. No timestamp, no serialNumber:
// the file must be byte-identical when the tree has not changed.
type Metadata struct {
	Component MetaComponent `json:"component"`
}

// MetaComponent is the subject of the BOM.
type MetaComponent struct {
	Type        string `json:"type"`
	BOMRef      string `json:"bom-ref"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Component is one cryptographic-asset.
type Component struct {
	Type             string           `json:"type"`
	BOMRef           string           `json:"bom-ref"`
	Name             string           `json:"name"`
	CryptoProperties CryptoProperties `json:"cryptoProperties"`
	Properties       []Property       `json:"properties"`
}

// CryptoProperties is CycloneDX 1.6 cryptoProperties.
type CryptoProperties struct {
	AssetType           string               `json:"assetType"`
	AlgorithmProperties *AlgorithmProperties `json:"algorithmProperties,omitempty"`
	ProtocolProperties  *ProtocolProperties  `json:"protocolProperties,omitempty"`
}

// AlgorithmProperties is the subset used for signature/KEM imports.
type AlgorithmProperties struct {
	Primitive                string `json:"primitive"`
	ParameterSetIdentifier   string `json:"parameterSetIdentifier,omitempty"`
	NISTQuantumSecurityLevel *int   `json:"nistQuantumSecurityLevel,omitempty"`
}

// ProtocolProperties is the subset used for TLS/SSH/other links.
type ProtocolProperties struct {
	Type    string `json:"type"`
	Version string `json:"version,omitempty"`
}

// Property is a name/value pair.
type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// component converts an Asset. Property order is fixed (sorted by name).
func (a Asset) component() Component {
	c := Component{
		Type:   "cryptographic-asset",
		BOMRef: a.Ref(),
		Name:   a.Name,
		CryptoProperties: CryptoProperties{
			AssetType: a.AssetType,
		},
	}
	switch a.AssetType {
	case assetAlgorithm:
		ap := &AlgorithmProperties{Primitive: a.Primitive, ParameterSetIdentifier: a.ParamSet}
		if a.NISTLevel >= 0 {
			lvl := a.NISTLevel
			ap.NISTQuantumSecurityLevel = &lvl
		}
		c.CryptoProperties.AlgorithmProperties = ap
	default:
		c.CryptoProperties.ProtocolProperties = &ProtocolProperties{Type: a.Protocol, Version: a.Version}
	}
	cnsa := "no"
	if a.CNSA2 {
		cnsa = "yes"
	}
	props := []Property{
		{propSource, a.Source()},
		{propPQStatus, a.PQStatus},
		{propLinkState, a.LinkState},
		{propOwner, a.Owner},
		{propCNSA2, cnsa},
		{propScanner, a.Scanner},
	}
	if a.Detail != "" {
		props = append(props, Property{propDetail, a.Detail})
	}
	if len(a.Groups) > 0 {
		props = append(props, Property{propGroups, strings.Join(a.Groups, ",")})
	}
	if len(a.Rules) > 0 {
		rules := append([]string(nil), a.Rules...)
		sort.Strings(rules)
		props = append(props, Property{propFinding, "check-pq-kex: " + strings.Join(rules, ",")})
	}
	sort.SliceStable(props, func(i, j int) bool { return props[i].Name < props[j].Name })
	c.Properties = props
	return c
}

// buildBOM turns assets into a deterministic BOM: components sorted by
// bom-ref, duplicate refs disambiguated with a #n suffix in sorted order.
func buildBOM(assets []Asset) BOM {
	sorted := append([]Asset(nil), assets...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, rj := sorted[i].Ref(), sorted[j].Ref()
		if ri != rj {
			return ri < rj
		}
		return sorted[i].Name < sorted[j].Name
	})
	comps := make([]Component, 0, len(sorted))
	seen := map[string]int{}
	for _, a := range sorted {
		c := a.component()
		seen[c.BOMRef]++
		if n := seen[c.BOMRef]; n > 1 {
			c.BOMRef = fmt.Sprintf("%s#%d", c.BOMRef, n)
		}
		comps = append(comps, c)
	}
	return BOM{
		BOMFormat:   bomFormat,
		SpecVersion: specVersion,
		Version:     1,
		Metadata: Metadata{Component: MetaComponent{
			Type:        "application",
			BOMRef:      "unheaded",
			Name:        "unheaded",
			Description: "Cryptographic inventory (CBOM) generated by cmd/crypto-inventory from the repository and compliance/crypto/hosts/*.json (ADR-105). Regenerate: go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json",
		}},
		Components: comps,
	}
}

// encodeBOM renders the BOM as indented JSON with a trailing newline.
func encodeBOM(b BOM) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return nil, fmt.Errorf("encode cbom: %w", err)
	}
	return buf.Bytes(), nil
}

// componentProp returns a property value, or "" when absent.
func componentProp(c Component, name string) string {
	for _, p := range c.Properties {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}
