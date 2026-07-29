// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Lich Security — offensive security test suite for Unheaded.
//
// Runs 6 campaign modules (D1-D6) against the Unheaded service stack:
//   - D1: Auth Bypass
//   - D2: Injection Attacks
//   - D3: Transport Security
//   - D4: Secrets Management
//   - D5: Privilege Escalation
//   - D6: Denial of Service
//
// Usage:
//
//	lich-security -campaign d1           # Run single campaign
//	lich-security -campaign all          # Run all campaigns
//	lich-security -campaign d1,d3        # Run specific campaigns
//	lich-security -report /tmp/report.md # Set report output path
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"unheaded/cmd/lich-security/campaigns"
)

func main() {
	campaignFlag := flag.String("campaign", "all", "Campaign(s) to run: d1,d2,d3,d4,d5,d6 or all")
	reportPath := flag.String("report", "/tmp/lich-reports/S39-SECURITY-AUDIT.md", "Report output path")
	gatewayAddr := flag.String("gateway", "http://localhost:21000", "Gateway base URL")
	wotanAddr := flag.String("wotan", "localhost:18001", "Wotan gRPC address")
	dashboardAddr := flag.String("dashboard", "http://localhost:20000", "Dashboard base URL")
	timeout := flag.Duration("timeout", 5*time.Minute, "Campaign timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cfg := campaigns.Config{
		GatewayAddr:   *gatewayAddr,
		WotanAddr:     *wotanAddr,
		DashboardAddr: *dashboardAddr,
	}

	runner := campaigns.NewRunner(cfg)

	// Parse campaign selection.
	selected := parseCampaigns(*campaignFlag)

	fmt.Println("=== LICH SECURITY TEST SUITE ===")
	fmt.Printf("Campaigns: %s\n", strings.Join(selected, ", "))
	fmt.Printf("Report: %s\n", *reportPath)
	fmt.Println()

	results := runner.Run(ctx, selected)

	// Print summary.
	totalFindings := 0
	for _, r := range results {
		status := "PASS"
		if len(r.Findings) > 0 {
			status = "FAIL"
		}
		fmt.Printf("[%s] %s: %s (%d findings, %s)\n",
			status, r.Campaign, r.Description, len(r.Findings), r.Duration.Round(time.Millisecond))
		totalFindings += len(r.Findings)
	}

	fmt.Printf("\nTotal findings: %d\n", totalFindings)

	// Write report.
	report := campaigns.GenerateReport(results)
	if err := writeReport(*reportPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Report written to %s\n", *reportPath)

	if totalFindings > 0 {
		os.Exit(1)
	}
}

func parseCampaigns(s string) []string {
	if s == "all" {
		return []string{"d1", "d2", "d3", "d4", "d5", "d6"}
	}
	var result []string
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(strings.ToLower(c))
		if c != "" {
			result = append(result, c)
		}
	}
	return result
}

func writeReport(path, content string) error {
	dir := path[:strings.LastIndex(path, "/")]
	if dir != "" {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0640) // #nosec G306 -- 0640 — group-readable only, no world access
}
