// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package crosswalk maps Unheaded's common controls onto the requirements of
// several frameworks at once (attest once, satisfy many) and derives, from
// machine-collected evidence, which requirements that evidence supports.
//
// Honesty rules (ADR-097), enforced here rather than by the page that shows
// the result:
//   - a mapping is not satisfaction: a requirement is EVIDENCED only when
//     every control mapped to it has current passing evidence;
//   - missing evidence is NOT_ASSESSED, never a pass and never a gap;
//   - evidence older than a control's freshness window is STALE, not PASS;
//   - one failing control marks every requirement it maps to FAILING;
//   - requirements no control maps to are counted (UNMAPPED), so every
//     framework figure keeps its full denominator.
package crosswalk

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// MaxCatalogBytes bounds each catalog file the loader will parse.
const MaxCatalogBytes = 1 << 20

// ErrInvalidCatalog wraps every loader rejection.
var ErrInvalidCatalog = errors.New("invalid compliance catalog")

// Evidence source kinds a collector knows how to gather.
const (
	KindGitHubJob     = "github-job"     // ref: "<workflow name>/<job name>"
	KindGateScript    = "gate-script"    // ref: repo-relative script path
	KindGitSignatures = "git-signatures" // ref: branch whose recent commits must be signed
	KindHostSysctl    = "host-sysctl"    // ref: "[host:]<sysctl.name><op><int>", op one of = >= <=
	// KindAttestation is a signed, expiring human statement
	// (compliance/attestations/<name>.yaml). Self-attested: the page labels
	// it so, and it never stands in for machine evidence.
	KindAttestation = "attestation"
	// KindHostSSHD: ref "[host:]<key><op><value>" against `sshd -T` (the
	// effective config, Include and Match resolved); op = for strings, = >= <= for integers.
	KindHostSSHD = "host-sshd"
	// KindHostProbe: ref "[host:]<probe>", probe from the fixed table in
	// collect.Probes; the catalog can name a probe, never a command.
	KindHostProbe = "host-probe"
)

var (
	controlIDRe     = regexp.MustCompile(`^UH-[A-Z]+-[0-9]{2}$`)
	frameworkIDRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	requirementIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.()-]{0,31}$`)
)

// Requirement is one entry of a framework at the framework's declared
// granularity (e.g. SOC 2 common criteria, not points of focus).
type Requirement struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
}

// Framework is a complete requirement list: the denominator every coverage
// figure for that framework is reported against.
type Framework struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Version     string `yaml:"version" json:"version"`
	Granularity string `yaml:"granularity" json:"granularity"`
	Source      string `yaml:"source" json:"source"`
	// DerivedFrom names a framework whose requirement IDs this one reuses
	// (FedRAMP baselines are 800-53 controls). Controls map to the base
	// only; Load projects each mapping onto the derived framework.
	DerivedFrom  string        `yaml:"derived_from,omitempty" json:"derived_from,omitempty"`
	Requirements []Requirement `yaml:"requirements" json:"requirements"`

	index map[string]bool
}

// Has reports whether id is one of the framework's requirements.
func (f *Framework) Has(id string) bool { return f.index[id] }

// Source is one piece of machine-checkable evidence a control relies on.
type Source struct {
	Kind string `yaml:"kind" json:"kind"`
	Ref  string `yaml:"ref" json:"ref"`
}

// Key identifies a source across controls and evidence records.
func (s Source) Key() string { return s.Kind + ":" + s.Ref }

// EvidenceSource is a Source as a control lists it, optionally mapped on
// its own to framework requirements. A requirement mapped this way follows
// that source's evidence alone, not the whole control's: a benchmark such
// as CIS has one recommendation per check, and one failing service must not
// fail its siblings. A control maps a given framework either at control
// level or per source, never both.
type EvidenceSource struct {
	Source `yaml:",inline"`
	Maps   map[string][]string `yaml:"maps,omitempty" json:"maps,omitempty"`
}

// Control is a common control: one engineering practice, evidenced by
// machine-collected sources, mapped to requirements in many frameworks.
type Control struct {
	ID        string `yaml:"id" json:"id"`
	Title     string `yaml:"title" json:"title"`
	Statement string `yaml:"statement" json:"statement"`
	// Rationale says why the evidence supports each mapping, so a reviewer
	// can challenge the mapping instead of taking it on trust.
	Rationale     string              `yaml:"rationale,omitempty" json:"rationale,omitempty"`
	FreshnessDays int                 `yaml:"freshness_days" json:"freshness_days"`
	Evidence      []EvidenceSource    `yaml:"evidence" json:"evidence"`
	Mappings      map[string][]string `yaml:"mappings" json:"mappings"`
}

// Catalog is a validated set of frameworks and the controls mapped onto them.
type Catalog struct {
	Frameworks []*Framework
	Controls   []*Control

	byID map[string]*Framework
}

// Framework returns the framework with id, or nil.
func (c *Catalog) Framework(id string) *Framework { return c.byID[id] }

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidCatalog}, args...)...)
}

func decodeStrict(b []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	return dec.Decode(v)
}

// Load parses and validates a frameworks file and a controls file.
func Load(frameworksYAML, controlsYAML []byte) (*Catalog, error) {
	return LoadDocs([][]byte{frameworksYAML}, controlsYAML)
}

// LoadFS loads every *.yaml under frameworksDir (one or more frameworks per
// file) and the controls file, from fsys.
func LoadFS(fsys fs.FS, frameworksDir, controlsPath string) (*Catalog, error) {
	names, err := fs.Glob(fsys, path.Join(frameworksDir, "*.yaml"))
	if err != nil || len(names) == 0 {
		return nil, invalid("no framework files in %s (%v)", frameworksDir, err)
	}
	sort.Strings(names)
	docs := make([][]byte, 0, len(names))
	for _, n := range names {
		b, err := readBounded(fsys, n)
		if err != nil {
			return nil, err
		}
		docs = append(docs, b)
	}
	ctl, err := readBounded(fsys, controlsPath)
	if err != nil {
		return nil, err
	}
	return LoadDocs(docs, ctl)
}

func readBounded(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, invalid("open %s: %v", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxCatalogBytes+1))
	if err != nil {
		return nil, invalid("read %s: %v", name, err)
	}
	if len(b) > MaxCatalogBytes {
		return nil, invalid("%s larger than %d bytes", name, MaxCatalogBytes)
	}
	return b, nil
}

// LoadDocs parses and validates several frameworks files and a controls file.
func LoadDocs(frameworksYAML [][]byte, controlsYAML []byte) (*Catalog, error) {
	if len(controlsYAML) > MaxCatalogBytes {
		return nil, invalid("catalog file larger than %d bytes", MaxCatalogBytes)
	}
	var fwDoc struct {
		Frameworks []*Framework `yaml:"frameworks"`
	}
	for _, b := range frameworksYAML {
		if len(b) > MaxCatalogBytes {
			return nil, invalid("catalog file larger than %d bytes", MaxCatalogBytes)
		}
		var d struct {
			Frameworks []*Framework `yaml:"frameworks"`
		}
		if err := decodeStrict(b, &d); err != nil {
			return nil, invalid("frameworks: %v", err)
		}
		fwDoc.Frameworks = append(fwDoc.Frameworks, d.Frameworks...)
	}
	var ctlDoc struct {
		Controls []*Control `yaml:"controls"`
	}
	if err := decodeStrict(controlsYAML, &ctlDoc); err != nil {
		return nil, invalid("controls: %v", err)
	}

	c := &Catalog{byID: make(map[string]*Framework)}
	for _, fw := range fwDoc.Frameworks {
		if fw == nil || !frameworkIDRe.MatchString(fw.ID) {
			return nil, invalid("framework id %q", frameworkID(fw))
		}
		if c.byID[fw.ID] != nil {
			return nil, invalid("duplicate framework %q", fw.ID)
		}
		if len(fw.Requirements) == 0 {
			return nil, invalid("framework %q has no requirements", fw.ID)
		}
		fw.index = make(map[string]bool, len(fw.Requirements))
		for _, r := range fw.Requirements {
			if !requirementIDRe.MatchString(r.ID) {
				return nil, invalid("framework %q: requirement id %q", fw.ID, r.ID)
			}
			if fw.index[r.ID] {
				return nil, invalid("framework %q: duplicate requirement %q", fw.ID, r.ID)
			}
			fw.index[r.ID] = true
		}
		c.byID[fw.ID] = fw
		c.Frameworks = append(c.Frameworks, fw)
	}

	for _, fw := range c.Frameworks {
		if fw.DerivedFrom == "" {
			continue
		}
		base := c.byID[fw.DerivedFrom]
		switch {
		case base == nil:
			return nil, invalid("framework %q: unknown base %q", fw.ID, fw.DerivedFrom)
		case base.DerivedFrom != "":
			return nil, invalid("framework %q: base %q is itself derived", fw.ID, base.ID)
		}
		for _, r := range fw.Requirements {
			if !base.Has(r.ID) {
				return nil, invalid("framework %q: requirement %q is not in its base %s", fw.ID, r.ID, base.ID)
			}
		}
	}

	seen := make(map[string]bool)
	for _, ctl := range ctlDoc.Controls {
		if ctl == nil || !controlIDRe.MatchString(ctl.ID) {
			return nil, invalid("control id %q (want UH-<AREA>-<NN>)", controlID(ctl))
		}
		if seen[ctl.ID] {
			return nil, invalid("duplicate control %q", ctl.ID)
		}
		seen[ctl.ID] = true
		if ctl.Title == "" || ctl.Statement == "" {
			return nil, invalid("control %q needs a title and a statement", ctl.ID)
		}
		if ctl.FreshnessDays < 1 || ctl.FreshnessDays > 365 {
			return nil, invalid("control %q: freshness_days %d outside 1..365", ctl.ID, ctl.FreshnessDays)
		}
		if len(ctl.Evidence) == 0 {
			return nil, invalid("control %q has no evidence sources", ctl.ID)
		}
		for _, s := range ctl.Evidence {
			switch s.Kind {
			case KindGitHubJob, KindGateScript, KindGitSignatures, KindHostSysctl, KindAttestation, KindHostSSHD, KindHostProbe:
			default:
				return nil, invalid("control %q: evidence kind %q", ctl.ID, s.Kind)
			}
			if s.Ref == "" {
				return nil, invalid("control %q: %s evidence with empty ref", ctl.ID, s.Kind)
			}
		}
		n := 0
		for _, s := range ctl.Evidence {
			for fwID, reqs := range s.Maps {
				fw := c.byID[fwID]
				switch {
				case fw == nil:
					return nil, invalid("control %q: %s maps to unknown framework %q", ctl.ID, s.Key(), fwID)
				case fw.DerivedFrom != "":
					return nil, invalid("control %q: %s maps to %q, which is derived from %s: map to the base", ctl.ID, s.Key(), fwID, fw.DerivedFrom)
				case ctl.Mappings[fwID] != nil:
					return nil, invalid("control %q maps %s both at control level and per source", ctl.ID, fwID)
				}
				for _, r := range reqs {
					if !fw.Has(r) {
						return nil, invalid("control %q: %s: %q is not a requirement of %s", ctl.ID, s.Key(), r, fwID)
					}
					n++
				}
			}
		}
		for fwID, reqs := range ctl.Mappings {
			fw := c.byID[fwID]
			if fw == nil {
				return nil, invalid("control %q maps to unknown framework %q", ctl.ID, fwID)
			}
			if fw.DerivedFrom != "" {
				return nil, invalid("control %q maps to %q, which is derived from %s: map to the base", ctl.ID, fwID, fw.DerivedFrom)
			}
			for _, r := range reqs {
				if !fw.Has(r) {
					return nil, invalid("control %q: %q is not a requirement of %s", ctl.ID, r, fwID)
				}
				n++
			}
		}
		if n == 0 {
			return nil, invalid("control %q maps to no requirement", ctl.ID)
		}
		c.project(ctl)
		c.Controls = append(c.Controls, ctl)
	}
	return c, nil
}

// project copies ctl's mappings onto every framework derived from a base it
// maps to, keeping only the requirements the derived framework contains.
func (c *Catalog) project(ctl *Control) {
	if ctl.Mappings == nil {
		ctl.Mappings = make(map[string][]string)
	}
	for _, fw := range c.Frameworks {
		if fw.DerivedFrom == "" {
			continue
		}
		for i := range ctl.Evidence {
			var reqs []string
			for _, r := range ctl.Evidence[i].Maps[fw.DerivedFrom] {
				if fw.Has(r) {
					reqs = append(reqs, r)
				}
			}
			if len(reqs) > 0 {
				ctl.Evidence[i].Maps[fw.ID] = reqs
			}
		}
		var reqs []string
		for _, r := range ctl.Mappings[fw.DerivedFrom] {
			if fw.Has(r) {
				reqs = append(reqs, r)
			}
		}
		if len(reqs) > 0 {
			ctl.Mappings[fw.ID] = reqs
		}
	}
}

func frameworkID(f *Framework) string {
	if f == nil {
		return ""
	}
	return f.ID
}

func controlID(c *Control) string {
	if c == nil {
		return ""
	}
	return c.ID
}
