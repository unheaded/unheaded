// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package retrieval

import (
	"context"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
	"unheaded/pkg/codeindex"
)

// How a Grimoire chunk reaches the gate (ADR-057, DP-719).
const (
	corpusBackendName = "grimoire"
	corpusSourceLabel = "aux_grimoire"
	corpusCategory    = "grimoire"
)

// CorpusBackend answers from a text corpus built by codeindex.BuildCorpus: the
// Grimoire's threat models and MITRE ATT&CK data. It reads files and nothing
// else, so it works on an air-gapped host. Its Score is word overlap.
type CorpusBackend struct {
	store *codeindex.Store
}

// NewCorpusBackend returns a CorpusBackend over store.
func NewCorpusBackend(store *codeindex.Store) *CorpusBackend {
	return &CorpusBackend{store: store}
}

// Name labels the backend.
func (c *CorpusBackend) Name() string { return corpusBackendName }

// Retrieve returns the k best chunks for query's keywords, and past them every
// external one without text. codeindex.CorpusTrust decides which is which:
// "local" text is the project's, like the source shard; "external" text reaches
// the model and never justifies a write.
func (c *CorpusBackend) Retrieve(_ context.Context, query string, k int) ([]champion.Reference, []agent.TopicContent, error) {
	return retrieveShard(c.store, "corpus index", codeindex.Keywords(query), k, func(ch codeindex.Chunk) champion.Reference {
		trust := codeindex.CorpusTrust(ch)
		kind := "user-custom"
		if !champion.TrustedSourceLabels[trust] {
			kind = "user-source"
		}
		return champion.Reference{
			Topic: ch.ID, Category: corpusCategory,
			SourceKind: kind, SourceTrust: trust,
			SourcePath: ch.Path, SourceLabel: corpusSourceLabel,
		}
	})
}
