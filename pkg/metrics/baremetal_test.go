//go:build !integration

// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestParseProcMeminfo(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantKeys []string
	}{
		{
			name: "typical meminfo",
			content: `MemTotal:        8167848 kB
MemFree:         4458204 kB
MemAvailable:    5123456 kB
Buffers:          224896 kB
Cached:           456789 kB`,
			wantKeys: []string{"total", "free", "available", "buffers", "cached"},
		},
		{
			name:     "empty content",
			content:  "",
			wantKeys: []string{},
		},
		{
			name: "partial content",
			content: `MemTotal:        8167848 kB
MemFree:         4458204 kB`,
			wantKeys: []string{"total", "free"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBareMetalCollector()
			result := b.parseProcMeminfo(tt.content)

			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q not found in result", key)
				}
			}
		})
	}
}

func TestParseProcLoadavg(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int // expected length of result
	}{
		{
			name:    "typical loadavg",
			content: "1.23 0.45 0.67 2/256 12345",
			want:    3,
		},
		{
			name:    "low load",
			content: "0.00 0.00 0.00 1/128 1234",
			want:    3,
		},
		{
			name:    "incomplete content",
			content: "1.23 0.45",
			want:    0, // requires 3+ fields
		},
		{
			name:    "empty content",
			content: "",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBareMetalCollector()
			result := b.parseProcLoadavg(tt.content)

			if len(result) != tt.want {
				t.Errorf("parseProcLoadavg() got %d items, want %d", len(result), tt.want)
			}

			for _, v := range result {
				if v < 0 {
					t.Errorf("expected non-negative load average, got %f", v)
				}
			}
		})
	}
}

func TestParseProcStat(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int // expected number of cpu time values
	}{
		{
			name: "typical cpu line",
			content: `cpu  2255 34 33195 22625563 6290 127 456 0 0 0
cpu0 1132 16 17875 11311718 3675 110 341 0 0 0`,
			want: 7,
		},
		{
			name:    "minimal cpu line",
			content: "cpu 100 10 50 900 20 5 10 0 0 0",
			want:    7,
		},
		{
			name:    "no cpu line",
			content: "intr 123456789",
			want:    0,
		},
		{
			name:    "empty content",
			content: "",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBareMetalCollector()
			result := b.parseProcStat(tt.content)

			if len(result) != tt.want {
				t.Errorf("parseProcStat() got %d values, want %d", len(result), tt.want)
			}

			for _, v := range result {
				if v < 0 {
					t.Errorf("expected non-negative cpu time, got %f", v)
				}
			}
		})
	}
}

func TestParseNetStats(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(tmpDir string) error
		ifName    string
		wantStats []string
	}{
		{
			name: "complete stats",
			setupFunc: func(tmpDir string) error {
				ifDir := filepath.Join(tmpDir, "eth0", "statistics")
				if err := os.MkdirAll(ifDir, 0755); err != nil {
					return err
				}

				stats := map[string]string{
					"rx_bytes":   "1234567890",
					"rx_packets": "987654",
					"tx_bytes":   "9876543210",
					"tx_packets": "123456",
				}

				for name, value := range stats {
					if err := ioutil.WriteFile(filepath.Join(ifDir, name), []byte(value), 0644); err != nil {
						return err
					}
				}
				return nil
			},
			ifName:    "eth0",
			wantStats: []string{"rx_bytes", "rx_packets", "tx_bytes", "tx_packets"},
		},
		{
			name: "partial stats",
			setupFunc: func(tmpDir string) error {
				ifDir := filepath.Join(tmpDir, "eth1", "statistics")
				if err := os.MkdirAll(ifDir, 0755); err != nil {
					return err
				}

				stats := map[string]string{
					"rx_bytes": "1000000",
					"rx_packets": "5000",
				}

				for name, value := range stats {
					if err := ioutil.WriteFile(filepath.Join(ifDir, name), []byte(value), 0644); err != nil {
						return err
					}
				}
				return nil
			},
			ifName:    "eth1",
			wantStats: []string{"rx_bytes", "rx_packets"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			if err := tt.setupFunc(testDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			b := NewBareMetalCollector()
			result := b.parseNetStats(testDir, tt.ifName)

			for _, stat := range tt.wantStats {
				if _, ok := result[stat]; !ok {
					t.Errorf("expected stat %q not found in result", stat)
				}
			}

			for _, v := range result {
				if v < 0 {
					t.Errorf("expected non-negative stat value, got %f", v)
				}
			}
		})
	}
}

func TestBareMetalCollector_Name(t *testing.T) {
	b := NewBareMetalCollector()
	if name := b.Name(); name != "baremetal" {
		t.Errorf("Name() = %q, want %q", name, "baremetal")
	}
}

func TestBareMetalCollector_Describe(t *testing.T) {
	b := NewBareMetalCollector()
	desc := b.Describe()

	if len(desc) == 0 {
		t.Error("Describe() returned empty list")
	}

	for _, d := range desc {
		if d == "" {
			t.Error("Describe() contains empty string")
		}
	}
}

func TestBareMetalCollector_Collect(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal /proc structure
	procDir := filepath.Join(tmpDir, "proc")
	if err := os.MkdirAll(procDir, 0755); err != nil {
		t.Fatalf("failed to create proc dir: %v", err)
	}

	// Create minimal /sys structure
	sysDir := filepath.Join(tmpDir, "sys", "class", "net")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatalf("failed to create sys dir: %v", err)
	}

	// Create meminfo file
	if err := ioutil.WriteFile(filepath.Join(procDir, "meminfo"), []byte("MemTotal:        8167848 kB\nMemFree:         4458204 kB\n"), 0644); err != nil {
		t.Fatalf("failed to create meminfo: %v", err)
	}

	// Create loadavg file
	if err := ioutil.WriteFile(filepath.Join(procDir, "loadavg"), []byte("0.50 0.45 0.42 2/256 12345\n"), 0644); err != nil {
		t.Fatalf("failed to create loadavg: %v", err)
	}

	// Create stat file
	if err := ioutil.WriteFile(filepath.Join(procDir, "stat"), []byte("cpu  100 10 50 900 20 5 10 0 0 0\n"), 0644); err != nil {
		t.Fatalf("failed to create stat: %v", err)
	}

	b := NewBareMetalCollector()
	b.procRoot = procDir
	b.sysRoot = filepath.Join(tmpDir, "sys")

	ctx := context.Background()
	samples, err := b.Collect(ctx)

	if err != nil {
		t.Errorf("Collect() error = %v", err)
	}

	if len(samples) == 0 {
		t.Error("Collect() returned no samples")
	}

	for _, s := range samples {
		if s.Labels == nil {
			t.Error("sample has nil Labels")
		}
		if s.Labels["collector"] != "baremetal" {
			t.Errorf("expected collector=baremetal label, got %v", s.Labels)
		}
	}
}
