// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==================== ListenerConfig Tests ====================

func TestNewListener_Defaults(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0})

	if l.config.Address != "0.0.0.0" {
		t.Errorf("default Address = %q, want %q", l.config.Address, "0.0.0.0")
	}
	if l.config.ReadTimeout != 30*time.Second {
		t.Errorf("default ReadTimeout = %v, want %v", l.config.ReadTimeout, 30*time.Second)
	}
	if l.config.WriteTimeout != 30*time.Second {
		t.Errorf("default WriteTimeout = %v, want %v", l.config.WriteTimeout, 30*time.Second)
	}
}

func TestNewListener_CustomConfig(t *testing.T) {
	cfg := ListenerConfig{
		Port:           9999,
		Address:        "127.0.0.1",
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxConnections: 50,
	}

	l := NewListener(cfg)
	if l.config.Port != 9999 {
		t.Errorf("Port = %d, want 9999", l.config.Port)
	}
	if l.config.Address != "127.0.0.1" {
		t.Errorf("Address = %q, want %q", l.config.Address, "127.0.0.1")
	}
	if l.config.MaxConnections != 50 {
		t.Errorf("MaxConnections = %d, want 50", l.config.MaxConnections)
	}
}

// ==================== Listener Start/Stop Tests ====================

func TestListener_StartAndStop(t *testing.T) {
	l := NewListener(ListenerConfig{
		Port:    0,
		Address: "127.0.0.1",
	})

	l.SetHandler(func(conn net.Conn) {
		conn.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- l.Start(ctx)
	}()

	// Wait for listener to start
	time.Sleep(100 * time.Millisecond)

	addr := l.Address()
	if addr == "" {
		t.Fatal("listener address is empty")
	}

	// Stop via context cancel
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("Start returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not stop after context cancel")
	}

	// Explicit stop should be a no-op
	err := l.Stop()
	if err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

func TestListener_StopBeforeStart(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})
	err := l.Stop()
	if err != nil {
		t.Errorf("Stop before Start returned error: %v", err)
	}
}

func TestListener_DoubleStart(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})
	l.SetHandler(func(conn net.Conn) { conn.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go l.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Second start should return nil (already running)
	err := l.Start(ctx)
	if err != nil {
		t.Errorf("second Start returned error: %v", err)
	}

	l.Stop()
}

// ==================== Listener Handler Tests ====================

func TestListener_SetHandler(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})

	var called int32
	l.SetHandler(func(conn net.Conn) {
		atomic.AddInt32(&called, 1)
		conn.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go l.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := l.Address()
	if addr == "" {
		t.Fatal("address empty")
	}

	// Connect
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	conn.Close()

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&called) == 0 {
		t.Error("handler was not called")
	}

	l.Stop()
}

func TestListener_NoHandler(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})
	// No handler set - connection should be closed immediately

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go l.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := l.Address()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	// Should be closed by listener
	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = conn.Read(buf)
	conn.Close()
	// err should be EOF or similar - connection closed

	l.Stop()
}

// ==================== Listener Stats Tests ====================

func TestListener_ActiveAndTotalConnections(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})

	holdConn := make(chan struct{})
	l.SetHandler(func(conn net.Conn) {
		<-holdConn
		conn.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go l.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := l.Address()
	if l.ActiveConnections() != 0 {
		t.Errorf("initial ActiveConnections = %d, want 0", l.ActiveConnections())
	}
	if l.TotalConnections() != 0 {
		t.Errorf("initial TotalConnections = %d, want 0", l.TotalConnections())
	}

	// Make a connection that stays held
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if l.ActiveConnections() != 1 {
		t.Errorf("ActiveConnections = %d, want 1", l.ActiveConnections())
	}
	if l.TotalConnections() != 1 {
		t.Errorf("TotalConnections = %d, want 1", l.TotalConnections())
	}

	// Release the handler
	close(holdConn)
	time.Sleep(100 * time.Millisecond)

	if l.ActiveConnections() != 0 {
		t.Errorf("ActiveConnections after release = %d, want 0", l.ActiveConnections())
	}

	conn.Close()
	l.Stop()
}

func TestListener_Errors(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})
	if l.Errors() != 0 {
		t.Errorf("initial Errors = %d, want 0", l.Errors())
	}
}

func TestListener_Address_BeforeStart(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})
	addr := l.Address()
	if addr != "" {
		t.Errorf("Address before start = %q, want empty", addr)
	}
}

// ==================== MaxConnections Tests ====================

func TestListener_MaxConnections(t *testing.T) {
	l := NewListener(ListenerConfig{
		Port:           0,
		Address:        "127.0.0.1",
		MaxConnections: 2,
	})

	holdConn := make(chan struct{})
	l.SetHandler(func(conn net.Conn) {
		<-holdConn
		conn.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go l.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := l.Address()

	// Open max connections
	conns := make([]net.Conn, 2)
	for i := 0; i < 2; i++ {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatalf("dial %d failed: %v", i, err)
		}
		conns[i] = c
	}

	time.Sleep(100 * time.Millisecond)

	// Third connection should be accepted at TCP level but closed by listener
	extraConn, err := net.DialTimeout("tcp", addr, time.Second)
	if err == nil {
		// Connection was accepted, but should be closed quickly
		buf := make([]byte, 1)
		extraConn.SetReadDeadline(time.Now().Add(time.Second))
		extraConn.Read(buf) // will get EOF
		extraConn.Close()
	}

	close(holdConn)
	for _, c := range conns {
		c.Close()
	}
	l.Stop()
}

// ==================== ErrListenerClosed Test ====================

func TestErrListenerClosed(t *testing.T) {
	if ErrListenerClosed.Error() != "listener closed" {
		t.Errorf("ErrListenerClosed = %q", ErrListenerClosed.Error())
	}
}

// ==================== HTTPListener Tests ====================

func TestNewHTTPListener(t *testing.T) {
	hl := NewHTTPListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})
	if hl == nil {
		t.Fatal("NewHTTPListener returned nil")
	}
	if hl.Listener == nil {
		t.Fatal("HTTPListener.Listener is nil")
	}
}

func TestHTTPListener_SetHTTPHandler(t *testing.T) {
	hl := NewHTTPListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})

	var capturedMethod, capturedPath string
	var mu sync.Mutex

	hl.SetHTTPHandler(func(conn net.Conn, method, path string, headers map[string]string, body []byte) {
		mu.Lock()
		capturedMethod = method
		capturedPath = path
		mu.Unlock()

		// Send a simple response
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hl.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := hl.Address()
	if addr == "" {
		t.Fatal("address empty")
	}

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Send HTTP request
	req := "GET /api/v1/test HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\n\r\n"
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	conn.Write([]byte(req))

	// Read response
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	conn.Read(buf)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if capturedMethod != "GET" {
		t.Errorf("method = %q, want %q", capturedMethod, "GET")
	}
	if capturedPath != "/api/v1/test" {
		t.Errorf("path = %q, want %q", capturedPath, "/api/v1/test")
	}
	mu.Unlock()

	hl.Stop()
}

// ==================== parseHTTPRequest Tests ====================

func TestParseHTTPRequest(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantMethod     string
		wantPath       string
		wantHeaders    map[string]string
		wantBodyNonNil bool
	}{
		{
			name:       "simple GET",
			input:      "GET /path HTTP/1.1\r\nHost: localhost\r\n\r\n",
			wantMethod: "GET",
			wantPath:   "/path",
			wantHeaders: map[string]string{
				"Host": "localhost",
			},
		},
		{
			name:       "POST with body",
			input:      "POST /api/data HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\n\r\n{\"key\":\"value\"}",
			wantMethod: "POST",
			wantPath:   "/api/data",
			wantHeaders: map[string]string{
				"Host":         "example.com",
				"Content-Type": "application/json",
			},
			wantBodyNonNil: true,
		},
		{
			name:       "empty input",
			input:      "",
			wantMethod: "",
			wantPath:   "",
		},
		{
			name:       "method only",
			input:      "GET /\r\n",
			wantMethod: "GET",
			wantPath:   "/",
		},
		{
			name:       "headers with spaces",
			input:      "GET / HTTP/1.1\r\nX-Custom:  value with spaces  \r\n\r\n",
			wantMethod: "GET",
			wantPath:   "/",
			wantHeaders: map[string]string{
				"X-Custom": "value with spaces",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, path, headers, body := parseHTTPRequest([]byte(tt.input))

			if method != tt.wantMethod {
				t.Errorf("method = %q, want %q", method, tt.wantMethod)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}

			for k, v := range tt.wantHeaders {
				if headers[k] != v {
					t.Errorf("header %q = %q, want %q", k, headers[k], v)
				}
			}

			if tt.wantBodyNonNil && body == nil {
				t.Error("expected non-nil body")
			}
		})
	}
}

// ==================== Helper Function Tests ====================

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"one", 1},
		{"one\ntwo", 2},
		{"one\r\ntwo", 2},
		{"one\r\ntwo\r\nthree", 3},
		{"trailing\n", 1},
	}

	for _, tt := range tests {
		lines := splitLines(tt.input)
		if len(lines) != tt.want {
			t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(lines), tt.want)
		}
	}
}

func TestSplitSpaces(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"one", 1},
		{"one two", 2},
		{"one  two", 2},
		{" leading", 1},
		{"trailing ", 1},
		{"GET /path HTTP/1.1", 3},
	}

	for _, tt := range tests {
		parts := splitSpaces(tt.input)
		if len(parts) != tt.want {
			t.Errorf("splitSpaces(%q) = %d parts, want %d (got %v)", tt.input, len(parts), tt.want, parts)
		}
	}
}

func TestIndexByte(t *testing.T) {
	if indexByte("hello", 'l') != 2 {
		t.Error("indexByte('hello', 'l') should be 2")
	}
	if indexByte("hello", 'x') != -1 {
		t.Error("indexByte('hello', 'x') should be -1")
	}
	if indexByte("", 'a') != -1 {
		t.Error("indexByte('', 'a') should be -1")
	}
	if indexByte("a:b", ':') != 1 {
		t.Error("indexByte('a:b', ':') should be 1")
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{" hello", "hello"},
		{"hello ", "hello"},
		{" hello ", "hello"},
		{"\thello\t", "hello"},
		{"  ", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := trimSpace(tt.input)
		if got != tt.want {
			t.Errorf("trimSpace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ==================== Listener Concurrent Access ====================

func TestListener_ConcurrentConnections(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})

	var connCount int32
	l.SetHandler(func(conn net.Conn) {
		atomic.AddInt32(&connCount, 1)
		conn.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go l.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := l.Address()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", addr, time.Second)
			if err != nil {
				return
			}
			conn.Close()
		}()
	}
	wg.Wait()

	time.Sleep(200 * time.Millisecond)

	total := l.TotalConnections()
	if total == 0 {
		t.Error("TotalConnections should be > 0 after concurrent connects")
	}

	l.Stop()
}

// ==================== isRunning Tests ====================

func TestListener_IsRunning(t *testing.T) {
	l := NewListener(ListenerConfig{Port: 0, Address: "127.0.0.1"})

	if l.isRunning() {
		t.Error("should not be running before Start")
	}

	l.SetHandler(func(conn net.Conn) { conn.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	go l.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	if !l.isRunning() {
		t.Error("should be running after Start")
	}

	cancel()
	l.Stop()
	time.Sleep(100 * time.Millisecond)

	if l.isRunning() {
		t.Error("should not be running after Stop")
	}
}
