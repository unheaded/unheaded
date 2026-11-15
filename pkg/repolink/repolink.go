// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package repolink holds the one address of this repository's files on
// GitHub. A runbook link in an alert, a "Source" footer in the wiki and a
// search hit that names a file all point there, and they must agree.
package repolink

import (
	"net/url"
	"path"
	"strings"
)

// BlobBase is the address of a file of the develop branch, without the
// file's path. GitHub redirects blob/ to tree/ for a directory.
const BlobBase = "https://github.com/unheaded/unheaded/blob/develop/"

// Blob returns the GitHub address of the repository path p ("docs/x.md"),
// with the path escaped. It returns "" for an empty path and for any path
// with ".." or a NUL in it: such a path names nothing in the repository.
func Blob(p string) string {
	if strings.Contains(p, "..") || strings.ContainsRune(p, 0) {
		return ""
	}
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	if p == "" {
		return ""
	}
	return BlobBase + (&url.URL{Path: p}).EscapedPath()
}
