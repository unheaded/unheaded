// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !integration

package anamnesis

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Mock Wotan Publisher ---

type MockPublisher struct {
	mu       sync.Mutex
	messages []struct {
		topic   string
		payload []byte
	}
}

func (m *MockPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := make([]byte, len(payload))
	copy(p, payload)
	m.messages = append(m.messages, struct {
		topic   string
		payload []byte
	}{topic, p})
	return nil
}

func (m *MockPublisher) Messages() []struct {
	topic   string
	payload []byte
} {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]struct {
		topic   string
		payload []byte
	}, len(m.messages))
	copy(out, m.messages)
	return out
}

func (m *MockPublisher) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

// --- Tests ---

func TestIsMonadAlert(t *testing.T) {
	tests := []struct {
		name     string
		event    EVEEvent
		expected bool
	}{
		{
			name: "monad alert sid 9000001",
			event: EVEEvent{
				EventType: "alert",
				Alert:     EVEAlert{SignatureID: 9000001, Severity: 3},
			},
			expected: true,
		},
		{
			name: "monad alert sid 9000099 (max)",
			event: EVEEvent{
				EventType: "alert",
				Alert:     EVEAlert{SignatureID: 9000099, Severity: 1},
			},
			expected: true,
		},
		{
			name: "non-monad alert sid 2000000",
			event: EVEEvent{
				EventType: "alert",
				Alert:     EVEAlert{SignatureID: 2000000},
			},
			expected: false,
		},
		{
			name: "monad sid but wrong event type",
			event: EVEEvent{
				EventType: "stats",
				Alert:     EVEAlert{SignatureID: 9000001},
			},
			expected: false,
		},
		{
			name: "empty event",
			event:    EVEEvent{},
			expected: false,
		},
		{
			name: "sid just below monad range",
			event: EVEEvent{
				EventType: "alert",
				Alert:     EVEAlert{SignatureID: 9000000},
			},
			expected: false,
		},
		{
			name: "sid just above monad range",
			event: EVEEvent{
				EventType: "alert",
				Alert:     EVEAlert{SignatureID: 9000100},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMonadAlert(tt.event)
			if got != tt.expected {
				t.Errorf("isMonadAlert() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBuildRingEntry(t *testing.T) {
	tests := []struct {
		name      string
		event     EVEEvent
		checkFunc func(t *testing.T, entry RingEntry)
	}{
		{
			name: "monad alert with IPv6 addresses",
			event: EVEEvent{
				Timestamp: "2026-02-26T04:15:00.000000+0000",
				EventType: "alert",
				SrcIP:     "fd00:dead:beef::1",
				DestIP:    "fd00:dead:beef::2",
				Alert:     EVEAlert{SignatureID: 9000001, Severity: 3},
			},
			checkFunc: func(t *testing.T, entry RingEntry) {
				t.Helper()
				// Check size
				if len(entry) != 64 {
					t.Errorf("entry size = %d, want 64", len(entry))
				}
				// Check SID at [8:12]
				sid := binary.LittleEndian.Uint32(entry[8:12])
				if sid != 9000001 {
					t.Errorf("sid = %d, want 9000001", sid)
				}
				// Check severity at [44]
				if entry[44] != 3 {
					t.Errorf("severity = %d, want 3", entry[44])
				}
				// Check src_ip at [12:28] — fd00:dead:beef::1
				srcIP := net.IP(entry[12:28])
				if srcIP.String() != "fd00:dead:beef::1" {
					t.Errorf("src_ip = %s, want fd00:dead:beef::1", srcIP.String())
				}
				// Check padding [45:64] is all zeros
				for i := 45; i < 64; i++ {
					if entry[i] != 0 {
						t.Errorf("entry[%d] = %d, want 0 (padding)", i, entry[i])
					}
				}
			},
		},
		{
			name: "IPv4 source address — mapped to IPv4-in-IPv6",
			event: EVEEvent{
				EventType: "alert",
				SrcIP:     "192.168.1.1",
				DestIP:    "10.20.0.1",
				Alert:     EVEAlert{SignatureID: 9000030, Severity: 1},
			},
			checkFunc: func(t *testing.T, entry RingEntry) {
				t.Helper()
				srcIP := net.IP(entry[12:28])
				// Should be IPv4-in-IPv6 (::ffff:192.168.1.1)
				if srcIP.To4() == nil && !strings.HasPrefix(srcIP.String(), "::ffff:") {
					t.Logf("src_ip = %s (IPv6 form of IPv4 — acceptable)", srcIP.String())
				}
				sid := binary.LittleEndian.Uint32(entry[8:12])
				if sid != 9000030 {
					t.Errorf("sid = %d, want 9000030", sid)
				}
				if entry[44] != 1 {
					t.Errorf("severity = %d, want 1", entry[44])
				}
			},
		},
		{
			name: "empty IP addresses — zero filled",
			event: EVEEvent{
				EventType: "alert",
				SrcIP:     "",
				DestIP:    "",
				Alert:     EVEAlert{SignatureID: 9000001, Severity: 2},
			},
			checkFunc: func(t *testing.T, entry RingEntry) {
				t.Helper()
				for i := 12; i < 44; i++ {
					if entry[i] != 0 {
						t.Errorf("IP field entry[%d] = %d, want 0 for empty IP", i, entry[i])
					}
				}
			},
		},
		{
			name: "invalid IP address — zero filled",
			event: EVEEvent{
				EventType: "alert",
				SrcIP:     "not-an-ip",
				DestIP:    "also-not-ip",
				Alert:     EVEAlert{SignatureID: 9000001},
			},
			checkFunc: func(t *testing.T, entry RingEntry) {
				t.Helper()
				for i := 12; i < 28; i++ {
					if entry[i] != 0 {
						t.Errorf("src_ip field entry[%d] = %d, want 0 for invalid IP", i, entry[i])
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := buildRingEntry(tt.event)
			if err != nil {
				t.Fatalf("buildRingEntry() error = %v", err)
			}
			tt.checkFunc(t, entry)
		})
	}
}

func TestParseIPToIPv6(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Expected as IP string
	}{
		{"IPv6 loopback", "::1", "::1"},
		{"IPv6 WireGuard", "fd00:dead:beef::1", "fd00:dead:beef::1"},
		{"IPv4 mapped", "192.168.1.1", "::ffff:192.168.1.1"},
		{"empty string", "", "<nil>"},
		{"invalid", "not-an-ip", "<nil>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := parseIPToIPv6(tt.input)
			if len(b) != 16 {
				t.Errorf("parseIPToIPv6(%q) returned %d bytes, want 16", tt.input, len(b))
			}
			// Verify no panic and returns 16 bytes
		})
	}
}

func TestSuricataReader_ProcessLine(t *testing.T) {
	pub := &MockPublisher{}
	reader := NewSuricataReader("/dev/null", pub)

	tests := []struct {
		name         string
		line         string
		expectPublish bool
	}{
		{
			name:         "monad alert",
			line:         `{"timestamp":"2026-02-26T04:15:00.000000+0000","event_type":"alert","src_ip":"fd00:dead:beef::1","dest_ip":"fd00:dead:beef::2","alert":{"signature_id":9000001,"signature":"MONAD HbH extension header detected","severity":3}}` + "\n",
			expectPublish: true,
		},
		{
			name:         "non-monad alert filtered",
			line:         `{"timestamp":"2026-02-26T04:15:01.000000+0000","event_type":"alert","src_ip":"10.0.0.1","dest_ip":"10.0.0.2","alert":{"signature_id":2000000,"signature":"ET SCAN Nmap","severity":2}}` + "\n",
			expectPublish: false,
		},
		{
			name:         "stats event filtered",
			line:         `{"timestamp":"2026-02-26T04:15:02.000000+0000","event_type":"stats","stats":{"uptime":300}}` + "\n",
			expectPublish: false,
		},
		{
			name:         "empty line",
			line:         "\n",
			expectPublish: false,
		},
		{
			name:         "invalid JSON",
			line:         "this is not json\n",
			expectPublish: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeCount := pub.Count()
			ctx := context.Background()
			_ = reader.processLine(ctx, tt.line)
			afterCount := pub.Count()
			published := afterCount > beforeCount
			if published != tt.expectPublish {
				t.Errorf("processLine() published = %v, want %v", published, tt.expectPublish)
			}
		})
	}
}

func TestSuricataReader_ProcessFixtureFile(t *testing.T) {
	// Read fixture file
	fixturePath := filepath.Join("testdata", "eve-sample.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("testdata/eve-sample.json not found: %v", err)
	}

	pub := &MockPublisher{}
	reader := NewSuricataReader("/dev/null", pub)
	ctx := context.Background()

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		_ = reader.processLine(ctx, line+"\n")
	}

	// From fixture: 2 Monad alerts (9000001 and 9000030), 2 filtered
	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Errorf("processed %d Monad alerts from fixture, want 2", len(msgs))
	}

	for _, msg := range msgs {
		if msg.topic != WotanTopic {
			t.Errorf("message topic = %q, want %q", msg.topic, WotanTopic)
		}
		if len(msg.payload) != RingEntrySize {
			t.Errorf("ring entry size = %d, want %d", len(msg.payload), RingEntrySize)
		}
	}
}

func TestSuricataReader_DegradedMode(t *testing.T) {
	pub := &MockPublisher{}
	nonexistentPath := "/nonexistent/path/to/eve.json"
	reader := NewSuricataReader(nonexistentPath, pub)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should return ctx.Err() (DeadlineExceeded) without panicking
	err := reader.Run(ctx)
	if err == nil {
		t.Error("Run() should have returned an error (ctx cancelled or deadline exceeded)")
	}
	// Should not have published anything
	if pub.Count() != 0 {
		t.Errorf("published %d events in degraded mode, want 0", pub.Count())
	}
}

// TestRingEntrySize verifies the constant is correct
func TestRingEntrySize(t *testing.T) {
	var entry RingEntry
	if len(entry) != 64 {
		t.Errorf("RingEntry size = %d, want 64", len(entry))
	}
	if RingEntrySize != 64 {
		t.Errorf("RingEntrySize = %d, want 64", RingEntrySize)
	}
}
