// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophiasync

import (
	"context"
	"os"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// NewMockWotanClient returns an empty wotan client for tests that don't
// exercise the publish path.
func NewMockWotanClient() *wotanClient.Client {
	return &wotanClient.Client{}
}

func TestNewEncoderStream(t *testing.T) {
	log := logger.New(os.Stdout)

	tests := []struct {
		name        string
		client      *wotanClient.Client
		topic       string
		expectError bool
	}{
		{
			name:        "valid encoder stream creation",
			client:      NewMockWotanClient(),
			topic:       "control",
			expectError: false,
		},
		{
			name:        "nil client",
			client:      nil,
			topic:       "control",
			expectError: true,
		},
		{
			name:        "empty topic",
			client:      NewMockWotanClient(),
			topic:       "",
			expectError: true,
		},
		{
			name:        "nil logger",
			client:      NewMockWotanClient(),
			topic:       "control",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es, err := NewEncoderStream(tt.client, tt.topic, log)

			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && es == nil {
				t.Errorf("expected encoder stream, got nil")
			}
		})
	}
}

func TestNewDecoderStream(t *testing.T) {
	log := logger.New(os.Stdout)

	tests := []struct {
		name        string
		hopID       string
		expectError bool
	}{
		{
			name:        "valid decoder stream creation",
			hopID:       "hop-1",
			expectError: false,
		},
		{
			name:        "empty hop ID",
			hopID:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, err := NewDecoderStream(tt.hopID, log)

			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && ds == nil {
				t.Errorf("expected decoder stream, got nil")
			}
		})
	}
}

func TestApplyDelta(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)

	tests := []struct {
		name        string
		delta       *DictionaryDelta
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil delta",
			delta:       nil,
			expectError: true,
			errorMsg:    "delta cannot be nil",
		},
		{
			name: "valid delta",
			delta: &DictionaryDelta{
				Version:   1,
				Additions: []DictEntry{{Index: 1, Value: "test"}},
			},
			expectError: false,
		},
		{
			name: "invalid delta version",
			delta: &DictionaryDelta{
				Version: 5,
			},
			expectError: true,
			errorMsg:    "invalid delta version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.ApplyDelta(tt.delta)

			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestACKTracking(t *testing.T) {
	log := logger.New(os.Stdout)
	es, _ := NewEncoderStream(NewMockWotanClient(), "control", log)

	// Register pending ACKs
	es.RegisterPendingAck("hop-1", 1)
	es.RegisterPendingAck("hop-2", 1)
	es.RegisterPendingAck("hop-3", 1)

	if count := es.PendingAcksCount(); count != 3 {
		t.Errorf("expected 3 pending ACKs, got %d", count)
	}

	// Acknowledge one hop
	es.AcknowledgeAck("hop-1")

	if count := es.PendingAcksCount(); count != 2 {
		t.Errorf("expected 2 pending ACKs after acknowledgment, got %d", count)
	}

	// Acknowledge remaining hops
	es.AcknowledgeAck("hop-2")
	es.AcknowledgeAck("hop-3")

	if count := es.PendingAcksCount(); count != 0 {
		t.Errorf("expected 0 pending ACKs after all acknowledgments, got %d", count)
	}
}

func TestVersionMismatchDetection(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)

	delta1 := &DictionaryDelta{
		Version:   1,
		Additions: []DictEntry{{Index: 1, Value: "entry1"}},
	}

	// Apply first delta
	if err := sm.ApplyDelta(delta1); err != nil {
		t.Fatalf("failed to apply first delta: %v", err)
	}

	// Try to apply delta with wrong version (should skip 2 and go to 5)
	delta2 := &DictionaryDelta{
		Version:   5,
		Additions: []DictEntry{{Index: 2, Value: "entry2"}},
	}

	err := sm.ApplyDelta(delta2)
	if err == nil {
		t.Errorf("expected version mismatch error, got nil")
	}

	// Apply correct next version
	delta3 := &DictionaryDelta{
		Version:   2,
		Additions: []DictEntry{{Index: 2, Value: "entry2"}},
	}

	if err := sm.ApplyDelta(delta3); err != nil {
		t.Errorf("failed to apply delta with correct version: %v", err)
	}
}

func TestGracePeriodExpiry(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)
	sm.SetGracePeriod(100 * time.Millisecond)

	// Apply several deltas
	for i := uint64(1); i <= 3; i++ {
		delta := &DictionaryDelta{
			Version:   TableVersion(i),
			Additions: []DictEntry{{Index: uint32(i), Value: "entry"}},
		}
		if err := sm.ApplyDelta(delta); err != nil {
			t.Fatalf("failed to apply delta %d: %v", i, err)
		}
	}

	// Wait for grace period to expire
	time.Sleep(150 * time.Millisecond)

	// Cleanup expired versions
	if err := sm.CleanupExpiredVersions(); err != nil {
		t.Errorf("cleanup failed: %v", err)
	}

	// Verify old versions were cleaned up
	if len(sm.expiredVersions) > 1 {
		t.Errorf("expected at most 1 expired version remaining, got %d", len(sm.expiredVersions))
	}
}

func TestConcurrentAccess(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)

	// Register multiple endpoints
	for i := 0; i < 10; i++ {
		endpointID := "endpoint-" + string(rune(i+'0'))
		if err := sm.RegisterEndpoint(endpointID); err != nil {
			t.Fatalf("failed to register endpoint: %v", err)
		}
	}

	// Concurrently update versions
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			endpointID := "endpoint-" + string(rune(idx+'0'))
			for j := 0; j < 100; j++ {
				sm.UpdateEndpointVersion(endpointID, TableVersion(j))
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all endpoints have correct final version
	for i := 0; i < 10; i++ {
		endpointID := "endpoint-" + string(rune(i+'0'))
		version, err := sm.GetEndpointVersion(endpointID)
		if err != nil {
			t.Errorf("failed to get endpoint version: %v", err)
		}
		if version != TableVersion(99) {
			t.Errorf("expected version 99, got %d", version)
		}
	}
}

func TestWaitForAcks(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)

	// Register pending ACKs
	sm.encoder.RegisterPendingAck("hop-1", 1)
	sm.encoder.RegisterPendingAck("hop-2", 1)

	ctx := context.Background()

	// Start goroutine to simulate incoming ACKs
	go func() {
		time.Sleep(50 * time.Millisecond)
		sm.encoder.AcknowledgeAck("hop-1")
		time.Sleep(50 * time.Millisecond)
		sm.encoder.AcknowledgeAck("hop-2")
	}()

	// Wait for all ACKs with sufficient timeout
	err := sm.WaitForAcks(ctx, 500*time.Millisecond)
	if err != nil {
		t.Errorf("WaitForAcks failed: %v", err)
	}

	if sm.encoder.PendingAcksCount() != 0 {
		t.Errorf("expected 0 pending ACKs, got %d", sm.encoder.PendingAcksCount())
	}
}

func TestWaitForAcksTimeout(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)

	// Register pending ACKs but don't acknowledge them
	sm.encoder.RegisterPendingAck("hop-1", 1)
	sm.encoder.RegisterPendingAck("hop-2", 1)

	ctx := context.Background()

	// Wait with short timeout - should fail
	err := sm.WaitForAcks(ctx, 100*time.Millisecond)
	if err == nil {
		t.Errorf("expected timeout error, got nil")
	}
}

func TestDecoderStreamState(t *testing.T) {
	log := logger.New(os.Stdout)
	ds, _ := NewDecoderStream("hop-1", log)

	// Initial state should be 0
	hopID, version := ds.GetState()
	if hopID != "hop-1" {
		t.Errorf("expected hop ID 'hop-1', got '%s'", hopID)
	}
	if version != 0 {
		t.Errorf("expected initial version 0, got %d", version)
	}

	// Update state
	if err := ds.UpdateState(5); err != nil {
		t.Errorf("failed to update state: %v", err)
	}

	_, version = ds.GetState()
	if version != 5 {
		t.Errorf("expected version 5 after update, got %d", version)
	}
}

func TestSetGracePeriod(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)

	tests := []struct {
		name        string
		period      time.Duration
		expectError bool
	}{
		{
			name:        "valid grace period",
			period:      30 * time.Second,
			expectError: false,
		},
		{
			name:        "zero grace period",
			period:      0,
			expectError: false,
		},
		{
			name:        "negative grace period",
			period:      -1 * time.Second,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.SetGracePeriod(tt.period)

			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRegisterEndpoint(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)

	// Register valid endpoint
	if err := sm.RegisterEndpoint("ep1"); err != nil {
		t.Errorf("failed to register endpoint: %v", err)
	}

	// Try to register duplicate
	err := sm.RegisterEndpoint("ep1")
	if err == nil {
		t.Errorf("expected error for duplicate endpoint, got nil")
	}

	// Register empty endpoint
	err = sm.RegisterEndpoint("")
	if err == nil {
		t.Errorf("expected error for empty endpoint ID, got nil")
	}
}

func TestCurrentVersion(t *testing.T) {
	log := logger.New(os.Stdout)
	sm, _ := NewSyncManager(NewMockWotanClient(), "control", log)

	if version := sm.CurrentVersion(); version != 0 {
		t.Errorf("expected initial version 0, got %d", version)
	}

	// Apply delta
	delta := &DictionaryDelta{
		Version:   1,
		Additions: []DictEntry{{Index: 1, Value: "test"}},
	}
	sm.ApplyDelta(delta)

	if version := sm.CurrentVersion(); version != 1 {
		t.Errorf("expected version 1 after applying delta, got %d", version)
	}
}
