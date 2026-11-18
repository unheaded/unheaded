// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package ports

import (
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Ports 19001-19005 belonged to architect, captain, micromanager and the
// monad and sophia services, removed in 2026-11 (ADR-147 decision 7). They
// are retired, not reused. pkg/auth's TestRetiredIdentitiesStayOut keeps the
// names out of the files that grant an identity; this keeps the ports and the
// names out of the files that call, start, draw or scrape a service. The
// first sweep after the removal (2026-11-17) found the Zhen app still
// health-checking all five, east's bootstrap still writing two of their
// units and the dashboard still offering them in its filters.
var retiredPort = regexp.MustCompile(`(^|[^0-9.])1900[1-5]([^0-9]|$)`)

// callerFiles (globs from the repository root) are read for a retired port.
var callerFiles = []string{
	"raft/*.py",
	"scripts/*.sh",
	"scripts/*/*.sh",
	"lxd/hosts/*/*.sh",
	"lxd/hosts/*/*.yaml",
	"containers/containerd/services/*.json",
	"dashboard/*.html",
	"dashboard/js/*.js",
	"cmd/dashboard-backend/static/*.html",
	"cmd/dashboard-backend/static/*.js",
	"cmd/dashboard-backend/services.example",
	"cmd/demo-trace-injector/main.go",
	"tomb/dark-mirror/*.yml",
	"tomb/provision.sh",
	"tomb/ansible/*.yml",
	"tomb/cerydwyn/prompts/*.txt",
	"demos/*.html",
}

// starterFiles are also read for the five names as whole words: these files
// start, order or list services by name, with no port beside it. "monad" and
// "sophia" are also the wire format and the dictionaries, so the name check
// stays off files that speak of the protocol.
var starterFiles = []string{
	"scripts/east-bootstrap.sh",
	"scripts/bare-metal/validate-cross-host.sh",
	"lxd/hosts/host-b/launch-minimal.sh",
	"lxd/hosts/host-a/static-ips.yaml",
	"containers/containerd/services/gateway.json",
	"dashboard/logs.html",
	"cmd/dashboard-backend/static/index.html",
	"cmd/dashboard-backend/static/logs.html",
	"cmd/dashboard-backend/services.example",
}

var retiredNames = map[string]bool{
	"monad": true, "sophia": true, "captain": true, "micromanager": true, "architect": true,
}

// nameTokens splits a line into name tokens: letters, digits, '-' and '_'
// ("sophia-eye" and "monad_state" are one token each and not retired).
func nameTokens(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_'
	})
}

// uncommented drops a shell or YAML comment; other files are read whole.
func uncommented(path, line string) string {
	if strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		if i := strings.Index(line, "#"); i >= 0 {
			return line[:i]
		}
	}
	return line
}

func TestRetiredPortsStayOut(t *testing.T) {
	repo := os.DirFS("../..")
	names := map[string]bool{}
	for _, f := range starterFiles {
		names[f] = true
	}
	seen := map[string]bool{}
	for _, g := range append(append([]string{}, callerFiles...), starterFiles...) {
		paths, err := fs.Glob(repo, g)
		if err != nil {
			t.Fatal(err)
		}
		for _, rel := range paths {
			if seen[rel] {
				continue
			}
			seen[rel] = true
			data, err := fs.ReadFile(repo, rel)
			if err != nil {
				t.Fatal(err)
			}
			for i, raw := range strings.Split(string(data), "\n") {
				line := uncommented(rel, raw)
				if retiredPort.MatchString(line) {
					t.Errorf("%s:%d names a retired port (19001-19005): %s", rel, i+1, strings.TrimSpace(raw))
				}
				if !names[rel] {
					continue
				}
				for _, tok := range nameTokens(line) {
					if retiredNames[tok] {
						t.Errorf("%s:%d names the removed service %q: %s", rel, i+1, tok, strings.TrimSpace(raw))
					}
				}
			}
		}
	}
	for _, f := range starterFiles {
		if !seen[f] {
			t.Errorf("%s is gone: take it off starterFiles", f)
		}
	}
	if len(seen) < 60 {
		t.Fatalf("read %d files: the globs no longer fit the tree, so this test checks little", len(seen))
	}
}

func TestRetiredPortPattern(t *testing.T) {
	for line, want := range map[string]bool{
		"'captain': 19002,":            true,
		"targets: ['localhost:19005']": true,
		"19001":                        true,
		"port 19006":                   false,
		"word 0x19001":                 true, // a hex word is not excluded: no caller file holds one
		"ts=1719001234":                false,
		"0.19003":                      false,
		"219004":                       false,
	} {
		if got := retiredPort.MatchString(line); got != want {
			t.Errorf("%q: match %v, want %v", line, got, want)
		}
	}
	if got := strings.Join(nameTokens(`for svc in wotan monad sophia-eye "captain"; monad_state`), " "); got != "for svc in wotan monad sophia-eye captain monad_state" {
		t.Fatalf("tokens %q", got)
	}
}
