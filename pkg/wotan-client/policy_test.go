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

// clientConstructors are the calls that build a Wotan client.
var clientConstructors = map[string]bool{
	"NewClient": true, "NewClientWithGRPC": true, "NewClientWithTLS": true,
	"NewTopicStreamClient": true, "NewTopicStreamClientWithTLS": true,
	"NewGRPCClient": true, "NewGRPCClientWithTLS": true,
}

// clientBuilds returns the positions in path where a Wotan client is built:
// a constructor of this package, or transport.NewWotanClient.
func clientBuilds(t *testing.T, fset *token.FileSet, path string) []string {
	t.Helper()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.ParseComments)
	if err != nil {
		return nil // the build judges it
	}
	names := map[string]string{} // local import name -> which package
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		var local string
		switch p {
		case "unheaded/pkg/wotan-client":
			local = "wotanClient"
		case "unheaded/pkg/transport":
			local = "transport"
		default:
			continue
		}
		if imp.Name != nil {
			local = imp.Name.Name
		}
		names[local] = p
	}
	if len(names) == 0 {
		return nil
	}
	f, err = parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch names[id.Name] {
		case "unheaded/pkg/wotan-client":
			if clientConstructors[sel.Sel.Name] {
				out = append(out, fset.Position(call.Pos()).String())
			}
		case "unheaded/pkg/transport":
			if sel.Sel.Name == "NewWotanClient" {
				out = append(out, fset.Position(call.Pos()).String())
			}
		}
		return true
	})
	return out
}

// TestLibrariesBuildNoClient: a shared package takes the process's Wotan
// client; it does not build its own (ADR-147 Phase 7, Developer D6:
// discovery, health reports and log forwarding each built one, so a
// process held up to four). pkg/wotan-client and pkg/transport build them.
func TestLibrariesBuildNoClient(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	for _, path := range goFiles(t, filepath.Join(root, "pkg")) {
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, filepath.Join("pkg", "wotan-client")+string(filepath.Separator)) ||
			strings.HasPrefix(rel, filepath.Join("pkg", "transport")+string(filepath.Separator)) {
			continue
		}
		for _, at := range clientBuilds(t, fset, path) {
			t.Errorf("%s: a library builds a Wotan client; take the process's instead", at)
		}
	}
}

// oneClientAllowlist may only shrink: binaries with more than one client
// construction, each run in its own process.
var oneClientAllowlist = map[string]int{
	"cmd/upc-cartridgectl": 2, // two subcommands; one runs per process
}

// TestOneWotanClientPerProcess: each binary under cmd/ and services/*/cmd/
// builds at most one Wotan client and hands it to everything that needs
// one (ADR-147 Phase 7, Developer D6).
func TestOneWotanClientPerProcess(t *testing.T) {
	root := filepath.Join("..", "..")
	bins, err := filepath.Glob(filepath.Join(root, "cmd", "*"))
	if err != nil {
		t.Fatal(err)
	}
	svcCmds, err := filepath.Glob(filepath.Join(root, "services", "*", "cmd"))
	if err != nil {
		t.Fatal(err)
	}
	bins = append(bins, svcCmds...)
	fset := token.NewFileSet()
	checked := 0
	for _, dir := range bins {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		rel, _ := filepath.Rel(root, dir)
		rel = filepath.ToSlash(rel)
		var builds []string
		for _, path := range goFiles(t, dir) {
			builds = append(builds, clientBuilds(t, fset, path)...)
		}
		checked++
		limit := 1
		if n, ok := oneClientAllowlist[rel]; ok {
			limit = n
			if len(builds) < n {
				t.Errorf("%s builds %d clients; lower its allowlist entry from %d", rel, len(builds), n)
			}
		}
		if len(builds) > limit {
			t.Errorf("%s builds %d Wotan clients (at most %d): %v", rel, len(builds), limit, builds)
		}
	}
	if checked < 30 {
		t.Fatalf("checked only %d binaries: the walk is broken", checked)
	}
}
