// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package bpfschema

// rust_source_test.go derives the expected Go layouts from the Rust eBPF
// source at test time (S-TRUE-WIRE Steps 115-118, M-185). The Rust is what
// the verifier loads; the Go mirrors follow it. Nothing here is hardcoded
// except the MONAD_SIZE sentinel in parity_test.go, so a struct or map that
// changes in Rust and not in Go fails this test.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ebpfRoot is the Rust eBPF workspace relative to this package.
const ebpfRoot = "../../../ebpf"

// rustField is one field of a parsed #[repr(C)] struct.
type rustField struct {
	name    string
	typ     string
	feature string // non-empty when gated by #[cfg(feature = "...")]
}

// rustStruct is a parsed #[repr(C)] or #[repr(C, packed)] struct.
type rustStruct struct {
	name   string
	file   string
	packed bool
	fields []rustField
}

// rustSource holds every repr(C) struct, repr(u8) enum, usize const and
// #[map] declaration found under ebpf/*/src.
type rustSource struct {
	structs map[string][]rustStruct // name -> definitions (one per file)
	u8enums map[string]bool
	consts  map[string]int
	maps    map[string]rustMap // kernel map name -> declaration
}

type rustMap struct {
	file, kind, key, value string
}

type layoutField struct {
	name         string
	offset, size uintptr
}

type layout struct {
	size, align uintptr
	fields      []layoutField
}

var (
	reReprStruct = regexp.MustCompile(`#\[repr\((C(?:\s*,\s*packed)?)\)\]\s*(?:#\[[^\]]*\]\s*)*(?:pub(?:\([^)]*\))?\s+)?struct\s+(\w+)\s*\{`)
	reReprU8Enum = regexp.MustCompile(`#\[repr\(u8\)\]\s*(?:#\[[^\]]*\]\s*)*(?:pub(?:\([^)]*\))?\s+)?enum\s+(\w+)`)
	reUsizeConst = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?const\s+(\w+)\s*:\s*usize\s*=\s*([0-9_]+)\s*;`)
	reMapDecl    = regexp.MustCompile(`#\[map(?:\(\s*name\s*=\s*"([^"]+)"\s*\))?\]\s*(?:#\[[^\]]*\]\s*)*(?:pub(?:\([^)]*\))?\s+)?static\s+(?:mut\s+)?(\w+)\s*:\s*(\w+)\s*(<[^=]*>)?\s*=`)
	reField      = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?(\w+)\s*:\s*(.+?)\s*,?$`)
	reCfgFeature = regexp.MustCompile(`^#\[cfg\(feature\s*=\s*"([^"]+)"\)\]$`)
	reArray      = regexp.MustCompile(`^\[\s*(.+?)\s*;\s*(\w+)\s*\]$`)
)

// stripLineComment removes a // comment (Rust doc comments included).
func stripLineComment(s string) string {
	if i := strings.Index(s, "//"); i >= 0 {
		return s[:i]
	}
	return s
}

// structBody returns the text between the '{' at open and its matching '}'.
func structBody(src string, open int) (string, error) {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open+1 : i], nil
			}
		}
	}
	return "", fmt.Errorf("unbalanced braces at offset %d", open)
}

func parseFields(body string) ([]rustField, error) {
	var fields []rustField
	feature := ""
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(stripLineComment(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#[") {
			if m := reCfgFeature.FindStringSubmatch(line); m != nil {
				feature = m[1]
			} else if strings.HasPrefix(line, "#[cfg") {
				return nil, fmt.Errorf("unsupported cfg attribute %q", line)
			}
			continue
		}
		m := reField.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unparsed field line %q", line)
		}
		fields = append(fields, rustField{name: m[1], typ: strings.TrimSpace(m[2]), feature: feature})
		feature = ""
	}
	return fields, nil
}

// loadRustSource parses every .rs file under ebpf/*/src (skipping target/).
func loadRustSource(t *testing.T) *rustSource {
	t.Helper()
	if _, err := os.Stat(ebpfRoot); err != nil {
		t.Fatalf("Rust eBPF source not found at %s: %v (the parity test must not silently pass)", ebpfRoot, err)
	}
	rs := &rustSource{
		structs: map[string][]rustStruct{},
		u8enums: map[string]bool{},
		consts:  map[string]int{},
		maps:    map[string]rustMap{},
	}
	err := filepath.WalkDir(ebpfRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "target" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".rs") || !strings.Contains(filepath.ToSlash(path), "/src/") {
			return nil
		}
		b, err := os.ReadFile(path) // #nosec G304 -- the repo's own Rust source
		if err != nil {
			return err
		}
		src := string(b)
		rel, _ := filepath.Rel(ebpfRoot, path)
		rel = filepath.ToSlash(rel)
		for _, m := range reReprStruct.FindAllStringSubmatchIndex(src, -1) {
			repr, name := src[m[2]:m[3]], src[m[4]:m[5]]
			body, err := structBody(src, m[1]-1)
			if err != nil {
				return fmt.Errorf("%s struct %s: %w", rel, name, err)
			}
			fields, err := parseFields(body)
			if err != nil {
				return fmt.Errorf("%s struct %s: %w", rel, name, err)
			}
			rs.structs[name] = append(rs.structs[name], rustStruct{
				name: name, file: rel, packed: strings.Contains(repr, "packed"), fields: fields,
			})
		}
		for _, m := range reReprU8Enum.FindAllStringSubmatch(src, -1) {
			rs.u8enums[m[1]] = true
		}
		for _, m := range reUsizeConst.FindAllStringSubmatch(src, -1) {
			n, err := strconv.Atoi(strings.ReplaceAll(m[2], "_", ""))
			if err == nil {
				rs.consts[m[1]] = n
			}
		}
		for _, m := range reMapDecl.FindAllStringSubmatch(src, -1) {
			name := m[2]
			if m[1] != "" {
				name = m[1]
			}
			var key, value string
			if g := strings.TrimSpace(m[4]); g != "" {
				parts := splitGenerics(g[1 : len(g)-1])
				if len(parts) == 2 {
					key, value = parts[0], parts[1]
				}
			}
			rs.maps[name] = rustMap{file: rel, kind: m[3], key: key, value: value}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parse Rust source: %v", err)
	}
	return rs
}

// splitGenerics splits "A, [u8; 4]" at top-level commas.
func splitGenerics(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '<', '[', '(':
			depth++
		case '>', ']', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// structIn returns the definition of name declared in file (or the only one).
func (rs *rustSource) structIn(name, file string) (rustStruct, error) {
	defs := rs.structs[name]
	for _, d := range defs {
		if file == "" || d.file == file {
			return d, nil
		}
	}
	if file == "" && len(defs) == 1 {
		return defs[0], nil
	}
	return rustStruct{}, fmt.Errorf("struct %s not found in %q (found in %d files)", name, file, len(defs))
}

// typeLayout returns the size and alignment of a Rust type under C rules.
// file disambiguates struct names declared in more than one crate.
func (rs *rustSource) typeLayout(typ, file string, features map[string]bool) (size, align uintptr, err error) {
	switch typ {
	case "u8", "i8", "bool":
		return 1, 1, nil
	case "u16", "i16":
		return 2, 2, nil
	case "u32", "i32":
		return 4, 4, nil
	case "u64", "i64":
		return 8, 8, nil
	}
	if m := reArray.FindStringSubmatch(typ); m != nil {
		n, convErr := strconv.Atoi(m[2])
		if convErr != nil {
			c, ok := rs.consts[m[2]]
			if !ok {
				return 0, 0, fmt.Errorf("unknown array length %q", m[2])
			}
			n = c
		}
		es, ea, err := rs.typeLayout(m[1], file, features)
		if err != nil {
			return 0, 0, err
		}
		return es * uintptr(n), ea, nil
	}
	if rs.u8enums[typ] {
		return 1, 1, nil
	}
	if _, ok := rs.structs[typ]; ok {
		d, err := rs.structIn(typ, file)
		if err != nil {
			d, err = rs.structIn(typ, "")
		}
		if err != nil {
			return 0, 0, err
		}
		l, err := rs.layoutOf(d, features)
		if err != nil {
			return 0, 0, err
		}
		return l.size, l.align, nil
	}
	return 0, 0, fmt.Errorf("unsupported Rust type %q (extend the parser; never loosen it)", typ)
}

// layoutOf computes C layout (or packed layout) for a parsed struct.
func (rs *rustSource) layoutOf(s rustStruct, features map[string]bool) (layout, error) {
	var l layout
	l.align = 1
	off := uintptr(0)
	for _, f := range s.fields {
		if f.feature != "" && !features[f.feature] {
			continue
		}
		size, align, err := rs.typeLayout(f.typ, s.file, features)
		if err != nil {
			return layout{}, fmt.Errorf("%s.%s: %w", s.name, f.name, err)
		}
		if s.packed {
			align = 1
		}
		off = (off + align - 1) / align * align
		l.fields = append(l.fields, layoutField{name: f.name, offset: off, size: size})
		off += size
		if align > l.align {
			l.align = align
		}
	}
	l.size = (off + l.align - 1) / l.align * l.align
	return l, nil
}

// normName folds Go and Rust spellings onto one key (QoSClass, qos_class -> qosclass).
func normName(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "_", ""))
	if alias, ok := fieldAliases[s]; ok {
		return alias
	}
	return s
}

// fieldAliases maps Go field spellings to Rust ones where they differ by more than case.
var fieldAliases = map[string]string{
	"registers": "regs",
}

func isPad(name string) bool { return strings.HasPrefix(name, "_") }

func goLayout(t reflect.Type) []layoutField {
	var out []layoutField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if isPad(f.Name) {
			continue
		}
		out = append(out, layoutField{name: f.Name, offset: f.Offset, size: f.Type.Size()})
	}
	return out
}

// structParity lists every Go mirror of a Rust struct and where the Rust lives.
var structParity = []struct {
	goType     reflect.Type
	rustFile   string
	rustStruct string
}{
	{reflect.TypeOf(MonadRegister{}), "monad-common/src/lib.rs", "Monad"},
	{reflect.TypeOf(AnamnesisEvent{}), "monad-common/src/lib.rs", "AnamnesisEvent"},
	{reflect.TypeOf(MbcCpuState{}), "monad-common/src/lib.rs", "MbcCpuState"},
	{reflect.TypeOf(FlowKey{}), "common/src/lib.rs", "FlowKey"},
	{reflect.TypeOf(FlowState{}), "common/src/lib.rs", "FlowState"},
	{reflect.TypeOf(FlowMigrationTokenValue{}), "flow-tracker/src/main.rs", "MigrationTokenValue"},
	{reflect.TypeOf(FlowCancelValue{}), "flow-tracker/src/main.rs", "CancelFlowValue"},
}

// TestRustSourceStructParity compares size and every non-padding field
// offset of each Go mirror with the layout computed from the Rust source.
func TestRustSourceStructParity(t *testing.T) {
	rs := loadRustSource(t)
	for _, p := range structParity {
		t.Run(p.goType.Name(), func(t *testing.T) {
			def, err := rs.structIn(p.rustStruct, p.rustFile)
			if err != nil {
				t.Fatal(err)
			}
			want, err := rs.layoutOf(def, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := p.goType.Size(); got != want.size {
				t.Errorf("size: Go %s = %d bytes, Rust %s (%s) = %d bytes", p.goType.Name(), got, p.rustStruct, p.rustFile, want.size)
			}
			var rustFields []layoutField
			for _, f := range want.fields {
				if !isPad(f.name) {
					rustFields = append(rustFields, f)
				}
			}
			goFields := goLayout(p.goType)
			for i := 0; i < len(goFields) || i < len(rustFields); i++ {
				switch {
				case i >= len(goFields):
					r := rustFields[i]
					t.Errorf("Rust field %s @0x%02X (%dB) has no Go counterpart", r.name, r.offset, r.size)
				case i >= len(rustFields):
					g := goFields[i]
					t.Errorf("Go field %s @0x%02X (%dB) has no Rust counterpart", g.name, g.offset, g.size)
				default:
					g, r := goFields[i], rustFields[i]
					if normName(g.name) != normName(r.name) || g.offset != r.offset || g.size != r.size {
						t.Errorf("field %d: Go %s @0x%02X (%dB), Rust %s @0x%02X (%dB)", i, g.name, g.offset, g.size, r.name, r.offset, r.size)
					}
				}
			}
		})
	}
}

// TestRustSourceSizeConstants checks the Go size constants against the Rust.
func TestRustSourceSizeConstants(t *testing.T) {
	rs := loadRustSource(t)
	if got, ok := rs.consts["MONAD_SIZE"]; !ok || got != MonadRegisterSize {
		t.Errorf("Rust MONAD_SIZE = %d (found %v), Go MonadRegisterSize = %d", got, ok, MonadRegisterSize)
	}
	def, err := rs.structIn("MbcCpuState", "monad-common/src/lib.rs")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		features map[string]bool
		goConst  uintptr
		label    string
	}{
		{nil, MbcCpuStateSize, "MbcCpuStateSize (default features)"},
		{map[string]bool{"ascend-linux": true}, MbcCpuStateAscendSize, "MbcCpuStateAscendSize (--features ascend-linux)"},
	} {
		l, err := rs.layoutOf(def, c.features)
		if err != nil {
			t.Fatal(err)
		}
		if l.size != c.goConst {
			t.Errorf("%s = %d, Rust MbcCpuState = %d", c.label, c.goConst, l.size)
		}
	}
}

// goMapNames lists every map-name constant in this package with the Go key
// and value types that mirror it (nil = no Go mirror yet).
var goMapNames = []struct {
	constName, value string
	key, val         reflect.Type
}{
	{"MapHMACKeys", MapHMACKeys, reflect.TypeOf(HMACKeyKey{}), reflect.TypeOf(HMACKeyValue{})},
	{"MapSeqCounters", MapSeqCounters, reflect.TypeOf(SeqCounterKey{}), reflect.TypeOf(SeqCounterValue{})},
	{"MapMigrationTokens", MapMigrationTokens, reflect.TypeOf(FlowKey{}), reflect.TypeOf(FlowMigrationTokenValue{})},
	{"MapRetryTokens", MapRetryTokens, nil, nil},
	{"MapErrorCounters", MapErrorCounters, reflect.TypeOf(ErrorCounterKey{}), reflect.TypeOf(ErrorCounterValue{})},
	{"MapSettings", MapSettings, reflect.TypeOf(SettingsKey{}), reflect.TypeOf(SettingsValue{})},
	{"MapDoSState", MapDoSState, reflect.TypeOf(DoSStateKey{}), reflect.TypeOf(DoSStateValue{})},
	{"MapFlowTypes", MapFlowTypes, nil, reflect.TypeOf(FlowTypeEntry{})},
	{"MapGoawayState", MapGoawayState, reflect.TypeOf(GoawayStateKey{}), reflect.TypeOf(GoawayStateValue{})},
	{"MapCancelFlows", MapCancelFlows, reflect.TypeOf(FlowKey{}), reflect.TypeOf(FlowCancelValue{})},
	{"MapHopValidators", MapHopValidators, reflect.TypeOf(HopValidatorKey{}), reflect.TypeOf(HopValidatorValue{})},
	{"MapAuthority", MapAuthority, reflect.TypeOf(AuthorityKey{}), reflect.TypeOf(AuthorityValue{})},
}

// legacyMapNames are map-name constants Go still uses that no Rust program
// declares, each with its reason. Empty: every constant names a loaded map.
var legacyMapNames = map[string]string{}

// TestRustSourceMapNames asserts every Go map-name constant is the name of a
// map some Rust program declares (Aya names a #[map] by its static identifier
// unless #[map(name = "...")] overrides it), that the Go key/value mirrors
// have the Rust map's sizes, and that no map-name constant is left in
// bpfschema.go without an entry in goMapNames.
func TestRustSourceMapNames(t *testing.T) {
	rs := loadRustSource(t)
	for _, m := range goMapNames {
		rm, ok := rs.maps[m.value]
		if !ok {
			if reason, legacy := legacyMapNames[m.value]; legacy {
				t.Logf("%s = %q: legacy, %s", m.constName, m.value, reason)
				continue
			}
			t.Errorf("%s = %q names no map declared under ebpf/*/src (dead name)", m.constName, m.value)
			continue
		}
		for _, side := range []struct {
			label   string
			goType  reflect.Type
			rustTyp string
		}{{"key", m.key, rm.key}, {"value", m.val, rm.value}} {
			if side.goType == nil || side.rustTyp == "" {
				continue
			}
			size, _, err := rs.typeLayout(side.rustTyp, rm.file, nil)
			if err != nil {
				t.Errorf("%s %s: %v", m.value, side.label, err)
				continue
			}
			if got := side.goType.Size(); got != size {
				t.Errorf("%s (%s) %s: Go %s = %d bytes, Rust %s = %d bytes", m.value, rm.file, side.label, side.goType.Name(), got, side.rustTyp, size)
			}
		}
	}

	// Every Map* constant in bpfschema.go must be listed above, so a new
	// constant cannot skip the check.
	src, err := os.ReadFile("bpfschema.go")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, m := range goMapNames {
		listed[m.constName] = true
	}
	var missing []string
	for _, m := range regexp.MustCompile(`(?m)^\s*(Map\w+)\s*=\s*"`).FindAllStringSubmatch(string(src), -1) {
		if !listed[m[1]] {
			missing = append(missing, m[1])
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("map-name constants not covered by goMapNames: %v", missing)
	}

	// Vice versa: any Rust map that sets an explicit unhd_* name must have a Go constant.
	goValues := map[string]bool{}
	for _, m := range goMapNames {
		goValues[m.value] = true
	}
	for name, rm := range rs.maps {
		if strings.HasPrefix(name, "unhd_") && !goValues[name] {
			t.Errorf("Rust map %q (%s) has no Go map-name constant", name, rm.file)
		}
	}
}

// TestRustSourceParserSanity keeps the parser honest: it must find the
// structs and maps the tests above depend on.
func TestRustSourceParserSanity(t *testing.T) {
	rs := loadRustSource(t)
	if len(rs.maps) < 50 {
		t.Errorf("parsed %d #[map] declarations, expected the full set (>= 50)", len(rs.maps))
	}
	for _, p := range structParity {
		if _, err := rs.structIn(p.rustStruct, p.rustFile); err != nil {
			t.Error(err)
		}
	}
	if !rs.u8enums["ConnectionState"] {
		t.Error("repr(u8) enum ConnectionState not parsed")
	}
}
