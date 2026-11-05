// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package codeindex

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var shaRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

func gitOut(ctx context.Context, repo string, args ...string) ([]byte, error) {
	// #nosec G204 -- repo is the operator's -repo; the other arguments are constants plus validated hex shas
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Refresh indexes the committed tree at HEAD of repo into dir, parsing only
// blobs that changed since the saved index.
func Refresh(ctx context.Context, repo, dir string) (Stats, error) {
	var st Stats
	head, err := gitOut(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return st, err
	}
	tree, err := gitOut(ctx, repo, "ls-tree", "-r", "-z", "HEAD")
	if err != nil {
		return st, err
	}
	old, err := Load(dir)
	if err != nil {
		old = &Index{}
	}
	oldChunks := map[string][]Chunk{}
	for _, c := range old.Chunks {
		oldChunks[c.Path] = append(oldChunks[c.Path], c)
	}
	oldCalls := map[string][]Call{}
	for _, c := range old.Calls {
		oldCalls[c.Path] = append(oldCalls[c.Path], c)
	}
	ix := &Index{Head: strings.TrimSpace(string(head)), Files: map[string]string{}}
	for _, ent := range strings.Split(string(tree), "\x00") {
		tab := strings.IndexByte(ent, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(ent[:tab])
		path := ent[tab+1:]
		if len(meta) != 3 || meta[1] != "blob" || !Included(path) || !shaRe.MatchString(meta[2]) {
			continue
		}
		sha := meta[2]
		ix.Files[path] = sha
		if old.Files[path] == sha {
			st.Reused++
			ix.Chunks = append(ix.Chunks, oldChunks[path]...)
			ix.Calls = append(ix.Calls, oldCalls[path]...)
			continue
		}
		src, err := gitOut(ctx, repo, "cat-file", "blob", sha)
		if err != nil {
			return st, err
		}
		st.Parsed++
		switch {
		case strings.HasSuffix(path, ".go"):
			c, k, err := ChunkGo(path, src, sha)
			if err == nil {
				ix.Chunks = append(ix.Chunks, c...)
				ix.Calls = append(ix.Calls, k...)
			}
		case strings.HasSuffix(path, ".rs"):
			ix.Chunks = append(ix.Chunks, ChunkRust(path, src, sha)...)
		case strings.HasSuffix(path, ".py"):
			ix.Chunks = append(ix.Chunks, ChunkPython(path, src, sha)...)
		}
	}
	for p := range old.Files {
		if _, ok := ix.Files[p]; !ok {
			st.Deleted++
		}
	}
	st.Files, st.Chunks, st.Calls = len(ix.Files), len(ix.Chunks), len(ix.Calls)
	if err := Save(dir, ix); err != nil {
		return st, err
	}
	return st, nil
}
