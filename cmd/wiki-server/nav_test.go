// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Doom left this repository, so the wiki's navigation must not link to it.
// This lives here, not with the move-out residue table: a path named under
// scripts/ makes the live-path inventory read wiki-server as a tool.
func TestNavigationHasNoDoomLink(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if loc := regexp.MustCompile(`:16666|>Doom<`).Find(src); loc != nil {
		t.Errorf("main.go still links to Doom: %q", loc)
	}
}

const sidebarFixture = `### [[Home]]

---

**Getting Started**
- [[Quick Start|Quick-Start]]
- [[Vision]]

**ADRs**
- [[ADR Index|ADR-Index]]
  - [[Accepted|ADR-Accepted]]
- [[Old Thing|Old-Thing]] *(legacy)*

<details><summary>All ADR pages (2)</summary>

- [[ADR-001|ADR-001-First]]
- [[ADR-002|ADR-002-Gone]]

</details>

**Empty Section**

some stray line
`

func TestParseSidebar(t *testing.T) {
	groups := parseSidebar([]byte(sidebarFixture))
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (a title with no links is dropped): %+v", len(groups), groups)
	}
	gs := groups[0]
	if gs.Title != "Getting Started" || len(gs.Blocks) != 1 || len(gs.Blocks[0].Items) != 2 {
		t.Fatalf("first group = %+v", gs)
	}
	if it := gs.Blocks[0].Items[0]; it.Title != "Quick Start" || it.Slug != "Quick-Start" || it.Sub {
		t.Errorf("[[Title|Page]] entry = %+v", it)
	}
	if it := gs.Blocks[0].Items[1]; it.Title != "Vision" || it.Slug != "Vision" {
		t.Errorf("[[Page]] entry = %+v", it)
	}

	adr := groups[1]
	if adr.Title != "ADRs" || len(adr.Blocks) != 2 {
		t.Fatalf("ADR group = %+v", adr)
	}
	plain, folded := adr.Blocks[0], adr.Blocks[1]
	if plain.Summary != "" || len(plain.Items) != 3 {
		t.Fatalf("plain block = %+v", plain)
	}
	if !plain.Items[1].Sub || plain.Items[1].Slug != "ADR-Accepted" {
		t.Errorf("indented entry = %+v", plain.Items[1])
	}
	if plain.Items[2].Note != "(legacy)" {
		t.Errorf("note = %q, want (legacy)", plain.Items[2].Note)
	}
	if folded.Summary != "All ADR pages (2)" || len(folded.Items) != 2 {
		t.Errorf("folded block = %+v", folded)
	}
	for _, g := range groups {
		for _, b := range g.Blocks {
			for _, it := range b.Items {
				if it.Slug == "Home" {
					t.Error("the Home header line became an entry; the pinned Top link already leads home")
				}
			}
		}
	}
}

// The parser knows the shapes wiki/_Sidebar.md is written in today. A link
// written in a new shape would vanish from the served sidebar without a
// word, so every [[link]] in the real file must come out as an entry.
func TestParseSidebar_RepoSidebarLosesNoLink(t *testing.T) {
	src, err := os.ReadFile("../../wiki/_Sidebar.md")
	if err != nil {
		t.Skipf("no repo sidebar: %v", err)
	}
	want := len(regexp.MustCompile(`\[\[`).FindAll(src, -1))
	if regexp.MustCompile(`(?m)^#+\s*\[\[Home\]\]`).Match(src) {
		want-- // the header line, served as the pinned Top link
	}
	got := 0
	groups := parseSidebar(src)
	for _, g := range groups {
		if g.Title == "" {
			t.Errorf("a block of %d links has no section title", len(g.Blocks[0].Items))
		}
		for _, b := range g.Blocks {
			got += len(b.Items)
		}
	}
	if got != want {
		t.Errorf("parsed %d entries from wiki/_Sidebar.md, the file has %d links", got, want)
	}
	if len(groups) < 2 {
		t.Errorf("wiki/_Sidebar.md parsed into %d sections", len(groups))
	}
}

func navTestWiki(t *testing.T, sidebar string, pages ...string) *WikiServer {
	t.Helper()
	dir := t.TempDir()
	for _, name := range pages {
		if err := os.WriteFile(dir+"/"+name+".md", []byte("# "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if sidebar != "" {
		if err := os.WriteFile(dir+"/_Sidebar.md", []byte(sidebar), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func findEntry(groups []NavGroup, slug string) (NavGroup, NavBlock, NavEntry, bool) {
	for _, g := range groups {
		for _, b := range g.Blocks {
			for _, it := range b.Items {
				if it.Slug == slug {
					return g, b, it, true
				}
			}
		}
	}
	return NavGroup{}, NavBlock{}, NavEntry{}, false
}

func TestNavGroups_FollowsTheSidebarFile(t *testing.T) {
	ws := navTestWiki(t, sidebarFixture,
		"Home", "README", "Quick-Start", "Vision", "ADR-Index", "ADR-Accepted", "Old-Thing", "ADR-001-First", "Unlisted")

	groups := ws.navGroups("ADR-001-First")
	var titles []string
	for _, g := range groups {
		titles = append(titles, g.Title)
	}
	if got, want := strings.Join(titles, "|"), "Getting Started|ADRs|"+otherPagesTitle; got != want {
		t.Fatalf("sections = %q, want %q", got, want)
	}

	_, block, it, ok := findEntry(groups, "ADR-001-First")
	if !ok || !it.Active || it.Missing {
		t.Errorf("page being shown = %+v", it)
	}
	if !block.Open {
		t.Error("the folded block holding the page being shown is closed")
	}
	if _, _, it, _ := findEntry(groups, "ADR-002-Gone"); !it.Missing || it.Active {
		t.Errorf("entry with no file behind it = %+v, want Missing", it)
	}
	if _, _, it, _ := findEntry(groups, "Vision"); it.Active || it.Missing {
		t.Errorf("another page = %+v", it)
	}

	// Pages the sidebar forgot are still reachable; the homepage, the sidebar
	// file itself and the footer are not pages.
	g, _, _, ok := findEntry(groups, "Unlisted")
	if !ok || g.Title != otherPagesTitle {
		t.Errorf("unlisted page not under %q: %+v", otherPagesTitle, groups)
	}
	if g, _, _, ok := findEntry(groups, "README"); !ok || g.Title != otherPagesTitle {
		t.Error("README is no longer the homepage here and must stay reachable")
	}
	for _, slug := range []string{"Home", "_Sidebar", "_Footer"} {
		if _, _, _, ok := findEntry(groups, slug); ok {
			t.Errorf("%s is listed as a page", slug)
		}
	}
}

func TestNavGroups_ClosedFoldWhenThePageIsElsewhere(t *testing.T) {
	ws := navTestWiki(t, sidebarFixture, "Home", "Vision", "ADR-001-First")
	_, block, _, ok := findEntry(ws.navGroups("Vision"), "ADR-001-First")
	if !ok || block.Open {
		t.Errorf("fold open = %v, want closed", block.Open)
	}
}

func TestNavGroups_NoSidebarFileIsTheFlatList(t *testing.T) {
	ws := navTestWiki(t, "", "README", "Beta", "Alpha")
	groups := ws.navGroups("Beta")
	if len(groups) != 1 || groups[0].Title != "" || len(groups[0].Blocks) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	items := groups[0].Blocks[0].Items
	if len(items) != 2 || items[0].Slug != "Alpha" || items[1].Slug != "Beta" || !items[1].Active {
		t.Errorf("flat list = %+v", items)
	}
}

// A slug in _Sidebar.md is text somebody wrote. One that points out of the
// wiki directory has no page behind it and is never made a link.
func TestNavGroups_SlugOutsideTheWikiIsNotALink(t *testing.T) {
	ws := navTestWiki(t, "**S**\n- [[Up|../secret]]\n- [[Dot|.hidden]]\n- [[Ok|Page]]\n", "Page")
	groups := ws.navGroups("Page")
	for _, slug := range []string{"../secret", ".hidden"} {
		if _, _, it, ok := findEntry(groups, slug); !ok || !it.Missing {
			t.Errorf("%q = %+v, want Missing", slug, it)
		}
	}
}

func TestRootFile_HomeThenReadme(t *testing.T) {
	if got := navTestWiki(t, "", "README").rootFile(); got != "README.md" {
		t.Errorf("without Home.md: %q", got)
	}
	if got := navTestWiki(t, "", "README", "Home").rootFile(); got != "Home.md" {
		t.Errorf("with Home.md: %q", got)
	}
}

func TestWikiPage_GroupedSidebar(t *testing.T) {
	ws := navTestWiki(t, sidebarFixture, "Home", "Quick-Start", "Vision", "ADR-Index", "ADR-001-First")
	mux := setupMux(ws)

	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		return rec.Body.String()
	}

	page := get("/wiki/ADR-001-First")
	for _, want := range []string{
		"<h2>Getting Started</h2>",
		"<h2>ADRs</h2>",
		`<details open><summary>All ADR pages (2)</summary>`,
		`<a href="/wiki/ADR-001-First" class="active">ADR-001</a>`,
		`<span class="nav-missing">ADR-002</span>`,
		`<span class="nav-missing">Old Thing <span class="nav-note">(legacy)</span></span>`,
		`<li class="sub">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Contains(page, `href="/wiki/ADR-002-Gone"`) {
		t.Error("an entry with no page behind it is a link")
	}
	if strings.Contains(page, `class="sidebar-home active"`) {
		t.Error("Top is marked active on another page")
	}

	// /wiki/ serves Home.md when the wiki has one, and marks the pinned link.
	home := get("/wiki/")
	if !strings.Contains(home, "<h1") || !strings.Contains(home, "Home") {
		t.Error("/wiki/ did not serve Home.md")
	}
	if !strings.Contains(home, `class="sidebar-home active"`) {
		t.Error("Top is not marked active on the homepage")
	}
	if !strings.Contains(home, "<details><summary>All ADR pages (2)</summary>") {
		t.Error("the fold is open on a page outside it")
	}
}

// What a sidebar line says is text. It reaches the page escaped.
func TestWikiPage_SidebarTextIsEscaped(t *testing.T) {
	ws := navTestWiki(t, "**<script>alert(1)</script>**\n- [[<img src=x onerror=alert(1)>|Page]] *<b>n</b>*\n", "Page")
	rec := httptest.NewRecorder()
	setupMux(ws).ServeHTTP(rec, httptest.NewRequest("GET", "/wiki/Page", nil))
	body := rec.Body.String()
	for _, bad := range []string{"<script>alert(1)", "<img src=x", "<b>n</b>"} {
		if strings.Contains(body, bad) {
			t.Errorf("sidebar text served as markup: %s", bad)
		}
	}
}

// The page's own links to the TLS-only pages must say https, or a click lands
// on a refused connection (found in the 2026-11-11 test sitting).
func TestNavigationLinksToTLSPagesUseHTTPS(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if loc := regexp.MustCompile(`http://[^"]*:(20000|20001|20103)`).Find(src); loc != nil {
		t.Errorf("plain-http link to a TLS-only page: %q", loc)
	}
}

// A _Sidebar.md that is a symlink to a file outside the wiki is not read: the
// sidebar falls back to the flat list and none of the outside text is served.
func TestNavGroups_SymlinkedSidebarIsNotRead(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("**Leaked**\n- [[Stolen|Page]]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws := navTestWiki(t, "", "Page")
	if err := os.Symlink(outside, filepath.Join(ws.wikiDir, sidebarFile)); err != nil {
		t.Skipf("symlink: %v", err)
	}
	groups := ws.navGroups("Page")
	if len(groups) != 1 || groups[0].Title != "" {
		t.Fatalf("groups = %+v, want the flat list", groups)
	}
	if it := groups[0].Blocks[0].Items[0]; it.Title == "Stolen" {
		t.Error("a sidebar file outside the wiki was read through a symlink")
	}
}
