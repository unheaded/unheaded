// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package inventory reads the components a tree pins from its own lock data:
// go.mod, Cargo.lock, pinned requirements files and the repo's SBOM JSON
// (DP-721 stage 1a). Only the standard library is used.
package inventory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"unheaded/pkg/threatfeed/osv"
)

// maxLine bounds one manifest line; hash-locked requirements stay far under it.
const maxLine = 1 << 20

// Component is one third-party package at one version.
type Component struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Source    string `json:"source"`
}

func scanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64<<10), maxLine)
	return s
}

// ReadGoMod returns every required module. A module replaced by a local path
// is first-party code and is skipped. Versions lose their leading v.
func ReadGoMod(r io.Reader, source string) ([]Component, error) {
	var req [][2]string
	local := map[string]bool{}
	block := ""
	s := scanner(r)
	for s.Scan() {
		line := s.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if block != "" {
			if f[0] == ")" {
				block = ""
				continue
			}
			f = append([]string{block}, f...)
		} else if len(f) == 2 && f[1] == "(" {
			block = f[0]
			continue
		}
		switch f[0] {
		case "require":
			if len(f) >= 3 {
				req = append(req, [2]string{f[1], f[2]})
			}
		case "replace":
			for i, w := range f {
				if w == "=>" && i+1 < len(f) && (strings.HasPrefix(f[i+1], "./") || strings.HasPrefix(f[i+1], "../")) {
					local[f[1]] = true
				}
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	var out []Component
	for _, m := range req {
		if !local[m[0]] {
			out = append(out, Component{osv.EcoGo, m[0], strings.TrimPrefix(m[1], "v"), source})
		}
	}
	return out, nil
}

// ReadCargoLock returns every package with a registry source; path and
// workspace crates have none and are skipped.
func ReadCargoLock(r io.Reader, source string) ([]Component, error) {
	var out []Component
	name, version, src := "", "", ""
	flush := func() {
		if name != "" && version != "" && strings.HasPrefix(src, "registry+") {
			out = append(out, Component{osv.EcoCrates, name, version, source})
		}
		name, version, src = "", "", ""
	}
	unquote := func(l string) string {
		_, v, _ := strings.Cut(l, "=")
		return strings.Trim(strings.TrimSpace(v), `"`)
	}
	s := scanner(r)
	for s.Scan() {
		l := s.Text()
		switch {
		case l == "[[package]]":
			flush()
		case strings.HasPrefix(l, "name = "):
			name = unquote(l)
		case strings.HasPrefix(l, "version = "):
			version = unquote(l)
		case strings.HasPrefix(l, "source = "):
			src = unquote(l)
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	flush()
	return out, nil
}

var pinned = regexp.MustCompile(`^([A-Za-z0-9._-]+)(?:\[[^\]]*\])?==([^=\s]+)$`)

// ReadRequirements returns the name==version lines and, separately, the
// requirement lines that are not exact pins: what those install changes by
// the day, so the caller reports them rather than claiming they were checked.
func ReadRequirements(r io.Reader, source string) (comps []Component, unpinned []string, err error) {
	s := scanner(r)
	for s.Scan() {
		l := s.Text()
		if i := strings.IndexByte(l, '#'); i >= 0 {
			l = l[:i]
		}
		if i := strings.IndexByte(l, ';'); i >= 0 {
			l = l[:i]
		}
		l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l), `\`))
		if l == "" || strings.HasPrefix(l, "-") {
			continue
		}
		if m := pinned.FindStringSubmatch(l); m != nil {
			comps = append(comps, Component{osv.EcoPyPI, osv.NormalizeName(osv.EcoPyPI, m[1]), m[2], source})
			continue
		}
		unpinned = append(unpinned, source+": "+l)
	}
	if err := s.Err(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", source, err)
	}
	return comps, unpinned, nil
}

// ReadSBOMJSON reads sbom/go-dependencies.json or sbom/rust-dependencies.json.
func ReadSBOMJSON(r io.Reader, source string) ([]Component, error) {
	var doc struct {
		Packages []struct {
			Module  string `json:"module"`
			Crate   string `json:"crate"`
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	out := make([]Component, 0, len(doc.Packages))
	for _, p := range doc.Packages {
		switch {
		case p.Module != "" && p.Version != "":
			out = append(out, Component{osv.EcoGo, p.Module, strings.TrimPrefix(p.Version, "v"), source})
		case p.Crate != "" && p.Version != "":
			out = append(out, Component{osv.EcoCrates, p.Crate, p.Version, source})
		default:
			return nil, fmt.Errorf("%s: a package has no module or crate name, or no version", source)
		}
	}
	return out, nil
}

// skipDirs are vendored trees, build output and frozen fixtures.
var skipDirs = map[string]bool{
	".git": true, ".claude": true, "target": true, "node_modules": true, "testdata": true,
	"fixtures": true, "llama.cpp": true, "doom": true, "upstream": true, "vendor": true,
}

// isManifest reports whether a file name is lock data this package reads.
func isManifest(name string) bool {
	switch name {
	case "go.mod", "Cargo.lock", "python-pip-freeze.txt":
		return true
	}
	return strings.Contains(name, "requirements") && strings.HasSuffix(name, ".txt")
}

// Discover lists the lock data under root, relative and sorted.
func Discover(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if isManifest(d.Name()) {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// ScanTree reads everything Discover finds under root. Components are unique
// by ecosystem, name and version, keeping the first source, sorted.
func ScanTree(root string) (comps []Component, unpinned []string, err error) {
	paths, err := Discover(root)
	if err != nil {
		return nil, nil, err
	}
	seen := map[[3]string]bool{}
	for _, rel := range paths {
		got, un, err := readFile(filepath.Join(root, rel), rel)
		if err != nil {
			return nil, nil, err
		}
		unpinned = append(unpinned, un...)
		for _, c := range got {
			k := [3]string{c.Ecosystem, c.Name, c.Version}
			if !seen[k] {
				seen[k] = true
				comps = append(comps, c)
			}
		}
	}
	sort.Slice(comps, func(i, j int) bool {
		a, b := comps[i], comps[j]
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})
	return comps, unpinned, nil
}

func readFile(path, rel string) ([]Component, []string, error) {
	f, err := os.Open(path) // #nosec G304 -- path comes from Discover under the operator's root
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	switch filepath.Base(rel) {
	case "go.mod":
		c, err := ReadGoMod(f, rel)
		return c, nil, err
	case "Cargo.lock":
		c, err := ReadCargoLock(f, rel)
		return c, nil, err
	}
	return ReadRequirements(f, rel)
}
