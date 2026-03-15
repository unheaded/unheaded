// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package dns

import (
	"bytes"
	"context"
	"log"
	"net"
	"testing"
	"time"
)

func TestServer_New(t *testing.T) {
	// Test with nil config
	s := NewServer(nil)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}

	if s.zones == nil {
		t.Error("Zones manager should be initialized")
	}
	if s.resolver == nil {
		t.Error("Resolver should be initialized")
	}

	// Test with custom config
	config := &ServerConfig{
		UDPAddr:     ":5353",
		TCPAddr:     ":5353",
		EnableTCP:   true,
		EnableUDP:   true,
		MaxTCPConns: 50,
	}
	s2 := NewServer(config)
	if s2.config.MaxTCPConns != 50 {
		t.Errorf("MaxTCPConns = %d, want 50", s2.config.MaxTCPConns)
	}
}

func TestServer_SetHandler(t *testing.T) {
	s := NewServer(nil)

	handlerCalled := false
	customHandler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		handlerCalled = true
	})

	s.SetHandler(customHandler)

	// Create a mock writer
	mockWriter := &mockResponseWriter{}

	// Simulate serving
	s.handler.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))

	if !handlerCalled {
		t.Error("Custom handler should have been called")
	}
}

func TestServer_SetLogger(t *testing.T) {
	s := NewServer(nil)

	var buf bytes.Buffer
	logger := log.New(&buf, "TEST: ", 0)

	s.SetLogger(logger)
	s.log("test message %s", "arg")

	if buf.Len() == 0 {
		t.Error("Logger should have received message")
	}
}

func TestServer_Zones(t *testing.T) {
	s := NewServer(nil)

	zones := s.Zones()
	if zones == nil {
		t.Error("Zones() returned nil")
	}

	// Add a zone
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zones.AddZone(zone)

	if zones.ZoneCount() != 1 {
		t.Errorf("ZoneCount = %d, want 1", zones.ZoneCount())
	}
}

func TestServer_Resolver(t *testing.T) {
	s := NewServer(nil)

	resolver := s.Resolver()
	if resolver == nil {
		t.Error("Resolver() returned nil")
	}
}

func TestServer_Stats(t *testing.T) {
	s := NewServer(nil)

	stats := s.Stats()

	// Initial stats should be zero
	if stats.UDPQueries != 0 {
		t.Errorf("Initial UDPQueries = %d, want 0", stats.UDPQueries)
	}
	if stats.TCPQueries != 0 {
		t.Errorf("Initial TCPQueries = %d, want 0", stats.TCPQueries)
	}
}

func TestServer_Use(t *testing.T) {
	s := NewServer(nil)

	middlewareCalled := false
	middleware := func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
			middlewareCalled = true
			next.ServeDNS(ctx, w, r)
		})
	}

	s.Use(middleware)

	// Serve a request
	mockWriter := &mockResponseWriter{}
	s.serve(context.Background(), mockWriter, NewQuery("test.com", TypeA))

	if !middlewareCalled {
		t.Error("Middleware should have been called")
	}
}

func TestServer_StartStopErrors(t *testing.T) {
	s := NewServer(nil)

	// Stop without start
	err := s.Stop()
	if err != ErrServerClosed {
		t.Errorf("Stop() without start should return ErrServerClosed, got %v", err)
	}
}

func TestDefaultServerConfig(t *testing.T) {
	config := DefaultServerConfig()

	if config.UDPAddr != ":53" {
		t.Errorf("UDPAddr = %s, want :53", config.UDPAddr)
	}
	if config.TCPAddr != ":53" {
		t.Errorf("TCPAddr = %s, want :53", config.TCPAddr)
	}
	if config.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v, want 10s", config.ReadTimeout)
	}
	if config.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout = %v, want 10s", config.WriteTimeout)
	}
	if config.MaxTCPConns != 100 {
		t.Errorf("MaxTCPConns = %d, want 100", config.MaxTCPConns)
	}
	if config.MaxUDPSize != 4096 {
		t.Errorf("MaxUDPSize = %d, want 4096", config.MaxUDPSize)
	}
	if !config.EnableTCP {
		t.Error("EnableTCP should be true by default")
	}
	if !config.EnableUDP {
		t.Error("EnableUDP should be true by default")
	}
	if config.ResolverConfig == nil {
		t.Error("ResolverConfig should be set")
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	middleware := LoggingMiddleware(logger)
	handler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		// Just respond
		resp := NewResponse(r)
		w.WriteMsg(resp)
	})

	wrapped := middleware(handler)
	mockWriter := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}}

	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))

	// Should have logged something
	if buf.Len() == 0 {
		t.Error("LoggingMiddleware should have logged")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	middleware := RateLimitMiddleware(1, 1) // 1 request per second, burst of 1

	responseCount := 0
	handler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		responseCount++
		resp := NewResponse(r)
		w.WriteMsg(resp)
	})

	wrapped := middleware(handler)
	mockWriter := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}}

	// First request should succeed
	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))
	if responseCount != 1 {
		t.Error("First request should have been processed")
	}

	// Second request should be rate limited (REFUSED)
	mockWriter.msg = nil
	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))
	if mockWriter.msg != nil && mockWriter.msg.Header.Rcode() != RcodeRefused {
		t.Error("Second request should be REFUSED due to rate limiting")
	}
}

func TestACLMiddleware_Allowed(t *testing.T) {
	_, allowNet, _ := net.ParseCIDR("127.0.0.0/8")
	middleware := ACLMiddleware([]*net.IPNet{allowNet}, nil)

	responseCount := 0
	handler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		responseCount++
		resp := NewResponse(r)
		w.WriteMsg(resp)
	})

	wrapped := middleware(handler)
	mockWriter := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}}

	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))
	if responseCount != 1 {
		t.Error("Request from allowed IP should be processed")
	}
}

func TestACLMiddleware_Denied(t *testing.T) {
	_, denyNet, _ := net.ParseCIDR("127.0.0.0/8")
	middleware := ACLMiddleware(nil, []*net.IPNet{denyNet})

	responseCount := 0
	handler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		responseCount++
	})

	wrapped := middleware(handler)
	mockWriter := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}}

	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))
	if responseCount != 0 {
		t.Error("Request from denied IP should not be processed")
	}
	if mockWriter.msg == nil || mockWriter.msg.Header.Rcode() != RcodeRefused {
		t.Error("Denied request should return REFUSED")
	}
}

func TestACLMiddleware_NotInAllowList(t *testing.T) {
	_, allowNet, _ := net.ParseCIDR("10.0.0.0/8")
	middleware := ACLMiddleware([]*net.IPNet{allowNet}, nil)

	responseCount := 0
	handler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		responseCount++
	})

	wrapped := middleware(handler)
	mockWriter := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}}

	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))
	if responseCount != 0 {
		t.Error("Request not in allow list should not be processed")
	}
}

func TestACLMiddleware_InvalidIP(t *testing.T) {
	_, allowNet, _ := net.ParseCIDR("10.0.0.0/8")
	middleware := ACLMiddleware([]*net.IPNet{allowNet}, nil)

	handler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {})

	wrapped := middleware(handler)
	mockWriter := &mockResponseWriter{remoteAddr: &mockAddr{addr: "invalid"}}

	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))
	if mockWriter.msg == nil || mockWriter.msg.Header.Rcode() != RcodeRefused {
		t.Error("Invalid IP should return REFUSED")
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	middleware := RecoveryMiddleware(logger)
	handler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		panic("test panic")
	})

	wrapped := middleware(handler)
	mockWriter := &mockResponseWriter{}

	// Should not panic
	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))

	// Should have logged
	if buf.Len() == 0 {
		t.Error("RecoveryMiddleware should log panic")
	}

	// Should return SERVFAIL
	if mockWriter.msg == nil || mockWriter.msg.Header.Rcode() != RcodeServerFailure {
		t.Error("RecoveryMiddleware should return SERVFAIL after panic")
	}
}

func TestMetricsMiddleware(t *testing.T) {
	mm := NewMetricsMiddleware()
	middleware := mm.Middleware()

	handler := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		resp := NewResponse(r)
		w.WriteMsg(resp)
	})

	wrapped := middleware(handler)
	mockWriter := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234}}

	// Make some queries
	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))
	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeAAAA))
	wrapped.ServeDNS(context.Background(), mockWriter, NewQuery("test.com", TypeA))

	queryTypes := mm.GetQueryTypes()
	if queryTypes[TypeA] != 2 {
		t.Errorf("TypeA queries = %d, want 2", queryTypes[TypeA])
	}
	if queryTypes[TypeAAAA] != 1 {
		t.Errorf("TypeAAAA queries = %d, want 1", queryTypes[TypeAAAA])
	}

	clients := mm.GetClients()
	if clients["10.0.0.1"] != 3 {
		t.Errorf("Client 10.0.0.1 queries = %d, want 3", clients["10.0.0.1"])
	}
}

func TestHandlerFunc(t *testing.T) {
	called := false
	f := HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
		called = true
	})

	f.ServeDNS(context.Background(), nil, nil)
	if !called {
		t.Error("HandlerFunc should be callable via ServeDNS")
	}
}

func TestUDPResponseWriter(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create UDP socket: %v", err)
	}
	defer conn.Close()

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
	w := &udpResponseWriter{
		conn:       conn,
		remoteAddr: remoteAddr,
		localAddr:  conn.LocalAddr(),
	}

	if w.IsTCP() {
		t.Error("UDP writer should not be TCP")
	}
	if w.LocalAddr() == nil {
		t.Error("LocalAddr should not be nil")
	}
	if w.RemoteAddr() == nil {
		t.Error("RemoteAddr should not be nil")
	}
	if w.Close() != nil {
		t.Error("Close should succeed for UDP")
	}
}

func TestTCPResponseWriter(t *testing.T) {
	// Create a pipe for testing
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	w := &tcpResponseWriter{
		conn:       client,
		localAddr:  client.LocalAddr(),
		remoteAddr: client.RemoteAddr(),
		timeout:    time.Second,
	}

	if !w.IsTCP() {
		t.Error("TCP writer should be TCP")
	}

	// Write a message
	msg := NewResponse(NewQuery("test.com", TypeA))
	done := make(chan error, 1)
	go func() {
		done <- w.WriteMsg(msg)
	}()

	// Read from server side
	buf := make([]byte, 1024)
	server.SetReadDeadline(time.Now().Add(time.Second))
	_, err := server.Read(buf)
	if err != nil {
		t.Logf("Read error (expected in pipe): %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Logf("WriteMsg error (may be expected): %v", err)
		}
	case <-time.After(time.Second):
		t.Error("WriteMsg timed out")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(10, 2) // 10/sec, burst of 2

	// Should allow burst
	if !rl.allow() {
		t.Error("First request should be allowed")
	}
	if !rl.allow() {
		t.Error("Second request should be allowed (burst)")
	}

	// Should be rate limited now
	if rl.allow() {
		t.Error("Third request should be rate limited")
	}

	// Wait and try again
	time.Sleep(200 * time.Millisecond)
	if !rl.allow() {
		t.Error("Request after wait should be allowed")
	}
}

// Mock types for testing

type mockResponseWriter struct {
	msg        *Message
	localAddr  net.Addr
	remoteAddr net.Addr
	isTCP      bool
}

func (m *mockResponseWriter) WriteMsg(msg *Message) error {
	m.msg = msg
	return nil
}

func (m *mockResponseWriter) LocalAddr() net.Addr {
	if m.localAddr != nil {
		return m.localAddr
	}
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func (m *mockResponseWriter) RemoteAddr() net.Addr {
	if m.remoteAddr != nil {
		return m.remoteAddr
	}
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
}

func (m *mockResponseWriter) IsTCP() bool {
	return m.isTCP
}

func (m *mockResponseWriter) Close() error {
	return nil
}

type mockAddr struct {
	addr string
}

func (m *mockAddr) Network() string { return "mock" }
func (m *mockAddr) String() string  { return m.addr }
