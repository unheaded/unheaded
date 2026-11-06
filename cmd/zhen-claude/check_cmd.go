// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	gobuild "go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// runCheckCmd is `zhen-claude check go|json|cite`: the standard -check
// commands (DP-312). The answer is on stdin. Exit 0 accepts, 1 rejects,
// 2 is bad usage. Nothing here runs the answer: stdlib parsers only.
//
//	zhen-claude -route auto -check "zhen-claude check json" -p "..."
func runCheckCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		_, _ = fmt.Fprintln(stdout, "usage: zhen-claude check go|json|cite [--root DIR]: want go, json or cite, the answer on stdin")
		return 0
	}
	if len(args) == 0 || (args[0] != "go" && args[0] != "json" && args[0] != "cite") {
		_, _ = fmt.Fprintln(stderr, "zhen-claude check: want go, json or cite")
		return 2
	}
	name := args[0]
	root := "."
	if name == "cite" {
		fs := flag.NewFlagSet("check cite", flag.ContinueOnError)
		fs.SetOutput(stderr)
		fs.StringVar(&root, "root", ".", "directory the cited paths are relative to")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "zhen-claude check %s: %v\n", name, err)
		return 1
	}
	answer := string(raw)
	switch name {
	case "go":
		err = checkGo(answer)
	case "json":
		err = checkJSON(answer)
	default:
		err = checkCite(answer, root)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "zhen-claude check %s: %v\n", name, err)
		return 1
	}
	return 0
}

// checkNote receives the one non-fatal notice checkGo can print.
var checkNote io.Writer = os.Stderr

// goSourceAvailable reports whether go/importer's "source" mode can work:
// it reads GOROOT/src, not a go binary on PATH. Import go/build as gobuild:
// this package already has a func named build (main.go).
var goSourceAvailable = func() bool {
	_, err := os.Stat(filepath.Join(gobuild.Default.GOROOT, "src", "fmt"))
	return err == nil
}

// checkGo parses the snippet, then type-checks it with the source importer.
func checkGo(answer string) error {
	src := answer
	if b, ok := fencedBlock(answer, "go"); ok {
		src = b
	}
	if strings.TrimSpace(src) == "" {
		return errors.New("empty answer")
	}
	fset := token.NewFileSet()
	var f *ast.File
	var firstErr error
	for _, cand := range []string{src, "package snippet\n" + src, "package snippet\nfunc _() {\n" + src + "\n}\n"} {
		parsed, err := parser.ParseFile(fset, "snippet.go", cand, parser.AllErrors)
		if err == nil {
			f = parsed
			break
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if f == nil {
		return firstErr
	}
	if !goSourceAvailable() {
		_, _ = fmt.Fprintln(checkNote, "zhen-claude check go: no Go source tree, syntax only")
		return nil
	}
	conf := &types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	_, err := conf.Check("snippet", fset, []*ast.File{f}, nil)
	return err
}

// checkJSON accepts exactly one JSON value and nothing after it.
func checkJSON(answer string) error {
	src := answer
	if b, ok := fencedBlock(answer, "json"); ok {
		src = b
	}
	dec := json.NewDecoder(strings.NewReader(src))
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	if err := dec.Decode(&v); !errors.Is(err, io.EOF) {
		return errors.New("data after the first JSON value")
	}
	return nil
}

var citeRE = regexp.MustCompile(`[A-Za-z0-9_./-]+\.(go|rs|py|md|sql|ya?ml|sh|toml|json)\b`)

// checkCite requires at least one cited path, and every one must exist
// under root.
func checkCite(answer, root string) error {
	matches := citeRE.FindAllString(answer, -1)
	if len(matches) == 0 {
		return errors.New("no file citation found")
	}
	for _, m := range matches {
		if filepath.IsAbs(m) || !filepath.IsLocal(m) {
			return fmt.Errorf("cited path %s is outside the tree", m)
		}
		if _, err := os.Stat(filepath.Join(root, m)); err != nil {
			return fmt.Errorf("cited path %s does not exist", m)
		}
	}
	return nil
}

// fencedBlock returns the first block opened by a line "```<lang>", up to
// the next line "```".
func fencedBlock(answer, lang string) (string, bool) {
	lines := strings.Split(answer, "\n")
	open := "```" + lang
	for i, l := range lines {
		if strings.TrimSpace(l) != open {
			continue
		}
		var body []string
		for _, m := range lines[i+1:] {
			if strings.TrimSpace(m) == "```" {
				return strings.Join(body, "\n") + "\n", true
			}
			body = append(body, m)
		}
		return "", false
	}
	return "", false
}
