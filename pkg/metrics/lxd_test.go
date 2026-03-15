//go:build !integration

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"context"
	"testing"
	"time"
)

func TestLXDCollector_DegradedMode(t *testing.T) {
	// Test with non-existent socket path (degraded mode)
	collector := NewLXDCollector()
	collector.socketPath = "/tmp/nonexistent-lxd-socket-" + time.Now().Format("20060102150405")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	samples, err := collector.Collect(ctx)

	// Should not return error in degraded mode
	if err != nil {
		t.Errorf("Collect() should not error in degraded mode, got: %v", err)
	}

	// Should return at least the availability sample
	if len(samples) < 1 {
		t.Fatal("Collect() should return at least one sample in degraded mode")
	}

	// Find availability sample
	foundAvailability := false
	for _, s := range samples {
		if s.Name == "unheaded_lxd_available" {
			foundAvailability = true
			if s.Value != 0 {
				t.Errorf("Expected availability=0 in degraded mode, got %f", s.Value)
			}
			if s.Labels == nil || s.Labels["collector"] != "lxd" {
				t.Error("Expected collector=lxd label")
			}
			break
		}
	}

	if !foundAvailability {
		t.Error("Expected unheaded_lxd_available sample in degraded mode")
	}
}

func TestLXDCollector_Name(t *testing.T) {
	collector := NewLXDCollector()
	if name := collector.Name(); name != "lxd" {
		t.Errorf("Name() = %q, want %q", name, "lxd")
	}
}

func TestLXDCollector_Describe(t *testing.T) {
	collector := NewLXDCollector()
	desc := collector.Describe()

	if len(desc) == 0 {
		t.Error("Describe() returned empty list")
	}

	for _, d := range desc {
		if d == "" {
			t.Error("Describe() contains empty string")
		}
	}
}

func TestParsePrometheusFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected number of samples
	}{
		{
			name: "simple metrics",
			input: `# HELP lxd_cpu_seconds Total CPU seconds
# TYPE lxd_cpu_seconds counter
lxd_cpu_seconds{container="web"} 123.45
lxd_memory_bytes{container="db"} 1024000`,
			want: 2,
		},
		{
			name: "with timestamp",
			input: `metric1{label="value"} 42.5 1609459200000
metric2 100 1609459200000`,
			want: 2,
		},
		{
			name: "only comments",
			input: `# HELP metric help text
# TYPE metric counter`,
			want: 0,
		},
		{
			name: "empty input",
			input: "",
			want: 0,
		},
		{
			name: "single metric without labels",
			input: "unheaded_metric 42",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewLXDCollector()
			samples := collector.parsePrometheusFormat(tt.input)

			if len(samples) != tt.want {
				t.Errorf("parsePrometheusFormat() got %d samples, want %d", len(samples), tt.want)
			}
		})
	}
}

func TestParseMetricLine(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantName  string
		wantValue float64
		wantOK    bool
	}{
		{
			name:      "metric with labels",
			input:     `metric_name{label1="value1",label2="value2"} 42.5`,
			wantName:  "metric_name",
			wantValue: 42.5,
			wantOK:    true,
		},
		{
			name:      "metric without labels",
			input:     "simple_metric 100",
			wantName:  "simple_metric",
			wantValue: 100,
			wantOK:    true,
		},
		{
			name:      "metric with timestamp",
			input:     `metric{x="y"} 77.7 1234567890`,
			wantName:  "metric",
			wantValue: 77.7,
			wantOK:    true,
		},
		{
			name:      "invalid value",
			input:     `metric{x="y"} invalid`,
			wantOK:    false,
		},
		{
			name:      "empty input",
			input:     "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewLXDCollector()
			sample := collector.parseMetricLine(tt.input)

			if (sample != nil) != tt.wantOK {
				t.Errorf("parseMetricLine() ok = %v, want %v", sample != nil, tt.wantOK)
				return
			}

			if sample != nil {
				if sample.Name != tt.wantName {
					t.Errorf("Name = %q, want %q", sample.Name, tt.wantName)
				}
				if sample.Value != tt.wantValue {
					t.Errorf("Value = %f, want %f", sample.Value, tt.wantValue)
				}
			}
		})
	}
}

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantKV    map[string]string
	}{
		{
			name:      "simple labels",
			input:     `label1="value1",label2="value2"`,
			wantCount: 2,
			wantKV:    map[string]string{"label1": "value1", "label2": "value2"},
		},
		{
			name:      "single label",
			input:     `container="web"`,
			wantCount: 1,
			wantKV:    map[string]string{"container": "web"},
		},
		{
			name:      "labels with special chars",
			input:     `image="ubuntu:20.04",status="running"`,
			wantCount: 2,
			wantKV:    map[string]string{"image": "ubuntu:20.04", "status": "running"},
		},
		{
			name:      "empty input",
			input:     "",
			wantCount: 0,
			wantKV:    map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewLXDCollector()
			labels := collector.parseLabels(tt.input)

			if len(labels) != tt.wantCount {
				t.Errorf("parseLabels() got %d labels, want %d", len(labels), tt.wantCount)
			}

			for k, v := range tt.wantKV {
				if labels[k] != v {
					t.Errorf("label %q = %q, want %q", k, labels[k], v)
				}
			}
		})
	}
}

func TestNewLXDCollector(t *testing.T) {
	collector := NewLXDCollector()

	if collector == nil {
		t.Fatal("NewLXDCollector() returned nil")
	}

	if collector.socketPath == "" {
		t.Error("socketPath should be set")
	}

	if collector.timeout == 0 {
		t.Error("timeout should be set")
	}

	if collector.timeout < 1*time.Second {
		t.Errorf("timeout %v should be at least 1 second", collector.timeout)
	}
}
