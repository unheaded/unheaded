// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package retrieval

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
	"unheaded/pkg/codeindex"
)

// How a shard chunk reaches the gate (ADR-057).
const (
	codeBackendName = "code"
	codeSourceLabel = "aux_unheaded_code"
	codeCategory    = "code"
)

// CodeBackend answers from the source shard. Its Score is word overlap.
type CodeBackend struct {
	store *codeindex.Store
}

// NewCodeBackend returns a CodeBackend over store.
func NewCodeBackend(store *codeindex.Store) *CodeBackend {
	return &CodeBackend{store: store}
}

// Name labels the backend.
func (c *CodeBackend) Name() string { return codeBackendName }

// Retrieve returns the k best chunks. A missing index is no hits, not an
// error. Test chunks give way while any real chunk matched, and a repeated ID
// drops, since two texts under one topic would fail the agent's chain check.
// Both happen before the cut, so neither uses up k.
func (c *CodeBackend) Retrieve(_ context.Context, query string, k int) ([]champion.Reference, []agent.TopicContent, error) {
	ix, err := c.store.Get()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("code index: %w", err)
	}
	hits := ix.Search(query, len(ix.Chunks))
	realHit := false
	for _, h := range hits {
		if !h.Chunk.IsTest {
			realHit = true
			break
		}
	}
	seen := make(map[string]bool, len(hits))
	var (
		refs     []champion.Reference
		contents []agent.TopicContent
	)
	for _, h := range hits {
		if len(refs) >= k {
			break
		}
		if (realHit && h.Chunk.IsTest) || seen[h.Chunk.ID] {
			continue
		}
		seen[h.Chunk.ID] = true
		ref := champion.Reference{
			Topic: h.Chunk.ID, Category: codeCategory,
			SourceKind: "user-custom", SourceTrust: "local",
			SourcePath: h.Chunk.Path, SourceLabel: codeSourceLabel,
		}
		refs = append(refs, ref)
		contents = append(contents, agent.TopicContent{Ref: ref, Content: h.Chunk.Content, Score: h.Score})
	}
	return refs, contents, nil
}
