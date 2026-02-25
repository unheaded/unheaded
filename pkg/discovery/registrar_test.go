// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"unheaded/pkg/transport"
)

// mockTransportConn implements transport.Connection for testing.
type mockTransportConn struct {
	mu        sync.Mutex
	published map[string][][]byte
	healthy   bool
}

func newMockTransportConn() *mockTransportConn {
	return &mockTransportConn{
		published: make(map[string][][]byte),
		healthy:   true,
	}
}

func (m *mockTransportConn) Type() transport.Type  { return transport.HTTP }
func (m *mockTransportConn) Close() error          { return nil }
func (m *mockTransportConn) Healthy() bool         { return m.healthy }
func (m *mockTransportConn) Publish(_ context.Context, topic string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published[topic] = append(m.published[topic], data)
	return nil
}
func (m *mockTransportConn) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	return make(chan []byte), nil
}

func (m *mockTransportConn) getPublished(topic string) [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.published[topic]
}

func TestRegistrar_Register(t *testing.T) {
	conn := newMockTransportConn()
	reg := NewRegistrar(conn, "timeguru", 19000, "http")

	if err := reg.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	msgs := conn.getPublished(RegisterTopic)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages on %s, want 1", len(msgs), RegisterTopic)
	}

	var msg RegistrationMessage
	if err := json.Unmarshal(msgs[0], &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Name != "timeguru" {
		t.Errorf("name = %q, want %q", msg.Name, "timeguru")
	}
	if msg.Port != 19000 {
		t.Errorf("port = %d, want 19000", msg.Port)
	}
	if msg.Proto != "http" {
		t.Errorf("proto = %q, want %q", msg.Proto, "http")
	}
	if msg.PID == 0 {
		t.Error("expected non-zero PID")
	}
	if msg.StartedAt == 0 {
		t.Error("expected non-zero started_at")
	}
}

func TestRegistrar_Deregister(t *testing.T) {
	conn := newMockTransportConn()
	reg := NewRegistrar(conn, "timeguru", 19000, "http")

	if err := reg.Deregister(context.Background()); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	msgs := conn.getPublished(DeregisterTopic)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages on %s, want 1", len(msgs), DeregisterTopic)
	}

	var msg RegistrationMessage
	json.Unmarshal(msgs[0], &msg)
	if msg.Health != "down" {
		t.Errorf("health = %q, want %q", msg.Health, "down")
	}
}

func TestRegistrar_NilConnection(t *testing.T) {
	reg := NewRegistrar(nil, "test", 9000, "http")

	if err := reg.Register(context.Background()); err == nil {
		t.Error("expected error for nil connection")
	}
	if err := reg.Deregister(context.Background()); err == nil {
		t.Error("expected error for nil connection")
	}
}

func TestRegistrar_ServiceName(t *testing.T) {
	reg := NewRegistrar(nil, "myservice", 9000, "http")
	if reg.ServiceName() != "myservice" {
		t.Errorf("ServiceName = %q, want %q", reg.ServiceName(), "myservice")
	}
}

func TestRegistrar_MessageFields(t *testing.T) {
	conn := newMockTransportConn()
	reg := NewRegistrar(conn, "captain", 19002, "grpc")
	reg.Register(context.Background())

	msgs := conn.getPublished(RegisterTopic)
	var msg RegistrationMessage
	json.Unmarshal(msgs[0], &msg)

	if msg.Health != "healthy" {
		t.Errorf("health = %q, want %q", msg.Health, "healthy")
	}
	if msg.Proto != "grpc" {
		t.Errorf("proto = %q, want %q", msg.Proto, "grpc")
	}
}
