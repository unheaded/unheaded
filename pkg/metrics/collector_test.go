//go:build !integration

// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockCollector is a test implementation of the Collector interface.
type mockCollector struct {
	name     string
	samples  []Sample
	err      error
	describe []string
}

func (m *mockCollector) Collect(ctx context.Context) ([]Sample, error) {
	return m.samples, m.err
}

func (m *mockCollector) Name() string {
	return m.name
}

func (m *mockCollector) Describe() []string {
	return m.describe
}

func TestCollectorRegistry_Register(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*CollectorRegistry)
		want    error
		wantErr bool
	}{
		{
			name: "register single collector",
			setup: func(r *CollectorRegistry) {
				c := &mockCollector{name: "test"}
				err := r.Register(c)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
			wantErr: false,
		},
		{
			name: "register duplicate collector",
			setup: func(r *CollectorRegistry) {
				c1 := &mockCollector{name: "test"}
				c2 := &mockCollector{name: "test"}
				if err := r.Register(c1); err != nil {
					t.Fatalf("unexpected error on first register: %v", err)
				}
				if err := r.Register(c2); err == nil {
					t.Fatal("expected error for duplicate collector, got nil")
				}
			},
			wantErr: true,
		},
		{
			name: "register nil collector",
			setup: func(r *CollectorRegistry) {
				err := r.Register(nil)
				if err == nil {
					t.Fatal("expected error for nil collector, got nil")
				}
			},
			wantErr: true,
		},
		{
			name: "register collector with empty name",
			setup: func(r *CollectorRegistry) {
				c := &mockCollector{name: ""}
				err := r.Register(c)
				if err == nil {
					t.Fatal("expected error for empty name, got nil")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewCollectorRegistry()
			tt.setup(r)
		})
	}
}

func TestCollectorRegistry_CollectAll(t *testing.T) {
	tests := []struct {
		name        string
		collectors  []SystemCollector
		wantSamples int
		wantErr     bool
	}{
		{
			name:        "empty registry",
			collectors:  []SystemCollector{},
			wantSamples: 0,
			wantErr:     false,
		},
		{
			name: "single collector with samples",
			collectors: []SystemCollector{
				&mockCollector{
					name: "test",
					samples: []Sample{
						{
							Name:  "test_metric",
							Help:  "test help",
							Type:  TypeGauge,
							Value: 42.0,
						},
					},
				},
			},
			wantSamples: 1,
			wantErr:     false,
		},
		{
			name: "multiple collectors with samples",
			collectors: []SystemCollector{
				&mockCollector{
					name: "collector1",
					samples: []Sample{
						{
							Name:  "metric1",
							Help:  "help1",
							Type:  TypeGauge,
							Value: 1.0,
						},
					},
				},
				&mockCollector{
					name: "collector2",
					samples: []Sample{
						{
							Name:  "metric2",
							Help:  "help2",
							Type:  TypeCounter,
							Value: 2.0,
						},
					},
				},
			},
			wantSamples: 2,
			wantErr:     false,
		},
		{
			name: "collector with empty samples",
			collectors: []SystemCollector{
				&mockCollector{
					name:    "empty",
					samples: []Sample{},
				},
			},
			wantSamples: 0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewCollectorRegistry()
			for _, c := range tt.collectors {
				if err := r.Register(c); err != nil {
					t.Fatalf("failed to register collector: %v", err)
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			samples, err := r.CollectAll(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("CollectAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(samples) != tt.wantSamples {
				t.Errorf("CollectAll() got %d samples, want %d", len(samples), tt.wantSamples)
			}

			// Verify host label was added
			for _, s := range samples {
				if s.Labels == nil {
					t.Error("expected Labels to be non-nil")
				}
				if _, hasHost := s.Labels["host"]; !hasHost {
					t.Error("expected host label to be added")
				}
			}
		})
	}
}

func TestCollectorRegistry_CollectAllWithFailingCollector(t *testing.T) {
	r := NewCollectorRegistry()

	// Register a working collector
	working := &mockCollector{
		name: "working",
		samples: []Sample{
			{
				Name:  "working_metric",
				Help:  "working",
				Type:  TypeGauge,
				Value: 1.0,
			},
		},
	}

	// Register a failing collector
	failing := &mockCollector{
		name: "failing",
		err:  errors.New("simulated failure"),
	}

	if err := r.Register(working); err != nil {
		t.Fatalf("failed to register working collector: %v", err)
	}
	if err := r.Register(failing); err != nil {
		t.Fatalf("failed to register failing collector: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	samples, err := r.CollectAll(ctx)

	// Should not fail entirely, but should log warning and continue
	if err != nil {
		t.Errorf("CollectAll() should not fail on partial failure, got: %v", err)
	}

	// Should have gotten the sample from the working collector
	if len(samples) != 1 {
		t.Errorf("CollectAll() got %d samples, want 1", len(samples))
	}

	if len(samples) > 0 && samples[0].Name != "working_metric" {
		t.Errorf("CollectAll() got wrong metric name: %s", samples[0].Name)
	}
}

func TestCollectorRegistry_HostLabelAdded(t *testing.T) {
	r := NewCollectorRegistry()

	c := &mockCollector{
		name: "test",
		samples: []Sample{
			{
				Name:   "metric1",
				Help:   "help",
				Type:   TypeGauge,
				Value:  1.0,
				Labels: map[string]string{"foo": "bar"},
			},
		},
	}

	if err := r.Register(c); err != nil {
		t.Fatalf("failed to register collector: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	samples, err := r.CollectAll(ctx)
	if err != nil {
		t.Fatalf("CollectAll() error: %v", err)
	}

	if len(samples) != 1 {
		t.Fatalf("got %d samples, want 1", len(samples))
	}

	// Verify both original and host labels are present
	if v, ok := samples[0].Labels["foo"]; !ok || v != "bar" {
		t.Errorf("original label lost or incorrect: %v", samples[0].Labels)
	}

	if _, ok := samples[0].Labels["host"]; !ok {
		t.Error("host label not added")
	}
}
