// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package httputil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// isResponseWriter reports whether a parameter's type is http.ResponseWriter.
func isResponseWriter(e ast.Expr, httpName string) bool {
	s, ok := e.(*ast.SelectorExpr)
	if !ok || s.Sel.Name != "ResponseWriter" {
		return false
	}
	pkg, ok := s.X.(*ast.Ident)
	return ok && pkg.Name == httpName
}

// importName is the name file f knows the package at path by, "" without it.
func importName(f *ast.File, path string) string {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != path {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return path[strings.LastIndexByte(path, '/')+1:]
	}
	return ""
}

// encoderScan looks for a JSON encoder made on a ResponseWriter in one file.
type encoderScan struct {
	fset      *token.FileSet
	jsonName  string
	httpName  string
	handlers  int
	offenders []string
}

func (s *encoderScan) function(ft *ast.FuncType, body *ast.BlockStmt, outer map[string]bool) {
	writers := make(map[string]bool, len(outer)+1)
	for name := range outer {
		writers[name] = true
	}
	if ft.Params != nil {
		for _, p := range ft.Params.List {
			for _, n := range p.Names {
				if isResponseWriter(p.Type, s.httpName) {
					writers[n.Name] = true
					s.handlers++
				} else {
					delete(writers, n.Name) // shadowed
				}
			}
		}
	}
	if body == nil {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			s.function(v.Type, v.Body, writers)
			return false
		case *ast.AssignStmt: // rw := w
			if len(v.Lhs) == len(v.Rhs) {
				for i, rhs := range v.Rhs {
					id, ok := rhs.(*ast.Ident)
					lhs, lok := v.Lhs[i].(*ast.Ident)
					if ok && lok && writers[id.Name] {
						writers[lhs.Name] = true
					}
				}
			}
		case *ast.CallExpr:
			sel, ok := v.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "NewEncoder" || len(v.Args) != 1 {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != s.jsonName {
				return true
			}
			if arg, ok := v.Args[0].(*ast.Ident); ok && writers[arg.Name] {
				s.offenders = append(s.offenders, s.fset.Position(v.Pos()).String())
			}
		}
		return true
	})
}

// scanSource returns the encoders made on a ResponseWriter in one Go file,
// and how many ResponseWriter parameters it saw.
func scanSource(path string, src []byte) (offenders []string, handlers int, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, 0, err
	}
	s := &encoderScan{
		fset:     fset,
		jsonName: importName(f, "encoding/json"),
		httpName: importName(f, "net/http"),
	}
	if s.jsonName == "" || s.httpName == "" {
		return nil, 0, nil
	}
	for _, d := range f.Decls {
		switch v := d.(type) {
		case *ast.FuncDecl:
			s.function(v.Type, v.Body, nil)
		case *ast.GenDecl:
			ast.Inspect(v, func(n ast.Node) bool {
				if lit, ok := n.(*ast.FuncLit); ok {
					s.function(lit.Type, lit.Body, nil)
					return false
				}
				return true
			})
		}
	}
	return s.offenders, s.handlers, nil
}

// TestNoHandlerEncodesToTheResponseWriter is the ratchet: an Encoder on the
// ResponseWriter writes nothing when a value will not marshal, so the client
// gets an empty 200 or the handler's status with no body. Use WriteJSON. Do
// not add an exception. scripts/check-json-responses.sh runs this.
func TestNoHandlerEncodesToTheResponseWriter(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	handlers := 0
	for _, dir := range []string{"cmd", "services", "pkg", "deploy", "internal", "demos"} {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "vendor", "testdata", "target":
					return filepath.SkipDir
				}
				if path == filepath.Join(root, "pkg", "httputil") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			found, n, err := scanSource(path, src)
			if err != nil {
				return nil // not ours to judge; the build catches it
			}
			offenders = append(offenders, found...)
			handlers += n
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if handlers < 200 {
		t.Fatalf("saw %d ResponseWriter parameters; the walk is broken", handlers)
	}
	for _, o := range offenders {
		t.Errorf("%s: json.NewEncoder on a ResponseWriter; use httputil.WriteJSON", o)
	}
}

// The scanner must see every spelling, or the ratchet is green for nothing.
func TestScanSource_FindsEverySpelling(t *testing.T) {
	const head = "package p\nimport (\n\t\"encoding/json\"\n\t\"net/http\"\n)\n"
	for name, body := range map[string]string{
		"plain":        `func h(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(1) }`,
		"other name":   `func h(rw http.ResponseWriter) { _ = json.NewEncoder(rw).Encode(1) }`,
		"encoder var":  `func h(w http.ResponseWriter) { enc := json.NewEncoder(w); _ = enc.Encode(1) }`,
		"alias":        `func h(w http.ResponseWriter) { x := w; json.NewEncoder(x).Encode(1) }`,
		"func literal": `func r() http.HandlerFunc { return func(w http.ResponseWriter, _ *http.Request) { json.NewEncoder(w).Encode(1) } }`,
		"package var":  `var h = func(w http.ResponseWriter) { json.NewEncoder(w).Encode(1) }`,
		"captured":     `func h(w http.ResponseWriter) { func() { json.NewEncoder(w).Encode(1) }() }`,
	} {
		found, _, err := scanSource("x.go", []byte(head+body+"\n"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(found) != 1 {
			t.Errorf("%s: found %d, want 1", name, len(found))
		}
	}

	aliased := "package p\nimport (\n\tj \"encoding/json\"\n\th \"net/http\"\n)\nfunc f(w h.ResponseWriter) { j.NewEncoder(w).Encode(1) }\n"
	if found, _, err := scanSource("x.go", []byte(aliased)); err != nil || len(found) != 1 {
		t.Errorf("aliased imports: found %d, err %v", len(found), err)
	}
}

// An encoder on anything else is none of this ratchet's business.
func TestScanSource_LeavesOtherWritersAlone(t *testing.T) {
	src := `package p
import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)
func f(w io.Writer) { json.NewEncoder(w).Encode(1) }
func g(w http.ResponseWriter) { var b bytes.Buffer; json.NewEncoder(&b).Encode(1); _, _ = w.Write(b.Bytes()) }
func h(w http.ResponseWriter) { func(w io.Writer) { json.NewEncoder(w).Encode(1) }(w) }
`
	found, _, err := scanSource("x.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("found %v", found)
	}
}
