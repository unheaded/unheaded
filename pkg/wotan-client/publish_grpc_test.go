// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	chatpb "unheaded/services/wotan/proto"
)

// pubWotan records publishes on both sides: gRPC PublishTopic (on a
// switchWotan) and the HTTP join and publish.
type pubWotan struct {
	*switchWotan
	mu        sync.Mutex
	grpcPubs  []string
	httpJoins []string
	httpPubs  []string
}

func (p *pubWotan) PublishTopic(_ context.Context, req *chatpb.TopicPublishRequest) (*chatpb.TopicPublishResponse, error) {
	p.mu.Lock()
	p.grpcPubs = append(p.grpcPubs, req.Topic+"="+string(req.Payload))
	p.mu.Unlock()
	return &chatpb.TopicPublishResponse{}, nil
}

func (p *pubWotan) counts() (grpcN, joins, httpN int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.grpcPubs), len(p.httpJoins), len(p.httpPubs)
}

func newPubWotan(t *testing.T) *pubWotan {
	t.Helper()
	p := &pubWotan{}
	w := &switchWotan{t: t, grpcAddr: refusedAddr(t), wake: make(chan struct{}), polls: map[string]int{}}
	p.switchWotan = w
	w.server = p
	hs := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/topics/")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/subscribe"):
			p.mu.Lock()
			p.httpJoins = append(p.httpJoins, strings.TrimSuffix(path, "/subscribe"))
			p.mu.Unlock()
			rw.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(rw).Encode(map[string]any{"subscriber": map[string]any{"subscriber_id": "s1", "status": "approved", "display_name": "trace-collector"}})
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/publish"):
			p.mu.Lock()
			p.httpPubs = append(p.httpPubs, strings.TrimSuffix(path, "/publish"))
			p.mu.Unlock()
			rw.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(rw).Encode(map[string]any{"message_id": "m", "seq": 1})
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(hs.Close)
	w.httpAddr = strings.TrimPrefix(hs.URL, "http://")
	w.accept()
	t.Cleanup(w.refuse)
	return p
}

// TestPublishPreferGRPC: the one client publishes over gRPC while gRPC is
// healthy (no HTTP join, no HTTP publish), and over HTTP, joining once,
// when it is not (ADR-147 Phase 7: the trace collector's second client was
// a TopicStreamClient kept only to publish over gRPC).
func TestPublishPreferGRPC(t *testing.T) {
	p := newPubWotan(t)
	c, err := newClientWithGRPC(p.httpAddr, p.grpcAddr, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !c.IsGRPCHealthy() {
		t.Fatal("gRPC not healthy with the fake accepting")
	}
	for i := 0; i < 3; i++ {
		if err := c.PublishPreferGRPC(ctx, "traces.flow", "trace-collector", []byte("x")); err != nil {
			t.Fatalf("publish over gRPC: %v", err)
		}
	}
	if g, j, h := p.counts(); g != 3 || j != 0 || h != 0 {
		t.Fatalf("gRPC healthy: grpc=%d joins=%d http=%d, want 3 0 0", g, j, h)
	}

	p.refuse()
	for i := 0; i < 3; i++ {
		_ = c.PublishPreferGRPC(ctx, "traces.flow", "trace-collector", []byte("y"))
	}
	if c.IsGRPCHealthy() {
		t.Error("a failed gRPC publish left the client on gRPC")
	}
	if _, j, h := p.counts(); j != 1 || h < 2 {
		t.Fatalf("gRPC refused: joins=%d http=%d, want 1 join and the publishes over HTTP", j, h)
	}
}

// TestPublishPreferGRPCWithoutGRPC: a client with no gRPC address
// publishes over HTTP.
func TestPublishPreferGRPCWithoutGRPC(t *testing.T) {
	p := newPubWotan(t)
	c, err := NewClient(p.httpAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if err := c.PublishPreferGRPC(context.Background(), "traces.flow", "trace-collector", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if g, j, h := p.counts(); g != 0 || j != 1 || h != 1 {
		t.Fatalf("grpc=%d joins=%d http=%d, want 0 1 1", g, j, h)
	}
}
