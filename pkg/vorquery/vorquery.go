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
//
// Two more, from the 2026-11-11 test ("getting the ccna" answered from the
// Kerberos sheet; a JNCIA question found nothing though vor has the sheets):
//
//   - A substring is not a word: "ccna" sits inside KRB5CCNAME. Covers counts
//     the query words a topic's text carries at the start of a word, and a
//     topic that does not carry every one is not a hit.
//   - An AND of every word of a long question matches no sheet. When the full
//     query yields nothing, Select searches word by word and keeps only topics
//     that a query word names (a token of the name or category) and that carry
//     two of the words. Counting words alone is not enough: for a JNCIA
//     question the JNCIA sheets carried two of five words and unrelated docs
//     carried three.
//
// A question with no topic that passes has no source. That is the answer:
// the model is then told nothing, and the page says so.
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
	"unicode"
	"unicode/utf8"
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

// Words returns the distinct words of terms the way vor splits them: on
// anything that is not a letter or a digit, so "jncia-junos" is "jncia" and
// "junos". Order is kept.
func Words(terms []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, term := range terms {
		for _, w := range strings.FieldsFunc(strings.ToLower(term), notWordRune) {
			if _, dup := seen[w]; dup {
				continue
			}
			seen[w] = struct{}{}
			out = append(out, w)
		}
	}
	return out
}

func notWordRune(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// StartsWord reports whether word occurs in text at the start of a word:
// "kube" starts a word of "kubernetes", "ccna" does not in "krb5ccname".
// Both are expected in lower case.
func StartsWord(text, word string) bool {
	if word == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(text[from:], word)
		if i < 0 {
			return false
		}
		i += from
		if i == 0 {
			return true
		}
		if r, _ := utf8.DecodeLastRuneInString(text[:i]); notWordRune(r) {
			return true
		}
		from = i + 1
	}
}

// Covers returns how many of words start a word of text (any case).
func Covers(text string, words []string) int {
	text = strings.ToLower(text)
	n := 0
	for _, w := range words {
		if StartsWord(text, w) {
			n++
		}
	}
	return n
}

// MinFallbackWords is the fewest query words for which a word-by-word
// search is tried after the full query found nothing.
const MinFallbackWords = 2

// MaxFallbackWords caps the searches one question can cost.
const MaxFallbackWords = 8

// MinFallbackCover is how many query words a topic found word by word must
// carry at a word start. One shared word is a coincidence.
const MinFallbackCover = 2

// MaxFetches caps the topics fetched and checked in each of the two passes.
const MaxFetches = 12

// Candidates orders the topics of word-by-word searches: perWord[i] is the
// hits for one query word, in vor's order. Topics hit by more words come
// first, then the order they were first seen. Worktree copies are dropped.
func Candidates(perWord [][]Topic) []string {
	count := make(map[string]int)
	var first []string
	for _, hits := range perWord {
		inThis := make(map[string]struct{})
		for _, h := range hits {
			if h.Name == "" || IsWorktreeCopy(h.Category) {
				continue
			}
			if _, dup := inThis[h.Name]; dup {
				continue
			}
			inThis[h.Name] = struct{}{}
			if count[h.Name] == 0 {
				first = append(first, h.Name)
			}
			count[h.Name]++
		}
	}
	sort.SliceStable(first, func(i, j int) bool { return count[first[i]] > count[first[j]] })
	return first
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

var anchorTokenRE = regexp.MustCompile(`[-_.\s/]+`)

// Anchored finds the topics a query word names: a word equal to a token of
// the topic's name or category ("junos" names junos-interfaces, and every
// sheet filed under a junos category). It returns how many words name each
// topic and the names in listing order. Worktree copies are skipped.
func Anchored(topics []Topic, words []string) (map[string]int, []string) {
	want := make(map[string]struct{}, len(words))
	for _, w := range words {
		want[w] = struct{}{}
	}
	count := make(map[string]int)
	var names []string
	for _, tp := range topics {
		if tp.Name == "" || IsWorktreeCopy(tp.Category) {
			continue
		}
		hit := make(map[string]struct{})
		for _, t := range anchorTokenRE.Split(strings.ToLower(tp.Name+" "+tp.Category), -1) {
			if _, ok := want[t]; ok {
				hit[t] = struct{}{}
			}
		}
		if len(hit) == 0 {
			continue
		}
		if _, dup := count[tp.Name]; !dup {
			names = append(names, tp.Name)
		}
		if len(hit) > count[tp.Name] {
			count[tp.Name] = len(hit)
		}
	}
	return count, names
}

// Text is what Covers is run over for a fetched topic.
func Text(name, category, title, content string) string {
	return name + " " + category + " " + title + " " + content
}

// SearchFunc runs one vor /api/search and returns its hits in vor's order.
type SearchFunc func(ctx context.Context, q string) ([]Topic, error)

// FetchFunc fetches one topic and returns its Text; ok is false when the
// fetch failed.
type FetchFunc func(ctx context.Context, name string) (text string, ok bool)

// Select returns the names of at most k topics that answer terms, best
// first. It may return none.
//
// First pass: one search for all the terms, the hits interleaved with the
// topics named for them (Order), and a topic is kept only when its text
// carries every query word at a word start. Second pass, when the first kept
// nothing and there are at least MinFallbackWords words: one search per word,
// then only topics a query word names (Anchored), hit by more words first,
// then named topics no search returned; a topic is kept when it carries
// MinFallbackCover of the words, most words first.
//
// Each pass fetches at most MaxFetches topics; no topic is fetched twice. A
// failed first search is the error. A failed word search skips the word. A
// failed topic listing is returned as warn for the caller to log: the first
// pass then runs on vor's order alone and the second keeps nothing.
//
// Select makes every search through the caller's search, so the caller sees
// every hit of every search and can hand the untrusted ones to its gate.
func (x *Index) Select(ctx context.Context, terms []string, k int, search SearchFunc, fetch FetchFunc) (names []string, warn, err error) {
	hits, err := search(ctx, strings.Join(terms, " "))
	if err != nil {
		return nil, nil, err
	}
	order, warn := x.Order(ctx, terms, hits)
	words := Words(terms)

	type fetched struct {
		text string
		ok   bool
	}
	memo := make(map[string]fetched)
	get := func(name string) (string, bool) {
		f, done := memo[name]
		if !done {
			f.text, f.ok = fetch(ctx, name)
			memo[name] = f
		}
		return f.text, f.ok
	}

	seen := make(map[string]struct{})
	for _, name := range order {
		if _, dup := seen[name]; dup || name == "" {
			continue
		}
		if len(seen) == MaxFetches {
			break
		}
		seen[name] = struct{}{}
		text, ok := get(name)
		if !ok || Covers(text, words) < len(words) {
			continue
		}
		names = append(names, name)
		if len(names) >= k {
			return names, warn, nil
		}
	}
	if len(names) > 0 || len(words) < MinFallbackWords {
		return names, warn, nil
	}

	if len(words) > MaxFallbackWords {
		words = words[:MaxFallbackWords]
	}
	var perWord [][]Topic
	for _, w := range words {
		h, err := search(ctx, w)
		if err != nil {
			continue
		}
		perWord = append(perWord, h)
	}
	topics, err := x.listing(ctx)
	if err != nil {
		return nil, err, nil
	}
	anchors, listed := Anchored(topics, words)
	var cand []string
	searched := make(map[string]struct{})
	for _, name := range Candidates(perWord) {
		searched[name] = struct{}{}
		if anchors[name] > 0 {
			cand = append(cand, name)
		}
	}
	var rest []string
	for _, name := range listed {
		if _, was := searched[name]; !was {
			rest = append(rest, name)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return anchors[rest[i]] > anchors[rest[j]] })
	cand = append(cand, rest...)
	if len(cand) > MaxFetches {
		cand = cand[:MaxFetches]
	}

	cover := make(map[string]int)
	for _, name := range cand {
		text, ok := get(name)
		if !ok {
			continue
		}
		if c := Covers(text, words); c >= MinFallbackCover {
			cover[name] = c
			names = append(names, name)
		}
	}
	sort.SliceStable(names, func(i, j int) bool { return cover[names[i]] > cover[names[j]] })
	if len(names) > k {
		names = names[:k]
	}
	return names, warn, nil
}
