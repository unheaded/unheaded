// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package modules answers one question for platform code: is an optional
// module on (ADR-149)? A module is a set of host units behind one systemd
// target (Zhen's is unheaded-zhen.target). While the target is active, a
// oneshot inside it publishes the module's fragments (rules, scrape jobs,
// health targets) under Dir()/<name>/; when the target stops, the directory
// goes. So the directory exists only while the module is on, and its absence
// is the normal "no module is on" state, never an error.
//
// Standard library only. Nothing here logs.
package modules

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// EnvDir overrides DefaultDir (tests, containers that mount it elsewhere).
const EnvDir = "UNHEADED_MODULES_DIR"

// DefaultDir is where module fragments live while a module's switch is on.
const DefaultDir = "/run/unheaded/modules"

// nameRE is a module name: lower case, starts with a letter, at most 32.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// Dir is $UNHEADED_MODULES_DIR, else DefaultDir.
func Dir() string {
	if d := os.Getenv(EnvDir); d != "" {
		return filepath.Clean(d)
	}
	return DefaultDir
}

// ValidName reports whether name is a well-formed module name.
func ValidName(name string) bool { return nameRE.MatchString(name) }

// Enabled reports whether module name is on: Dir()/name is a directory
// (not a symlink). A malformed name is never on.
func Enabled(name string) bool {
	if !ValidName(name) {
		return false
	}
	fi, err := os.Lstat(filepath.Join(Dir(), name))
	return err == nil && fi.IsDir()
}

// cleanRel returns rel cleaned, or "" if it is empty, absolute or climbs out.
func cleanRel(rel string) string {
	if rel == "" || filepath.IsAbs(rel) {
		return ""
	}
	c := filepath.Clean(rel)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return ""
	}
	return c
}

// Fragments returns, sorted, every Dir()/<module>/rel that is a regular file
// (not a symlink), for every well-formed module name. rel is cleaned and
// refused (nil) if it is absolute or leaves the module's directory.
func Fragments(rel string) []string {
	c := cleanRel(rel)
	if c == "" {
		return nil
	}
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !ValidName(name) || e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, name, c)
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ModuleOf returns the <module> element of a path under Dir()/<module>/...,
// or "" for any other path.
func ModuleOf(path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(Dir(), filepath.Clean(path))
	if err != nil {
		return ""
	}
	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) != 2 || parts[1] == "" || !ValidName(parts[0]) {
		return ""
	}
	return parts[0]
}
