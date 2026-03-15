// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package proxy provides the sidecar proxy for the service mesh.
// THE HAUBERK - TCP/HTTP listeners for the Unheaded Kingdom.
package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrListenerClosed is returned when the listener is closed.
	ErrListenerClosed = errors.New("listener closed")
)

// ListenerConfig configures a listener.
type ListenerConfig struct {
	// Port to listen on.
	Port int
	// Address to bind to (default: 0.0.0.0).
	Address string
	// TLSConfig for TLS connections.
	TLSConfig *tls.Config
	// ReadTimeout for connections.
	ReadTimeout time.Duration
	// WriteTimeout for connections.
	WriteTimeout time.Duration
	// MaxConnections limits concurrent connections.
	MaxConnections int
}

// ConnectionHandler handles incoming connections.
type ConnectionHandler func(conn net.Conn)

// Listener handles incoming connections.
type Listener struct {
	config ListenerConfig

	mu       sync.RWMutex
	listener net.Listener
	handler  ConnectionHandler
	running  bool

	activeConns int64
	totalConns  int64
	errors      int64
}

// NewListener creates a new listener.
func NewListener(config ListenerConfig) *Listener {
	if config.Address == "" {
		config.Address = "0.0.0.0"
	}
	if config.ReadTimeout <= 0 {
		config.ReadTimeout = 30 * time.Second
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = 30 * time.Second
	}

	return &Listener{
		config: config,
	}
}

// SetHandler sets the connection handler.
func (l *Listener) SetHandler(h ConnectionHandler) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handler = h
}

// Start starts the listener.
func (l *Listener) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return nil
	}

	addr := fmt.Sprintf("%s:%d", l.config.Address, l.config.Port)

	var listener net.Listener
	var err error

	if l.config.TLSConfig != nil {
		listener, err = tls.Listen("tcp", addr, l.config.TLSConfig)
	} else {
		listener, err = net.Listen("tcp", addr)
	}

	if err != nil {
		l.mu.Unlock()
		return fmt.Errorf("failed to start listener: %w", err)
	}

	l.listener = listener
	l.running = true
	l.mu.Unlock()

	return l.acceptLoop(ctx)
}

// acceptLoop accepts incoming connections.
func (l *Listener) acceptLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		l.mu.RLock()
		listener := l.listener
		running := l.running
		l.mu.RUnlock()

		if !running || listener == nil {
			return ErrListenerClosed
		}

		// Set accept deadline to check for context cancellation
		if tcpListener, ok := listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(time.Second))
		}

		conn, err := listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if !l.isRunning() {
				return ErrListenerClosed
			}
			atomic.AddInt64(&l.errors, 1)
			continue
		}

		// Check max connections
		if l.config.MaxConnections > 0 {
			active := atomic.LoadInt64(&l.activeConns)
			if active >= int64(l.config.MaxConnections) {
				conn.Close()
				continue
			}
		}

		atomic.AddInt64(&l.activeConns, 1)
		atomic.AddInt64(&l.totalConns, 1)

		go l.handleConnection(conn)
	}
}

// handleConnection handles a single connection.
func (l *Listener) handleConnection(conn net.Conn) {
	defer func() {
		atomic.AddInt64(&l.activeConns, -1)
	}()

	// Set timeouts
	if l.config.ReadTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(l.config.ReadTimeout))
	}
	if l.config.WriteTimeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(l.config.WriteTimeout))
	}

	l.mu.RLock()
	handler := l.handler
	l.mu.RUnlock()

	if handler != nil {
		handler(conn)
	} else {
		conn.Close()
	}
}

// Stop stops the listener.
func (l *Listener) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.running {
		return nil
	}

	l.running = false
	if l.listener != nil {
		return l.listener.Close()
	}

	return nil
}

// isRunning checks if the listener is running.
func (l *Listener) isRunning() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.running
}

// ActiveConnections returns the number of active connections.
func (l *Listener) ActiveConnections() int64 {
	return atomic.LoadInt64(&l.activeConns)
}

// TotalConnections returns the total connections handled.
func (l *Listener) TotalConnections() int64 {
	return atomic.LoadInt64(&l.totalConns)
}

// Errors returns the number of errors encountered.
func (l *Listener) Errors() int64 {
	return atomic.LoadInt64(&l.errors)
}

// Address returns the listener address.
func (l *Listener) Address() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.listener != nil {
		return l.listener.Addr().String()
	}
	return ""
}

// HTTPListener wraps a listener for HTTP handling.
type HTTPListener struct {
	*Listener
	handler HTTPHandler
}

// HTTPHandler handles HTTP requests.
type HTTPHandler func(conn net.Conn, method, path string, headers map[string]string, body []byte)

// NewHTTPListener creates a new HTTP listener.
func NewHTTPListener(config ListenerConfig) *HTTPListener {
	return &HTTPListener{
		Listener: NewListener(config),
	}
}

// SetHTTPHandler sets the HTTP handler.
func (l *HTTPListener) SetHTTPHandler(h HTTPHandler) {
	l.handler = h
	l.Listener.SetHandler(l.handleHTTPConnection)
}

// handleHTTPConnection handles an HTTP connection.
func (l *HTTPListener) handleHTTPConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	method, path, headers, body := parseHTTPRequest(buf[:n])
	if l.handler != nil {
		l.handler(conn, method, path, headers, body)
	}
}

// parseHTTPRequest parses a basic HTTP request.
func parseHTTPRequest(data []byte) (method, path string, headers map[string]string, body []byte) {
	headers = make(map[string]string)

	s := string(data)
	lines := splitLines(s)

	if len(lines) == 0 {
		return
	}

	// Parse request line
	requestLine := lines[0]
	parts := splitSpaces(requestLine)
	if len(parts) >= 2 {
		method = parts[0]
		path = parts[1]
	}

	// Parse headers
	bodyStart := 0
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			bodyStart = i + 1
			break
		}

		colonIdx := indexByte(line, ':')
		if colonIdx > 0 {
			key := line[:colonIdx]
			value := trimSpace(line[colonIdx+1:])
			headers[key] = value
		}
	}

	// Extract body
	if bodyStart > 0 && bodyStart < len(lines) {
		bodyStr := ""
		for i := bodyStart; i < len(lines); i++ {
			bodyStr += lines[i]
		}
		body = []byte(bodyStr)
	}

	return
}

// Helper functions to avoid importing strings package
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			end := i
			if end > start && s[end-1] == '\r' {
				end--
			}
			lines = append(lines, s[start:end])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitSpaces(s string) []string {
	var parts []string
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if start >= 0 {
				parts = append(parts, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		parts = append(parts, s[start:])
	}
	return parts
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
