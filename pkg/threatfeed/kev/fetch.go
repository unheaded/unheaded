// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package kev

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// FetchResult is one GET of the feed.
type FetchResult struct {
	NotModified  bool   // 304: the stored copy is current
	Body         []byte // nil when NotModified
	ETag         string
	LastModified string
	Status       int
}

// Fetcher does conditional GETs of the catalog: a 304 costs CISA's CDN
// nothing and leaves the stored copy in place.
type Fetcher struct {
	URL       string
	Client    *http.Client
	UserAgent string
	MaxBytes  int64 // 0 means MaxCatalogBytes
}

// NewFetcher returns a Fetcher for rawURL. Only https is accepted, unless
// allowHTTP is set (tests against httptest servers). TLS 1.2 is the floor
// because the far end is a public CDN, not a Kingdom service; 1.3 is
// negotiated whenever the CDN offers it.
func NewFetcher(rawURL string, timeout time.Duration, allowHTTP bool) (*Fetcher, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("feed URL: %w", err)
	}
	httpOK := allowHTTP && u.Scheme == "http"
	if u.Host == "" || (u.Scheme != "https" && !httpOK) {
		return nil, fmt.Errorf("feed URL %q: want an https URL with a host", rawURL)
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("http.DefaultTransport is not an *http.Transport")
	}
	tr := base.Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &Fetcher{
		URL: u.String(),
		Client: &http.Client{
			Timeout:   timeout,
			Transport: tr,
			// A redirect off https would defeat the scheme check above.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				if req.URL.Scheme != u.Scheme {
					return fmt.Errorf("redirect to %s refused", req.URL.Scheme)
				}
				return nil
			},
		},
		UserAgent: "unheaded-kev-poller/1 (+https://github.com/unheaded/unheaded)",
	}, nil
}

// Fetch GETs the feed, sending prev's validators when it has them. The body
// is capped at MaxCatalogBytes; a longer one is an error, not a truncation.
func (f *Fetcher) Fetch(ctx context.Context, prev *State) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "application/json")
	if prev != nil {
		if prev.ETag != "" {
			req.Header.Set("If-None-Match", prev.ETag)
		}
		if prev.LastModified != "" {
			req.Header.Set("If-Modified-Since", prev.LastModified)
		}
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res := &FetchResult{
		Status:       resp.StatusCode,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	switch resp.StatusCode {
	case http.StatusNotModified:
		res.NotModified = true
		return res, nil
	case http.StatusOK:
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return res, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}
	limit := f.MaxBytes
	if limit <= 0 || limit > MaxCatalogBytes {
		limit = MaxCatalogBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return res, fmt.Errorf("read feed: %w", err)
	}
	if int64(len(body)) > limit {
		return res, fmt.Errorf("%w: body exceeds %d bytes", ErrInvalid, limit)
	}
	res.Body = body
	return res, nil
}
