// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package retrieval fans one query out to several corpora under a deadline
// and merges what answered. The same merge serves seed retrieval and the
// agent's per-turn justification check, so what it drops, Rule 2 never sees:
// the cut at k limits what the model reads, never what the gate reads.
//
// Trust ranks first (canonical 3, local 2, anything else 1), then Score,
// stable in backend order. Scores are not comparable across backends: vor has
// no similarity and reports 1/(rank+1), the code shard reports word overlap.
// They order hits of one trust and nothing more, which is why each backend
// gets its share of k before rank fills the rest. There is no reranker
// (DP-313).
package retrieval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
)

// Outcomes New's callback reports, once per backend per call.
const (
	OutcomeOK      = "ok"
	OutcomeTimeout = "timeout"
	OutcomeError   = "error"
)

// Backend is one corpus. Name labels its metric and its Unavailable reference.
type Backend interface {
	agent.Retriever
	Name() string
}

// Federated is an agent.Retriever over Backends.
type Federated struct {
	backends  []Backend
	deadline  time.Duration
	onOutcome func(backend, outcome string)
}

// New returns a Federated. A deadline of 0 or less leaves only the caller's
// context; onOutcome may be nil, and is called from Retrieve's goroutine.
func New(backends []Backend, deadline time.Duration, onOutcome func(backend, outcome string)) *Federated {
	return &Federated{backends: backends, deadline: deadline, onOutcome: onOutcome}
}

// answer is what one backend gave. err set means it did not answer.
type answer struct {
	refs     []champion.Reference
	contents []agent.TopicContent
	err      error
}

// Retrieve asks every backend at once and waits no longer than the deadline.
// A backend that fails or runs out of time is skipped and leaves a
// champion.Unavailable reference past k, so its silence still holds a write.
// When none answers, the first error in backend order returns. With one
// backend configured its answer passes through untouched.
func (f *Federated) Retrieve(ctx context.Context, query string, k int) ([]champion.Reference, []agent.TopicContent, error) {
	if len(f.backends) == 0 {
		return nil, nil, errors.New("retrieval: no backends")
	}
	all := f.gather(ctx, query, k)
	if len(all) == 1 {
		return all[0].refs, all[0].contents, all[0].err
	}
	var (
		answered []answer
		silent   []string
		firstErr error
	)
	for i, a := range all {
		if a.err == nil {
			answered = append(answered, a)
			continue
		}
		silent = append(silent, f.backends[i].Name())
		if firstErr == nil {
			firstErr = a.err
		}
	}
	if len(answered) == 0 {
		return nil, nil, firstErr
	}
	refs, contents := merge(answered, k)
	for _, name := range silent {
		refs = append(refs, champion.Unavailable(name))
	}
	return refs, contents, nil
}

// landed is one backend's return. late holds ctx.Err() as it returned: a
// backend that skips what it cannot fetch comes back at the deadline with
// part of an answer and a nil error, and the receiving select would keep or
// lose that at random.
type landed struct {
	i    int
	a    answer
	late error
}

func (f *Federated) gather(parent context.Context, query string, k int) []answer {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if f.deadline > 0 {
		ctx, cancel = context.WithTimeout(parent, f.deadline)
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	defer cancel()

	// Buffered: a backend that outlives the call must not block on its send.
	ch := make(chan landed, len(f.backends))
	for i, b := range f.backends {
		go func() {
			l := landed{i: i}
			defer func() {
				if p := recover(); p != nil {
					l.a = answer{err: fmt.Errorf("retrieval: backend %s panicked: %v", b.Name(), p)}
				}
				l.late = ctx.Err()
				ch <- l
			}()
			l.a.refs, l.a.contents, l.a.err = b.Retrieve(ctx, query, k)
		}()
	}

	out := make([]answer, len(f.backends))
	settled := make([]bool, len(f.backends))
	settle := func(l landed) {
		settled[l.i] = true
		if l.late != nil {
			out[l.i] = f.silent(l.i, l.late)
			return
		}
		out[l.i] = l.a
		if l.a.err != nil {
			f.report(l.i, OutcomeError)
			return
		}
		f.report(l.i, OutcomeOK)
	}
	for range f.backends {
		select {
		case l := <-ch:
			settle(l)
		case <-ctx.Done():
			// select picks at random when both are ready: take what landed
			// in time before calling the rest silent.
			for len(ch) > 0 {
				settle(<-ch)
			}
			for i := range out {
				if !settled[i] {
					out[i] = f.silent(i, ctx.Err())
				}
			}
			return out
		}
	}
	return out
}

// silent records a backend that ran out of time, or whose caller gave up.
func (f *Federated) silent(i int, cause error) answer {
	outcome := OutcomeError
	if errors.Is(cause, context.DeadlineExceeded) {
		outcome = OutcomeTimeout
	}
	f.report(i, outcome)
	return answer{err: fmt.Errorf("retrieval: backend %s: %w", f.backends[i].Name(), cause)}
}

func (f *Federated) report(i int, outcome string) {
	if f.onOutcome != nil {
		f.onOutcome(f.backends[i].Name(), outcome)
	}
}

// entry is one reference on its way through merge.
type entry struct {
	ref     champion.Reference
	text    string
	hasText bool
	score   float64
	backend int
	drop    bool // a trusted duplicate
	refOnly bool // the gate reads it, the model does not: it takes no slot
	picked  bool
}

func trustRank(trust string) int {
	switch trust {
	case "canonical":
		return 3
	case "local":
		return 2
	}
	return 1
}

// merge ranks, dedupes by (Topic, Category), gives each answering backend up
// to ceil(k/n) slots and fills the rest in rank order up to k. A picked entry
// keeps its text. Everything untrusted that was not picked follows without
// text, so an untrusted reference survives every cut: dedupe may drop a
// trusted copy, never an untrusted one. Text with no reference is dropped.
func merge(answered []answer, k int) ([]champion.Reference, []agent.TopicContent) {
	var es []entry
	for b, a := range answered {
		// Keyed without Digest and Excerpt, which say nothing of identity.
		text := make(map[champion.Reference]agent.TopicContent, len(a.contents))
		for _, c := range a.contents {
			key := bareRef(c.Ref)
			if _, dup := text[key]; !dup {
				text[key] = c
			}
		}
		for _, r := range a.refs {
			c, ok := text[bareRef(r)]
			es = append(es, entry{ref: r, text: c.Content, hasText: ok, score: c.Score, backend: b})
		}
	}
	sort.SliceStable(es, func(i, j int) bool {
		if ri, rj := trustRank(es[i].ref.SourceTrust), trustRank(es[j].ref.SourceTrust); ri != rj {
			return ri > rj
		}
		return es[i].score > es[j].score
	})

	seen := make(map[[2]string]bool, len(es))
	for i := range es {
		e := &es[i]
		trusted := champion.TrustedSourceLabels[e.ref.SourceTrust]
		key := [2]string{e.ref.Topic, e.ref.Category}
		dup := seen[key]
		seen[key] = true
		switch {
		case dup && trusted:
			e.drop = true
		case dup, !trusted && !e.hasText:
			e.refOnly = true
		}
	}

	share := 0
	if k > 0 {
		share = (k + len(answered) - 1) / len(answered)
	}
	taken := make([]int, len(answered))
	total := 0
	pick := func(fits func(e *entry) bool) {
		for i := range es {
			e := &es[i]
			if total >= k {
				return
			}
			if e.drop || e.refOnly || e.picked || !fits(e) {
				continue
			}
			e.picked = true
			taken[e.backend]++
			total++
		}
	}
	pick(func(e *entry) bool { return taken[e.backend] < share })
	pick(func(*entry) bool { return true })

	var (
		refs     []champion.Reference
		contents []agent.TopicContent
	)
	for i := range es {
		e := &es[i]
		if !e.picked {
			continue
		}
		refs = append(refs, e.ref)
		if e.hasText {
			contents = append(contents, agent.TopicContent{Ref: e.ref, Content: e.text, Score: e.score})
		}
	}
	for i := range es {
		e := &es[i]
		if !e.picked && !e.drop && !champion.TrustedSourceLabels[e.ref.SourceTrust] {
			refs = append(refs, e.ref)
		}
	}
	return refs, contents
}

func bareRef(r champion.Reference) champion.Reference {
	r.Digest, r.Excerpt = "", ""
	return r
}
