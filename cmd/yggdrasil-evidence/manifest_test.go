// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const schemaPath = "../../nix/yggdrasil/evidence-pack/schema/manifest-v1.yaml"

func fixtures(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata/manifests", dir, "*.yaml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures in %s: %v", dir, err)
	}
	out := map[string][]byte{}
	for _, p := range paths {
		b, err := os.ReadFile(p) // #nosec G304 -- test fixtures
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(filepath.Base(p), ".yaml")] = b
	}
	return out
}

func TestManifest_ValidFixtures(t *testing.T) {
	for name, b := range fixtures(t, "valid") {
		m, err := ValidateManifest(b)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if m.SchemaVersion != "1.0.0" || m.Image.IsoSha256 == "" || len(m.ChainOfCustody) == 0 {
			t.Errorf("%s: typed decode incomplete: %+v", name, m)
		}
	}
}

// Every invalid fixture breaks exactly one rule; the error must name the field.
func TestManifest_InvalidFixtures(t *testing.T) {
	wantField := map[string]string{
		"missing-anchor":   "anchor",
		"schema-version-2": "schema_version",
		"iso-sha-short":    "image.iso_sha256",
		"critical-1":       "cve_scan.critical",
		"high-1-no-waiver": "cve_scan.high",
		"cis-94":           "cis_benchmark.score_percent",
		"lynis-89":         "cis_benchmark.lynis_hardening_score",
		"patch-count-51":   "overlay.patch_count",
		"loc-5001":         "overlay.total_loc_delta",
		"codename-buster":  "anchor.release_codename",
		"waiver-bad-cve":   "cve_scan.waivers[0].cve_id",
		"bad-date-time":    "image.build_date_utc",
		"doctor-exit-1":    "upc_doctor.exit_code",
		"unknown-field":    "image.signed_by",
		"unquoted-date":    "anchor.release_date",
		"yes-as-string":    "image.build_host",
	}
	got := fixtures(t, "invalid")
	for name, b := range got {
		field, ok := wantField[name]
		if !ok {
			t.Errorf("fixture %s has no expected field in this test", name)
			continue
		}
		_, err := ValidateManifest(b)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if !strings.Contains(err.Error(), field) {
			t.Errorf("%s: error does not name %q: %v", name, field, err)
		}
		if n := strings.Count(err.Error(), "\n") + 1; n != 1 {
			t.Errorf("%s: want exactly one violation, got %d: %v", name, n, err)
		}
	}
	for name := range wantField {
		if _, ok := got[name]; !ok {
			t.Errorf("expected fixture invalid/%s.yaml is missing", name)
		}
	}
}

func TestManifest_HostileInputs(t *testing.T) {
	minimal := fixtures(t, "valid")["minimal"]
	big := append(bytes.Repeat([]byte("#"), maxManifestBytes), '\n')
	cases := map[string][]byte{
		"empty":            nil,
		"only a comment":   []byte("# nothing\n"),
		"not a mapping":    []byte("- a\n- b\n"),
		"scalar root":      []byte("hello\n"),
		"two documents":    append(append([]byte{}, minimal...), []byte("---\nschema_version: \"1.0.0\"\n")...),
		"alias":            bytes.Replace(minimal, []byte("  build_host: \"builder-1\""), []byte("  build_host: &h \"builder-1\"\n  iso_path_alias: *h"), 1),
		"duplicate key":    bytes.Replace(minimal, []byte("  build_host: \"builder-1\""), []byte("  build_host: \"builder-1\"\n  build_host: \"other\""), 1),
		"custom tag":       bytes.Replace(minimal, []byte(`build_host: "builder-1"`), []byte(`build_host: !evil "builder-1"`), 1),
		"hex integer":      bytes.Replace(minimal, []byte("iso_size_bytes: 4096"), []byte("iso_size_bytes: 0x1000"), 1),
		"octal patch":      bytes.Replace(minimal, []byte("patch_count: 0"), []byte("patch_count: 061"), 1),
		"sexagesimal str":  bytes.Replace(minimal, []byte(`build_host: "builder-1"`), []byte(`build_host: 1:30`), 1),
		"float as integer": bytes.Replace(minimal, []byte("iso_size_bytes: 4096"), []byte("iso_size_bytes: 4096.0"), 1),
		"nan score":        bytes.Replace(minimal, []byte("score_percent: 96.5"), []byte("score_percent: .nan"), 1),
		"null required":    bytes.Replace(minimal, []byte(`build_host: "builder-1"`), []byte(`build_host: ~`), 1),
		"bad calendar day": bytes.Replace(minimal, []byte(`release_date: "2024-08-31"`), []byte(`release_date: "2024-02-30"`), 1),
		"trailing newline": bytes.Replace(minimal, []byte(`iso_sha256: "0000000000000000000000000000000000000000000000000000000000000000"`), []byte(`iso_sha256: "0000000000000000000000000000000000000000000000000000000000000000\n"`), 1),
		"oversized":        big,
		"not yaml":         []byte("{{{{"),
		"license not int":  bytes.Replace(minimal, []byte("MIT: 2"), []byte("MIT: lots"), 1),
	}
	for name, b := range cases {
		if _, err := ValidateManifest(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Spelling YAML 1.1 booleans is fine where the schema wants a boolean.
	ok := bytes.Replace(minimal, []byte("xdp_supported: true"), []byte("xdp_supported: yes"), 1)
	if _, err := ValidateManifest(ok); err != nil {
		t.Errorf("yes for a boolean: %v", err)
	}
}

// The Go rules are a copy of the schema, not loaded from it. This guard fails
// when the schema gains, loses or changes a rule the Go table does not mirror.
func TestSchemaDriftGuard(t *testing.T) {
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if drift := schemaDrift(schema); len(drift) != 0 {
		t.Fatalf("schema and Go rule table disagree:\n%s", strings.Join(drift, "\n"))
	}
	// The guard itself must be able to fail.
	for name, edit := range map[string][2]string{
		"maximum changed":   {"maximum: 50", "maximum: 60"},
		"new keyword":       {"    description: \"Bumped via Round Table", "    maxLength: 5\n    description: \"Bumped via Round Table"},
		"enum widened":      {"enum: [bookworm, trixie, forky]", "enum: [bookworm, trixie, forky, buster]"},
		"required dropped":  {"  - chain_of_custody\n", ""},
		"pattern changed":   {`pattern: "^CVE-[0-9]{4}-[0-9]+$"`, `pattern: "^CVE-.*$"`},
		"new property rule": {"      build_host:\n        type: string\n", "      build_host:\n        type: string\n        minLength: 1\n"},
	} {
		mut := strings.Replace(string(schema), edit[0], edit[1], 1)
		if mut == string(schema) {
			t.Fatalf("%s: edit did not apply", name)
		}
		if len(schemaDrift([]byte(mut))) == 0 {
			t.Errorf("%s: drift guard did not notice", name)
		}
	}
}

func TestRuleTableCoversEveryKeyword(t *testing.T) {
	rules := flattenRules(manifestSchema, "")
	var ptrs []string
	for p := range rules {
		ptrs = append(ptrs, p)
	}
	sort.Strings(ptrs)
	if len(ptrs) < 50 {
		t.Errorf("rule table suspiciously small (%d pointers)", len(ptrs))
	}
}

func FuzzValidateManifest(f *testing.F) {
	for _, dir := range []string{"valid", "invalid"} {
		paths, _ := filepath.Glob(filepath.Join("testdata/manifests", dir, "*.yaml"))
		for _, p := range paths {
			b, err := os.ReadFile(p) // #nosec G304 -- test fixtures
			if err == nil {
				f.Add(b)
			}
		}
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		m, err := ValidateManifest(in)
		if err == nil && m == nil {
			t.Fatal("nil manifest without error")
		}
	})
}
