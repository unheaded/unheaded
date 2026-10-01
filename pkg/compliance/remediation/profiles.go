// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package remediation

import "regexp"

// Profiles and host bindings (ADR-099). The Well holds them; the YAML in
// compliance/profiles and compliance/hosts is their seed and the read-only
// fallback, parsed and validated here so both paths agree on what is valid.

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Profile turns a set of fixes on. All turns every fix on.
type Profile struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	All         bool     `yaml:"all,omitempty" json:"all,omitempty"`
	Fixes       []string `yaml:"fixes,omitempty" json:"fixes,omitempty"`
}

// ProfileSet is the parsed profiles file, resolved against a model.
type ProfileSet struct {
	Profiles []Profile `yaml:"profiles" json:"profiles"`
	model    *Model
}

// ParseProfiles decodes and validates profiles against the fix model.
func ParseProfiles(raw []byte, m *Model) (*ProfileSet, error) {
	var ps ProfileSet
	if err := decodeStrict(raw, &ps); err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, f := range m.Fixes {
		known[f.ID] = true
	}
	seen := map[string]bool{}
	for _, p := range ps.Profiles {
		if !idRe.MatchString(p.Name) {
			return nil, invalid("profile name %q", p.Name)
		}
		if seen[p.Name] {
			return nil, invalid("profile %s listed twice", p.Name)
		}
		seen[p.Name] = true
		if p.All && len(p.Fixes) > 0 {
			return nil, invalid("profile %s has all and a fix list", p.Name)
		}
		dup := map[string]bool{}
		for _, id := range p.Fixes {
			if !known[id] {
				return nil, invalid("profile %s turns on unknown fix %s", p.Name, id)
			}
			if dup[id] {
				return nil, invalid("profile %s lists %s twice", p.Name, id)
			}
			dup[id] = true
		}
	}
	ps.model = m
	return &ps, nil
}

// Has reports whether a profile of that name exists.
func (ps *ProfileSet) Has(name string) bool {
	for _, p := range ps.Profiles {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Resolve returns the fix IDs a profile turns on, in model order; nil for
// an unknown profile.
func (ps *ProfileSet) Resolve(name string) []string {
	for _, p := range ps.Profiles {
		if p.Name != name {
			continue
		}
		on := map[string]bool{}
		for _, id := range p.Fixes {
			on[id] = true
		}
		out := []string{}
		for _, f := range ps.model.Fixes {
			if p.All || on[f.ID] {
				out = append(out, f.ID)
			}
		}
		return out
	}
	return nil
}

// Applied records one fix applied to a host.
type Applied struct {
	Fix        string `yaml:"fix" json:"fix"`
	On         string `yaml:"on" json:"on"`
	ApprovedBy string `yaml:"approved_by" json:"approved_by"`
}

// Binding is one host's profile and ADR-098 mode.
type Binding struct {
	Host    string    `yaml:"host" json:"host"`
	Profile string    `yaml:"profile" json:"profile"`
	Mode    string    `yaml:"mode" json:"mode"`
	Applied []Applied `yaml:"applied,omitempty" json:"applied,omitempty"`
}

// ParseBinding decodes and validates a host binding against the profiles.
// Enforcing is refused here: ADR-098 gates it on conditions a file cannot
// show (LICH-012 run, 7-day soak, Stevie's sign-off).
func ParseBinding(raw []byte, ps *ProfileSet) (*Binding, error) {
	var b Binding
	if err := decodeStrict(raw, &b); err != nil {
		return nil, err
	}
	if !isKnownHost(b.Host) {
		return nil, invalid("host %q is unknown", b.Host)
	}
	if !ps.Has(b.Profile) {
		return nil, invalid("host %s: profile %q is unknown", b.Host, b.Profile)
	}
	if b.Mode != "audit" {
		return nil, invalid("host %s: mode %q; only audit is allowed until ADR-098's enforcing gate is met", b.Host, b.Mode)
	}
	all := ps.Resolve("all")
	for _, a := range b.Applied {
		if !contains(all, a.Fix) {
			return nil, invalid("host %s: applied fix %s is unknown", b.Host, a.Fix)
		}
		if !dateRe.MatchString(a.On) || a.ApprovedBy == "" {
			return nil, invalid("host %s: applied %s needs a YYYY-MM-DD date and an approver", b.Host, a.Fix)
		}
	}
	return &b, nil
}
