// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrOutOfScope is returned for any target that is not on the server-side
// allowlist. It is the single refusal the scope layer emits; callers wrap it
// (errors.Is) rather than inspecting the message.
var ErrOutOfScope = errors.New("lichtools: target out of scope")

// TargetKind says which allowlist a target is checked against. It is set by
// the Provider from the tool that was called — NEVER from a model-supplied
// argument. A driver chooses the tool; the server chooses the kind.
type TargetKind int

const (
	KindUnknown TargetKind = iota
	// KindSandbox is an LXD container the server created; its name must start
	// with the configured sandbox prefix.
	KindSandbox
	// KindHarness is a local fuzz harness under the configured harness dir,
	// addressed by its three-digit id (matching tomb/lich/lich_<id>_*).
	KindHarness
)

func (k TargetKind) String() string {
	switch k {
	case KindSandbox:
		return "sandbox"
	case KindHarness:
		return "harness"
	default:
		return "unknown"
	}
}

// Target is what a tool wants to act on. Name is the only field a driver
// influences; Kind is server-chosen.
type Target struct {
	Kind TargetKind
	Name string
}

// Scope is the server-side allowlist. Its fields are set once from Config and
// are not exported — nothing a model controls can mutate them. Validate is the
// only gate a target passes through.
type Scope struct {
	sandboxPrefix string
	harnessDir    string // absolute
}

// NewScope builds a Scope from a sandbox prefix and an absolute harness dir.
func NewScope(sandboxPrefix, harnessDirAbs string) (*Scope, error) {
	if sandboxPrefix == "" {
		return nil, errors.New("lichtools: NewScope needs a sandbox prefix")
	}
	if !lxdNameRe.MatchString(strings.TrimRight(sandboxPrefix, "-")) {
		return nil, fmt.Errorf("lichtools: sandbox prefix %q is not a valid LXD name stem", sandboxPrefix)
	}
	if harnessDirAbs == "" || !filepath.IsAbs(harnessDirAbs) {
		return nil, fmt.Errorf("lichtools: harness dir must be an absolute path, got %q", harnessDirAbs)
	}
	return &Scope{sandboxPrefix: sandboxPrefix, harnessDir: filepath.Clean(harnessDirAbs)}, nil
}

// ScopeFromConfig builds a Scope from a loaded Config.
func ScopeFromConfig(c *Config) (*Scope, error) {
	return NewScope(c.Scope.SandboxPrefix, c.HarnessDirAbs())
}

// SandboxPrefix is the allowlisted container-name prefix (read-only accessor).
func (s *Scope) SandboxPrefix() string { return s.sandboxPrefix }

// HarnessDir is the allowlisted harness directory (read-only accessor).
func (s *Scope) HarnessDir() string { return s.harnessDir }

// lxdNameRe matches LXD-safe container names: lowercase alnum + hyphen, start
// and end alphanumeric, <= 63 chars. No dots, underscores, slashes, uppercase.
var lxdNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// harnessIDRe matches a three-digit harness id (lich-runner's --harness NNN).
// Pure digits leave no room for path separators, "..", or homoglyphs.
var harnessIDRe = regexp.MustCompile(`^[0-9]{3}$`)

// Validate returns nil if t is on the allowlist and ErrOutOfScope otherwise.
// It never executes, reads file contents, or trusts the Kind to anything the
// model said.
func (s *Scope) Validate(t Target) error {
	if err := validName(t.Name); err != nil {
		return err
	}
	switch t.Kind {
	case KindSandbox:
		return s.validateSandbox(t.Name)
	case KindHarness:
		return s.validateHarness(t.Name)
	default:
		return fmt.Errorf("%w: unknown target kind %q", ErrOutOfScope, t.Kind)
	}
}

// validName rejects empty, over-long, and any non-printable or non-ASCII rune.
// This kills embedded newlines (log injection), control bytes, and Unicode
// homoglyphs of the prefix before any kind-specific check.
func validName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty target name", ErrOutOfScope)
	}
	if len(name) > 253 {
		return fmt.Errorf("%w: target name too long (%d bytes)", ErrOutOfScope, len(name))
	}
	for i, r := range name {
		if r < 0x20 || r > 0x7E {
			return fmt.Errorf("%w: target name has non-printable/non-ASCII rune at byte %d", ErrOutOfScope, i)
		}
	}
	return nil
}

func (s *Scope) validateSandbox(name string) error {
	if !strings.HasPrefix(name, s.sandboxPrefix) {
		return fmt.Errorf("%w: sandbox %q lacks required prefix %q", ErrOutOfScope, name, s.sandboxPrefix)
	}
	if len(name) <= len(s.sandboxPrefix) {
		return fmt.Errorf("%w: sandbox name has no suffix after prefix %q", ErrOutOfScope, s.sandboxPrefix)
	}
	if !lxdNameRe.MatchString(name) {
		return fmt.Errorf("%w: sandbox %q is not a valid LXD name", ErrOutOfScope, name)
	}
	return nil
}

func (s *Scope) validateHarness(id string) error {
	if !harnessIDRe.MatchString(id) {
		return fmt.Errorf("%w: harness id %q is not a three-digit id", ErrOutOfScope, id)
	}
	// A harness is valid only if a file matching lich_<id>_* exists under the
	// allowlisted dir AND its symlink-resolved real path stays inside it.
	glob := filepath.Join(s.harnessDir, "lich_"+id+"_*")
	matches, err := filepath.Glob(glob)
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("%w: no harness file for id %q under %s", ErrOutOfScope, id, s.harnessDir)
	}
	realBase, err := filepath.EvalSymlinks(s.harnessDir)
	if err != nil {
		realBase = s.harnessDir
	}
	for _, m := range matches {
		real, err := filepath.EvalSymlinks(m)
		if err != nil {
			return fmt.Errorf("%w: harness %q cannot be resolved: %v", ErrOutOfScope, id, err)
		}
		if !within(realBase, real) {
			return fmt.Errorf("%w: harness %q resolves (via symlink) outside %s", ErrOutOfScope, id, s.harnessDir)
		}
		if info, err := os.Lstat(real); err == nil && info.IsDir() {
			return fmt.Errorf("%w: harness %q resolves to a directory", ErrOutOfScope, id)
		}
	}
	return nil
}

// within reports whether target is base or a path strictly under base, using a
// relative-path check so neither ".." escapes nor prefix aliasing (/a vs /ab)
// slips through.
func within(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
