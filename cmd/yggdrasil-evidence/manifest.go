// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// Manifest validation without a JSON-Schema dependency (ADR-105 decision 6,
// ADR-004). The rules of nix/yggdrasil/evidence-pack/schema/manifest-v1.yaml
// are COPIED into manifestSchema below, not loaded at runtime: a schema edit
// without a matching code edit trips TestSchemaDriftGuard (and `validate
// --schema`), so the two cannot silently disagree.
//
// Go is deliberately stricter than the schema in these places; everything the
// schema rejects, Go rejects too (proven on the fixtures by
// nix/yggdrasil/tests/test-manifest-differential.sh):
//
//   - unknown fields in any object that declares properties (the schema has no
//     additionalProperties: false; changing that is a Round Table decision);
//   - YAML anchors, aliases, explicit tags, duplicate keys, multiple documents;
//   - integers must be plain decimal (no 0x, octal, underscores or 4096.0);
//     numbers must be plain decimal (no .inf/.nan);
//   - patterns are matched with Go's $ (end of text), so a trailing newline that
//     Python's re.search would let through is refused;
//   - license_summary values must be integers (the schema's description says
//     "license-id -> count" but declares no value type).
//
// Scalar typing follows PyYAML's YAML 1.1 resolvers, which is what the schema's
// other consumer (Python jsonschema over yaml.safe_load) sees: an unquoted
// 2024-08-31 is a timestamp and a plain `yes` is a boolean, so neither is a
// string here either.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxManifestBytes = 1 << 20

// rule is the subset of JSON Schema 2020-12 that manifest-v1.yaml uses.
type rule struct {
	Type     string
	Required []string
	Props    map[string]*rule
	Items    *rule
	Const    any
	Enum     []any
	Minimum  *float64
	Maximum  *float64
	Pattern  string
	Format   string

	re *regexp.Regexp
}

func f64(v float64) *float64 { return &v }

func str() *rule               { return &rule{Type: "string"} }
func strPat(p string) *rule    { return &rule{Type: "string", Pattern: p} }
func strFmt(f string) *rule    { return &rule{Type: "string", Format: f} }
func strConst(c string) *rule  { return &rule{Type: "string", Const: c} }
func strEnum(e ...any) *rule   { return &rule{Type: "string", Enum: e} }
func boolConst() *rule         { return &rule{Type: "boolean", Const: true} }
func intMin(min float64) *rule { return &rule{Type: "integer", Minimum: f64(min)} }
func intRange(lo, hi float64) *rule {
	return &rule{Type: "integer", Minimum: f64(lo), Maximum: f64(hi)}
}

// manifestSchema mirrors schema/manifest-v1.yaml rule for rule.
var manifestSchema = &rule{
	Type: "object",
	Required: []string{"schema_version", "image", "anchor", "overlay", "sbom",
		"cve_scan", "cis_benchmark", "upc_doctor", "chain_of_custody"},
	Props: map[string]*rule{
		"schema_version": strConst("1.0.0"),
		"image": {
			Type:     "object",
			Required: []string{"iso_path", "iso_sha256", "iso_size_bytes", "build_date_utc", "build_host"},
			Props: map[string]*rule{
				"iso_path":       strPat(`^[A-Za-z0-9._/-]+\.iso$`),
				"iso_sha256":     strPat(`^[a-f0-9]{64}$`),
				"iso_size_bytes": intMin(1),
				"build_date_utc": strFmt("date-time"),
				"build_host":     str(),
			},
		},
		"anchor": {
			Type:     "object",
			Required: []string{"release_codename", "release_version", "release_date", "debian_commit"},
			Props: map[string]*rule{
				"release_codename": strEnum("bookworm", "trixie", "forky"),
				"release_version":  strPat(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`),
				"release_date":     strFmt("date"),
				"debian_commit":    strPat(`^[a-f0-9]{40}$`),
			},
		},
		"overlay": {
			Type:     "object",
			Required: []string{"patch_count", "patches", "total_loc_delta"},
			Props: map[string]*rule{
				"patch_count": intRange(0, 50),
				"patches": {
					Type: "array",
					Items: &rule{
						Type:     "object",
						Required: []string{"filename", "sha256", "subject", "reason", "upstream_status", "authored_date"},
						Props: map[string]*rule{
							"filename":        strPat(`^[0-9]{4}-[a-z0-9-]+\.patch$`),
							"sha256":          strPat(`^[a-f0-9]{64}$`),
							"subject":         str(),
							"reason":          str(),
							"upstream_status": strEnum("submitted", "accepted", "rejected", "downstream-only"),
							"authored_date":   strFmt("date"),
						},
					},
				},
				"total_loc_delta": intRange(0, 5000),
			},
		},
		"sbom": {
			Type:     "object",
			Required: []string{"spdx_path", "cyclonedx_path", "package_count", "license_summary"},
			Props: map[string]*rule{
				"spdx_path":       strConst("sbom.spdx.json"),
				"cyclonedx_path":  strConst("sbom.cyclonedx.json"),
				"package_count":   intMin(1),
				"license_summary": {Type: "object"},
			},
		},
		"cve_scan": {
			Type:     "object",
			Required: []string{"scan_date_utc", "scanner", "scanner_version", "critical", "high", "medium", "low", "waivers"},
			Props: map[string]*rule{
				"scan_date_utc":   strFmt("date-time"),
				"scanner":         strEnum("trivy", "grype", "snyk"),
				"scanner_version": str(),
				"critical":        intRange(0, 0),
				"high":            intRange(0, 0),
				"medium":          intMin(0),
				"low":             intMin(0),
				"waivers": {
					Type: "array",
					Items: &rule{
						Type:     "object",
						Required: []string{"cve_id", "reason", "approver", "expires_date"},
						Props: map[string]*rule{
							"cve_id":       strPat(`^CVE-[0-9]{4}-[0-9]+$`),
							"reason":       str(),
							"approver":     str(),
							"expires_date": strFmt("date"),
						},
					},
				},
			},
		},
		"cis_benchmark": {
			Type:     "object",
			Required: []string{"benchmark", "level", "score_percent", "lynis_hardening_score"},
			Props: map[string]*rule{
				"benchmark":             strConst("CIS Debian Linux 12 Benchmark"),
				"level":                 {Type: "integer", Enum: []any{1, 2}},
				"score_percent":         {Type: "number", Minimum: f64(95)},
				"lynis_hardening_score": intRange(90, 100),
			},
		},
		"upc_doctor": {
			Type: "object",
			Required: []string{"exit_code", "kernel_version", "bpf_subsystem", "xdp_supported",
				"upc_bootctl_present", "upc_tty_bridge_active", "monad_cpu_ebpf_present", "healthz_reachable"},
			Props: map[string]*rule{
				"exit_code":              {Type: "integer", Const: 0},
				"kernel_version":         strPat(`^[0-9]+\.[0-9]+\.[0-9]+`),
				"bpf_subsystem":          boolConst(),
				"xdp_supported":          {Type: "boolean"},
				"upc_bootctl_present":    boolConst(),
				"upc_tty_bridge_active":  boolConst(),
				"monad_cpu_ebpf_present": boolConst(),
				"healthz_reachable":      boolConst(),
			},
		},
		"chain_of_custody": {
			Type: "array",
			Items: &rule{
				Type:     "object",
				Required: []string{"timestamp_utc", "actor", "action"},
				Props: map[string]*rule{
					"timestamp_utc": strFmt("date-time"),
					"actor":         str(),
					"action":        str(),
					"signature":     str(),
				},
			},
		},
	},
}

func init() { compileRules(manifestSchema) }

func compileRules(r *rule) {
	if r == nil {
		return
	}
	if r.Pattern != "" {
		r.re = regexp.MustCompile(r.Pattern)
	}
	for _, p := range r.Props {
		compileRules(p)
	}
	compileRules(r.Items)
}

// --- typed manifest -------------------------------------------------------

// Manifest is manifest.yaml after validation.
type Manifest struct {
	SchemaVersion  string         `yaml:"schema_version"`
	Image          ManifestImage  `yaml:"image"`
	Anchor         ManifestAnchor `yaml:"anchor"`
	Overlay        Overlay        `yaml:"overlay"`
	SBOM           SBOM           `yaml:"sbom"`
	CVEScan        CVEScan        `yaml:"cve_scan"`
	CISBenchmark   CISBenchmark   `yaml:"cis_benchmark"`
	UPCDoctor      UPCDoctor      `yaml:"upc_doctor"`
	ChainOfCustody []CustodyEntry `yaml:"chain_of_custody"`
}

// ManifestImage is manifest.image.
type ManifestImage struct {
	IsoPath      string `yaml:"iso_path"`
	IsoSha256    string `yaml:"iso_sha256"`
	IsoSizeBytes int64  `yaml:"iso_size_bytes"`
	BuildDateUTC string `yaml:"build_date_utc"`
	BuildHost    string `yaml:"build_host"`
}

// ManifestAnchor is manifest.anchor.
type ManifestAnchor struct {
	ReleaseCodename string `yaml:"release_codename"`
	ReleaseVersion  string `yaml:"release_version"`
	ReleaseDate     string `yaml:"release_date"`
	DebianCommit    string `yaml:"debian_commit"`
}

// Overlay is manifest.overlay.
type Overlay struct {
	PatchCount    int     `yaml:"patch_count"`
	Patches       []Patch `yaml:"patches"`
	TotalLOCDelta int     `yaml:"total_loc_delta"`
}

// Patch is one overlay patch.
type Patch struct {
	Filename       string `yaml:"filename"`
	Sha256         string `yaml:"sha256"`
	Subject        string `yaml:"subject"`
	Reason         string `yaml:"reason"`
	UpstreamStatus string `yaml:"upstream_status"`
	AuthoredDate   string `yaml:"authored_date"`
}

// SBOM is manifest.sbom.
type SBOM struct {
	SPDXPath       string         `yaml:"spdx_path"`
	CycloneDXPath  string         `yaml:"cyclonedx_path"`
	PackageCount   int            `yaml:"package_count"`
	LicenseSummary map[string]int `yaml:"license_summary"`
}

// CVEScan is manifest.cve_scan.
type CVEScan struct {
	ScanDateUTC    string   `yaml:"scan_date_utc"`
	Scanner        string   `yaml:"scanner"`
	ScannerVersion string   `yaml:"scanner_version"`
	Critical       int      `yaml:"critical"`
	High           int      `yaml:"high"`
	Medium         int      `yaml:"medium"`
	Low            int      `yaml:"low"`
	Waivers        []Waiver `yaml:"waivers"`
}

// Waiver is one Round Table CVE waiver.
type Waiver struct {
	CVEID       string `yaml:"cve_id"`
	Reason      string `yaml:"reason"`
	Approver    string `yaml:"approver"`
	ExpiresDate string `yaml:"expires_date"`
}

// CISBenchmark is manifest.cis_benchmark.
type CISBenchmark struct {
	Benchmark           string  `yaml:"benchmark"`
	Level               int     `yaml:"level"`
	ScorePercent        float64 `yaml:"score_percent"`
	LynisHardeningScore int     `yaml:"lynis_hardening_score"`
}

// UPCDoctor is manifest.upc_doctor.
type UPCDoctor struct {
	ExitCode            int    `yaml:"exit_code"`
	KernelVersion       string `yaml:"kernel_version"`
	BPFSubsystem        bool   `yaml:"bpf_subsystem"`
	XDPSupported        bool   `yaml:"xdp_supported"`
	UPCBootctlPresent   bool   `yaml:"upc_bootctl_present"`
	UPCTTYBridgeActive  bool   `yaml:"upc_tty_bridge_active"`
	MonadCPUEBPFPresent bool   `yaml:"monad_cpu_ebpf_present"`
	HealthzReachable    bool   `yaml:"healthz_reachable"`
}

// CustodyEntry is one chain_of_custody entry.
type CustodyEntry struct {
	TimestampUTC string `yaml:"timestamp_utc"`
	Actor        string `yaml:"actor"`
	Action       string `yaml:"action"`
	Signature    string `yaml:"signature,omitempty"`
}

// --- validation -----------------------------------------------------------

// ValidationError lists every rule a manifest breaks, one per line, each
// prefixed with the field's path (e.g. "cve_scan.critical").
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string { return strings.Join(e.Problems, "\n") }

// ValidateManifest checks data against schema v1 (plus the Go-only strictness
// above) and returns the typed manifest.
func ValidateManifest(data []byte) (*Manifest, error) {
	if len(data) > maxManifestBytes {
		return nil, &ValidationError{[]string{fmt.Sprintf("(root): manifest is %d bytes, limit %d", len(data), maxManifestBytes)}}
	}
	root, err := singleDocument(data)
	if err != nil {
		return nil, &ValidationError{[]string{"(root): " + err.Error()}}
	}
	var probs []string
	manifestSchema.check(root, "", &probs)
	if len(probs) > 0 {
		return nil, &ValidationError{probs}
	}
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, &ValidationError{[]string{"(root): " + err.Error()}}
	}
	return &m, nil
}

func singleDocument(data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("empty document")
		}
		return nil, fmt.Errorf("not YAML: %v", err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("more than one YAML document")
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, errors.New("empty document")
	}
	return doc.Content[0], nil
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func shown(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}

// PyYAML 1.1 implicit resolvers (yaml/resolver.py), so a plain scalar gets the
// type Python jsonschema would see after yaml.safe_load.
var (
	pyBool  = regexp.MustCompile(`^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$`)
	pyFloat = regexp.MustCompile(`^(?:[-+]?(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+][0-9]+)?|\.[0-9][0-9_]*(?:[eE][-+][0-9]+)?|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*|[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`)
	pyInt   = regexp.MustCompile(`^(?:[-+]?0b[0-1_]+|[-+]?0[0-7_]+|[-+]?(?:0|[1-9][0-9_]*)|[-+]?0x[0-9a-fA-F_]+|[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+)$`)
	pyNull  = regexp.MustCompile(`^(?:~|null|Null|NULL|)$`)
	pyTime  = regexp.MustCompile(`^(?:[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]|[0-9][0-9][0-9][0-9]-[0-9][0-9]?-[0-9][0-9]?(?:[Tt]|[ \t]+)[0-9][0-9]?:[0-9][0-9]:[0-9][0-9](?:\.[0-9]*)?(?:[ \t]*(?:Z|[-+][0-9][0-9]?(?::[0-9][0-9])?))?)$`)
	pyOther = regexp.MustCompile(`^(?:<<|=)$`)

	decimalInt   = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)$`)
	decimalFloat = regexp.MustCompile(`^-?[0-9]+\.[0-9]+(?:[eE][-+][0-9]+)?$`)
	rfc3339      = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|[+-][0-9]{2}:[0-9]{2})$`)
	isoDate      = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// scalar is a typed YAML scalar: kind is a JSON Schema type name.
type scalar struct {
	kind string // string, integer, number, boolean, null
	val  any    // string, int64, float64, bool, nil
}

func scalarOf(n *yaml.Node) (scalar, error) {
	if n.Style&yaml.TaggedStyle != 0 {
		return scalar{}, fmt.Errorf("explicit YAML tag %s is not allowed", n.Tag)
	}
	if n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return scalar{"string", n.Value}, nil
	}
	v := n.Value
	switch {
	case pyBool.MatchString(v):
		b := strings.EqualFold(v, "yes") || strings.EqualFold(v, "true") || strings.EqualFold(v, "on")
		return scalar{"boolean", b}, nil
	case pyFloat.MatchString(v):
		if !decimalFloat.MatchString(v) {
			return scalar{}, fmt.Errorf("number %q must be plain decimal", v)
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return scalar{}, fmt.Errorf("number %q: %v", v, err)
		}
		return scalar{"number", f}, nil
	case pyInt.MatchString(v):
		if !decimalInt.MatchString(v) {
			return scalar{}, fmt.Errorf("integer %q must be plain decimal", v)
		}
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return scalar{}, fmt.Errorf("integer %q: %v", v, err)
		}
		return scalar{"integer", i}, nil
	case pyNull.MatchString(v):
		return scalar{"null", nil}, nil
	case pyTime.MatchString(v):
		return scalar{"timestamp", v}, nil
	case pyOther.MatchString(v):
		return scalar{}, fmt.Errorf("%q has a special YAML meaning; quote it", v)
	}
	return scalar{"string", v}, nil
}

func typeOK(want, got string) bool {
	return want == got || (want == "number" && got == "integer")
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func sameValue(a, b any) bool {
	fa, okA := asFloat(a)
	fb, okB := asFloat(b)
	if okA || okB {
		return okA && okB && fa == fb
	}
	return a == b
}

func (r *rule) check(n *yaml.Node, path string, probs *[]string) {
	add := func(format string, args ...any) {
		*probs = append(*probs, shown(path)+": "+fmt.Sprintf(format, args...))
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		add("YAML anchors and aliases are not allowed")
		return
	}
	switch r.Type {
	case "object":
		if n.Kind != yaml.MappingNode || n.Style&yaml.TaggedStyle != 0 {
			add("must be a mapping")
			return
		}
		r.checkObject(n, path, probs)
		return
	case "array":
		if n.Kind != yaml.SequenceNode || n.Style&yaml.TaggedStyle != 0 {
			add("must be a list")
			return
		}
		for i, item := range n.Content {
			if r.Items != nil {
				r.Items.check(item, fmt.Sprintf("%s[%d]", shown(path), i), probs)
			}
		}
		return
	}
	if n.Kind != yaml.ScalarNode {
		add("must be a %s", r.Type)
		return
	}
	s, err := scalarOf(n)
	if err != nil {
		add("%v", err)
		return
	}
	if !typeOK(r.Type, s.kind) {
		add("must be a %s, got %s %q", r.Type, s.kind, n.Value)
		return
	}
	if r.Const != nil && !sameValue(r.Const, s.val) {
		add("must be %v, got %v", r.Const, s.val)
	}
	if r.Enum != nil {
		found := false
		for _, e := range r.Enum {
			if sameValue(e, s.val) {
				found = true
				break
			}
		}
		if !found {
			add("%v is not one of %v", s.val, r.Enum)
		}
	}
	if f, ok := asFloat(s.val); ok {
		if r.Minimum != nil && f < *r.Minimum {
			add("%v is below minimum %v", s.val, *r.Minimum)
		}
		if r.Maximum != nil && f > *r.Maximum {
			add("%v is above maximum %v", s.val, *r.Maximum)
		}
	}
	if str, ok := s.val.(string); ok {
		if r.re != nil && !r.re.MatchString(str) {
			add("%q does not match %s", str, r.Pattern)
		}
		switch r.Format {
		case "date-time":
			if _, err := time.Parse(time.RFC3339Nano, str); err != nil || !rfc3339.MatchString(str) {
				add("%q is not an RFC 3339 date-time (YYYY-MM-DDTHH:MM:SSZ)", str)
			}
		case "date":
			if _, err := time.Parse("2006-01-02", str); err != nil || !isoDate.MatchString(str) {
				add("%q is not a date (YYYY-MM-DD)", str)
			}
		}
	}
}

func (r *rule) checkObject(n *yaml.Node, path string, probs *[]string) {
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		ks, err := scalarOf(k)
		if k.Kind != yaml.ScalarNode || err != nil || ks.kind != "string" {
			*probs = append(*probs, shown(path)+": keys must be plain strings")
			continue
		}
		key := k.Value
		if seen[key] {
			*probs = append(*probs, join(path, key)+": duplicate key")
			continue
		}
		seen[key] = true
		if r.Props == nil {
			// Free-form map (license_summary): any key, any value the
			// typed decode accepts. Still refuse anchors/aliases.
			if v.Kind == yaml.AliasNode || v.Anchor != "" {
				*probs = append(*probs, join(path, key)+": YAML anchors and aliases are not allowed")
			}
			continue
		}
		sub, ok := r.Props[key]
		if !ok {
			*probs = append(*probs, join(path, key)+": unknown field (not in schema v1)")
			continue
		}
		sub.check(v, join(path, key), probs)
	}
	for _, req := range r.Required {
		if !seen[req] {
			*probs = append(*probs, join(path, req)+": required field missing")
		}
	}
}

// --- schema drift ---------------------------------------------------------

// flattenRules maps every rule in r to a JSON pointer into the schema, with a
// normalized value, so the Go table and the YAML schema can be compared.
func flattenRules(r *rule, ptr string) map[string]any {
	out := map[string]any{}
	if r.Type != "" {
		out[ptr+"/type"] = r.Type
	}
	if r.Required != nil {
		out[ptr+"/required"] = normalize(r.Required)
	}
	if r.Const != nil {
		out[ptr+"/const"] = normalize(r.Const)
	}
	if r.Enum != nil {
		out[ptr+"/enum"] = normalize(r.Enum)
	}
	if r.Minimum != nil {
		out[ptr+"/minimum"] = *r.Minimum
	}
	if r.Maximum != nil {
		out[ptr+"/maximum"] = *r.Maximum
	}
	if r.Pattern != "" {
		out[ptr+"/pattern"] = r.Pattern
	}
	if r.Format != "" {
		out[ptr+"/format"] = r.Format
	}
	for name, p := range r.Props {
		out[ptr+"/properties/"+name] = "property"
		for k, v := range flattenRules(p, ptr+"/properties/"+name) {
			out[k] = v
		}
	}
	if r.Items != nil {
		for k, v := range flattenRules(r.Items, ptr+"/items") {
			out[k] = v
		}
	}
	return out
}

func normalize(v any) any {
	if f, ok := asFloat(v); ok {
		return f
	}
	switch x := v.(type) {
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	}
	return v
}

var schemaAnnotations = map[string]bool{"description": true, "$schema": true, "$id": true, "title": true}
var schemaValueKeywords = map[string]bool{"type": true, "required": true, "const": true, "enum": true,
	"minimum": true, "maximum": true, "pattern": true, "format": true}

// flattenSchema is flattenRules for the YAML schema. Keywords the Go engine
// does not implement are reported as "unsupported" so they show up as drift.
func flattenSchema(s map[string]any, ptr string, out map[string]any) {
	for k, v := range s {
		switch {
		case schemaAnnotations[k]:
		case schemaValueKeywords[k]:
			out[ptr+"/"+k] = normalize(v)
		case k == "properties":
			props, ok := v.(map[string]any)
			if !ok {
				out[ptr+"/properties"] = "unsupported: properties is not a mapping"
				continue
			}
			for name, sub := range props {
				out[ptr+"/properties/"+name] = "property"
				if m, ok := sub.(map[string]any); ok {
					flattenSchema(m, ptr+"/properties/"+name, out)
				} else {
					out[ptr+"/properties/"+name] = "unsupported: not a schema object"
				}
			}
		case k == "items":
			if m, ok := v.(map[string]any); ok {
				flattenSchema(m, ptr+"/items", out)
			} else {
				out[ptr+"/items"] = "unsupported: not a schema object"
			}
		default:
			out[ptr+"/"+k] = fmt.Sprintf("unsupported keyword %q (the Go validator does not implement it)", k)
		}
	}
}

// schemaDrift returns one line per rule the schema and the Go table disagree
// on (missing on either side, or a different value). Empty means in sync.
func schemaDrift(schemaYAML []byte) []string {
	var s map[string]any
	if err := yaml.Unmarshal(schemaYAML, &s); err != nil {
		return []string{"schema does not parse: " + err.Error()}
	}
	fromSchema := map[string]any{}
	flattenSchema(s, "", fromSchema)
	fromGo := flattenRules(manifestSchema, "")
	var drift []string
	for p, sv := range fromSchema {
		gv, ok := fromGo[p]
		switch {
		case !ok:
			drift = append(drift, fmt.Sprintf("%s: in schema (%v), but not in the Go rule table", p, sv))
		case !reflect.DeepEqual(sv, gv):
			drift = append(drift, fmt.Sprintf("%s: schema %v, Go %v", p, sv, gv))
		}
	}
	for p, gv := range fromGo {
		if _, ok := fromSchema[p]; !ok {
			drift = append(drift, fmt.Sprintf("%s: in the Go rule table (%v), not in the schema", p, gv))
		}
	}
	sort.Strings(drift)
	return drift
}
