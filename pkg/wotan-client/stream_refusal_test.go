// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatpb "unheaded/services/wotan/proto"
)

// deniedWotan refuses one topic's stream with PermissionDenied and serves
// the rest.
type deniedWotan struct {
	*switchWotan
	denied string
	mu     sync.Mutex
	tries  int
}

func (d *deniedWotan) StreamTopics(req *chatpb.TopicStreamRequest, stream chatpb.TopicStream_StreamTopicsServer) error {
	if req.TopicPattern == d.denied {
		d.mu.Lock()
		d.tries++
		d.mu.Unlock()
		return status.Error(codes.PermissionDenied, "topics.permissions: not allowed to read "+d.denied)
	}
	return d.switchWotan.StreamTopics(req, stream)
}

func (d *deniedWotan) attempts() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.tries
}

// REV-9: a per-topic refusal (PermissionDenied) is not a transport
// failure. It is counted and logged for that topic and retried later on
// gRPC; the client stays on gRPC, so the other topics do not flap to HTTP
// polling and back on every re-probe.
func TestTopicRefusalDoesNotDegradeTransport(t *testing.T) {
	const ok, denied = "test.allowed", "test.denied"
	w := &switchWotan{t: t, grpcAddr: refusedAddr(t), wake: make(chan struct{}), polls: map[string]int{}}
	d := &deniedWotan{switchWotan: w, denied: denied}
	w.server = d
	hs := httptest.NewServer(http.HandlerFunc(w.serveHTTP))
	t.Cleanup(hs.Close)
	w.httpAddr = strings.TrimPrefix(hs.URL, "http://")
	w.accept()
	t.Cleanup(w.refuse)

	c, err := newClientWithGRPC(w.httpAddr, w.grpcAddr, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	fastStreams(t, c)
	gc, _ := c.getOrCreateGRPCClient()
	gc.refusedDelay = 20 * time.Millisecond
	for _, topic := range []string{ok, denied} {
		c.subscribers[topic] = &Subscriber{SubscriberID: "s", DisplayName: "timeguru", Status: "approved"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	okCh, err := c.StreamMessages(ctx, ok)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.StreamMessages(ctx, denied); err != nil {
		t.Fatal(err)
	}
	refusedBefore, _ := seriesValue(t, wotanClientStreamRefused, "unheaded_wotan_client_stream_refused_total", `code="PermissionDenied",topic="`+denied+`"`)

	// Several refusals, retried on gRPC.
	if !reprobeWait(t, 10*time.Second, func() bool { return d.attempts() >= 3 }) {
		t.Fatalf("the refused topic was tried %d times on gRPC, want it retried", d.attempts())
	}
	if !c.IsGRPCHealthy() {
		t.Fatal("a per-topic refusal degraded the whole client to HTTP")
	}
	if p := pollCount(t, ok); p != 0 {
		t.Fatalf("the allowed topic polled HTTP %v times", p)
	}
	if p := pollCount(t, denied); p != 0 {
		t.Fatalf("the refused topic polled HTTP %v times (HTTP refuses it too)", p)
	}
	w.publish(ok, 1)
	if got := recvSeqs(t, okCh, 1, 5*time.Second); got[0] != 1 {
		t.Fatalf("allowed topic got %v", got)
	}
	if h := c.Health(); h.Transport != "gRPC" || h.StreamTransport[ok] != "grpc" {
		t.Fatalf("Health: transport %q, %s on %q", h.Transport, ok, h.StreamTransport[ok])
	}
	after, _ := seriesValue(t, wotanClientStreamRefused, "unheaded_wotan_client_stream_refused_total", `code="PermissionDenied",topic="`+denied+`"`)
	if after-refusedBefore < 3 {
		t.Fatalf("unheaded_wotan_client_stream_refused_total rose %v, want at least 3", after-refusedBefore)
	}
}

func TestIsTopicRefusal(t *testing.T) {
	for code, want := range map[codes.Code]bool{
		codes.PermissionDenied: true, codes.Unauthenticated: true, codes.NotFound: true,
		codes.InvalidArgument: true, codes.FailedPrecondition: true,
		codes.Unavailable: false, codes.ResourceExhausted: false, codes.DeadlineExceeded: false, codes.Internal: false,
	} {
		if got := isTopicRefusal(status.Error(code, "x")); got != want {
			t.Errorf("isTopicRefusal(%v) = %v, want %v", code, got, want)
		}
	}
	if isTopicRefusal(nil) || isTopicRefusal(context.Canceled) {
		t.Error("nil or a context error is not a refusal")
	}
}
