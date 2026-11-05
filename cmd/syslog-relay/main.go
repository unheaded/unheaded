// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// syslog-relay receives RFC 5424 and RFC 3164 syslog over UDP and TCP and
// writes each message to the local journal through the native protocol, with
// the sender recorded, so Vector's journald source ships it (ADR-124).
//
// Everything a message says is the sender's claim and is treated as hostile;
// see pkg/syslog. What the relay adds itself (REMOTE_ADDR, UNHEADED_SOURCE) comes
// from the socket and sources.yaml. Senders not in sources.yaml are counted and
// dropped.
//
// Nothing here logs per message: the relay's own output lands in the journal,
// so a line per packet would turn a flood into a bigger flood. Per-message
// outcomes are counters.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/journald"
	"unheaded/pkg/logsources"
	"unheaded/pkg/metrics"
	"unheaded/pkg/syslog"
)

const (
	serviceName = "syslog-relay"

	defaultSources  = "config/logging/sources.yaml"
	defaultRate     = 200
	defaultBurst    = 1000
	defaultMaxConns = 256
	defaultIdle     = 5 * time.Minute
	defaultMetrics  = "127.0.0.1:9599"

	// RFC 5424 §6.1 asks a receiver to take at least 2048. The top keeps the
	// largest entry well inside one journal datagram.
	minMaxMsg = 2048
	maxMaxMsg = 64 << 10

	// sourceUnknown is UNHEADED_SOURCE for a sender or tag with no row.
	sourceUnknown = "unknown"

	udpReadBuffer = 64 << 10 // the largest datagram UDP can carry
	metricsRetry  = 30 * time.Second
	acceptBackoff = 100 * time.Millisecond

	// severityFloor is the most urgent PRIORITY a relayed line carries.
	// journald writes emerg entries to every login tty (ForwardToWall,
	// MaxLevelWall=emerg by default), and a sender must not be able to do
	// that. A host that raises MaxLevelWall raises this too.
	severityFloor = 1
)

// Drop reasons, the values of syslog_relay_dropped_total{reason}.
const (
	dropRate        = "rate"
	dropEmpty       = "empty"
	dropJournal     = "journal"
	dropConns       = "conns"
	dropSenderConns = "sender_conns"
	dropFrameLarge  = "frame_too_large"
	dropBadFrame    = "bad_frame"
	dropShortFrame  = "short_frame"
)

var dropReasons = []string{dropRate, dropEmpty, dropJournal, dropConns, dropSenderConns, dropFrameLarge, dropBadFrame, dropShortFrame}

var relaySpec = cmdline.Spec{
	Name:    serviceName,
	Purpose: "Receive syslog over UDP and TCP and write it to the local journal (ADR-124).",
	Examples: []string{
		"syslog-relay --listen udp://10.10.20.1:514 --listen tcp://10.10.20.1:514 --sources /etc/unheaded/log-sources.yaml",
	},
	Details: "Senders are the kind: syslog-device rows of the sources file that are not excluded; a match is <ip> or\n" +
		"<ip> <tag>. Others are counted and dropped. SIGHUP reloads the sources file; a bad file keeps the old list.",
}

type listenAddr struct {
	network string // udp or tcp
	addr    netip.AddrPort
}

type config struct {
	listens        []listenAddr
	sources        string
	rate           int
	burst          int
	maxMsg         int
	maxConns       int
	maxConnsSender int
	idle           time.Duration
	metrics        []string
	acceptUnknown  bool
	journalSocket  string
	metricsRetry   time.Duration
}

// frameCap is the largest TCP frame kept. Twice the message cap, so a line at
// the cap plus a header is truncated and flagged. Closing on it would make
// rsyslog's omfwd resend that one message forever (ADR-124).
func (c config) frameCap() int { return 2 * c.maxMsg }

// senderCap is how many TCP connections one sender may hold. Left at 0 it is a
// sixteenth of --max-conns, so a compromised sender cannot fill the table.
func (c config) senderCap() int {
	if c.maxConnsSender > 0 {
		return c.maxConnsSender
	}
	return max(1, c.maxConns/16)
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
		metric  repeated
	)
	c := config{metricsRetry: metricsRetry}
	fs := flag.NewFlagSet(serviceName, flag.ContinueOnError)
	fs.Var(&listens, "listen", "udp://IP:PORT or tcp://IP:PORT, repeatable; required, no wildcard address")
	fs.StringVar(&c.sources, "sources", defaultSources, "sources file (config/logging/sources.yaml)")
	fs.IntVar(&c.rate, "rate", defaultRate, "messages per second per sender; a row's rate: lowers it")
	fs.IntVar(&c.burst, "burst", defaultBurst, "token bucket size per sender; a row's burst: lowers it")
	fs.IntVar(&c.maxMsg, "max-msg", syslog.DefaultMaxMessage, "message cap in bytes; longer is truncated and flagged")
	fs.IntVar(&c.maxConns, "max-conns", defaultMaxConns, "TCP connections at once, across every listener")
	fs.IntVar(&c.maxConnsSender, "max-conns-per-sender", 0, "TCP connections one sender may hold, below --max-conns; 0 means a sixteenth of it")
	fs.DurationVar(&c.idle, "idle-timeout", defaultIdle, "time a TCP sender has to complete a frame")
	fs.Var(&metric, "metrics", "address for /metrics, /health and /ready, repeatable (default "+defaultMetrics+")")
	fs.BoolVar(&c.acceptUnknown, "accept-unknown", false, "ship unknown senders as source unknown; for the inventory phase only")
	fs.StringVar(&c.journalSocket, "journal-socket", journald.DefaultSocket, "the journal's native socket")
	if err := cmdline.ParseFlags(fs, relaySpec, args, stdout, stderr); err != nil {
		return c, err
	}
	if len(listens) == 0 {
		return c, errors.New("at least one --listen is required")
	}
	for _, l := range listens {
		a, err := parseListen(l)
		if err != nil {
			return c, err
		}
		c.listens = append(c.listens, a)
	}
	c.metrics = metric
	if len(c.metrics) == 0 {
		c.metrics = []string{defaultMetrics}
	}
	for _, m := range c.metrics {
		if _, err := parseBindAddr(m); err != nil {
			return c, fmt.Errorf("--metrics %s: %w", m, err)
		}
	}
	switch {
	case c.rate < 1:
		return c, errors.New("--rate must be at least 1")
	case c.burst < 1:
		return c, errors.New("--burst must be at least 1")
	case c.maxMsg < minMaxMsg || c.maxMsg > maxMaxMsg:
		return c, fmt.Errorf("--max-msg must be %d..%d", minMaxMsg, maxMaxMsg)
	case c.maxConns < 1:
		return c, errors.New("--max-conns must be at least 1")
	case c.maxConnsSender < 0:
		return c, errors.New("--max-conns-per-sender must not be negative")
	case c.maxConnsSender >= c.maxConns:
		return c, errors.New("--max-conns-per-sender must be below --max-conns, or one sender could hold every slot")
	case c.idle <= 0:
		return c, errors.New("--idle-timeout must be positive")
	}
	return c, nil
}

func parseListen(s string) (listenAddr, error) {
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok || (scheme != "udp" && scheme != "tcp") {
		return listenAddr{}, fmt.Errorf("--listen %s: want udp://IP:PORT or tcp://IP:PORT", s)
	}
	addr, err := parseBindAddr(rest)
	if err != nil {
		return listenAddr{}, fmt.Errorf("--listen %s: %w", s, err)
	}
	return listenAddr{network: scheme, addr: addr}, nil
}

// parseBindAddr reads IP:PORT. A literal address only, so nothing resolves
// names at start, and never a wildcard: which interfaces hear this is a
// decision (ADR-124).
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

// bucket is a token bucket.
type bucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newBucket(rate, burst int, now time.Time) *bucket {
	return &bucket{rate: float64(rate), burst: float64(burst), tokens: float64(burst), last: now}
}

// allow takes one token if there is one.
func (b *bucket) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if elapsed := now.Sub(b.last); elapsed > 0 {
		b.tokens = min(b.burst, b.tokens+elapsed.Seconds()*b.rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sender is one address on the allowlist.
type sender struct {
	id     string            // the row with no tag, if any
	tags   map[string]string // tag to row id
	bucket *bucket
}

// source names the row for a message. A sender can only reach rows on its own
// address, so a claimed tag never impersonates another host.
func (s *sender) source(tag string) string {
	if id, ok := s.tags[tag]; ok {
		return id
	}
	if s.id != "" {
		return s.id
	}
	return sourceUnknown
}

// allowlist is immutable; a reload swaps in a new one. State is bounded by its
// rows: unknown senders share one bucket, so spoofed addresses allocate nothing.
type allowlist struct {
	senders map[netip.Addr]*sender
	unknown *bucket
}

func buildAllowlist(rows []logsources.Sender, rate, burst int, now time.Time) *allowlist {
	limits := map[netip.Addr][2]int{}
	for _, row := range rows {
		l, seen := limits[row.Addr]
		if !seen {
			l = [2]int{rate, burst}
		}
		if row.Rate > 0 {
			l[0] = min(l[0], row.Rate)
		}
		if row.Burst > 0 {
			l[1] = min(l[1], row.Burst)
		}
		limits[row.Addr] = l
	}
	a := &allowlist{senders: make(map[netip.Addr]*sender, len(limits)), unknown: newBucket(rate, burst, now)}
	for _, row := range rows {
		s := a.senders[row.Addr]
		if s == nil {
			l := limits[row.Addr]
			s = &sender{tags: map[string]string{}, bucket: newBucket(l[0], l[1], now)}
			a.senders[row.Addr] = s
		}
		if row.Tag == "" {
			s.id = row.ID
		} else {
			s.tags[row.Tag] = row.ID
		}
	}
	return a
}

type relayMetrics struct {
	reg            *metrics.Registry
	received       *metrics.CounterVec
	dropped        *metrics.CounterVec
	unknownSender  *metrics.Counter
	malformed      *metrics.Counter
	truncated      *metrics.Counter
	reloadFailures *metrics.Counter
	lastSeen       *metrics.GaugeVec
	connections    *metrics.Gauge
}

func newRelayMetrics() *relayMetrics {
	m := &relayMetrics{
		reg:            metrics.NewRegistry(),
		received:       metrics.NewCounterVec("syslog_relay_received_total", "Messages read from the network, before any check.", nil, []string{"transport"}),
		dropped:        metrics.NewCounterVec("syslog_relay_dropped_total", "Messages or connections dropped, by reason. Unknown senders are counted apart.", nil, []string{"reason"}),
		unknownSender:  metrics.NewCounter("syslog_relay_unknown_sender_total", "Messages and TCP connections from senders not in sources.yaml.", nil),
		malformed:      metrics.NewCounter("syslog_relay_malformed_total", "Messages whose header would not parse, shipped as text.", nil),
		truncated:      metrics.NewCounter("syslog_relay_truncated_total", "Messages cut to fit and flagged UNHEADED_TRUNCATED.", nil),
		reloadFailures: metrics.NewCounter("syslog_relay_reload_failures_total", "SIGHUP reloads refused; the previous allowlist stayed.", nil),
		lastSeen:       metrics.NewGaugeVec("syslog_relay_source_last_seen_timestamp_seconds", "Unix time of the last message written for a source.", nil, []string{"source"}),
		connections:    metrics.NewGauge("syslog_relay_tcp_connections", "TCP connections open now.", nil),
	}
	m.reg.MustRegister(m.received)
	m.reg.MustRegister(m.dropped)
	m.reg.MustRegister(m.unknownSender)
	m.reg.MustRegister(m.malformed)
	m.reg.MustRegister(m.truncated)
	m.reg.MustRegister(m.reloadFailures)
	m.reg.MustRegister(m.lastSeen)
	m.reg.MustRegister(m.connections)
	// increase() cannot see a series' first sample, so every reason exists
	// from the start or the first drop would never alert.
	for _, reason := range dropReasons {
		m.dropped.WithLabelValues(reason)
	}
	m.received.WithLabelValues("udp")
	m.received.WithLabelValues("tcp")
	return m
}

type relay struct {
	cfg         config
	log         zerolog.Logger
	journal     *journald.Writer
	metrics     *relayMetrics
	list        atomic.Pointer[allowlist]
	slots       chan struct{}
	perSenderMu sync.Mutex
	perSender   map[netip.Addr]int

	udp []*net.UDPConn
	tcp []*net.TCPListener
}

func newRelay(cfg config, log zerolog.Logger) (*relay, error) {
	r := &relay{
		cfg:       cfg,
		log:       log,
		journal:   journald.New(cfg.journalSocket),
		metrics:   newRelayMetrics(),
		slots:     make(chan struct{}, cfg.maxConns),
		perSender: make(map[netip.Addr]int),
	}
	list, err := r.loadAllowlist()
	if err != nil {
		return nil, err
	}
	r.list.Store(list)
	return r, nil
}

func (r *relay) loadAllowlist() (*allowlist, error) {
	sources, err := logsources.Load(r.cfg.sources)
	if err != nil {
		return nil, err
	}
	rows, err := logsources.SyslogSenders(sources)
	if err != nil {
		return nil, err
	}
	return buildAllowlist(rows, r.cfg.rate, r.cfg.burst, time.Now()), nil
}

// reload swaps in the sources file. A file that fails keeps the list we have:
// a typo must not turn the relay into a black hole, or into an open door.
func (r *relay) reload() {
	list, err := r.loadAllowlist()
	if err != nil {
		r.metrics.reloadFailures.Inc()
		r.log.Error().Err(err).Msg("reload refused; keeping the previous allowlist")
		return
	}
	r.list.Store(list)
	r.log.Info().Int("senders", len(list.senders)).Msg("sources reloaded")
}

// listen binds everything, so a bad address fails before anything is served.
func (r *relay) listen() error {
	for _, l := range r.cfg.listens {
		switch l.network {
		case "udp":
			c, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(l.addr))
			if err != nil {
				return err
			}
			r.udp = append(r.udp, c)
		case "tcp":
			c, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(l.addr))
			if err != nil {
				return err
			}
			r.tcp = append(r.tcp, c)
		}
	}
	return nil
}

// serve runs until ctx ends and every connection has closed.
func (r *relay) serve(ctx context.Context) {
	var wg sync.WaitGroup
	for _, c := range r.udp {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.serveUDP(ctx, c)
		}()
	}
	for _, l := range r.tcp {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.serveTCP(ctx, l, &wg)
		}()
	}
	for _, addr := range r.cfg.metrics {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.serveMetrics(ctx, addr)
		}()
	}
	r.log.Info().Int("udp", len(r.udp)).Int("tcp", len(r.tcp)).Int("senders", len(r.list.Load().senders)).Msg("relay started")
	wg.Wait()
	if err := r.journal.Close(); err != nil {
		r.log.Warn().Err(err).Msg("journal socket close")
	}
}

func (r *relay) serveUDP(ctx context.Context, c *net.UDPConn) {
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	buf := make([]byte, udpReadBuffer)
	for {
		n, from, err := c.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			r.log.Warn().Err(err).Msg("udp read")
			continue
		}
		r.metrics.received.WithLabelValues("udp").Inc()
		r.handle(from.Addr(), buf[:n])
	}
}

func (r *relay) serveTCP(ctx context.Context, l *net.TCPListener, wg *sync.WaitGroup) {
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()
	for {
		c, err := l.AcceptTCP()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			// Usually out of descriptors; a tight loop would make it worse.
			r.log.Warn().Err(err).Msg("tcp accept")
			select {
			case <-ctx.Done():
				return
			case <-time.After(acceptBackoff):
			}
			continue
		}
		addr := normal(remoteAddr(c))
		if !r.cfg.acceptUnknown && r.list.Load().senders[addr] == nil {
			r.metrics.unknownSender.Inc()
			_ = c.Close()
			continue
		}
		if !r.takeSlot(addr) {
			_ = c.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer r.freeSlot(addr)
			r.serveConn(ctx, c, addr)
		}()
	}
}

// takeSlot claims one of the sender's connections and one of the relay's. A
// refusal is counted under the cap that caused it.
func (r *relay) takeSlot(addr netip.Addr) bool {
	r.perSenderMu.Lock()
	defer r.perSenderMu.Unlock()
	if r.perSender[addr] >= r.cfg.senderCap() {
		r.metrics.dropped.WithLabelValues(dropSenderConns).Inc()
		return false
	}
	select {
	case r.slots <- struct{}{}:
	default:
		r.metrics.dropped.WithLabelValues(dropConns).Inc()
		return false
	}
	r.perSender[addr]++
	return true
}

// freeSlot gives both back. The entry goes with the last connection, so the
// table holds open senders only, however many addresses have come and gone.
func (r *relay) freeSlot(addr netip.Addr) {
	r.perSenderMu.Lock()
	defer r.perSenderMu.Unlock()
	<-r.slots
	if r.perSender[addr]--; r.perSender[addr] <= 0 {
		delete(r.perSender, addr)
	}
}

func remoteAddr(c *net.TCPConn) netip.Addr {
	if a, ok := c.RemoteAddr().(*net.TCPAddr); ok {
		return a.AddrPort().Addr()
	}
	return netip.Addr{}
}

func (r *relay) serveConn(ctx context.Context, c *net.TCPConn, addr netip.Addr) {
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	defer func() { _ = c.Close() }()
	r.metrics.connections.Inc()
	defer r.metrics.connections.Dec()
	br := bufio.NewReader(c)
	for {
		// One deadline per frame, not per read, so a byte a minute does not
		// hold a slot.
		if err := c.SetReadDeadline(time.Now().Add(r.cfg.idle)); err != nil {
			return
		}
		frame, err := syslog.Frame(br, r.cfg.frameCap())
		switch {
		case err == nil:
		case errors.Is(err, syslog.ErrFrameTooLarge):
			r.metrics.dropped.WithLabelValues(dropFrameLarge).Inc()
			return
		case errors.Is(err, syslog.ErrBadFrame):
			r.metrics.dropped.WithLabelValues(dropBadFrame).Inc()
			return
		case errors.Is(err, io.ErrUnexpectedEOF):
			r.metrics.dropped.WithLabelValues(dropShortFrame).Inc()
			return
		default:
			// EOF, idle timeout, shutdown: the connection ended, nothing lost
			// that we were holding.
			return
		}
		r.metrics.received.WithLabelValues("tcp").Inc()
		if !r.handle(addr, frame) {
			return
		}
	}
}

// normal strips the forms of one address that would not compare equal: an
// IPv4-mapped IPv6 from a dual-stack socket, and a zone.
func normal(a netip.Addr) netip.Addr { return a.Unmap().WithZone("") }

// handle takes one message through the checks, cheapest first, and writes it.
// It returns false when the sender is not (or no longer) on the allowlist, so
// a reload that removes a row also ends that row's open connection.
func (r *relay) handle(from netip.Addr, raw []byte) bool {
	addr := normal(from)
	list := r.list.Load()
	s := list.senders[addr]
	b := list.unknown
	if s != nil {
		b = s.bucket
	} else {
		r.metrics.unknownSender.Inc()
		if !r.cfg.acceptUnknown {
			return false
		}
	}
	if !b.allow(time.Now()) {
		r.metrics.dropped.WithLabelValues(dropRate).Inc()
		return true
	}

	m, err := syslog.ParseMax(raw, r.cfg.maxMsg)
	if errors.Is(err, syslog.ErrEmpty) {
		r.metrics.dropped.WithLabelValues(dropEmpty).Inc()
		return true
	}
	source := sourceUnknown
	if s != nil {
		source = s.source(m.AppName)
	}
	fields := journalFields(m, addr, source, err)
	if err != nil {
		r.metrics.malformed.Inc()
	}
	if m.Truncated {
		r.metrics.truncated.Inc()
	}
	if err := r.journal.Send(fields); err != nil {
		r.metrics.dropped.WithLabelValues(dropJournal).Inc()
		return true
	}
	r.metrics.lastSeen.WithLabelValues(source).SetToCurrentTime()
	return true
}

// journalFields builds the entry. Everything out of m is sanitised and is only
// the sender's claim; ADR-124 lists which fields are which. The names are all
// constants, so nothing a sender writes can choose a field.
func journalFields(m syslog.Message, addr netip.Addr, source string, parseErr error) map[string]string {
	fields := map[string]string{
		"MESSAGE":         m.Text,
		"PRIORITY":        strconv.Itoa(max(m.Severity, severityFloor)),
		"SYSLOG_FACILITY": strconv.Itoa(m.Facility),
		"REMOTE_ADDR":     addr.String(),
		"UNHEADED_SOURCE": source,
	}
	for name, value := range map[string]string{
		"SYSLOG_IDENTIFIER":       m.AppName,
		"REMOTE_HOSTNAME_CLAIMED": m.Hostname,
		"SYSLOG_TIMESTAMP":        m.Timestamp,
		"SYSLOG_PID":              m.ProcID,
		"SYSLOG_MSGID":            m.MsgID,
		"SYSLOG_STRUCTURED_DATA":  m.StructuredData,
	} {
		if value != "" {
			fields[name] = value
		}
	}
	if m.Severity < severityFloor {
		fields["UNHEADED_SEVERITY_CLAIMED"] = strconv.Itoa(m.Severity)
	}
	if m.Truncated {
		fields["UNHEADED_TRUNCATED"] = "1"
	}
	if kind := malformedKind(parseErr); kind != "" {
		fields["UNHEADED_MALFORMED"] = kind
	}
	return fields
}

func malformedKind(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, syslog.ErrPRI):
		return "pri"
	case errors.Is(err, syslog.ErrStructuredData):
		return "structured_data"
	}
	return "header"
}

// metricsHandler serves the relay's registry plus go_* and process_*.
func (r *relay) metricsHandler() http.Handler {
	ok := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"healthy"}`+"\n")
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.HandlerFor(r.metrics.reg, metrics.DefaultRegistry))
	mux.HandleFunc("GET /health", ok)
	mux.HandleFunc("GET /ready", ok)
	return mux
}

// serveMetrics keeps a metrics listener up. A failure here is logged and
// retried, never fatal: the overlay address may not exist yet, logs matter
// more than the graph of them, and up == 0 reports it.
func (r *relay) serveMetrics(ctx context.Context, addr string) {
	srv := &http.Server{
		Handler:           r.metricsHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	stop := context.AfterFunc(ctx, func() { _ = srv.Close() })
	defer stop()
	for {
		l, err := new(net.ListenConfig).Listen(ctx, "tcp", addr)
		if err == nil {
			err = srv.Serve(l)
		}
		if ctx.Err() != nil || errors.Is(err, http.ErrServerClosed) {
			return
		}
		r.log.Error().Err(err).Str("addr", addr).Msg("metrics listener down; retrying")
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.cfg.metricsRetry):
		}
	}
}

func run(ctx context.Context, cfg config, log zerolog.Logger) error {
	if err := checkJournalSocket(cfg.journalSocket); err != nil {
		return err
	}
	r, err := newRelay(cfg, log)
	if err != nil {
		return err
	}
	if err := r.listen(); err != nil {
		return err
	}
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				r.reload()
			}
		}
	}()
	r.serve(ctx)
	return nil
}

// checkJournalSocket fails at start rather than dropping every message after.
func checkJournalSocket(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("journal socket: %w", err)
	}
	if st.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("journal socket: %s is not a socket", path)
	}
	return nil
}

func main() {
	cfg, err := parseFlags(os.Args[1:], os.Stdout, os.Stderr)
	var exit *cmdline.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.Code)
	}
	if err != nil {
		os.Exit(cmdline.ReportUsageError(os.Stderr, relaySpec, "%v", err))
	}
	log := zerolog.New(os.Stderr).With().Timestamp().Str("service", serviceName).Logger()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, log); err != nil {
		log.Error().Err(err).Msg("relay stopped")
		os.Exit(1)
	}
}
