//go:build !integration

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"context"
	"testing"
	"time"
)

func TestDockerCollector_DegradedMode(t *testing.T) {
	// Test with non-existent socket path
	collector := NewDockerCollector()
	collector.socketPath = "/tmp/nonexistent-docker-socket-" + time.Now().Format("20060102150405")

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
		if s.Name == "unheaded_docker_available" {
			foundAvailability = true
			if s.Value != 0 {
				t.Errorf("Expected availability=0 in degraded mode, got %f", s.Value)
			}
			if s.Labels == nil || s.Labels["collector"] != "docker" {
				t.Error("Expected collector=docker label")
			}
			break
		}
	}

	if !foundAvailability {
		t.Error("Expected unheaded_docker_available sample in degraded mode")
	}
}

func TestDockerCollector_Name(t *testing.T) {
	collector := NewDockerCollector()
	if name := collector.Name(); name != "docker" {
		t.Errorf("Name() = %q, want %q", name, "docker")
	}
}

func TestDockerCollector_Describe(t *testing.T) {
	collector := NewDockerCollector()
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

func TestCalculateCPUPercent(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() dockerStats
		wantMin float64
		wantMax float64
	}{
		{
			name: "cpu usage increased",
			setup: func() dockerStats {
				var stats dockerStats
				stats.CPUStats.CPUUsage.TotalUsage = 1000000000
				stats.CPUStats.SystemCPUUsage = 10000000000
				stats.PreviousCPUStats.CPUUsage.TotalUsage = 900000000
				stats.PreviousCPUStats.SystemCPUUsage = 9000000000
				return stats
			},
			wantMin: 0,
			wantMax: 50,
		},
		{
			name: "no change",
			setup: func() dockerStats {
				var stats dockerStats
				stats.CPUStats.CPUUsage.TotalUsage = 1000000000
				stats.CPUStats.SystemCPUUsage = 10000000000
				stats.PreviousCPUStats.CPUUsage.TotalUsage = 1000000000
				stats.PreviousCPUStats.SystemCPUUsage = 10000000000
				return stats
			},
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewDockerCollector()
			stats := tt.setup()
			result := collector.calculateCPUPercent(stats)

			if result < tt.wantMin || result > tt.wantMax {
				t.Errorf("calculateCPUPercent() = %f, want between %f and %f", result, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSumNetworkStats(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() dockerStats
		wantRXSum uint64
		wantTXSum uint64
	}{
		{
			name: "multiple interfaces",
			setup: func() dockerStats {
				var stats dockerStats
				stats.Networks = make(map[string]struct {
					RXBytes uint64 `json:"rx_bytes"`
					TXBytes uint64 `json:"tx_bytes"`
				})
				stats.Networks["eth0"] = struct {
					RXBytes uint64 `json:"rx_bytes"`
					TXBytes uint64 `json:"tx_bytes"`
				}{RXBytes: 1000, TXBytes: 2000}
				stats.Networks["eth1"] = struct {
					RXBytes uint64 `json:"rx_bytes"`
					TXBytes uint64 `json:"tx_bytes"`
				}{RXBytes: 500, TXBytes: 800}
				return stats
			},
			wantRXSum: 1500,
			wantTXSum: 2800,
		},
		{
			name: "no interfaces",
			setup: func() dockerStats {
				var stats dockerStats
				stats.Networks = make(map[string]struct {
					RXBytes uint64 `json:"rx_bytes"`
					TXBytes uint64 `json:"tx_bytes"`
				})
				return stats
			},
			wantRXSum: 0,
			wantTXSum: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewDockerCollector()
			stats := tt.setup()

			rxSum := collector.sumNetworkRX(stats)
			txSum := collector.sumNetworkTX(stats)

			if rxSum != tt.wantRXSum {
				t.Errorf("sumNetworkRX() = %d, want %d", rxSum, tt.wantRXSum)
			}

			if txSum != tt.wantTXSum {
				t.Errorf("sumNetworkTX() = %d, want %d", txSum, tt.wantTXSum)
			}
		})
	}
}

func TestNewDockerCollector(t *testing.T) {
	collector := NewDockerCollector()

	if collector == nil {
		t.Fatal("NewDockerCollector() returned nil")
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
