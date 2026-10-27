// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package healthreport

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Every service that announces itself on system.discovery also votes on
// Akira's ballot (ADR-029, M-200): a file calling discovery.Start must call
// healthreport.StartService too, so a new service cannot join one and skip
// the other.
func TestEveryDiscoveryCallerReports(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	callers := 0
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
			discovers, reports := false, false
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				switch {
				case pkg.Name == "discovery" && sel.Sel.Name == "Start":
					discovers = true
				case pkg.Name == "healthreport" && sel.Sel.Name == "StartService":
					reports = true
				}
				return true
			})
			if discovers {
				callers++
				if !reports {
					t.Errorf("%s calls discovery.Start but not healthreport.StartService", path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if callers < 11 {
		t.Fatalf("found %d discovery.Start callers; the walk is broken (12 on 2026-10-27)", callers)
	}
}
