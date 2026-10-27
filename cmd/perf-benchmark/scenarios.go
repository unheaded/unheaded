// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

// The scenarios behind -scenario (S-WELL-SPRING Phase 10, M-089). Each
// returns a Result; the run writes one perf/v1 report that
// scripts/check-perf-regression.sh compares with docs/policy/perf-baseline.json.

const (
	reportSchema = "perf/v1"

	scenarioE2E            = "e2e-trace"
	scenarioContainerStart = "container-start"
	scenarioHTTPLoad       = "http-load"
	scenarioHTTP3          = "http3"
)

// allScenarios is the order "all" runs them in.
var allScenarios = []string{scenarioE2E, scenarioContainerStart, scenarioHTTPLoad, scenarioHTTP3}

// Result is one scenario's outcome. Durations are nanoseconds in JSON.
type Result struct {
	Name         string        `json:"name"`
	Target       string        `json:"target,omitempty"`
	N            int           `json:"n"`        // successful samples
	Attempts     int           `json:"attempts"` // samples tried
	Errors       int           `json:"errors"`
	P50          time.Duration `json:"p50"`
	P95          time.Duration `json:"p95"`
	P99          time.Duration `json:"p99"`
	ErrorRatio   float64       `json:"error_ratio"`
	RateAchieved float64       `json:"rate_achieved,omitempty"` // http-load: completed requests per second
	Skipped      string        `json:"skipped,omitempty"`       // why the scenario was not measured
	Note         string        `json:"note,omitempty"`
	FirstError   string        `json:"first_error,omitempty"`
}

// Report is the perf/v1 document -output receives.
type Report struct {
	Schema       string    `json:"schema"`
	Host         string    `json:"host"`
	Commit       string    `json:"commit"`
	At           time.Time `json:"at"`
	DurationUnit string    `json:"duration_unit"`
	Results      []Result  `json:"results"`
}

// Config carries every scenario's settings (flags in main.go).
type Config struct {
	Iterations int
	Poll       time.Duration // re-read interval while waiting for something to appear

	// http-load
	HealthURL   string
	Rate        int
	Duration    time.Duration
	Concurrency int

	// e2e-trace
	WotanURL     string
	DashboardURL string
	E2ETopic     string
	WotanName    string
	E2ETimeout   time.Duration

	// container-start
	AllowLXD   bool
	CuirassURL string
	CuirassKey string
	CSImage    string
	CSTimeout  time.Duration

	// http3
	HTTP3URL      string
	Curl          string
	HTTP3Insecure bool
}

func newReport(host, commit string, at time.Time, results []Result) Report {
	return Report{Schema: reportSchema, Host: host, Commit: commit, At: at.UTC(), DurationUnit: "ns", Results: results}
}

// parseScenarios splits -scenario, expands "all" and drops repeats. viaAll
// reports whether "all" was named (container-start then skips instead of
// refusing when -allow-lxd is absent).
func parseScenarios(s string) (names []string, viaAll bool, err error) {
	known := map[string]bool{}
	for _, n := range allScenarios {
		known[n] = true
	}
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, part := range strings.Split(s, ",") {
		n := strings.TrimSpace(part)
		switch {
		case n == "":
			continue
		case n == "all":
			viaAll = true
			for _, a := range allScenarios {
				add(a)
			}
		case known[n]:
			add(n)
		default:
			return nil, false, fmt.Errorf("unknown scenario %q (want %s or all)", n, strings.Join(allScenarios, ", "))
		}
	}
	if len(names) == 0 {
		return nil, false, errors.New("no scenario named")
	}
	return names, viaAll, nil
}

// validate refuses a run that cannot be what was asked for, before
// anything is sent anywhere.
func validate(names []string, viaAll bool, cfg Config) error {
	for _, n := range names {
		switch n {
		case scenarioContainerStart:
			if !cfg.AllowLXD {
				if viaAll {
					continue // skipped, with the reason, in its result
				}
				return errors.New("container-start creates and deletes an LXD instance through Cuirass; pass -allow-lxd (host window only)")
			}
			if cfg.CuirassKey == "" {
				return errors.New("container-start: CUIRASS_API_KEY is not set; credentials are not stored in the repo")
			}
		case scenarioHTTPLoad:
			if cfg.Rate < 1 || cfg.Duration <= 0 {
				return fmt.Errorf("http-load: rate %d/s for %v is not a load", cfg.Rate, cfg.Duration)
			}
		case scenarioE2E, scenarioHTTP3:
			if cfg.Iterations < 1 {
				return fmt.Errorf("%s: -iterations must be at least 1", n)
			}
		}
	}
	return nil
}

func runScenario(ctx context.Context, name string, cfg Config) Result {
	switch name {
	case scenarioE2E:
		return runE2ETrace(ctx, cfg)
	case scenarioContainerStart:
		if !cfg.AllowLXD {
			return Result{Name: name, Skipped: "creates an LXD instance; needs -allow-lxd inside a host window"}
		}
		return runContainerStart(ctx, cfg)
	case scenarioHTTPLoad:
		return runHTTPLoad(ctx, cfg)
	case scenarioHTTP3:
		return runHTTP3(ctx, cfg)
	}
	return Result{Name: name, Errors: 1, Attempts: 1, ErrorRatio: 1, FirstError: "unknown scenario"}
}

// run runs names in order and writes the report to out.
func run(ctx context.Context, names []string, cfg Config, out, host, commit string) error {
	at := time.Now()
	var results []Result
	for _, n := range names {
		if ctx.Err() != nil {
			results = append(results, Result{Name: n, Skipped: "run canceled"})
			continue
		}
		r := runScenario(ctx, n, cfg)
		printResult(r)
		results = append(results, r)
	}
	b, err := json.MarshalIndent(newReport(host, commit, at, results), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(b, '\n'), 0o600)
}

func printResult(r Result) {
	if r.Skipped != "" {
		fmt.Printf("%-16s skipped: %s\n", r.Name, r.Skipped)
		return
	}
	fmt.Printf("%-16s n=%d/%d p50=%v p95=%v p99=%v error_ratio=%.4f", r.Name, r.N, r.Attempts,
		r.P50.Round(time.Microsecond), r.P95.Round(time.Microsecond), r.P99.Round(time.Microsecond), r.ErrorRatio)
	if r.RateAchieved > 0 {
		fmt.Printf(" rate=%.1f/s", r.RateAchieved)
	}
	fmt.Println()
	if r.FirstError != "" {
		fmt.Printf("%-16s first error: %s\n", "", r.FirstError)
	}
	if r.Note != "" {
		fmt.Printf("%-16s note: %s\n", "", r.Note)
	}
}

// summarize turns successful samples plus an error count into a Result.
// Percentiles are nearest-rank (a value that was actually observed).
func summarize(name string, samples []time.Duration, errs int) Result {
	r := Result{Name: name, N: len(samples), Errors: errs, Attempts: len(samples) + errs}
	if r.Attempts > 0 {
		r.ErrorRatio = float64(errs) / float64(r.Attempts)
	}
	if len(samples) == 0 {
		return r
	}
	s := append([]time.Duration(nil), samples...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	r.P50 = nearestRank(s, 0.50)
	r.P95 = nearestRank(s, 0.95)
	r.P99 = nearestRank(s, 0.99)
	return r
}

func nearestRank(sorted []time.Duration, p float64) time.Duration {
	idx := int(p*float64(len(sorted))+0.999999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// sleepCtx waits d or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// localClient never follows redirects and ignores proxy settings from the
// environment: every target here is a local service.
func localClient(timeout time.Duration, conns int) *http.Client {
	if conns < 2 {
		conns = 2
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{Proxy: nil, MaxIdleConns: conns, MaxIdleConnsPerHost: conns},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// commitOf names the code that ran: -commit, the binary's VCS stamp, then
// git, then "unknown".
func commitOf(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				return s.Value
			}
		}
	}
	if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil { // #nosec G204 -- fixed argv
		return strings.TrimSpace(string(out))
	}
	return "unknown"
}
