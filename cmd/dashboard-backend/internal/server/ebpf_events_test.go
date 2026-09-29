// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	ebpfPkg "unheaded/cmd/dashboard-backend/internal/ebpf"
	wotanClient "unheaded/pkg/wotan-client"
)

// chanWotan hands the ingestor one channel per topic.
type chanWotan struct {
	mu  sync.Mutex
	chs map[string]chan *wotanClient.Message
}

func (c *chanWotan) Subscribe(context.Context, string, string) (*wotanClient.Subscriber, error) {
	return &wotanClient.Subscriber{SubscriberID: "test", Status: "active"}, nil
}

func (c *chanWotan) StreamMessages(_ context.Context, topic string) (<-chan *wotanClient.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan *wotanClient.Message, 16)
	c.chs[topic] = ch
	return ch, nil
}

func (c *chanWotan) Close() error { return nil }

func (c *chanWotan) send(t *testing.T, topic, payload string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.mu.Lock()
		ch := c.chs[topic]
		c.mu.Unlock()
		if ch != nil {
			ch <- &wotanClient.Message{Topic: topic, Payload: payload}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ingestor never streamed %s", topic)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type eventsPage struct {
	Events []struct {
		Seq uint64 `json:"seq"`
	} `json:"events"`
	Count   int    `json:"count"`
	LastSeq uint64 `json:"last_seq"`
}

func getEvents(t *testing.T, srv *Server, query string) (int, eventsPage) {
	t.Helper()
	w := httptest.NewRecorder()
	srv.handleEBPFEvents(w, httptest.NewRequest(http.MethodGet, "/api/v1/ebpf/events"+query, nil))
	var p eventsPage
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatalf("decode %q: %v", w.Body.String(), err)
		}
	}
	return w.Code, p
}

func TestHandleEBPFEvents_AfterSeqPages(t *testing.T) {
	wc := &chanWotan{chs: map[string]chan *wotanClient.Message{}}
	ing := ebpfPkg.NewIngestor(ebpfPkg.DefaultIngestorConfig(), wc, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ing.Start(ctx); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t)
	srv.ebpfIngestor = ing

	pkt := `{"timestamp_ns":1,"trace_id":{"high":1,"low":2},"flow_key":{"src_addr":"10.0.0.1",` +
		`"dst_addr":"10.0.0.2","src_port":1,"dst_port":2,"protocol":6},"packet_len":60,` +
		`"action":"pass","direction":"ingress"}`
	for i := 0; i < 3; i++ {
		wc.send(t, "ebpf.packet.events", pkt)
	}
	deadline := time.Now().Add(2 * time.Second)
	for ing.Stats().PacketsIngested < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("ingested %d of 3", ing.Stats().PacketsIngested)
		}
		time.Sleep(5 * time.Millisecond)
	}

	code, p := getEvents(t, srv, "")
	if code != http.StatusOK || p.Count != 3 || p.LastSeq != 3 || p.Events[0].Seq != 3 {
		t.Fatalf("first poll = %d %+v, want 3 events newest seq 3, last_seq 3", code, p)
	}
	// The poll that used to re-send the same events: now nothing new.
	for i := 0; i < 2; i++ {
		if code, p = getEvents(t, srv, "?after_seq=3"); code != http.StatusOK || p.Count != 0 || p.LastSeq != 3 {
			t.Fatalf("caught-up poll = %d %+v, want 0 events, last_seq 3", code, p)
		}
	}
	if _, p = getEvents(t, srv, "?after_seq=1"); p.Count != 2 || p.Events[0].Seq != 3 || p.Events[1].Seq != 2 {
		t.Errorf("after_seq=1 = %+v, want seqs 3,2", p)
	}
	// A cursor from before a restart: last_seq tells the client to rewind.
	if _, p = getEvents(t, srv, "?after_seq=500"); p.Count != 0 || p.LastSeq != 3 {
		t.Errorf("after_seq=500 = %+v, want 0 events, last_seq 3", p)
	}
	for _, bad := range []string{"-1", "x", "1.5"} {
		if code, _ := getEvents(t, srv, "?after_seq="+bad); code != http.StatusBadRequest {
			t.Errorf("after_seq=%s status %d, want 400", bad, code)
		}
	}
}
