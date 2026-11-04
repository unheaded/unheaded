// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCommands(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		in   string
		want int
	}{
		{"go program", []string{"go"}, "package main\nimport \"fmt\"\nfunc main() { fmt.Println(1) }\n", 0},
		{"go fence", []string{"go"}, "Try this:\n```go\nx := 1\n_ = x\n```\n", 0},
		{"go syntax", []string{"go"}, "package main\nfunc main( {\n", 1},
		{"go undefined", []string{"go"}, "package main\nfunc main() { y() }\n", 1},
		{"go empty", []string{"go"}, "   \n", 1},
		{"json object", []string{"json"}, `{"a": 1}`, 0},
		{"json fence", []string{"json"}, "```json\n[1, 2]\n```\n", 0},
		{"json trailing", []string{"json"}, `{"a": 1} x`, 1},
		{"json bad", []string{"json"}, `{a: 1}`, 1},
		{"cite ok", []string{"cite", "--root", root}, "see `pkg/a.go` line 1", 0},
		{"cite with line", []string{"cite", "--root", root}, "see pkg/a.go:1", 0},
		{"cite missing", []string{"cite", "--root", root}, "see `pkg/b.go`", 1},
		{"cite one of two missing", []string{"cite", "--root", root}, "`pkg/a.go` and `pkg/b.go`", 1},
		{"cite none", []string{"cite", "--root", root}, "no citation here", 1},
		{"cite escape", []string{"cite", "--root", root}, "`../etc/passwd.go`", 1},
		{"cite absolute", []string{"cite", "--root", root}, "`/etc/hosts.go`", 1},
		{"unknown", []string{"yaml"}, "x", 2},
		{"none", nil, "x", 2},
		{"help", []string{"--help"}, "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if got := runCheckCmd(c.args, strings.NewReader(c.in), &out, &errb); got != c.want {
				t.Fatalf("exit %d, want %d: %s", got, c.want, errb.String())
			}
		})
	}
}

// --help must answer without reading stdin: on a terminal that would hang.
func TestCheckHelpDoesNotReadStdin(t *testing.T) {
	var out, errb bytes.Buffer
	if got := runCheckCmd([]string{"--help"}, panicReader{}, &out, &errb); got != 0 || !strings.Contains(out.String(), "go, json or cite") {
		t.Fatalf("exit %d: %q", got, out.String())
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("stdin was read") }

// With no Go source tree, an import cannot be type-checked: the snippet is
// judged on its syntax, and says so, instead of failing every import.
func TestCheckGoWithoutSourceTree(t *testing.T) {
	old := goSourceAvailable
	goSourceAvailable = func() bool { return false }
	t.Cleanup(func() { goSourceAvailable = old })
	if err := checkGo("package main\nimport \"fmt\"\nfunc main() { fmt.Println(1) }\n"); err != nil {
		t.Fatal(err)
	}
	if err := checkGo("package main\nfunc main( {\n"); err == nil {
		t.Fatal("a syntax error must still reject")
	}
}

func TestRunDispatchesCheck(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"check", "json"}, strings.NewReader("[]"), &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
}
