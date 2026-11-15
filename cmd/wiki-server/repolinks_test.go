// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoLink(t *testing.T) {
	t.Parallel()
	const gh = "https://github.com/unheaded/unheaded/blob/develop/"
	for _, tc := range []struct {
		dest string
		want string // "" means the link is left as it is
	}{
		// Into the repository.
		{"../cmd/sword/main.go", gh + "cmd/sword/main.go"},
		{"../docs/adr/ADR-103-auth.md", gh + "docs/adr/ADR-103-auth.md"},
		{"../CLAUDE.md", gh + "CLAUDE.md"},
		{"../docs/", gh + "docs"},
		{"../VISION.md#who-it-is-for", gh + "VISION.md#who-it-is-for"},
		{"../docs/./adr//x.md", gh + "docs/adr/x.md"},
		{"./../pkg/auth", gh + "pkg/auth"},
		{"../docs/a b.md", gh + "docs/a%20b.md"},
		{"../docs/a%20b.md", gh + "docs/a%20b.md"},
		{"../docs/x.md?plain=1#L10", gh + "docs/x.md?plain=1#L10"},
		// Encoded, but still inside: the URL is built from the cleaned path.
		{"%2e%2e/docs/x.md", gh + "docs/x.md"},
		{"..%2fdocs%2fx.md", gh + "docs/x.md"},
		{"../docs/sub/%2e%2e/x.md", gh + "docs/x.md"},
		{"../docs/a?b=<script>", gh + "docs/a?b=%3Cscript%3E"},

		// Wiki pages, anchors and absolute URLs.
		{"Architecture", ""},
		{"Architecture.md", ""},
		{"./Architecture", ""},
		{"sub/../Architecture", ""},
		{"../wiki/Architecture", ""},
		{"/wiki/Architecture", ""},
		{"#section", ""},
		{"", ""},
		{"https://example.com/../x", ""},
		{"http://example.com/a", ""},
		{"//example.com/a", ""},
		{"mailto:someone@example.com", ""},
		{"javascript:alert(1)", ""},
		{"/etc/passwd", ""},

		// Out of the repository, however it is spelled.
		{"..", ""},
		{"../", ""},
		{"../..", ""},
		{"../../etc/passwd", ""},
		{"../../../../../../etc/passwd", ""},
		{"../docs/../../etc/passwd", ""},
		{"%2e%2e/%2e%2e/etc/passwd", ""},
		{"../docs/%2e%2e/%2e%2e/etc/passwd", ""},
		{"..%2f..%2fetc%2fpasswd", ""},
		{"../docs/..%2f..%2f..%2fetc", ""},
		{"%2E%2E/%2E%2E/%2E%2E/etc/shadow", ""},

		// Not a path this server will guess at.
		{`..\..\etc\passwd`, ""},
		{`../docs\..\..\..\etc`, ""},
		{"../docs/%5c..%5c..%5cetc", ""},
		{"../docs/%zz", ""},
		{"../docs/\x00x", ""},
		{"../docs/a\nb", ""},
	} {
		got, ok := repoLink(tc.dest)
		if ok != (tc.want != "") || got != tc.want {
			t.Errorf("repoLink(%q) = %q, %v; want %q", tc.dest, got, ok, tc.want)
		}
		if ok && (!strings.HasPrefix(got, gh) || strings.Contains(got, "..") || strings.Contains(got, `\`)) {
			t.Errorf("repoLink(%q) = %q: not a plain path under %s", tc.dest, got, gh)
		}
	}
}

// TestRenderPage_RepoLinks: the rewrite happens in the rendered page, for
// Markdown links only, and the file is not what changes.
func TestRenderPage_RepoLinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const src = `# Sword

See [[Architecture]], [the registry](Port-Registry), [here](#sword) and
[upstream](https://example.com/x). The ADR is [ADR-103][adr].

A path in text, ../cmd/sword/main.go, and in code, ` + "`[x](../cmd/sword/main.go)`" + `:

    [indented](../cmd/sword/indented.go)

Escapes: [one](../../etc/passwd) [two](%2e%2e/%2e%2e/etc/passwd) [three](../docs/%2e%2e/%2e%2e/etc/passwd).

---

*Source: [cmd/sword/main.go](../cmd/sword/main.go), [docs](../docs/protocol/README.md#wire "the spec")*

[adr]: ../docs/adr/ADR-103-auth.md
`
	if err := os.WriteFile(filepath.Join(dir, "Sword.md"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, out, err := ws.renderPage("Sword")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`<a href="https://github.com/unheaded/unheaded/blob/develop/cmd/sword/main.go">cmd/sword/main.go</a>`,
		`<a href="https://github.com/unheaded/unheaded/blob/develop/docs/protocol/README.md#wire" title="the spec">docs</a>`,
		`<a href="https://github.com/unheaded/unheaded/blob/develop/docs/adr/ADR-103-auth.md">ADR-103</a>`,
		`<a href="/wiki/Architecture">Architecture</a>`,
		`<a href="Port-Registry">the registry</a>`,
		`<a href="#sword">here</a>`,
		`<a href="https://example.com/x">upstream</a>`,
		// Text and code are not links, and stay as written.
		`A path in text, ../cmd/sword/main.go,`,
		`<code>[x](../cmd/sword/main.go)</code>`,
		`[indented](../cmd/sword/indented.go)`,
		// A link out of the repository is not turned into a GitHub URL.
		`<a href="../../etc/passwd">one</a>`,
		`<a href="%2e%2e/%2e%2e/etc/passwd">two</a>`,
		`<a href="../docs/%2e%2e/%2e%2e/etc/passwd">three</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page lacks %s", want)
		}
	}
	if n := strings.Count(out, "github.com"); n != 3 {
		t.Errorf("%d GitHub links in the page, want 3:\n%s", n, out)
	}
	if strings.Contains(out, "github.com/unheaded/unheaded/blob/develop/etc") || strings.Contains(out, "develop/..") {
		t.Errorf("a link out of the repository reached GitHub:\n%s", out)
	}

	after, err := os.ReadFile(filepath.Join(dir, "Sword.md"))
	if err != nil || string(after) != src {
		t.Errorf("the Markdown file changed (err %v)", err)
	}
}

// TestRenderPage_PageDotMdLinks: a link written "Page.md" (what GitHub's own
// view of wiki/ needs) opens the page here. Served as written it would ask
// for Page.md.md and 404.
func TestRenderPage_PageDotMdLinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const src = `# Links

[a](Architecture.md) [b](Architecture.md#wire) [c](Port%20Registry.md) [home](README.md)
[missing](Nope.md) [repo](../docs/a.md) [slug](Architecture) [abs](https://example.com/x.md)
[sub](sub/Page.md) [up](../Architecture.md) [furniture](_Sidebar.md) [query](Architecture.md?x=1)
[rooted](/Architecture.md) [dots](..%2FArchitecture.md) [notmd](Architecture.mdx) [ref][r]

Code is not a link: ` + "`[x](Architecture.md)`" + `

[r]: Architecture.md "titled"
`
	for name, body := range map[string]string{
		"Links.md":         src,
		"Architecture.md":  "# Architecture\n",
		"Port Registry.md": "# Ports\n",
		"README.md":        "# Home\n",
		"_Sidebar.md":      "- x\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "Page.md"), []byte("# Sub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, out, err := ws.renderPage("Links")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<a href="/wiki/Architecture">a</a>`,
		`<a href="/wiki/Architecture#wire">b</a>`,
		`<a href="/wiki/Port%20Registry">c</a>`,
		`<a href="/wiki/README">home</a>`,
		`<a href="/wiki/Architecture" title="titled">ref</a>`,
		// No such page: left as written.
		`<a href="Nope.md">missing</a>`,
		// The repository link still goes to GitHub; a slug and an absolute
		// URL are untouched.
		`<a href="https://github.com/unheaded/unheaded/blob/develop/docs/a.md">repo</a>`,
		`<a href="Architecture">slug</a>`,
		`<a href="https://example.com/x.md">abs</a>`,
		// Only a page of this directory: no subdirectory, no furniture, no
		// query, no rooted path, nothing that is not ".md".
		`<a href="sub/Page.md">sub</a>`,
		`<a href="_Sidebar.md">furniture</a>`,
		`<a href="Architecture.md?x=1">query</a>`,
		`<a href="/Architecture.md">rooted</a>`,
		`<a href="Architecture.mdx">notmd</a>`,
		`<code>[x](Architecture.md)</code>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page lacks %s:\n%s", want, out)
		}
	}
	// "../Architecture.md" is the repository's Architecture.md, not the
	// wiki's: it goes to GitHub like any other repository path.
	for _, text := range []string{"up", "dots"} {
		if !strings.Contains(out, `<a href="https://github.com/unheaded/unheaded/blob/develop/Architecture.md">`+text+`</a>`) {
			t.Errorf("the %q link was not sent to GitHub:\n%s", text, out)
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "Links.md"))
	if err != nil || string(after) != src {
		t.Errorf("the Markdown file changed (err %v)", err)
	}
}

func TestWikiPageLink(t *testing.T) {
	t.Parallel()
	pages := map[string]bool{"Architecture": true, "Port Registry": true, "a.b": true, "100%": true}
	cases := []struct {
		dest, want string
		ok         bool
	}{
		{"Architecture.md", "/wiki/Architecture", true},
		{"Architecture.md#wire-format", "/wiki/Architecture#wire-format", true},
		{"./Architecture.md", "/wiki/Architecture", true},
		{"Port%20Registry.md", "/wiki/Port%20Registry", true},
		{"a.b.md", "/wiki/a.b", true},
		{"100%25.md", "/wiki/100%25", true},
		{"Nope.md", "", false},
		{"Architecture", "", false},
		{"architecture.md", "", false}, // slugs are case-sensitive, as files are
		{"Architecture.md?x=1", "", false},
		{"sub/Architecture.md", "", false},
		{"../Architecture.md", "", false},
		{"/Architecture.md", "", false},
		{"..%2FArchitecture.md", "", false},
		{"https://example.com/Architecture.md", "", false},
		{"//example.com/Architecture.md", "", false},
		{"mailto:Architecture.md", "", false},
		{"#Architecture.md", "", false},
		{".md", "", false},
		{"", "", false},
		{"%zz.md", "", false},
	}
	for _, c := range cases {
		got, ok := wikiPageLink(c.dest, pages)
		if ok != c.ok || got != c.want {
			t.Errorf("wikiPageLink(%q) = %q, %v; want %q, %v", c.dest, got, ok, c.want, c.ok)
		}
	}
}
