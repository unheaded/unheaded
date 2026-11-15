// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/yuin/goldmark/text"
)

// renderSheet renders src as the sheet shell/<x> would be, with known as
// the listing.
func renderSheet(t *testing.T, src string, known func(category, name string) bool) string {
	t.Helper()
	ws, err := NewWikiServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(src)
	doc := ws.md.Parser().Parse(text.NewReader(data))
	rewriteSheetLinks(doc, "shell", known)
	var buf bytes.Buffer
	if err := ws.md.Renderer().Render(&buf, data, doc); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func knownSheets(category, name string) bool {
	switch category + "/" + name {
	case "shell/tmux", "shell/bash", "editors/neovim", "networking/tcp", "shell/a b":
		return true
	}
	return false
}

func TestSheetLinks(t *testing.T) {
	t.Parallel()
	const src = "# Sheet\n\n" +
		"[t](tmux.md) [d](./tmux.md) [bare](tmux) [n](../editors/neovim.md) [q](networking/tcp) [qm](networking/tcp.md)\n" +
		"[f](tmux.md#panes) [nf](../editors/neovim.md#lsp \"titled\") [sp](a%20b.md) [ref][r]\n\n" +
		"[gone](nosuch.md) [gone2](../editors/nosuch.md) [gone3](nosuch/tcp) [gone4](notes.txt) **[bold](nosuch.md)**\n\n" +
		"[web](https://example.com/tmux.md) [anchor](#loops) [abs](/abs/tmux.md) [out](../../x.md) [deep](a/b/c.md)\n" +
		"[query](tmux.md?x=1) [mail](mailto:a@example.com) [proto](//example.com/x) ![img](tmux.md)\n\n" +
		"```bash\nif [[ $x == y ]]; then echo \"[t](tmux.md)\"; fi\n```\n\n" +
		"Inline `[[ -f x ]]` and `[t](tmux.md)`.\n\n" +
		"[r]: ../editors/neovim.md\n"
	out := renderSheet(t, src, knownSheets)

	for _, want := range []string{
		// The sheet's own directory, the directory beside it, and a path
		// from the top: all three forms are in vor's sheets.
		`<a href="/sheet/shell/tmux">t</a>`,
		`<a href="/sheet/shell/tmux">d</a>`,
		`<a href="/sheet/shell/tmux">bare</a>`,
		`<a href="/sheet/editors/neovim">n</a>`,
		`<a href="/sheet/networking/tcp">q</a>`,
		`<a href="/sheet/networking/tcp">qm</a>`,
		`<a href="/sheet/shell/tmux#panes">f</a>`,
		`<a href="/sheet/editors/neovim#lsp" title="titled">nf</a>`,
		`<a href="/sheet/shell/a%20b">sp</a>`,
		`<a href="/sheet/editors/neovim">ref</a>`,
		// A sheet vor does not have: the text stays, the link goes.
		`<p>gone gone2 gone3 gone4 <strong>bold</strong></p>`,
		// Not links between sheets: left as written.
		`<a href="https://example.com/tmux.md">web</a>`,
		`<a href="#loops">anchor</a>`,
		`<a href="/abs/tmux.md">abs</a>`,
		`<a href="../../x.md">out</a>`,
		`<a href="a/b/c.md">deep</a>`,
		`<a href="tmux.md?x=1">query</a>`,
		`<a href="mailto:a@example.com">mail</a>`,
		`<a href="//example.com/x">proto</a>`,
		`<img src="tmux.md" alt="img">`,
		// Code is not a link, and [[ ]] is shell, not a wiki link.
		"if [[ $x == y ]]; then echo &quot;[t](tmux.md)&quot;; fi",
		"<code>[[ -f x ]]</code>",
		"<code>[t](tmux.md)</code>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered sheet lacks %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/wiki/") || strings.Contains(out, "github.com") {
		t.Errorf("a sheet link was sent to the wiki or to GitHub:\n%s", out)
	}
}

// TestSheetLinks_NoListing: without the listing nothing is known, so every
// link is left as written rather than removed.
func TestSheetLinks_NoListing(t *testing.T) {
	t.Parallel()
	const src = "[t](tmux.md) [gone](nosuch.md) [n](../editors/neovim.md)\n"
	out := renderSheet(t, src, nil)
	for _, want := range []string{`<a href="tmux.md">t</a>`, `<a href="nosuch.md">gone</a>`, `<a href="../editors/neovim.md">n</a>`} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered sheet lacks %s:\n%s", want, out)
		}
	}
}

// TestSheetLinks_OddCategory: a sheet in a category no URL can name gets no
// rewriting at all.
func TestSheetLinks_OddCategory(t *testing.T) {
	t.Parallel()
	ws, err := NewWikiServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("[t](tmux.md) [gone](nosuch.md)\n")
	for _, category := range []string{"a/b", "", "..", "."} {
		doc := ws.md.Parser().Parse(text.NewReader(data))
		rewriteSheetLinks(doc, category, func(string, string) bool { return true })
		var buf bytes.Buffer
		if err := ws.md.Renderer().Render(&buf, data, doc); err != nil {
			t.Fatal(err)
		}
		if got := buf.String(); !strings.Contains(got, `<a href="tmux.md">t</a>`) || !strings.Contains(got, `<a href="nosuch.md">gone</a>`) {
			t.Errorf("category %q: links were rewritten:\n%s", category, got)
		}
	}
}

// TestSheetLinks_OnThePage: the served sheet has its links rewritten against
// vor's listing, and falls back to the text as written when vor's listing
// is not to be had.
func TestSheetLinks_OnThePage(t *testing.T) {
	h, vor := searchFixture(t)
	w := get(t, h, "/sheet/networking/tcp")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /sheet/networking/tcp = %d", w.Code)
	}
	content := mainOf(t, w.Body.String())
	for _, want := range []string{
		`<a href="/sheet/networking/arp">arp</a>`,
		`<a href="/sheet/shell/bash#loops">bash loops</a>`,
		`<a href="/sheet/auth/saml">saml</a>`,
		// udp is not a sheet here.
		`and udp.`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("sheet page lacks %s:\n%s", want, content)
		}
	}
	if strings.Contains(content, "udp.md") {
		t.Errorf("a link to a sheet vor does not have is still a link:\n%s", content)
	}
	wantInert(t, w)

	// One listing serves every sheet view.
	if n := vor.count("/api/topics"); n != 1 {
		t.Errorf("the listing was fetched %d times, want 1", n)
	}
	if w := get(t, h, "/sheet/shell/bash"); w.Code != http.StatusOK {
		t.Fatalf("GET /sheet/shell/bash = %d", w.Code)
	}
	if n := vor.count("/api/topics"); n != 1 {
		t.Errorf("the listing was fetched %d times after a second sheet, want 1", n)
	}
}
