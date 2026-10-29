// SPDX-License-Identifier: GPL-3.0-or-later

// perf-benchmark is the Unheaded performance benchmarking harness.
//
// Without -scenario it runs the original suite: latency across the P2P
// link, the WireGuard overlay, one HTTP health endpoint and the Wotan
// publish path (pkg/benchmark JSON).
//
// With -scenario it runs the measured scenarios and writes a perf/v1
// report (schema, host, commit, at, results[] with n, p50, p95, p99 in ns,
// error_ratio, skipped), which scripts/check-perf-regression.sh compares
// with docs/policy/perf-baseline.json:
//
//	e2e-trace        publish a trace-id'd packet event to Wotan, time until
//	                 dashboard-backend's /api/v1/ebpf/events returns it
//	container-start  steel-echo instance via Cuirass until healthy, then
//	                 deleted (creates LXD instances: needs -allow-lxd)
//	http-load        -rate req/s for -duration against -health
//	http3            curl --http3-only against -http3-url (skipped when
//	                 curl lacks HTTP3)
//	all              the four above
//
//	go run ./cmd/perf-benchmark -scenario e2e-trace,http-load,http3 -iterations 200 -duration 60s -output perf.json
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/benchmark"
	"unheaded/pkg/cmdline"
	wotanClient "unheaded/pkg/wotan-client"
)

var (
	iterations = flag.Int("iterations", 50, "Number of iterations per test")
	output     = flag.String("output", "benchmark-results.json", "JSON output file")
	eastP2P    = flag.String("east-p2p", "192.168.13.1", "EAST P2P IPv4 address")
	eastWG     = flag.String("east-wg", "fd00:dead:beef::1", "EAST WireGuard IPv6 address")
	healthURL  = flag.String("health", "http://localhost:19000/health", "Health endpoint URL")
	wotanURL   = flag.String("wotan", "http://localhost:18000", "Wotan HTTP endpoint")

	scenario      = flag.String("scenario", "", "Comma-separated scenarios: e2e-trace, container-start, http-load, http3, all (empty: the original suite)")
	duration      = flag.Duration("duration", 60*time.Second, "http-load: how long to hold the rate")
	rate          = flag.Int("rate", 1000, "http-load: requests per second")
	concurrency   = flag.Int("concurrency", 64, "http-load: workers")
	dashboardURL  = flag.String("dashboard", "http://localhost:20000", "e2e-trace: dashboard-backend base URL")
	e2eTopic      = flag.String("e2e-topic", "bench.perf.packet", "e2e-trace: Wotan topic the bench packet is published on (dashboard-backend ingests it)")
	wotanName     = flag.String("wotan-name", "perf-benchmark", "e2e-trace: Wotan display name (must be in topics.auto_approve)")
	e2eTimeout    = flag.Duration("e2e-timeout", 5*time.Second, "e2e-trace: longest wait for one event")
	poll          = flag.Duration("poll", 5*time.Millisecond, "e2e-trace/container-start: re-read interval while waiting")
	allowLXD      = flag.Bool("allow-lxd", false, "container-start: allow creating and deleting LXD instances through Cuirass (host window only)")
	cuirassURL    = flag.String("cuirass", "http://127.0.0.1:19006", "container-start: Cuirass base URL (key in CUIRASS_API_KEY)")
	csImage       = flag.String("cs-image", "steel-echo-v1", "container-start: image to start")
	csTimeout     = flag.Duration("cs-timeout", 2*time.Minute, "container-start: longest wait for one instance to turn healthy")
	http3URL      = flag.String("http3-url", "https://localhost:21443/health", "http3: URL fetched over HTTP/3")
	http3Insecure = flag.Bool("http3-insecure", false, "http3: skip TLS verification (self-signed gateway)")
	curlPath      = flag.String("curl", "curl", "http3: curl binary")
	commitFlag    = flag.String("commit", "", "report commit (default: build info, then git rev-parse HEAD)")
)

func main() {
	cmdline.Parse(cmdline.Spec{
		Name:     "perf-benchmark",
		Purpose:  "Run the Unheaded performance benchmark suite or one named scenario.",
		Examples: []string{"perf-benchmark", "perf-benchmark -scenario e2e -output results.json"},
	})
	wotanClient.InitServiceIdentity("perf-benchmark") // ADR-103 §6: every Wotan call carries perf-benchmark's service token

	if *scenario != "" {
		os.Exit(runScenarios())
	}

	fmt.Println("==========================================================")
	fmt.Println(" Unheaded Performance Benchmark — S77 Age 2")
	fmt.Printf(" %s\n", time.Now().Format(time.RFC3339))
	fmt.Println("==========================================================")
	fmt.Println()

	suite := benchmark.NewSuite()
	suite.SetIterations(*iterations)
	suite.SetWarmup(3)

	// Register benchmark tests
	suite.AddTest("p2p-direct-latency", p2pDirectLatency)
	suite.AddTest("wireguard-ipv6-latency", wireguardIPv6Latency)
	suite.AddTest("http-health-latency", httpHealthLatency)
	suite.AddTest("wotan-publish-latency", wotanPublishLatency)

	fmt.Printf("Running %d tests with %d iterations each...\n\n", 4, *iterations)

	results := suite.Run()

	// Print summary
	fmt.Print(suite.SummaryReport())

	// Save JSON
	if err := suite.SaveJSON(*output); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save JSON: %v\n", err)
	} else {
		fmt.Printf("Results saved to %s\n", *output)
	}

	// Check for failures
	for _, r := range results {
		if r.Error != "" {
			fmt.Fprintf(os.Stderr, "WARN: %s failed: %s\n", r.Name, r.Error)
		}
	}
}

// p2pDirectLatency measures ICMP ping latency over the direct P2P link.
func p2pDirectLatency(iterations int) error {
	for i := 0; i < iterations; i++ {
		out, err := exec.Command("ping", "-c", "1", "-W", "3", *eastP2P).CombinedOutput() // #nosec G204 -- fixed ping binary; target from an operator CLI flag, argv separate
		if err != nil {
			return fmt.Errorf("ping %s: %w (output: %s)", *eastP2P, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// wireguardIPv6Latency measures ICMP6 ping latency over the WireGuard tunnel.
func wireguardIPv6Latency(iterations int) error {
	for i := 0; i < iterations; i++ {
		out, err := exec.Command("ping6", "-c", "1", "-W", "3", *eastWG).CombinedOutput() // #nosec G204 -- fixed ping binary; target from an operator CLI flag, argv separate
		if err != nil {
			// Try ping -6 as fallback (some systems use ping -6 instead of ping6)
			out2, err2 := exec.Command("ping", "-6", "-c", "1", "-W", "3", *eastWG).CombinedOutput() // #nosec G204 -- fixed ping binary; target from an operator CLI flag, argv separate
			if err2 != nil {
				return fmt.Errorf("ping6 %s: %w (output: %s / %s)", *eastWG, err, strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
			}
		}
		_ = out
	}
	return nil
}

// httpHealthLatency measures HTTP GET latency to a service health endpoint.
func httpHealthLatency(iterations int) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for i := 0; i < iterations; i++ {
		resp, err := client.Get(*healthURL)
		if err != nil {
			return fmt.Errorf("GET %s: %w", *healthURL, err)
		}
		resp.Body.Close() // #nosec G104 -- draining and closing a response body; a close error cannot change the outcome
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GET %s returned %d", *healthURL, resp.StatusCode)
		}
	}
	return nil
}

// wotanPublishLatency measures the latency of publishing a message to Wotan via HTTP.
func wotanPublishLatency(iterations int) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	publishURL := *wotanURL + "/api/v1/publish"

	for i := 0; i < iterations; i++ {
		payload := fmt.Sprintf(`{"topic":"benchmark.test","data":{"iteration":%d,"timestamp":%d}}`,
			i, time.Now().UnixNano())

		resp, err := client.Post(publishURL, "application/json", strings.NewReader(payload))
		if err != nil {
			return fmt.Errorf("POST %s: %w", publishURL, err)
		}
		resp.Body.Close() // #nosec G104 -- draining and closing a response body; a close error cannot change the outcome

		// Accept 200-299 as success (Wotan might return 201 or 202)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("POST %s returned %d", publishURL, resp.StatusCode)
		}
	}
	return nil
}

// runScenarios is the -scenario path; it returns the exit code.
func runScenarios() int {
	names, viaAll, err := parseScenarios(*scenario)
	if err != nil {
		fmt.Fprintln(os.Stderr, "perf-benchmark:", err)
		return 2
	}
	cfg := Config{
		Iterations: *iterations, Poll: *poll,
		HealthURL: *healthURL, Rate: *rate, Duration: *duration, Concurrency: *concurrency,
		WotanURL: *wotanURL, DashboardURL: *dashboardURL, E2ETopic: *e2eTopic, WotanName: *wotanName, E2ETimeout: *e2eTimeout,
		AllowLXD: *allowLXD, CuirassURL: *cuirassURL, CuirassKey: os.Getenv("CUIRASS_API_KEY"), CSImage: *csImage, CSTimeout: *csTimeout,
		HTTP3URL: *http3URL, Curl: *curlPath, HTTP3Insecure: *http3Insecure,
	}
	if err := validate(names, viaAll, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "perf-benchmark:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	host, _ := os.Hostname()
	if err := run(ctx, names, cfg, *output, host, commitOf(*commitFlag)); err != nil {
		fmt.Fprintln(os.Stderr, "perf-benchmark:", err)
		return 1
	}
	fmt.Printf("Report (perf/v1) saved to %s\n", *output)
	return 0
}
