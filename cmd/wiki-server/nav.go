// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// The sidebar is drawn from wiki/_Sidebar.md, the file GitHub's wiki uses for
// its own navigation, so the page groups here and there are one list kept in
// one place. Without that file the sidebar is the flat alphabetical list.
const (
	sidebarFile = "_Sidebar.md"
	homeFile    = "Home.md"
	readmeFile  = "README.md"

	// otherPagesTitle heads the pages on disk that _Sidebar.md does not name,
	// so a page somebody forgot to list can still be reached.
	otherPagesTitle = "Other pages"
)

// NavEntry is one link in the sidebar.
type NavEntry struct {
	Slug  string
	Title string
	// Note is the plain text after the link on its line, such as "(legacy)".
	Note string
	// Sub marks an entry indented under the one above it.
	Sub bool
	// Active marks the page being shown.
	Active bool
	// Missing marks an entry with no page behind it; it is shown without a link.
	Missing bool
}

// NavBlock is a run of entries. A block with a Summary is folded (<details>)
// and opens by itself when the page being shown is inside it.
type NavBlock struct {
	Summary string
	Open    bool
	Items   []NavEntry
}

// NavGroup is one titled section of the sidebar.
type NavGroup struct {
	Title  string
	Blocks []NavBlock
}

var (
	sidebarGroupRe   = regexp.MustCompile(`^\*\*(.+?)\*\*\s*$`)
	sidebarItemRe    = regexp.MustCompile(`^(\s*)[-*]\s+\[\[([^\]|]+)(?:\|([^\]]+))?\]\]\s*(.*)$`)
	sidebarSummaryRe = regexp.MustCompile(`^<details>\s*<summary>(.*?)</summary>\s*$`)
)

// parseSidebar reads the shapes _Sidebar.md is written in and nothing else:
//
//	**Group**                         a section title
//	- [[Title|Page]]                  a link ([[Page]] when both are the same)
//	  - [[Title|Page]]                a link indented under the one above
//	- [[Title|Page]] *(note)*         a link with a note
//	<details><summary>X</summary>     a folded run of links, to </details>
//
// The "### [[Home]]" line, rules and blank lines are skipped: the pinned Top
// link already leads home. Any other line is skipped too, so a stray line in
// the file costs one entry, never the sidebar.
func parseSidebar(src []byte) []NavGroup {
	var groups []NavGroup
	folded := false

	block := func() *NavBlock {
		if len(groups) == 0 {
			groups = append(groups, NavGroup{})
		}
		g := &groups[len(groups)-1]
		if len(g.Blocks) == 0 {
			g.Blocks = append(g.Blocks, NavBlock{})
		}
		return &g.Blocks[len(g.Blocks)-1]
	}
	newBlock := func(summary string) {
		if len(groups) == 0 {
			groups = append(groups, NavGroup{})
		}
		g := &groups[len(groups)-1]
		g.Blocks = append(g.Blocks, NavBlock{Summary: summary})
	}

	for _, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case trimmed == "</details>":
			if folded {
				folded = false
				newBlock("")
			}
			continue
		}
		if m := sidebarSummaryRe.FindStringSubmatch(trimmed); m != nil {
			newBlock(strings.TrimSpace(m[1]))
			folded = true
			continue
		}
		if m := sidebarGroupRe.FindStringSubmatch(trimmed); m != nil {
			groups = append(groups, NavGroup{Title: strings.TrimSpace(m[1])})
			folded = false
			continue
		}
		if m := sidebarItemRe.FindStringSubmatch(line); m != nil {
			title := strings.TrimSpace(m[2])
			slug := strings.TrimSpace(m[3])
			if slug == "" {
				slug = title
			}
			b := block()
			b.Items = append(b.Items, NavEntry{
				Slug:  slug,
				Title: title,
				Note:  strings.Trim(m[4], "*_ \t"),
				Sub:   len(m[1]) > 0,
			})
		}
	}

	// Drop what ended up empty (a trailing block after </details>, a title
	// with no links under it).
	out := groups[:0]
	for _, g := range groups {
		blocks := g.Blocks[:0]
		for _, b := range g.Blocks {
			if len(b.Items) > 0 {
				blocks = append(blocks, b)
			}
		}
		g.Blocks = blocks
		if len(g.Blocks) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// rootFile names the page served at /wiki/: Home.md, the landing page of the
// GitHub wiki, when the directory has one, and README.md otherwise.
func (ws *WikiServer) rootFile() string {
	root, err := os.OpenRoot(ws.wikiDir)
	if err != nil {
		return readmeFile
	}
	defer root.Close()
	if info, err := root.Stat(homeFile); err == nil && info.Mode().IsRegular() {
		return homeFile
	}
	return readmeFile
}

// pageSlugs lists the pages on disk by slug. Names starting with "_" are the
// wiki's own furniture (_Sidebar, _Footer), not pages.
func (ws *WikiServer) pageSlugs() map[string]bool {
	pages := map[string]bool{}
	entries, err := os.ReadDir(ws.wikiDir)
	if err != nil {
		logf("ERROR", "list wiki dir", "err", err)
		return pages
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, "_") {
			continue
		}
		pages[strings.TrimSuffix(name, ".md")] = true
	}
	return pages
}

// navGroups builds the sidebar for the page being shown. active is the slug
// from the URL ("" at /wiki/).
func (ws *WikiServer) navGroups(active string) []NavGroup {
	if active == "" {
		active = strings.TrimSuffix(ws.rootFile(), ".md")
	}

	var groups []NavGroup
	if root, err := os.OpenRoot(ws.wikiDir); err == nil {
		// Through os.Root for the same reason renderPage uses it: a symlink
		// named _Sidebar.md must not read a file outside the wiki.
		if src, err := root.ReadFile(sidebarFile); err == nil {
			groups = parseSidebar(src)
		}
		root.Close()
	}

	if len(groups) == 0 {
		flat := NavBlock{}
		for _, p := range ws.listPages() {
			flat.Items = append(flat.Items, NavEntry{Slug: p.Slug, Title: p.Title, Active: p.Slug == active})
		}
		if len(flat.Items) == 0 {
			return nil
		}
		return []NavGroup{{Blocks: []NavBlock{flat}}}
	}

	pages := ws.pageSlugs()
	listed := map[string]bool{}
	for gi := range groups {
		for bi := range groups[gi].Blocks {
			b := &groups[gi].Blocks[bi]
			for ii := range b.Items {
				it := &b.Items[ii]
				listed[it.Slug] = true
				it.Missing = !pages[it.Slug]
				it.Active = !it.Missing && it.Slug == active
				if it.Active {
					b.Open = true
				}
			}
		}
	}

	rootSlug := strings.TrimSuffix(ws.rootFile(), ".md")
	var rest []NavEntry
	for slug := range pages {
		if listed[slug] || slug == rootSlug {
			continue
		}
		rest = append(rest, NavEntry{Slug: slug, Title: slugToTitle(slug), Active: slug == active})
	}
	if len(rest) > 0 {
		sort.Slice(rest, func(i, j int) bool { return rest[i].Title < rest[j].Title })
		groups = append(groups, NavGroup{Title: otherPagesTitle, Blocks: []NavBlock{{Items: rest}}})
	}
	return groups
}
