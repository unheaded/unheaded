// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package scriptlib

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// moveoutRepoFile reads a tracked file by its path from the repository root.
func moveoutRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// moveoutHitLines gives the 1-based line of every match of re in text.
func moveoutHitLines(text string, re *regexp.Regexp) []int {
	var lines []int
	for _, loc := range re.FindAllStringIndex(text, -1) {
		lines = append(lines, 1+strings.Count(text[:loc[0]], "\n"))
	}
	return lines
}

// moveoutGone lists living files that must stop describing what left this
// repository for unheaded-doom, unheaded-k8s and unheaded-forge. Dated
// evidence, old plans and ADR history are not listed: they stay as written.
var moveoutGone = []struct {
	file      string
	forbidden string
}{
	{"nix/containers/gateway.nix", `(?i)doom`},
	{"raft/DEMO_CHECKLIST.md", `# 4\. Verify\n` + "```"},
	{"raft/DEMO_CHECKLIST.md", `All 10 tests`},
	{"raft/DEMO_CHECKLIST.md", `04_integration_tests`},
	{"scripts/clean-artifacts.sh", `(?i)doom|build_monad`},
	{"nixos/modules/services/README.md", `(?i)doom`},
	{"dashboard/css/style.css", `(?i)doom`},
	{"deploy/docker/zhen-agentd.Dockerfile", `as the main Dockerfile does`},
	{"deploy/sophia-eye/sophia-gateway/Dockerfile", `as the root Dockerfile does`},
	{"wiki/Doom-on-UPC.md", `__umoddi3|__udivdi3`},
	{"wiki/Doom-on-UPC.md", `(?i)formal proof|computational-completeness proof|Computational-generality proof|No external state`},
}

func TestMoveOutResidueStaysGone(t *testing.T) {
	for _, row := range moveoutGone {
		t.Run(row.file+"/"+row.forbidden, func(t *testing.T) {
			re := regexp.MustCompile(row.forbidden)
			if lines := moveoutHitLines(moveoutRepoFile(t, row.file), re); len(lines) > 0 {
				t.Errorf("%s still matches %q at lines %v", row.file, row.forbidden, lines)
			}
		})
	}
}

// moveoutBanner lists wiki pages that describe moved code and must say so.
var moveoutBanner = []string{
	"wiki/Doom-on-UPC.md",
	"wiki/Doom-over-IPv6.md",
	"wiki/UPC-Dream-Ladder.md",
	"wiki/UPC-Overview.md",
}

func TestMovedPagesCarryABanner(t *testing.T) {
	banner := regexp.MustCompile(`(?m)^> \*\*Moved \(DP-305\)\.\*\*.*unheaded-doom.*DOOM-LESSONS\.md`)
	for _, file := range moveoutBanner {
		if !banner.MatchString(moveoutRepoFile(t, file)) {
			t.Errorf("%s has no moved banner pointing at Doom's lessons", file)
		}
	}
}

// A wiki link has to say what it opens, and has to open something here.
func TestWikiLinksMatchWhatTheyOpen(t *testing.T) {
	label := regexp.MustCompile("\\[`?docs/doom[^\\]]*\\]\\(")
	target := regexp.MustCompile(`\]\(\.\./(?:cmd/doom-go-injector|crates/doom-runner|crates/zhenai-forge|demos/doom|docs/doom)\b`)
	pages, err := filepath.Glob(filepath.Join("..", "..", "wiki", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("no wiki pages found")
	}
	for _, page := range pages {
		rel := filepath.ToSlash(filepath.Join("wiki", filepath.Base(page)))
		text := moveoutRepoFile(t, rel)
		if lines := moveoutHitLines(text, label); len(lines) > 0 {
			t.Errorf("%s: link label names docs/doom at lines %v, which opens another page", rel, lines)
		}
		if lines := moveoutHitLines(text, target); len(lines) > 0 {
			t.Errorf("%s: link into a moved path at lines %v", rel, lines)
		}
	}
}

// The matcher has to find a planted violation, or every row above passes
// for nothing.
func TestMoveOutMatcherFindsPlantedResidue(t *testing.T) {
	re := regexp.MustCompile(`(?i)doom`)
	if got := moveoutHitLines("clean\nDoom here\nclean\ndoom\n", re); len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Fatalf("planted hits = %v, want lines 2 and 4", got)
	}
	if got := moveoutHitLines("clean\n", re); len(got) != 0 {
		t.Fatalf("clean text hits = %v, want none", got)
	}
}

// A runbook step that names a repository script has to name one that exists:
// an operator following the step cannot tell a moved file from a typo.
func TestRunbooksNameOnlyScriptsThatExist(t *testing.T) {
	root := filepath.Join("..", "..")
	scriptRef := regexp.MustCompile(`(?:^|[\s"'=(])(?:\./)?(scripts/[A-Za-z0-9_./-]+\.(?:sh|py))\b`)
	seen := 0
	err := filepath.WalkDir(filepath.Join(root, "runbooks"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".yaml" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range scriptRef.FindAllSubmatch(b, -1) {
			seen++
			if _, err := os.Stat(filepath.Join(root, string(m[1]))); err != nil {
				t.Errorf("%s names %s, which does not exist", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("no script references found: the pattern no longer matches any runbook")
	}
}

// The health monitor divides by totalServices, so the count has to follow the
// dependency list.
func TestGatewayServiceCountMatchesDependencies(t *testing.T) {
	text := moveoutRepoFile(t, "nix/containers/gateway.nix")
	block := regexp.MustCompile(`(?s)dependencies = \[(.*?)\n\s*\];`).FindStringSubmatch(text)
	total := regexp.MustCompile(`totalServices = (\d+);`).FindStringSubmatch(text)
	if block == nil || total == nil {
		t.Fatal("gateway.nix: dependencies or totalServices not found")
	}
	want, err := strconv.Atoi(total[1])
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(block[1], "{ name = "); got != want {
		t.Errorf("gateway.nix lists %d dependencies, totalServices = %d", got, want)
	}
}
