// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	cssURL    = regexp.MustCompile(`url\(\s*["']?([^"')]+)["']?\s*\)`)
	cssImport = regexp.MustCompile(`@import\s+["']([^"']+)["']`)
	fontFace  = regexp.MustCompile(`@font-face`)
)

// cssFaults names what a stylesheet at name (inside files) asks a browser to
// fetch that this origin does not serve: another host, or a file that is not
// there. It also names a @font-face, which no stylesheet here may carry: the
// pages use system fonts and no font file is shipped (decision pack
// 2026-11-18 row 41, M-206.1).
func cssFaults(files fs.FS, name, css string) []string {
	var faults []string
	if fontFace.MatchString(css) {
		faults = append(faults, name+": @font-face (no font file is shipped; use the system's)")
	}
	refs := cssURL.FindAllStringSubmatch(css, -1)
	refs = append(refs, cssImport.FindAllStringSubmatch(css, -1)...)
	for _, m := range refs {
		ref := strings.TrimSpace(m[1])
		switch {
		case strings.HasPrefix(ref, "data:"), strings.HasPrefix(ref, "#"):
			continue
		case strings.Contains(ref, "//"):
			faults = append(faults, name+": loads from another host: "+ref)
			continue
		}
		target := path.Join(path.Dir(name), ref)
		if strings.HasPrefix(ref, "/") {
			target = path.Join("static", ref)
		}
		if _, err := fs.Stat(files, target); err != nil {
			faults = append(faults, name+": names a file that is not there: "+ref)
		}
	}
	return faults
}

// The dashboard's stylesheets said "self-hosted fonts" and pointed two
// @font-face rules at files that were never in the repository, so every page
// load asked for two files and got two 404s.
func TestStylesheetsLoadOnlyWhatIsServed(t *testing.T) {
	seen := 0
	err := fs.WalkDir(staticFiles, "static", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(name, ".css") {
			return err
		}
		css, err := fs.ReadFile(staticFiles, name)
		if err != nil {
			return err
		}
		seen++
		for _, fault := range cssFaults(staticFiles, name, string(css)) {
			t.Error(fault)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("no stylesheet under static/: the walk is not reading the embedded files")
	}
}

// dashboard/ holds a second copy of the pages (the image copies it to
// /app/viz). Its stylesheets are held to the same rule against the files
// beside them.
func TestRepoDashboardStylesheetsLoadOnlyWhatIsThere(t *testing.T) {
	root := filepath.Join("..", "..", "dashboard")
	sheets, err := filepath.Glob(filepath.Join(root, "css", "*.css"))
	if err != nil || len(sheets) == 0 {
		t.Fatalf("no stylesheet under %s/css (err %v)", root, err)
	}
	tree := os.DirFS(root)
	for _, sheet := range sheets {
		css, err := os.ReadFile(sheet)
		if err != nil {
			t.Fatal(err)
		}
		rel := path.Join("css", filepath.Base(sheet))
		for _, fault := range cssFaults(tree, rel, string(css)) {
			t.Error(fault)
		}
	}
}

func TestCSSFaultsFindPlantedLoads(t *testing.T) {
	tree := os.DirFS(t.TempDir())
	for _, tc := range []struct {
		name, css string
		want      int
	}{
		{"clean", `body { color: red; background: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E"); }`, 0},
		{"fragment", `a { fill: url(#grad); }`, 0},
		{"font-face with a missing file", `@font-face { font-family: X; src: url('../assets/fonts/X.woff2') format('woff2'); }`, 2},
		{"another host", `@font-face { font-family: X; src: url(https://fonts.gstatic.com/x.woff2); }`, 2},
		{"scheme-relative host", `body { background: url(//cdn.example/a.png); }`, 1},
		{"import from another host", `@import "https://fonts.googleapis.com/css2?family=X";`, 1},
		{"missing image", `body { background: url(img/none.png); }`, 1},
	} {
		if got := cssFaults(tree, "css/a.css", tc.css); len(got) != tc.want {
			t.Errorf("%s: faults = %v, want %d", tc.name, got, tc.want)
		}
	}
}
