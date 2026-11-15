// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goFiles returns the non-test .go files under root, skipping vendored and
// fixture trees.
func goFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestFirstPartyBinariesDoNotPoll: a binary under cmd/ or services/*/cmd/
// streams over gRPC and polls only while gRPC is down (ADR-147 Phase 6).
// WithHTTPPollStreams is for tools outside those trees.
func TestFirstPartyBinariesDoNotPoll(t *testing.T) {
	root := filepath.Join("..", "..")
	dirs := []string{filepath.Join(root, "cmd")}
	svcCmds, err := filepath.Glob(filepath.Join(root, "services", "*", "cmd"))
	if err != nil {
		t.Fatal(err)
	}
	dirs = append(dirs, svcCmds...)
	fset := token.NewFileSet()
	files := 0
	for _, dir := range dirs {
		for _, path := range goFiles(t, dir) {
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				continue // the build judges it
			}
			files++
			ast.Inspect(f, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && id.Name == "WithHTTPPollStreams" {
					t.Errorf("%s: a first-party binary asks for HTTP poll streams", fset.Position(id.Pos()))
				}
				return true
			})
		}
	}
	if files < 50 {
		t.Fatalf("parsed only %d files under cmd/ and services/*/cmd/: the walk is broken", files)
	}
}

// TestOneStreamOpener: exactly one function in this package opens a topic
// stream (the generated StreamTopics call), so a stream's transport,
// cursor and liveness cannot drift between copies (Developer D5: three
// stream clients recovered three ways).
func TestOneStreamOpener(t *testing.T) {
	fset := token.NewFileSet()
	var openers []string
	for _, path := range goFiles(t, ".") {
		if strings.Contains(path, string(filepath.Separator)) {
			continue // this package only, not mock/ or wotantest/
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "StreamTopics" {
					openers = append(openers, path+":"+fn.Name.Name)
				}
				return true
			})
		}
	}
	if len(openers) != 1 {
		t.Fatalf("StreamTopics is called from %d places, want exactly one: %v", len(openers), openers)
	}
}
