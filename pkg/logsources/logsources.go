// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package logsources reads config/logging/sources.yaml, the inventory of
// every log source the Kingdom expects to ship (S-LOG-HARVEST Phase 1), and
// validates it. Its tests are the static coverage gate (Phase 2): a systemd
// unit file, a compose service or a host without an entry fails CI.
package logsources

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Schema is the only schema this package reads.
const Schema = "unheaded.logging.sources/v1"

// Kinds a source may have.
var Kinds = []string{"journal-unit", "journal-container", "file", "lxd-file", "lxd-journal", "syslog-device"}

// Retentions are the classes docs/policy/retention-policy-v1-2026-05-06.md names
// for logs: audit (90 d online + 1 y archive) and operational (90 d).
var Retentions = []string{"audit", "operational"}

// Source is one inventory entry.
type Source struct {
	ID         string `yaml:"id"`
	Host       string `yaml:"host"`
	Kind       string `yaml:"kind"`
	Match      string `yaml:"match"`
	Retention  string `yaml:"retention"`
	MaxSilence string `yaml:"max_silence"`
	Canary     string `yaml:"canary"`
	Owner      string `yaml:"owner"`
	Status     string `yaml:"status"`
	Note       string `yaml:"note"`
}

// File is the document.
type File struct {
	Schema  string   `yaml:"schema"`
	Sources []Source `yaml:"sources"`
}

var (
	idRe         = regexp.MustCompile(`^[a-z0-9][A-Za-z0-9._/:@-]*$`)
	hostRe       = regexp.MustCompile(`^(west|east|pihole|device:[0-9A-Za-z.:-]+)$`)
	maxSilenceRe = regexp.MustCompile(`^(n/a|[0-9]+(m|h|d))$`)
	statusRe     = regexp.MustCompile(`^(shipped|gap|blocked:(D[0-9]+|FND-[0-9]+)|excluded:.+)$`)
	canaryRe     = regexp.MustCompile(`^(logger|http-get|dns-query|none: .+)$`)
)

// Load reads and validates a sources file. The error joins every finding,
// so a CI run names every problem at once.
func Load(path string) ([]Source, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- repo-relative inventory path chosen by the caller
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Schema != Schema {
		return nil, fmt.Errorf("%s: schema %q, want %q", path, f.Schema, Schema)
	}
	if len(f.Sources) == 0 {
		return nil, fmt.Errorf("%s: no sources; refusing to pass vacuously", path)
	}
	if errs := Validate(f.Sources); len(errs) > 0 {
		return f.Sources, errors.Join(errs...)
	}
	return f.Sources, nil
}

// Validate checks every entry on its own and the ids across entries.
func Validate(sources []Source) []error {
	var errs []error
	seen := map[string]int{}
	for i, s := range sources {
		where := fmt.Sprintf("source %d (%s)", i+1, s.ID)
		if !idRe.MatchString(s.ID) {
			errs = append(errs, fmt.Errorf("%s: id %q is not of the form host/name", where, s.ID))
		}
		seen[s.ID]++
		if !hostRe.MatchString(s.Host) {
			errs = append(errs, fmt.Errorf("%s: host %q is not west, east, pihole or device:<name>", where, s.Host))
		}
		if !contains(Kinds, s.Kind) {
			errs = append(errs, fmt.Errorf("%s: kind %q is not one of %s", where, s.Kind, strings.Join(Kinds, ", ")))
		}
		if strings.TrimSpace(s.Match) == "" {
			errs = append(errs, fmt.Errorf("%s: match is empty", where))
		}
		if !contains(Retentions, s.Retention) {
			errs = append(errs, fmt.Errorf("%s: retention %q is not a class the retention policy names (%s)", where, s.Retention, strings.Join(Retentions, ", ")))
		}
		if !maxSilenceRe.MatchString(s.MaxSilence) {
			errs = append(errs, fmt.Errorf("%s: max_silence %q is not <n>m|h|d or n/a", where, s.MaxSilence))
		}
		if !canaryRe.MatchString(s.Canary) {
			errs = append(errs, fmt.Errorf("%s: canary %q is not logger, http-get, dns-query or \"none: <reason>\"", where, s.Canary))
		}
		if strings.TrimSpace(s.Owner) == "" {
			errs = append(errs, fmt.Errorf("%s: owner is empty", where))
		}
		if !statusRe.MatchString(s.Status) {
			errs = append(errs, fmt.Errorf("%s: status %q is not shipped, gap, blocked:<D#|FND-#> or excluded:<reason>", where, s.Status))
		}
		if strings.HasPrefix(s.Status, "excluded:") && len(strings.TrimSpace(strings.TrimPrefix(s.Status, "excluded:"))) < 8 {
			errs = append(errs, fmt.Errorf("%s: an excluded source needs a reason, not %q", where, s.Status))
		}
	}
	var dups []string
	for id, n := range seen {
		if n > 1 {
			dups = append(dups, id)
		}
	}
	sort.Strings(dups)
	for _, id := range dups {
		errs = append(errs, fmt.Errorf("duplicate id %q", id))
	}
	return errs
}

// Covers reports whether some source of the given kind matches name.
func Covers(sources []Source, kind, name string) bool {
	for _, s := range sources {
		if s.Kind == kind && s.Match == name {
			return true
		}
	}
	return false
}

// CoversAny reports whether some source of the kind matches any of names.
func CoversAny(sources []Source, kind string, names ...string) bool {
	for _, n := range names {
		if Covers(sources, kind, n) {
			return true
		}
	}
	return false
}

// Hosts lists the distinct hosts named by the sources.
func Hosts(sources []Source) map[string]bool {
	m := map[string]bool{}
	for _, s := range sources {
		m[s.Host] = true
	}
	return m
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
