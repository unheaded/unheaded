// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
	wotanClient "unheaded/pkg/wotan-client"
)

// Audit sinks for both brains (S-TWO-BRAINS Phase 1, ADR-104 open
// question 5: both). Per-request telemetry goes to the Well's
// unheaded_ops.llm_requests (migration 030); tool-gate decisions go to
// unheaded_ops.audit_events (022) through pkg/audit's taxonomy; both are
// also published on Wotan as the live view. No sink ever records prompt,
// response or tool input text: records carry counts, cost and hashes.

// Backends, as the llm_requests.backend CHECK spells them.
const (
	BackendClaude = "claude"
	BackendLocal  = "local"
)

// Wotan topics for the live view.
const (
	TopicLLMRequests = "zhen.llm.requests"
	TopicToolCalls   = "zhen.audit.toolcalls"
)

// MaxWotanPayloadBytes caps one published record.
const MaxWotanPayloadBytes = 4 << 10

// maxAuditReasonBytes caps free-text reasons and errors in every sink.
const maxAuditReasonBytes = 512

// DefaultAuditQueue is each async sink's queue length.
const DefaultAuditQueue = 1024

// auditSources are the services that may write llm_requests (030's CHECK).
var auditSources = map[string]bool{"zhen-claude": true, "zhen-agentd": true, "zhen-eval": true, "zhen-mcp": true}

// wotanPublishers are the names allowlisted on Wotan (configs/wotan.yaml).
var wotanPublishers = map[string]bool{"zhen-claude": true, "zhen-agentd": true, "zhen-eval": true}

// routeReasons are 030's route_reason values; anything else is blanked.
var routeReasons = map[string]bool{
	"": true, "local_first": true, "escalated_check_fail": true, "escalated_parse_fail": true,
	"escalated_refusal": true, "claude_direct": true, "local_only": true, "manual": true,
}

var toolNameAuditRE = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

var (
	auditDropped = auto.NewCounterVec(prom.CounterOpts{
		Name: "zhen_audit_dropped_total",
		Help: "Audit records dropped because a sink's queue was full.",
	}, []string{"sink"})
	auditErrors = auto.NewCounterVec(prom.CounterOpts{
		Name: "zhen_audit_errors_total",
		Help: "Audit records a sink failed to write.",
	}, []string{"sink"})
)

// MultiAuditor fans every record out to each auditor concurrently. A slow
// auditor delays the call's return but not the others' delivery; a
// panicking auditor is contained and the rest still receive the record.
type MultiAuditor []Auditor

// Request implements Auditor.
func (m MultiAuditor) Request(ctx context.Context, r RequestRecord) {
	m.each(func(a Auditor) { a.Request(ctx, r) })
}

// ToolCall implements Auditor.
func (m MultiAuditor) ToolCall(ctx context.Context, r ToolRecord) {
	m.each(func(a Auditor) { a.ToolCall(ctx, r) })
}

func (m MultiAuditor) each(f func(Auditor)) {
	var wg sync.WaitGroup
	for _, a := range m {
		if a == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = recover() }() // one broken sink must not take the others down
			f(a)
		}()
	}
	wg.Wait()
}

// LLMRequestRow is one unheaded_ops.llm_requests row.
type LLMRequestRow struct {
	OccurredAt      time.Time `json:"occurred_at"`
	SourceService   string    `json:"source_service"`
	Backend         string    `json:"backend"`
	ModelRequested  string    `json:"model_requested"`
	ModelServed     string    `json:"model_served"`
	ConversationID  string    `json:"conversation_id"`
	Iteration       int       `json:"iteration"`
	TaskFamily      string    `json:"task_family"`
	RouteReason     string    `json:"route_reason"`
	StopReason      string    `json:"stop_reason"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	CacheRead       int64     `json:"cache_read"`
	CacheWrite      int64     `json:"cache_write"`
	CostNanoUSD     int64     `json:"cost_nano_usd"`
	ReservedNanoUSD int64     `json:"reserved_nano_usd"`
	DurationMS      int64     `json:"duration_ms"`
	Redactions      int       `json:"redactions"`
	Sent            bool      `json:"sent"`
	Error           string    `json:"error"`
}

// RequestSink stores llm_requests rows.
type RequestSink interface {
	InsertLLMRequest(ctx context.Context, r LLMRequestRow) error
}

// SQLRequestSink writes llm_requests through database/sql as ops_writer.
type SQLRequestSink struct{ DB *sql.DB }

// InsertLLMRequest implements RequestSink.
func (s SQLRequestSink) InsertLLMRequest(ctx context.Context, r LLMRequestRow) error {
	if s.DB == nil {
		return errors.New("claudebridge: SQLRequestSink has no database")
	}
	const q = `INSERT INTO llm_requests (occurred_at, source_service, backend, model_requested, model_served,
		conversation_id, iteration, task_family, route_reason, stop_reason, input_tokens, output_tokens,
		cache_read, cache_write, cost_nano_usd, reserved_nano_usd, duration_ms, redactions, sent, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`
	_, err := s.DB.ExecContext(ctx, q, r.OccurredAt, r.SourceService, r.Backend, r.ModelRequested, r.ModelServed,
		r.ConversationID, r.Iteration, r.TaskFamily, r.RouteReason, r.StopReason, r.InputTokens, r.OutputTokens,
		r.CacheRead, r.CacheWrite, r.CostNanoUSD, r.ReservedNanoUSD, r.DurationMS, r.Redactions, r.Sent, r.Error)
	return err
}

// asyncQueue runs jobs one at a time on its own goroutine. Enqueue never
// blocks: a full queue drops the job and counts it. One worker also
// serialises audit_events writes, so the hash chain has one writer per
// process.
type asyncQueue struct {
	name    string
	log     *slog.Logger
	jobs    chan func(context.Context) error
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
	mu      sync.RWMutex // guards closed against a send on a closed channel
	closed  bool
	dropped atomic.Int64
	errs    atomic.Int64
}

func newAsyncQueue(name string, size int, log *slog.Logger) *asyncQueue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &asyncQueue{name: name, log: log, jobs: make(chan func(context.Context) error, size), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go q.run()
	return q
}

func (q *asyncQueue) run() {
	defer close(q.done)
	for job := range q.jobs {
		jctx, cancel := context.WithTimeout(q.ctx, 10*time.Second)
		err := job(jctx)
		cancel()
		if err != nil {
			q.errs.Add(1)
			auditErrors.WithLabelValues(q.name).Inc()
			if q.log != nil {
				// The error text never holds record content: rows carry none.
				q.log.Warn("audit_sink_error", "sink", q.name, "error", truncateUTF8Plain(err.Error(), maxAuditReasonBytes))
			}
		}
	}
}

func (q *asyncQueue) enqueue(job func(context.Context) error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		q.drop()
		return
	}
	select {
	case q.jobs <- job:
	default:
		q.drop()
	}
}

func (q *asyncQueue) drop() {
	q.dropped.Add(1)
	auditDropped.WithLabelValues(q.name).Inc()
}

// close stops accepting jobs and waits for the queue to drain until ctx
// ends; then it cancels whatever is still running.
func (q *asyncQueue) close(ctx context.Context) error {
	q.once.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.jobs)
		q.mu.Unlock()
	})
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		q.cancel()
		<-q.done
		return fmt.Errorf("audit sink %s: closed before draining: %w", q.name, ctx.Err())
	}
}

// WellAuditorConfig configures a WellAuditor.
type WellAuditorConfig struct {
	Source    string        // source_service: zhen-claude, zhen-agentd, zhen-eval, zhen-mcp
	Backend   string        // default backend of this process's requests: claude or local
	Requests  RequestSink   // llm_requests; nil skips request rows
	Events    audit.Storage // audit_events; nil skips tool-gate events
	QueueSize int           // default DefaultAuditQueue
	Log       *slog.Logger  // sink errors; default discard
}

// WellAuditor writes request rows and tool-gate audit events to the
// Well, asynchronously: a record is queued and the call returns at once.
type WellAuditor struct {
	cfg      WellAuditorConfig
	q        *asyncQueue
	lastMu   sync.Mutex
	lastTime time.Time
}

// NewWellAuditor validates cfg and starts the writer.
func NewWellAuditor(cfg WellAuditorConfig) (*WellAuditor, error) {
	if cfg.Requests == nil && cfg.Events == nil {
		return nil, errors.New("claudebridge: WellAuditor needs a request sink or an event store")
	}
	if !auditSources[cfg.Source] {
		return nil, fmt.Errorf("claudebridge: audit source %q is not one of zhen-claude, zhen-agentd, zhen-eval, zhen-mcp", cfg.Source)
	}
	if cfg.Backend != BackendClaude && cfg.Backend != BackendLocal {
		return nil, fmt.Errorf("claudebridge: backend %q is not claude or local", cfg.Backend)
	}
	if cfg.QueueSize < 0 {
		return nil, errors.New("claudebridge: negative audit queue size")
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = DefaultAuditQueue
	}
	return &WellAuditor{cfg: cfg, q: newAsyncQueue("well", cfg.QueueSize, cfg.Log)}, nil
}

// Dropped is the number of records dropped on a full queue.
func (w *WellAuditor) Dropped() int64 { return w.q.dropped.Load() }

// Errors is the number of records the Well refused or could not take.
func (w *WellAuditor) Errors() int64 { return w.q.errs.Load() }

// Close drains the queue (bounded by ctx).
func (w *WellAuditor) Close(ctx context.Context) error { return w.q.close(ctx) }

// Request implements Auditor.
func (w *WellAuditor) Request(_ context.Context, r RequestRecord) {
	if w.cfg.Requests == nil {
		return
	}
	row := requestRow(w.cfg.Source, w.cfg.Backend, r, time.Now())
	w.q.enqueue(func(ctx context.Context) error { return w.cfg.Requests.InsertLLMRequest(ctx, row) })
}

// ToolCall implements Auditor.
func (w *WellAuditor) ToolCall(_ context.Context, r ToolRecord) {
	if w.cfg.Events == nil {
		return
	}
	ev := toolEvent(w.cfg.Source, w.cfg.Backend, r)
	w.q.enqueue(func(ctx context.Context) error { return w.store(ctx, ev) })
}

// Event queues an audit event built by the caller (confirmations, Phase
// 2). Its ID, timestamp and hash chain are filled here.
func (w *WellAuditor) Event(ev *audit.AuditEvent) {
	if w.cfg.Events == nil || ev == nil {
		return
	}
	w.q.enqueue(func(ctx context.Context) error { return w.store(ctx, ev) })
}

// store chains ev onto the last stored event and writes it. It runs on
// the queue's single worker, so GetLastHash and Store cannot interleave
// with another write from this process.
func (w *WellAuditor) store(ctx context.Context, ev *audit.AuditEvent) error {
	if ev.ID == "" {
		ev.ID = newEventID()
	}
	ev.Timestamp = w.nextTimestamp()
	prev, err := w.cfg.Events.GetLastHash(ctx)
	if err != nil {
		return fmt.Errorf("audit chain: %w", err)
	}
	ev.PreviousHash = prev
	ev.Hash = ev.ComputeHash()
	return w.cfg.Events.Store(ctx, ev)
}

// nextTimestamp is now at microsecond precision (what Postgres keeps),
// strictly after the previous one, so "the last event" is well defined.
func (w *WellAuditor) nextTimestamp() time.Time {
	w.lastMu.Lock()
	defer w.lastMu.Unlock()
	t := time.Now().UTC().Truncate(time.Microsecond)
	if !t.After(w.lastTime) {
		t = w.lastTime.Add(time.Microsecond)
	}
	w.lastTime = t
	return t
}

// requestRow maps a RequestRecord to a row; record fields win over the
// process defaults.
func requestRow(source, backend string, r RequestRecord, now time.Time) LLMRequestRow {
	if r.Backend != "" {
		backend = r.Backend
	}
	model := r.Model
	if model == "" {
		model = "unknown"
	}
	conv := r.Conversation
	if conv == "" {
		conv = "unknown"
	}
	route := r.RouteReason
	if !routeReasons[route] {
		route = ""
	}
	return LLMRequestRow{
		OccurredAt:      now.Add(-r.Duration).UTC().Truncate(time.Microsecond),
		SourceService:   source,
		Backend:         backend,
		ModelRequested:  truncateUTF8Plain(model, 128),
		ModelServed:     truncateUTF8Plain(r.ServedBy, 128),
		ConversationID:  truncateUTF8Plain(conv, 128),
		Iteration:       max(r.Iteration, 0),
		TaskFamily:      truncateUTF8Plain(r.TaskFamily, 64),
		RouteReason:     route,
		StopReason:      truncateUTF8Plain(r.StopReason, 64),
		InputTokens:     max(r.InputTokens, 0),
		OutputTokens:    max(r.OutputTokens, 0),
		CacheRead:       max(r.CacheRead, 0),
		CacheWrite:      max(r.CacheWrite, 0),
		CostNanoUSD:     max(int64(r.Cost), 0),
		ReservedNanoUSD: max(int64(r.Reserved), 0),
		DurationMS:      max(r.Duration.Milliseconds(), 0),
		Redactions:      max(r.Redactions, 0),
		Sent:            r.Sent,
		Error:           truncateUTF8Plain(r.Err, maxAuditReasonBytes),
	}
}

// actorID is how audit_events names a model: claude-api/<model> or
// zhen-local/<model>.
func actorID(backend, model string) string {
	if model == "" {
		model = "unknown"
	}
	prefix := "claude-api/"
	if backend == BackendLocal {
		prefix = "zhen-local/"
	}
	return truncateUTF8Plain(prefix+model, 160)
}

// toolResourceID is the tool name if it is a plain identifier, else
// "invalid": a model-chosen string never becomes a resource id as is.
func toolResourceID(name string) string {
	if toolNameAuditRE.MatchString(name) {
		return name
	}
	return "invalid"
}

// toolEvent maps a ToolRecord to an audit event (Step 31's mapping).
func toolEvent(source, backend string, r ToolRecord) *audit.AuditEvent {
	if r.Backend != "" {
		backend = r.Backend
	}
	outcome, severity := audit.OutcomeFailure, audit.SeverityWarning
	switch {
	case r.Decision == "allowed":
		outcome, severity = audit.OutcomeSuccess, audit.SeverityInfo
	case r.Pending:
		outcome = audit.OutcomePending
	}
	return &audit.AuditEvent{
		Type:     audit.EventTypeAuthorization,
		Severity: severity,
		Action:   "tool_call",
		Outcome:  outcome,
		Actor:    &audit.Actor{ID: actorID(backend, r.Model), Type: "model"},
		Resource: &audit.Resource{ID: toolResourceID(r.Name), Type: "tool"},
		Source:   &audit.Source{Service: source, RequestID: truncateUTF8Plain(r.Conversation, 128)},
		Details: map[string]any{
			"provider":     truncateUTF8Plain(r.Provider, 64),
			"decision":     truncateUTF8Plain(r.Decision, 32),
			"reason":       truncateUTF8Plain(r.Reason, maxAuditReasonBytes),
			"input_sha256": truncateUTF8Plain(r.InputSHA256, 64),
			"input_bytes":  r.InputBytes,
			"output_bytes": r.OutputBytes,
			"truncated":    r.Truncated,
			"pending":      r.Pending,
			"is_error":     r.IsError,
			"redactions":   r.Redactions,
			"tool_use_id":  truncateUTF8Plain(r.ToolUseID, 64),
		},
	}
}

// NewRecordID is a random 128-bit hex id for audit records and local
// conversations that have no Claude conversation id.
func NewRecordID() string { return newEventID() }

func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on Linux; a time-derived id still keeps
		// the primary key unique enough to surface rather than hide a write.
		return fmt.Sprintf("t%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// truncateUTF8Plain cuts s to at most n bytes on a rune boundary, no marker.
func truncateUTF8Plain(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && cut < len(s) && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}

// TopicPublisher is the part of a Wotan client the auditor uses.
type TopicPublisher interface {
	// Join subscribes displayName to topic (Wotan approves allowlisted names).
	Join(ctx context.Context, topic, displayName string) error
	Publish(ctx context.Context, topic string, payload []byte) error
}

// WotanAuditorConfig configures a WotanAuditor.
type WotanAuditorConfig struct {
	Pub       TopicPublisher
	Name      string // display name on Wotan: zhen-claude, zhen-agentd, zhen-eval
	Backend   string // default backend: claude or local
	QueueSize int
	Log       *slog.Logger
}

// WotanAuditor publishes every record on Wotan, asynchronously. It joins
// each topic before its first publish (the kanban-app lesson, 8c66b949d:
// publishing to a topic it never joined is refused).
type WotanAuditor struct {
	cfg    WotanAuditorConfig
	q      *asyncQueue
	joined map[string]bool // only touched by the queue's worker
}

// NewWotanAuditor validates cfg and starts the publisher.
func NewWotanAuditor(cfg WotanAuditorConfig) (*WotanAuditor, error) {
	if cfg.Pub == nil {
		return nil, errors.New("claudebridge: WotanAuditor needs a publisher")
	}
	if !wotanPublishers[cfg.Name] {
		return nil, fmt.Errorf("claudebridge: Wotan name %q is not one of the allowlisted zhen-claude, zhen-agentd, zhen-eval", cfg.Name)
	}
	if cfg.Backend != BackendClaude && cfg.Backend != BackendLocal {
		return nil, fmt.Errorf("claudebridge: backend %q is not claude or local", cfg.Backend)
	}
	if cfg.QueueSize < 0 {
		return nil, errors.New("claudebridge: negative audit queue size")
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = DefaultAuditQueue
	}
	return &WotanAuditor{cfg: cfg, q: newAsyncQueue("wotan", cfg.QueueSize, cfg.Log), joined: map[string]bool{}}, nil
}

// Dropped is the number of records dropped on a full queue.
func (w *WotanAuditor) Dropped() int64 { return w.q.dropped.Load() }

// Errors is the number of records not published (Wotan down, refused).
func (w *WotanAuditor) Errors() int64 { return w.q.errs.Load() }

// Close drains the queue (bounded by ctx).
func (w *WotanAuditor) Close(ctx context.Context) error { return w.q.close(ctx) }

// Request implements Auditor.
func (w *WotanAuditor) Request(_ context.Context, r RequestRecord) {
	row := requestRow(w.cfg.Name, w.cfg.Backend, r, time.Now())
	w.publish(TopicLLMRequests, row)
}

// ToolCall implements Auditor.
func (w *WotanAuditor) ToolCall(_ context.Context, r ToolRecord) {
	ev := toolEvent(w.cfg.Name, w.cfg.Backend, r)
	msg := map[string]any{
		"ts":           time.Now().UTC().Format(time.RFC3339Nano),
		"source":       w.cfg.Name,
		"conversation": ev.Source.RequestID,
		"actor":        ev.Actor.ID,
		"tool":         ev.Resource.ID,
		"outcome":      string(ev.Outcome),
	}
	for k, v := range ev.Details {
		msg[k] = v
	}
	w.publish(TopicToolCalls, msg)
}

// PublishJSON queues any small JSON record on topic (confirmations).
func (w *WotanAuditor) PublishJSON(topic string, v any) { w.publish(topic, v) }

func (w *WotanAuditor) publish(topic string, v any) {
	payload, err := json.Marshal(v)
	if err != nil || len(payload) > MaxWotanPayloadBytes {
		w.q.drop()
		return
	}
	w.q.enqueue(func(ctx context.Context) error {
		if !w.joined[topic] {
			if err := w.cfg.Pub.Join(ctx, topic, w.cfg.Name); err != nil {
				return fmt.Errorf("join %s: %w", topic, err)
			}
			w.joined[topic] = true
		}
		return w.cfg.Pub.Publish(ctx, topic, payload)
	})
}

// WotanSubscriber is the part of pkg/wotan-client's Client (and its mock)
// that WotanClientPublisher needs.
type WotanSubscriber interface {
	Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error)
	Publish(ctx context.Context, topic string, payload []byte) error
}

// WotanClientPublisher adapts a Wotan client to TopicPublisher.
type WotanClientPublisher struct{ C WotanSubscriber }

// Join implements TopicPublisher. A pending subscription is not an error:
// the client re-asks Wotan on later publishes (wotan-client's recheck).
func (p WotanClientPublisher) Join(ctx context.Context, topic, displayName string) error {
	if p.C == nil {
		return errors.New("claudebridge: no Wotan client")
	}
	_, err := p.C.Subscribe(ctx, topic, displayName)
	return err
}

// Publish implements TopicPublisher.
func (p WotanClientPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	if p.C == nil {
		return errors.New("claudebridge: no Wotan client")
	}
	return p.C.Publish(ctx, topic, payload)
}
