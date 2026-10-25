// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package vorquery shapes a Zhen question into a vor search and orders the
// topics that come back. It is the Go twin of retrieve() in
// raft/scripts/zhen_rag.py (2026-10-25, "what is wotan" regression); the
// stop-word list is kept identical and a test compares the two.
//
// vor's /api/search is an AND of case-insensitive substring matches, ranked
// first by how many query terms equal a token of the topic's name, and capped
// at 50 lines. Two consequences:
//
//   - Question words cost ranking and filter nothing ("is" is a substring of
//     almost every doc; "what is wotan" ranked the what-is-unheaded page
//     first). SearchTerms drops them.
//   - One long doc with dozens of matching section titles can fill all 50
//     lines, so the topics named for the subject (Service-Wotan,
//     Wotan-Memory-Model) never come back. Order interleaves the topics whose
//     name carries every query term with vor's hits.
package vorquery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// stopWords are dropped from a question before it goes to vor. Words that
// carry meaning in a code question (if, for, while, not, with, else) are
// deliberately absent. Must match _QUERY_STOPWORDS in raft/scripts/zhen_rag.py.
var stopWords = toSet(`
    a an the and or of to in on at by from as into about
    is are was were be been being am
    do does did done can could should would will shall may might must
    what whats what's who whom whose which where when why how
    i me my you your we us our it its it's this that these those there
    please tell explain describe define mean means meaning
`)

// termPunct is trimmed from both ends of each word (Python's _TERM_PUNCT).
const termPunct = "?!.,;:\"'`()[]{}"

// worktreeCategoryPrefix marks topics vor indexes under an agent worktree
// (.claude/worktrees/<id>/...): stale copies of the main tree, never
// promoted by name.
const worktreeCategoryPrefix = ".claude/"

// DefaultTopicTTL is how long Index keeps vor's topic listing (~2.5 MB for
// ~3.5k topics). New docs show up within it.
const DefaultTopicTTL = 5 * time.Minute

var nameTokenRE = regexp.MustCompile(`[-_.\s]+`)

func toSet(words string) map[string]struct{} {
	m := make(map[string]struct{})
	for _, w := range strings.Fields(words) {
		m[w] = struct{}{}
	}
	return m
}

// SearchTerms returns the lower-cased words of q with question and filler
// words removed. When nothing else is left ("what is this") it returns every
// word, so a query is never emptied.
func SearchTerms(q string) []string {
	var words, kept []string
	for _, w := range strings.Fields(strings.ToLower(q)) {
		w = strings.Trim(w, termPunct)
		if w == "" {
			continue
		}
		words = append(words, w)
		if _, stop := stopWords[w]; !stop {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		return words
	}
	return kept
}

// NameTokens splits a vor topic name the way its words read: Service-Wotan,
// WOTAN_TOPICS and draft-...-wotan-memory-03 all yield "wotan".
func NameTokens(name string) []string {
	var out []string
	for _, t := range nameTokenRE.Split(strings.ToLower(name), -1) {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Topic is one row of vor's GET /api/topics listing.
type Topic struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

// NamedBy returns the names in topics whose tokens include every term,
// fewest name tokens first (the name is mostly the query: Service-Wotan
// before ADR-023-wotan-virtual-memory), then by name. Worktree copies are
// skipped.
func NamedBy(topics []Topic, terms []string) []string {
	wanted := make(map[string]struct{})
	for _, term := range terms {
		for _, t := range NameTokens(term) {
			wanted[t] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	best := make(map[string]int)
	for _, tp := range topics {
		if tp.Name == "" || IsWorktreeCopy(tp.Category) {
			continue
		}
		tokens := NameTokens(tp.Name)
		have := make(map[string]struct{}, len(tokens))
		for _, t := range tokens {
			have[t] = struct{}{}
		}
		all := true
		for w := range wanted {
			if _, ok := have[w]; !ok {
				all = false
				break
			}
		}
		if all {
			best[tp.Name] = len(tokens)
		}
	}
	names := make([]string, 0, len(best))
	for n := range best {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if best[names[i]] != best[names[j]] {
			return best[names[i]] < best[names[j]]
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names
}

// IsWorktreeCopy reports whether a topic in category is vor's copy of a doc
// under an agent worktree. Such copies are skipped wherever they come from:
// they are stale, and fetching by name can return them in place of the
// main tree's doc of the same name.
func IsWorktreeCopy(category string) bool {
	return strings.HasPrefix(category, worktreeCategoryPrefix)
}

// Interleave returns named[0], searched[0], named[1], searched[1], ...
// Duplicates are left for the caller's existing seen-set to drop.
func Interleave(named, searched []string) []string {
	out := make([]string, 0, len(named)+len(searched))
	for i := 0; i < len(named) || i < len(searched); i++ {
		if i < len(named) {
			out = append(out, named[i])
		}
		if i < len(searched) {
			out = append(out, searched[i])
		}
	}
	return out
}

// Index caches vor's topic listing for NamedBy. The zero value is not
// usable; use NewIndex. Safe for concurrent use.
type Index struct {
	client  *http.Client
	baseURL string
	ttl     time.Duration

	mu     sync.Mutex
	topics []Topic
	at     time.Time
}

// NewIndex returns an Index reading baseURL's /api/topics through client.
func NewIndex(client *http.Client, baseURL string) *Index {
	return &Index{client: client, baseURL: strings.TrimRight(baseURL, "/"), ttl: DefaultTopicTTL}
}

// Named returns the topic names that carry every term (see NamedBy). On a
// listing error it returns the error and no names, which leaves the caller
// on vor's search order.
func (x *Index) Named(ctx context.Context, terms []string) ([]string, error) {
	topics, err := x.listing(ctx)
	if err != nil {
		return nil, err
	}
	return NamedBy(topics, terms), nil
}

func (x *Index) listing(ctx context.Context) ([]Topic, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.topics != nil && time.Since(x.at) <= x.ttl {
		return x.topics, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, x.baseURL+"/api/topics", nil)
	if err != nil {
		return nil, err
	}
	resp, err := x.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("vor topics: status %d: %s", resp.StatusCode, body)
	}
	var topics []Topic
	if err := json.NewDecoder(resp.Body).Decode(&topics); err != nil {
		return nil, fmt.Errorf("vor topics: decode: %w", err)
	}
	x.topics, x.at = topics, time.Now()
	return topics, nil
}

// Query is the string to send to vor's /api/search for question q.
func Query(q string) string { return strings.Join(SearchTerms(q), " ") }

// Order returns the topic names to fetch for terms: the topics named by
// every term interleaved with hits (vor's search hits, in order), worktree
// copies dropped. When the listing fails it returns the hits alone with the
// error, for the caller to log.
func (x *Index) Order(ctx context.Context, terms []string, hits []Topic) ([]string, error) {
	searched := make([]string, 0, len(hits))
	for _, h := range hits {
		if !IsWorktreeCopy(h.Category) {
			searched = append(searched, h.Name)
		}
	}
	named, err := x.Named(ctx, terms)
	return Interleave(named, searched), err
}
