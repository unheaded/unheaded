// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package vmquery reads VictoriaMetrics' Prometheus query API: instant
// vectors (/api/v1/query) and ranges (/api/v1/query_range). Every call is
// bounded: a 5 s timeout and a 4 MiB answer; an answer over the cap is an
// error, never a partial decode. The client follows no redirect and uses no
// proxy: a query only ever goes to the configured address.
package vmquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Timeout bounds one query, connection included.
	Timeout = 5 * time.Second
	// MaxAnswer is the largest answer body decoded.
	MaxAnswer = 4 << 20
)

// ErrNoVictoria is returned by New for an empty base URL.
var ErrNoVictoria = errors.New("vmquery: no VictoriaMetrics base URL")

// Sample is one element of an instant vector.
type Sample struct {
	Metric map[string]string
	Time   time.Time
	// Value is NaN when Raw is not a number.
	Value float64
	// Raw is the value as VictoriaMetrics wrote it ("NaN", "+Inf", "1.5").
	Raw string
}

// Point is one value of a range series.
type Point struct {
	Time  time.Time
	Value float64
}

// Series is one element of a range matrix.
type Series struct {
	Metric map[string]string
	Points []Point
}

// Client queries one VictoriaMetrics.
type Client struct {
	base    string
	hc      *http.Client
	timeout time.Duration
}

// New returns a client for baseURL (http or https, a host, no credentials).
// A nil client gets one with Timeout, no proxy and no redirects.
func New(baseURL string, client *http.Client) (*Client, error) {
	if baseURL == "" {
		return nil, ErrNoVictoria
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("vmquery: bad VictoriaMetrics URL %q", baseURL)
	}
	if client == nil {
		client = &http.Client{
			Timeout:       Timeout,
			Transport:     &http.Transport{Proxy: nil},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), hc: client, timeout: Timeout}, nil
}

// Instant evaluates promql now.
func (c *Client) Instant(ctx context.Context, promql string) ([]Sample, error) {
	var data struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	}
	if err := c.get(ctx, "/api/v1/query", url.Values{"query": {promql}}, &data); err != nil {
		return nil, err
	}
	out := make([]Sample, 0, len(data.Result))
	for _, r := range data.Result {
		t, raw, v, err := point(r.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, Sample{Metric: r.Metric, Time: t, Value: v, Raw: raw})
	}
	return out, nil
}

// Range evaluates promql from start to end every step.
func (c *Client) Range(ctx context.Context, promql string, start, end time.Time, step time.Duration) ([]Series, error) {
	if step <= 0 {
		return nil, errors.New("vmquery: step must be positive")
	}
	if end.Before(start) {
		return nil, errors.New("vmquery: end before start")
	}
	q := url.Values{
		"query": {promql},
		"start": {strconv.FormatInt(start.Unix(), 10)},
		"end":   {strconv.FormatInt(end.Unix(), 10)},
		"step":  {strconv.FormatInt(int64(math.Max(1, step.Seconds())), 10)},
	}
	var data struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Values [][]any           `json:"values"`
		} `json:"result"`
	}
	if err := c.get(ctx, "/api/v1/query_range", q, &data); err != nil {
		return nil, err
	}
	out := make([]Series, 0, len(data.Result))
	for _, r := range data.Result {
		s := Series{Metric: r.Metric, Points: make([]Point, 0, len(r.Values))}
		for _, pv := range r.Values {
			t, _, v, err := point(pv)
			if err != nil {
				return nil, err
			}
			s.Points = append(s.Points, Point{Time: t, Value: v})
		}
		out = append(out, s)
	}
	return out, nil
}

// get runs one query and decodes the answer's data into dst.
func (c *Client) get(ctx context.Context, path string, q url.Values, dst any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("vmquery: %w", ctxErr)
		}
		return fmt.Errorf("vmquery: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxAnswer+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("vmquery: %w", ctxErr)
		}
		return fmt.Errorf("vmquery: read answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vmquery: VictoriaMetrics answered %d", resp.StatusCode)
	}
	if len(body) > MaxAnswer {
		return fmt.Errorf("vmquery: answer over %d bytes", MaxAnswer)
	}
	var env struct {
		Status string          `json:"status"`
		Error  string          `json:"error"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&env); err != nil {
		return fmt.Errorf("vmquery: decode answer: %w", err)
	}
	if env.Status != "success" {
		return fmt.Errorf("vmquery: query did not succeed: %q", env.Error)
	}
	if err := json.Unmarshal(env.Data, dst); err != nil {
		return fmt.Errorf("vmquery: decode data: %w", err)
	}
	return nil
}

// point decodes [unix_seconds, "value"].
func point(pv []any) (time.Time, string, float64, error) {
	if len(pv) != 2 {
		return time.Time{}, "", 0, errors.New("vmquery: malformed sample")
	}
	ts, ok := pv[0].(float64)
	raw, ok2 := pv[1].(string)
	if !ok || !ok2 {
		return time.Time{}, "", 0, errors.New("vmquery: malformed sample")
	}
	// A value that is not a number is NaN (no number), Raw keeps what came:
	// one bad series must not lose the answer's others.
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		v = math.NaN()
	}
	sec, frac := math.Modf(ts)
	return time.Unix(int64(sec), int64(math.Round(frac*1e9))), raw, v, nil
}
