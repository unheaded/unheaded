// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	chatpb "unheaded/services/wotan/proto"
)

// switchWotan is a Wotan whose gRPC side can be refused and accepted again
// on the same address, over one message log that HTTP polls read too
// (ADR-147 Phase 6: a stream returns to gRPC when the probe answers).
type switchWotan struct {
	chatpb.UnimplementedTopicStreamServer

	t        *testing.T
	grpcAddr string
	httpAddr string

	mu    sync.Mutex
	msgs  []*chatpb.TopicEvent
	wake  chan struct{} // closed and replaced on every publish
	gs    *grpc.Server
	polls map[string]int
}

func newSwitchWotan(t *testing.T) *switchWotan {
	t.Helper()
	w := &switchWotan{t: t, grpcAddr: refusedAddr(t), wake: make(chan struct{}), polls: map[string]int{}}
	hs := httptest.NewServer(http.HandlerFunc(w.serveHTTP))
	t.Cleanup(hs.Close)
	w.httpAddr = strings.TrimPrefix(hs.URL, "http://")
	w.accept()
	t.Cleanup(w.refuse)
	return w
}

// accept starts gRPC (topic streams and grpc.health.v1) on grpcAddr.
func (w *switchWotan) accept() {
	w.t.Helper()
	var lis net.Listener
	var err error
	for i := 0; i < 50; i++ { // the port may linger a moment after refuse
		if lis, err = net.Listen("tcp", w.grpcAddr); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		w.t.Fatalf("listen %s: %v", w.grpcAddr, err)
	}
	gs := grpc.NewServer()
	chatpb.RegisterTopicStreamServer(gs, w)
	healthpb.RegisterHealthServer(gs, health.NewServer())
	go func() { _ = gs.Serve(lis) }()
	w.mu.Lock()
	w.gs = gs
	w.mu.Unlock()
}

// refuse stops gRPC: open streams end and new connections are refused.
func (w *switchWotan) refuse() {
	w.mu.Lock()
	gs := w.gs
	w.gs = nil
	w.mu.Unlock()
	if gs != nil {
		gs.Stop()
	}
}

func (w *switchWotan) publish(topic string, seq int64) {
	w.mu.Lock()
	w.msgs = append(w.msgs, &chatpb.TopicEvent{Topic: topic, Seq: seq, MessageId: strconv.FormatInt(seq, 10), Payload: []byte("m")})
	close(w.wake)
	w.wake = make(chan struct{})
	w.mu.Unlock()
}

func (w *switchWotan) after(topic string, since int64) ([]*chatpb.TopicEvent, chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []*chatpb.TopicEvent
	for _, m := range w.msgs {
		if m.Topic == topic && m.Seq > since {
			out = append(out, m)
		}
	}
	return out, w.wake
}

func (w *switchWotan) StreamTopics(req *chatpb.TopicStreamRequest, stream chatpb.TopicStream_StreamTopicsServer) error {
	since := req.SinceSeq
	for {
		msgs, wake := w.after(req.TopicPattern, since)
		for _, m := range msgs {
			if err := stream.Send(m); err != nil {
				return err
			}
			since = m.Seq
		}
		select {
		case <-wake:
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

func (w *switchWotan) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/messages") {
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	topic := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/topics/"), "/messages")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after_seq"), 10, 64)
	w.mu.Lock()
	w.polls[topic]++
	var last int64
	for _, m := range w.msgs {
		last = max(last, m.Seq)
	}
	w.mu.Unlock()
	evs, _ := w.after(topic, after)
	out := make([]map[string]any, 0, len(evs))
	for _, m := range evs {
		out = append(out, map[string]any{"seq": m.Seq, "topic": m.Topic, "payload": string(m.Payload), "message_id": m.MessageId})
	}
	_ = json.NewEncoder(rw).Encode(map[string]any{"messages": out, "last_seq": last})
}

// fastStreams makes the client's gRPC reconnect budget small, so an outage
// falls back to polling in about a second instead of eleven.
func fastStreams(t *testing.T, c *Client) {
	t.Helper()
	gc, err := c.getOrCreateGRPCClient()
	if err != nil {
		t.Fatal(err)
	}
	gc.maxRetries = 2
	gc.retryDelay = 20 * time.Millisecond
}

// seriesValue reads one series from a family's exposition text: its value
// and whether it is there at all.
func seriesValue(t *testing.T, family interface{ Write(io.Writer) error }, name, labels string) (float64, bool) {
	t.Helper()
	var buf bytes.Buffer
	if err := family.Write(&buf); err != nil {
		t.Fatal(err)
	}
	prefix := name + "{" + labels + "} "
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, prefix) {
			v, err := strconv.ParseFloat(strings.TrimPrefix(line, prefix), 64)
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			return v, true
		}
	}
	return 0, false
}

func pollCount(t *testing.T, topic string) float64 {
	t.Helper()
	v, _ := seriesValue(t, wotanClientPolls, "unheaded_wotan_client_polls_total", `topic="`+topic+`"`)
	return v
}

func recvSeqs(t *testing.T, ch <-chan *Message, n int, within time.Duration) []int64 {
	t.Helper()
	var got []int64
	deadline := time.After(within)
	for len(got) < n {
		select {
		case m, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed after %v", got)
			}
			got = append(got, m.Seq)
		case <-deadline:
			t.Fatalf("got %v, want %d messages", got, n)
		}
	}
	return got
}

func returnScenario(t *testing.T, topic string) (*switchWotan, *Client, <-chan *Message) {
	t.Helper()
	w := newSwitchWotan(t)
	c, err := newClientWithGRPC(w.httpAddr, w.grpcAddr, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	fastStreams(t, c)
	c.subscribers[topic] = &Subscriber{SubscriberID: "s", DisplayName: "timeguru", Status: "approved"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	ch, err := c.StreamMessages(ctx, topic)
	if err != nil {
		t.Fatal(err)
	}
	return w, c, ch
}

func TestStreamReturnsToGRPCAfterOutage(t *testing.T) {
	const topic = "test.return"
	w, c, ch := returnScenario(t, topic)
	var got []int64
	for s := int64(1); s <= 5; s++ {
		w.publish(topic, s)
	}
	got = append(got, recvSeqs(t, ch, 5, 5*time.Second)...)

	before := pollCount(t, topic)
	w.refuse()
	if !reprobeWait(t, 10*time.Second, func() bool { return pollCount(t, topic) > before }) {
		t.Fatal("the client never polled while gRPC was refused")
	}
	for s := int64(6); s <= 10; s++ {
		w.publish(topic, s)
	}
	got = append(got, recvSeqs(t, ch, 5, 5*time.Second)...)

	w.accept()
	// Within 10 re-probe intervals the poller stops: the counter holds across
	// four poll intervals.
	stopped := reprobeWait(t, 10*50*time.Millisecond+5*time.Second, func() bool {
		p := pollCount(t, topic)
		time.Sleep(4 * pollBackoffMin)
		return pollCount(t, topic) == p
	})
	if !stopped {
		t.Fatal("still polling after gRPC came back")
	}
	for s := int64(11); s <= 15; s++ {
		w.publish(topic, s)
	}
	got = append(got, recvSeqs(t, ch, 5, 5*time.Second)...)
	for i, s := range got {
		if s != int64(i+1) {
			t.Fatalf("delivered %v, want 1..15 once, in order", got)
		}
	}
	if h := c.Health(); h.StreamTransport[topic] != "grpc" || h.Transport != "gRPC" {
		t.Fatalf("after the return: transport %q, stream %q", h.Transport, h.StreamTransport[topic])
	}
}

func TestHealthNamesPollingStream(t *testing.T) {
	const topic = "test.health"
	w, c, ch := returnScenario(t, topic)
	w.publish(topic, 1)
	recvSeqs(t, ch, 1, 5*time.Second)
	if h := c.Health(); h.StreamTransport[topic] != "grpc" || h.Transport != "gRPC" {
		t.Fatalf("before the outage: transport %q, stream %q", h.Transport, h.StreamTransport[topic])
	}
	w.refuse()
	if !reprobeWait(t, 10*time.Second, func() bool { return c.Health().StreamTransport[topic] == "http-poll" }) {
		t.Fatalf("during the outage the stream says %q", c.Health().StreamTransport[topic])
	}
	if h := c.Health(); h.Transport != "HTTP" {
		t.Fatalf("during the outage transport %q, want HTTP", h.Transport)
	}
	w.accept()
	if !reprobeWait(t, 10*time.Second, func() bool { return c.Health().StreamTransport[topic] == "grpc" }) {
		t.Fatalf("after the return the stream says %q", c.Health().StreamTransport[topic])
	}
	if h := c.Health(); h.Transport != "gRPC" {
		t.Fatalf("after the return transport %q, want gRPC", h.Transport)
	}
}

func TestStreamWithoutGRPCRefused(t *testing.T) {
	w := newSwitchWotan(t)
	c, err := NewClient(w.httpAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.subscribers["t"] = &Subscriber{SubscriberID: "s", DisplayName: "x", Status: "approved"}
	if _, err := c.StreamMessages(context.Background(), "t"); !errors.Is(err, ErrNoStreamTransport) {
		t.Fatalf("StreamMessages without gRPC = %v, want ErrNoStreamTransport", err)
	}

	p, err := NewClient(w.httpAddr, WithHTTPPollStreams())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	p.subscribers["t"] = &Subscriber{SubscriberID: "s", DisplayName: "x", Status: "approved"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := p.StreamMessages(ctx, "t")
	if err != nil {
		t.Fatalf("WithHTTPPollStreams: %v", err)
	}
	w.publish("t", 1)
	recvSeqs(t, ch, 1, 3*time.Second)
	if got := p.Health().StreamTransport["t"]; got != "http-poll" {
		t.Fatalf("a poll stream says %q", got)
	}
}

func TestPollCounterCountsEachPoll(t *testing.T) {
	w := newSwitchWotan(t)
	p, err := NewClient(w.httpAddr, WithHTTPPollStreams())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	const topic = "test.pollcount"
	p.subscribers[topic] = &Subscriber{SubscriberID: "s", DisplayName: "x", Status: "approved"}
	before := pollCount(t, topic)
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := p.StreamMessages(ctx, topic); err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * pollBackoffMin)
	cancel()
	time.Sleep(2 * pollBackoffMin) // let the poller see ctx
	w.mu.Lock()
	served := w.polls[topic]
	w.mu.Unlock()
	if served == 0 {
		t.Fatal("no poll reached the server")
	}
	if got := pollCount(t, topic) - before; got != float64(served) {
		t.Fatalf("counter rose by %v, server answered %d polls", got, served)
	}
}

func TestStreamTransportGauge(t *testing.T) {
	const topic = "test.gauge"
	w, _, ch := returnScenario(t, topic)
	w.publish(topic, 1)
	recvSeqs(t, ch, 1, 5*time.Second)
	gauge := func(transport string) (float64, bool) {
		return seriesValue(t, wotanClientStreamTransport, "unheaded_wotan_client_stream_transport", `topic="`+topic+`",transport="`+transport+`"`)
	}
	if v, ok := gauge("grpc"); !ok || v != 1 {
		t.Fatalf("grpc gauge %v (present %v), want 1", v, ok)
	}
	if _, ok := gauge("http-poll"); ok {
		t.Fatal("http-poll gauge present while the stream is gRPC")
	}
	w.refuse()
	if !reprobeWait(t, 10*time.Second, func() bool { _, ok := gauge("http-poll"); return ok }) {
		t.Fatal("no http-poll gauge during the outage")
	}
	if _, ok := gauge("grpc"); ok {
		t.Fatal("grpc gauge still present while the stream polls")
	}
}
