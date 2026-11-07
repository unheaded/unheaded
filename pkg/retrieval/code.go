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

// Retrieve returns the k best chunks. A shard holds source, which is "local". A
// corpus built into its directory by mistake keeps the trust of a corpus.
func (c *CodeBackend) Retrieve(_ context.Context, query string, k int) ([]champion.Reference, []agent.TopicContent, error) {
	return retrieveShard(c.store, "code index", query, k, func(ch codeindex.Chunk) champion.Reference {
		trust := "local"
		if codeindex.IsCorpusText(ch) {
			trust = codeindex.CorpusTrust(ch)
		}
		return champion.Reference{
			Topic: ch.ID, Category: codeCategory,
			SourceKind: "user-custom", SourceTrust: trust,
			SourcePath: ch.Path, SourceLabel: codeSourceLabel,
		}
	})
}

// retrieveShard searches one index. A missing index is no hits, not an error.
// Test chunks give way while any real chunk matched, and a repeated ID drops,
// since two texts under one topic would fail the agent's chain check. Both
// happen before the cut, so neither uses up k. Past k an untrusted hit stays as
// a reference with no text: the model never reads it and the gate always does.
func retrieveShard(store *codeindex.Store, what, query string, k int, ref func(codeindex.Chunk) champion.Reference) ([]champion.Reference, []agent.TopicContent, error) {
	ix, err := store.Get()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", what, err)
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
		if (realHit && h.Chunk.IsTest) || seen[h.Chunk.ID] {
			continue
		}
		seen[h.Chunk.ID] = true
		r := ref(h.Chunk)
		if len(contents) >= k {
			if !champion.TrustedSourceLabels[r.SourceTrust] {
				refs = append(refs, r)
			}
			continue
		}
		refs = append(refs, r)
		contents = append(contents, agent.TopicContent{Ref: r, Content: h.Chunk.Content, Score: h.Score})
	}
	return refs, contents, nil
}
