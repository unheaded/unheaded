// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package compliance

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// This package holds no verdict over a packet: it records what the data path
// reports and scores it. Its package comment must not say otherwise, because
// that comment is what a control mapping quotes.
func TestPackageCommentClaimsNoEnforcement(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "wire_compliance.go", nil, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		t.Fatal(err)
	}
	if f.Doc == nil {
		t.Fatal("no package comment")
	}
	doc := strings.ToLower(f.Doc.Text())
	if strings.Contains(doc, "policy enforcement") || strings.Contains(doc, "ensuring") {
		t.Errorf("package comment claims enforcement:\n%s", f.Doc.Text())
	}
	if !strings.Contains(doc, "enforces nothing") {
		t.Errorf("package comment does not say it enforces nothing:\n%s", f.Doc.Text())
	}
}
