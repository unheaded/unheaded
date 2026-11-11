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
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"unheaded/pkg/vorclient"
)

// The search box, /search, /sheets and /sheet/ exist only with --vor-url.
// vor answers them all; the wiki server keeps no search data of its own.
//
// What is shown is narrower than what vor holds. vor is also pointed at the
// whole repository, so it knows the wiki pages (category "wiki") and every
// other Markdown file. Here a wiki topic leads to the wiki page, a sheet
// built into vor (source_trust "canonical") is shown at /sheet/<name>, and
// nothing else is shown or linked.
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

	// noPage is the "page being shown" for a page that is not a wiki page.
	// No file name holds a slash, so no sidebar entry lights up.
	noPage = "/"

	maxQueryBytes     = 256
	maxSheetNameBytes = 128
	// maxLineRunes cuts a matching line vor returned, which can be a whole
	// paragraph.
	maxLineRunes = 200

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
{{if .Results}}<p>{{len .Results}} {{if eq (len .Results) 1}}match{{else}}matches{{end}} for <strong>{{.Query}}</strong>.</p>
{{range .Results}}<h3><a href="{{.URL}}">{{.Title}}</a> <span class="page-note">{{.Kind}}</span></h3>
<ul>
{{range .Lines}}<li>{{if .Section}}<strong>{{.Section}}</strong> {{end}}{{.Text}}</li>
{{end}}</ul>
{{end}}{{else}}<p>No wiki page or vor sheet matches <strong>{{.Query}}</strong>.</p>
{{end}}`))

type searchPage struct {
	Query   string
	Results []*searchResult
}

// searchResult is one wiki page or one vor sheet, with its matching lines.
type searchResult struct {
	URL   string
	Title string
	Kind  string
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

// handleSearch serves /search?q=: the wiki pages and vor sheets vor finds
// for q, each with its matching lines, in vor's order, then the wiki pages
// only wikiHits found.
func (ws *WikiServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	switch {
	case q == "":
		ws.vorPage(w, http.StatusOK, "Search", "", noticeTmpl, notice{
			Heading: "Search",
			Text:    "Type a word or two in the search box. It looks through the wiki pages and the vor sheets.",
		})
		return
	case len(q) > maxQueryBytes || !utf8.ValidString(q):
		ws.vorPage(w, http.StatusBadRequest, "Search", "", noticeTmpl, notice{
			Heading: "Search",
			Text:    "That query is too long, or is not text. A word or two is enough.",
		})
		return
	}

	hits, err := ws.vor.Search(r.Context(), q)
	if err != nil {
		ws.vorDown(w, "Search", q, "search", err)
		return
	}
	hits = append(hits, ws.wikiHits(r.Context(), q)...)
	ws.vorPage(w, http.StatusOK, "Search", q, searchTmpl, searchPage{Query: q, Results: ws.searchResults(hits)})
}

// wikiHits asks vor for q once more with its wiki category as one more
// term, and returns the wiki hits of that answer. vor returns at most 50
// lines for a query, before anything is left out here, and one long file of
// the repository can take all 50: without this, "wotan" finds no wiki page.
// vor ranks a topic whose category is a query term first, so the second
// answer holds the wiki pages. A line that matched only the added term is
// dropped; the page stays. A failure here costs these hits and nothing
// else.
func (ws *WikiServer) wikiHits(ctx context.Context, q string) []vorclient.Hit {
	hits, err := ws.vor.Search(ctx, q+" "+wikiCategory)
	if err != nil {
		logf("WARN", "vor did not answer the wiki search", "err", err)
		return nil
	}
	terms := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var out []vorclient.Hit
	for _, h := range hits {
		if h.Category != wikiCategory {
			continue
		}
		if text := strings.ToLower(h.Section + " " + h.Line); !containsAny(text, terms) {
			h.Section, h.Line = "", ""
		}
		out = append(out, h)
	}
	return out
}

// containsAny reports whether s holds one of terms. No terms: true.
func containsAny(s string, terms []string) bool {
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return len(terms) == 0
}

// searchResults groups vor's hits by the page they lead to, in vor's order,
// and drops the hits that lead nowhere this server shows.
func (ws *WikiServer) searchResults(hits []vorclient.Hit) []*searchResult {
	pages := ws.pageSlugs()
	var results []*searchResult
	byURL := map[string]*searchResult{}
	type shown struct {
		url  string
		line searchLine
	}
	seen := map[shown]bool{}
	for _, h := range hits {
		var res searchResult
		switch {
		case h.SourceTrust == vorclient.TrustCanonical:
			res = searchResult{URL: sheetPath + url.PathEscape(h.Topic), Title: h.Topic, Kind: "vor sheet"}
		case h.Category == wikiCategory && pages[h.Topic]:
			res = searchResult{URL: "/wiki/" + url.PathEscape(h.Topic), Title: slugToTitle(h.Topic), Kind: "wiki page"}
		default:
			// The rest of the repository, or a wiki page that is not on
			// disk here.
			continue
		}
		got := byURL[res.URL]
		if got == nil {
			got = &res
			byURL[res.URL] = got
			results = append(results, got)
		}
		line := searchLine{Section: h.Section, Text: cutRunes(strings.TrimSpace(h.Line), maxLineRunes)}
		if line != (searchLine{}) && !seen[shown{res.URL, line}] {
			seen[shown{res.URL, line}] = true
			got.Lines = append(got.Lines, line)
		}
	}
	return results
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

// listSheets asks vor for the sheets /sheet/<name> can show.
//
// vor's listing (/api/topics) names every topic from every source without
// saying which source, and a topic name is unique only within a category.
// So each candidate is fetched once by name, and is listed only when vor
// answers with that name, that category and source_trust "canonical": the
// same test /sheet/<name> applies, which makes every link here one that
// works. Candidates are the topics whose category holds no slash: vor's own
// sheets are sheets/<category>/<name>.md, one level deep, while a directory
// vor was pointed at mostly yields deeper categories ("docs/adr").
func (ws *WikiServer) listSheets(ctx context.Context) (sheetsPage, error) {
	ctx, cancel := context.WithTimeout(ctx, sheetListTimeout)
	defer cancel()

	topics, err := ws.vor.Topics(ctx)
	if err != nil {
		return sheetsPage{}, err
	}

	var page sheetsPage
	byCategory := map[string][]sheetEntry{}
	for _, s := range topics {
		if strings.Contains(s.Category, "/") {
			continue
		}
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
		byCategory[t.Category] = append(byCategory[t.Category], sheetEntry{
			URL:   sheetPath + url.PathEscape(t.Name),
			Name:  t.Name,
			Title: t.Title,
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
