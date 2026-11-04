// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package codeindex

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func clip(s string) string {
	if len(s) <= MaxChunkBytes {
		return s
	}
	s = s[:MaxChunkBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// ChunkGo chunks one Go file with go/parser and returns its name-matched calls.
func ChunkGo(path string, src []byte, blob string) ([]Chunk, []Call, error) {
	for _, line := range bytes.Split(src, []byte("\n")) {
		t := bytes.TrimSpace(line)
		if len(t) == 0 {
			continue
		}
		if bytes.HasPrefix(t, []byte("// Code generated")) {
			return nil, nil, nil
		}
		break
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	isTest := isTestPath(path)
	var chunks []Chunk
	var calls []Call
	add := func(kind, symbol, parent string, doc *ast.CommentGroup, node ast.Node) string {
		start := fset.Position(node.Pos())
		end := fset.Position(node.End())
		from := start.Offset
		ls := start.Line
		if doc != nil {
			p := fset.Position(doc.Pos())
			from, ls = p.Offset, p.Line
		}
		id := path + "::" + symbol
		if parent != "" {
			id = path + "::" + parent + "." + symbol
		}
		chunks = append(chunks, Chunk{
			ID: id, Path: path, Language: "go", Kind: kind, Symbol: symbol, Parent: parent,
			LineStart: ls, LineEnd: end.Line, Content: clip(string(src[from:end.Offset])),
			BlobSHA: blob, IsTest: isTest,
		})
		return id
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			kind, parent := "func", ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind, parent = "method", recvName(d.Recv.List[0].Type)
			}
			id := add(kind, d.Name.Name, parent, d.Doc, d)
			if d.Body == nil {
				continue
			}
			ast.Inspect(d.Body, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch fn := ce.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				if name != "" {
					calls = append(calls, Call{Caller: id, Callee: name, Path: path, Line: fset.Position(ce.Pos()).Line})
				}
				return true
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				doc := d.Doc
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Doc != nil {
						doc = s.Doc
					}
					add("type", s.Name.Name, "", doc, s)
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					if s.Doc != nil {
						doc = s.Doc
					}
					for _, n := range s.Names {
						if n.Name == "_" {
							continue
						}
						add(kind, n.Name, "", doc, s)
					}
				}
			}
		}
	}
	return chunks, calls, nil
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	}
	return ""
}

var (
	rustRe = regexp.MustCompile(`^\s*(pub(\([a-z ]+\))?\s+)?(async\s+)?(unsafe\s+)?(fn|struct|enum|trait|impl)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	pyRe   = regexp.MustCompile(`^\s*(async\s+)?(def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

type lineMatch struct {
	line int // 0-based
	kind string
	sym  string
}

func chunkLines(path, lang string, src []byte, blob string, re *regexp.Regexp, kindGroup, symGroup int) []Chunk {
	lines := strings.Split(string(src), "\n")
	var ms []lineMatch
	for i, l := range lines {
		if m := re.FindStringSubmatch(l); m != nil {
			ms = append(ms, lineMatch{i, m[kindGroup], m[symGroup]})
		}
	}
	isTest := isTestPath(path)
	var out []Chunk
	for i, m := range ms {
		end := len(lines) // exclusive
		if i+1 < len(ms) {
			end = ms[i+1].line
		}
		if end-m.line > 200 {
			end = m.line + 200
		}
		for end > m.line+1 && strings.TrimSpace(lines[end-1]) == "" {
			end--
		}
		out = append(out, Chunk{
			ID:   path + "::" + m.sym + ":" + strconv.Itoa(m.line+1),
			Path: path, Language: lang, Kind: m.kind, Symbol: m.sym,
			LineStart: m.line + 1, LineEnd: end,
			Content: clip(strings.Join(lines[m.line:end], "\n")),
			BlobSHA: blob, IsTest: isTest,
		})
	}
	return out
}

// ChunkRust finds definitions by line match; no call graph.
func ChunkRust(path string, src []byte, blob string) []Chunk {
	return chunkLines(path, "rust", src, blob, rustRe, 5, 6)
}

// ChunkPython finds definitions by line match; no call graph.
func ChunkPython(path string, src []byte, blob string) []Chunk {
	return chunkLines(path, "python", src, blob, pyRe, 2, 3)
}
