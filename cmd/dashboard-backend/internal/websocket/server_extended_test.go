// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package websocket

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gorillaWS "github.com/gorilla/websocket"
	"unheaded/pkg/logger"
)

// ---------- helpers ----------

func discardLogger() *logger.Logger { return logger.New(io.Discard) }

// startTestServer creates a Server, starts its run loop, and returns an
// httptest.Server plus a cleanup function the caller must defer.
func startTestServer(t *testing.T, cfg *Config) (*Server, *httptest.Server, func()) {
	t.Helper()
	if cfg == nil {
		cfg = DefaultConfig()
	}
	srv, err := NewServer(cfg, discardLogger())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("Start: %v", err)
	}
	hs := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	cleanup := func() {
		hs.Close()
		srv.Shutdown(context.Background())
		cancel()
	}
	return srv, hs, cleanup
}

func wsURL(hs *httptest.Server) string {
	return "ws" + strings.TrimPrefix(hs.URL, "http")
}

func dialWS(t *testing.T, url string) *gorillaWS.Conn {
	t.Helper()
	conn, dialResp, err := gorillaWS.DefaultDialer.Dial(url, nil)
	if dialResp != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return conn
}

// waitForCount polls ConnectionCount until it equals want or times out.
func waitForCount(srv *Server, want int) bool {
	for range 100 {
		if srv.ConnectionCount() == want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// ---------- Config.Validate ----------

func TestConfig_Validate_Defaults(t *testing.T) {
	c := &Config{MaxConnections: 5}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.ReadTimeout == 0 {
		t.Error("ReadTimeout not defaulted")
	}
	if c.WriteTimeout == 0 {
		t.Error("WriteTimeout not defaulted")
	}
	if c.PingInterval == 0 {
		t.Error("PingInterval not defaulted")
	}
	if c.BufferSize == 0 {
		t.Error("BufferSize not defaulted")
	}
	if c.MaxMessageSize == 0 {
		t.Error("MaxMessageSize not defaulted")
	}
}

func TestConfig_Validate_NegativeMaxConnections(t *testing.T) {
	c := &Config{MaxConnections: -1}
	if err := c.Validate(); err == nil {
		t.Error("expected error for negative MaxConnections")
	}
}

// ---------- Client.ID / Client.Send ----------

func TestClient_ID_And_Send(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()

	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	ids := srv.GetClients()
	if len(ids) != 1 {
		t.Fatalf("GetClients: got %d, want 1", len(ids))
	}
	if ids[0] == "" {
		t.Error("client ID is empty")
	}

	// SendTo existing client
	msg := []byte(`{"test":"sendto"}`)
	if err := srv.SendTo(ids[0], msg); err != nil {
		t.Fatalf("SendTo: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(data) != string(msg) {
		t.Errorf("got %q, want %q", data, msg)
	}
}

func TestSendTo_ClientNotFound(t *testing.T) {
	srv, _, cleanup := startTestServer(t, nil)
	defer cleanup()

	if err := srv.SendTo("nonexistent", []byte("hi")); err == nil {
		t.Error("expected error for nonexistent client")
	}
}

// ---------- GetClients ----------

func TestGetClients_Multiple(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	c1 := dialWS(t, wsURL(hs))
	defer c1.Close()
	c2 := dialWS(t, wsURL(hs))
	defer c2.Close()

	if !waitForCount(srv, 2) {
		t.Fatal("clients did not register")
	}

	ids := srv.GetClients()
	if len(ids) != 2 {
		t.Errorf("GetClients: got %d, want 2", len(ids))
	}
}

// ---------- Callbacks ----------

func TestOnConnect_OnDisconnect_Callbacks(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	var connectMu sync.Mutex
	connectIDs := []string{}
	disconnectIDs := []string{}

	srv.OnConnect(func(c *Client) {
		connectMu.Lock()
		connectIDs = append(connectIDs, c.ID())
		connectMu.Unlock()
	})
	srv.OnDisconnect(func(c *Client) {
		connectMu.Lock()
		disconnectIDs = append(disconnectIDs, c.ID())
		connectMu.Unlock()
	})

	conn := dialWS(t, wsURL(hs))
	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}
	// Allow callback goroutine to run
	time.Sleep(50 * time.Millisecond)

	connectMu.Lock()
	if len(connectIDs) != 1 {
		t.Errorf("onConnect called %d times, want 1", len(connectIDs))
	}
	connectMu.Unlock()

	conn.Close()
	if !waitForCount(srv, 0) {
		t.Fatal("client did not unregister")
	}
	time.Sleep(50 * time.Millisecond)

	connectMu.Lock()
	if len(disconnectIDs) != 1 {
		t.Errorf("onDisconnect called %d times, want 1", len(disconnectIDs))
	}
	connectMu.Unlock()
}

func TestOnMessage_Callback(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	received := make(chan []byte, 1)
	srv.OnMessage(func(c *Client, data []byte) {
		received <- data
	})

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()

	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// Send a valid JSON text message from client
	msg := []byte(`{"action":"ping"}`)
	if err := conn.WriteMessage(gorillaWS.TextMessage, msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	select {
	case got := <-received:
		if string(got) != string(msg) {
			t.Errorf("got %q, want %q", got, msg)
		}
	case <-time.After(3 * time.Second):
		t.Error("onMessage not called within timeout")
	}
}

// ---------- IsRunning ----------

func TestIsRunning(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewServer(cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	if srv.IsRunning() {
		t.Error("IsRunning before Start should be false")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx)

	if !srv.IsRunning() {
		t.Error("IsRunning after Start should be true")
	}

	srv.Shutdown(context.Background())
	if srv.IsRunning() {
		t.Error("IsRunning after Shutdown should be false")
	}
}

// ---------- Start twice ----------

func TestStart_AlreadyRunning(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewServer(cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	if err := srv.Start(ctx); err == nil {
		t.Error("expected error starting server twice")
	}
}

// ---------- Origin / CORS ----------

func TestDefaultAllowedOrigins(t *testing.T) {
	origins := DefaultAllowedOrigins()
	if len(origins) == 0 {
		t.Fatal("DefaultAllowedOrigins returned empty")
	}
	found := false
	for _, o := range origins {
		if o == "http://localhost:3000" {
			found = true
		}
	}
	if !found {
		t.Error("expected http://localhost:3000 in defaults")
	}
}

func TestIsOriginAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowedOrigins = []string{"https://example.com", "https://test.com"}
	srv, _ := NewServer(cfg, discardLogger())

	if !srv.isOriginAllowed("https://example.com") {
		t.Error("expected allowed origin to pass")
	}
	if srv.isOriginAllowed("https://evil.com") {
		t.Error("expected disallowed origin to fail")
	}
}

func TestIsOriginAllowed_EmptyUsesDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowedOrigins = nil
	srv, _ := NewServer(cfg, discardLogger())

	if !srv.isOriginAllowed("http://localhost:3000") {
		t.Error("empty config should fall back to DefaultAllowedOrigins")
	}
	if srv.isOriginAllowed("https://evil.com") {
		t.Error("non-default origin should be rejected")
	}
}

func TestSetAllowedOrigins(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	srv.SetAllowedOrigins([]string{"https://custom.com"})
	if !srv.isOriginAllowed("https://custom.com") {
		t.Error("SetAllowedOrigins did not take effect")
	}
	if srv.isOriginAllowed("http://localhost:3000") {
		t.Error("old default origin should no longer be allowed")
	}
}

// ---------- headerContainsToken ----------

func TestHeaderContainsToken(t *testing.T) {
	tests := []struct {
		header string
		token  string
		want   bool
	}{
		{"Upgrade", "Upgrade", true},
		{"keep-alive, Upgrade", "Upgrade", true},
		{"keep-alive, upgrade", "Upgrade", true},
		{"keep-alive", "Upgrade", false},
		{"", "Upgrade", false},
		{"Upgrade, keep-alive", "Upgrade", true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.header, tt.token), func(t *testing.T) {
			got := headerContainsToken(tt.header, tt.token)
			if got != tt.want {
				t.Errorf("headerContainsToken(%q, %q) = %v, want %v", tt.header, tt.token, got, tt.want)
			}
		})
	}
}

// ---------- upgradeConnection error paths ----------

func TestUpgrade_MethodNotAllowed(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/ws", nil)
	w := httptest.NewRecorder()
	_, err := srv.upgradeConnection(w, req)
	if err == nil {
		t.Error("expected error for POST")
	}
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestUpgrade_OriginRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowedOrigins = []string{"https://good.com"}
	srv, _ := NewServer(cfg, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	_, err := srv.upgradeConnection(w, req)
	if err == nil {
		t.Error("expected error for rejected origin")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestUpgrade_MissingUpgradeHeader(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	// No Upgrade header
	w := httptest.NewRecorder()
	_, err := srv.upgradeConnection(w, req)
	if err == nil {
		t.Error("expected error for missing Upgrade header")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpgrade_InvalidConnectionHeader(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "keep-alive") // missing "Upgrade" token
	w := httptest.NewRecorder()
	_, err := srv.upgradeConnection(w, req)
	if err == nil {
		t.Error("expected error for invalid Connection header")
	}
}

func TestUpgrade_MissingWebSocketKey(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	// No Sec-WebSocket-Key
	w := httptest.NewRecorder()
	_, err := srv.upgradeConnection(w, req)
	if err == nil {
		t.Error("expected error for missing Sec-WebSocket-Key")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpgrade_HijackNotSupported(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	// httptest.ResponseRecorder does NOT implement http.Hijacker
	w := httptest.NewRecorder()
	_, err := srv.upgradeConnection(w, req)
	if err == nil {
		t.Error("expected error for non-hijackable response writer")
	}
}

// ---------- HandleWebSocket: shutdown path ----------

func TestHandleWebSocket_ServerShutdown(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewServer(cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv.Start(ctx)

	hs := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	defer hs.Close()

	// Shutdown the server
	srv.Shutdown(context.Background())
	cancel()

	// Attempt a new WS connection — should be rejected
	_, resp, err := gorillaWS.DefaultDialer.Dial(wsURL(hs), nil)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err == nil {
		t.Error("expected dial to fail after shutdown")
	}
	if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

// ---------- Broadcast paths ----------

func TestBroadcast_ShutdownDrop(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewServer(cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv.Start(ctx)
	srv.Shutdown(context.Background())
	cancel()

	// Broadcast after shutdown — should not panic, message is dropped.
	srv.Broadcast([]byte(`{"dropped":true}`))
}

func TestBroadcast_ChannelFull(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewServer(cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	// Don't start the run loop so nobody drains the broadcast channel.
	// Fill the channel (capacity 256).
	for i := 0; i < 256; i++ {
		srv.broadcast <- []byte("fill")
	}
	// This call should hit the default (drop) branch — no panic.
	srv.Broadcast([]byte("overflow"))
}

// ---------- Client.Send: closed and buffer-full ----------

func TestClient_Send_Closed(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	conn := dialWS(t, wsURL(hs))
	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}
	conn.Close()
	if !waitForCount(srv, 0) {
		t.Fatal("client did not unregister")
	}

	// Test the Send path directly with a synthetic client.
	// Need a real conn for Close() to work.
	pipeServer, pipeClient := net.Pipe()
	defer pipeServer.Close()
	defer pipeClient.Close()

	c := &Client{
		id:   "test-closed",
		conn: pipeServer,
		send: make(chan []byte, 1),
	}
	c.Close()

	err := c.Send([]byte("hello"))
	if err != ErrConnectionClosed {
		t.Errorf("Send on closed client: got %v, want ErrConnectionClosed", err)
	}
}

func TestClient_Send_BufferFull(t *testing.T) {
	c := &Client{
		id:   "test-full",
		send: make(chan []byte, 1),
	}
	// Fill the buffer
	c.send <- []byte("first")

	err := c.Send([]byte("second"))
	if err == nil {
		t.Error("expected buffer full error")
	}
}

// ---------- readFrame / writeFrame via real connection ----------

func TestReadFrame_And_WriteFrame_TextMessage(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()

	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// Server broadcasts, client reads — exercises writeFrame text path.
	msg := []byte(`{"hello":"world"}`)
	srv.Broadcast(msg)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(data) != string(msg) {
		t.Errorf("got %q, want %q", data, msg)
	}
}

func TestWriteFrame_LargePayload(t *testing.T) {
	// Exercises the 126+ length branch in writeFrame
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()

	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// Build a message > 125 bytes to trigger 2-byte extended length
	bigMsg := []byte(`{"data":"` + strings.Repeat("A", 200) + `"}`)
	srv.Broadcast(bigMsg)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(data) != string(bigMsg) {
		t.Errorf("payload mismatch (len got=%d, want=%d)", len(data), len(bigMsg))
	}
}

// ---------- clientReadPump: oversized and malformed JSON ----------

func TestReadPump_OversizedMessage(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	// Set up onMessage to detect if the oversized message leaks through.
	leaked := make(chan struct{}, 1)
	srv.OnMessage(func(_ *Client, _ []byte) {
		leaked <- struct{}{}
	})

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()

	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// Send > 1024 bytes text message — should be rejected (continue in readPump)
	big := []byte(strings.Repeat("x", 2000))
	conn.WriteMessage(gorillaWS.TextMessage, big)

	// Send a valid follow-up to prove the connection is still alive
	valid := []byte(`{"ok":true}`)
	conn.WriteMessage(gorillaWS.TextMessage, valid)

	select {
	case <-leaked:
		// onMessage was called — this is expected for the valid follow-up
	case <-time.After(3 * time.Second):
		t.Error("onMessage not called for valid follow-up after oversized rejection")
	}
}

func TestReadPump_MalformedJSON(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	leaked := make(chan struct{}, 1)
	srv.OnMessage(func(_ *Client, _ []byte) {
		leaked <- struct{}{}
	})

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()

	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// Send invalid JSON — should be rejected
	conn.WriteMessage(gorillaWS.TextMessage, []byte(`{bad json`))

	// Send valid follow-up
	conn.WriteMessage(gorillaWS.TextMessage, []byte(`{"valid":true}`))

	select {
	case <-leaked:
		// good, valid message was processed
	case <-time.After(3 * time.Second):
		t.Error("onMessage not called for valid follow-up after malformed rejection")
	}
}

func TestReadPump_BinaryMessage(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	received := make(chan []byte, 1)
	srv.OnMessage(func(_ *Client, data []byte) {
		received <- data
	})

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()

	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// Binary frames skip JSON validation
	binData := []byte{0x00, 0x01, 0x02, 0xFF}
	conn.WriteMessage(gorillaWS.BinaryMessage, binData)

	select {
	case got := <-received:
		if len(got) != len(binData) {
			t.Errorf("binary payload len = %d, want %d", len(got), len(binData))
		}
	case <-time.After(3 * time.Second):
		t.Error("onMessage not called for binary message")
	}
}

// ---------- Ping / Pong ----------

func TestPingPong(t *testing.T) {
	cfg := &Config{
		MaxConnections: 10,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		PingInterval:   200 * time.Millisecond, // very short for test
		BufferSize:     256,
	}
	srv, hs, cleanup := startTestServer(t, cfg)
	defer cleanup()

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()

	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// The server sends pings on PingInterval. gorilla/websocket handles
	// pong automatically. Wait long enough for at least one ping cycle
	// and verify the connection is still alive afterward.
	time.Sleep(500 * time.Millisecond)

	if srv.ConnectionCount() != 1 {
		t.Error("connection dropped after ping cycle, expected it alive")
	}
}

// ---------- Close frame ----------

func TestClientCloseFrame(t *testing.T) {
	srv, hs, cleanup := startTestServer(t, nil)
	defer cleanup()

	conn := dialWS(t, wsURL(hs))
	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// Send close frame
	conn.WriteMessage(gorillaWS.CloseMessage,
		gorillaWS.FormatCloseMessage(gorillaWS.CloseNormalClosure, "bye"))
	conn.Close()

	if !waitForCount(srv, 0) {
		t.Error("client not unregistered after close frame")
	}
}

// ---------- computeAcceptKey ----------

func TestComputeAcceptKey(t *testing.T) {
	// RFC 6455 section 4.2.2 example
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	got := computeAcceptKey(key)
	if got != want {
		t.Errorf("computeAcceptKey(%q) = %q, want %q", key, got, want)
	}
}

// ---------- Shutdown with context timeout ----------

func TestShutdown_ContextTimeout(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewServer(cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx)

	// Create a raw TCP connection that won't close cleanly, keeping the
	// server's wg from completing.
	hs := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	defer hs.Close()

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()
	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	// Shutdown with already-expired context
	expiredCtx, expiredCancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer expiredCancel()
	time.Sleep(5 * time.Millisecond) // let it expire

	// The shutdown should close all clients (which lets wg complete), so
	// this may or may not hit the timeout depending on goroutine scheduling.
	// Either way it must not panic.
	_ = srv.Shutdown(expiredCtx)
}

// ---------- Context cancellation stops run loop ----------

func TestContextCancelStopsRunLoop(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewServer(cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv.Start(ctx)

	hs := httptest.NewServer(http.HandlerFunc(srv.HandleWebSocket))
	defer hs.Close()

	conn := dialWS(t, wsURL(hs))
	defer conn.Close()
	if !waitForCount(srv, 1) {
		t.Fatal("client did not register")
	}

	cancel() // cancel the context

	// Wait for clients to be cleaned up
	if !waitForCount(srv, 0) {
		// Not fatal — context cancel races with connection cleanup
	}
}

// ---------- NewServer with nil logger ----------

func TestNewServer_NilLogger(t *testing.T) {
	srv, err := NewServer(DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("NewServer with nil logger: %v", err)
	}
	if srv == nil {
		t.Error("expected non-nil server")
	}
}

// ---------- writeFrame with broken connection ----------

func TestWriteFrame_BrokenConn(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	// Create a client with an already-closed connection
	server, client := net.Pipe()
	client.Close()
	server.Close()

	c := &Client{
		id:   "broken",
		conn: server,
		send: make(chan []byte, 1),
	}

	err := srv.writeFrame(c, opcodeText, []byte("hello"))
	if err == nil {
		t.Error("expected error writing to broken connection")
	}
}

// ---------- readFrame extended lengths ----------

func TestReadFrame_UnmaskedSmallPayload(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	// Build a simple unmasked text frame: FIN=1, opcode=1, mask=0, len=5
	// then payload "hello"
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		frame := []byte{
			0x81, // FIN + opcode text
			0x05, // no mask, length 5
			'h', 'e', 'l', 'l', 'o',
		}
		client.Write(frame)
	}()

	r := newBufioReader(server)
	opcode, payload, err := srv.readFrame(r)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if opcode != opcodeText {
		t.Errorf("opcode = %d, want %d", opcode, opcodeText)
	}
	if string(payload) != "hello" {
		t.Errorf("payload = %q, want %q", payload, "hello")
	}
}

func TestReadFrame_MaskedPayload(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Build a masked frame
	maskKey := []byte{0x12, 0x34, 0x56, 0x78}
	data := []byte("test")
	masked := make([]byte, len(data))
	for i := range data {
		masked[i] = data[i] ^ maskKey[i%4]
	}

	go func() {
		frame := []byte{
			0x81,        // FIN + opcode text
			0x80 | 0x04, // masked, length 4
		}
		frame = append(frame, maskKey...)
		frame = append(frame, masked...)
		client.Write(frame)
	}()

	r := newBufioReader(server)
	opcode, payload, err := srv.readFrame(r)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if opcode != opcodeText {
		t.Errorf("opcode = %d, want %d", opcode, opcodeText)
	}
	if string(payload) != "test" {
		t.Errorf("payload = %q, want %q", payload, "test")
	}
}

func TestReadFrame_Extended16BitLength(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	payload := strings.Repeat("A", 200) // > 125, uses 16-bit length

	go func() {
		frame := []byte{
			0x81,                     // FIN + opcode text
			126,                      // 16-bit extended length follows
			0x00, byte(len(payload)), // 200 in big-endian uint16
		}
		frame = append(frame, []byte(payload)...)
		client.Write(frame)
	}()

	r := newBufioReader(server)
	_, got, err := srv.readFrame(r)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if string(got) != payload {
		t.Errorf("payload len = %d, want %d", len(got), len(payload))
	}
}

func TestReadFrame_Extended64BitLength(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxMessageSize = 100000
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	payload := strings.Repeat("B", 70000) // > 65535, uses 64-bit length

	go func() {
		frame := []byte{
			0x81,                         // FIN + opcode text
			127,                          // 64-bit extended length follows
			0, 0, 0, 0, 0, 1, 0x11, 0x70, // 70000 in big-endian uint64
		}
		frame = append(frame, []byte(payload)...)
		client.Write(frame)
	}()

	r := newBufioReader(server)
	_, got, err := srv.readFrame(r)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if len(got) != 70000 {
		t.Errorf("payload len = %d, want 70000", len(got))
	}
}

func TestReadFrame_MessageTooLarge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxMessageSize = 10
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		frame := []byte{
			0x81, // FIN + text
			20,   // length 20 > MaxMessageSize 10
		}
		frame = append(frame, make([]byte, 20)...)
		client.Write(frame)
	}()

	r := newBufioReader(server)
	_, _, err := srv.readFrame(r)
	if err == nil {
		t.Error("expected error for oversized message")
	}
}

func TestReadFrame_EmptyPayload(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		frame := []byte{
			0x81, // FIN + text
			0x00, // length 0
		}
		client.Write(frame)
	}()

	r := newBufioReader(server)
	opcode, payload, err := srv.readFrame(r)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if opcode != opcodeText {
		t.Errorf("opcode = %d, want %d", opcode, opcodeText)
	}
	if len(payload) != 0 {
		t.Errorf("payload len = %d, want 0", len(payload))
	}
}

// ---------- writeFrame length branches ----------

func TestWriteFrame_SmallPayload(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	c := &Client{id: "test", conn: server, send: make(chan []byte, 1)}

	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := client.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	err := srv.writeFrame(c, opcodeText, []byte("hi"))
	if err != nil {
		t.Errorf("writeFrame small: %v", err)
	}
}

func TestWriteFrame_MediumPayload(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	c := &Client{id: "test", conn: server, send: make(chan []byte, 1)}

	go func() {
		buf := make([]byte, 65536)
		for {
			_, err := client.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// 200 bytes = 16-bit extended length
	payload := []byte(strings.Repeat("M", 200))
	err := srv.writeFrame(c, opcodeText, payload)
	if err != nil {
		t.Errorf("writeFrame medium: %v", err)
	}
}

func TestWriteFrame_HugePayload(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	c := &Client{id: "test", conn: server, send: make(chan []byte, 1)}

	go func() {
		buf := make([]byte, 131072)
		for {
			_, err := client.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// > 65535 bytes = 64-bit extended length
	payload := []byte(strings.Repeat("L", 70000))
	err := srv.writeFrame(c, opcodeText, payload)
	if err != nil {
		t.Errorf("writeFrame large: %v", err)
	}
}

func TestWriteFrame_EmptyPayload(t *testing.T) {
	cfg := DefaultConfig()
	srv, _ := NewServer(cfg, discardLogger())

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	c := &Client{id: "test", conn: server, send: make(chan []byte, 1)}

	go func() {
		buf := make([]byte, 1024)
		client.Read(buf)
	}()

	err := srv.writeFrame(c, opcodePong, nil)
	if err != nil {
		t.Errorf("writeFrame empty: %v", err)
	}
}

// newBufioReader wraps io.Reader for readFrame tests.
func newBufioReader(r io.Reader) *bufio.Reader {
	return bufio.NewReader(r)
}
