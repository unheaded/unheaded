// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// LXDCollector gathers metrics from the LXD daemon via its Unix socket.
// It operates in a graceful degraded mode: if the socket is unavailable,
// it returns a single availability metric set to 0 instead of failing.
type LXDCollector struct {
	// socketPath is the path to the LXD Unix socket (default: "/var/lib/lxd/unix.socket")
	socketPath string
	// timeout for HTTP requests to LXD
	timeout time.Duration
}

// NewLXDCollector creates a new LXD collector.
func NewLXDCollector() *LXDCollector {
	return &LXDCollector{
		socketPath: "/var/lib/lxd/unix.socket",
		timeout:    5 * time.Second,
	}
}

// Name returns the collector's name.
func (l *LXDCollector) Name() string {
	return "lxd"
}

// Describe returns descriptions of the metrics this collector produces.
func (l *LXDCollector) Describe() []string {
	return []string{
		"unheaded_lxd_available - Whether LXD metrics are available (0 or 1)",
		"unheaded_lxd_metrics - LXD container and system metrics",
	}
}

// Collect gathers metrics from LXD or returns availability=0 if unavailable.
func (l *LXDCollector) Collect(ctx context.Context) ([]Sample, error) {
	var samples []Sample
	baseLabels := map[string]string{"collector": "lxd"}

	// Try to fetch metrics from LXD
	resp, err := l.fetchMetrics(ctx)
	if err != nil {
		// Graceful degradation: return availability=0 instead of failing
		fmt.Fprintf(os.Stderr, "Warning: LXD metrics unavailable: %v\n", err)
		samples = append(samples, Sample{
			Name:   "unheaded_lxd_available",
			Help:   "LXD metrics availability",
			Type:   TypeGauge,
			Labels: baseLabels,
			Value:  0,
		})
		return samples, nil
	}

	// Parse the response
	parsed := l.parsePrometheusFormat(resp)
	for _, s := range parsed {
		// Add collector label if not already present
		if s.Labels == nil {
			s.Labels = make(map[string]string)
		}
		s.Labels["collector"] = "lxd"
		samples = append(samples, s)
	}

	// Add availability=1 indicator
	samples = append(samples, Sample{
		Name:   "unheaded_lxd_available",
		Help:   "LXD metrics availability",
		Type:   TypeGauge,
		Labels: baseLabels,
		Value:  1,
	})

	return samples, nil
}

// fetchMetrics retrieves metrics from the LXD daemon.
func (l *LXDCollector) fetchMetrics(ctx context.Context) (string, error) {
	// Check if socket exists
	if _, err := os.Stat(l.socketPath); err != nil {
		return "", fmt.Errorf("socket not accessible: %w", err)
	}

	// Create HTTP client with Unix socket transport
	client := &http.Client{
		Timeout: l.timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", l.socketPath)
			},
		},
	}

	// Make request to LXD metrics endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/1.0/metrics", nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LXD returned status %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	return string(body), nil
}

// parsePrometheusFormat parses Prometheus text exposition format.
func (l *LXDCollector) parsePrometheusFormat(text string) []Sample {
	var samples []Sample

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		sample := l.parseMetricLine(line)
		if sample != nil {
			samples = append(samples, *sample)
		}
	}

	return samples
}

// parseMetricLine parses a single Prometheus metric line.
// Format: metric_name{label1="value1",label2="value2"} value timestamp
func (l *LXDCollector) parseMetricLine(line string) *Sample {
	// Find where labels start
	labelStart := strings.Index(line, "{")
	labelEnd := strings.Index(line, "}")

	var name string
	var labels map[string]string

	if labelStart > 0 && labelEnd > labelStart {
		// Has labels
		name = line[:labelStart]
		labelStr := line[labelStart+1 : labelEnd]
		labels = l.parseLabels(labelStr)

		// Parse value and optional timestamp
		rest := strings.TrimSpace(line[labelEnd+1:])
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			return nil
		}

		value, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return nil
		}

		var timestamp int64
		if len(parts) > 1 {
			if ts, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				timestamp = ts
			}
		}

		return &Sample{
			Name:      name,
			Type:      TypeGauge,
			Labels:    labels,
			Value:     value,
			Timestamp: timestamp,
		}
	} else if labelStart < 0 {
		// No labels
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil
		}

		name = parts[0]
		value, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil
		}

		var timestamp int64
		if len(parts) > 2 {
			if ts, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
				timestamp = ts
			}
		}

		return &Sample{
			Name:      name,
			Type:      TypeGauge,
			Labels:    make(map[string]string),
			Value:     value,
			Timestamp: timestamp,
		}
	}

	return nil
}

// parseLabels parses label string: label1="value1",label2="value2"
func (l *LXDCollector) parseLabels(labelStr string) map[string]string {
	labels := make(map[string]string)

	// Simple parser: split by comma, but respect quoted values
	var current strings.Builder
	var inQuotes bool
	var escaped bool

	for i := 0; i < len(labelStr); i++ {
		ch := labelStr[i]

		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			current.WriteByte(ch)
			escaped = true
			continue
		}

		if ch == '"' {
			inQuotes = !inQuotes
			current.WriteByte(ch)
			continue
		}

		if ch == ',' && !inQuotes {
			// Parse the label pair
			pair := current.String()
			if key, val, ok := strings.Cut(pair, "="); ok {
				key = strings.TrimSpace(key)
				val = strings.Trim(val, "\"")
				labels[key] = val
			}
			current.Reset()
			continue
		}

		current.WriteByte(ch)
	}

	// Parse the last label pair
	pair := current.String()
	if key, val, ok := strings.Cut(pair, "="); ok {
		key = strings.TrimSpace(key)
		val = strings.Trim(val, "\"")
		labels[key] = val
	}

	return labels
}
