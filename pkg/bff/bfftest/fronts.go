// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bfftest

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
)

// bffImportPath is the package whose New makes a login front.
const bffImportPath = "unheaded/pkg/bff"

// Front is one binary that logs people in at the issuer through pkg/bff:
// it builds a BFF, so it spends login failures from its host's address.
type Front struct {
	// Name is the binary: the directory under cmd/ or services/.
	Name string
	// Calls are the places that build the BFF, as path:line from the root.
	Calls []string
}

// GoFronts reads the Go source under root and returns every binary that
// refers to bff.New outside its tests, sorted by name. The fronts are not
// declared in any list: this reads them from where they are built, so a
// test over the result sees a new front the day it is written.
//
// A reference from a package that is no binary's own (anything outside
// cmd/<name>/ and services/<name>/) is an error, and so is a dot import:
// neither can be laid at one binary's door, and an uncounted front is what
// this function exists to prevent.
func GoFronts(root string) ([]Front, error) {
	byName := map[string]*Front{}
	fset := token.NewFileSet()
	tree := os.DirFS(root)
	err := fs.WalkDir(tree, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(rel, d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		src, rerr := fs.ReadFile(tree, rel)
		if rerr != nil {
			return rerr
		}
		if !bytes.Contains(src, []byte(`"`+bffImportPath+`"`)) {
			return nil
		}
		lines, perr := newRefs(fset, rel, src)
		if perr != nil {
			return perr
		}
		if len(lines) == 0 {
			return nil
		}
		name := binaryOf(rel)
		if name == "" {
			return fmt.Errorf("%s:%d builds a BFF outside cmd/<name>/ and services/<name>/: it cannot be counted as one front", rel, lines[0])
		}
		f := byName[name]
		if f == nil {
			f = &Front{Name: name}
			byName[name] = f
		}
		for _, l := range lines {
			f.Calls = append(f.Calls, rel+":"+strconv.Itoa(l))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]Front, 0, len(byName))
	for _, f := range byName {
		sort.Strings(f.Calls)
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// skipDir leaves out what holds no first-party Go source, and pkg/bff
// itself.
func skipDir(rel, name string) bool {
	switch name {
	case "node_modules", "target", "testdata", "vendor":
		return true
	}
	if rel != "." && strings.HasPrefix(name, ".") {
		return true
	}
	return rel == "pkg/bff"
}

// binaryOf is the binary a file belongs to, or "".
func binaryOf(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) >= 3 && (parts[0] == "cmd" || parts[0] == "services") {
		return parts[1]
	}
	return ""
}

// newRefs returns the lines of src that refer to pkg/bff's New.
func newRefs(fset *token.FileSet, rel string, src []byte) ([]int, error) {
	file, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	local := ""
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != bffImportPath {
			continue
		}
		switch {
		case imp.Name == nil:
			local = "bff"
		case imp.Name.Name == ".":
			return nil, fmt.Errorf("%s: dot import of %s: its New cannot be told from another", rel, bffImportPath)
		case imp.Name.Name == "_":
		default:
			local = imp.Name.Name
		}
	}
	if local == "" {
		return nil, nil
	}
	var lines []int
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "New" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == local {
			lines = append(lines, fset.Position(sel.Pos()).Line)
		}
		return true
	})
	return lines, nil
}
