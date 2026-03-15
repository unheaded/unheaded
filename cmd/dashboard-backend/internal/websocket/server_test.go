// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package websocket

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"unheaded/pkg/logger"
)

// testLogger creates a logger that discards output for testing
func testLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// TestServer_NewServer tests server creation
func TestServer_NewServer(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				MaxConnections: 100,
				ReadTimeout:    10 * time.Second,
				WriteTimeout:   10 * time.Second,
				BufferSize:     256,
			},
			wantErr: false,
		},
		{
			name:    "nil config uses default",
			config:  nil,
			wantErr: false,
		},
		{
			name: "zero max connections",
			config: &Config{
				MaxConnections: 0,
				ReadTimeout:    10 * time.Second,
				WriteTimeout:   10 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, err := NewServer(tt.config, testLogger())
			if (err != nil) != tt.wantErr {
				t.Errorf("NewServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && srv == nil {
				t.Error("NewServer() returned nil server")
			}
		})
	}
}

// TestServer_HandleConnection tests WebSocket connection handling
func TestServer_HandleConnection(t *testing.T) {
	config := &Config{
		MaxConnections: 10,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		BufferSize:     256,
	}

	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}

	// Start the server's run loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer srv.Shutdown(context.Background())

	// Create test HTTP server
	httpServer := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	defer httpServer.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	// Connect client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial() failed: %v", err)
	}
	defer conn.Close()

	// Verify connection count (allow time for async upgrade)
	var count int
	for range 50 {
		count = srv.ConnectionCount()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if count != 1 {
		t.Errorf("ConnectionCount() = %d, want 1", count)
	}
}

// TestServer_Broadcast tests message broadcasting to all clients
func TestServer_Broadcast(t *testing.T) {
	config := &Config{
		MaxConnections: 10,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		BufferSize:     256,
	}

	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}

	// Start the server's run loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer srv.Shutdown(context.Background())

	// Create test HTTP server
	httpServer := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	// Connect 3 clients
	clients := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("client %d: Dial() failed: %v", i, err)
		}
		defer conn.Close()
		clients[i] = conn
	}

	// Give connections time to register
	time.Sleep(100 * time.Millisecond)

	// Broadcast message
	testMsg := []byte(`{"type":"test","data":"hello"}`)
	srv.Broadcast(testMsg)

	// Verify all clients receive message
	for i, client := range clients {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := client.ReadMessage()
		if err != nil {
			t.Errorf("client %d: ReadMessage() failed: %v", i, err)
			continue
		}
		if string(msg) != string(testMsg) {
			t.Errorf("client %d: got %s, want %s", i, msg, testMsg)
		}
	}
}

// TestServer_MaxConnections tests connection limit enforcement
func TestServer_MaxConnections(t *testing.T) {
	config := &Config{
		MaxConnections: 2,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		BufferSize:     256,
	}

	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}

	// Start the server's run loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer srv.Shutdown(context.Background())

	httpServer := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	// Connect max clients
	clients := make([]*websocket.Conn, 2)
	for i := 0; i < 2; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("client %d: Dial() failed: %v", i, err)
		}
		defer conn.Close()
		clients[i] = conn
	}

	time.Sleep(100 * time.Millisecond)

	// Try to exceed limit
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		conn.Close()
		t.Error("expected connection to fail, but succeeded")
	}
	if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

// TestServer_Shutdown tests graceful shutdown
func TestServer_Shutdown(t *testing.T) {
	config := &Config{
		MaxConnections: 10,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		BufferSize:     256,
	}

	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}

	// Start the server's run loop
	startCtx, startCancel := context.WithCancel(context.Background())
	defer startCancel()
	if err := srv.Start(startCtx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	httpServer := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	// Connect client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() failed: %v", err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	// Shutdown server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	// Verify connection count is zero
	if srv.ConnectionCount() != 0 {
		t.Errorf("ConnectionCount() after shutdown = %d, want 0", srv.ConnectionCount())
	}
}

// TestServer_ConcurrentBroadcast tests concurrent broadcasts
func TestServer_ConcurrentBroadcast(t *testing.T) {
	config := &Config{
		MaxConnections: 10,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		BufferSize:     256,
	}

	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}

	// Start the server's run loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer srv.Shutdown(context.Background())

	httpServer := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	// Connect client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() failed: %v", err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	// Concurrent broadcasts
	var wg sync.WaitGroup
	broadcasts := 50

	wg.Add(broadcasts)
	for i := 0; i < broadcasts; i++ {
		go func(n int) {
			defer wg.Done()
			msg := []byte(`{"seq":` + string(rune(n)) + `}`)
			srv.Broadcast(msg)
		}(i)
	}

	wg.Wait()

	// No race conditions should occur (test with -race flag)
}
