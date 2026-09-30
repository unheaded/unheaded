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
	"regexp"

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
	ID           string        `yaml:"id" json:"id"`
	Name         string        `yaml:"name" json:"name"`
	Version      string        `yaml:"version" json:"version"`
	Granularity  string        `yaml:"granularity" json:"granularity"`
	Source       string        `yaml:"source" json:"source"`
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

// Control is a common control: one engineering practice, evidenced by
// machine-collected sources, mapped to requirements in many frameworks.
type Control struct {
	ID            string              `yaml:"id" json:"id"`
	Title         string              `yaml:"title" json:"title"`
	Statement     string              `yaml:"statement" json:"statement"`
	FreshnessDays int                 `yaml:"freshness_days" json:"freshness_days"`
	Evidence      []Source            `yaml:"evidence" json:"evidence"`
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
	if len(frameworksYAML) > MaxCatalogBytes || len(controlsYAML) > MaxCatalogBytes {
		return nil, invalid("catalog file larger than %d bytes", MaxCatalogBytes)
	}
	var fwDoc struct {
		Frameworks []*Framework `yaml:"frameworks"`
	}
	if err := decodeStrict(frameworksYAML, &fwDoc); err != nil {
		return nil, invalid("frameworks: %v", err)
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
			case KindGitHubJob, KindGateScript, KindGitSignatures:
			default:
				return nil, invalid("control %q: evidence kind %q", ctl.ID, s.Kind)
			}
			if s.Ref == "" {
				return nil, invalid("control %q: %s evidence with empty ref", ctl.ID, s.Kind)
			}
		}
		n := 0
		for fwID, reqs := range ctl.Mappings {
			fw := c.byID[fwID]
			if fw == nil {
				return nil, invalid("control %q maps to unknown framework %q", ctl.ID, fwID)
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
		c.Controls = append(c.Controls, ctl)
	}
	return c, nil
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
