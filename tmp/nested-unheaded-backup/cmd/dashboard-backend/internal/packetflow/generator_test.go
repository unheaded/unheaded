package packetflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestGenerator_NewGenerator tests generator creation
func TestGenerator_NewGenerator(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Interval:       100 * time.Millisecond,
				MaxFlows:       10,
				TraceIDPattern: "trace-%d",
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "zero interval",
			config: &Config{
				Interval: 0,
				MaxFlows: 10,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := NewGenerator(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGenerator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && gen == nil {
				t.Error("NewGenerator() returned nil generator")
			}
		})
	}
}

// TestGenerator_Generate tests packet flow generation
func TestGenerator_Generate(t *testing.T) {
	config := &Config{
		Interval:       50 * time.Millisecond,
		MaxFlows:       5,
		TraceIDPattern: "trace-%d",
	}

	gen, err := NewGenerator(config)
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	flowCh, err := gen.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Receive at least one flow
	select {
	case flow := <-flowCh:
		if flow == nil {
			t.Error("received nil flow")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("timeout waiting for flow")
	}
}

// TestGenerator_FlowStructure tests packet flow data structure
func TestGenerator_FlowStructure(t *testing.T) {
	config := &Config{
		Interval:       50 * time.Millisecond,
		MaxFlows:       5,
		TraceIDPattern: "trace-%d",
	}

	gen, err := NewGenerator(config)
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	flowCh, err := gen.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Get flow
	select {
	case flow := <-flowCh:
		// Validate structure
		if flow.TraceID == "" {
			t.Error("flow missing TraceID")
		}
		if flow.Timestamp.IsZero() {
			t.Error("flow missing Timestamp")
		}
		if len(flow.Hops) == 0 {
			t.Error("flow has no hops")
		}

		// Validate hops
		for i, hop := range flow.Hops {
			if hop.Component == "" {
				t.Errorf("hop %d missing Component", i)
			}
			if hop.Timestamp.IsZero() {
				t.Errorf("hop %d missing Timestamp", i)
			}
			if hop.Latency < 0 {
				t.Errorf("hop %d has negative latency", i)
			}
		}

	case <-time.After(200 * time.Millisecond):
		t.Error("timeout waiting for flow")
	}
}

// TestGenerator_JSONSerialization tests JSON serialization
func TestGenerator_JSONSerialization(t *testing.T) {
	config := &Config{
		Interval:       50 * time.Millisecond,
		MaxFlows:       5,
		TraceIDPattern: "trace-%d",
	}

	gen, err := NewGenerator(config)
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	flowCh, err := gen.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Get flow
	select {
	case flow := <-flowCh:
		// Serialize to JSON
		data, err := json.Marshal(flow)
		if err != nil {
			t.Fatalf("json.Marshal() failed: %v", err)
		}

		// Deserialize
		var decoded PacketFlow
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal() failed: %v", err)
		}

		// Verify
		if decoded.TraceID != flow.TraceID {
			t.Errorf("TraceID mismatch: got %s, want %s", decoded.TraceID, flow.TraceID)
		}
		if len(decoded.Hops) != len(flow.Hops) {
			t.Errorf("Hops count mismatch: got %d, want %d", len(decoded.Hops), len(flow.Hops))
		}

	case <-time.After(200 * time.Millisecond):
		t.Error("timeout waiting for flow")
	}
}

// TestGenerator_ChannelClose tests clean shutdown
func TestGenerator_ChannelClose(t *testing.T) {
	config := &Config{
		Interval:       50 * time.Millisecond,
		MaxFlows:       5,
		TraceIDPattern: "trace-%d",
	}

	gen, err := NewGenerator(config)
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	flowCh, err := gen.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Receive one flow
	select {
	case <-flowCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for flow")
	}

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-flowCh:
		if ok {
			t.Error("channel still open after context cancel")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("channel not closed after context cancel")
	}
}

// TestGenerator_RealisticLatencies tests latency values are realistic
func TestGenerator_RealisticLatencies(t *testing.T) {
	config := &Config{
		Interval:       50 * time.Millisecond,
		MaxFlows:       5,
		TraceIDPattern: "trace-%d",
	}

	gen, err := NewGenerator(config)
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	flowCh, err := gen.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Check multiple flows
	for i := 0; i < 5; i++ {
		select {
		case flow := <-flowCh:
			for j, hop := range flow.Hops {
				// Latency should be reasonable (< 100ms per hop)
				if hop.Latency > 100*time.Millisecond {
					t.Errorf("flow %d hop %d: unrealistic latency %v", i, j, hop.Latency)
				}
				// Latency should be positive
				if hop.Latency <= 0 {
					t.Errorf("flow %d hop %d: non-positive latency %v", i, j, hop.Latency)
				}
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatal("timeout waiting for flow")
		}
	}
}

// TestGenerator_ComponentNames tests component naming
func TestGenerator_ComponentNames(t *testing.T) {
	config := &Config{
		Interval:       50 * time.Millisecond,
		MaxFlows:       5,
		TraceIDPattern: "trace-%d",
	}

	gen, err := NewGenerator(config)
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	flowCh, err := gen.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	select {
	case flow := <-flowCh:
		expectedComponents := []string{
			"xdp_packet_marker",
			"gateway",
			"busboy",
			"service",
			"trace-collector",
		}

		if len(flow.Hops) != len(expectedComponents) {
			t.Errorf("got %d hops, want %d", len(flow.Hops), len(expectedComponents))
		}

		for i, expected := range expectedComponents {
			if i >= len(flow.Hops) {
				break
			}
			if flow.Hops[i].Component != expected {
				t.Errorf("hop %d: got component %s, want %s", i, flow.Hops[i].Component, expected)
			}
		}

	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for flow")
	}
}

// TestGenerator_ConcurrentConsumers tests multiple consumers
func TestGenerator_ConcurrentConsumers(t *testing.T) {
	config := &Config{
		Interval:       50 * time.Millisecond,
		MaxFlows:       5,
		TraceIDPattern: "trace-%d",
	}

	gen, err := NewGenerator(config)
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	flowCh, err := gen.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Multiple consumers
	consumers := 3
	done := make(chan bool)

	for c := 0; c < consumers; c++ {
		go func(id int) {
			count := 0
			for flow := range flowCh {
				if flow != nil {
					count++
				}
				if count >= 3 {
					break
				}
			}
			done <- true
		}(c)
	}

	// Wait for all consumers (or timeout)
	timeout := time.After(3 * time.Second)
	for c := 0; c < consumers; c++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("consumer timeout")
		}
	}
}
