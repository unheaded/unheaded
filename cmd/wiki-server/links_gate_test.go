// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"html"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"unheaded/pkg/repolink"
)

// linkAttrRe finds the address of every link and image goldmark wrote.
var linkAttrRe = regexp.MustCompile(`<(?:a [^>]*?href|img [^>]*?src)="([^"]*)"`)

// deadWikiLinks renders every Markdown file of wikiDir the way the server
// does and returns one line for each link that leads nowhere:
//
//   - a link to /wiki/<x> must name a page on disk;
//   - a link into the repository (GitHub blob address) must name a path in
//     the checkout at repoRoot;
//   - any other address on this server is dead, whether it was written
//     relative or rooted: the server has no route for it. That covers an
//     image (the server serves no files) and a bare "..".
//
// Addresses on other hosts, mail addresses and in-page anchors are not
// checked.
func deadWikiLinks(t *testing.T, wikiDir, repoRoot string) []string {
	t.Helper()
	ws, err := NewWikiServer(wikiDir)
	if err != nil {
		t.Fatal(err)
	}
	pages := ws.pageSlugs()
	entries, err := os.ReadDir(wikiDir)
	if err != nil {
		t.Fatal(err)
	}

	var dead []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		_, out, err := ws.renderPage(slug)
		if err != nil {
			dead = append(dead, e.Name()+": does not render: "+err.Error())
			continue
		}
		base := &url.URL{Path: "/wiki/" + slug}
		for _, m := range linkAttrRe.FindAllStringSubmatch(out, -1) {
			raw := html.UnescapeString(m[1])
			if why := deadLink(base, raw, pages, repoRoot); why != "" {
				dead = append(dead, e.Name()+": "+raw+": "+why)
			}
		}
	}
	sort.Strings(dead)
	return dead
}

// deadLink returns why the address raw, on the page at base, leads
// nowhere, or "" when it leads somewhere or is not checked.
func deadLink(base *url.URL, raw string, pages map[string]bool, repoRoot string) string {
	if strings.HasPrefix(raw, repolink.BlobBase) {
		u, err := url.Parse(strings.TrimPrefix(raw, repolink.BlobBase))
		if err != nil {
			return "not a URL"
		}
		p := path.Clean("/" + u.Path)
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(p))); err != nil {
			return "no such path in the repository"
		}
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "not a URL"
	}
	if u.Scheme != "" || u.Host != "" {
		return "" // another host, or mailto:
	}
	if u.Path == "" {
		return "" // an anchor in the page itself
	}
	at := base.ResolveReference(u).Path
	if slug, ok := strings.CutPrefix(at, "/wiki/"); ok {
		slug = strings.TrimSuffix(slug, "/")
		if slug == "" || pages[slug] {
			return ""
		}
		return "no such wiki page"
	}
	switch {
	case at == searchPath, at == sheetsPath, strings.HasPrefix(at, sheetPath):
		return "" // vor's pages; what vor holds is not this test's to say
	}
	return "this server has no page at " + at
}

// TestEveryWikiLinkResolves is the standing gate on wiki/: no page links to
// something that 404s. 136 "Page.md" links did until 2026-11-12.
//
// A dead link is fixed in the page. A dated record is never edited: it gets
// a dated note, and its link goes in allowed with the reason.
func TestEveryWikiLinkResolves(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	wikiDir := filepath.Join(repoRoot, "wiki")
	if _, err := os.Stat(filepath.Join(wikiDir, "README.md")); err != nil {
		t.Skipf("no wiki directory beside this package: %v", err)
	}

	// "file.md: address" → why it is allowed to be dead.
	allowed := map[string]string{}

	dead := deadWikiLinks(t, wikiDir, repoRoot)
	used := map[string]bool{}
	n := 0
	for _, d := range dead {
		key := d
		if i := strings.LastIndex(d, ": "); i > 0 {
			key = d[:i]
		}
		if _, ok := allowed[key]; ok {
			used[key] = true
			continue
		}
		n++
		t.Errorf("dead link: %s", d)
	}
	for key := range allowed {
		if !used[key] {
			t.Errorf("allowed entry %q matches no dead link: remove it", key)
		}
	}
	if n > 0 {
		t.Errorf("%d dead link(s) in wiki/", n)
	}
}

// TestDeadWikiLinks_Finds: the gate sees each kind of dead link, in a wiki
// made for the test. wiki/ has no bare ".." link and no image today; the
// first one written must show up.
func TestDeadWikiLinks_Finds(t *testing.T) {
	repo := t.TempDir()
	wiki := filepath.Join(repo, "wiki")
	for name, body := range map[string]string{
		"wiki/README.md": "# Home\n\n[ok](Good.md) [slug](Good) [wikilink]: [[Good]] [anchor](#home) [web](https://example.com/x)\n" +
			"[repo](../docs/here.md) [dir](../docs) [sheets](/sheets) [mail](mailto:a@example.com)\n",
		"wiki/Good.md": "# Good\n\n[home](README.md) [root](/wiki/)\n",
		"wiki/Bad.md": "# Bad\n\n[up](..) [nopage](Nope.md) [noslug](Nope) [[Missing]] ![pic](pic.png)\n" +
			"[gone](../docs/gone.md) [sub](sub/Page.md) [rooted](/nowhere) [out](../../etc/passwd)\n",
		"wiki/_Sidebar.md": "- [nope](Nope.md)\n",
		"docs/here.md":     "# Here\n",
	} {
		p := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got := deadWikiLinks(t, wiki, repo)
	want := []string{
		"Bad.md: ..: this server has no page at /",
		"Bad.md: ../../etc/passwd: this server has no page at /etc/passwd",
		"Bad.md: /nowhere: this server has no page at /nowhere",
		"Bad.md: /wiki/Missing: no such wiki page",
		"Bad.md: Nope.md: no such wiki page",
		"Bad.md: Nope: no such wiki page",
		"Bad.md: " + repolink.BlobBase + "docs/gone.md: no such path in the repository",
		"Bad.md: pic.png: no such wiki page",
		"Bad.md: sub/Page.md: no such wiki page",
		"_Sidebar.md: Nope.md: no such wiki page",
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("dead links found:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
