// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package wotanClient provides a client for the Wotan message bus.
// It supports both REST control plane and gRPC data plane operations.
package wotanClient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"unheaded/pkg/wotansig"
)

// Backoff constants for pollMessages exponential backoff.
const (
	pollBackoffMin = 500 * time.Millisecond // initial/success poll interval
	pollBackoffMax = 30 * time.Second       // maximum backoff on consecutive errors
)

// pollPageSize is how many messages one HTTP poll asks for. A full page is
// followed by the next poll at once: at one page per interval a poller
// topped out at 200 messages/s and, on a topic busier than that, fell
// further behind until Wotan's ring evicted what it had not read.
const pollPageSize = 100

// Timeout constants for split HTTP clients.
const (
	controlPlaneTimeout = 5 * time.Second  // Subscribe, Publish
	streamingTimeout    = 30 * time.Second // GetMessages, polling
)

// drainBody fully reads and discards any remaining bytes in the response body.
// This is CRITICAL for HTTP keep-alive: the transport cannot reuse a connection
// until the entire response body has been consumed. json.NewDecoder only reads
// enough to parse one JSON value, leaving trailing bytes (newlines, whitespace)
// that block connection recycling.
func drainBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close() // #nosec G104 -- draining and closing a response body; a close error cannot change the outcome
	}
}

// Common errors
var (
	ErrNotConnected        = errors.New("client not connected")
	ErrSubscriptionPending = errors.New("subscription pending approval")
	ErrTopicNotFound       = errors.New("topic not found")
	ErrNotAuthorized       = errors.New("not authorized")
	ErrRateLimited         = errors.New("rate limited")
	// ErrSubscriberNotApproved: Wotan does not know this subscriber ID as an
	// approved member, e.g. because it restarted and forgot its members.
	ErrSubscriberNotApproved = errors.New("subscriber not approved")
)

// Message represents a Wotan message
type Message struct {
	MessageID string    `json:"message_id"`
	Topic     string    `json:"topic"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
	Seq       int64     `json:"seq"`
	Payload   string    `json:"payload"`
	Deleted   bool      `json:"deleted"`
	// Signed is the ML-DSA-65 signature a signed topic message (config.*,
	// drift.*) arrived with over gRPC, nil otherwise; check it with
	// VerifySignature (ADR-105).
	Signed *wotansig.Signed `json:"signed,omitempty"`
}

// Subscriber represents a topic subscriber
type Subscriber struct {
	SubscriberID string    `json:"subscriber_id"`
	Topic        string    `json:"topic"`
	DisplayName  string    `json:"display_name"`
	Status       string    `json:"status"` // pending, approved, denied, revoked
	RequestedAt  time.Time `json:"requested_at"`
}

// safeChannel wraps a channel with a sync.Once so it can be closed
// idempotently from both pollMessages and Client.Close without panic.
type safeChannel struct {
	ch   chan *Message
	done chan struct{} // signals that ch is closed; select on this before sending
	once sync.Once
}

// newSafeChannel creates a safeChannel with the done signal initialized.
func newSafeChannel(bufSize int) *safeChannel {
	return &safeChannel{
		ch:   make(chan *Message, bufSize),
		done: make(chan struct{}),
	}
}

// closeCh closes the underlying channel exactly once, regardless of how many
// times closeCh is called. Closes done first so senders can bail out.
func (sc *safeChannel) closeCh() {
	sc.once.Do(func() {
		close(sc.done)
		close(sc.ch)
	})
}

// send attempts to send a message on ch, returning false if the channel is
// closed or the context is cancelled. Prevents send-on-closed-channel panic.
func (sc *safeChannel) send(ctx context.Context, msg *Message) (sent bool) {
	defer func() {
		if recover() != nil {
			sent = false
		}
	}()
	select {
	case <-sc.done:
		return false
	case <-ctx.Done():
		return false
	case sc.ch <- msg:
		return true
	}
}

// TransportState tracks which transport is active for streaming.
type TransportState int

const (
	// TransportHTTP uses HTTP polling for message streaming (degraded mode).
	TransportHTTP TransportState = iota
	// TransportGRPC uses gRPC server-push streaming (primary, preferred).
	TransportGRPC
)

// String returns a human-readable transport name.
func (ts TransportState) String() string {
	switch ts {
	case TransportGRPC:
		return "gRPC"
	case TransportHTTP:
		return "HTTP"
	default:
		return "unknown"
	}
}

// Client provides access to Wotan message bus.
//
// Transport strategy:
//   - gRPC is the PRIMARY transport for streaming (lower latency, server-push).
//   - HTTP polling is the FALLBACK (circuit-breaker degraded mode only).
//   - HTTP REST is always used for control plane (Subscribe, Publish, admin).
//   - On startup, gRPC is probed. If healthy, it's used exclusively.
//   - If gRPC fails, the client degrades to HTTP polling and logs a warning.
//   - A background probe periodically re-checks gRPC and promotes back when healthy.
type Client struct {
	baseURL       string
	controlClient *http.Client // short timeout for Subscribe, Publish (5s)
	streamClient  *http.Client // long timeout for GetMessages, polling (30s)
	grpcAddr      string       // gRPC endpoint for streaming (empty = HTTP-only mode)
	grpcClient    *GRPCClient  // Lazy-initialized gRPC client
	grpcMu        sync.RWMutex // guards grpcClient initialization (allows retry on failure)

	// Transport state — gRPC primary, HTTP fallback
	transport   TransportState // current active transport
	grpcHealthy bool           // last known gRPC health status
	transportMu sync.RWMutex   // guards transport + grpcHealthy

	// gRPC re-probe while degraded (reprobe.go), guarded by reprobeMu.
	reprobeMu     sync.Mutex
	reprobeOn     bool
	reprobeClosed bool
	reprobeStop   chan struct{} // closed by Close
	reprobeDone   chan struct{} // closed when the current loop exits
	reprobeEvery  time.Duration // 0 means defaultReprobeInterval

	// Subscriber state (per-topic)
	subscribers map[string]*Subscriber
	mu          sync.RWMutex
	// pendingChecked: when Publish last asked Wotan about a pending
	// subscription, per topic (guarded by mu). See pendingRecheckDue.
	pendingChecked map[string]time.Time
	// pendingRecheck: how often a publish on a pending subscription asks
	// Wotan again; 0 means defaultPendingRecheck.
	pendingRecheck time.Duration

	// Message channels for subscriptions
	channels map[string]*safeChannel
	chanMu   sync.RWMutex

	// Fallback queue: telemetry/event publishes that failed because Wotan
	// was unreachable, delivered later by a background flusher (fallback.go).
	// Protected by fallbackMu. Max size: maxFallbackQueue.
	fallbackQueue []pendingMessage
	fallbackMu    sync.Mutex
	// flushMu serialises flushes so buffered messages leave in order.
	flushMu sync.Mutex
	// Atomic counters (FallbackStats).
	fallbackDrops   int64
	fallbackQueued  int64
	fallbackFlushed int64
	fallbackExpired int64
	fallbackRefused int64
	// fallbackMaxAge / fallbackEvery: 0 means maxFallbackAge / fallbackFlushEvery.
	fallbackMaxAge time.Duration
	fallbackEvery  time.Duration
	// now is the clock for queue ages; nil means time.Now.
	now func() time.Time

	// Stream liveness per topic (stream_health.go), guarded by streamsMu.
	streams   map[string]*streamState
	streamsMu sync.Mutex
	// streamUpAfter: 0 means defaultStreamUpAfter.
	streamUpAfter time.Duration

	// Flusher lifecycle, guarded by flusherMu.
	flusherMu     sync.Mutex
	flusherOn     bool
	closed        bool
	flusherCtx    context.Context
	flusherCancel context.CancelFunc
	flusherKick   chan struct{}
	flusherWG     sync.WaitGroup

	// Dead letter counter — total messages sent to dead letter topics.
	deadLetterCount int64 // atomic counter
}

// Transport returns the currently active streaming transport.
func (c *Client) Transport() TransportState {
	c.transportMu.RLock()
	defer c.transportMu.RUnlock()
	return c.transport
}

// IsGRPCHealthy returns whether gRPC was healthy at last check.
func (c *Client) IsGRPCHealthy() bool {
	c.transportMu.RLock()
	defer c.transportMu.RUnlock()
	return c.grpcHealthy
}

// NewClient creates a new Wotan client
func NewClient(addr string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	var baseURL string
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		baseURL = strings.TrimRight(addr, "/") + "/api/v1"
	} else {
		baseURL = fmt.Sprintf("http://%s/api/v1", addr)
	}

	return &Client{
		baseURL:       baseURL,
		controlClient: &http.Client{Timeout: controlPlaneTimeout},
		streamClient:  &http.Client{Timeout: streamingTimeout},
		subscribers:   make(map[string]*Subscriber),
		channels:      make(map[string]*safeChannel),
	}, nil
}

// NewClientWithTLS creates a client with TLS support and split timeouts.
func NewClientWithTLS(addr string, tlsConfig *http.Transport) (*Client, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	return &Client{
		baseURL:       fmt.Sprintf("https://%s/api/v1", addr),
		controlClient: &http.Client{Timeout: controlPlaneTimeout, Transport: tlsConfig},
		streamClient:  &http.Client{Timeout: streamingTimeout, Transport: tlsConfig},
		subscribers:   make(map[string]*Subscriber),
		channels:      make(map[string]*safeChannel),
	}, nil
}

// NewClientWithGRPC creates a Wotan client with gRPC as primary streaming transport.
//
// Transport strategy:
//   - gRPC is probed at startup. If healthy → used exclusively for StreamMessages.
//   - If gRPC probe fails → degrades to HTTP polling (circuit-breaker fallback).
//   - HTTP REST is always used for control plane (Subscribe, Publish).
//   - While degraded (startup probe failed, or a stream tripped the circuit
//     breaker) a background loop re-probes every 30 s and promotes back to
//     gRPC once Wotan answers SERVING. Close stops it.
func NewClientWithGRPC(httpAddr, grpcAddr string) (*Client, error) {
	return newClientWithGRPC(httpAddr, grpcAddr, 0)
}

// newClientWithGRPC is NewClientWithGRPC with the re-probe interval as a
// parameter (0 = defaultReprobeInterval); tests shorten it.
func newClientWithGRPC(httpAddr, grpcAddr string, reprobeEvery time.Duration) (*Client, error) {
	if httpAddr == "" {
		return nil, errors.New("HTTP address cannot be empty")
	}
	if grpcAddr == "" {
		return nil, errors.New("gRPC address cannot be empty")
	}

	c := &Client{
		baseURL:       fmt.Sprintf("http://%s/api/v1", httpAddr),
		controlClient: &http.Client{Timeout: controlPlaneTimeout},
		streamClient:  &http.Client{Timeout: streamingTimeout},
		grpcAddr:      grpcAddr,
		transport:     TransportHTTP, // start degraded, promote after probe
		grpcHealthy:   false,
		subscribers:   make(map[string]*Subscriber),
		channels:      make(map[string]*safeChannel),
		reprobeEvery:  reprobeEvery,
	}

	// Probe gRPC at startup — if healthy, promote to primary transport
	if err := c.ProbeGRPC(); err == nil {
		c.transport = TransportGRPC
		c.grpcHealthy = true
	} else {
		// Probe failed — stay on HTTP in degraded mode, but log so operators
		// know. ProbeGRPC has started the re-probe loop.
		log.Printf("[WARN] wotan_client: gRPC probe failed at startup, running in degraded HTTP mode, re-probing every %s (addr=%s, err=%v)", c.reprobeEveryOr(), grpcAddr, err)
	}

	return c, nil
}

// grpcProbeTimeout bounds one ProbeGRPC health check.
const grpcProbeTimeout = 2 * time.Second

// ProbeGRPC asks Wotan's grpc.health.v1 service whether it is SERVING and
// updates the transport state: gRPC when it is, HTTP polling otherwise.
// Anything but SERVING within 2 s is unhealthy. A failure starts the
// background re-probe loop (reprobe.go), so callers need not re-probe.
//
// It used to declare gRPC healthy as soon as the client object existed;
// grpc.NewClient connects lazily, so that was true with nothing listening.
func (c *Client) ProbeGRPC() error {
	if c.grpcAddr == "" {
		return errors.New("no gRPC address configured")
	}

	grpcCl, err := c.getOrCreateGRPCClient()
	if err == nil {
		err = checkGRPCHealth(grpcCl, grpcProbeTimeout)
	}
	if err != nil {
		c.transportMu.Lock()
		c.grpcHealthy = false
		c.transport = TransportHTTP
		c.transportMu.Unlock()
		c.ensureReprobe()
		return fmt.Errorf("gRPC probe failed: %w", err)
	}

	c.transportMu.Lock()
	wasUnhealthy := !c.grpcHealthy
	c.grpcHealthy = true
	c.transport = TransportGRPC
	c.transportMu.Unlock()

	if wasUnhealthy {
		log.Printf("[INFO] wotan_client: gRPC recovered, promoted to primary transport (addr=%s)", c.grpcAddr)
	}
	// Wotan answers: give anything buffered meanwhile a chance.
	c.kickFlusher()

	return nil
}

// checkGRPCHealth runs one grpc.health.v1 Check for the whole server ("")
// and fails unless the answer is SERVING.
func checkGRPCHealth(gc *GRPCClient, timeout time.Duration) error {
	if gc == nil || gc.conn == nil {
		return errors.New("no gRPC connection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resp, err := healthpb.NewHealthClient(gc.conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	if st := resp.GetStatus(); st != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("health check: %s", st)
	}
	return nil
}

// degradeToHTTP switches streaming to HTTP polling (circuit-breaker trip).
// Logs a warning on the first degradation so operators can see the transition.
func (c *Client) degradeToHTTP() {
	c.transportMu.Lock()
	wasHealthy := c.grpcHealthy
	c.grpcHealthy = false
	c.transport = TransportHTTP
	c.transportMu.Unlock()

	if wasHealthy {
		log.Printf("[WARN] wotan_client: gRPC unhealthy, degraded to HTTP polling (addr=%s)", c.grpcAddr)
	}
	c.ensureReprobe()
}

// Close closes the client and all subscriptions.
// It is safe to call Close even if pollMessages has already closed some channels.
func (c *Client) Close() error {
	c.stopFlusher()
	c.stopReprobe()

	c.chanMu.Lock()
	defer c.chanMu.Unlock()

	for topic, sc := range c.channels {
		sc.closeCh()
		delete(c.channels, topic)
	}

	// Close gRPC client if initialized. Under grpcMu, which every other
	// access to grpcClient holds: under chanMu alone this raced with
	// getOrCreateGRPCClient (StreamMessages, ProbeGRPC).
	c.grpcMu.Lock()
	if c.grpcClient != nil {
		_ = c.grpcClient.Close()
		c.grpcClient = nil
	}
	c.grpcMu.Unlock()

	return nil
}

// Subscribe requests subscription to a topic
// Returns subscriber info (may be in pending status)
func (c *Client) Subscribe(ctx context.Context, topic, displayName string) (*Subscriber, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}
	if displayName == "" {
		return nil, errors.New("display name cannot be empty")
	}

	body := map[string]string{"display_name": displayName}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/topics/%s/subscribe", c.baseURL, url.PathEscape(topic)),
		bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.controlClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subscribe request: %w", err)
	}
	defer drainBody(resp)

	if resp.StatusCode != http.StatusCreated {
		return nil, c.parseError(resp)
	}

	var result struct {
		Subscriber *Subscriber `json:"subscriber"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	// A 201 without a subscriber would be stored as nil and crash the next
	// Publish on the topic.
	if result.Subscriber == nil {
		return nil, errors.New("decode response: no subscriber")
	}

	c.mu.Lock()
	c.subscribers[topic] = result.Subscriber
	if result.Subscriber.Status == "approved" {
		delete(c.pendingChecked, topic)
	} else {
		c.setPendingChecked(topic, time.Now())
	}
	c.mu.Unlock()

	return result.Subscriber, nil
}

// defaultPendingRecheck is how often Publish asks Wotan again about a
// subscription it last saw pending.
const defaultPendingRecheck = 5 * time.Minute

// setPendingChecked records when a pending subscription on topic was last
// put to Wotan. Must be called with mu held.
func (c *Client) setPendingChecked(topic string, at time.Time) {
	if c.pendingChecked == nil {
		c.pendingChecked = make(map[string]time.Time)
	}
	c.pendingChecked[topic] = at
}

// pendingRecheckDue reports whether a publish on the pending subscription
// sub should go to Wotan rather than fail locally, and if so claims the
// check so concurrent publishes do not all make it.
//
// A pending status used to be final: Publish refused every message locally
// and never asked again, so a subscription approved later stayed refused
// until the process restarted. On east kev-poller joined while Wotan's
// allowlist lacked it; Wotan restarted with it on the list at 18:17 on
// 2026-10-20, and kev-poller's hourly publishes still failed "subscription
// pending approval" until it was restarted 17 hours later.
func (c *Client) pendingRecheckDue(topic string, sub *Subscriber) bool {
	every := c.pendingRecheck
	if every <= 0 {
		every = defaultPendingRecheck
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subscribers[topic] != sub {
		return false // re-subscribed meanwhile; the caller retries with that
	}
	last, ok := c.pendingChecked[topic]
	if ok && now.Sub(last) < every {
		return false
	}
	c.setPendingChecked(topic, now)
	// The first sight of a pending subscription only starts the clock.
	return ok
}

// markApproved records that Wotan accepted a publish from sub, so its
// pending status is out of date.
func (c *Client) markApproved(topic string, sub *Subscriber) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subscribers[topic] != sub {
		return
	}
	approved := *sub
	approved.Status = "approved"
	c.subscribers[topic] = &approved
	delete(c.pendingChecked, topic)
}

// Publish sends a message to a topic.
// When Wotan cannot be reached (connection failure, 502/503/504) the message
// is buffered in a local fallback queue and a background flusher delivers it
// once Wotan answers, unless it is older than maxFallbackAge by then. Control
// topics (chaos., config., deploy., cuirass.) are never buffered: Publish
// returns the error and the caller decides.
func (c *Client) Publish(ctx context.Context, topic string, payload []byte) error {
	if topic == "" {
		return errors.New("topic cannot be empty")
	}
	if len(payload) == 0 {
		return errors.New("payload cannot be empty")
	}

	c.mu.RLock()
	sub, ok := c.subscribers[topic]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("not subscribed to topic %s", topic)
	}
	pending := sub.Status != "approved"
	// Now and then a pending subscription is put to Wotan again: by
	// publishing with it (an admin may have approved it) and, if refused,
	// by joining again (Wotan may have restarted with the name allowlisted).
	if pending && !c.pendingRecheckDue(topic, sub) {
		return ErrSubscriptionPending
	}

	networkErr, appErr := c.doPublish(ctx, topic, sub.SubscriberID, payload)
	if pending && networkErr == nil && appErr == nil {
		c.markApproved(topic, sub)
	}
	if errors.Is(appErr, ErrSubscriberNotApproved) {
		// Wotan keeps memberships in memory, so after it restarts the ID it
		// issued is unknown and every publish is refused until this process
		// restarts too. Join again under the same name and retry once.
		fresh, err := c.Subscribe(ctx, topic, sub.DisplayName)
		if err != nil {
			return fmt.Errorf("rejoin %s: %w", topic, err)
		}
		if fresh.Status != "approved" {
			return ErrSubscriptionPending
		}
		networkErr, appErr = c.doPublish(ctx, topic, fresh.SubscriberID, payload)
	}
	if appErr != nil {
		// Application-level error (403, 404, etc.) — return directly, don't buffer.
		return appErr
	}
	if networkErr != nil {
		if !bufferable(topic) {
			c.countFallback("refused", 1)
			return fmt.Errorf("publish %s (control topic, not buffered): %w", topic, networkErr)
		}
		// Network/transport error — buffer for retry on reconnect.
		c.enqueueFallback(topic, payload)
		return fmt.Errorf("publish request (buffered for retry): %w", networkErr)
	}

	// Wotan answered: anything still queued has a chance now.
	c.kickFlusher()
	return nil
}

// doPublish performs the actual HTTP POST to Wotan.
// Returns (networkErr, appErr). Only one will be non-nil at a time.
// networkErr: transport/connectivity failures (safe to retry).
// appErr: server-side rejections (403, 404, etc.) that should not be retried.
func (c *Client) doPublish(ctx context.Context, topic, subscriberID string, payload []byte) (networkErr, appErr error) {
	body := map[string]string{
		"subscriber_id": subscriberID,
		"payload":       string(payload),
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err), nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/topics/%s/publish", c.baseURL, url.PathEscape(topic)),
		bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err), nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.controlClient.Do(req)
	if err != nil {
		return fmt.Errorf("publish request: %w", err), nil
	}
	defer drainBody(resp)

	switch resp.StatusCode {
	case http.StatusCreated:
		return nil, nil
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		// Wotan (or the proxy in front of it) cannot take the message right
		// now: as retryable as a refused connection.
		return fmt.Errorf("publish: HTTP %d: %w", resp.StatusCode, c.parseError(resp)), nil
	default:
		return nil, c.parseError(resp)
	}
}

// GetMessages retrieves messages from a topic
func (c *Client) GetMessages(ctx context.Context, topic string, afterSeq int64, limit int) ([]*Message, error) {
	msgs, _, err := c.getMessagesPage(ctx, topic, afterSeq, limit)
	return msgs, err
}

// getMessagesPage is GetMessages plus the server's last_seq, nil when the
// server does not report one (a Wotan older than the field).
func (c *Client) getMessagesPage(ctx context.Context, topic string, afterSeq int64, limit int) ([]*Message, *int64, error) {
	if topic == "" {
		return nil, nil, errors.New("topic cannot be empty")
	}

	// PathEscape: a pattern's "#" would otherwise start a URL fragment.
	reqURL := fmt.Sprintf("%s/topics/%s/messages?after_seq=%d", c.baseURL, url.PathEscape(topic), afterSeq)
	if limit > 0 {
		reqURL = fmt.Sprintf("%s&limit=%d", reqURL, limit)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("get messages request: %w", err)
	}
	defer drainBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, nil, c.parseError(resp)
	}

	var result struct {
		Messages []*Message `json:"messages"`
		LastSeq  *int64     `json:"last_seq"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Messages, result.LastSeq, nil
}

// nextCursor is where a poller resumes after a page. A server whose last_seq
// is below the cursor has restarted (seqs are in memory and begin again at
// 1), so the cursor rewinds to 0 rather than waiting for the new seqs to
// overtake the old ones.
func nextCursor(cursor int64, serverLast *int64) int64 {
	if serverLast != nil && *serverLast < cursor {
		return 0
	}
	return cursor
}

// streamCursor advances a gRPC resume cursor for an event carrying seq got,
// on a stream opened with since_seq requested. Normally it only moves
// forward. But a correct Wotan never sends a seq at or below since_seq, so an
// event that does is from a restarted Wotan numbering from 1 again, and the
// cursor follows it down; left above the new seqs, every later reconnect
// would ask for the wrong range. Wotan's seqs are global across topics, so
// this holds for pattern streams too.
func streamCursor(requested, cursor, got int64) int64 {
	if got > cursor || (requested > 0 && got <= requested) {
		return got
	}
	return cursor
}

// StreamMessages opens a channel for receiving messages.
// Uses the active transport: gRPC (primary) or HTTP polling (circuit-breaker fallback).
// The channel will be closed when context is cancelled.
func (c *Client) StreamMessages(ctx context.Context, topic string) (<-chan *Message, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}

	c.mu.RLock()
	sub, ok := c.subscribers[topic]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("not subscribed to topic %s", topic)
	}
	if sub.Status != "approved" {
		return nil, ErrSubscriptionPending
	}

	// Use gRPC if it's the active transport and configured
	c.transportMu.RLock()
	activeTransport := c.transport
	c.transportMu.RUnlock()

	if activeTransport == TransportGRPC && c.grpcAddr != "" {
		grpcCl, err := c.getOrCreateGRPCClient()
		if err == nil {
			_, _ = grpcCl.Subscribe(ctx, topic, sub.DisplayName)
			ch, err := grpcCl.StreamMessages(ctx, topic)
			if err == nil {
				sc := newSafeChannel(100)
				c.chanMu.Lock()
				c.channels[topic] = sc
				c.chanMu.Unlock()
				go c.relayGRPC(ctx, topic, ch, sc)
				return sc.ch, nil
			}
		}
		// gRPC failed at runtime — trip circuit breaker, degrade to HTTP
		c.degradeToHTTP()
	}

	// HTTP polling — either primary (no gRPC configured) or circuit-breaker fallback
	sc := newSafeChannel(100)

	c.chanMu.Lock()
	c.channels[topic] = sc
	c.chanMu.Unlock()

	go c.pollMessages(ctx, topic, sc)
	return sc.ch, nil
}

// relayGRPC forwards a gRPC stream to sc and, when the gRPC client gives up
// on it, carries on by HTTP polling from the last seq delivered.
//
// The gRPC client ends a stream after maxRetries reconnects within one
// outage (about 11 s of backoff) or on any non-transient error, and the
// caller's channel used to close with it: a Wotan restart longer than that
// ended a service's subscription for good (timeguru's listeners log
// "channel closed" and return).
func (c *Client) relayGRPC(ctx context.Context, topic string, in <-chan *Message, sc *safeChannel) {
	var lastSeq int64
	for msg := range in {
		if msg == nil {
			continue
		}
		if !sc.send(ctx, msg) {
			c.endStream(topic, sc)
			return
		}
		c.streamMessage(topic)
		lastSeq = msg.Seq
	}
	select {
	case <-ctx.Done():
		c.endStream(topic, sc)
		return
	case <-sc.done:
		c.endStream(topic, sc) // Close
		return
	default:
	}
	c.degradeToHTTP()
	c.pollMessagesFrom(ctx, topic, sc, lastSeq)
}

// endStream closes sc and removes it from the channels map unless a later
// StreamMessages on the topic has replaced it there. The topic's health
// entry goes with it: a stream its caller ended is not an outage.
func (c *Client) endStream(topic string, sc *safeChannel) {
	sc.closeCh()
	c.chanMu.Lock()
	current := c.channels[topic] == sc
	if current {
		delete(c.channels, topic)
	}
	c.chanMu.Unlock()
	if current {
		c.forgetStream(topic)
	}
}

// getOrCreateGRPCClient lazily initializes the gRPC client using true
// double-check locking. The fast path reads grpcClient under a read lock;
// only if nil is the write lock acquired and grpcClient checked again before
// creating a new connection. Unlike sync.Once, this allows retrying if the
// initial connection failed (e.g., Wotan was temporarily unreachable).
func (c *Client) getOrCreateGRPCClient() (*GRPCClient, error) {
	// First check (read lock) — fast path for existing client
	c.grpcMu.RLock()
	if c.grpcClient != nil {
		cl := c.grpcClient
		c.grpcMu.RUnlock()
		return cl, nil
	}
	c.grpcMu.RUnlock()

	// Second check (write lock) — verify another goroutine didn't create it
	c.grpcMu.Lock()
	defer c.grpcMu.Unlock()

	if c.grpcClient != nil {
		return c.grpcClient, nil
	}

	// Still nil — create the gRPC client under write lock
	client, err := NewGRPCClient(c.grpcAddr)
	if err != nil {
		return nil, fmt.Errorf("create gRPC client: %w", err)
	}
	client.onStreamState = c.setStreamUp
	client.upAfter = c.streamUpAfterOr()
	c.grpcClient = client
	return client, nil
}

// backoff calculates exponential backoff: min(pollBackoffMin * 2^failures, pollBackoffMax).
func backoff(failures int) time.Duration {
	if failures <= 0 {
		return pollBackoffMin
	}
	delay := time.Duration(float64(pollBackoffMin) * math.Pow(2, float64(failures)))
	// Guard against int64 overflow (large exponents wrap negative).
	if delay <= 0 || delay > pollBackoffMax {
		return pollBackoffMax
	}
	return delay
}

// pollMessages polls for new messages (fallback when gRPC not available).
// Uses exponential backoff on errors: 500ms -> 1s -> 2s -> ... -> 30s max.
func (c *Client) pollMessages(ctx context.Context, topic string, sc *safeChannel) {
	c.pollMessagesFrom(ctx, topic, sc, 0)
}

// pollMessagesFrom is pollMessages resuming after seq lastSeq.
func (c *Client) pollMessagesFrom(ctx context.Context, topic string, sc *safeChannel, lastSeq int64) {
	// Remove from the channels map so Close() won't try again.
	defer c.endStream(topic, sc)

	var consecutiveFailures int

	timer := time.NewTimer(pollBackoffMin)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sc.done:
			// Close closed the channel. Without this a poller on a quiet
			// topic noticed only when a message came, and otherwise kept
			// polling Wotan every 500 ms for as long as its ctx lived.
			return
		case <-timer.C:
			msgs, serverLast, err := c.getMessagesPage(ctx, topic, lastSeq, pollPageSize)
			if err != nil {
				if ctx.Err() == nil {
					c.setStreamUp(topic, false)
				}
				consecutiveFailures++
				timer.Reset(backoff(consecutiveFailures))
				continue
			}
			// Success — reset backoff to base interval. A poll that answers
			// is a live stream, messages or not.
			c.setStreamUp(topic, true)
			consecutiveFailures = 0
			lastSeq = nextCursor(lastSeq, serverLast)
			polledFrom := lastSeq
			for _, msg := range msgs {
				// Never deliver at or below the cursor, whatever the server sent.
				if msg == nil || msg.Seq <= lastSeq {
					continue
				}
				if !sc.send(ctx, msg) {
					return
				}
				c.streamMessage(topic)
				lastSeq = msg.Seq
			}
			// A full page that moved the cursor: more are waiting. (One
			// that did not is a server ignoring after_seq; no tight loop.)
			if len(msgs) >= pollPageSize && lastSeq > polledFrom {
				timer.Reset(0)
			} else {
				timer.Reset(pollBackoffMin)
			}
		}
	}
}

// parseError extracts error details from response
func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	switch errResp.Error.Code {
	case "SUBSCRIPTION_PENDING":
		return ErrSubscriptionPending
	case "INVALID_TOPIC":
		return ErrTopicNotFound
	case "NOT_AUTHORIZED", "SUBSCRIPTION_DENIED", "SUBSCRIPTION_REVOKED":
		return ErrNotAuthorized
	case "RATE_LIMITED":
		return ErrRateLimited
	case "Forbidden":
		return fmt.Errorf("%w (%s)", ErrSubscriberNotApproved, errResp.Error.Message)
	default:
		return fmt.Errorf("%s: %s", errResp.Error.Code, errResp.Error.Message)
	}
}

// GetSubscriber returns the subscriber for a topic
func (c *Client) GetSubscriber(topic string) (*Subscriber, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sub, ok := c.subscribers[topic]
	return sub, ok
}

// IsApproved checks if subscription to topic is approved
func (c *Client) IsApproved(topic string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sub, ok := c.subscribers[topic]
	return ok && sub.Status == "approved"
}
