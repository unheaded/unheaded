// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"unheaded/pkg/vorclient"
)

// The search box, /search, /sheets and /sheet/ exist only with --vor-url.
// vor answers them all; the wiki server keeps no search data of its own.
//
// What is shown is narrower than what vor holds. vor is also pointed at the
// whole repository, so it knows the wiki pages (category "wiki") and every
// other Markdown file. Here a wiki topic leads to the wiki page, a sheet
// built into vor (source_trust "canonical") is shown at /sheet/<name>, a
// file under docs/ is a search result that links to GitHub (repolinks.go),
// and nothing else is shown or linked. docs/ is never in the sidebar or on
// /sheets.
//
// The query and every field vor returns are untrusted text. They reach a
// page only through html/template, or, for the Markdown of a sheet, through
// the same goldmark that renders the wiki, which emits no raw HTML.
const (
	searchPath = "/search"
	sheetsPath = "/sheets"
	sheetPath  = "/sheet/"

	// wikiCategory is the category vor files the wiki pages under.
	wikiCategory = "wiki"
	// docsDir is the repository directory whose files search may show.
	docsDir = "docs"

	// noPage is the "page being shown" for a page that is not a wiki page.
	// No file name holds a slash, so no sidebar entry lights up.
	noPage = "/"

	maxQueryBytes     = 256
	maxSheetNameBytes = 128
	// maxLineRunes cuts a matching line vor returned, which can be a whole
	// paragraph.
	maxLineRunes = 200

	// One search asks vor for up to searchLimit lines, searchPerTopic from
	// any one topic, so that one long file cannot take the whole answer, and
	// once more for the wiki category alone, so that the 186 wiki pages are
	// not crowded out by 800 sheets and docs. A group on the page shows its
	// first maxGroupResults topics, each with at most searchPerTopic lines.
	searchLimit     = 500
	wikiSearchLimit = 200
	searchPerTopic  = 2
	maxGroupResults = 25

	// The sheet listing is kept this long, and a failure to build it this
	// long, so that page views do not each ask vor again.
	sheetListTTL   = 10 * time.Minute
	sheetListRetry = 10 * time.Second
	// sheetListTimeout bounds one build of the listing.
	sheetListTimeout = 20 * time.Second
)

// noticeTmpl is a page that only says something: no query, nothing found,
// vor down.
var noticeTmpl = template.Must(template.New("notice").Parse(
	`<h1>{{.Heading}}</h1>
<p>{{.Text}}</p>
`))

type notice struct {
	Heading string
	Text    string
}

var searchTmpl = template.Must(template.New("search").Parse(
	`<h1>Search</h1>
{{if .Total}}<p>{{.Total}} {{if eq .Total 1}}match{{else}}matches{{end}} for <strong>{{.Query}}</strong>.</p>
{{range .Groups}}{{if .Results}}<h2>{{.Name}}</h2>
{{if gt .Count (len .Results)}}<p class="page-note">The first {{len .Results}} of {{.Count}}. More words narrow it.</p>
{{end}}{{range .Results}}<h3><a href="{{.URL}}">{{.Title}}</a>{{if .Note}} <span class="page-note">{{.Note}}</span>{{end}}</h3>
<ul>
{{range .Lines}}<li>{{if .Section}}<strong>{{.Section}}</strong> {{end}}{{.Text}}</li>
{{end}}</ul>
{{end}}{{end}}{{end}}{{else}}<p>Nothing in the wiki, the vor sheets or docs/ matches <strong>{{.Query}}</strong>.</p>
{{end}}`))

type searchPage struct {
	Query  string
	Total  int
	Groups []searchGroup
}

// searchGroup is one kind of result: wiki pages, vor sheets, docs. Count is
// how many vor found; Results the first maxGroupResults of them.
type searchGroup struct {
	Name    string
	Count   int
	Results []*searchResult
}

// The groups, in the order the page shows them.
const (
	groupWiki = iota
	groupSheets
	groupDocs
	groupCount
)

var groupNames = [groupCount]string{"Wiki pages", "vor sheets", "docs/ on GitHub"}

// searchResult is one wiki page, vor sheet or docs file, with its matching
// lines. Note is its category, for a sheet or a docs file.
type searchResult struct {
	URL   string
	Title string
	Note  string
	Lines []searchLine
}

type searchLine struct {
	Section string
	Text    string
}

var sheetsTmpl = template.Must(template.New("sheets").Parse(
	`<h1>Sheets</h1>
<p>{{.Count}} vor sheets in {{len .Categories}} categories.</p>
<p class="sheet-index">{{range $i, $c := .Categories}}<a href="#cat-{{$i}}">{{$c.Name}}</a> {{end}}</p>
{{range $i, $c := .Categories}}<h2 id="cat-{{$i}}">{{$c.Name}}</h2>
<ul>
{{range $c.Sheets}}<li><a href="{{.URL}}">{{.Name}}</a>{{if .Title}} <span class="page-note">{{.Title}}</span>{{end}}</li>
{{end}}</ul>
{{end}}`))

type sheetsPage struct {
	Count      int
	Categories []sheetCategory
}

type sheetCategory struct {
	Name   string
	Sheets []sheetEntry
}

type sheetEntry struct {
	URL   string
	Name  string
	Title string
}

// sheetFootTmpl closes a sheet page by saying what it is.
var sheetFootTmpl = template.Must(template.New("sheetfoot").Parse(
	`<hr>
<p class="page-note">vor sheet {{.Category}}/{{.Name}}</p>
`))

// sheetList is the listing behind /sheets, kept between page views.
type sheetList struct {
	mu   sync.Mutex
	at   time.Time
	page sheetsPage
	err  error
}

// vorPage sends a page made from vor's answer, or a notice, in the wiki's
// template. query is what the search box holds.
func (ws *WikiServer) vorPage(w http.ResponseWriter, status int, title, query string, tmpl *template.Template, data any) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		logf("ERROR", "execute template", "page", title, "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	ws.writePage(w, status, PageData{Title: title, Query: query, NavGroups: ws.navGroups(noPage)}, buf.String())
}

// vorDown says that vor did not answer. The wiki itself is not affected, so
// this is a page and a 503, never a 500.
func (ws *WikiServer) vorDown(w http.ResponseWriter, title, query, what string, err error) {
	logf("ERROR", "vor did not answer", "for", what, "err", err)
	ws.vorPage(w, http.StatusServiceUnavailable, title, query, noticeTmpl, notice{
		Heading: title,
		Text:    "vor did not answer, so " + what + " is unavailable for now. The wiki pages are not affected.",
	})
}

// handleSearch serves /search?q=: the wiki pages, vor sheets and docs files
// vor finds for q, in three groups, each in vor's order with its matching
// lines.
func (ws *WikiServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	switch {
	case q == "":
		ws.vorPage(w, http.StatusOK, "Search", "", noticeTmpl, notice{
			Heading: "Search",
			Text:    "Type a word or two in the search box. It looks through the wiki pages, the vor sheets and docs/.",
		})
		return
	case len(q) > maxQueryBytes || !utf8.ValidString(q):
		ws.vorPage(w, http.StatusBadRequest, "Search", "", noticeTmpl, notice{
			Heading: "Search",
			Text:    "That query is too long, or is not text. A word or two is enough.",
		})
		return
	}

	hits, err := ws.vor.Search(r.Context(), q, vorclient.SearchOptions{Limit: searchLimit, PerTopic: searchPerTopic})
	if err != nil {
		ws.vorDown(w, "Search", q, "search", err)
		return
	}
	// The wiki pages on their own. A failure here costs the wiki pages the
	// first answer did not hold, and nothing else.
	wiki, err := ws.vor.Search(r.Context(), q, vorclient.SearchOptions{
		Category: wikiCategory, Limit: wikiSearchLimit, PerTopic: searchPerTopic,
	})
	if err != nil {
		logf("WARN", "vor did not answer the wiki search", "err", err)
	}
	ws.vorPage(w, http.StatusOK, "Search", q, searchTmpl, ws.searchResults(q, append(hits, wiki...)))
}

// searchResults groups vor's hits by the page they lead to, in vor's order,
// and drops the hits that lead nowhere this server shows. vor ranks the same
// way in both answers, so the wiki hits of the first come first among those
// of the second.
func (ws *WikiServer) searchResults(q string, hits []vorclient.Hit) searchPage {
	pages := ws.pageSlugs()
	docs := ws.openRepo()
	if docs != nil {
		defer func() { _ = docs.Close() }()
	}

	page := searchPage{Query: q, Groups: make([]searchGroup, groupCount)}
	for g := range page.Groups {
		page.Groups[g].Name = groupNames[g]
	}
	type shown struct {
		url  string
		line searchLine
	}
	byURL := map[string]*searchResult{}
	seen := map[shown]bool{}
	for _, h := range hits {
		var res searchResult
		var group int
		switch {
		case h.SourceTrust == vorclient.TrustCanonical:
			group = groupSheets
			res = searchResult{URL: sheetPath + url.PathEscape(h.Topic), Title: h.Topic, Note: h.Category}
		case h.Category == wikiCategory:
			if !pages[h.Topic] {
				continue // a wiki page that is not on disk here
			}
			group = groupWiki
			res = searchResult{URL: "/wiki/" + url.PathEscape(h.Topic), Title: slugToTitle(h.Topic)}
		default:
			link, ok := docsLink(docs, h.Category, h.Topic)
			if !ok {
				continue // the rest of the repository, or a file not here
			}
			group = groupDocs
			res = searchResult{URL: link, Title: h.Topic, Note: h.Category}
		}
		got := byURL[res.URL]
		if got == nil {
			got = &res
			byURL[res.URL] = got
			g := &page.Groups[group]
			g.Count++
			page.Total++
			if len(g.Results) < maxGroupResults {
				g.Results = append(g.Results, got)
			}
		}
		line := searchLine{Section: h.Section, Text: cutRunes(strings.TrimSpace(h.Line), maxLineRunes)}
		if line != (searchLine{}) && !seen[shown{res.URL, line}] && len(got.Lines) < searchPerTopic {
			seen[shown{res.URL, line}] = true
			got.Lines = append(got.Lines, line)
		}
	}
	return page
}

// openRepo opens the repository the wiki directory sits in, for docsLink.
// nil, and no docs results, when it cannot be opened.
func (ws *WikiServer) openRepo() *os.Root {
	root, err := os.OpenRoot(filepath.Dir(ws.wikiDir))
	if err != nil {
		logf("WARN", "open the repository for docs results", "err", err)
		return nil
	}
	return root
}

// docsLink returns the GitHub URL of the file vor knows as topic in
// category, when the category is docs/ or below it and the file is in this
// server's copy of the repository. vor's copy can be older than this one:
// a file since moved or deleted is not linked. vor files a repository file
// as <directory>/<name>.md, so that is the only path tried; anything that
// path cleaning would change, or that leaves the repository, is refused.
func docsLink(repo *os.Root, category, topic string) (string, bool) {
	if repo == nil || topic == "" || strings.ContainsAny(topic, `/\`) || strings.Contains(category, `\`) ||
		(category != docsDir && !strings.HasPrefix(category, docsDir+"/")) {
		return "", false
	}
	p := category + "/" + topic + ".md"
	if path.Clean(p) != p {
		return "", false
	}
	// os.Root refuses to resolve out of the repository, symlinks included.
	info, err := repo.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return repoBlobURL + (&url.URL{Path: p}).EscapedPath(), true
}

// cutRunes shortens s to at most n runes, marking the cut.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// handleSheet serves /sheet/<name>: one sheet built into vor, rendered like
// a wiki page. Any other topic vor holds is a 404 here.
func (ws *WikiServer) handleSheet(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, sheetPath)
	if name == "" || len(name) > maxSheetNameBytes {
		http.Error(w, "Sheet not found", http.StatusNotFound)
		return
	}

	t, err := ws.vor.Topic(r.Context(), name)
	if errors.Is(err, vorclient.ErrNotFound) || (err == nil && t.SourceTrust != vorclient.TrustCanonical) {
		http.Error(w, "Sheet not found", http.StatusNotFound)
		return
	}
	if err != nil {
		ws.vorDown(w, "Sheet", "", "this sheet", err)
		return
	}

	// A sheet's links are its own, not the wiki's: neither [[Page]] nor the
	// repository links are rewritten.
	var body bytes.Buffer
	if err := ws.md.Convert([]byte(t.Content), &body); err != nil {
		logf("ERROR", "render sheet", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := sheetFootTmpl.Execute(&body, t); err != nil {
		logf("ERROR", "execute template", "page", "sheet", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	title := t.Title
	if title == "" {
		title = t.Name
	}
	ws.writePage(w, http.StatusOK, PageData{Title: title, NavGroups: ws.navGroups(noPage)}, body.String())
}

// handleSheets serves /sheets: the sheets built into vor, by category.
func (ws *WikiServer) handleSheets(w http.ResponseWriter, r *http.Request) {
	page, err := ws.sheetsPage(r.Context())
	if err != nil {
		ws.vorDown(w, "Sheets", "", "the list of sheets", err)
		return
	}
	if page.Count == 0 {
		ws.vorPage(w, http.StatusOK, "Sheets", "", noticeTmpl, notice{
			Heading: "Sheets",
			Text:    "vor has no sheets of its own to list.",
		})
		return
	}
	ws.vorPage(w, http.StatusOK, "Sheets", "", sheetsTmpl, page)
}

// sheetsPage returns the listing, building it when the one kept is older
// than sheetListTTL. One build runs at a time; a failure is kept for
// sheetListRetry so that a vor that is down is not asked on every view.
func (ws *WikiServer) sheetsPage(ctx context.Context) (sheetsPage, error) {
	l := &ws.sheets
	l.mu.Lock()
	defer l.mu.Unlock()

	keep := sheetListTTL
	if l.err != nil {
		keep = sheetListRetry
	}
	if !l.at.IsZero() && time.Since(l.at) < keep {
		return l.page, l.err
	}

	page, err := ws.listSheets(ctx)
	if err != nil && ctx.Err() != nil {
		// The reader went away mid-build: that says nothing about vor.
		return sheetsPage{}, err
	}
	l.at, l.page, l.err = time.Now(), page, err
	return page, err
}

// listSheets asks vor for the sheets /sheet/<name> can show: the topics its
// listing marks source_trust "canonical".
//
// vor finds a topic by name alone, in any case, and a name is unique only
// within a category: for a name the listing holds twice, /sheet/<name> may
// get the other topic. Those few are fetched by name and listed only when
// vor answers with that name, that category and source_trust "canonical":
// the same test /sheet/<name> applies, which makes every link here one that
// works.
func (ws *WikiServer) listSheets(ctx context.Context) (sheetsPage, error) {
	ctx, cancel := context.WithTimeout(ctx, sheetListTimeout)
	defer cancel()

	topics, err := ws.vor.Topics(ctx)
	if err != nil {
		return sheetsPage{}, err
	}
	names := map[string]int{}
	for _, s := range topics {
		names[strings.ToLower(s.Name)]++
	}

	var page sheetsPage
	byCategory := map[string][]sheetEntry{}
	for _, s := range topics {
		if s.SourceTrust != vorclient.TrustCanonical {
			continue
		}
		if names[strings.ToLower(s.Name)] > 1 {
			t, err := ws.vor.Topic(ctx, s.Name)
			if errors.Is(err, vorclient.ErrNotFound) {
				continue
			}
			if err != nil {
				return sheetsPage{}, err
			}
			if t.Category != s.Category || t.SourceTrust != vorclient.TrustCanonical {
				continue
			}
		}
		byCategory[s.Category] = append(byCategory[s.Category], sheetEntry{
			URL:   sheetPath + url.PathEscape(s.Name),
			Name:  s.Name,
			Title: s.Title,
		})
		page.Count++
	}

	for name, sheets := range byCategory {
		sort.Slice(sheets, func(i, j int) bool { return sheets[i].Name < sheets[j].Name })
		page.Categories = append(page.Categories, sheetCategory{Name: name, Sheets: sheets})
	}
	sort.Slice(page.Categories, func(i, j int) bool { return page.Categories[i].Name < page.Categories[j].Name })
	return page, nil
}
