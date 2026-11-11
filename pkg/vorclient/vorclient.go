// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package vorclient is the read-only HTTP client for vor, the cheat-sheet
// server (http://127.0.0.1:20110 on a Kingdom host): search, topic fetch and
// the topic listing.
//
// Every call carries a deadline and reads a bounded response. Everything vor
// returns is untrusted text: the caller escapes it before showing it.
package vorclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
// The listing does not say where a topic came from; only Topic and Search
// carry source_trust.
type TopicSummary struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	HasDetail   bool     `json:"has_detail"`
	SeeAlso     []string `json:"see_also"`
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
func (c *Client) Search(ctx context.Context, q string) ([]Hit, error) {
	var hits []Hit
	if _, err := c.get(ctx, "/api/search?q="+url.QueryEscape(q), MaxResponseBytes, &hits); err != nil {
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
	// Never topic names, and a server that cleans the path would answer for
	// another URL ("/api/topics/." is the whole listing).
	if name == "" || name == "." || strings.Contains(name, "..") || strings.Contains(name, "/") {
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
