// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package vorclient is the read-only HTTP client for vor, the cheat-sheet
// server (http://127.0.0.1:20110 on a Kingdom host): search, topic fetch and
// the topic listing.
//
// Every call carries a deadline and reads a bounded response. Everything vor
// returns is untrusted text: the caller escapes it before showing it.
//
// Topic names. A vor of 2026-11-12 or later answers GET /api/topics/<name>
// with the topic of exactly that name (any case) or 404. A name is unique
// only within its category, so "?category=" picks the sheet of that name in
// that category (TopicIn); without it vor answers with one of them. Since
// vor of 2026-11-22 a category may be a directory ("docs/adr"). A vor
// from before that date ignores the category and answers an unknown name
// with its nearest match (200, another topic). Topic and TopicIn check the
// reply against what was asked, so both vors give ErrNotFound, never
// another topic.
//
// Search. vor matches substrings by default ("ccna" finds KRB5CCNAME).
// SearchOptions.Words asks for "match=word": a term must start a word. A
// vor from before 2026-11-12 ignores it.
//
// Every read of vor's search and topics by Go code in this repository goes
// through this package (the wiki server, zhen-agentd, zhen-agent, zhen-cli,
// zhen-mcp, zhen-rag, pkg/vorquery). Only liveness pings of /api/health are
// written where they are used.
package vorclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Timeout bounds one call, whatever deadline the caller's context has.
	Timeout = 5 * time.Second

	// MaxResponseBytes bounds the body of a search or a topic. vor's longest
	// topic is far below it.
	MaxResponseBytes = 4 << 20

	// MaxListingBytes bounds the body of the topic listing, which grows with
	// every directory vor is pointed at.
	MaxListingBytes = 16 << 20

	// TrustCanonical is the source_trust of the sheets built into vor. The
	// other values are "local" (the operator's own sheets) and "external"
	// (directories vor was pointed at, such as a repository's docs).
	TrustCanonical = "canonical"

	// MaxCategoryBytes is the longest ?category= vor takes, all its
	// segments and slashes together (vor's maxCategoryLength).
	MaxCategoryBytes = 256
)

// ErrNotFound means vor has no topic of the name asked for.
var ErrNotFound = errors.New("vor: topic not found")

// Hit is one row of GET /api/search: a line of a topic that matched. Several
// hits can share a topic, and a topic name is unique only within its category.
type Hit struct {
	Topic       string `json:"topic"`
	Category    string `json:"category"`
	Section     string `json:"section"`
	Line        string `json:"line"`
	SourceKind  string `json:"source_kind"`
	SourceTrust string `json:"source_trust"`
	SourcePath  string `json:"source_path"`
	SourceLabel string `json:"source_label"`
}

// Topic is the reply of GET /api/topics/<name>. Content is Markdown.
type Topic struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	SeeAlso     []string `json:"see_also"`
	HasDetail   bool     `json:"has_detail"`
	SourceKind  string   `json:"source_kind"`
	SourceTrust string   `json:"source_trust"`
	SourcePath  string   `json:"source_path"`
	SourceLabel string   `json:"source_label"`
}

// TopicSummary is one row of GET /api/topics: a topic without its content.
// A vor from before 2026-11-11 leaves SourceKind and SourceTrust empty.
type TopicSummary struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	HasDetail   bool     `json:"has_detail"`
	SeeAlso     []string `json:"see_also"`
	SourceKind  string   `json:"source_kind"`
	SourceTrust string   `json:"source_trust"`
}

// SearchOptions narrows GET /api/search. Zero values leave vor's defaults:
// every category, 50 lines, no cap per topic, substring matching. A vor from
// before 2026-11-11 ignores all of them, so a caller that depends on Category
// checks each hit.
type SearchOptions struct {
	// Category keeps only topics in exactly this category.
	Category string
	// Limit is the most lines returned, 1 to 500.
	Limit int
	// PerTopic is the most lines from any one topic, 1 to 50.
	PerTopic int
	// Words keeps a topic only when every term starts a word in it
	// (match=word). False is vor's default, substring matching.
	Words bool
}

func (o SearchOptions) query(q string) string {
	v := url.Values{"q": {q}}
	if o.Category != "" {
		v.Set("category", o.Category)
	}
	if o.Limit > 0 {
		v.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.PerTopic > 0 {
		v.Set("per_topic", strconv.Itoa(o.PerTopic))
	}
	if o.Words {
		v.Set("match", "word")
	}
	return v.Encode()
}

// Client talks to one vor. Safe for concurrent use.
type Client struct {
	base string
	http *http.Client
}

// New returns a Client for the vor at baseURL, such as
// "http://127.0.0.1:20110". A nil hc means a client of its own.
func New(baseURL string, hc *http.Client) (*Client, error) {
	base := strings.TrimRight(baseURL, "/")
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("vor URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("vor URL %q: want http://host:port", baseURL)
	}
	if hc == nil {
		hc = &http.Client{Timeout: Timeout}
	}
	return &Client{base: base, http: hc}, nil
}

// Search returns vor's hits for q, in vor's order. No hits is an empty
// result, not an error.
func (c *Client) Search(ctx context.Context, q string, opt SearchOptions) ([]Hit, error) {
	var hits []Hit
	if _, err := c.get(ctx, "/api/search?"+opt.query(q), MaxResponseBytes, &hits); err != nil {
		return nil, err
	}
	return hits, nil
}

// Topics returns every topic vor has, from every source, in vor's order.
func (c *Client) Topics(ctx context.Context) ([]TopicSummary, error) {
	var topics []TopicSummary
	if _, err := c.get(ctx, "/api/topics", MaxListingBytes, &topics); err != nil {
		return nil, err
	}
	return topics, nil
}

// Topic returns the topic called name, or ErrNotFound.
//
// vor answers an unknown name with its nearest match (200, another topic),
// and matches names without regard to case. A reply whose name is not exactly
// the one asked for is therefore ErrNotFound, never another topic.
func (c *Client) Topic(ctx context.Context, name string) (*Topic, error) {
	if !plainName(name) {
		return nil, ErrNotFound
	}
	var t Topic
	status, err := c.get(ctx, "/api/topics/"+url.PathEscape(name), MaxResponseBytes, &t)
	if status == http.StatusNotFound || status == http.StatusBadRequest {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if t.Name != name {
		return nil, ErrNotFound
	}
	return &t, nil
}

// TopicIn returns the topic called name in exactly that category, or
// ErrNotFound. It is the only way to reach both sheets when two categories
// hold the same name. The name matches in any case, as vor matches it; the
// reply carries the name as stored. A category may be a directory
// ("docs/adr"), under the rule vor applies (see validCategory).
//
// An empty category is an error, not a request: use Topic. A reply from
// another category or of another name (a vor from before 2026-11-12) is
// ErrNotFound, and so is a directory category to a vor from before
// 2026-11-22, which refuses it.
func (c *Client) TopicIn(ctx context.Context, category, name string) (*Topic, error) {
	t, _, err := c.topicIn(ctx, category, name)
	return t, err
}

// topicIn is TopicIn, and also returns vor's status (0 when no request
// was sent).
func (c *Client) topicIn(ctx context.Context, category, name string) (*Topic, int, error) {
	if category == "" {
		return nil, 0, errors.New("vor: TopicIn needs a category")
	}
	if !plainName(name) || !validCategory(category) {
		return nil, 0, ErrNotFound
	}
	var t Topic
	q := url.Values{"category": {category}}
	status, err := c.get(ctx, "/api/topics/"+url.PathEscape(name)+"?"+q.Encode(), MaxResponseBytes, &t)
	if status == http.StatusNotFound || status == http.StatusBadRequest {
		return nil, status, ErrNotFound
	}
	if err != nil {
		return nil, status, err
	}
	if t.Category != category || !strings.EqualFold(t.Name, name) {
		return nil, status, ErrNotFound
	}
	return &t, status, nil
}

// TopicFor returns the topic a search hit or a listing row names: the topic
// called name in category when vor can be asked by category, and the topic
// vor gives for the bare name when it cannot.
//
// A file of a directory vor was pointed at has its directory as its
// category ("docs/adr", or "." at the top). A vor of 2026-11-22 or later
// takes a directory as ?category=, so two files of one name in two
// directories are two topics. The top of a directory ("."), no category, or
// one vor would refuse is read by name alone, as every topic was before
// 2026-11-12. A vor from before 2026-11-22 answers a directory category
// with 400; TopicFor then reads the bare name, as it did before, and gets
// whichever file of that name vor gives.
func (c *Client) TopicFor(ctx context.Context, category, name string) (*Topic, error) {
	if !validCategory(category) {
		return c.Topic(ctx, name)
	}
	t, status, err := c.topicIn(ctx, category, name)
	if status == http.StatusBadRequest && strings.Contains(category, "/") {
		return c.Topic(ctx, name)
	}
	return t, err
}

// plainName reports whether s can be a topic name. "", "." and anything
// with ".." or "/" never are, and a server that cleans the path would answer
// for another URL ("/api/topics/." is the whole listing).
func plainName(s string) bool {
	return s != "" && s != "." && !strings.Contains(s, "..") && !strings.Contains(s, "/")
}

// validCategory reports whether s can be asked for as ?category=, by the
// rule vor applies: one or more segments joined by "/", each non-empty, not
// ".", without "..", a backslash or a NUL, and at most MaxCategoryBytes in
// all. A leading, trailing or doubled slash is an empty segment.
func validCategory(s string) bool {
	if s == "" || len(s) > MaxCategoryBytes {
		return false
	}
	for _, seg := range strings.Split(s, "/") {
		if !plainName(seg) || strings.ContainsAny(seg, "\\\x00") {
			return false
		}
	}
	return true
}

// get decodes the JSON body of a 200, of at most limit bytes, into v. The
// status is returned with any error so a caller can tell "not found" from
// "vor is down" (status 0).
func (c *Client) get(ctx context.Context, pathAndQuery string, limit int64, v any) (int, error) {
	endpoint, _, _ := strings.Cut(pathAndQuery, "?")
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+pathAndQuery, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return resp.StatusCode, err
	}
	if int64(len(body)) > limit {
		return resp.StatusCode, fmt.Errorf("vor %s: response exceeds %d bytes", endpoint, limit)
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("vor %s: HTTP %d", endpoint, resp.StatusCode)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return resp.StatusCode, fmt.Errorf("vor %s: %w", endpoint, err)
	}
	return resp.StatusCode, nil
}
