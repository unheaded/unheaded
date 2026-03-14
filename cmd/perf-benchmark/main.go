// SPDX-License-Identifier: MIT

// perf-benchmark is the Unheaded performance benchmarking harness.
// It measures latency across the P2P link, WireGuard overlay, HTTP services,
// and Wotan publish path.
//
// Usage:
//
//	go run ./cmd/perf-benchmark [flags]
//	  -iterations int   Number of iterations per test (default 50)
//	  -output string    JSON output file (default "benchmark-results.json")
//	  -east-p2p string  EAST P2P IPv4 address (default "192.168.13.1")
//	  -east-wg string   EAST WireGuard IPv6 address (default "fd00:dead:beef::1")
//	  -health string    Health endpoint URL (default "http://localhost:19000/health")
//	  -wotan string     Wotan HTTP endpoint (default "http://localhost:18000")
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"unheaded/pkg/benchmark"
)

var (
	iterations = flag.Int("iterations", 50, "Number of iterations per test")
	output     = flag.String("output", "benchmark-results.json", "JSON output file")
	eastP2P    = flag.String("east-p2p", "192.168.13.1", "EAST P2P IPv4 address")
	eastWG     = flag.String("east-wg", "fd00:dead:beef::1", "EAST WireGuard IPv6 address")
	healthURL  = flag.String("health", "http://localhost:19000/health", "Health endpoint URL")
	wotanURL   = flag.String("wotan", "http://localhost:18000", "Wotan HTTP endpoint")
)

func main() {
	flag.Parse()

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
		out, err := exec.Command("ping", "-c", "1", "-W", "3", *eastP2P).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ping %s: %w (output: %s)", *eastP2P, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// wireguardIPv6Latency measures ICMP6 ping latency over the WireGuard tunnel.
func wireguardIPv6Latency(iterations int) error {
	for i := 0; i < iterations; i++ {
		out, err := exec.Command("ping6", "-c", "1", "-W", "3", *eastWG).CombinedOutput()
		if err != nil {
			// Try ping -6 as fallback (some systems use ping -6 instead of ping6)
			out2, err2 := exec.Command("ping", "-6", "-c", "1", "-W", "3", *eastWG).CombinedOutput()
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
		resp.Body.Close()
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
		resp.Body.Close()

		// Accept 200-299 as success (Wotan might return 201 or 202)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("POST %s returned %d", publishURL, resp.StatusCode)
		}
	}
	return nil
}

