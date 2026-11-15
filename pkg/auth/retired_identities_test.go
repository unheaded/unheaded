// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// retiredIdentities are the role services removed in 2026-11 (ADR-147
// decision 7). No file that admits a caller, issues a certificate, answers
// a name, scrapes, checks or subscribes may name them again: a trusted
// identity nobody runs is a key anyone who finds it can use.
var retiredIdentities = map[string]bool{
	"monad": true, "sophia": true, "captain": true, "micromanager": true, "architect": true,
}

// identityFiles are the files (globs, from the repository root) that grant
// or use a service identity. Comments are not read: "the Monad wire format"
// is not an identity.
var identityFiles = []string{
	"deploy/systemd/*.service",
	"docker-compose.yml",
	"docker/*.yml",
	"docker/*/*.yml",
	"monitoring/docker-compose.yml",
	"monitoring/prometheus/prometheus.yml",
	"config/victoria/scrape.yml",
	"config/coredns/Corefile",
	"config/akira/akira.yaml",
	"configs/akira-*.yaml",
	"configs/wotan.yaml",
	"configs/services.yaml",
	"services.yaml",
	"cmd/cert-gen/main.go",
	"pkg/transport/mtls/init.go",
	"cmd/akira/main.go",
	".github/workflows/*.yml",
}

// stripComment drops a line's comment: # for YAML, units, Corefile and
// workflows, // for Go.
func stripComment(path, line string) string {
	marker := "#"
	if strings.HasSuffix(path, ".go") {
		marker = "//"
	}
	if i := strings.Index(line, marker); i >= 0 {
		return line[:i]
	}
	return line
}

// identityTokens splits a line into name tokens: letters, digits, '-' and
// '_' ("sophia-eye" and "monad-mbc" are one token each and not retired).
func identityTokens(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_'
	})
}

func TestRetiredIdentitiesStayOut(t *testing.T) {
	const root = "../.."
	checked := 0
	for _, g := range identityFiles {
		paths, err := filepath.Glob(filepath.Join(root, g))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range paths {
			data, err := os.ReadFile(p) // #nosec G304 -- repository files under test
			if err != nil {
				t.Fatal(err)
			}
			checked++
			rel, _ := filepath.Rel(root, p)
			for i, line := range strings.Split(string(data), "\n") {
				for _, tok := range identityTokens(stripComment(p, line)) {
					if retiredIdentities[tok] {
						t.Errorf("%s:%d names the retired identity %q: %s", rel, i+1, tok, strings.TrimSpace(line))
					}
				}
			}
		}
	}
	if checked < 20 {
		t.Fatalf("checked %d files: the globs no longer fit the tree, so this test checks little", checked)
	}
}

func TestIdentityTokens(t *testing.T) {
	got := identityTokens(`Environment=AUTH_ALLOW_CALLERS=kanban-app,micromanager sophia-eye captain.unheaded.local {Name: "monad", IP}`)
	want := []string{"Environment", "AUTH_ALLOW_CALLERS", "kanban-app", "micromanager", "sophia-eye", "captain", "unheaded", "local", "Name", "monad", "IP"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("tokens %q\nwant   %q", got, want)
	}
	if stripComment("x.yml", `a: b # captain`) != "a: b " || stripComment("x.go", `x // captain`) != "x " {
		t.Fatal("stripComment")
	}
}
