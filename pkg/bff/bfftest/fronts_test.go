// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bfftest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/bff/bfftest"
)

func write(t *testing.T, root, rel, src string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
}

func names(fs []bfftest.Front) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return strings.Join(out, ",")
}

func TestGoFronts_FindsEveryBinaryThatBuildsABFF(t *testing.T) {
	root := t.TempDir()
	write(t, root, "cmd/alpha/main.go", `package main
import "unheaded/pkg/bff"
func main() { _, _ = bff.New(bff.Config{}) }
`)
	// A front may build its BFF in a package below its directory, under
	// another import name.
	write(t, root, "cmd/beta/internal/server/login.go", `package server
import login "unheaded/pkg/bff"
func start() { _, _ = login.New(login.Config{}) }
`)
	write(t, root, "services/gamma/cmd/gamma/main.go", `package main
import "unheaded/pkg/bff"
var build = bff.New
`)
	// Serving the shared script is not a login.
	write(t, root, "cmd/delta/main.go", `package main
import "unheaded/pkg/bff"
var _ = bff.ServeNavJS
`)
	// A test's BFF is no front, and a local named bff is not the package.
	write(t, root, "cmd/epsilon/main_test.go", `package main
import "unheaded/pkg/bff"
var _, _ = bff.New(bff.Config{})
`)
	write(t, root, "cmd/zeta/main.go", `package main
type thing struct{}
func (thing) New() {}
func main() { bff := thing{}; bff.New() }
`)
	got, err := bfftest.GoFronts(root)
	if err != nil {
		t.Fatal(err)
	}
	if names(got) != "alpha,beta,gamma" {
		t.Fatalf("fronts %q, want alpha,beta,gamma", names(got))
	}
	if len(got[1].Calls) != 1 || !strings.HasPrefix(got[1].Calls[0], "cmd/beta/internal/server/login.go:") {
		t.Fatalf("beta's call site: %v", got[1].Calls)
	}
}

// A BFF built in a shared package belongs to no one binary, so it cannot be
// counted: that is an error, not a front silently left out.
func TestGoFronts_RefusesABFFBuiltInALibrary(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pkg/shared/login.go", `package shared
import "unheaded/pkg/bff"
func Start() { _, _ = bff.New(bff.Config{}) }
`)
	if _, err := bfftest.GoFronts(root); err == nil || !strings.Contains(err.Error(), "pkg/shared/login.go") {
		t.Fatalf("err = %v, want one naming pkg/shared/login.go", err)
	}
}

func TestGoFronts_RefusesADotImport(t *testing.T) {
	root := t.TempDir()
	write(t, root, "cmd/alpha/main.go", `package main
import . "unheaded/pkg/bff"
func main() { _, _ = New(Config{}) }
`)
	if _, err := bfftest.GoFronts(root); err == nil || !strings.Contains(err.Error(), "cmd/alpha/main.go") {
		t.Fatalf("err = %v, want one naming cmd/alpha/main.go", err)
	}
}

// The repository itself: the three pkg/bff fronts of 2026-11-21. A fourth
// shows up here and in cmd/unheaded-auth's budget sum without anyone adding
// it to a list.
func TestGoFronts_ThisRepository(t *testing.T) {
	got, err := bfftest.GoFronts("../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dashboard-backend", "kanban-app", "wiki-server"} {
		if !strings.Contains(","+names(got)+",", ","+want+",") {
			t.Errorf("front %s not found in %q", want, names(got))
		}
	}
}
