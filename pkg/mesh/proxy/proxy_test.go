// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==================== DefaultConfig Tests ====================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.InboundAddr != "127.0.0.1" {
		t.Errorf("InboundAddr = %q, want %q", cfg.InboundAddr, "127.0.0.1")
	}
	if cfg.InboundPort != 15001 {
		t.Errorf("InboundPort = %d, want %d", cfg.InboundPort, 15001)
	}
	if cfg.OutboundAddr != "127.0.0.1" {
		t.Errorf("OutboundAddr = %q, want %q", cfg.OutboundAddr, "127.0.0.1")
	}
	if cfg.OutboundPort != 15006 {
		t.Errorf("OutboundPort = %d, want %d", cfg.OutboundPort, 15006)
	}
	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("ConnectTimeout = %v, want %v", cfg.ConnectTimeout, 5*time.Second)
	}
	if cfg.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 30*time.Second)
	}
	if cfg.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 30*time.Second)
	}
	if cfg.IdleTimeout != 90*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, 90*time.Second)
	}
	if cfg.ReadBufferSize != 32*1024 {
		t.Errorf("ReadBufferSize = %d, want %d", cfg.ReadBufferSize, 32*1024)
	}
	if cfg.WriteBufferSize != 32*1024 {
		t.Errorf("WriteBufferSize = %d, want %d", cfg.WriteBufferSize, 32*1024)
	}
	if cfg.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want %d", cfg.MaxIdleConns, 100)
	}
	if cfg.MaxConnsPerHost != 10 {
		t.Errorf("MaxConnsPerHost = %d, want %d", cfg.MaxConnsPerHost, 10)
	}
	if cfg.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want %v", cfg.IdleConnTimeout, 90*time.Second)
	}
}

// ==================== NewProxy Tests ====================

func TestNewProxy_NilConfig(t *testing.T) {
	p := NewProxy(nil)
	if p == nil {
		t.Fatal("NewProxy(nil) returned nil")
	}
	if p.config == nil {
		t.Fatal("proxy config is nil")
	}
	if p.config.InboundPort != 15001 {
		t.Errorf("expected default InboundPort 15001, got %d", p.config.InboundPort)
	}
	if p.poolManager == nil {
		t.Fatal("poolManager is nil")
	}
	p.Stop()
}

func TestNewProxy_CustomConfig(t *testing.T) {
	cfg := &Config{
		InboundAddr:     "0.0.0.0",
		InboundPort:     9000,
		OutboundAddr:    "0.0.0.0",
		OutboundPort:    9001,
		ConnectTimeout:  1 * time.Second,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
		IdleTimeout:     5 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    50,
		MaxConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second,
	}

	p := NewProxy(cfg)
	if p.config.InboundPort != 9000 {
		t.Errorf("expected InboundPort 9000, got %d", p.config.InboundPort)
	}
	if p.config.MaxConnsPerHost != 5 {
		t.Errorf("expected MaxConnsPerHost 5, got %d", p.config.MaxConnsPerHost)
	}
	p.Stop()
}

// ==================== Proxy Setters Tests ====================

func TestProxy_SetBackendSelector(t *testing.T) {
	p := NewProxy(nil)
	defer p.Stop()

	called := false
	p.SetBackendSelector(func(ctx context.Context) (string, error) {
		called = true
		return "backend:8080", nil
	})

	p.mu.RLock()
	fn := p.selectBackend
	p.mu.RUnlock()

	if fn == nil {
		t.Fatal("selectBackend is nil after SetBackendSelector")
	}

	addr, err := fn(context.Background())
	if err != nil {
		t.Fatalf("selectBackend returned error: %v", err)
	}
	if addr != "backend:8080" {
		t.Errorf("selectBackend returned %q, want %q", addr, "backend:8080")
	}
	if !called {
		t.Error("selectBackend function was not called")
	}
}

func TestProxy_SetTLSConfig(t *testing.T) {
	p := NewProxy(nil)
	defer p.Stop()

	p.SetTLSConfig(nil)
	p.mu.RLock()
	if p.tlsConfig != nil {
		t.Error("expected nil tlsConfig")
	}
	p.mu.RUnlock()
}

func TestProxy_SetClientTLSConfig(t *testing.T) {
	p := NewProxy(nil)
	defer p.Stop()

	p.SetClientTLSConfig(nil)
	p.mu.RLock()
	if p.clientTLSConfig != nil {
		t.Error("expected nil clientTLSConfig")
	}
	p.mu.RUnlock()
}

func TestProxy_OnRequest(t *testing.T) {
	p := NewProxy(nil)
	defer p.Stop()

	var captured *Request
	p.OnRequest(func(r *Request) {
		captured = r
	})

	p.mu.RLock()
	fn := p.onRequest
	p.mu.RUnlock()

	if fn == nil {
		t.Fatal("onRequest callback is nil")
	}

	req := &Request{Method: "GET", Path: "/test"}
	fn(req)
	if captured == nil || captured.Method != "GET" {
		t.Error("onRequest callback did not capture request correctly")
	}
}

func TestProxy_OnResponse(t *testing.T) {
	p := NewProxy(nil)
	defer p.Stop()

	var captured *Response
	p.OnResponse(func(r *Response) {
		captured = r
	})

	p.mu.RLock()
	fn := p.onResponse
	p.mu.RUnlock()

	if fn == nil {
		t.Fatal("onResponse callback is nil")
	}

	resp := &Response{StatusCode: 200, Backend: "test"}
	fn(resp)
	if captured == nil || captured.StatusCode != 200 {
		t.Error("onResponse callback did not capture response correctly")
	}
}

// ==================== Proxy Start/Stop Tests ====================

func TestProxy_StartAndStop(t *testing.T) {
	cfg := &Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     0,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    0,
		ConnectTimeout:  1 * time.Second,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     5 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second,
	}

	// Use ephemeral ports to avoid conflicts
	inLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	inPort := inLn.Addr().(*net.TCPAddr).Port
	inLn.Close()

	outLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	outPort := outLn.Addr().(*net.TCPAddr).Port
	outLn.Close()

	cfg.InboundPort = inPort
	cfg.OutboundPort = outPort

	p := NewProxy(cfg)

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Starting again should return error
	if err := p.Start(); err == nil {
		t.Error("expected error when starting already-running proxy")
	}

	// Stop
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stop again should be a no-op
	if err := p.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

// ==================== Stats Tests ====================

func TestProxy_Stats(t *testing.T) {
	p := NewProxy(nil)
	defer p.Stop()

	stats := p.Stats()
	if stats.ActiveConnections != 0 {
		t.Errorf("ActiveConnections = %d, want 0", stats.ActiveConnections)
	}
	if stats.TotalConnections != 0 {
		t.Errorf("TotalConnections = %d, want 0", stats.TotalConnections)
	}
	if stats.TotalRequests != 0 {
		t.Errorf("TotalRequests = %d, want 0", stats.TotalRequests)
	}
	if stats.TotalBytes != 0 {
		t.Errorf("TotalBytes = %d, want 0", stats.TotalBytes)
	}

	// Manually increment stats to test atomic reads
	atomic.AddInt64(&p.activeConns, 3)
	atomic.AddUint64(&p.totalConns, 10)
	atomic.AddUint64(&p.totalRequests, 5)
	atomic.AddUint64(&p.totalBytes, 1024)

	stats = p.Stats()
	if stats.ActiveConnections != 3 {
		t.Errorf("ActiveConnections = %d, want 3", stats.ActiveConnections)
	}
	if stats.TotalConnections != 10 {
		t.Errorf("TotalConnections = %d, want 10", stats.TotalConnections)
	}
	if stats.TotalRequests != 5 {
		t.Errorf("TotalRequests = %d, want 5", stats.TotalRequests)
	}
	if stats.TotalBytes != 1024 {
		t.Errorf("TotalBytes = %d, want 1024", stats.TotalBytes)
	}
}

// ==================== isHTTPMethod Tests ====================

func TestIsHTTPMethod(t *testing.T) {
	tests := []struct {
		b    byte
		want bool
	}{
		{'G', true},  // GET
		{'P', true},  // POST, PUT, PATCH
		{'D', true},  // DELETE
		{'H', true},  // HEAD
		{'O', true},  // OPTIONS
		{'C', true},  // CONNECT
		{'T', true},  // TRACE
		{'X', false}, // Not HTTP
		{'g', false}, // lowercase
		{'0', false}, // digit
		{0, false},   // null
	}

	for _, tt := range tests {
		t.Run(string(tt.b), func(t *testing.T) {
			if got := isHTTPMethod(tt.b); got != tt.want {
				t.Errorf("isHTTPMethod(%c) = %v, want %v", tt.b, got, tt.want)
			}
		})
	}
}

// ==================== sendHTTPError Tests ====================

func TestProxy_sendHTTPError(t *testing.T) {
	p := NewProxy(nil)
	defer p.Stop()

	// Use a pipe as a mock connection
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.sendHTTPError(server, http.StatusServiceUnavailable)
	}()

	// Read the HTTP response from the client side
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	<-done
}

func TestProxy_sendHTTPError_BadGateway(t *testing.T) {
	p := NewProxy(nil)
	defer p.Stop()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.sendHTTPError(server, http.StatusBadGateway)
	}()

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}

	<-done
}

// ==================== TransparentProxy Tests ====================

func TestNewTransparentProxy(t *testing.T) {
	tp := NewTransparentProxy(nil)
	if tp == nil {
		t.Fatal("NewTransparentProxy returned nil")
	}
	if tp.Proxy == nil {
		t.Fatal("TransparentProxy.Proxy is nil")
	}
	tp.Stop()
}

func TestTransparentProxy_GetOriginalDst(t *testing.T) {
	tp := NewTransparentProxy(nil)
	defer tp.Stop()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	_, err := tp.GetOriginalDst(server)
	if err == nil {
		t.Error("expected error from GetOriginalDst")
	}
}

// ==================== Error Variables Tests ====================

func TestErrorVariables(t *testing.T) {
	if ErrProxyClosed.Error() != "proxy closed" {
		t.Errorf("ErrProxyClosed = %q", ErrProxyClosed.Error())
	}
	if ErrNoBackend.Error() != "no backend available" {
		t.Errorf("ErrNoBackend = %q", ErrNoBackend.Error())
	}
	if ErrTimeout.Error() != "operation timed out" {
		t.Errorf("ErrTimeout = %q", ErrTimeout.Error())
	}
}

// ==================== HTTP Proxy End-to-End Tests ====================

func TestProxy_HTTPProxy_EndToEnd(t *testing.T) {
	// Start a backend HTTP server
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendListener.Close()

	backendAddr := backendListener.Addr().String()

	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				_, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				// Write raw HTTP response to avoid http.Response.Write nil body issues
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			}(conn)
		}
	}()

	// Get ephemeral ports for proxy
	inLn, _ := net.Listen("tcp", "127.0.0.1:0")
	inPort := inLn.Addr().(*net.TCPAddr).Port
	inLn.Close()

	outLn, _ := net.Listen("tcp", "127.0.0.1:0")
	outPort := outLn.Addr().(*net.TCPAddr).Port
	outLn.Close()

	cfg := &Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     inPort,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    outPort,
		ConnectTimeout:  2 * time.Second,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     5 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second,
	}

	p := NewProxy(cfg)
	p.SetBackendSelector(func(ctx context.Context) (string, error) {
		return backendAddr, nil
	})

	var requestCaptured bool
	var responseCaptured bool
	var mu sync.Mutex

	p.OnRequest(func(r *Request) {
		mu.Lock()
		requestCaptured = true
		mu.Unlock()
	})
	p.OnResponse(func(r *Response) {
		mu.Lock()
		responseCaptured = true
		mu.Unlock()
	})

	if err := p.Start(); err != nil {
		t.Fatalf("proxy Start failed: %v", err)
	}
	defer p.Stop()

	// Connect to the proxy inbound port and send an HTTP request
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", inPort), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer conn.Close()

	// Send HTTP request
	reqStr := "GET /test HTTP/1.1\r\nHost: " + backendAddr + "\r\nConnection: close\r\n\r\n"
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write([]byte(reqStr))
	if err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("response StatusCode = %d, want 200", resp.StatusCode)
	}

	// Verify callbacks were called
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if !requestCaptured {
		t.Error("OnRequest callback was not called")
	}
	if !responseCaptured {
		t.Error("OnResponse callback was not called")
	}
	mu.Unlock()

	// Verify stats
	stats := p.Stats()
	if stats.TotalConnections == 0 {
		t.Error("TotalConnections should be > 0")
	}
	if stats.TotalRequests == 0 {
		t.Error("TotalRequests should be > 0")
	}
}

// ==================== HTTP Proxy Error Cases ====================

func TestProxy_HTTPProxy_NoBackendSelector(t *testing.T) {
	// When no backend selector is set, proxy uses req.Host
	// With an unreachable host, it should return 502
	inLn, _ := net.Listen("tcp", "127.0.0.1:0")
	inPort := inLn.Addr().(*net.TCPAddr).Port
	inLn.Close()

	outLn, _ := net.Listen("tcp", "127.0.0.1:0")
	outPort := outLn.Addr().(*net.TCPAddr).Port
	outLn.Close()

	cfg := &Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     inPort,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    outPort,
		ConnectTimeout:  500 * time.Millisecond,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
		IdleTimeout:     2 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second,
	}

	p := NewProxy(cfg)
	// No backend selector set - uses Host header

	if err := p.Start(); err != nil {
		t.Fatalf("proxy Start failed: %v", err)
	}
	defer p.Stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", inPort), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send request to unreachable host
	reqStr := "GET /test HTTP/1.1\r\nHost: 127.0.0.1:1\r\nConnection: close\r\n\r\n"
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte(reqStr))

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestProxy_HTTPProxy_BackendSelectorError(t *testing.T) {
	inLn, _ := net.Listen("tcp", "127.0.0.1:0")
	inPort := inLn.Addr().(*net.TCPAddr).Port
	inLn.Close()

	outLn, _ := net.Listen("tcp", "127.0.0.1:0")
	outPort := outLn.Addr().(*net.TCPAddr).Port
	outLn.Close()

	cfg := &Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     inPort,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    outPort,
		ConnectTimeout:  500 * time.Millisecond,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
		IdleTimeout:     2 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second,
	}

	p := NewProxy(cfg)
	p.SetBackendSelector(func(ctx context.Context) (string, error) {
		return "", errors.New("no backend")
	})

	if err := p.Start(); err != nil {
		t.Fatalf("proxy Start failed: %v", err)
	}
	defer p.Stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", inPort), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /test HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte(reqStr))

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

// ==================== L4 Proxy Tests ====================

func TestProxy_L4Proxy_EndToEnd(t *testing.T) {
	// Backend server that reads data and sends a fixed response, then closes
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLn.Close()
	backendAddr := backendLn.Addr().String()

	go func() {
		for {
			conn, err := backendLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 256)
				n, _ := c.Read(buf)
				if n > 0 {
					c.Write(buf[:n]) // echo back what was received
				}
			}(conn)
		}
	}()

	// Proxy setup
	inLn, _ := net.Listen("tcp", "127.0.0.1:0")
	inPort := inLn.Addr().(*net.TCPAddr).Port
	inLn.Close()

	outLn, _ := net.Listen("tcp", "127.0.0.1:0")
	outPort := outLn.Addr().(*net.TCPAddr).Port
	outLn.Close()

	cfg := &Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     inPort,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    outPort,
		ConnectTimeout:  2 * time.Second,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     5 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second,
	}

	p := NewProxy(cfg)
	p.SetBackendSelector(func(ctx context.Context) (string, error) {
		return backendAddr, nil
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	// Connect and send non-HTTP data (starts with a byte not matching HTTP methods)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", inPort), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send binary data (starts with 0x01, not an HTTP method)
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write(data)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Read echoed data
	buf := make([]byte, len(data))
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("failed to read echo: %v", err)
	}
	if n != len(data) {
		t.Errorf("echoed %d bytes, want %d", n, len(data))
	}
	for i, b := range buf {
		if b != data[i] {
			t.Errorf("byte %d: got %d, want %d", i, b, data[i])
		}
	}
}

func TestProxy_L4Proxy_NoBackendSelector(t *testing.T) {
	// Without a backend selector, L4 proxy should return immediately
	inLn, _ := net.Listen("tcp", "127.0.0.1:0")
	inPort := inLn.Addr().(*net.TCPAddr).Port
	inLn.Close()

	outLn, _ := net.Listen("tcp", "127.0.0.1:0")
	outPort := outLn.Addr().(*net.TCPAddr).Port
	outLn.Close()

	cfg := &Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     inPort,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    outPort,
		ConnectTimeout:  500 * time.Millisecond,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
		IdleTimeout:     2 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second,
	}

	p := NewProxy(cfg)
	// No backend selector

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", inPort), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send non-HTTP data
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	conn.Write([]byte{0x01, 0x02})

	// Connection should be closed by proxy
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("expected connection to be closed")
	}
}

// ==================== Outbound Connection Tests ====================

func TestProxy_OutboundConnection(t *testing.T) {
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLn.Close()
	backendAddr := backendLn.Addr().String()

	go func() {
		for {
			conn, err := backendLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				_, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			}(conn)
		}
	}()

	inLn, _ := net.Listen("tcp", "127.0.0.1:0")
	inPort := inLn.Addr().(*net.TCPAddr).Port
	inLn.Close()

	outLn, _ := net.Listen("tcp", "127.0.0.1:0")
	outPort := outLn.Addr().(*net.TCPAddr).Port
	outLn.Close()

	cfg := &Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     inPort,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    outPort,
		ConnectTimeout:  2 * time.Second,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     5 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second,
	}

	p := NewProxy(cfg)
	p.SetBackendSelector(func(ctx context.Context) (string, error) {
		return backendAddr, nil
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	// Connect to outbound port
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", outPort), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to outbound: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /outbound-test HTTP/1.1\r\nHost: " + backendAddr + "\r\nConnection: close\r\n\r\n"
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte(reqStr))

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("failed to read response from outbound: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("outbound response StatusCode = %d, want 200", resp.StatusCode)
	}
}

// ==================== Concurrent Proxy Access ====================

func TestProxy_ConcurrentConnections(t *testing.T) {
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLn.Close()
	backendAddr := backendLn.Addr().String()

	go func() {
		for {
			conn, err := backendLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				_, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			}(conn)
		}
	}()

	inLn, _ := net.Listen("tcp", "127.0.0.1:0")
	inPort := inLn.Addr().(*net.TCPAddr).Port
	inLn.Close()

	outLn, _ := net.Listen("tcp", "127.0.0.1:0")
	outPort := outLn.Addr().(*net.TCPAddr).Port
	outLn.Close()

	cfg := &Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     inPort,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    outPort,
		ConnectTimeout:  2 * time.Second,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     5 * time.Second,
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxIdleConns:    50,
		MaxConnsPerHost: 20,
		IdleConnTimeout: 30 * time.Second,
	}

	p := NewProxy(cfg)
	p.SetBackendSelector(func(ctx context.Context) (string, error) {
		return backendAddr, nil
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	var wg sync.WaitGroup
	const numConns = 10

	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", inPort), 2*time.Second)
			if err != nil {
				return
			}
			defer conn.Close()

			reqStr := "GET /concurrent HTTP/1.1\r\nHost: " + backendAddr + "\r\nConnection: close\r\n\r\n"
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			conn.Write([]byte(reqStr))

			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				return
			}
			resp.Body.Close()
		}()
	}

	wg.Wait()

	stats := p.Stats()
	if stats.TotalConnections < uint64(numConns/2) {
		t.Errorf("TotalConnections = %d, expected at least %d", stats.TotalConnections, numConns/2)
	}
}

// ==================== Request/Response Types Tests ====================

func TestRequestType(t *testing.T) {
	r := &Request{
		Method:     "POST",
		Path:       "/api/v1/data",
		Host:       "localhost:8080",
		Headers:    http.Header{"Content-Type": {"application/json"}},
		RemoteAddr: "127.0.0.1:12345",
		StartTime:  time.Now(),
		Inbound:    true,
	}

	if r.Method != "POST" {
		t.Errorf("Method = %q, want %q", r.Method, "POST")
	}
	if r.Path != "/api/v1/data" {
		t.Errorf("Path = %q, want %q", r.Path, "/api/v1/data")
	}
	if !r.Inbound {
		t.Error("Inbound should be true")
	}
}

func TestResponseType(t *testing.T) {
	r := &Response{
		StatusCode: 201,
		Headers:    http.Header{"X-Custom": {"value"}},
		Duration:   100 * time.Millisecond,
		Backend:    "10.0.0.1:8080",
		Error:      nil,
	}

	if r.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", r.StatusCode)
	}
	if r.Backend != "10.0.0.1:8080" {
		t.Errorf("Backend = %q", r.Backend)
	}
	if r.Error != nil {
		t.Errorf("Error = %v, want nil", r.Error)
	}
}

func TestProxyStatsType(t *testing.T) {
	ps := ProxyStats{
		ActiveConnections: 5,
		TotalConnections:  100,
		TotalRequests:     200,
		TotalBytes:        1024 * 1024,
		PoolStats:         map[string]PoolStats{"backend:8080": {NumOpen: 3, NumIdle: 1}},
	}

	if ps.ActiveConnections != 5 {
		t.Errorf("ActiveConnections = %d", ps.ActiveConnections)
	}
	if ps.PoolStats["backend:8080"].NumOpen != 3 {
		t.Error("PoolStats not properly stored")
	}
}
