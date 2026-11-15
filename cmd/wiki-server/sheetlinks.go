// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// vor's sheets link each other with relative paths, written three ways:
// "tmux.md" (the same category), "../editors/neovim.md" (the category
// beside it) and "networking/tcp" (from the top). None of them names a
// page of this server, where a sheet is /sheet/<category>/<name>.
//
// rewriteSheetLinks points such a link at the sheet when known says vor
// has it. When the link names a sheet vor does not have, the link is
// removed and its text kept: a link that can only 404 is worse than none.
// Everything else is left as written: absolute URLs, anchors, rooted
// paths, paths that climb out of the sheets, and images. Text and code are
// never touched, so the "[[ ... ]]" of a shell test stays what it is.
//
// category is the category of the sheet being rendered. known reports
// whether vor has a sheet of its own of that name in that category; nil
// means the listing is not to be had, and then nothing is rewritten.
func rewriteSheetLinks(doc ast.Node, category string, known func(category, name string) bool) {
	if known == nil || !sheetPart(category) {
		return
	}
	var dead []*ast.Link
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		link, ok := n.(*ast.Link)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		dest, isSheetLink := sheetLink(string(link.Destination), category, known)
		switch {
		case dest != "":
			link.Destination = []byte(dest)
		case isSheetLink:
			dead = append(dead, link)
		}
		return ast.WalkContinue, nil
	})
	// After the walk: taking a node out of the tree under it would end it.
	for _, link := range dead {
		parent := link.Parent()
		if parent == nil {
			continue
		}
		for c := link.FirstChild(); c != nil; {
			next := c.NextSibling()
			link.RemoveChild(link, c)
			parent.InsertBefore(parent, link, c)
			c = next
		}
		parent.RemoveChild(parent, link)
	}
}

// sheetLink reads dest as a link from a sheet in category to another
// sheet. It returns the sheet's path on this server when vor has that
// sheet. isSheetLink is true when dest has the shape of a link between
// sheets, whether or not the sheet exists; with an empty path that is a
// link to remove.
func sheetLink(dest, category string, known func(category, name string) bool) (target string, isSheetLink bool) {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.RawQuery != "" || u.Path == "" ||
		strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, `\`) {
		return "", false
	}
	// u.Path is decoded, so an escaped ".." is cleaned like the ".." it is.
	p := strings.TrimSuffix(u.Path, ".md")
	// From the sheet's own directory first, then from the top.
	for _, full := range []string{path.Join(category, p), path.Clean(p)} {
		c, name, ok := strings.Cut(full, "/")
		if !ok || c == ".." || !sheetPart(c) || !sheetPart(name) {
			continue
		}
		isSheetLink = true
		if !known(c, name) {
			continue
		}
		out, ok := sheetURL(c, name)
		if !ok {
			continue
		}
		if u.Fragment != "" {
			out += "#" + u.EscapedFragment()
		}
		return out, true
	}
	return "", isSheetLink
}

// knownSheets returns the test rewriteSheetLinks needs, from the listing
// behind /sheets, or nil when vor does not give the listing. A sheet page
// is never held up or refused for want of it.
func (ws *WikiServer) knownSheets(ctx context.Context) func(category, name string) bool {
	page, err := ws.sheetsPage(ctx)
	if err != nil {
		return nil
	}
	return func(category, name string) bool { return page.known[sheetKey{category, name}] }
}
