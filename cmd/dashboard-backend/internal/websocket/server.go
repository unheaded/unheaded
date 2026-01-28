// Package websocket provides a WebSocket server for real-time dashboard updates.
package websocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
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
)

// Config holds WebSocket server configuration
type Config struct {
	MaxConnections int           // Maximum concurrent connections
	ReadTimeout    time.Duration // Read timeout per message
	WriteTimeout   time.Duration // Write timeout per message
	BufferSize     int           // Message buffer size per client
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
		c.ReadTimeout = 10 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 10 * time.Second
	}
	if c.BufferSize == 0 {
		c.BufferSize = 256
	}
	return nil
}

// Client represents a connected WebSocket client
type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	server   *Server
	mu       sync.Mutex
	closed   bool
	clientID string
}

// Server manages WebSocket connections and broadcasts
type Server struct {
	config    *Config
	upgrader  websocket.Upgrader
	clients   map[*Client]bool
	clientsMu sync.RWMutex

	broadcast chan []byte
	register  chan *Client
	unregister chan *Client

	shutdown     chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup
}

// NewServer creates a new WebSocket server
func NewServer(config *Config) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	s := &Server{
		config: config,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Implement proper origin validation in production
				return true
			},
		},
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		shutdown:   make(chan struct{}),
	}

	// Start hub goroutine
	s.wg.Add(1)
	go s.run()

	return s, nil
}

// run handles client registration, unregistration, and broadcasting
func (s *Server) run() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdown:
			// Close all clients
			s.clientsMu.Lock()
			for client := range s.clients {
				s.closeClient(client)
			}
			s.clients = make(map[*Client]bool)
			s.clientsMu.Unlock()
			return

		case client := <-s.register:
			s.clientsMu.Lock()
			s.clients[client] = true
			s.clientsMu.Unlock()
			log.Debug().Str("client_id", client.clientID).Msg("client registered")

		case client := <-s.unregister:
			s.clientsMu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				s.closeClient(client)
			}
			s.clientsMu.Unlock()
			log.Debug().Str("client_id", client.clientID).Msg("client unregistered")

		case message := <-s.broadcast:
			s.clientsMu.RLock()
			for client := range s.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, skip
					log.Warn().Str("client_id", client.clientID).Msg("client buffer full, dropping message")
				}
			}
			s.clientsMu.RUnlock()
		}
	}
}

// closeClient closes a client connection
func (s *Server) closeClient(client *Client) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if !client.closed {
		close(client.send)
		client.conn.Close()
		client.closed = true
	}
}

// HandleWebSocket handles WebSocket upgrade and client management
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check connection limit
	s.clientsMu.RLock()
	count := len(s.clients)
	s.clientsMu.RUnlock()

	if count >= s.config.MaxConnections {
		log.Warn().Int("count", count).Int("max", s.config.MaxConnections).Msg("max connections reached")
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
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("websocket upgrade failed")
		return
	}

	// Create client
	client := &Client{
		conn:     conn,
		send:     make(chan []byte, s.config.BufferSize),
		server:   s,
		clientID: fmt.Sprintf("%s-%d", r.RemoteAddr, time.Now().UnixNano()),
	}

	// Register client
	s.register <- client

	// Start client goroutines
	s.wg.Add(2)
	go client.writePump()
	go client.readPump()
}

// readPump handles reading from WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.server.unregister <- c
		c.server.wg.Done()
	}()

	c.conn.SetReadDeadline(time.Now().Add(c.server.config.ReadTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(c.server.config.ReadTimeout))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("client_id", c.clientID).Msg("read error")
			}
			break
		}
		// Dashboard clients only receive, don't send
		// Ignore any incoming messages
	}
}

// writePump handles writing to WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second) // Ping interval
	defer func() {
		ticker.Stop()
		c.server.wg.Done()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(c.server.config.WriteTimeout))
			if !ok {
				// Channel closed
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Error().Err(err).Str("client_id", c.clientID).Msg("write error")
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(c.server.config.WriteTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Broadcast sends a message to all connected clients
func (s *Server) Broadcast(message []byte) {
	select {
	case s.broadcast <- message:
	case <-s.shutdown:
		// Server shutting down, drop message
	default:
		// Broadcast channel full, drop message
		log.Warn().Msg("broadcast channel full, dropping message")
	}
}

// ConnectionCount returns the number of active connections
func (s *Server) ConnectionCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error

	s.shutdownOnce.Do(func() {
		log.Info().Msg("shutting down websocket server")
		close(s.shutdown)

		// Wait for all goroutines with timeout
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Info().Msg("websocket server shutdown complete")
		case <-ctx.Done():
			shutdownErr = ctx.Err()
			log.Error().Err(shutdownErr).Msg("websocket server shutdown timeout")
		}
	})

	return shutdownErr
}
