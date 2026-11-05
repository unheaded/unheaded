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
	{"go.mod", `=> \.\./`},
	{"Dockerfile", `=> \\\.\\\.`},
	{"cmd/dashboard-backend/static/index.html", `:16666|>Doom<`},
	{"cmd/dashboard-backend/static/logs.html", `:16666|>Doom<`},
	{"cmd/dashboard-backend/static/compliance.html", `:16666|>Doom<`},
	{"cmd/kanban-app/static/index.html", `:16666|>Doom<`},
	{"raft/static/index.html", `:16666|>Doom<`},
	{"cmd/wiki-server/main.go", `:16666|>Doom<`},
	{"raft/DEMO_CHECKLIST.md", `# 4\. Verify\n` + "```"},
	{"raft/DEMO_CHECKLIST.md", `All 10 tests`},
	{"raft/DEMO_CHECKLIST.md", `04_integration_tests`},
	{"raft/DEMO_CHECKLIST.md", `Mistral|nvidia-smi|21K|integration test results`},
	{"config/dashboard/services.txt", `(?i)doom`},
	{".gitignore", `ci-fetch-doomgeneric|scripts/doom/`},
	{"docs/EXTERNAL_DEPENDENCIES.md", `crates/zhenai-forge`},
	{"scripts/clean-artifacts.sh", `(?i)doom|build_monad`},
	{"nixos/modules/services/README.md", `(?i)doom`},
	{"dashboard/css/style.css", `(?i)doom`},
	{"deploy/docker/zhen-agentd.Dockerfile", `as the main Dockerfile does`},
	{"deploy/sophia-eye/sophia-gateway/Dockerfile", `as the root Dockerfile does`},
	{"docs/architecture/ZHEN_ARCHITECTURE.md", `crates/zhenai-forge`},
	{"docs/architecture/service-ontology.drawio", `zhenai-forge`},
	{"wiki/Zhen-Training-Data.md", "Train only with `zhenai-forge"},
	{"wiki/Performance.md", "(?m)^Location: `cmd/doom-go-injector/main\\.go`$"},
	{"wiki/Performance.md", "documented in `docs/doom/PERFORMANCE\\.md`\\."},
	{"wiki/Performance.md", `(?m)^\*\*Usage:\*\*$`},
	{"wiki/Performance.md", `(?m)^## Key Files$`},
	{"wiki/Doom-on-UPC.md", `__umoddi3|__udivdi3`},
	{"wiki/Doom-on-UPC.md", `(?i)formal proof|computational-completeness proof|Computational-generality proof|No external state`},
	{"wiki/Doom-on-UPC.md", `(?m)^cd ~/tmp/unheaded$`},
	{"wiki/SBOM.md", `ci-fetch-doomgeneric|doomgeneric|22 Doom scripts`},
	{"wiki/Project-Structure.md", "doom-runner|doom-reference|`doom/`|`internal/`"},
	{"wiki/Rust-Components.md", `doom-runner`},
	{"wiki/Microservices.md", `doom-(?:bridge|loader|cpu-dump|go-injector|injector-c)|test_batch`},
	{"wiki/Profiling.md", `(?i)doom`},
	{"wiki/Architecture.md", "default 16, `crates/doom-runner`\\)"},
	{"wiki/eBPF-Programs.md", "by `demos/doom/Makefile` into"},
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
	"wiki/Performance.md",
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

// moveoutDeadLinks gives every repository link in a page under wiki/ that opens
// nothing below root, the repository root.
func moveoutDeadLinks(root, text string) []string {
	link := regexp.MustCompile(`\]\((\.\./[^)#\s]+)`)
	var dead []string
	for _, m := range link.FindAllStringSubmatch(text, -1) {
		if _, err := os.Stat(filepath.Join(root, "wiki", filepath.FromSlash(m[1]))); err != nil {
			dead = append(dead, m[1])
		}
	}
	return dead
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
		if dead := moveoutDeadLinks(filepath.Join("..", ".."), text); len(dead) > 0 {
			t.Errorf("%s: links open nothing here: %v", rel, dead)
		}
	}
}

func TestDeadLinkCheckFindsAnyMissingTarget(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"wiki", "cmd/kept"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	text := "[a](../cmd/kept/) [b](../cmd/doom-bridge/) [c](../deploy/k8s/#x) " +
		"[d](../scripts/doom-xdp-attach.sh) [e](https://example.org/x) [f](Home)"
	got := strings.Join(moveoutDeadLinks(root, text), " ")
	if want := "../cmd/doom-bridge/ ../deploy/k8s/ ../scripts/doom-xdp-attach.sh"; got != want {
		t.Fatalf("dead links = %q, want %q", got, want)
	}
}

// Doom is evidence for the generality hypothesis, not a proof of it: the
// machine is finite and the wording has to stay honest on every page.
func TestWikiMakesNoCompletenessProofClaim(t *testing.T) {
	claim := regexp.MustCompile(`(?i)computational[- ]completeness proof|proof of computational completeness|completeness proven`)
	pages, err := filepath.Glob(filepath.Join("..", "..", "wiki", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("no wiki pages found")
	}
	for _, page := range pages {
		rel := filepath.ToSlash(filepath.Join("wiki", filepath.Base(page)))
		if lines := moveoutHitLines(moveoutRepoFile(t, rel), claim); len(lines) > 0 {
			t.Errorf("%s: claims a completeness proof at lines %v", rel, lines)
		}
	}
}

// Every path a Code-table row or crate list names has to be in this checkout.
func TestWikiStructurePagesNameWhatIsHere(t *testing.T) {
	root := filepath.Join("..", "..")
	structure := moveoutRepoFile(t, "wiki/Project-Structure.md")
	code := regexp.MustCompile(`(?s)## Code\n(.*?)\n## `).FindStringSubmatch(structure)
	if code == nil {
		t.Fatal("wiki/Project-Structure.md: no Code section")
	}
	dirs := regexp.MustCompile("(?m)^\\| `([a-z]+(?:/[a-z]+)*)/`").FindAllStringSubmatch(code[1], -1)
	if len(dirs) == 0 {
		t.Fatal("wiki/Project-Structure.md: no directories found")
	}
	for _, m := range dirs {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(m[1]))); err != nil {
			t.Errorf("wiki/Project-Structure.md lists %s/, which is not here", m[1])
		}
	}
	list := regexp.MustCompile("(?m)^\\| `crates/` \\| Other Rust crates: (.*) \\|$").FindStringSubmatch(structure)
	if list == nil {
		t.Fatal("wiki/Project-Structure.md: no crate list")
	}
	crates := strings.Split(list[1], ", ")
	section := regexp.MustCompile("(?s)## `crates/`\\n(.*?)\\n## ").FindStringSubmatch(moveoutRepoFile(t, "wiki/Rust-Components.md"))
	if section == nil {
		t.Fatal("wiki/Rust-Components.md: no crates section")
	}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z0-9-]+)` \\|").FindAllStringSubmatch(section[1], -1) {
		crates = append(crates, m[1])
	}
	for _, name := range crates {
		if _, err := os.Stat(filepath.Join(root, "crates", name)); err != nil {
			t.Errorf("wiki names crate %s, which is not in crates/", name)
		}
	}
}

// demoChecklistFaults names each check in the checklist that a healthy Zhen
// stack cannot pass. It compares the checklist with what start-zhen.sh brings
// up and what zhen_app.py and its page answer, so a stale port, route or stats
// field fails here rather than five minutes before a demo.
func demoChecklistFaults(checklist, app, start, page string) []string {
	var faults []string
	host := regexp.MustCompile(`(?:localhost|127\.0\.0\.1):(\d+)`)
	ports := map[string]bool{}
	for _, m := range host.FindAllStringSubmatch(start, -1) {
		ports[m[1]] = true
	}
	for _, m := range host.FindAllStringSubmatch(checklist, -1) {
		if !ports[m[1]] {
			faults = append(faults, "port "+m[1]+" is not one start-zhen.sh uses")
		}
	}
	for _, m := range regexp.MustCompile(`localhost:20103(/[A-Za-z0-9_/-]*)`).FindAllStringSubmatch(checklist, -1) {
		if !strings.Contains(app, "@app.route('"+m[1]+"'") {
			faults = append(faults, "route "+m[1]+" is not in zhen_app.py")
		}
	}
	body := app
	if i := strings.Index(body, "def stats():"); i >= 0 {
		body = body[i:]
		if j := strings.Index(body, "@app.route"); j >= 0 {
			body = body[:j]
		}
	}
	claimed := 0
	for _, line := range regexp.MustCompile("(?m)^.*api/v1/stats` shows (.*)$").FindAllStringSubmatch(checklist, -1) {
		for _, m := range regexp.MustCompile("`([a-z_]+)").FindAllStringSubmatch(line[1], -1) {
			claimed++
			if !strings.Contains(body, "'"+m[1]+"':") {
				faults = append(faults, "stats field "+m[1]+" is not returned by /api/v1/stats")
			}
		}
	}
	if claimed == 0 {
		faults = append(faults, "no stats field is checked")
	}
	for _, m := range regexp.MustCompile(`returns HTML containing "([^"]+)"`).FindAllStringSubmatch(checklist, -1) {
		if !strings.Contains(page, m[1]) {
			faults = append(faults, "the page does not contain "+m[1])
		}
	}
	return faults
}

func TestDemoChecklistChecksWhatTheStackServes(t *testing.T) {
	faults := demoChecklistFaults(moveoutRepoFile(t, "raft/DEMO_CHECKLIST.md"), moveoutRepoFile(t, "raft/zhen_app.py"),
		moveoutRepoFile(t, "raft/start-zhen.sh"), moveoutRepoFile(t, "raft/static/index.html"))
	for _, fault := range faults {
		t.Error(fault)
	}
}

func TestDemoChecklistFaultsFindPlantedChecks(t *testing.T) {
	app := "@app.route('/health')\n@app.route('/api/v1/stats')\ndef stats():\n    return {'backend': 1}\n@app.route('/x')\n"
	start := "curl http://127.0.0.1:8081/health\ncurl http://localhost:20103/health\n"
	good := "`curl http://localhost:8081/v1/models`\n`curl http://localhost:20103/api/v1/stats` shows `backend`\n"
	if got := demoChecklistFaults(good, app, start, "Zhenai"); len(got) != 0 {
		t.Fatalf("clean checklist faults = %v", got)
	}
	for _, planted := range []string{
		"curl http://localhost:20100/v1/models\n",
		"curl http://localhost:20103/nope\n",
		"`curl http://localhost:20103/api/v1/stats` shows `index_vectors > 20000`\n",
		"returns HTML containing \"ZHEN\"\n",
	} {
		if got := demoChecklistFaults(good+planted, app, start, "Zhenai"); len(got) != 1 {
			t.Errorf("planted %q: faults = %v, want one", planted, got)
		}
	}
	if got := demoChecklistFaults("no stats here\n", app, start, "Zhenai"); len(got) != 1 {
		t.Errorf("checklist without a stats check: faults = %v, want one", got)
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

// moveoutScriptRef matches a repository script however a runbook spells it:
// bare, ./-relative, through $REPO or the checkout path, with or without an
// extension. A path under another directory (raft/scripts/) is not one.
var moveoutScriptRef = regexp.MustCompile("(?:^|[\\s\"'=(`]|\\./|\\$\\{?REPO[A-Z_]*\\}?/|(?:~|\\$\\{?HOME\\}?)/tmp/unheaded/)" +
	`(scripts/[A-Za-z0-9_./-]*[A-Za-z0-9_-])`)

// moveoutMissingRunbookScripts walks root/runbooks and gives every script
// reference that opens nothing below root, with how many references it saw.
func moveoutMissingRunbookScripts(root string) (missing []string, seen int, err error) {
	err = filepath.WalkDir(filepath.Join(root, "runbooks"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range moveoutScriptRef.FindAllSubmatch(b, -1) {
			seen++
			if _, err := os.Stat(filepath.Join(root, string(m[1]))); err != nil {
				missing = append(missing, filepath.Base(path)+": "+string(m[1]))
			}
		}
		return nil
	})
	return missing, seen, err
}

// A runbook step that names a repository script has to name one that exists:
// an operator following the step cannot tell a moved file from a typo.
func TestRunbooksNameOnlyScriptsThatExist(t *testing.T) {
	missing, seen, err := moveoutMissingRunbookScripts(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range missing {
		t.Errorf("runbook names a script that does not exist: %s", ref)
	}
	if seen == 0 {
		t.Fatal("no script references found: the pattern no longer matches any runbook")
	}
}

func TestRunbookScriptCheckCoversEverySpelling(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"runbooks", "scripts/lib"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "lib", "kept.sh"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	plant := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "runbooks", name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plant("kept.yaml", `run: scripts/lib/kept.sh; "${REPO}/scripts/lib/kept.sh"; raft/scripts/other.sh; scripts/`+"\n")
	if missing, seen, err := moveoutMissingRunbookScripts(root); err != nil || len(missing) != 0 || seen != 2 {
		t.Fatalf("clean runbook: missing %v, seen %d, err %v; want none missing, 2 seen", missing, seen, err)
	}
	for _, c := range []struct{ name, text, want string }{
		{"a.yaml", "bash scripts/nope1.sh\n", "scripts/nope1.sh"},
		{"b.yaml", "./scripts/nope2.py\n", "scripts/nope2.py"},
		{"c.yaml", "\"${REPO}/scripts/nope3.sh\"\n", "scripts/nope3.sh"},
		{"d.yaml", "$REPO_ROOT/scripts/nope4.sh\n", "scripts/nope4.sh"},
		{"e.yaml", "~/tmp/unheaded/scripts/nope5.sh\n", "scripts/nope5.sh"},
		{"f.yaml", "$HOME/tmp/unheaded/scripts/nope6.sh\n", "scripts/nope6.sh"},
		{"g.yaml", "run scripts/nope7 now\n", "scripts/nope7"},
		{"h.yaml", "see `scripts/nope8.sh`.\n", "scripts/nope8.sh"},
		{"i.yml", "bash scripts/nope9.sh\n", "scripts/nope9.sh"},
	} {
		plant(c.name, c.text)
		missing, _, err := moveoutMissingRunbookScripts(root)
		if err != nil {
			t.Fatal(err)
		}
		if want := c.name + ": " + c.want; len(missing) != 1 || missing[0] != want {
			t.Errorf("%q: missing = %v, want %q", c.text, missing, want)
		}
		if err := os.Remove(filepath.Join(root, "runbooks", c.name)); err != nil {
			t.Fatal(err)
		}
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
