package loadbalancer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// L4Proxy handles Layer 4 (TCP/UDP) load balancing.
type L4Proxy struct {
	config         *Config
	pool           *BackendPool
	health         *HealthAggregator
	sessionManager *SessionAffinity

	// TCP listener
	tcpListener net.Listener

	// UDP connection for UDP proxy
	udpConn *net.UDPConn

	// TLS config for TLS termination
	tlsConfig *tls.Config

	// Connection tracking
	activeConns   int64
	totalConns    int64
	totalBytes    int64
	droppedConns  int64

	// State management
	running int32
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// Connection pools per backend
	connPools   map[string]*ConnectionPool
	connPoolsMu sync.RWMutex

	// Callbacks
	onConnect    func(clientAddr, backendAddr string)
	onDisconnect func(clientAddr, backendAddr string, duration time.Duration, bytesIn, bytesOut int64)
	onError      func(clientAddr string, err error)
}

// NewL4Proxy creates a new Layer 4 proxy.
func NewL4Proxy(config *Config, pool *BackendPool, health *HealthAggregator) (*L4Proxy, error) {
	proxy := &L4Proxy{
		config:         config,
		pool:           pool,
		health:         health,
		sessionManager: NewSessionAffinity(config.SessionPersistence.TTL, config.SessionPersistence.MaxEntries),
		stopCh:         make(chan struct{}),
		connPools:      make(map[string]*ConnectionPool),
	}

	// Build TLS config if needed
	if config.TLS != nil {
		tlsConfig, err := config.TLS.BuildTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("build TLS config: %w", err)
		}
		proxy.tlsConfig = tlsConfig
	}

	// Initialize connection pools if enabled
	if config.ConnectionPool.Enabled {
		for _, b := range pool.All() {
			backend, _ := pool.Get(b.Config.Name)
			proxy.connPools[b.Config.Name] = NewConnectionPool(
				backend,
				config.ConnectionPool.MaxIdlePerBackend,
				config.ConnectionPool.IdleTimeout,
			)
		}
	}

	return proxy, nil
}

// OnConnect sets the callback for new connections.
func (p *L4Proxy) OnConnect(fn func(clientAddr, backendAddr string)) {
	p.onConnect = fn
}

// OnDisconnect sets the callback for closed connections.
func (p *L4Proxy) OnDisconnect(fn func(clientAddr, backendAddr string, duration time.Duration, bytesIn, bytesOut int64)) {
	p.onDisconnect = fn
}

// OnError sets the callback for connection errors.
func (p *L4Proxy) OnError(fn func(clientAddr string, err error)) {
	p.onError = fn
}

// Start starts the L4 proxy.
func (p *L4Proxy) Start() error {
	if !atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		return errors.New("proxy already running")
	}

	switch p.config.Protocol {
	case ProtocolTCP:
		return p.startTCP()
	case ProtocolUDP:
		return p.startUDP()
	default:
		atomic.StoreInt32(&p.running, 0)
		return fmt.Errorf("unsupported protocol for L4: %s", p.config.Protocol)
	}
}

// startTCP starts the TCP proxy.
func (p *L4Proxy) startTCP() error {
	var err error

	if p.tlsConfig != nil {
		// TLS listener
		p.tcpListener, err = tls.Listen("tcp", p.config.ListenAddress, p.tlsConfig)
	} else {
		// Plain TCP listener
		p.tcpListener, err = net.Listen("tcp", p.config.ListenAddress)
	}

	if err != nil {
		atomic.StoreInt32(&p.running, 0)
		return fmt.Errorf("listen: %w", err)
	}

	p.wg.Add(1)
	go p.acceptTCP()

	return nil
}

// acceptTCP accepts incoming TCP connections.
func (p *L4Proxy) acceptTCP() {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		conn, err := p.tcpListener.Accept()
		if err != nil {
			select {
			case <-p.stopCh:
				return
			default:
				// Log error and continue
				continue
			}
		}

		// Check connection limits
		if p.config.Limits.MaxConnections > 0 {
			if atomic.LoadInt64(&p.activeConns) >= int64(p.config.Limits.MaxConnections) {
				conn.Close()
				atomic.AddInt64(&p.droppedConns, 1)
				continue
			}
		}

		p.wg.Add(1)
		go p.handleTCP(conn)
	}
}

// handleTCP handles a single TCP connection.
func (p *L4Proxy) handleTCP(clientConn net.Conn) {
	defer p.wg.Done()
	defer clientConn.Close()

	startTime := time.Now()
	clientAddr := clientConn.RemoteAddr().String()

	atomic.AddInt64(&p.activeConns, 1)
	atomic.AddInt64(&p.totalConns, 1)
	defer atomic.AddInt64(&p.activeConns, -1)

	// Select backend
	backend, err := p.selectBackend(clientAddr)
	if err != nil {
		if p.onError != nil {
			p.onError(clientAddr, err)
		}
		return
	}

	// Get backend connection
	ctx, cancel := context.WithTimeout(context.Background(), p.config.Timeouts.Connect)
	defer cancel()

	backendConn, err := p.dialBackend(ctx, backend)
	if err != nil {
		if p.onError != nil {
			p.onError(clientAddr, fmt.Errorf("dial backend: %w", err))
		}
		p.health.RecordError(backend, err)
		return
	}

	backend.IncrementConnections()
	defer backend.DecrementConnections()

	if p.onConnect != nil {
		p.onConnect(clientAddr, backend.Config.Address)
	}

	// Store session affinity
	if p.config.SessionPersistence.Type == SessionPersistenceSourceIP {
		host, _, _ := net.SplitHostPort(clientAddr)
		p.sessionManager.Set(host, backend)
	}

	// Proxy data in both directions
	var bytesIn, bytesOut int64
	done := make(chan struct{}, 2)

	// Client -> Backend
	go func() {
		n, _ := p.copy(backendConn, clientConn, p.config.Timeouts.Write)
		atomic.AddInt64(&bytesIn, n)
		done <- struct{}{}
	}()

	// Backend -> Client
	go func() {
		n, _ := p.copy(clientConn, backendConn, p.config.Timeouts.Read)
		atomic.AddInt64(&bytesOut, n)
		done <- struct{}{}
	}()

	// Wait for either direction to complete
	<-done

	// Close connections to trigger other goroutine to finish
	clientConn.Close()
	backendConn.Close()

	// Wait for second direction
	<-done

	totalBytes := bytesIn + bytesOut
	atomic.AddInt64(&p.totalBytes, totalBytes)
	backend.AddBytes(totalBytes)

	if p.onDisconnect != nil {
		p.onDisconnect(clientAddr, backend.Config.Address, time.Since(startTime), bytesIn, bytesOut)
	}
}

// selectBackend selects a backend for a client.
func (p *L4Proxy) selectBackend(clientAddr string) (*Backend, error) {
	// Check session affinity
	if p.config.SessionPersistence.Type == SessionPersistenceSourceIP {
		host, _, _ := net.SplitHostPort(clientAddr)
		if backend, found := p.sessionManager.Get(host); found {
			return backend, nil
		}
	}

	// Use algorithm to select
	ctx := context.Background()
	return p.pool.Select(ctx, clientAddr)
}

// dialBackend connects to a backend.
func (p *L4Proxy) dialBackend(ctx context.Context, backend *Backend) (net.Conn, error) {
	// Try connection pool if enabled
	if p.config.ConnectionPool.Enabled {
		p.connPoolsMu.RLock()
		pool, ok := p.connPools[backend.Config.Name]
		p.connPoolsMu.RUnlock()

		if ok {
			conn, err := pool.Get(ctx)
			if err == nil {
				return conn, nil
			}
		}
	}

	// Dial new connection
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", backend.Config.Address)
	if err != nil {
		return nil, err
	}

	// Upgrade to TLS if configured
	if backend.tlsConfig != nil {
		tlsConn := tls.Client(conn, backend.tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		return tlsConn, nil
	}

	return conn, nil
}

// copy copies data from src to dst with timeout.
func (p *L4Proxy) copy(dst, src net.Conn, timeout time.Duration) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64

	for {
		select {
		case <-p.stopCh:
			return total, nil
		default:
		}

		// Set read deadline
		if timeout > 0 {
			src.SetReadDeadline(time.Now().Add(timeout))
		}

		n, err := src.Read(buf)
		if n > 0 {
			// Set write deadline
			if timeout > 0 {
				dst.SetWriteDeadline(time.Now().Add(timeout))
			}

			written, writeErr := dst.Write(buf[:n])
			total += int64(written)

			if writeErr != nil {
				return total, writeErr
			}
		}

		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}

// startUDP starts the UDP proxy.
func (p *L4Proxy) startUDP() error {
	addr, err := net.ResolveUDPAddr("udp", p.config.ListenAddress)
	if err != nil {
		atomic.StoreInt32(&p.running, 0)
		return fmt.Errorf("resolve address: %w", err)
	}

	p.udpConn, err = net.ListenUDP("udp", addr)
	if err != nil {
		atomic.StoreInt32(&p.running, 0)
		return fmt.Errorf("listen UDP: %w", err)
	}

	p.wg.Add(1)
	go p.handleUDP()

	return nil
}

// handleUDP handles UDP packets.
func (p *L4Proxy) handleUDP() {
	defer p.wg.Done()

	buf := make([]byte, 65535)

	// Map client addresses to backend connections
	clientBackends := make(map[string]*udpSession)
	var mu sync.Mutex

	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		// Set read deadline for clean shutdown
		p.udpConn.SetReadDeadline(time.Now().Add(time.Second))

		n, clientAddr, err := p.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		// Get or create session
		mu.Lock()
		session, ok := clientBackends[clientAddr.String()]
		if !ok || time.Since(session.lastActivity) > p.config.Timeouts.Idle {
			// Select backend
			backend, err := p.selectBackend(clientAddr.String())
			if err != nil {
				mu.Unlock()
				continue
			}

			// Create backend connection
			backendAddr, err := net.ResolveUDPAddr("udp", backend.Config.Address)
			if err != nil {
				mu.Unlock()
				continue
			}

			backendConn, err := net.DialUDP("udp", nil, backendAddr)
			if err != nil {
				mu.Unlock()
				continue
			}

			session = &udpSession{
				backend:      backend,
				backendConn:  backendConn,
				lastActivity: time.Now(),
			}
			clientBackends[clientAddr.String()] = session

			// Start goroutine to receive responses
			go func(sess *udpSession, cAddr *net.UDPAddr) {
				respBuf := make([]byte, 65535)
				for {
					sess.backendConn.SetReadDeadline(time.Now().Add(p.config.Timeouts.Idle))
					n, err := sess.backendConn.Read(respBuf)
					if err != nil {
						break
					}
					sess.lastActivity = time.Now()
					p.udpConn.WriteToUDP(respBuf[:n], cAddr)
					atomic.AddInt64(&p.totalBytes, int64(n))
				}
			}(session, clientAddr)
		}
		mu.Unlock()

		// Forward packet to backend
		session.lastActivity = time.Now()
		session.backendConn.Write(buf[:n])
		atomic.AddInt64(&p.totalBytes, int64(n))
		atomic.AddInt64(&p.totalConns, 1)
	}
}

type udpSession struct {
	backend      *Backend
	backendConn  *net.UDPConn
	lastActivity time.Time
}

// Stop stops the L4 proxy.
func (p *L4Proxy) Stop() error {
	if !atomic.CompareAndSwapInt32(&p.running, 1, 0) {
		return errors.New("proxy not running")
	}

	close(p.stopCh)

	if p.tcpListener != nil {
		p.tcpListener.Close()
	}

	if p.udpConn != nil {
		p.udpConn.Close()
	}

	// Wait for drain timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(p.config.DrainTimeout):
	}

	// Close connection pools
	p.connPoolsMu.Lock()
	for _, pool := range p.connPools {
		pool.Close()
	}
	p.connPoolsMu.Unlock()

	return nil
}

// Stats returns proxy statistics.
func (p *L4Proxy) Stats() L4ProxyStats {
	return L4ProxyStats{
		ActiveConnections: atomic.LoadInt64(&p.activeConns),
		TotalConnections:  atomic.LoadInt64(&p.totalConns),
		TotalBytes:        atomic.LoadInt64(&p.totalBytes),
		DroppedConnections: atomic.LoadInt64(&p.droppedConns),
	}
}

// L4ProxyStats holds L4 proxy statistics.
type L4ProxyStats struct {
	ActiveConnections  int64
	TotalConnections   int64
	TotalBytes         int64
	DroppedConnections int64
}

// HalfCloseConn wraps a net.Conn to support half-close (for TCP).
type HalfCloseConn struct {
	net.Conn
}

// CloseWrite closes the write side of the connection.
func (c *HalfCloseConn) CloseWrite() error {
	if tcpConn, ok := c.Conn.(*net.TCPConn); ok {
		return tcpConn.CloseWrite()
	}
	if tlsConn, ok := c.Conn.(*tls.Conn); ok {
		return tlsConn.CloseWrite()
	}
	return nil
}

// CloseRead closes the read side of the connection.
func (c *HalfCloseConn) CloseRead() error {
	if tcpConn, ok := c.Conn.(*net.TCPConn); ok {
		return tcpConn.CloseRead()
	}
	return nil
}

// ProxyProtocolHandler handles PROXY protocol.
type ProxyProtocolHandler struct {
	inner net.Listener
}

// NewProxyProtocolListener wraps a listener to handle PROXY protocol.
func NewProxyProtocolListener(inner net.Listener) *ProxyProtocolHandler {
	return &ProxyProtocolHandler{inner: inner}
}

// Accept accepts a connection and parses PROXY protocol header.
func (p *ProxyProtocolHandler) Accept() (net.Conn, error) {
	conn, err := p.inner.Accept()
	if err != nil {
		return nil, err
	}

	// Try to read PROXY protocol header
	proxyConn := &proxyProtocolConn{
		Conn: conn,
	}

	if err := proxyConn.parseHeader(); err != nil {
		// Not a PROXY protocol connection, that's OK
		proxyConn.realAddr = conn.RemoteAddr()
	}

	return proxyConn, nil
}

// Close closes the listener.
func (p *ProxyProtocolHandler) Close() error {
	return p.inner.Close()
}

// Addr returns the listener's network address.
func (p *ProxyProtocolHandler) Addr() net.Addr {
	return p.inner.Addr()
}

type proxyProtocolConn struct {
	net.Conn
	realAddr net.Addr
	peeked   []byte
}

func (c *proxyProtocolConn) parseHeader() error {
	// Read first line
	buf := make([]byte, 108) // Max PROXY protocol v1 header size
	n, err := c.Conn.Read(buf)
	if err != nil {
		return err
	}

	data := string(buf[:n])

	// Check for PROXY protocol v1
	if len(data) >= 6 && data[:6] == "PROXY " {
		// Parse v1 header
		// Format: PROXY TCP4/TCP6/UNKNOWN src_addr dst_addr src_port dst_port\r\n
		var protocol, srcAddr, dstAddr string
		var srcPort, dstPort int

		_, err := fmt.Sscanf(data, "PROXY %s %s %s %d %d\r\n",
			&protocol, &srcAddr, &dstAddr, &srcPort, &dstPort)
		if err != nil {
			c.peeked = buf[:n]
			return err
		}

		c.realAddr = &net.TCPAddr{
			IP:   net.ParseIP(srcAddr),
			Port: srcPort,
		}

		// Find end of header
		headerEnd := 0
		for i := 0; i < n-1; i++ {
			if buf[i] == '\r' && buf[i+1] == '\n' {
				headerEnd = i + 2
				break
			}
		}

		if headerEnd < n {
			c.peeked = buf[headerEnd:n]
		}

		return nil
	}

	// Not PROXY protocol, save data for later reads
	c.peeked = buf[:n]
	return errors.New("not PROXY protocol")
}

func (c *proxyProtocolConn) Read(b []byte) (int, error) {
	if len(c.peeked) > 0 {
		n := copy(b, c.peeked)
		c.peeked = c.peeked[n:]
		return n, nil
	}
	return c.Conn.Read(b)
}

func (c *proxyProtocolConn) RemoteAddr() net.Addr {
	if c.realAddr != nil {
		return c.realAddr
	}
	return c.Conn.RemoteAddr()
}

// TCPKeepaliveConfig configures TCP keepalive.
type TCPKeepaliveConfig struct {
	Enabled  bool
	Idle     time.Duration
	Interval time.Duration
	Count    int
}

// SetTCPKeepalive configures TCP keepalive on a connection.
func SetTCPKeepalive(conn net.Conn, config TCPKeepaliveConfig) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil // Not a TCP connection
	}

	if !config.Enabled {
		return tcpConn.SetKeepAlive(false)
	}

	if err := tcpConn.SetKeepAlive(true); err != nil {
		return err
	}

	if config.Idle > 0 {
		if err := tcpConn.SetKeepAlivePeriod(config.Idle); err != nil {
			return err
		}
	}

	return nil
}

// PortRangeListener listens on a range of ports.
type PortRangeListener struct {
	listeners []net.Listener
	acceptCh  chan net.Conn
	errCh     chan error
	stopCh    chan struct{}
}

// NewPortRangeListener creates a listener for a range of ports.
func NewPortRangeListener(host string, startPort, endPort int) (*PortRangeListener, error) {
	prl := &PortRangeListener{
		acceptCh: make(chan net.Conn, 100),
		errCh:    make(chan error, 1),
		stopCh:   make(chan struct{}),
	}

	for port := startPort; port <= endPort; port++ {
		addr := fmt.Sprintf("%s:%d", host, port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			// Close already opened listeners
			prl.Close()
			return nil, fmt.Errorf("listen on %s: %w", addr, err)
		}
		prl.listeners = append(prl.listeners, listener)

		go func(l net.Listener) {
			for {
				conn, err := l.Accept()
				if err != nil {
					select {
					case <-prl.stopCh:
						return
					default:
						continue
					}
				}
				select {
				case prl.acceptCh <- conn:
				case <-prl.stopCh:
					conn.Close()
					return
				}
			}
		}(listener)
	}

	return prl, nil
}

// Accept accepts a connection from any port.
func (prl *PortRangeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-prl.acceptCh:
		return conn, nil
	case err := <-prl.errCh:
		return nil, err
	case <-prl.stopCh:
		return nil, errors.New("listener closed")
	}
}

// Close closes all listeners.
func (prl *PortRangeListener) Close() error {
	close(prl.stopCh)
	for _, l := range prl.listeners {
		l.Close()
	}
	return nil
}

// Addr returns the first listener's address.
func (prl *PortRangeListener) Addr() net.Addr {
	if len(prl.listeners) > 0 {
		return prl.listeners[0].Addr()
	}
	return nil
}
