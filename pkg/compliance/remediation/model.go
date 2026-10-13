// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package remediation is the fix model of ADR-099: every remediation once,
// as data, from a closed set of action kinds. Fixes run as root, so the
// model is authored in the repository (compliance/remediation/fixes.yaml),
// validated strictly here, and only mirrored into The Well. Nothing in a
// fix is free-form shell: commands come from fixed allowlists.
package remediation

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/compliance/crosswalk"
)

// ErrInvalidModel wraps every rejection of a fix model, profile set or
// host binding.
var ErrInvalidModel = errors.New("invalid remediation model")

const maxModelBytes = 256 << 10

// KnownHosts are the hosts host-specific content is defined for.
var KnownHosts = []string{"west", "east"}

// Allowlists. A value outside them is rejected at load time, so no fix can
// carry a command nobody reviewed into this table.
var (
	AllowedRun      = map[string]bool{"sysctl --system": true, "systemctl reload ssh.service": true, "augenrules --load": true, "systemctl restart systemd-journald.service": true}
	AllowedValidate = map[string]bool{"visudo -c": true, "sshd -t": true, "auditd-never-halts": true}
	AllowedPrecheck = map[string]bool{"visudo -cf": true}
)

// GenRootAndGroupMembers renders root followed by a group's members,
// deduplicated (cron.allow).
const GenRootAndGroupMembers = "root-and-group-members"

var (
	idRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	unitRe    = regexp.MustCompile(`^[A-Za-z0-9@._-]+\.(service|socket|timer)$`)
	pkgRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]{0,99}$`)
	keyRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]{0,63}$`)
	accountRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	risks     = map[string]bool{"none": true, "low": true, "medium": true, "high": true}
	facts     = map[string]bool{"sudo-rs": true}
)

// Model is the parsed fix model.
type Model struct {
	Fixes []Fix `yaml:"fixes"`
}

// Fix is one remediation.
type Fix struct {
	ID       string   `yaml:"id" json:"id"`
	Title    string   `yaml:"title" json:"title"`
	Controls []string `yaml:"controls" json:"controls"`
	// Closes lists evidence-source keys (kind:host:ref) the fix makes pass;
	// "*" as the host means every known host.
	Closes  []string `yaml:"closes" json:"closes"`
	Risk    string   `yaml:"risk" json:"risk"`
	Class   string   `yaml:"class" json:"class"`
	Note    string   `yaml:"note,omitempty" json:"note,omitempty"`
	Actions []Action `yaml:"actions" json:"actions"`
}

// Action is exactly one of its fields.
type Action struct {
	File     *FileAction    `yaml:"file,omitempty" json:"file,omitempty"`
	Setting  *SettingAction `yaml:"setting,omitempty" json:"setting,omitempty"`
	Mode     *ModeAction    `yaml:"mode,omitempty" json:"mode,omitempty"`
	Service  *ServiceAction `yaml:"service,omitempty" json:"service,omitempty"`
	Package  *PackageAction `yaml:"package,omitempty" json:"package,omitempty"`
	Absent   string         `yaml:"absent,omitempty" json:"absent,omitempty"`
	Run      string         `yaml:"run,omitempty" json:"run,omitempty"`
	Validate string         `yaml:"validate,omitempty" json:"validate,omitempty"`
}

// FileAction writes a whole file. Content is Parts or a Generator.
type FileAction struct {
	Path           string `yaml:"path" json:"path"`
	Mode           string `yaml:"mode" json:"mode"`
	Owner          string `yaml:"owner,omitempty" json:"owner,omitempty"`
	Group          string `yaml:"group,omitempty" json:"group,omitempty"`
	Precheck       string `yaml:"precheck,omitempty" json:"precheck,omitempty"`
	Parts          []Part `yaml:"parts,omitempty" json:"parts,omitempty"`
	Generator      string `yaml:"generator,omitempty" json:"generator,omitempty"`
	GeneratorGroup string `yaml:"generator_group,omitempty" json:"generator_group,omitempty"`
}

// Part is a slice of a file's content, optionally limited to some hosts or
// left out when a fact holds.
type Part struct {
	Text       string   `yaml:"text" json:"text"`
	Hosts      []string `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	UnlessFact string   `yaml:"unless_fact,omitempty" json:"unless_fact,omitempty"`
}

// SettingAction sets one key=value line in a file a package owns.
type SettingAction struct {
	Path  string `yaml:"path" json:"path"`
	Key   string `yaml:"key" json:"key"`
	Value string `yaml:"value" json:"value"`
}

// ModeAction sets mode, owner and group on existing paths or a glob.
type ModeAction struct {
	Paths []string `yaml:"paths,omitempty" json:"paths,omitempty"`
	Glob  string   `yaml:"glob,omitempty" json:"glob,omitempty"`
	Mode  string   `yaml:"mode" json:"mode"`
	Owner string   `yaml:"owner" json:"owner"`
	Group string   `yaml:"group" json:"group"`
}

// ServiceAction sets a unit's state; nil fields are left alone.
type ServiceAction struct {
	Name    string `yaml:"name" json:"name"`
	Enabled *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Active  *bool  `yaml:"active,omitempty" json:"active,omitempty"`
	Masked  *bool  `yaml:"masked,omitempty" json:"masked,omitempty"`
}

// PackageAction installs a package. Removal is never automatic (ADR-098).
type PackageAction struct {
	Name string `yaml:"name" json:"name"`
}

// Facts are what a host is, as far as content depends on it.
type Facts struct {
	SudoRS bool // sudo is sudo-rs, which rejects `Defaults logfile`
}

func (f Facts) holds(name string) bool { return name == "sudo-rs" && f.SudoRS }

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidModel, fmt.Sprintf(format, a...))
}

// decodeStrict decodes one YAML document, refusing unknown fields and
// oversized input.
func decodeStrict(raw []byte, v any) error {
	if len(raw) > maxModelBytes {
		return invalid("larger than %d bytes", maxModelBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return invalid("%v", err)
	}
	return nil
}

// Parse decodes and validates a fix model on its own (CheckCatalog adds
// the cross-check against the control catalog).
func Parse(raw []byte) (*Model, error) {
	var m Model
	if err := decodeStrict(raw, &m); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i := range m.Fixes {
		f := &m.Fixes[i]
		if !idRe.MatchString(f.ID) {
			return nil, invalid("fix id %q", f.ID)
		}
		if seen[f.ID] {
			return nil, invalid("fix %s listed twice", f.ID)
		}
		seen[f.ID] = true
		if err := f.validate(); err != nil {
			return nil, fmt.Errorf("fix %s: %w", f.ID, err)
		}
	}
	return &m, nil
}

func (f *Fix) validate() error {
	if strings.TrimSpace(f.Title) == "" {
		return invalid("no title")
	}
	if !risks[f.Risk] {
		return invalid("risk %q is not none, low, medium or high", f.Risk)
	}
	if !crosswalk.IsFindingClass(f.Class) {
		return invalid("class %q is not an ADR-098 class", f.Class)
	}
	if len(f.Controls) == 0 {
		return invalid("serves no control")
	}
	if len(f.Actions) == 0 {
		return invalid("no actions")
	}
	for i, a := range f.Actions {
		if err := a.validate(); err != nil {
			return fmt.Errorf("action %d: %w", i+1, err)
		}
	}
	return nil
}

func (a Action) validate() error {
	n := 0
	for _, set := range []bool{a.File != nil, a.Setting != nil, a.Mode != nil, a.Service != nil,
		a.Package != nil, a.Absent != "", a.Run != "", a.Validate != ""} {
		if set {
			n++
		}
	}
	if n != 1 {
		return invalid("an action is exactly one of file, setting, mode, service, package, absent, run, validate (has %d)", n)
	}
	switch {
	case a.File != nil:
		return a.File.validate()
	case a.Setting != nil:
		if err := checkPath(a.Setting.Path); err != nil {
			return err
		}
		if !keyRe.MatchString(a.Setting.Key) {
			return invalid("setting key %q", a.Setting.Key)
		}
		if strings.ContainsAny(a.Setting.Value, "\n\r\x00") {
			return invalid("setting value spans lines")
		}
	case a.Mode != nil:
		return a.Mode.validate()
	case a.Service != nil:
		if !unitRe.MatchString(a.Service.Name) {
			return invalid("unit %q", a.Service.Name)
		}
		if a.Service.Enabled == nil && a.Service.Active == nil && a.Service.Masked == nil {
			return invalid("service %s sets nothing", a.Service.Name)
		}
	case a.Package != nil:
		if !pkgRe.MatchString(a.Package.Name) {
			return invalid("package %q", a.Package.Name)
		}
	case a.Absent != "":
		return checkPath(a.Absent)
	case a.Run != "":
		if !AllowedRun[a.Run] {
			return invalid("run %q is not in the allowlist", a.Run)
		}
	case a.Validate != "":
		if !AllowedValidate[a.Validate] {
			return invalid("validate %q is not in the allowlist", a.Validate)
		}
	}
	return nil
}

// checkPath admits clean absolute paths under /etc only.
func checkPath(p string) error {
	if !strings.HasPrefix(p, "/etc/") || path.Clean(p) != p || strings.Contains(p, "..") {
		return invalid("path %q is not a clean absolute path under /etc", p)
	}
	return nil
}

func parseMode(s string) (uint32, error) {
	if len(s) != 4 {
		return 0, invalid("mode %q is not four octal digits", s)
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, invalid("mode %q is not octal", s)
	}
	if v&0o7000 != 0 {
		return 0, invalid("mode %s sets setuid, setgid or sticky", s)
	}
	if v&0o002 != 0 {
		return 0, invalid("mode %s is world-writable", s)
	}
	return uint32(v), nil
}

func checkAccount(name string) error {
	if name != "" && !accountRe.MatchString(name) {
		return invalid("owner/group %q", name)
	}
	return nil
}

func (fa *FileAction) validate() error {
	if err := checkPath(fa.Path); err != nil {
		return err
	}
	if _, err := parseMode(fa.Mode); err != nil {
		return err
	}
	if err := errors.Join(checkAccount(fa.Owner), checkAccount(fa.Group)); err != nil {
		return err
	}
	if fa.Precheck != "" && !AllowedPrecheck[fa.Precheck] {
		return invalid("precheck %q is not in the allowlist", fa.Precheck)
	}
	switch {
	case len(fa.Parts) > 0 && fa.Generator != "":
		return invalid("%s has both parts and a generator", fa.Path)
	case fa.Generator != "":
		if fa.Generator != GenRootAndGroupMembers {
			return invalid("generator %q is unknown", fa.Generator)
		}
		if !accountRe.MatchString(fa.GeneratorGroup) {
			return invalid("generator group %q", fa.GeneratorGroup)
		}
	case len(fa.Parts) == 0:
		return invalid("%s has no content", fa.Path)
	}
	for _, p := range fa.Parts {
		for _, h := range p.Hosts {
			if !isKnownHost(h) {
				return invalid("part for unknown host %q", h)
			}
		}
		if p.UnlessFact != "" && !facts[p.UnlessFact] {
			return invalid("fact %q is unknown", p.UnlessFact)
		}
	}
	return nil
}

func (ma *ModeAction) validate() error {
	if (len(ma.Paths) == 0) == (ma.Glob == "") {
		return invalid("mode needs paths or a glob, not both")
	}
	for _, p := range ma.Paths {
		if err := checkPath(p); err != nil {
			return err
		}
	}
	if ma.Glob != "" {
		if err := checkPath(strings.TrimSuffix(ma.Glob, "/*")); err != nil || !strings.HasSuffix(ma.Glob, "/*") {
			return invalid("glob %q is not <dir under /etc>/*", ma.Glob)
		}
	}
	if _, err := parseMode(ma.Mode); err != nil {
		return err
	}
	if ma.Owner == "" || ma.Group == "" {
		return invalid("mode action needs owner and group")
	}
	return errors.Join(checkAccount(ma.Owner), checkAccount(ma.Group))
}

func isKnownHost(h string) bool {
	for _, k := range KnownHosts {
		if h == k {
			return true
		}
	}
	return false
}

// CheckCatalog verifies that every control a fix serves exists and every
// source it closes is a source the catalog checks, for each host "*"
// stands for.
func (m *Model) CheckCatalog(cat *crosswalk.Catalog) error {
	controls := map[string]bool{}
	sources := map[string]bool{}
	for _, c := range cat.Controls {
		controls[c.ID] = true
		for _, s := range c.Evidence {
			sources[s.Key()] = true
		}
	}
	for _, f := range m.Fixes {
		for _, c := range f.Controls {
			if !controls[c] {
				return invalid("fix %s: control %s is not in the catalog", f.ID, c)
			}
		}
		for _, key := range f.Closes {
			kind, rest, ok := strings.Cut(key, ":")
			host, ref, ok2 := strings.Cut(rest, ":")
			if !ok || !ok2 {
				return invalid("fix %s: closes %q is not kind:host:ref", f.ID, key)
			}
			hosts := []string{host}
			if host == "*" {
				hosts = KnownHosts
			} else if !isKnownHost(host) {
				return invalid("fix %s: closes %q names unknown host %q", f.ID, key, host)
			}
			for _, h := range hosts {
				if k := kind + ":" + h + ":" + ref; !sources[k] {
					return invalid("fix %s: closes %s, which no control checks", f.ID, k)
				}
			}
		}
	}
	return nil
}

// Perm returns the file mode bits.
func (fa *FileAction) Perm() (uint32, error) { return parseMode(fa.Mode) }

// Render returns the file content for host, as the executor writes it:
// parts joined in order, ending in exactly one newline. groupFile is the
// host's /etc/group, needed only by a generator.
func (fa *FileAction) Render(host string, f Facts, groupFile io.Reader) (string, error) {
	if !isKnownHost(host) {
		return "", invalid("content for unknown host %q", host)
	}
	if fa.Generator == GenRootAndGroupMembers {
		if groupFile == nil {
			return "", invalid("%s: generator needs the group file", fa.Path)
		}
		return rootAndGroupMembers(groupFile, fa.GeneratorGroup)
	}
	var b strings.Builder
	for _, p := range fa.Parts {
		if len(p.Hosts) > 0 && !contains(p.Hosts, host) {
			continue
		}
		if p.UnlessFact != "" && f.holds(p.UnlessFact) {
			continue
		}
		b.WriteString(p.Text)
		if !strings.HasSuffix(p.Text, "\n") {
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func rootAndGroupMembers(groupFile io.Reader, group string) (string, error) {
	out := []string{"root"}
	seen := map[string]bool{"root": true}
	sc := bufio.NewScanner(io.LimitReader(groupFile, 1<<20))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) != 4 || fields[0] != group {
			continue
		}
		for _, m := range strings.Split(fields[3], ",") {
			if m != "" && !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read group file: %w", err)
	}
	return strings.Join(out, "\n") + "\n", nil
}
