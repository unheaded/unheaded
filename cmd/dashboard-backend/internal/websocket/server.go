// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package websocket provides a WebSocket server for real-time dashboard updates.
// This implementation uses only the Go standard library - no external dependencies.
package websocket

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
)

var (
	// ErrNilConfig indicates nil configuration
	ErrNilConfig = errors.New("config cannot be nil")
	// ErrInvalidMaxConnections indicates invalid max connections
	ErrInvalidMaxConnections = errors.New("max connections must be > 0")
	// ErrMaxConnectionsReached indicates connection limit reached
	ErrMaxConnectionsReached = errors.New("max connections reached")
	// ErrServerShutdown indicates server is shutting down
	ErrServerShutdown = errors.New("server is shutting down")
	// ErrConnectionClosed indicates connection is closed
	ErrConnectionClosed = errors.New("connection closed")
)

// WebSocket magic GUID for handshake
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Frame opcodes
const (
	opcodeContinuation = 0x0
	opcodeText         = 0x1
	opcodeBinary       = 0x2
	opcodeClose        = 0x8
	opcodePing         = 0x9
	opcodePong         = 0xA
)

// Config holds WebSocket server configuration
type Config struct {
	MaxConnections int           // Maximum concurrent connections
	ReadTimeout    time.Duration // Read timeout per message
	WriteTimeout   time.Duration // Write timeout per message
	PingInterval   time.Duration // Interval between ping frames
	BufferSize     int           // Message buffer size per client
	MaxMessageSize int64         // Maximum message size in bytes
	AllowedOrigins []string      // Allowed origins for CORS validation (empty = allow all)
}

// DefaultConfig returns default WebSocket configuration
func DefaultConfig() *Config {
	return &Config{
		MaxConnections: 100,
		ReadTimeout:    60 * time.Second,
		WriteTimeout:   10 * time.Second,
		PingInterval:   30 * time.Second,
		BufferSize:     256,
		MaxMessageSize: 65536,
	}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.MaxConnections <= 0 {
		return ErrInvalidMaxConnections
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 60 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 10 * time.Second
	}
	if c.PingInterval == 0 {
		c.PingInterval = 30 * time.Second
	}
	if c.BufferSize == 0 {
		c.BufferSize = 256
	}
	if c.MaxMessageSize == 0 {
		c.MaxMessageSize = 65536
	}
	return nil
}

// Client represents a connected WebSocket client
type Client struct {
	id        string
	conn      net.Conn
	server    *Server
	send      chan []byte
	closed    bool
	closeMu   sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
}

// ID returns the client's unique identifier
func (c *Client) ID() string {
	return c.id
}

// Send sends a message to the client
func (c *Client) Send(data []byte) error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return ErrConnectionClosed
	}
	c.closeMu.Unlock()

	select {
	case c.send <- data:
		return nil
	default:
		return errors.New("send buffer full")
	}
}

// Close closes the client connection
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.closeMu.Lock()
		c.closed = true
		c.closeMu.Unlock()

		close(c.send)
		c.conn.Close()
	})
}

// Server manages WebSocket connections and broadcasts
type Server struct {
	config    *Config
	log       *logger.Logger
	clients   map[*Client]bool
	clientsMu sync.RWMutex

	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client

	shutdown     chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup
	running      bool
	runMu        sync.RWMutex

	// clientIDCounter avoids collision risk of UnixNano()-only IDs under high throughput
	clientIDCounter int64

	// Callbacks
	onConnect    func(*Client)
	onDisconnect func(*Client)
	onMessage    func(*Client, []byte)
}

// NewServer creates a new WebSocket server
func NewServer(config *Config, log *logger.Logger) (*Server, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if log == nil {
		log = logger.New(nil)
	}

	s := &Server{
		config:     config,
		log:        log,
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		shutdown:   make(chan struct{}),
	}

	return s, nil
}

// Start starts the WebSocket server hub
func (s *Server) Start(ctx context.Context) error {
	s.runMu.Lock()
	if s.running {
		s.runMu.Unlock()
		return errors.New("server already running")
	}
	s.running = true
	s.shutdown = make(chan struct{})
	s.runMu.Unlock()

	s.wg.Add(1)
	go s.run(ctx)

	s.log.Info().Msg("websocket server started")
	return nil
}

// run handles client registration, unregistration, and broadcasting
func (s *Server) run(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			s.closeAllClients()
			return
		case <-s.shutdown:
			s.closeAllClients()
			return

		case client := <-s.register:
			s.clientsMu.Lock()
			s.clients[client] = true
			s.clientsMu.Unlock()

			s.log.Debug().
				Str("client_id", client.id).
				Int("total_clients", len(s.clients)).
				Msg("client registered")

			if s.onConnect != nil {
				go s.onConnect(client)
			}

		case client := <-s.unregister:
			s.clientsMu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				client.Close()
			}
			s.clientsMu.Unlock()

			s.log.Debug().
				Str("client_id", client.id).
				Int("total_clients", len(s.clients)).
				Msg("client unregistered")

			if s.onDisconnect != nil {
				go s.onDisconnect(client)
			}

		case message := <-s.broadcast:
			s.clientsMu.RLock()
			for client := range s.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, skip
					s.log.Warn().
						Str("client_id", client.id).
						Msg("client buffer full, dropping message")
				}
			}
			s.clientsMu.RUnlock()
		}
	}
}

// closeAllClients closes all connected clients
func (s *Server) closeAllClients() {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	for client := range s.clients {
		client.Close()
		delete(s.clients, client)
	}
}

// HandleWebSocket handles WebSocket upgrade and client management
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check connection limit
	s.clientsMu.RLock()
	count := len(s.clients)
	s.clientsMu.RUnlock()

	if count >= s.config.MaxConnections {
		s.log.Warn().
			Int("count", count).
			Int("max", s.config.MaxConnections).
			Msg("max connections reached")
		http.Error(w, "max connections reached", http.StatusServiceUnavailable)
		return
	}

	// Check if shutting down
	select {
	case <-s.shutdown:
		http.Error(w, "server shutting down", http.StatusServiceUnavailable)
		return
	default:
	}

	// Upgrade connection
	conn, err := s.upgradeConnection(w, r)
	if err != nil {
		s.log.Error().Err(err).Msg("websocket upgrade failed")
		return
	}

	// Create client with atomic counter + timestamp to avoid ID collisions
	counter := atomic.AddInt64(&s.clientIDCounter, 1)
	client := &Client{
		id:     fmt.Sprintf("%s-%d-%d", r.RemoteAddr, time.Now().Unix(), counter),
		conn:   conn,
		server: s,
		send:   make(chan []byte, s.config.BufferSize),
	}

	// Register client
	s.register <- client

	// Start client goroutines
	s.wg.Add(2)
	go s.clientWritePump(client)
	go s.clientReadPump(client)
}

// upgradeConnection performs the WebSocket handshake
func (s *Server) upgradeConnection(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	// Validate WebSocket request
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, errors.New("method not allowed")
	}

	// CORS origin validation - prevents cross-origin WebSocket hijacking
	origin := r.Header.Get("Origin")
	if origin != "" && !s.isOriginAllowed(origin) {
		s.log.Warn().
			Str("origin", origin).
			Msg("WebSocket connection rejected: origin not allowed")
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return nil, errors.New("origin not allowed")
	}

	upgrade := r.Header.Get("Upgrade")
	if !strings.EqualFold(upgrade, "websocket") {
		http.Error(w, "not a websocket request", http.StatusBadRequest)
		return nil, errors.New("not a websocket request")
	}

	// RFC 6455 requires the Connection header to contain "Upgrade" as a token.
	// Browsers may send "Upgrade", "keep-alive, Upgrade", etc. — use
	// case-insensitive token search instead of exact string comparison.
	connection := r.Header.Get("Connection")
	if !headerContainsToken(connection, "Upgrade") {
		http.Error(w, "invalid connection header", http.StatusBadRequest)
		return nil, errors.New("invalid connection header")
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("missing Sec-WebSocket-Key")
	}

	// Calculate accept key
	acceptKey := computeAcceptKey(key)

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return nil, errors.New("hijacking not supported")
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack failed: %w", err)
	}

	// Send upgrade response
	response := fmt.Sprintf(
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n"+
			"\r\n",
		acceptKey,
	)

	if _, err := bufrw.WriteString(response); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write response failed: %w", err)
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("flush failed: %w", err)
	}

	return conn, nil
}

// computeAcceptKey computes the Sec-WebSocket-Accept key
func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// clientReadPump handles reading from WebSocket connection
func (s *Server) clientReadPump(c *Client) {
	defer func() {
		select {
		case s.unregister <- c:
		case <-s.shutdown:
		}
		s.wg.Done()
	}()

	reader := bufio.NewReader(c.conn)

	for {
		// Set read deadline
		c.conn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))

		// Read frame
		opcode, payload, err := s.readFrame(reader)
		if err != nil {
			if err != io.EOF {
				s.log.Debug().
					Err(err).
					Str("client_id", c.id).
					Msg("read error")
			}
			return
		}

		switch opcode {
		case opcodeText, opcodeBinary:
			// Reject oversized client messages (1KB max for control messages)
			if len(payload) > 1024 {
				s.log.Warn().
					Str("client_id", c.id).
					Int("size", len(payload)).
					Msg("rejected oversized WebSocket message")
				continue
			}
			// Validate JSON structure for text frames — reject malformed payloads
			if opcode == opcodeText && len(payload) > 0 && !json.Valid(payload) {
				s.log.Warn().
					Str("client_id", c.id).
					Int("size", len(payload)).
					Msg("rejected malformed JSON WebSocket message")
				continue
			}
			if s.onMessage != nil {
				s.onMessage(c, payload)
			}
		case opcodePing:
			// Send pong
			s.writeFrame(c, opcodePong, payload)
		case opcodePong:
			// Pong received, connection is alive
		case opcodeClose:
			// Send close frame and exit
			s.writeFrame(c, opcodeClose, nil)
			return
		}
	}
}

// clientWritePump handles writing to WebSocket connection
func (s *Server) clientWritePump(c *Client) {
	ticker := time.NewTicker(s.config.PingInterval)
	defer func() {
		ticker.Stop()
		s.wg.Done()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// Channel closed
				s.writeFrame(c, opcodeClose, nil)
				return
			}

			if err := s.writeFrame(c, opcodeText, message); err != nil {
				s.log.Debug().
					Err(err).
					Str("client_id", c.id).
					Msg("write error")
				return
			}

		case <-ticker.C:
			if err := s.writeFrame(c, opcodePing, nil); err != nil {
				return
			}
		}
	}
}

// readFrame reads a WebSocket frame
func (s *Server) readFrame(r *bufio.Reader) (byte, []byte, error) {
	// Read first two bytes (FIN, opcode, MASK, payload length)
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	// fin := (header[0] & 0x80) != 0
	opcode := header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	length := uint64(header[1] & 0x7F)

	// Extended payload length
	switch length {
	case 126:
		extLen := make([]byte, 2)
		if _, err := io.ReadFull(r, extLen); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extLen))
	case 127:
		extLen := make([]byte, 8)
		if _, err := io.ReadFull(r, extLen); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(extLen)
	}

	// Check message size
	if length > uint64(s.config.MaxMessageSize) {
		return 0, nil, errors.New("message too large")
	}

	// Read mask key (if masked)
	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(r, maskKey); err != nil {
			return 0, nil, err
		}
	}

	// Read payload
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}

	// Unmask payload
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return opcode, payload, nil
}

// writeFrame writes a WebSocket frame
func (s *Server) writeFrame(c *Client, opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))

	length := len(payload)

	// Build frame header
	var header []byte

	// First byte: FIN + opcode
	header = append(header, 0x80|opcode)

	// Second byte: payload length (server doesn't mask)
	if length < 126 {
		header = append(header, byte(length))
	} else if length < 65536 {
		header = append(header, 126)
		extLen := make([]byte, 2)
		binary.BigEndian.PutUint16(extLen, uint16(length))
		header = append(header, extLen...)
	} else {
		header = append(header, 127)
		extLen := make([]byte, 8)
		binary.BigEndian.PutUint64(extLen, uint64(length))
		header = append(header, extLen...)
	}

	// Write header
	if _, err := c.conn.Write(header); err != nil {
		return err
	}

	// Write payload
	if length > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return err
		}
	}

	return nil
}

// Broadcast sends a message to all connected clients
func (s *Server) Broadcast(message []byte) {
	select {
	case s.broadcast <- message:
	case <-s.shutdown:
		// Server shutting down, drop message
	default:
		// Broadcast channel full, drop message
		s.log.Warn().Msg("broadcast channel full, dropping message")
	}
}

// SendTo sends a message to a specific client
func (s *Server) SendTo(clientID string, message []byte) error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for client := range s.clients {
		if client.id == clientID {
			return client.Send(message)
		}
	}

	return errors.New("client not found")
}

// ConnectionCount returns the number of active connections
func (s *Server) ConnectionCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

// GetClients returns all connected client IDs
func (s *Server) GetClients() []string {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	ids := make([]string, 0, len(s.clients))
	for client := range s.clients {
		ids = append(ids, client.id)
	}
	return ids
}

// OnConnect sets the connection callback
func (s *Server) OnConnect(fn func(*Client)) {
	s.onConnect = fn
}

// OnDisconnect sets the disconnection callback
func (s *Server) OnDisconnect(fn func(*Client)) {
	s.onDisconnect = fn
}

// OnMessage sets the message callback
func (s *Server) OnMessage(fn func(*Client, []byte)) {
	s.onMessage = fn
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error

	s.shutdownOnce.Do(func() {
		s.log.Info().Msg("shutting down websocket server")

		s.runMu.Lock()
		s.running = false
		close(s.shutdown)
		s.runMu.Unlock()

		// Wait for all goroutines with timeout
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			s.log.Info().Msg("websocket server shutdown complete")
		case <-ctx.Done():
			shutdownErr = ctx.Err()
			s.log.Error().Err(shutdownErr).Msg("websocket server shutdown timeout")
		}
	})

	return shutdownErr
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.runMu.RLock()
	defer s.runMu.RUnlock()
	return s.running
}

// DefaultAllowedOrigins returns localhost origins for development.
func DefaultAllowedOrigins() []string {
	return []string{
		"http://localhost:20000",
		"http://localhost:20001",
		"http://localhost:3000",
		"http://127.0.0.1:20000",
		"http://127.0.0.1:20001",
		"http://127.0.0.1:3000",
		"https://localhost:20000",
		"https://localhost:20001",
		"https://localhost:3000",
	}
}

// isOriginAllowed checks if the given origin is in the allowed list.
// Denies by default when no origins are configured.
func (s *Server) isOriginAllowed(origin string) bool {
	origins := s.config.AllowedOrigins
	if len(origins) == 0 {
		origins = DefaultAllowedOrigins()
	}
	for _, allowed := range origins {
		if origin == allowed {
			return true
		}
	}
	return false
}

// headerContainsToken checks whether an HTTP header value contains the given
// token as a comma-separated, case-insensitive entry per RFC 7230 section 3.2.6.
func headerContainsToken(headerValue, token string) bool {
	for _, part := range strings.Split(headerValue, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// SetAllowedOrigins sets the allowed origins for CORS validation
func (s *Server) SetAllowedOrigins(origins []string) {
	s.config.AllowedOrigins = origins
}
