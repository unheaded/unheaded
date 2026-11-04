// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package logsources_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/logsources"
)

// These tests are the static coverage gate (S-LOG-HARVEST Phase 2): the
// inventory must name every systemd unit file, compose service and host, or
// CI is red and names the culprit. Paths are repo-relative from this package.

const repo = "../.."

func sourcesPath() string {
	if p := os.Getenv("LOGSOURCES_FILE"); p != "" {
		return p
	}
	return filepath.Join(repo, "config", "logging", "sources.yaml")
}

func unitsDir() string {
	if p := os.Getenv("LOGSOURCES_UNITS_DIR"); p != "" {
		return p
	}
	return filepath.Join(repo, "deploy", "systemd")
}

func composePath() string {
	if p := os.Getenv("LOGSOURCES_COMPOSE"); p != "" {
		return p
	}
	return filepath.Join(repo, "docker-compose.yml")
}

func load(t *testing.T) []logsources.Source {
	t.Helper()
	src, err := logsources.Load(sourcesPath())
	if err != nil {
		t.Fatalf("sources.yaml does not validate:\n%v", err)
	}
	return src
}

func TestEveryUnitHasASource(t *testing.T) {
	src := load(t)
	entries, err := os.ReadDir(unitsDir())
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".service") {
			continue // timers, paths, targets and drop-in dirs ride their service
		}
		if !logsources.Covers(src, "journal-unit", name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("systemd unit files with no journal-unit entry in sources.yaml (add one, status gap or excluded:<reason>): %s", strings.Join(missing, ", "))
	}
}

func TestEveryComposeServiceHasASource(t *testing.T) {
	src := load(t)
	raw, err := os.ReadFile(composePath())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			ContainerName string `yaml:"container_name"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", composePath(), err)
	}
	if len(doc.Services) == 0 {
		t.Fatalf("%s: no services; refusing to pass vacuously", composePath())
	}
	var missing []string
	for name, svc := range doc.Services {
		if !logsources.CoversAny(src, "journal-container", name, "unheaded-"+name, svc.ContainerName) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("compose services with no journal-container entry in sources.yaml: %s", strings.Join(missing, ", "))
	}
}

func TestEveryHostHasASource(t *testing.T) {
	src := load(t)
	hosts := logsources.Hosts(src)
	files, err := filepath.Glob(filepath.Join(repo, "compliance", "hosts", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no compliance/hosts/*.yaml: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f) // #nosec G304 -- repo files from a glob
		if err != nil {
			t.Fatal(err)
		}
		var h struct {
			Host string `yaml:"host"`
		}
		if err := yaml.Unmarshal(raw, &h); err != nil || h.Host == "" {
			t.Fatalf("%s: no host: %v", f, err)
		}
		if !hosts[h.Host] {
			t.Fatalf("host %s (%s) has no source at all in sources.yaml", h.Host, f)
		}
	}
}

func TestNoDuplicateIDs(t *testing.T) {
	src := load(t)
	seen := map[string]bool{}
	for _, s := range src {
		if seen[s.ID] {
			t.Fatalf("duplicate id %q", s.ID)
		}
		seen[s.ID] = true
	}
	if errs := logsources.Validate(append(src, src[0])); len(errs) == 0 {
		t.Fatal("Validate accepted a duplicate id")
	}
}

func TestRetentionClassKnown(t *testing.T) {
	src := load(t)
	for _, s := range src {
		ok := false
		for _, r := range logsources.Retentions {
			ok = ok || r == s.Retention
		}
		if !ok {
			t.Fatalf("%s: retention %q unknown", s.ID, s.Retention)
		}
	}
	bad := src[0]
	bad.Retention = "forever"
	if errs := logsources.Validate([]logsources.Source{bad}); len(errs) == 0 {
		t.Fatal("Validate accepted retention \"forever\"")
	}
}

func TestExcludedNeedsReason(t *testing.T) {
	src := load(t)
	for _, s := range src {
		if strings.HasPrefix(s.Status, "excluded") && !strings.HasPrefix(s.Status, "excluded:") {
			t.Fatalf("%s: excluded without a reason", s.ID)
		}
	}
	bad := src[0]
	bad.Status = "excluded:"
	if errs := logsources.Validate([]logsources.Source{bad}); len(errs) == 0 {
		t.Fatal("Validate accepted a bare excluded:")
	}
	bad.Status = "excluded"
	if errs := logsources.Validate([]logsources.Source{bad}); len(errs) == 0 {
		t.Fatal("Validate accepted excluded with no colon")
	}
}

func TestLoadRefusesWrongSchemaAndEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.yaml")
	if err := os.WriteFile(p, []byte("schema: other/v9\nsources: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := logsources.Load(p); err == nil {
		t.Fatal("wrong schema accepted")
	}
	if err := os.WriteFile(p, []byte("schema: "+logsources.Schema+"\nsources: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := logsources.Load(p); err == nil {
		t.Fatal("empty inventory accepted")
	}
}

func senderRow(id, match, status string) logsources.Source {
	return logsources.Source{ID: id, Host: "west", Kind: logsources.SyslogKind, Match: match, Retention: "operational",
		MaxSilence: "24h", Canary: "none: device decides", Owner: "sentinel", Status: status}
}

func TestSyslogSenders(t *testing.T) {
	rows := []logsources.Source{
		senderRow("a/plain", "192.0.2.1", "gap"),
		senderRow("a/tagged", "192.0.2.2 pihole-dns", "blocked:D7"),
		senderRow("a/v6", "2001:db8::1", "shipped"),
		senderRow("a/mapped", "::ffff:192.0.2.3", "gap"),
		senderRow("a/excluded", "192.0.2.4", "excluded:a workstation, not a log source"),
		{ID: "a/file", Host: "west", Kind: "file", Match: "/var/log/x", Status: "gap"},
	}
	got, err := logsources.SyslogSenders(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Addr.String() != "192.0.2.1" || got[0].Tag != "" || got[0].ID != "a/plain" {
		t.Errorf("plain: %+v", got[0])
	}
	if got[1].Addr.String() != "192.0.2.2" || got[1].Tag != "pihole-dns" {
		t.Errorf("tagged: %+v", got[1])
	}
	if got[3].Addr.String() != "192.0.2.3" {
		t.Errorf("a mapped address must compare equal to the IPv4 one: %+v", got[3])
	}
}

func TestValidateSenderMatch(t *testing.T) {
	bad := map[string]string{
		"not an ip":          "match",
		"host.example.org":   "match",
		"0.0.0.0":            "unspecified",
		"::":                 "unspecified",
		"192.0.2.1 tag more": "not <ip> or <ip> <tag>",
	}
	for match, want := range bad {
		errs := logsources.Validate([]logsources.Source{senderRow("a/b", match, "gap")})
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), want) {
			t.Errorf("%q: errs = %v, want one naming %q", match, errs, want)
		}
	}
}

func TestValidateDuplicateSender(t *testing.T) {
	errs := logsources.Validate([]logsources.Source{senderRow("a/one", "192.0.2.1 x", "gap"), senderRow("a/two", "192.0.2.1 x", "gap")})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), `already source "a/one"`) {
		t.Errorf("errs = %v", errs)
	}
	// The same address under different tags is how one host has many sources.
	if errs := logsources.Validate([]logsources.Source{senderRow("a/one", "192.0.2.1 x", "gap"), senderRow("a/two", "192.0.2.1 y", "gap")}); len(errs) != 0 {
		t.Errorf("errs = %v", errs)
	}
}

func TestValidateRateNotNegative(t *testing.T) {
	row := senderRow("a/b", "192.0.2.1", "gap")
	row.Rate = -1
	if errs := logsources.Validate([]logsources.Source{row}); len(errs) != 1 {
		t.Errorf("errs = %v", errs)
	}
}
