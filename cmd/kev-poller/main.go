// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// kev-poller keeps the CISA Known Exploited Vulnerabilities catalog current
// on this host (ADR-055).
//
// Each pull is a conditional GET. A changed catalog is validated, stored
// under --state-dir (latest.json, state.json, snapshots/), diffed against
// the previous one, and announced on Wotan:
//
//	threats.cisa-kev.updates  every successful pull: version, count, sha256
//	threats.cisa-kev.delta    CVEs added/removed since the last catalog
//
// A failed pull keeps the last good catalog and retries with backoff.
//
// Usage:
//
//	kev-poller --once                    # one pull, print a summary, exit
//	kev-poller --interval 1h             # daemon (15m..24h)
//	kev-poller report --since 2026-04-27 # threat-register entry from the stored catalog
//
// With --stack (config/kev-poller/stack.yaml), added entries that match what
// the Kingdom runs are tagged with their categories, logged as warnings and
// counted separately (ADR-055 Phase 3).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
	"unheaded/pkg/threatfeed/kev"
	wotanClient "unheaded/pkg/wotan-client"
)

const (
	serviceName  = "kev-poller"
	topicUpdates = "threats.cisa-kev.updates"
	topicDelta   = "threats.cisa-kev.delta"

	minInterval = 15 * time.Minute // polite to CISA's CDN
	maxInterval = 24 * time.Hour   // ADR-052: the register must not go a week stale
	minBackoff  = time.Minute
)

type config struct {
	url      string
	interval time.Duration
	stateDir string
	keep     time.Duration
	timeout  time.Duration
	port     int
	wotan    string
	node     string
	stack    string
	once     bool
}

func parseFlags(args []string) (config, error) {
	var c config
	fs := flag.NewFlagSet(serviceName, flag.ContinueOnError)
	fs.StringVar(&c.url, "url", envOr("KEV_FEED_URL", kev.DefaultURL), "catalog URL (https only)")
	interval := fs.String("interval", envOr("KEV_PULL_INTERVAL", "1h"), "time between pulls, 15m..24h")
	fs.StringVar(&c.stateDir, "state-dir", envOr("KEV_OUTPUT_PATH", "/var/lib/unheaded/threat-feeds/cisa-kev"), "where latest.json, state.json and snapshots/ live")
	fs.DurationVar(&c.keep, "keep", 30*24*time.Hour, "delete snapshots older than this (the newest is always kept)")
	fs.DurationVar(&c.timeout, "timeout", 60*time.Second, "per-pull HTTP timeout")
	fs.IntVar(&c.port, "port", ports.KEVPoller, "HTTP port for /health, /ready, /metrics, /api/v1/kev")
	fs.StringVar(&c.wotan, "wotan", envOr("WOTAN_ADDR", "localhost:18000"), "Wotan HTTP address; empty disables publishing")
	fs.StringVar(&c.node, "node", "", "node identifier (default: hostname)")
	fs.StringVar(&c.stack, "stack", os.Getenv("KEV_STACK"), "stack file (config/kev-poller/stack.yaml): tag entries the Kingdom runs")
	fs.BoolVar(&c.once, "once", false, "pull once, print a summary, exit (non-zero on failure)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	d, err := time.ParseDuration(*interval)
	if err != nil {
		return c, fmt.Errorf("--interval: %w", err)
	}
	if d < minInterval || d > maxInterval {
		return c, fmt.Errorf("--interval %s outside %s..%s", d, minInterval, maxInterval)
	}
	c.interval = d
	if c.keep < 0 {
		return c, errors.New("--keep must not be negative")
	}
	if c.node == "" {
		c.node, _ = os.Hostname()
	}
	return c, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = zerolog.New(os.Stderr).With().Timestamp().Str("service", serviceName).Logger()

	if len(os.Args) > 1 && os.Args[1] == "report" {
		os.Exit(runReport(os.Args[2:], os.Stdout))
	}
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal().Err(err).Msg("bad flags")
	}
	fetcher, err := kev.NewFetcher(cfg.url, cfg.timeout, false)
	if err != nil {
		log.Fatal().Err(err).Msg("bad feed URL")
	}
	var stack *kev.Stack
	if cfg.stack != "" {
		if stack, err = kev.LoadStack(cfg.stack); err != nil {
			log.Fatal().Err(err).Msg("bad stack file")
		}
	}
	p := &kev.Poller{Fetcher: fetcher, Store: kev.Store{Dir: cfg.stateDir}}
	if err := p.Init(); err != nil {
		// The next good pull replaces it; until then there is nothing to diff against.
		log.Error().Err(err).Str("dir", cfg.stateDir).Msg("stored catalog unusable; next pull is a bootstrap")
	}

	if cfg.once {
		os.Exit(runOnce(p, stack, cfg))
	}

	m := newPollMetrics()
	d := &daemon{cfg: cfg, poller: p, stack: stack, metrics: m, publish: connectWotan(cfg.wotan)}
	if st := p.Current(); st != nil {
		m.count.Set(float64(st.Count))
	}
	d.setRelevantCount()

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.port),
		Handler:           d.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("HTTP listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP listen failed")
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Info().Str("url", cfg.url).Dur("interval", cfg.interval).Str("dir", cfg.stateDir).Msg("kev-poller started")
	d.run(ctx)

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	log.Info().Msg("kev-poller stopped")
}

func runOnce(p *kev.Poller, stack *kev.Stack, cfg config) int {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout+10*time.Second)
	defer cancel()
	out, err := p.Poll(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kev-poller: pull failed: %v\n", err)
		return 1
	}
	if _, err := p.Store.Prune(cfg.keep, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "kev-poller: prune: %v\n", err)
	}
	fmt.Printf("catalog %s: %d entries, sha256 %s\n", out.State.CatalogVersion, out.State.Count, out.State.SHA256)
	switch {
	case out.NotModified:
		fmt.Println("unchanged since the last pull")
	case out.Bootstrap:
		fmt.Printf("first catalog stored in %s (baseline, no delta)\n", cfg.stateDir)
	default:
		fmt.Printf("%d added, %d removed\n", len(out.Delta.Added), len(out.Delta.Removed))
		for _, v := range out.Delta.Added {
			tag := ""
			if cats := stack.Classify(v); len(cats) > 0 {
				tag = "  [STACK: " + strings.Join(cats, ", ") + "]"
			}
			fmt.Printf("  + %s %s %s%s\n", v.CVEID, v.VendorProject, v.Product, tag)
		}
		for _, id := range out.Delta.Removed {
			fmt.Printf("  - %s\n", id)
		}
	}
	return 0
}

type pollMetrics struct {
	reg         *metrics.Registry
	success     *metrics.Counter
	failure     *metrics.Counter
	lastPull    *metrics.Gauge
	lastSuccess *metrics.Gauge
	count       *metrics.Gauge
	delta       *metrics.Gauge
	relevant    *metrics.Gauge
	relAdded    *metrics.Counter
}

func newPollMetrics() *pollMetrics {
	m := &pollMetrics{
		reg:         metrics.NewRegistry(),
		success:     metrics.NewCounter("unheaded_kev_pull_success_total", "Pulls that ended with a valid catalog stored or confirmed unchanged", nil),
		failure:     metrics.NewCounter("unheaded_kev_pull_failure_total", "Pulls that failed (network, HTTP status, validation, storage)", nil),
		lastPull:    metrics.NewGauge("unheaded_kev_last_pull_timestamp_seconds", "Unix time of the last pull attempt", nil),
		lastSuccess: metrics.NewGauge("unheaded_kev_last_success_timestamp_seconds", "Unix time of the last successful pull; alert on its age", nil),
		count:       metrics.NewGauge("unheaded_kev_total_count", "Entries in the stored catalog", nil),
		delta:       metrics.NewGauge("unheaded_kev_delta_count", "CVEs added by the last catalog change", nil),
		relevant:    metrics.NewGauge("unheaded_kev_stack_count", "Entries in the stored catalog that match the stack file", nil),
		relAdded:    metrics.NewCounter("unheaded_kev_stack_added_total", "Added entries that match the stack file, since start", nil),
	}
	for _, c := range []metrics.Collector{m.success, m.failure, m.lastPull, m.lastSuccess, m.count, m.delta, m.relevant, m.relAdded} {
		m.reg.MustRegister(c)
	}
	return m
}

// publisher sends one payload to one topic; nil when Wotan is off.
type publisher func(topic string, payload []byte) error

func connectWotan(addr string) publisher {
	if addr == "" {
		return nil
	}
	wotanClient.InitServiceIdentity(serviceName) // ADR-103 §6: Wotan calls carry kev-poller's service token
	c, err := wotanClient.NewClient(strings.TrimPrefix(addr, "http://"))
	if err != nil {
		log.Warn().Err(err).Str("addr", addr).Msg("Wotan unavailable; running without publishing")
		return nil
	}
	// A client must subscribe to a topic before it may publish there.
	var mu sync.Mutex
	joined := map[string]bool{}
	return func(topic string, payload []byte) error {
		mu.Lock()
		defer mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !joined[topic] {
			if _, err := c.Subscribe(ctx, topic, serviceName); err != nil {
				return fmt.Errorf("join %s: %w", topic, err)
			}
			joined[topic] = true
		}
		return c.Publish(ctx, topic, payload)
	}
}

type daemon struct {
	cfg     config
	poller  *kev.Poller
	stack   *kev.Stack // nil: no tagging
	metrics *pollMetrics
	publish publisher

	mu      sync.Mutex
	lastErr string
	nextAt  time.Time
}

// run polls at once, then every interval; after a failure it retries
// sooner, doubling from a minute up to the interval.
func (d *daemon) run(ctx context.Context) {
	backoff := minBackoff
	for {
		wait := d.cfg.interval
		if err := d.pollOnce(ctx); err != nil {
			wait = backoff
			backoff = min(backoff*2, d.cfg.interval)
		} else {
			backoff = minBackoff
		}
		d.mu.Lock()
		d.nextAt = time.Now().Add(wait)
		d.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (d *daemon) pollOnce(ctx context.Context) error {
	pctx, cancel := context.WithTimeout(ctx, d.cfg.timeout+10*time.Second)
	defer cancel()
	d.metrics.lastPull.SetToCurrentTime()
	out, err := d.poller.Poll(pctx)
	d.mu.Lock()
	if err != nil {
		d.lastErr = err.Error()
	} else {
		d.lastErr = ""
	}
	d.mu.Unlock()
	if err != nil {
		d.metrics.failure.Inc()
		log.Warn().Err(err).Msg("pull failed; keeping the last good catalog")
		return err
	}
	d.metrics.success.Inc()
	d.metrics.lastSuccess.SetToCurrentTime()
	d.metrics.count.Set(float64(out.State.Count))
	tagged := d.stack.Relevant(out.Delta.Added)
	for _, t := range tagged {
		log.Warn().Str("cve", t.CVEID).Str("vendor", t.VendorProject).Str("product", t.Product).
			Strs("categories", t.Categories).Str("due", t.DueDate).Msg("KEV entry added for something the Kingdom runs")
	}
	if !out.NotModified {
		d.metrics.delta.Set(float64(len(out.Delta.Added)))
		d.metrics.relAdded.Add(float64(len(tagged)))
		d.setRelevantCount()
		if n, err := d.poller.Store.Prune(d.cfg.keep, time.Now()); err != nil {
			log.Warn().Err(err).Msg("snapshot prune failed")
		} else if n > 0 {
			log.Info().Int("removed", n).Msg("pruned old snapshots")
		}
	}
	log.Info().
		Str("catalog_version", out.State.CatalogVersion).
		Int("count", out.State.Count).
		Bool("unchanged", out.NotModified).
		Bool("bootstrap", out.Bootstrap).
		Int("added", len(out.Delta.Added)).
		Int("removed", len(out.Delta.Removed)).
		Msg("pull ok")
	d.announce(out, len(tagged))
	return nil
}

func (d *daemon) setRelevantCount() {
	if d.stack == nil {
		return
	}
	if c := d.poller.Catalog(); c != nil {
		d.metrics.relevant.Set(float64(len(d.stack.Relevant(c.Vulnerabilities))))
	}
}

// updateMsg is published on every successful pull.
type updateMsg struct {
	Feed           string `json:"feed"`
	Node           string `json:"node"`
	CatalogVersion string `json:"catalog_version"`
	Count          int    `json:"count"`
	SHA256         string `json:"sha256"`
	Changed        bool   `json:"changed"`
	Bootstrap      bool   `json:"bootstrap"`
	Added          int    `json:"added"`
	StackAdded     int    `json:"stack_added"` // of Added, entries matching the stack file
	Removed        int    `json:"removed"`
	Time           int64  `json:"time"` // Unix ms
}

// deltaMsg carries part of a delta. A catalog change can add hundreds of
// entries (CISA's backfills); Wotan bodies are capped at 1 MB, so the delta
// goes out in chunks of deltaChunk entries, Removed in the first.
type deltaMsg struct {
	Feed           string       `json:"feed"`
	Node           string       `json:"node"`
	CatalogVersion string       `json:"catalog_version"`
	Chunk          int          `json:"chunk"`  // 1-based
	Chunks         int          `json:"chunks"` // total
	Added          []kev.Tagged `json:"added"`  // categories set where the stack file matches
	Removed        []string     `json:"removed,omitempty"`
	Time           int64        `json:"time"`
}

const deltaChunk = 50

func deltaMessages(node string, stack *kev.Stack, out kev.Outcome, now time.Time) []deltaMsg {
	if out.Delta.Empty() {
		return nil
	}
	n := (len(out.Delta.Added) + deltaChunk - 1) / deltaChunk
	if n == 0 {
		n = 1
	}
	msgs := make([]deltaMsg, 0, n)
	for i := 0; i < n; i++ {
		lo, hi := i*deltaChunk, min((i+1)*deltaChunk, len(out.Delta.Added))
		m := deltaMsg{
			Feed: "cisa-kev", Node: node, CatalogVersion: out.State.CatalogVersion,
			Chunk: i + 1, Chunks: n, Added: []kev.Tagged{}, Time: now.UnixMilli(),
		}
		for _, v := range out.Delta.Added[lo:hi] {
			m.Added = append(m.Added, kev.Tagged{Vulnerability: v, Categories: stack.Classify(v)})
		}
		if i == 0 {
			m.Removed = out.Delta.Removed
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func (d *daemon) announce(out kev.Outcome, stackAdded int) {
	if d.publish == nil {
		return
	}
	now := time.Now()
	send := func(topic string, v any) {
		b, err := json.Marshal(v)
		if err == nil {
			err = d.publish(topic, b)
		}
		if err != nil {
			log.Warn().Err(err).Str("topic", topic).Msg("publish failed")
		}
	}
	send(topicUpdates, updateMsg{
		Feed: "cisa-kev", Node: d.cfg.node, CatalogVersion: out.State.CatalogVersion,
		Count: out.State.Count, SHA256: out.State.SHA256, Changed: !out.NotModified,
		Bootstrap: out.Bootstrap, Added: len(out.Delta.Added), StackAdded: stackAdded,
		Removed: len(out.Delta.Removed), Time: now.UnixMilli(),
	})
	for _, m := range deltaMessages(d.cfg.node, d.stack, out, now) {
		send(topicDelta, m)
	}
}

func (d *daemon) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", httputil.ProbeMethods(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"ok","service":"kev-poller"}`)
	}))
	// Ready once a valid catalog is on disk, pulled now or loaded at start:
	// consumers read latest.json, not this process.
	mux.HandleFunc("/ready", httputil.ProbeMethods(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if d.poller.Current() == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"status":"not_ready","reason":"no valid catalog stored yet"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":"ready"}`)
	}))
	mux.Handle("/metrics", metrics.HandlerFor(d.metrics.reg, metrics.DefaultRegistry))
	mux.HandleFunc("GET /api/v1/kev", func(w http.ResponseWriter, _ *http.Request) {
		d.mu.Lock()
		resp := struct {
			Catalog   *kev.State `json:"catalog"`
			LastError string     `json:"last_error,omitempty"`
			NextPull  time.Time  `json:"next_pull,omitzero"`
			Interval  string     `json:"interval"`
		}{d.poller.Current(), d.lastErr, d.nextAt, d.cfg.interval.String()}
		d.mu.Unlock()
		httputil.WriteJSON(w, http.StatusOK, resp)
	})
	return httputil.SecurityHeaders(httputil.NewServiceMetrics(serviceName).Instrument(mux, mux))
}

// runReport prints a threat-register refresh entry (markdown) from the
// stored catalog; it does not pull.
func runReport(args []string, w io.Writer) int {
	fs := flag.NewFlagSet("kev-poller report", flag.ContinueOnError)
	since := fs.String("since", "", "count entries added on or after this date (YYYY-MM-DD); required")
	stateDir := fs.String("state-dir", envOr("KEV_OUTPUT_PATH", "/var/lib/unheaded/threat-feeds/cisa-kev"), "where latest.json is")
	stackPath := fs.String("stack", envOr("KEV_STACK", "config/kev-poller/stack.yaml"), "stack file; empty to skip matching")
	recent := fs.Int("recent", 15, "how many other new entries to list")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if _, err := time.Parse("2006-01-02", *since); err != nil {
		fmt.Fprintln(os.Stderr, "kev-poller report: --since YYYY-MM-DD is required")
		return 2
	}
	c, st, err := kev.Store{Dir: *stateDir}.Load()
	if err != nil || c == nil {
		fmt.Fprintf(os.Stderr, "kev-poller report: no usable catalog in %s (%v); pull one with --once\n", *stateDir, err)
		return 1
	}
	var stack *kev.Stack
	if *stackPath != "" {
		if stack, err = kev.LoadStack(*stackPath); err != nil {
			fmt.Fprintf(os.Stderr, "kev-poller report: %v\n", err)
			return 1
		}
	}
	err = kev.WriteReport(w, c, st, stack, kev.ReportOptions{
		Since: *since, Today: time.Now().UTC().Format("2006-01-02"), StackPath: *stackPath, Recent: *recent,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "kev-poller report: %v\n", err)
		return 1
	}
	return 0
}
