// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// repoBlobURL is where a link from a wiki page to a file or directory of the
// repository is sent. GitHub redirects blob/ to tree/ for a directory.
const repoBlobURL = "https://github.com/unheaded/unheaded/blob/develop/"

// wikiDirName is the wiki's directory in the repository: what a relative
// link in a wiki page is relative to.
const wikiDirName = "wiki"

// rewriteRepoLinks points the links that leave the wiki directory for the
// rest of the repository at GitHub: "../cmd/sword/main.go", as the Source
// footers have. The pages are served from /wiki/, so such a link would
// otherwise name a path on this server that does not exist. The files keep
// their relative links, which is what GitHub's own wiki needs.
func rewriteRepoLinks(doc ast.Node) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if link, ok := n.(*ast.Link); ok && entering {
			if dest, ok := repoLink(string(link.Destination)); ok {
				link.Destination = []byte(dest)
			}
		}
		return ast.WalkContinue, nil
	})
}

// repoLink returns the GitHub URL for dest when dest is a relative link that
// resolves, from the wiki directory, to a path inside the repository and
// outside wiki/. Absolute URLs, in-page anchors, links to other wiki pages
// and paths that climb out of the repository are left as they are.
func repoLink(dest string) (string, bool) {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" ||
		strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, `\`) {
		return "", false
	}
	// u.Path is decoded, so "%2e%2e" is cleaned like the ".." it is, and the
	// URL is built from the cleaned path, never from the text of the link.
	p := path.Join(wikiDirName, u.Path)
	if p == "." || p == ".." || strings.HasPrefix(p, "../") ||
		p == wikiDirName || strings.HasPrefix(p, wikiDirName+"/") {
		return "", false
	}
	out := repoBlobURL + (&url.URL{Path: p}).EscapedPath()
	if u.RawQuery != "" {
		out += "?" + u.Query().Encode()
	}
	if u.Fragment != "" {
		out += "#" + u.EscapedFragment()
	}
	return out, true
}
