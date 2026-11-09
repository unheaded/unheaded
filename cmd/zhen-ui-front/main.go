// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// zhen-ui-front terminates TLS for the Zhen web UI and proxies to the Flask
// app's Unix socket (ADR-138). The listener is TLS 1.3 with hybrid ML-KEM
// key exchange only (pkg/tls Strict): a client that offers only classical
// groups fails the handshake. Python's ssl cannot refuse such a client, so
// the app no longer serves TLS and listens on the socket alone.
//
// The app treats every request on the socket as having arrived over this
// listener. That holds because the front has no plaintext mode, refuses to
// start without a certificate and key, and will not use a socket other local
// users can open. No header says "this was TLS": the front adds none, and it
// drops every forwarding header a client sends. The one header it sets is
// the TCP peer's address, which the app's login limiter keys on.
//
// Everything a client sends is hostile: methods are an allowlist, the header
// block and the body are capped, every phase has a deadline, connections are
// counted, and errors to the client say nothing about the host.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/cmdline"
	svchttp "unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
	pqtls "unheaded/pkg/tls"
)

const (
	serviceName = "zhen-ui-front"

	defaultSocket   = "/run/unheaded-zhen-ui/app.sock"
	defaultTimeout  = 8 * time.Minute // the app's slowest call waits 420 s on the model
	defaultMaxBody  = 4 << 20
	defaultMaxConns = 256

	minTimeout  = time.Second
	maxTimeout  = time.Hour
	maxMaxBody  = 64 << 20
	maxMaxConns = 65536
	// maxSocketPath keeps the path inside sockaddr_un's 108 bytes.
	maxSocketPath = 100

	maxHeaderBytes    = 32 << 10
	readHeaderTimeout = 5 * time.Second // also bounds the TLS handshake
	readTimeout       = 60 * time.Second
	idleTimeout       = 60 * time.Second
	writeSlack        = 30 * time.Second
	dialTimeout       = 5 * time.Second
	probeTimeout      = time.Second
	shutdownGrace     = 10 * time.Second
	opsRetry          = 30 * time.Second

	// clientAddrHeader carries the TCP peer's address to the app. A client's
	// own copy never passes (untrustedHeader).
	clientAddrHeader = "X-Unheaded-Client-Addr"

	// upstreamHost fills the URL the transport needs; the dialer ignores it
	// and the app never sees it (Host is the client's).
	upstreamHost = "zhen-ui.invalid"

	// errorLogBurst is how many lines a minute net/http's own complaints
	// (failed handshakes, mostly) and upstream failures may each log.
	errorLogBurst = 10
)

// Values of unheaded_zhen_ui_front_tls_client_hellos_total{kex}.
const (
	kexHybrid    = "hybrid"
	kexClassical = "classical"
)

// Values of unheaded_zhen_ui_front_upstream_errors_total{reason}.
const (
	reasonSocket   = "socket"         // missing, not a socket, or open to other users
	reasonDial     = "dial"           // nobody listening
	reasonTimeout  = "timeout"        // the app did not answer in --timeout
	reasonBody     = "body_too_large" // the body passed --max-body mid-stream
	reasonCanceled = "client_gone"    // the client left first
	reasonOther    = "other"
)

var upstreamReasons = []string{reasonSocket, reasonDial, reasonTimeout, reasonBody, reasonCanceled, reasonOther}

// allowedMethods is what the app's routes use, plus the rest of the ordinary
// set. CONNECT, TRACE and extension methods never reach it.
var allowedMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

var frontSpec = cmdline.Spec{
	Name:    serviceName,
	Purpose: "Terminate hybrid-only TLS 1.3 for the Zhen web UI and proxy to its Unix socket (ADR-138).",
	Examples: []string{
		"zhen-ui-front --listen 127.0.0.1:20103 --socket /run/unheaded-zhen-ui/app.sock",
	},
	Details: "The certificate and key are the files UNHEADED_TLS_CERT and UNHEADED_TLS_KEY_FILE name; without both the\n" +
		"front refuses to start. There is no plaintext mode, and UNHEADED_TLS_PROFILE is not read: key exchange is\n" +
		"hybrid ML-KEM only, so a client without it fails the handshake. The socket must not be open to other users.\n" +
		"Forwarding headers from clients are dropped; the app gets the TCP peer in X-Unheaded-Client-Addr.",
}

type config struct {
	listens  []netip.AddrPort
	metrics  netip.AddrPort
	socket   string
	timeout  time.Duration
	maxBody  int64
	maxConns int
}

// repeated is a flag that may be given more than once.
type repeated []string

func (r *repeated) String() string { return strings.Join(*r, ",") }

func (r *repeated) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func parseFlags(args []string, stdout, stderr io.Writer) (config, error) {
	var (
		listens repeated
		metric  string
		c       config
	)
	defaultListen := netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), ports.ZhenWebUI).String()
	defaultMetrics := netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), ports.ZhenUIFrontMetrics).String()
	fs := flag.NewFlagSet(serviceName, flag.ContinueOnError)
	fs.Var(&listens, "listen", "IP:PORT to serve TLS on, repeatable; no wildcard address (default "+defaultListen+")")
	fs.StringVar(&metric, "metrics", defaultMetrics, "loopback IP:PORT for /metrics, /health and /ready, plain HTTP")
	fs.StringVar(&c.socket, "socket", defaultSocket, "the app's Unix socket")
	fs.DurationVar(&c.timeout, "timeout", defaultTimeout, "time the app has to start answering a request")
	fs.Int64Var(&c.maxBody, "max-body", defaultMaxBody, "request body cap in bytes")
	fs.IntVar(&c.maxConns, "max-conns", defaultMaxConns, "client connections at once, across every listener")
	if err := cmdline.ParseFlags(fs, frontSpec, args, stdout, stderr); err != nil {
		return c, err
	}
	if len(listens) == 0 {
		listens = repeated{defaultListen}
	}
	for _, l := range listens {
		a, err := parseBindAddr(l)
		if err != nil {
			return c, fmt.Errorf("--listen %s: %w", l, err)
		}
		if slices.Contains(c.listens, a) {
			return c, fmt.Errorf("--listen %s is given twice", l)
		}
		c.listens = append(c.listens, a)
	}
	m, err := parseBindAddr(metric)
	if err != nil {
		return c, fmt.Errorf("--metrics %s: %w", metric, err)
	}
	if !m.Addr().IsLoopback() {
		return c, fmt.Errorf("--metrics %s: plain HTTP stays on loopback", metric)
	}
	if m.Port() != 0 && slices.Contains(c.listens, m) {
		return c, fmt.Errorf("--metrics %s is also a --listen address", metric)
	}
	c.metrics = m
	// A path with "..", "." or doubled slashes names somewhere other than it
	// reads; refuse it instead of quietly using the cleaned one.
	given := c.socket
	c.socket = filepath.Clean(given)
	switch {
	case !filepath.IsAbs(given):
		return c, fmt.Errorf("--socket %q must be an absolute path", given)
	case c.socket != given:
		return c, fmt.Errorf("--socket %q is not a clean path", given)
	case len(c.socket) > maxSocketPath:
		return c, fmt.Errorf("--socket path is too long (%d bytes; a Unix socket address holds about %d)", len(c.socket), maxSocketPath)
	case c.timeout < minTimeout || c.timeout > maxTimeout:
		return c, fmt.Errorf("--timeout must be %s..%s", minTimeout, maxTimeout)
	case c.maxBody < 1 || c.maxBody > maxMaxBody:
		return c, fmt.Errorf("--max-body must be 1..%d", maxMaxBody)
	case c.maxConns < 1 || c.maxConns > maxMaxConns:
		return c, fmt.Errorf("--max-conns must be 1..%d", maxMaxConns)
	}
	return c, nil
}

// parseBindAddr reads IP:PORT. A literal address only, so nothing resolves
// names at start, and never a wildcard: which interfaces serve the UI is a
// decision.
func parseBindAddr(s string) (netip.AddrPort, error) {
	addr, err := netip.ParseAddrPort(s)
	if err != nil {
		return netip.AddrPort{}, err
	}
	if addr.Addr().IsUnspecified() {
		return netip.AddrPort{}, errors.New("a wildcard address is refused; name the interface's address")
	}
	return addr, nil
}

// loadTLS builds the listener's TLS config from the files the environment
// names. It reads no switch that could turn TLS off or widen key exchange:
// UNHEADED_TLS_ENABLED, UNHEADED_TLS_PROFILE and the mTLS variables other
// services honour do nothing here. No client certificate is asked for; the
// client is a browser and the app's login is the identity.
func loadTLS(getenv func(string) string) (*tls.Config, error) {
	certFile := strings.TrimSpace(getenv(pqtls.EnvCert))
	keyFile := strings.TrimSpace(getenv(pqtls.EnvKeyFile))
	switch {
	case certFile == "":
		return nil, fmt.Errorf("%s is not set: the front serves TLS only and has no plaintext mode", pqtls.EnvCert)
	case keyFile == "":
		return nil, fmt.Errorf("%s is not set: the front serves TLS only and has no plaintext mode", pqtls.EnvKeyFile)
	case !filepath.IsAbs(certFile) || !filepath.IsAbs(keyFile):
		return nil, fmt.Errorf("%s and %s must be absolute paths", pqtls.EnvCert, pqtls.EnvKeyFile)
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("certificate (%s, %s): %w", pqtls.EnvCert, pqtls.EnvKeyFile, err)
	}
	return pqtls.ApplyStrict(&tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"http/1.1"},
	}), nil
}

type frontMetrics struct {
	reg            *metrics.Registry
	hellos         *metrics.CounterVec
	upstreamErrors *metrics.CounterVec
	connections    *metrics.Gauge
}

func newMetrics() *frontMetrics {
	m := &frontMetrics{
		reg:            metrics.NewRegistry(),
		hellos:         metrics.NewCounterVec("unheaded_zhen_ui_front_tls_client_hellos_total", "TLS ClientHellos by the key exchange offered. A classical one is refused.", nil, []string{"kex"}),
		upstreamErrors: metrics.NewCounterVec("unheaded_zhen_ui_front_upstream_errors_total", "Requests the app's socket did not answer, by reason.", nil, []string{"reason"}),
		connections:    metrics.NewGauge("unheaded_zhen_ui_front_connections", "Client connections open now.", nil),
	}
	m.reg.MustRegister(m.hellos)
	m.reg.MustRegister(m.upstreamErrors)
	m.reg.MustRegister(m.connections)
	for _, k := range []string{kexHybrid, kexClassical} {
		m.hellos.WithLabelValues(k).Add(0)
	}
	for _, r := range upstreamReasons {
		m.upstreamErrors.WithLabelValues(r).Add(0)
	}
	return m
}

type front struct {
	cfg       config
	log       zerolog.Logger
	quiet     zerolog.Logger // burst-limited: one line per failure would follow a flood
	m         *frontMetrics
	transport *http.Transport
	slots     chan struct{}
}

func newFront(cfg config, log zerolog.Logger) *front {
	f := &front{
		cfg:   cfg,
		log:   log,
		quiet: log.Sample(&zerolog.BurstSampler{Burst: errorLogBurst, Period: time.Minute}),
		m:     newMetrics(),
		slots: make(chan struct{}, cfg.maxConns),
	}
	f.transport = &http.Transport{
		Proxy:                 nil, // never the environment's proxy
		DialContext:           f.dialApp,
		DisableCompression:    true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   16,
		MaxConnsPerHost:       cfg.maxConns,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: cfg.timeout,
		ExpectContinueTimeout: time.Second,
	}
	return f
}

// errSocket marks a socket the front will not use.
var errSocket = errors.New("app socket refused")

// checkSocket refuses a path that is not a socket, or one that other local
// users can open: such a socket would let them reach the app without TLS and
// name any client address.
func (f *front) checkSocket() error {
	// Looked up as one name inside its opened directory, never by walking
	// the path again.
	dir, err := os.OpenRoot(filepath.Dir(f.cfg.socket))
	if err != nil {
		return fmt.Errorf("%w: %w", errSocket, err)
	}
	defer func() { _ = dir.Close() }()
	fi, err := dir.Lstat(filepath.Base(f.cfg.socket))
	if err != nil {
		return fmt.Errorf("%w: %w", errSocket, err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%w: %s is not a socket", errSocket, f.cfg.socket)
	}
	if perm := fi.Mode().Perm(); perm&0o007 != 0 {
		return fmt.Errorf("%w: %s is mode %04o, open to other users", errSocket, f.cfg.socket, perm)
	}
	return nil
}

func (f *front) dialApp(ctx context.Context, _, _ string) (net.Conn, error) {
	if err := f.checkSocket(); err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: dialTimeout}
	return d.DialContext(ctx, "unix", f.cfg.socket)
}

// untrustedHeader reports whether a request header is one a client must not
// be able to hand the app: anything that states where the request came from,
// how it arrived, or what it was before a proxy rewrote it; the front's own
// X-Unheaded- names; and Upgrade, which would turn the proxy into a pipe.
// Underscores count as hyphens, since WSGI servers fold the two together.
func untrustedHeader(name string) bool {
	n := strings.ReplaceAll(strings.ToLower(name), "_", "-")
	switch n {
	case "forwarded", "x-real-ip", "x-client-ip", "x-cluster-client-ip", "true-client-ip",
		"x-https", "x-scheme", "x-url-scheme", "front-end-https",
		"x-original-url", "x-rewrite-url", "x-original-host", "x-host", "upgrade":
		return true
	}
	return strings.HasPrefix(n, "x-forwarded") || strings.HasPrefix(n, "x-unheaded-")
}

// clientAddr is the address in a TCP peer's "IP:PORT", without zone or
// IPv4-mapped form, or "" when it is not one.
func clientAddr(remote string) string {
	ap, err := netip.ParseAddrPort(remote)
	if err != nil {
		return ""
	}
	return ap.Addr().Unmap().WithZone("").String()
}

// bodyState records that a request's body passed the cap while the proxy
// was sending it, so the error handler can say 413.
type bodyState struct{ tooLarge atomic.Bool }

type bodyStateKey struct{}

type cappedBody struct {
	io.ReadCloser
	state *bodyState
}

func (b cappedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		b.state.tooLarge.Store(true)
	}
	return n, err
}

// handler is what the TLS listener serves: checks, then the proxy.
func (f *front) handler() http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = upstreamHost
			// The app compares Host with Origin (CSRF) and with its allowlist.
			pr.Out.Host = pr.In.Host
			for name := range pr.Out.Header {
				if untrustedHeader(name) {
					delete(pr.Out.Header, name)
				}
			}
			if addr := clientAddr(pr.In.RemoteAddr); addr != "" {
				pr.Out.Header[clientAddrHeader] = []string{addr}
			}
		},
		Transport: f.transport,
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Server") // the app's server and Python versions
			return nil
		},
		ErrorHandler: f.upstreamError,
		ErrorLog:     stdlog.New(newErrorLogWriter(f.log), "", 0),
	}
	checked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(allowedMethods, r.Method) {
			w.Header().Set("Allow", strings.Join(allowedMethods, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.ContentLength > f.cfg.maxBody {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		// Before the proxy looks: it would honour Upgrade.
		for name := range r.Header {
			if untrustedHeader(name) {
				delete(r.Header, name)
			}
		}
		state := &bodyState{}
		r.Body = cappedBody{ReadCloser: http.MaxBytesReader(w, r.Body, f.cfg.maxBody), state: state}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), bodyStateKey{}, state)))
	})
	// The mux is only the label source: one pattern, so one path series.
	mux := http.NewServeMux()
	mux.Handle("/", checked)
	return svchttp.NewServiceMetrics(serviceName).Instrument(mux, checked)
}

// upstreamError answers for a request the app did not. The client learns the
// class of failure and nothing about the host.
func (f *front) upstreamError(w http.ResponseWriter, r *http.Request, err error) {
	reason, status, text := reasonOther, http.StatusBadGateway, "bad gateway"
	var netErr net.Error
	state, _ := r.Context().Value(bodyStateKey{}).(*bodyState)
	switch {
	case state != nil && state.tooLarge.Load():
		reason, status, text = reasonBody, http.StatusRequestEntityTooLarge, "request body too large"
	case errors.Is(err, errSocket):
		reason = reasonSocket
	case errors.Is(err, context.Canceled):
		reason = reasonCanceled
	case errors.As(err, &netErr) && netErr.Timeout(), errors.Is(err, context.DeadlineExceeded):
		reason, status, text = reasonTimeout, http.StatusGatewayTimeout, "gateway timeout"
	default:
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			reason = reasonDial
		}
	}
	f.m.upstreamErrors.WithLabelValues(reason).Inc()
	f.quiet.Warn().Err(err).Str("reason", reason).Int("status", status).Msg("the app did not answer")
	http.Error(w, text, status)
}

// countHello records what each ClientHello offers, so a refused classical
// client shows on a graph instead of only as a handshake error.
func (f *front) countHello(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	kex := kexClassical
	if slices.ContainsFunc(hello.SupportedCurves, pqtls.IsHybrid) {
		kex = kexHybrid
	}
	f.m.hellos.WithLabelValues(kex).Inc()
	return nil, nil
}

// server is the TLS listener's http.Server: HTTP/1.1 only, a deadline on
// every phase.
func (f *front) server(tlsCfg *tls.Config) *http.Server {
	cfg := tlsCfg.Clone()
	cfg.GetConfigForClient = f.countHello
	h1 := new(http.Protocols)
	h1.SetHTTP1(true)
	return &http.Server{
		Handler:           f.handler(),
		TLSConfig:         cfg,
		Protocols:         h1,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      f.cfg.timeout + writeSlack,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          stdlog.New(newErrorLogWriter(f.log), "", 0),
	}
}

// limit caps the connections served at once at --max-conns, across every
// listener. Past it, clients wait in the kernel's queue.
func (f *front) limit(ln net.Listener) net.Listener {
	return &limitListener{Listener: ln, front: f, done: make(chan struct{})}
}

type limitListener struct {
	net.Listener
	front *front
	done  chan struct{}
	once  sync.Once
}

func (l *limitListener) Accept() (net.Conn, error) {
	select {
	case l.front.slots <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}
	c, err := l.Listener.Accept()
	if err != nil {
		<-l.front.slots
		return nil, err
	}
	l.front.m.connections.Inc()
	return &limitConn{Conn: c, release: sync.OnceFunc(func() {
		<-l.front.slots
		l.front.m.connections.Dec()
	})}, nil
}

func (l *limitListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type limitConn struct {
	net.Conn
	release func()
}

func (c *limitConn) Close() error {
	err := c.Conn.Close()
	c.release()
	return err
}

// errorLogWriter turns net/http's log lines into structured ones, at most
// errorLogBurst a minute: a scanner must not be able to fill the journal.
type errorLogWriter struct{ log zerolog.Logger }

func newErrorLogWriter(log zerolog.Logger) io.Writer {
	return errorLogWriter{log: log.Sample(&zerolog.BurstSampler{Burst: errorLogBurst, Period: time.Minute})}
}

func (w errorLogWriter) Write(p []byte) (int, error) {
	w.log.Warn().Str("source", "net/http").Msg(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// ready reports whether the app's socket is one the front will use and
// something accepts on it. It sends the app nothing.
func (f *front) ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	conn, err := f.dialApp(ctx, "", "")
	if err != nil {
		return err
	}
	return conn.Close()
}

// opsHandler is the loopback listener: health, readiness and metrics. It
// proxies nothing.
func (f *front) opsHandler() http.Handler {
	answer := func(w http.ResponseWriter, status int, state string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "{\"status\":%q}\n", state)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.HandlerFor(f.m.reg, metrics.DefaultRegistry))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		answer(w, http.StatusOK, "healthy")
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if err := f.ready(r.Context()); err != nil {
			answer(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		answer(w, http.StatusOK, "ready")
	})
	return mux
}

// serveOps keeps the loopback listener up. A failure here is logged and
// retried, never fatal: the UI matters more than the graph of it.
func (f *front) serveOps(ctx context.Context) {
	srv := &http.Server{
		Handler:           f.opsHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	stop := context.AfterFunc(ctx, func() { _ = srv.Close() })
	defer stop()
	for {
		l, err := new(net.ListenConfig).Listen(ctx, "tcp", f.cfg.metrics.String())
		if err == nil {
			err = srv.Serve(l)
		}
		if ctx.Err() != nil || errors.Is(err, http.ErrServerClosed) {
			return
		}
		f.log.Error().Err(err).Stringer("addr", f.cfg.metrics).Msg("metrics listener down; retrying")
		select {
		case <-ctx.Done():
			return
		case <-time.After(opsRetry):
		}
	}
}

func run(ctx context.Context, cfg config, log zerolog.Logger) error {
	tlsCfg, err := loadTLS(os.Getenv)
	if err != nil {
		return err
	}
	f := newFront(cfg, log)
	var lns []net.Listener
	for _, a := range cfg.listens {
		ln, err := new(net.ListenConfig).Listen(ctx, "tcp", a.String())
		if err != nil {
			for _, open := range lns {
				_ = open.Close()
			}
			return fmt.Errorf("listen %s: %w", a, err)
		}
		lns = append(lns, f.limit(ln))
	}
	if err := f.checkSocket(); err != nil {
		log.Warn().Err(err).Msg("the app's socket is not usable yet; requests get 502 until it is")
	}
	srv := f.server(tlsCfg)
	go f.serveOps(ctx)
	failed := make(chan error, len(lns))
	for _, ln := range lns {
		go func() { failed <- srv.ServeTLS(ln, "", "") }()
	}
	log.Info().Str("version", frontSpec.VersionString()).Strs("listen", addrStrings(cfg.listens)).
		Str("socket", cfg.socket).Stringer("metrics", cfg.metrics).
		Msg("serving TLS 1.3, hybrid key exchange only")

	select {
	case <-ctx.Done():
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			_ = srv.Close()
			return err
		}
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(stopCtx); err != nil {
		_ = srv.Close()
	}
	f.transport.CloseIdleConnections()
	return nil
}

func addrStrings(addrs []netip.AddrPort) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.String())
	}
	return out
}

func main() {
	cfg, err := parseFlags(os.Args[1:], os.Stdout, os.Stderr)
	var exit *cmdline.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.Code)
	}
	if err != nil {
		os.Exit(cmdline.ReportUsageError(os.Stderr, frontSpec, "%v", err))
	}
	log := zerolog.New(os.Stderr).With().Timestamp().Str("service", serviceName).Logger()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, log); err != nil {
		log.Error().Err(err).Msg("front stopped")
		os.Exit(1)
	}
}
