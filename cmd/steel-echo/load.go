// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The load generator is the game day's client-side meter: it sends a fixed
// request rate and writes one CSV line per second of send time:
//
//	ts,sent,ok,err5xx,errconn,p50_ms,p99_ms,versions_seen
//
// ok is 2xx, err5xx is 5xx, errconn is a request that got no HTTP answer
// (refused, reset, timed out, or no target to send to). A 3xx/4xx answer
// counts in sent only, so availability is ok/sent either way. Latency
// percentiles are over the requests that got an answer. versions_seen is
// the ;-joined set of versions the answers named.
//
// Discover mode reads Cuirass's actual state (GET /api/v1/state/actual,
// no key needed) every refresh period and round-robins over the healthy,
// running, managed instances labelled svc=<name>, so traffic share equals
// replica share (D-04: the canary split is client-side).

const (
	maxRPS          = 5000
	maxLoadDuration = 24 * time.Hour
	maxInflight     = 512
	maxAnswerBody   = 1 << 10
	maxStateBody    = 4 << 20
	csvHeader       = "ts,sent,ok,err5xx,errconn,p50_ms,p99_ms,versions_seen"
)

var (
	svcNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	versionRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)
)

type loadConfig struct {
	target   string
	discover string
	svc      string
	port     int
	rps      int
	duration time.Duration
	timeout  time.Duration
	refresh  time.Duration
	out      string
}

func checkHTTPURL(flagName, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("--%s: %w", flagName, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("--%s %q: want an http(s) URL", flagName, raw)
	}
	if u.Host == "" || u.User != nil {
		return fmt.Errorf("--%s %q: want a host and no credentials", flagName, raw)
	}
	return nil
}

func parseLoadFlags(args []string) (loadConfig, error) {
	var c loadConfig
	fs := flag.NewFlagSet("load", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.target, "target", "", "fixed URL to send GETs to (e.g. the stable proxy endpoint)")
	fs.StringVar(&c.discover, "discover", "", "Cuirass base URL: target its healthy instances of --svc")
	fs.StringVar(&c.svc, "svc", "", "service label to discover (svc=<name>)")
	fs.IntVar(&c.port, "port", 8080, "instance port in discover mode")
	fs.IntVar(&c.rps, "rps", 10, "requests per second (1..5000)")
	fs.DurationVar(&c.duration, "duration", 60*time.Second, "how long to send (at most 24h)")
	fs.DurationVar(&c.timeout, "timeout", 2*time.Second, "per-request timeout")
	fs.DurationVar(&c.refresh, "refresh", 2*time.Second, "discover refresh period")
	fs.StringVar(&c.out, "out", "", "CSV file (default stdout)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	switch {
	case c.target == "" && c.discover == "":
		return c, errors.New("set --target or --discover")
	case c.target != "" && c.discover != "":
		return c, errors.New("set --target or --discover, not both")
	case c.target != "":
		if err := checkHTTPURL("target", c.target); err != nil {
			return c, err
		}
	default:
		if err := checkHTTPURL("discover", c.discover); err != nil {
			return c, err
		}
		if !svcNameRe.MatchString(c.svc) {
			return c, fmt.Errorf("--svc %q: want a lowercase service name", c.svc)
		}
		if c.port < 1 || c.port > 65535 {
			return c, fmt.Errorf("--port %d out of range", c.port)
		}
	}
	if c.rps < 1 || c.rps > maxRPS {
		return c, fmt.Errorf("--rps %d: want 1..%d", c.rps, maxRPS)
	}
	if c.duration <= 0 || c.duration > maxLoadDuration {
		return c, fmt.Errorf("--duration %v: want (0, 24h]", c.duration)
	}
	if c.timeout <= 0 || c.timeout > time.Minute {
		return c, fmt.Errorf("--timeout %v: want (0, 1m]", c.timeout)
	}
	if c.refresh < 100*time.Millisecond || c.refresh > time.Minute {
		return c, fmt.Errorf("--refresh %v: want 100ms..1m", c.refresh)
	}
	return c, nil
}

// --- discovery ---

type discoverer struct {
	stateURL string
	svc      string
	port     int
	client   *http.Client

	mu      sync.RWMutex
	targets []string
}

func newDiscoverer(base, svc string, port int, client *http.Client) *discoverer {
	return &discoverer{
		stateURL: strings.TrimRight(base, "/") + "/api/v1/state/actual",
		svc:      svc,
		port:     port,
		client:   client,
	}
}

// instanceState is the subset of Cuirass's ContainerState discovery reads.
type instanceState struct {
	Status  string            `json:"status"`
	IP      string            `json:"ip"`
	Health  string            `json:"health"`
	Managed bool              `json:"managed"`
	Labels  map[string]string `json:"labels"`
}

// routableInstanceIP refuses addresses an instance has no business
// reporting: an instance chooses its own interface addresses, so a
// compromised one could otherwise point the meter at the host itself.
func routableInstanceIP(s string) (net.IP, bool) {
	ip := net.ParseIP(s)
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return nil, false
	}
	return ip, true
}

// refresh replaces the target list from one read of Cuirass's actual
// state. On any error the previous list stays.
func (d *discoverer) refresh(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.stateURL, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("discover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discover: Cuirass answered %d", resp.StatusCode)
	}
	var state map[string]instanceState
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxStateBody)).Decode(&state); err != nil {
		return fmt.Errorf("discover: %w", err)
	}
	names := make([]string, 0, len(state))
	for name := range state {
		names = append(names, name)
	}
	sort.Strings(names)
	targets := make([]string, 0, len(names))
	for _, name := range names {
		in := state[name]
		if !in.Managed || in.Status != "running" || in.Health != "healthy" || in.Labels["svc"] != d.svc {
			continue
		}
		ip, ok := routableInstanceIP(in.IP)
		if !ok {
			continue
		}
		targets = append(targets, "http://"+net.JoinHostPort(ip.String(), strconv.Itoa(d.port))+"/")
	}
	d.mu.Lock()
	d.targets = targets
	d.mu.Unlock()
	return nil
}

func (d *discoverer) run(ctx context.Context, period time.Duration) {
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := d.refresh(ctx); err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "steel-echo load: %v (keeping %d targets)\n", err, len(d.snapshot()))
			}
		}
	}
}

func (d *discoverer) snapshot() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]string(nil), d.targets...)
}

// --- meter ---

type result struct {
	status  int
	latency time.Duration
	version string
	connErr bool
}

type bucket struct {
	sent, ok, e5, econn int
	lat                 []float64 // ms, answered requests only
	versions            map[string]bool
}

type summary struct {
	sent, ok, err5xx, errconn int
}

type meter struct {
	mu      sync.Mutex
	buckets map[int64]*bucket
	total   summary
}

func newMeter() *meter { return &meter{buckets: map[int64]*bucket{}} }

// record adds one request to the second it was sent in.
func (m *meter) record(ts int64, r result) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.buckets[ts]
	if b == nil {
		b = &bucket{versions: map[string]bool{}}
		m.buckets[ts] = b
	}
	b.sent++
	m.total.sent++
	if r.connErr {
		b.econn++
		m.total.errconn++
		return
	}
	switch {
	case r.status >= 200 && r.status < 300:
		b.ok++
		m.total.ok++
	case r.status >= 500:
		b.e5++
		m.total.err5xx++
	}
	b.lat = append(b.lat, float64(r.latency)/float64(time.Millisecond))
	if r.version != "" {
		v := r.version
		if !versionRe.MatchString(v) {
			v = "?" // never write an arbitrary answer into the CSV
		}
		b.versions[v] = true
	}
}

func (m *meter) writeHeader(w io.Writer) error {
	_, err := io.WriteString(w, csvHeader+"\n")
	return err
}

// percentile is nearest-rank over sorted values.
func percentile(sorted []float64, p float64) string {
	if len(sorted) == 0 {
		return ""
	}
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return strconv.FormatFloat(sorted[rank-1], 'f', 1, 64)
}

// flushBefore writes, in time order, and forgets every second before ts.
func (m *meter) flushBefore(w io.Writer, ts int64) error {
	m.mu.Lock()
	keys := make([]int64, 0, len(m.buckets))
	for k := range m.buckets {
		if k < ts {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	done := make([]*bucket, len(keys))
	for i, k := range keys {
		done[i] = m.buckets[k]
		delete(m.buckets, k)
	}
	m.mu.Unlock()

	cw := csv.NewWriter(w)
	for i, b := range done {
		sort.Float64s(b.lat)
		vs := make([]string, 0, len(b.versions))
		for v := range b.versions {
			vs = append(vs, v)
		}
		sort.Strings(vs)
		if err := cw.Write([]string{
			strconv.FormatInt(keys[i], 10),
			strconv.Itoa(b.sent), strconv.Itoa(b.ok), strconv.Itoa(b.e5), strconv.Itoa(b.econn),
			percentile(b.lat, 0.50), percentile(b.lat, 0.99),
			strings.Join(vs, ";"),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func (m *meter) summary() summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.total
}

// --- the run ---

func sendOne(ctx context.Context, client *http.Client, target string) result {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return result{connErr: true}
	}
	resp, err := client.Do(req)
	if err != nil {
		return result{connErr: true}
	}
	defer resp.Body.Close()
	var body struct {
		Version string `json:"version"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxAnswerBody)).Decode(&body)
	return result{status: resp.StatusCode, latency: time.Since(start), version: body.Version}
}

// runLoad sends cfg.rps GETs per second for cfg.duration and streams the
// per-second CSV to out. client's redirects are never followed.
func runLoad(ctx context.Context, cfg loadConfig, out io.Writer, client *http.Client) (summary, error) {
	c := *client
	c.Timeout = cfg.timeout
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	var pick func() string
	if cfg.discover != "" {
		d := newDiscoverer(cfg.discover, cfg.svc, cfg.port, &c)
		if err := d.refresh(ctx); err != nil {
			return summary{}, err
		}
		refresh := cfg.refresh
		if refresh <= 0 {
			refresh = 2 * time.Second
		}
		dctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go d.run(dctx, refresh)
		var mu sync.Mutex
		next := 0
		pick = func() string {
			ts := d.snapshot()
			if len(ts) == 0 {
				return ""
			}
			mu.Lock()
			defer mu.Unlock()
			next++
			return ts[next%len(ts)]
		}
	} else {
		pick = func() string { return cfg.target }
	}

	m := newMeter()
	if err := m.writeHeader(out); err != nil {
		return summary{}, err
	}
	sem := make(chan struct{}, maxInflight)
	var wg sync.WaitGroup
	tick := time.NewTicker(time.Second / time.Duration(cfg.rps))
	defer tick.Stop()
	end := time.Now().Add(cfg.duration)
	lag := int64(cfg.timeout/time.Second) + 2 // seconds a bucket may still be filling

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case now := <-tick.C:
			if !now.Before(end) {
				break loop
			}
			ts := now.Unix()
			target := pick()
			if target == "" {
				m.record(ts, result{connErr: true}) // nothing healthy to send to
				continue
			}
			select {
			case sem <- struct{}{}:
			default:
				m.record(ts, result{connErr: true}) // the meter itself is saturated
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				m.record(ts, sendOne(ctx, &c, target))
			}()
			if err := m.flushBefore(out, ts-lag); err != nil {
				return m.summary(), err
			}
		}
	}
	wg.Wait()
	if err := m.flushBefore(out, math.MaxInt64); err != nil {
		return m.summary(), err
	}
	return m.summary(), nil
}

func runLoadCmd(args []string) error {
	cfg, err := parseLoadFlags(args)
	if err != nil {
		return err
	}
	out := io.Writer(os.Stdout)
	if cfg.out != "" {
		f, err := os.OpenFile(filepath.Clean(cfg.out), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := &http.Client{Transport: &http.Transport{
		Proxy:               nil, // never HTTP_PROXY: the meter talks to lab addresses only
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     30 * time.Second,
	}}
	sum, err := runLoad(ctx, cfg, out, client)
	if sum.sent > 0 {
		fmt.Fprintf(os.Stderr, "sent %d ok %d err5xx %d errconn %d availability %.4f\n",
			sum.sent, sum.ok, sum.err5xx, sum.errconn, float64(sum.ok)/float64(sum.sent))
	}
	return err
}
