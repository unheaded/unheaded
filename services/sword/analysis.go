// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sword

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/deploy/strategy"
	"unheaded/pkg/sloguard"
)

// Canary analysis (S-REAL-STEEL D-06). Each analysis interval Sword reads
// the canary and stable instances' own /metrics twice, one interval apart,
// and judges the canary on the deltas of the house HTTP series
// (unheaded_http_requests_total, unheaded_http_request_duration_seconds),
// leaving out /health, /ready and /metrics so Cuirass's probes and Sword's
// own scrapes do not count. Fewer than min_sample_size canary requests in
// an interval is inconclusive: it holds (never promotes), and three
// inconclusive intervals in a row fail the analysis. Separately the Kingdom
// SLO guard (pkg/sloguard) runs before and after the measurement: a page
// SLO burning at the fast tier, or missing, fails the analysis.
//
// The thresholds (error ratio, p99) are the strategy's to apply to the
// numbers measured here. This replaces services/canary's evaluateCanary,
// whose metrics were constants: its SLOThreshold maps onto
// error_rate_threshold (MaxErrorRatePPM/1e6), latency_p99_threshold
// (MaxLatencyP99MS) and min_sample_size per interval (MinThroughputPPS x
// interval).
//
// Instances are reached at the address Cuirass reports for them, on the
// health port, path /metrics: no redirects, no proxy from the environment,
// 2 s per scrape, 1 MiB per body; loopback, link-local, unspecified and
// multicast addresses are refused (the same rule as Cuirass's probe).

var _ Analysis = (*Analyzer)(nil)

// SLOChecker is the Kingdom SLO guard.
type SLOChecker interface {
	Check(ctx context.Context) (sloguard.Verdict, error)
}

const (
	defaultMaxInconclusive = 3
	defaultMinSamples      = 100
	defaultInterval        = 30 * time.Second
	scrapeTimeout          = 2 * time.Second
	maxScrapeBody          = 1 << 20
	maxScrapeLines         = 50000
)

var excludedPaths = map[string]bool{"/health": true, "/ready": true, "/metrics": true}

// Analyzer judges canary steps.
type Analyzer struct {
	guard           SLOChecker
	client          *http.Client
	maxInconclusive int
	metricsURL      func(inst strategy.Instance, port int) (string, error)
	// probeURL and probeEvery drive the client-side probes: during an
	// interval each canary instance's /health is fetched every probeEvery
	// and the round trips feed ClientLatencyP99. A network fault between
	// Sword and the canary (netem on its host veth; game day 1 Scenario A)
	// never reaches the instance's own histogram, so p99 is the worse of
	// the two readings. A failed probe counts as the scrape timeout.
	probeURL   func(inst strategy.Instance, port int) (string, error)
	probeEvery time.Duration
}

// NewAnalyzer returns an Analyzer guarded by g. A nil g fails every
// analysis: without the guard a canary could burn the Kingdom's budget.
func NewAnalyzer(g SLOChecker) *Analyzer {
	a := &Analyzer{
		guard:           g,
		maxInconclusive: defaultMaxInconclusive,
		client: &http.Client{
			Timeout:       scrapeTimeout,
			Transport:     &http.Transport{Proxy: nil},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	a.metricsURL = defaultMetricsURL
	a.probeURL = defaultProbeURL
	a.probeEvery = defaultProbeEvery
	return a
}

const defaultProbeEvery = time.Second

func defaultProbeURL(inst strategy.Instance, port int) (string, error) {
	u, err := defaultMetricsURL(inst, port)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(u, "/metrics") + "/health", nil
}

func defaultMetricsURL(inst strategy.Instance, port int) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("instance %s: port %d", inst.ID, port)
	}
	ip := net.ParseIP(inst.Address)
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return "", fmt.Errorf("instance %s: address %q is not one Sword will scrape", inst.ID, inst.Address)
	}
	return "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)) + "/metrics", nil
}

func fail(msg string, m *strategy.AnalysisMetrics) *strategy.AnalysisResult {
	return &strategy.AnalysisResult{Pass: false, ShouldRollback: true, Message: msg, Metrics: m}
}

func (a *Analyzer) guardCheck(ctx context.Context) string {
	if a.guard == nil {
		return "no Kingdom SLO guard configured (VM_URL): analysis fails closed"
	}
	v, err := a.guard.Check(ctx)
	if err != nil {
		return "Kingdom SLO guard: " + err.Error()
	}
	if !v.OK {
		return "Kingdom SLO guard: " + v.Reason
	}
	return ""
}

// Analyze measures the canary against stable. It returns an error only
// when ctx ends; every measured or refused outcome is a result.
func (a *Analyzer) Analyze(ctx context.Context, req *strategy.AnalysisRequest, port int) (*strategy.AnalysisResult, error) {
	if req == nil || len(req.Canary) == 0 {
		return fail("no canary instance to analyse", nil), nil
	}
	if why := a.guardCheck(ctx); why != "" {
		return fail(why, nil), nil
	}
	minSamples, interval := defaultMinSamples, defaultInterval
	if req.Config != nil {
		if req.Config.MinSampleSize > 0 {
			minSamples = req.Config.MinSampleSize
		}
		if req.Config.Interval > 0 {
			interval = req.Config.Interval
		}
	}

	last := ""
	for attempt := 1; attempt <= a.maxInconclusive; attempt++ {
		c0, cerr0 := a.scrapeSet(ctx, req.Canary, port)
		s0, serr0 := a.scrapeSet(ctx, req.Stable, port)
		pctx, pcancel := context.WithCancel(ctx)
		probes := make(chan []time.Duration, 1)
		go func() { probes <- a.probe(pctx, req.Canary, port) }()
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			pcancel()
			<-probes
			return nil, ctx.Err()
		case <-t.C:
		}
		pcancel()
		rtts := <-probes
		c1, cerr1 := a.scrapeSet(ctx, req.Canary, port)
		s1, serr1 := a.scrapeSet(ctx, req.Stable, port)

		if err := errors.Join(cerr0, cerr1); err != nil {
			last = "canary /metrics unreadable: " + err.Error()
			continue
		}
		cd := c1.minus(c0)
		if cd.requests < float64(minSamples) {
			last = fmt.Sprintf("%.0f canary requests in %v, need %d", cd.requests, interval, minSamples)
			continue
		}
		m := cd.metrics()
		if cp := clientQuantile(rtts, 0.99); cp > 0 {
			m.ClientLatencyP99 = cp
			if cp > m.LatencyP99 {
				m.LatencyP99 = cp
			}
		}
		msg := fmt.Sprintf("canary error ratio %.4f, p99 %v (client-observed p99 %v over %d probes) over %d requests",
			m.ErrorRate, m.LatencyP99, m.ClientLatencyP99, len(rtts), m.RequestCount)
		if len(req.Stable) > 0 {
			if serr := errors.Join(serr0, serr1); serr == nil {
				sm := s1.minus(s0).metrics()
				msg += fmt.Sprintf("; stable error ratio %.4f, p99 %v over %d", sm.ErrorRate, sm.LatencyP99, sm.RequestCount)
			} else {
				msg += "; stable /metrics unreadable"
			}
		}
		if why := a.guardCheck(ctx); why != "" {
			return fail(why+" (after measuring: "+msg+")", m), nil
		}
		return &strategy.AnalysisResult{Pass: true, Message: msg, Metrics: m}, nil
	}
	return fail(fmt.Sprintf("inconclusive after %d intervals: %s", a.maxInconclusive, last), nil), nil
}

// probe fetches each canary instance's probe URL every probeEvery until
// ctx ends and returns every round trip. An instance whose URL is refused is
// skipped (the scrape reports it); a probe that fails or times out counts
// as scrapeTimeout, so an unreachable canary reads as slow, not as fast.
func (a *Analyzer) probe(ctx context.Context, insts []strategy.Instance, port int) []time.Duration {
	var urls []string
	for _, inst := range insts {
		if u, err := a.probeURL(inst, port); err == nil {
			urls = append(urls, u)
		}
	}
	var rtts []time.Duration
	if len(urls) == 0 {
		return rtts
	}
	every := a.probeEvery
	if every <= 0 {
		every = defaultProbeEvery
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		for _, u := range urls {
			rtts = append(rtts, a.probeOnce(ctx, u))
		}
		select {
		case <-ctx.Done():
			return rtts
		case <-tick.C:
		}
	}
}

func (a *Analyzer) probeOnce(ctx context.Context, u string) time.Duration {
	// The probe's own deadline, not the interval's: a probe cut short by the
	// interval ending still measures what it saw up to that point.
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), scrapeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(pctx, http.MethodGet, u, nil)
	if err != nil {
		return scrapeTimeout
	}
	start := time.Now()
	resp, err := a.client.Do(req)
	if err != nil {
		return scrapeTimeout
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return scrapeTimeout
	}
	return time.Since(start)
}

// clientQuantile is the q-th round trip (nearest rank); 0 for no probes.
func clientQuantile(rtts []time.Duration, q float64) time.Duration {
	if len(rtts) == 0 {
		return 0
	}
	s := make([]time.Duration, len(rtts))
	copy(s, rtts)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := int(math.Ceil(q*float64(len(s)))) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(s) {
		i = len(s) - 1
	}
	return s[i]
}

// scrapeSet sums the instances' samples. Any instance failing is an error.
func (a *Analyzer) scrapeSet(ctx context.Context, insts []strategy.Instance, port int) (httpSample, error) {
	total := newSample()
	var errs []error
	for _, inst := range insts {
		s, err := a.scrape(ctx, inst, port)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		total.add(s)
	}
	return total, errors.Join(errs...)
}

func (a *Analyzer) scrape(ctx context.Context, inst strategy.Instance, port int) (httpSample, error) {
	u, err := a.metricsURL(inst, port)
	if err != nil {
		return httpSample{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, scrapeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return httpSample{}, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return httpSample{}, fmt.Errorf("%s: %w", inst.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return httpSample{}, fmt.Errorf("%s: /metrics answered %d", inst.ID, resp.StatusCode)
	}
	s, err := parseHTTPMetrics(io.LimitReader(resp.Body, maxScrapeBody))
	if err != nil {
		return httpSample{}, fmt.Errorf("%s: %w", inst.ID, err)
	}
	return s, nil
}

// --- the house HTTP series ---

type httpSample struct {
	requests float64
	errors   float64
	buckets  map[float64]float64 // le (seconds, +Inf included) -> cumulative count
}

func newSample() httpSample { return httpSample{buckets: map[float64]float64{}} }

func (s *httpSample) add(o httpSample) {
	s.requests += o.requests
	s.errors += o.errors
	for le, v := range o.buckets {
		s.buckets[le] += v
	}
}

// minus is the delta since an earlier sample. A counter that went down was
// reset (the instance restarted): the later value is the delta then.
func (s httpSample) minus(earlier httpSample) httpSample {
	d := newSample()
	if s.requests < earlier.requests {
		return s
	}
	d.requests = s.requests - earlier.requests
	d.errors = max(s.errors-earlier.errors, 0)
	for le, v := range s.buckets {
		d.buckets[le] = max(v-earlier.buckets[le], 0)
	}
	return d
}

func (s httpSample) metrics() *strategy.AnalysisMetrics {
	m := &strategy.AnalysisMetrics{RequestCount: int(s.requests), ErrorCount: int(s.errors)}
	if s.requests > 0 {
		m.ErrorRate = s.errors / s.requests
		m.SuccessRate = 1 - m.ErrorRate
	}
	m.LatencyP99 = s.quantile(0.99)
	m.LatencyP50 = s.quantile(0.50)
	return m
}

// quantile is the upper bound of the bucket holding the q-th request (the
// usual conservative histogram reading). In the +Inf bucket it reports the
// largest finite bound.
func (s httpSample) quantile(q float64) time.Duration {
	les := make([]float64, 0, len(s.buckets))
	for le := range s.buckets {
		les = append(les, le)
	}
	sort.Float64s(les)
	if len(les) == 0 {
		return 0
	}
	total := s.buckets[les[len(les)-1]]
	if total <= 0 {
		return 0
	}
	target := q * total
	finite := 0.0
	for _, le := range les {
		if !math.IsInf(le, 1) {
			finite = le
		}
		if s.buckets[le] >= target {
			if math.IsInf(le, 1) {
				return time.Duration(finite * float64(time.Second))
			}
			return time.Duration(le * float64(time.Second))
		}
	}
	return time.Duration(finite * float64(time.Second))
}

const (
	familyRequests = "unheaded_http_requests_total"
	familyBuckets  = "unheaded_http_request_duration_seconds_bucket"
)

// parseHTTPMetrics reads the two house families from Prometheus text.
// Every other family and every comment is skipped; a malformed line of
// the two families is an error (a garbled scrape is not zero traffic).
func parseHTTPMetrics(r io.Reader) (httpSample, error) {
	s := newSample()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 64<<10)
	lines := 0
	for sc.Scan() {
		lines++
		if lines > maxScrapeLines {
			return s, errors.New("too many lines")
		}
		line := sc.Text()
		var family string
		switch {
		case strings.HasPrefix(line, familyRequests+"{"):
			family = familyRequests
		case strings.HasPrefix(line, familyBuckets+"{"):
			family = familyBuckets
		default:
			continue
		}
		labels, value, err := parseSampleLine(line[len(family):])
		if err != nil {
			return s, fmt.Errorf("%s: %w", family, err)
		}
		if excludedPaths[labels["path"]] {
			continue
		}
		if family == familyRequests {
			s.requests += value
			if strings.HasPrefix(labels["status"], "5") {
				s.errors += value
			}
			continue
		}
		le, err := strconv.ParseFloat(labels["le"], 64)
		if err != nil || math.IsNaN(le) {
			return s, fmt.Errorf("bucket le %q", labels["le"])
		}
		s.buckets[le] += value
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	return s, nil
}

// parseSampleLine parses `{k="v",...} value [timestamp]`.
func parseSampleLine(rest string) (map[string]string, float64, error) {
	if !strings.HasPrefix(rest, "{") {
		return nil, 0, errors.New("no labels")
	}
	labels := map[string]string{}
	i := 1
	for {
		if i >= len(rest) {
			return nil, 0, errors.New("unclosed labels")
		}
		if rest[i] == '}' {
			i++
			break
		}
		eq := strings.IndexByte(rest[i:], '=')
		if eq <= 0 || i+eq+1 >= len(rest) || rest[i+eq+1] != '"' {
			return nil, 0, errors.New("bad label")
		}
		key := rest[i : i+eq]
		i += eq + 2
		var val strings.Builder
		for {
			if i >= len(rest) {
				return nil, 0, errors.New("unclosed label value")
			}
			c := rest[i]
			if c == '\\' && i+1 < len(rest) {
				val.WriteByte(rest[i+1])
				i += 2
				continue
			}
			if c == '"' {
				i++
				break
			}
			val.WriteByte(c)
			i++
		}
		labels[key] = val.String()
		if i < len(rest) && rest[i] == ',' {
			i++
		}
	}
	fields := strings.Fields(rest[i:])
	if len(fields) < 1 || len(fields) > 2 {
		return nil, 0, errors.New("no value")
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil, 0, fmt.Errorf("value %q", fields[0])
	}
	return labels, v, nil
}
