// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"unheaded/pkg/vorclient"
)

// fakeVor answers the three calls the wiki server makes, the way vor does:
// a topic is found by name alone, the first of that name wins, and an
// unknown name gets the nearest match, which here is the first topic.
type fakeVor struct {
	hits   map[string][]vorclient.Hit
	fail   map[string]bool // queries answered with a 500
	topics []vorclient.Topic

	mu    sync.Mutex
	calls map[string]int // by request path
}

func (f *fakeVor) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}

func (f *fakeVor) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		n += c
	}
	return n
}

func (f *fakeVor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[r.URL.Path]++
	f.mu.Unlock()

	switch {
	case r.URL.Path == "/api/search" && f.fail[r.URL.Query().Get("q")]:
		http.Error(w, "boom", http.StatusInternalServerError)
	case r.URL.Path == "/api/search":
		_ = json.NewEncoder(w).Encode(f.hits[r.URL.Query().Get("q")]) // no hits: null, as vor answers
	case r.URL.Path == "/api/topics":
		var list []vorclient.TopicSummary
		for _, t := range f.topics {
			list = append(list, vorclient.TopicSummary{Name: t.Name, Category: t.Category, Title: t.Title})
		}
		_ = json.NewEncoder(w).Encode(list)
	case strings.HasPrefix(r.URL.Path, "/api/topics/"):
		name := strings.TrimPrefix(r.URL.Path, "/api/topics/")
		for _, t := range f.topics {
			if strings.EqualFold(t.Name, name) {
				_ = json.NewEncoder(w).Encode(t)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(f.topics[0])
	default:
		http.NotFound(w, r)
	}
}

// Hostile text for every place vor's text, or the reader's, reaches a page.
const (
	evilScript = `"><script>alert(1)</script>`
	evilImg    = `<img src=x onerror=alert(1)>`
	evilAttr   = `" onmouseover="alert(1)`
	evilSVG    = `</li></ul><svg onload=alert(1)>`
	evilCat    = `<svg onload=alert(1)> & co`
)

func canonical(category, name, title, content string) vorclient.Topic {
	return vorclient.Topic{Name: name, Category: category, Title: title, Content: content,
		SourceKind: "embedded", SourceTrust: vorclient.TrustCanonical}
}

func external(category, name, title string) vorclient.Topic {
	return vorclient.Topic{Name: name, Category: category, Title: title, Content: "# " + title + "\n\nNot for this server.\n",
		SourceKind: "user-source", SourceTrust: "external", SourcePath: "/var/zhen/vor-source/unheaded", SourceLabel: "unheaded"}
}

func hit(t vorclient.Topic, section, line string) vorclient.Hit {
	return vorclient.Hit{Topic: t.Name, Category: t.Category, Section: section, Line: line,
		SourceKind: t.SourceKind, SourceTrust: t.SourceTrust, SourcePath: t.SourcePath, SourceLabel: t.SourceLabel}
}

// searchFixture is a wiki of three pages and a vor that holds them, the
// rest of a repository, and sheets of its own.
func searchFixture(t *testing.T) (http.Handler, *fakeVor) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"README.md":            "# Home\n",
		"Wotan.md":             "# Wotan\n\nThe all-father.\n",
		"Chaos-Engineering.md": "# Chaos Engineering\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	bash := canonical("shell", "bash", "Bash", "# Bash\n\n## Loops\n\n    for f in *; do echo \"$f\"; done\n\nSee [zsh](../shell/zsh.md) and [[ -f x ]].\n")
	wotan := external("wiki", "Wotan", "Wotan")
	ghost := external("wiki", "Ghost-Page", "Ghost Page")
	adr := external("docs/adr", "ADR-900-unpublished", "ADR-900: Unpublished")
	hostile := canonical("shell", evilImg, evilScript,
		"# Hostile\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(1)> inline\n\n"+
			"[click](javascript:alert(1)) [data](data:text/html,<script>alert(1)</script>)\n\n"+
			"<iframe src=\"https://example.com\"></iframe>\n\n<div onclick=\"alert(1)\">x</div>\n")

	vor := &fakeVor{
		topics: []vorclient.Topic{
			bash, // first: the nearest match for a name vor does not have
			wotan, ghost, adr, hostile,
			canonical("networking", "tcp", "TCP", "# TCP\n"),
			canonical("networking", "arp", "ARP", "# ARP\n"),
			canonical("shell", evilAttr, "Attr", "# Attr\n"),
			canonical(evilCat, "odd", "Odd", "# Odd\n"),
			// A repository file in a category vor's own sheets also use.
			external("security", "cve-fixes", "CVE fixes"),
			// One name, twice: vor answers with the wiki page, so the
			// sheet of that name cannot be shown.
			external("wiki", "Chaos-Engineering", "Chaos Engineering"),
			canonical("testing", "chaos-engineering", "Chaos engineering", "# Chaos\n"),
			{Name: "mine", Category: "notes", Title: "Mine", Content: "# Mine\n", SourceKind: "user-custom", SourceTrust: "local"},
		},
	}
	vor.hits = map[string][]vorclient.Hit{
		"loop": {
			hit(adr, "Decision", "the event loop is unpublished"),
			hit(bash, "Loops", `for f in *; do echo "$f"; done`),
			hit(wotan, "Heartbeat", "Wotan's loop runs every second"),
			hit(ghost, "Intro", "a loop nobody can read"),
			hit(bash, "While", "while read -r line; do :; done"),
			hit(wotan, "", strings.Repeat("long ", 100)),
		},
		"docsonly": {hit(adr, "Decision", "only the repository matches")},
		// What vor does with a common word: one repository file takes
		// every line of the answer, and the wiki pages come only when the
		// wiki category is a term too.
		"father": {
			hit(adr, "Decision", "the all-father is unpublished"),
			hit(adr, "Context", "father of the unpublished"),
		},
		"father wiki": {
			hit(wotan, "Wotan", "The all-father."),
			hit(wotan, "Wotan", "The all-father."),
			hit(wotan, "See also", "the wiki has more"),
			hit(ghost, "Intro", "a father nobody can read"),
			hit(adr, "Decision", "the wiki's father is unpublished"),
			hit(bash, "Loops", "father of wiki loops"),
			hit(external("wiki", "Chaos-Engineering", "Chaos Engineering"), "", "the wiki, and nothing else"),
		},
		"loop wiki": {
			hit(wotan, "Heartbeat", "Wotan's loop runs every second"),
			hit(wotan, "Timers", "a second loop"),
		},
		"half": {hit(bash, "Loops", "half a loop")},
		evilScript: {
			hit(hostile, evilScript, evilSVG),
			{Topic: "javascript:alert(1)", Category: "shell", Section: evilImg, Line: evilAttr, SourceTrust: vorclient.TrustCanonical},
			{Topic: evilImg, Category: evilSVG, Section: "s", Line: "wiki hit for a page that does not exist", SourceTrust: "external"},
		},
	}
	vor.fail = map[string]bool{"half wiki": true}
	srv := httptest.NewServer(vor)
	t.Cleanup(srv.Close)

	h, err := buildHandler(dir, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return h, vor
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

var mainRe = regexp.MustCompile(`(?s)<main class="main">(.*)</main>`)

// mainOf returns the part of a page that holds its content.
func mainOf(t *testing.T, body string) string {
	t.Helper()
	m := mainRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("page has no <main>:\n%s", body)
	}
	return m[1]
}

// wantInert fails when hostile text came through as markup: a tag the page
// template does not write, an event handler attribute, or a script URL.
func wantInert(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	body := w.Body.String()
	content := mainOf(t, body)
	for _, live := range []string{"<script", "<img", "<svg", "<iframe", "<div", "<form", "<style", "<input"} {
		if strings.Contains(content, live) {
			t.Errorf("live %s in the page content:\n%s", live, content)
		}
	}
	// Outside the content the template writes one script, one style, one
	// form and one input, and nothing may add to them.
	for tag, want := range map[string]int{"<script": 1, "<style": 1, "<svg": 0, "<img": 0, "<iframe": 0} {
		if n := strings.Count(body, tag); n != want {
			t.Errorf("%d %s in the page, want %d", n, tag, want)
		}
	}
	// Every attribute the pages write is name="value" with the value's own
	// quotes escaped, so an on* name after whole attributes is a live one.
	if m := regexp.MustCompile(`(?i)<[a-z][a-z0-9]*(?:\s+[a-z-]+(?:=(?:"[^"]*"|[^\s"'>]+))?)*\s+on[a-z]+\s*=`).FindString(body); m != "" {
		t.Errorf("event handler attribute in the page: %s", m)
	}
	if m := regexp.MustCompile(`(?i)(href|src|action)\s*=\s*["']?\s*(javascript|data|vbscript):[^"'>]*`).FindString(body); m != "" {
		t.Errorf("script URL in the page: %s", m)
	}
	wantCSPHolds(t, w)
}

// wantCSPHolds: the page is served under the wiki's policy, and its inline
// blocks are the two the policy allows.
func wantCSPHolds(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	csp := w.Header().Get("Content-Security-Policy")
	if csp != wikiCSP() {
		t.Errorf("CSP = %q, want the wiki's %q", csp, wikiCSP())
	}
	if strings.Contains(csp, "unsafe-inline") || strings.Count(csp, "'sha256-") != 2 {
		t.Errorf("CSP = %q, want exactly two hashes and no unsafe-inline", csp)
	}
	for _, tag := range []string{"script", "style"} {
		for _, m := range regexp.MustCompile(`(?s)<`+tag+`>(.*?)</`+tag+`>`).FindAllStringSubmatch(w.Body.String(), -1) {
			sum := sha256.Sum256([]byte(m[1]))
			if !strings.Contains(csp, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
				t.Errorf("served <%s> block is not allowed by the CSP", tag)
			}
		}
	}
	if strings.Contains(w.Body.String(), ` style="`) {
		t.Error("inline style attribute in the page; the CSP refuses those")
	}
}

func TestSearch_OffWithoutVorURL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := buildHandler(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/search?q=x", "/search", "/sheets", "/sheet/bash"} {
		if w := get(t, h, target); w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 without --vor-url", target, w.Code)
		}
	}
	w := get(t, h, "/wiki/")
	if body := w.Body.String(); w.Code != http.StatusOK || strings.Contains(body, "<form") || strings.Contains(body, `href="/sheets"`) {
		t.Errorf("GET /wiki/ = %d; the search box or the Sheets link is shown without --vor-url", w.Code)
	}

	if _, err := buildHandler(dir, "127.0.0.1:20110"); err == nil {
		t.Error("a --vor-url that is not a URL was accepted")
	}
}

func TestSearch_BoxSitsBetweenTheLinksAndTop(t *testing.T) {
	h, vor := searchFixture(t)
	w := get(t, h, "/wiki/Wotan")
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("GET /wiki/Wotan = %d", w.Code)
	}
	last := -1
	for _, part := range []string{
		`>Zhenai</a>`,
		`<form class="sidebar-search" action="/search" method="get" role="search">`,
		`<input type="search" name="q" value=""`,
		`<a href="/sheets">Sheets</a>`,
		`class="sidebar-home`,
	} {
		at := strings.Index(body, part)
		if at < 0 || at < last {
			t.Fatalf("sidebar lacks %s, or it is out of order", part)
		}
		last = at
	}
	wantCSPHolds(t, w)
	if vor.total() != 0 {
		t.Errorf("a wiki page made %d calls to vor, want 0", vor.total())
	}
}

func TestSearch_GroupsByPageAndLeavesOutTheRest(t *testing.T) {
	h, _ := searchFixture(t)
	w := get(t, h, "/search?q=loop")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /search?q=loop = %d", w.Code)
	}
	content := mainOf(t, w.Body.String())

	// One group a page, in vor's order, each with its lines.
	bash := strings.Index(content, `<h3><a href="/sheet/bash">bash</a> <span class="page-note">vor sheet</span></h3>`)
	wotan := strings.Index(content, `<h3><a href="/wiki/Wotan">Wotan</a> <span class="page-note">wiki page</span></h3>`)
	if bash < 0 || wotan < bash {
		t.Fatalf("want the bash sheet, then the Wotan page:\n%s", content)
	}
	if n := strings.Count(content, "<h3>"); n != 2 {
		t.Errorf("%d groups, want 2", n)
	}
	for _, want := range []string{
		"2 matches for <strong>loop</strong>",
		`<li><strong>Loops</strong> for f in *; do echo &#34;$f&#34;; done</li>`,
		`<li><strong>While</strong> while read -r line; do :; done</li>`,
		`<li><strong>Heartbeat</strong> Wotan&#39;s loop runs every second</li>`,
		// Found only by the second question to vor.
		`<li><strong>Timers</strong> a second loop</li>`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("results lack %s:\n%s", want, content)
		}
	}
	if n := strings.Count(content, "Heartbeat"); n != 1 {
		t.Errorf("a line both answers hold is shown %d times, want 1", n)
	}
	if !strings.Contains(content, "long long") || !strings.Contains(content, "…</li>") ||
		strings.Contains(content, strings.Repeat("long ", 50)) {
		t.Error("a long matching line was not cut")
	}

	// The repository's own files, and a wiki page that is not on disk.
	for _, hidden := range []string{"ADR-900", "unpublished", "Ghost", "nobody can read", "docs/adr"} {
		if strings.Contains(w.Body.String(), hidden) {
			t.Errorf("results show %q, which is neither a wiki page here nor a vor sheet", hidden)
		}
	}
	if !strings.Contains(w.Body.String(), `name="q" value="loop"`) {
		t.Error("the search box does not hold the query")
	}
	wantCSPHolds(t, w)
}

// TestSearch_WikiPagesBehindVorsCap: the plain query is answered with the
// repository's own files alone, and the wiki pages still come.
func TestSearch_WikiPagesBehindVorsCap(t *testing.T) {
	h, vor := searchFixture(t)
	w := get(t, h, "/search?q=father")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /search?q=father = %d", w.Code)
	}
	content := mainOf(t, w.Body.String())
	wotan := strings.Index(content, `<h3><a href="/wiki/Wotan">Wotan</a> <span class="page-note">wiki page</span></h3>`)
	chaos := strings.Index(content, `<h3><a href="/wiki/Chaos-Engineering">Chaos Engineering</a> <span class="page-note">wiki page</span></h3>`)
	if wotan < 0 || chaos < wotan {
		t.Fatalf("want the Wotan page, then the Chaos Engineering page:\n%s", content)
	}
	if n := strings.Count(content, "<h3>"); n != 2 {
		t.Errorf("%d groups, want 2:\n%s", n, content)
	}
	if n := strings.Count(content, "<li>"); n != 1 || !strings.Contains(content, `<li><strong>Wotan</strong> The all-father.</li>`) {
		t.Errorf("%d lines, want the one line that holds the query, once:\n%s", n, content)
	}
	// Only wiki pages are taken from the second answer, and no line that
	// matched the added term alone.
	for _, hidden := range []string{"ADR-900", "unpublished", "Ghost", "nobody can read", "/sheet/bash", "wiki loops", "has more", "nothing else"} {
		if strings.Contains(content, hidden) {
			t.Errorf("results show %q", hidden)
		}
	}
	if !strings.Contains(w.Body.String(), `name="q" value="father"`) {
		t.Error("the search box does not hold the query as typed")
	}
	wantCSPHolds(t, w)

	// vor answers the first question and not the second: the first stands.
	before := vor.total()
	w = get(t, h, "/search?q=half")
	if w.Code != http.StatusOK || !strings.Contains(mainOf(t, w.Body.String()), `<li><strong>Loops</strong> half a loop</li>`) {
		t.Errorf("GET /search?q=half = %d, want 200 and the bash sheet:\n%s", w.Code, mainOf(t, w.Body.String()))
	}
	if got := vor.total() - before; got != 2 {
		t.Errorf("%d calls to vor, want 2", got)
	}
}

func TestSearch_PlainPages(t *testing.T) {
	h, vor := searchFixture(t)
	for _, tc := range []struct {
		name, target string
		code         int
		want         string
		vorCalls     int
	}{
		{"no query", "/search", http.StatusOK, "Type a word or two", 0},
		{"empty query", "/search?q=", http.StatusOK, "Type a word or two", 0},
		{"blank query", "/search?q=+%09+", http.StatusOK, "Type a word or two", 0},
		{"too long", "/search?q=" + strings.Repeat("a", maxQueryBytes+1), http.StatusBadRequest, "too long", 0},
		{"not text", "/search?q=%ff%fe", http.StatusBadRequest, "not text", 0},
		{"no hits", "/search?q=nothing", http.StatusOK, "No wiki page or vor sheet matches <strong>nothing</strong>", 2},
		{"hits, none shown", "/search?q=docsonly", http.StatusOK, "No wiki page or vor sheet matches <strong>docsonly</strong>", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := vor.total()
			w := get(t, h, tc.target)
			if w.Code != tc.code || !strings.Contains(mainOf(t, w.Body.String()), tc.want) {
				t.Errorf("GET %s = %d, want %d and %q:\n%s", tc.target, w.Code, tc.code, tc.want, mainOf(t, w.Body.String()))
			}
			if got := vor.total() - before; got != tc.vorCalls {
				t.Errorf("%d calls to vor, want %d", got, tc.vorCalls)
			}
			if strings.Contains(w.Body.String(), "only the repository matches") {
				t.Error("a repository hit is shown")
			}
			wantInert(t, w)
		})
	}
}

func TestSearch_HostileQueryAndHostileVor(t *testing.T) {
	h, _ := searchFixture(t)
	w := get(t, h, "/search?q="+url.QueryEscape(evilScript))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	wantInert(t, w)

	body := w.Body.String()
	for _, want := range []string{
		// The query, in the box and in the text.
		`value="&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;"`,
		`<strong>&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;</strong>`,
		// vor's topic, section and line.
		`<a href="/sheet/%3Cimg%20src=x%20onerror=alert%281%29%3E">&lt;img src=x onerror=alert(1)&gt;</a>`,
		`&lt;/li&gt;&lt;/ul&gt;&lt;svg onload=alert(1)&gt;`,
		// A topic named like a script URL is a path under /sheet/.
		`<a href="/sheet/javascript:alert%281%29">javascript:alert(1)</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks the escaped %s:\n%s", want, mainOf(t, body))
		}
	}
	if strings.Contains(body, "does not exist") {
		t.Error("a hit that is neither a wiki page nor a vor sheet is shown")
	}

	// Other ways to break out of the box.
	for _, q := range []string{evilAttr, evilImg, evilSVG, `'"><svg/onload=alert(1)>`, "x\r\nSet-Cookie: a=b", "a\x00b", "{{.Title}}"} {
		w := get(t, h, "/search?q="+url.QueryEscape(q))
		if w.Code != http.StatusOK {
			t.Errorf("q=%q: status = %d", q, w.Code)
		}
		wantInert(t, w)
		if w.Header().Get("Set-Cookie") != "" {
			t.Errorf("q=%q set a cookie", q)
		}
	}
}

func TestSheet(t *testing.T) {
	h, _ := searchFixture(t)

	w := get(t, h, "/sheet/bash")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /sheet/bash = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"<title>Bash | (U)NHEADED Wiki</title>",
		`<h1 id="bash">Bash</h1>`,
		`for f in *; do echo &quot;$f&quot;; done`,
		`<p class="page-note">vor sheet shell/bash</p>`,
		// A sheet's own text is left alone: no wiki link, no GitHub link.
		`<a href="../shell/zsh.md">zsh</a>`,
		`[[ -f x ]]`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sheet page lacks %s:\n%s", want, mainOf(t, body))
		}
	}
	if strings.Contains(body, "github.com") {
		t.Error("a sheet's link was rewritten to GitHub")
	}
	if strings.Contains(body, `class="active"`) {
		t.Error("a sidebar entry is marked active on a sheet page")
	}
	wantInert(t, w)

	for _, tc := range []struct{ name, target string }{
		{"unknown name, vor answers with its nearest match", "/sheet/nosuch"},
		{"another case", "/sheet/Bash"},
		{"a wiki page", "/sheet/Wotan"},
		{"a repository file", "/sheet/ADR-900-unpublished"},
		{"the operator's own sheet", "/sheet/mine"},
		{"a sheet hidden behind a wiki page of its name", "/sheet/chaos-engineering"},
		{"no name", "/sheet/"},
		{"two segments", "/sheet/shell/bash"},
		{"trailing slash", "/sheet/bash/"},
		{"dots", "/sheet/..%2f..%2fapi%2ftopics"},
		{"too long", "/sheet/" + strings.Repeat("a", maxSheetNameBytes+1)},
	} {
		w := get(t, h, tc.target)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: GET %s = %d, want 404", tc.name, tc.target, w.Code)
		}
		for _, hidden := range []string{"Not for this server", "# Mine", "All-father"} {
			if strings.Contains(w.Body.String(), hidden) {
				t.Errorf("%s: GET %s shows a topic that is not a vor sheet", tc.name, tc.target)
			}
		}
	}
}

func TestSheet_HostileContent(t *testing.T) {
	h, _ := searchFixture(t)
	w := get(t, h, "/sheet/"+url.PathEscape(evilImg))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	wantInert(t, w)
	body := w.Body.String()
	for _, want := range []string{
		`<title>&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt; | (U)NHEADED Wiki</title>`,
		`<p class="page-note">vor sheet shell/&lt;img src=x onerror=alert(1)&gt;</p>`,
		// goldmark drops raw HTML, block or inline, and empties a script URL.
		`<p><!-- raw HTML omitted --> inline</p>`,
		`<p><a href="">click</a> <a href="">data</a></p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s:\n%s", want, mainOf(t, body))
		}
	}
}

func TestSheets_ListsOnlyWhatSheetCanShow(t *testing.T) {
	h, vor := searchFixture(t)
	w := get(t, h, "/sheets")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /sheets = %d:\n%s", w.Code, w.Body.String())
	}
	wantInert(t, w)
	content := mainOf(t, w.Body.String())

	// Categories in order, sheets in order under them.
	last := -1
	for _, part := range []string{
		`6 vor sheets in 3 categories.`,
		`<a href="#cat-0">&lt;svg onload=alert(1)&gt; &amp; co</a>`,
		`<h2 id="cat-0">&lt;svg onload=alert(1)&gt; &amp; co</h2>`,
		`<li><a href="/sheet/odd">odd</a> <span class="page-note">Odd</span></li>`,
		`<h2 id="cat-1">networking</h2>`,
		`<li><a href="/sheet/arp">arp</a> <span class="page-note">ARP</span></li>`,
		`<li><a href="/sheet/tcp">tcp</a> <span class="page-note">TCP</span></li>`,
		`<h2 id="cat-2">shell</h2>`,
		`<li><a href="/sheet/%22%20onmouseover=%22alert%281%29">&#34; onmouseover=&#34;alert(1)</a> <span class="page-note">Attr</span></li>`,
		`<li><a href="/sheet/%3Cimg%20src=x%20onerror=alert%281%29%3E">&lt;img src=x onerror=alert(1)&gt;</a> <span class="page-note">&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;</span></li>`,
		`<li><a href="/sheet/bash">bash</a> <span class="page-note">Bash</span></li>`,
	} {
		at := strings.Index(content, part)
		if at < 0 || at < last {
			t.Fatalf("listing lacks %s, or it is out of order:\n%s", part, content)
		}
		last = at
	}
	if !strings.Contains(content, `<a href="#cat-1">networking</a>`) {
		t.Error("the category index does not link to its heading")
	}

	// Wiki pages, repository files (even in a category the sheets use), the
	// operator's sheets, and a sheet vor cannot hand out by name.
	for _, hidden := range []string{"Wotan", "Ghost", "ADR-900", "cve-fixes", "CVE fixes", "mine", "Mine", "notes", "haos", "testing", "security"} {
		if strings.Contains(content, hidden) {
			t.Errorf("listing shows %q:\n%s", hidden, content)
		}
	}
	// A topic in a nested category is never one of vor's own, and is not
	// fetched to find that out.
	if n := vor.count("/api/topics/ADR-900-unpublished"); n != 0 {
		t.Errorf("fetched a docs/adr topic %d times, want 0", n)
	}

	// Every link in the listing opens.
	for _, m := range regexp.MustCompile(`<li><a href="(/sheet/[^"]+)"`).FindAllStringSubmatch(content, -1) {
		if got := get(t, h, m[1]); got.Code != http.StatusOK {
			t.Errorf("listed link %s = %d, want 200", m[1], got.Code)
		}
	}

	// The listing is kept: a second view asks vor nothing.
	before := vor.total()
	if again := get(t, h, "/sheets"); again.Code != http.StatusOK || again.Body.String() != w.Body.String() {
		t.Errorf("second GET /sheets = %d, or a different page", again.Code)
	}
	if got := vor.total() - before; got != 0 {
		t.Errorf("second view made %d calls to vor, want 0", got)
	}
}

func TestSheets_NothingToList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(&fakeVor{topics: []vorclient.Topic{external("wiki", "Home", "Home")}})
	t.Cleanup(srv.Close)
	h, err := buildHandler(dir, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	w := get(t, h, "/sheets")
	if w.Code != http.StatusOK || !strings.Contains(mainOf(t, w.Body.String()), "vor has no sheets of its own to list.") {
		t.Errorf("GET /sheets = %d:\n%s", w.Code, w.Body.String())
	}
}

// TestVorDown: a vor that does not answer costs the three vor pages, each a
// plain 503 page, and nothing else.
func TestVorDown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Reserved port 1: nothing listens.
	h, err := buildHandler(dir, "http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/search?q=" + url.QueryEscape(evilScript), "/sheets", "/sheets", "/sheet/bash"} {
		w := get(t, h, target)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503", target, w.Code)
		}
		content := mainOf(t, w.Body.String())
		if !strings.Contains(content, "vor did not answer") || !strings.Contains(content, "The wiki pages are not affected.") {
			t.Errorf("GET %s does not say what happened:\n%s", target, content)
		}
		if strings.Contains(w.Body.String(), "127.0.0.1:1") || strings.Contains(w.Body.String(), "connection refused") {
			t.Errorf("GET %s shows the reader the error text", target)
		}
		wantInert(t, w)
	}
	for _, target := range []string{"/wiki/", "/health", "/search"} {
		if w := get(t, h, target); w.Code != http.StatusOK {
			t.Errorf("GET %s = %d with vor down, want 200", target, w.Code)
		}
	}
}

// TestVorBroken: a vor that answers nonsense is a 503 page too.
func TestVorBroken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><script>alert(1)</script>`))
	}))
	t.Cleanup(srv.Close)
	h, err := buildHandler(dir, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/search?q=x", "/sheets", "/sheet/bash"} {
		w := get(t, h, target)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503", target, w.Code)
		}
		wantInert(t, w)
	}
}
