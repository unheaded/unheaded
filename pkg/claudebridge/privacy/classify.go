// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package privacy

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// repoTools return repository text only (the corpus and runbooks are
// built from tracked files).
var repoTools = map[string]bool{"corpus_search": true, "runbook_list": true, "runbook_show": true}

// fileTools read one file: T0 when it is tracked, else T1.
var fileTools = map[string]bool{"read_file": true, "file_read": true}

// denyPatterns mirror the Champion's default denied paths: a tracked file
// that matches one is still treated as ops data.
var denyPatterns = []string{"/etc/shadow", "/etc/passwd", "/.ssh/", "/id_rsa", "/id_ed25519", ".env", "credentials", "secrets"}

// Classifier assigns a tool result its tier. Tools it does not know are
// T1 (fail closed).
type Classifier struct {
	// UserDataRoots: anything read under these is T2 and never leaves.
	UserDataRoots []string
	// Tracked reports whether path (absolute, symlinks resolved) is
	// tracked at HEAD in the repository at root. Default: git ls-files.
	Tracked func(root, path string) bool
}

// Classify returns the tier of what tool returns for args.
func (c Classifier) Classify(tool string, args map[string]any, projectRoot string) Tier {
	path, _ := args["path"].(string)
	if path != "" {
		resolved := resolve(path)
		for _, ud := range c.UserDataRoots {
			if within(resolved, resolve(ud)) {
				return TierUserData
			}
		}
	}
	if repoTools[tool] {
		return TierRepo
	}
	if fileTools[tool] && path != "" && projectRoot != "" {
		resolved := resolve(path)
		root := resolve(projectRoot)
		if !within(resolved, root) {
			return TierOps
		}
		for _, d := range denyPatterns {
			if strings.Contains(resolved, d) {
				return TierOps
			}
		}
		tracked := c.Tracked
		if tracked == nil {
			tracked = gitTracked
		}
		if tracked(root, resolved) {
			return TierRepo
		}
	}
	return TierOps
}

func resolve(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return abs
}

func within(p, root string) bool {
	if root == "" {
		return false
	}
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))
}

// gitTracked asks git whether path is in the index at root (tracked files
// are the repository's public text).
func gitTracked(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// #nosec G204 -- fixed binary and flags; rel is passed after "--", no shell
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "--error-unmatch", "--", rel)
	return cmd.Run() == nil
}
