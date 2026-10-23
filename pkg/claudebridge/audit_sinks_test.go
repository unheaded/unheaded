// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/audit/storage"
	"unheaded/pkg/wotan-client/mock"
)

const secretMarker = "S2B-SECRET-MARKER"

// recordingAuditor keeps everything it is given.
type recordingAuditor struct {
	mu    sync.Mutex
	reqs  []RequestRecord
	tools []ToolRecord
	delay time.Duration
	panic bool
}

func (a *recordingAuditor) Request(_ context.Context, r RequestRecord) {
	if a.panic {
		panic("sink exploded")
	}
	time.Sleep(a.delay)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reqs = append(a.reqs, r)
}

func (a *recordingAuditor) ToolCall(_ context.Context, r ToolRecord) {
	if a.panic {
		panic("sink exploded")
	}
	time.Sleep(a.delay)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tools = append(a.tools, r)
}

func (a *recordingAuditor) counts() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.reqs), len(a.tools)
}

func TestMultiAuditor_FansOutDespiteSlowAndPanickingSinks(t *testing.T) {
	fast := &recordingAuditor{}
	slow := &recordingAuditor{delay: 300 * time.Millisecond}
	bad := &recordingAuditor{panic: true}
	m := MultiAuditor{bad, slow, fast}

	done := make(chan struct{})
	go func() {
		m.Request(context.Background(), RequestRecord{Conversation: "c"})
		m.ToolCall(context.Background(), ToolRecord{Conversation: "c", Name: "read_file"})
		close(done)
	}()
	// The fast sink sees the request before the slow one finishes.
	deadline := time.After(250 * time.Millisecond)
	for {
		if r, _ := fast.counts(); r == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("fast sink was held up by the slow one")
		case <-time.After(5 * time.Millisecond):
		}
	}
	<-done
	for name, a := range map[string]*recordingAuditor{"fast": fast, "slow": slow} {
		if r, tc := a.counts(); r != 1 || tc != 1 {
			t.Errorf("%s sink: %d requests, %d tool calls; want 1, 1", name, r, tc)
		}
	}
}

// fakeRequestSink records rows; it can fail or block.
type fakeRequestSink struct {
	mu    sync.Mutex
	rows  []LLMRequestRow
	err   error
	block chan struct{}
}

func (s *fakeRequestSink) InsertLLMRequest(ctx context.Context, r LLMRequestRow) error {
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.rows = append(s.rows, r)
	return nil
}

func (s *fakeRequestSink) all() []LLMRequestRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LLMRequestRow(nil), s.rows...)
}

func newTestWellAuditor(t *testing.T, rs RequestSink, es audit.Storage) *WellAuditor {
	t.Helper()
	w, err := NewWellAuditor(WellAuditorConfig{
		Source:   "zhen-claude",
		Backend:  BackendClaude,
		Requests: rs,
		Events:   es,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close(context.Background()) })
	return w
}

func TestWellAuditor_RequestBecomesOneRow(t *testing.T) {
	rs := &fakeRequestSink{}
	w := newTestWellAuditor(t, rs, storage.NewMemoryStorage())
	w.Request(context.Background(), RequestRecord{
		Conversation: "conv1", Iteration: 2, Model: "claude-opus-5-5", ServedBy: "claude-opus-5-5",
		StopReason: "end_turn", InputTokens: 231, OutputTokens: 4, CacheRead: 7, CacheWrite: 9,
		Reserved: 5000, Cost: 1004000, Duration: 2376 * time.Millisecond, Sent: true,
		TaskFamily: "syntax", RouteReason: "local_first", Redactions: 3,
		Err: strings.Repeat("e", 2000),
	})
	if err := w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows := rs.all()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.SourceService != "zhen-claude" || r.Backend != "claude" || r.ModelRequested != "claude-opus-5-5" ||
		r.ConversationID != "conv1" || r.Iteration != 2 || r.InputTokens != 231 || r.OutputTokens != 4 ||
		r.CacheRead != 7 || r.CacheWrite != 9 || r.CostNanoUSD != 1004000 || r.ReservedNanoUSD != 5000 ||
		r.DurationMS != 2376 || !r.Sent || r.TaskFamily != "syntax" || r.RouteReason != "local_first" || r.Redactions != 3 {
		t.Errorf("row = %+v", r)
	}
	if len(r.Error) > maxAuditReasonBytes {
		t.Errorf("error is %d bytes, want <= %d", len(r.Error), maxAuditReasonBytes)
	}
	if r.OccurredAt.IsZero() {
		t.Error("occurred_at not set")
	}
}

func TestWellAuditor_UnknownRouteReasonIsBlanked(t *testing.T) {
	rs := &fakeRequestSink{}
	w := newTestWellAuditor(t, rs, storage.NewMemoryStorage())
	w.Request(context.Background(), RequestRecord{Conversation: "c", Model: "m", RouteReason: "because'; DROP TABLE x"})
	_ = w.Close(context.Background())
	if got := rs.all()[0].RouteReason; got != "" {
		t.Errorf("route_reason = %q, want blank (the table's CHECK would refuse it)", got)
	}
}

func TestWellAuditor_ToolCallBecomesOneAuditEvent(t *testing.T) {
	es := storage.NewMemoryStorage()
	w := newTestWellAuditor(t, &fakeRequestSink{}, es)
	ctx := context.Background()
	w.ToolCall(ctx, ToolRecord{Conversation: "conv1", Model: "claude-opus-5-5", ToolUseID: "tu1", Name: "read_file", Provider: "champion",
		Decision: "allowed", InputBytes: 40, InputSHA256: strings.Repeat("a", 64), OutputBytes: 99})
	w.ToolCall(ctx, ToolRecord{Conversation: "conv1", Model: "claude-opus-5-5", ToolUseID: "tu2", Name: "write_file", Provider: "champion",
		Decision: "refused", Pending: true, IsError: true, Reason: "mutating tool needs confirmation"})
	if err := w.Close(ctx); err != nil {
		t.Fatal(err)
	}
	evs, err := es.Query(ctx, &audit.AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("events = %d, want 2", len(evs))
	}
	byTool := map[string]*audit.AuditEvent{}
	for _, e := range evs {
		byTool[e.Resource.ID] = e
	}
	ok, pend := byTool["read_file"], byTool["write_file"]
	if ok == nil || pend == nil {
		t.Fatalf("missing events: %+v", byTool)
	}
	if ok.Type != audit.EventTypeAuthorization || ok.Action != "tool_call" || ok.Outcome != audit.OutcomeSuccess {
		t.Errorf("allowed event = %+v", ok)
	}
	if pend.Outcome != audit.OutcomePending {
		t.Errorf("pending event outcome = %s", pend.Outcome)
	}
	if ok.Actor.ID != "claude-api/claude-opus-5-5" || ok.Actor.Type != "model" {
		t.Errorf("actor = %+v", ok.Actor)
	}
	if ok.Resource.Type != "tool" || ok.Source.Service != "zhen-claude" || ok.Source.RequestID != "conv1" {
		t.Errorf("resource/source = %+v %+v", ok.Resource, ok.Source)
	}
	if ok.Details["input_sha256"] != strings.Repeat("a", 64) || ok.Details["provider"] != "champion" {
		t.Errorf("details = %v", ok.Details)
	}
	// The chain: each event hashes over the previous one.
	for _, e := range evs {
		if !e.VerifyHash() {
			t.Errorf("event %s hash does not verify", e.ID)
		}
	}
	first, second := evs[0], evs[1]
	if first.Timestamp.After(second.Timestamp) {
		first, second = second, first
	}
	if second.PreviousHash != first.Hash {
		t.Errorf("chain broken: second.previous_hash=%q first.hash=%q", second.PreviousHash, first.Hash)
	}
}

func TestWellAuditor_NoInputTextReachesAnySink(t *testing.T) {
	rs := &fakeRequestSink{}
	es := storage.NewMemoryStorage()
	pub := &fakePublisher{}
	w := newTestWellAuditor(t, rs, es)
	wa := newTestWotanAuditor(t, pub)
	m := MultiAuditor{w, wa}
	ctx := context.Background()
	input := `{"path":"/x","content":"` + secretMarker + `"}`
	m.ToolCall(ctx, ToolRecord{Conversation: "c", Model: "m", Name: "write_file", Provider: "champion", Decision: "refused",
		InputBytes: len(input), InputSHA256: sha256Hex([]byte(input)), Reason: "refused"})
	m.Request(ctx, RequestRecord{Conversation: "c", Model: "m", Sent: true})
	_ = w.Close(ctx)
	_ = wa.Close(ctx)

	var blob strings.Builder
	for _, r := range rs.all() {
		b, _ := json.Marshal(r)
		blob.Write(b)
	}
	evs, _ := es.Query(ctx, &audit.AuditQuery{})
	for _, e := range evs {
		b, _ := json.Marshal(e)
		blob.Write(b)
	}
	for _, p := range pub.all() {
		blob.Write(p.payload)
	}
	if strings.Contains(blob.String(), secretMarker) {
		t.Fatal("a sink recorded tool input text")
	}
	if !strings.Contains(blob.String(), sha256Hex([]byte(input))) {
		t.Fatal("the input's SHA-256 is missing")
	}
}

func TestWellAuditor_FullQueueDropsAndCounts(t *testing.T) {
	rs := &fakeRequestSink{block: make(chan struct{})}
	w, err := NewWellAuditor(WellAuditorConfig{Source: "zhen-claude", Backend: BackendClaude, Requests: rs, Events: storage.NewMemoryStorage(), QueueSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		w.Request(ctx, RequestRecord{Conversation: "c", Model: "m"})
	}
	if w.Dropped() == 0 {
		t.Fatal("a full queue dropped nothing: Request must never block the caller")
	}
	close(rs.block)
	_ = w.Close(ctx)
}

func TestWellAuditor_SinkErrorsAreCountedNotFatal(t *testing.T) {
	rs := &fakeRequestSink{err: errors.New("well down")}
	w := newTestWellAuditor(t, rs, storage.NewMemoryStorage())
	w.Request(context.Background(), RequestRecord{Conversation: "c", Model: "m"})
	_ = w.Close(context.Background())
	if w.Errors() != 1 {
		t.Errorf("errors = %d, want 1", w.Errors())
	}
}

func TestNewWellAuditor_Validates(t *testing.T) {
	for name, cfg := range map[string]WellAuditorConfig{
		"no sinks":       {Source: "zhen-claude", Backend: BackendClaude},
		"bad source":     {Source: "mallory", Backend: BackendClaude, Requests: &fakeRequestSink{}},
		"bad backend":    {Source: "zhen-claude", Backend: "gpt", Requests: &fakeRequestSink{}},
		"negative queue": {Source: "zhen-claude", Backend: BackendClaude, Requests: &fakeRequestSink{}, QueueSize: -1},
	} {
		if _, err := NewWellAuditor(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestToolResourceID_Validated(t *testing.T) {
	for in, want := range map[string]string{
		"read_file":                   "read_file",
		"mcp.corpus_search":           "mcp.corpus_search",
		"Bad Name; DROP":              "invalid",
		strings.Repeat("a", 65):       "invalid",
		"":                            "invalid",
		"write_file\n" + secretMarker: "invalid",
	} {
		if got := toolResourceID(in); got != want {
			t.Errorf("toolResourceID(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakePublisher records joins and publishes.
type fakePublisher struct {
	mu       sync.Mutex
	joined   map[string]bool
	msgs     []published
	joinErr  error
	pubErr   error
	joinCall int
}

type published struct {
	topic   string
	payload []byte
}

func (p *fakePublisher) Join(_ context.Context, topic, name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.joinCall++
	if p.joinErr != nil {
		return p.joinErr
	}
	if name == "" {
		return errors.New("no name")
	}
	if p.joined == nil {
		p.joined = map[string]bool{}
	}
	p.joined[topic] = true
	return nil
}

func (p *fakePublisher) Publish(_ context.Context, topic string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.joined[topic] {
		return errors.New("not subscribed to topic " + topic)
	}
	if p.pubErr != nil {
		return p.pubErr
	}
	p.msgs = append(p.msgs, published{topic, append([]byte(nil), payload...)})
	return nil
}

func (p *fakePublisher) all() []published {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]published(nil), p.msgs...)
}

func newTestWotanAuditor(t *testing.T, pub TopicPublisher) *WotanAuditor {
	t.Helper()
	w, err := NewWotanAuditor(WotanAuditorConfig{Pub: pub, Name: "zhen-claude", Backend: BackendClaude})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close(context.Background()) })
	return w
}

func TestWotanAuditor_JoinsThenPublishesBothTopics(t *testing.T) {
	pub := &fakePublisher{}
	w := newTestWotanAuditor(t, pub)
	ctx := context.Background()
	w.Request(ctx, RequestRecord{Conversation: "c", Model: "m", Cost: 10, Sent: true})
	w.ToolCall(ctx, ToolRecord{Conversation: "c", Model: "m", Name: "read_file", Decision: "allowed", Reason: strings.Repeat("r", 8000)})
	_ = w.Close(ctx)
	msgs := pub.all()
	if len(msgs) != 2 {
		t.Fatalf("published %d messages, want 2 (joined before publishing?)", len(msgs))
	}
	topics := map[string]bool{}
	for _, m := range msgs {
		topics[m.topic] = true
		if len(m.payload) > MaxWotanPayloadBytes {
			t.Errorf("%s payload %d bytes > %d", m.topic, len(m.payload), MaxWotanPayloadBytes)
		}
		var obj map[string]any
		if err := json.Unmarshal(m.payload, &obj); err != nil {
			t.Errorf("%s payload is not JSON: %v", m.topic, err)
		}
	}
	if !topics[TopicLLMRequests] || !topics[TopicToolCalls] {
		t.Errorf("topics = %v", topics)
	}
}

func TestWotanAuditor_WotanDownIsCountedNotFatal(t *testing.T) {
	pub := &fakePublisher{joinErr: errors.New("connection refused")}
	w := newTestWotanAuditor(t, pub)
	w.Request(context.Background(), RequestRecord{Conversation: "c", Model: "m"})
	_ = w.Close(context.Background())
	if w.Errors() == 0 {
		t.Error("a failed join was not counted")
	}
}

func TestNewWotanAuditor_Validates(t *testing.T) {
	if _, err := NewWotanAuditor(WotanAuditorConfig{Name: "zhen-claude", Backend: BackendClaude}); err == nil {
		t.Error("nil publisher accepted")
	}
	if _, err := NewWotanAuditor(WotanAuditorConfig{Pub: &fakePublisher{}, Name: "mallory", Backend: BackendClaude}); err == nil {
		t.Error("name outside the allowlisted publishers accepted")
	}
}

func TestWotanClientPublisher_WithMockClient(t *testing.T) {
	mc := mock.NewMockClient(mock.WithAutoApprove())
	w := newTestWotanAuditor(t, WotanClientPublisher{C: mc})
	w.Request(context.Background(), RequestRecord{Conversation: "c", Model: "m"})
	if err := w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w.Errors() != 0 {
		t.Fatalf("errors = %d", w.Errors())
	}
	if err := (WotanClientPublisher{}).Publish(context.Background(), "t", []byte("x")); err == nil {
		t.Error("nil client published")
	}
	if err := (WotanClientPublisher{}).Join(context.Background(), "t", "n"); err == nil {
		t.Error("nil client joined")
	}
}

func TestConfirmationEvents(t *testing.T) {
	es := storage.NewMemoryStorage()
	w := newTestWellAuditor(t, &fakeRequestSink{}, es)
	pub := &fakePublisher{}
	wa := newTestWotanAuditor(t, pub)
	m := MultiAuditor{SlogAuditor{Log: discardLog()}, w, wa, &recordingAuditor{}}
	ctx := context.Background()
	m.Confirmation(ctx, ConfirmRecord{Conversation: "c", Model: "m", Tool: "write_file", Action: "confirm_issue", QueueID: "0123456789abcdef", CallSHA256: strings.Repeat("b", 64)})
	m.Confirmation(ctx, ConfirmRecord{Conversation: "c", Tool: "write_file", Action: "confirm_approve", DecidedBy: "stevie@west"})
	m.Confirmation(ctx, ConfirmRecord{Conversation: "c", Tool: "write_file", Action: "made_up"})
	_ = w.Close(ctx)
	_ = wa.Close(ctx)
	evs, _ := es.Query(ctx, &audit.AuditQuery{})
	if len(evs) != 3 {
		t.Fatalf("events = %d", len(evs))
	}
	got := map[string]*audit.AuditEvent{}
	for _, e := range evs {
		got[e.Action+"/"+e.Actor.Type] = e
	}
	if e := got["confirm_approve/human"]; e == nil || e.Actor.ID != "stevie@west" || e.Outcome != audit.OutcomeSuccess {
		t.Errorf("approve event = %+v", e)
	}
	if e := got["confirm_issue/model"]; e == nil || e.Outcome != audit.OutcomePending {
		t.Errorf("issue events = %+v", got)
	}
	if len(pub.all()) != 3 {
		t.Errorf("published %d", len(pub.all()))
	}
}

func discardLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }
