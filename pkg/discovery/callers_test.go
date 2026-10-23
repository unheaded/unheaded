// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// A service announces the port peers will dial. Most services take their
// listen address from a flag or env var (-listen, PORT, HTTP_ADDR, ...) but
// announced a literal: move the listener and discovery keeps sending peers
// to the old port, which is how trace-collector came to announce 16670
// while serving only 16671. The announced port must be derived from the
// address the service actually listens on, never written out as a number.
func TestStartCallersDoNotAnnounceLiteralPorts(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	var offenders []string
	for _, dir := range []string{"cmd", "services"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil // not ours to judge; the build catches it
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 4 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Start" {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "discovery" {
					return true
				}
				if lit, ok := call.Args[3].(*ast.BasicLit); ok && lit.Kind == token.INT {
					offenders = append(offenders, fset.Position(call.Pos()).String()+" announces "+lit.Value)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range offenders {
		t.Error(o)
	}
}
