// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// runHTTP3 times -iterations GETs of -http3-url over HTTP/3 only, with
// curl. The module has no Go HTTP/3 client (quic-go is not a dependency,
// and adding one needs an ADR-004 check), so without a curl built with
// HTTP3 the scenario is skipped and says why.
func runHTTP3(ctx context.Context, cfg Config) Result {
	r := Result{Name: scenarioHTTP3, Target: cfg.HTTP3URL}
	curl := cfg.Curl
	if curl == "" {
		curl = "curl"
	}
	ver, err := exec.CommandContext(ctx, curl, "--version").Output() // #nosec G204 -- curl path from an operator flag, fixed argv
	if err != nil {
		r.Skipped = fmt.Sprintf("curl not runnable (%v); no Go HTTP/3 client in go.mod", err)
		return r
	}
	if !curlHasHTTP3(string(ver)) {
		r.Skipped = "curl lacks HTTP3 (Features line) and no Go HTTP/3 client is in go.mod (quic-go not a dependency)"
		return r
	}

	var lat []time.Duration
	errs := 0
	first := ""
	for i := 0; i < cfg.Iterations; i++ {
		if ctx.Err() != nil {
			break
		}
		d, err := curlHTTP3Once(ctx, curl, cfg)
		if err != nil {
			errs++
			if first == "" {
				first = err.Error()
			}
			continue
		}
		lat = append(lat, d)
	}
	out := summarize(scenarioHTTP3, lat, errs)
	out.Target = cfg.HTTP3URL
	out.FirstError = first
	out.Note = "curl --http3-only time_total (includes the QUIC handshake: one new connection per request)"
	return out
}

func curlHasHTTP3(version string) bool {
	for _, line := range strings.Split(version, "\n") {
		if strings.HasPrefix(line, "Features:") {
			for _, f := range strings.Fields(line) {
				if f == "HTTP3" {
					return true
				}
			}
		}
	}
	return false
}

func curlHTTP3Once(ctx context.Context, curl string, cfg Config) (time.Duration, error) {
	args := []string{"--http3-only", "-s", "-o", "/dev/null", "--max-time", "5", "-w", "%{http_code} %{time_total}"}
	if cfg.HTTP3Insecure {
		args = append(args, "-k")
	}
	args = append(args, "--", cfg.HTTP3URL)
	out, err := exec.CommandContext(ctx, curl, args...).Output() // #nosec G204 -- curl path and URL from operator flags, argv separate, URL after --
	if err != nil {
		return 0, fmt.Errorf("curl --http3-only %s: %w", cfg.HTTP3URL, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return 0, fmt.Errorf("curl output %q", string(out))
	}
	code, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, fmt.Errorf("curl status %q", fields[0])
	}
	if code < 200 || code > 299 {
		return 0, fmt.Errorf("GET %s over HTTP/3: %d", cfg.HTTP3URL, code)
	}
	secs, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || secs < 0 {
		return 0, fmt.Errorf("curl time_total %q", fields[1])
	}
	return time.Duration(secs * float64(time.Second)), nil
}
