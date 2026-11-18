// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	ebpfPkg "unheaded/cmd/dashboard-backend/internal/ebpf"
	"unheaded/cmd/dashboard-backend/internal/events"
	wotanClient "unheaded/pkg/wotan-client"
)

// ADR-147 Phase 10: one message on traces.flow reaches both of the
// dashboard's views through the one subscription the ingestor holds: the
// flow graph counts one flow, the trace buffer holds one trace event, and
// the page gets one "ebpf_flow" and one "trace" frame, and no generic
// "event" frame for it (each event used to arrive three times: on the
// ebpf.flow.events mirror, and twice from traces.flow).
func TestTraceEventReachesBothConsumersOnce(t *testing.T) {
	wc := &chanWotan{chs: map[string]chan *wotanClient.Message{}}
	ing := ebpfPkg.NewIngestor(ebpfPkg.DefaultIngestorConfig(), wc, nil)

	cfg := DefaultConfig()
	cfg.WotanAddr = "localhost:19999"
	cfg.EBPFIngestor = ing
	srv, err := NewServer(cfg, testPipelineLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer scancel()
		srv.Shutdown(sctx)
	}()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}

	ws := srv.GetWebSocketServer()
	hs := httptest.NewServer(http.HandlerFunc(ws.HandleWebSocket))
	defer hs.Close()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(hs.URL, "http"), sameOriginHeader(hs.URL))
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for deadline := time.Now().Add(2 * time.Second); ws.ConnectionCount() == 0 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // the broadcasters register their listeners

	flow := `[{"timestamp_ns":1,"flow_key":{"src_addr":"10.9.0.1","dst_addr":"10.9.0.2","src_port":40000,` +
		`"dst_port":443,"protocol":6},"flow_state":{"state":"established"},"event_type":"new"}]`
	wc.send(t, "traces.flow", flow)

	counts := map[string]int{}
	for deadline := time.Now().Add(700 * time.Millisecond); time.Now().Before(deadline); {
		_ = conn.SetReadDeadline(deadline)
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg struct {
			Type string `json:"type"`
			Data struct {
				Topic string `json:"topic"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if msg.Type == "event" && msg.Data.Topic != "traces.flow" {
			continue
		}
		counts[msg.Type]++
	}
	if counts["ebpf_flow"] != 1 || counts["trace"] != 1 || counts["event"] != 0 {
		t.Errorf("frames %v, want one ebpf_flow, one trace and no traces.flow event", counts)
	}
	if n := ing.Stats().FlowsIngested; n != 1 {
		t.Errorf("flow graph ingested %d flows, want 1", n)
	}
	if n := srv.GetTraceBuffer().Count(); n != 1 {
		t.Errorf("trace buffer holds %d, want 1", n)
	}
}

// traces.packet carries trace_id as {high, low}; the trace view keeps every
// bit of it (through a map the halves would be float64s).
func TestTraceRecordsKeepTraceIDBits(t *testing.T) {
	raw := `[{"timestamp_ns":1,"trace_id":{"high":18446744073709551615,"low":1311768467463790321},` +
		`"flow_key":{"src_addr":"10.9.0.1","dst_addr":"10.9.0.2","src_port":1,"dst_port":2,"protocol":6},` +
		`"packet_len":60,"action":"pass","direction":"ingress"}]`
	srv := newTestServer(t)
	recs := traceRecords(events.Event{Topic: "traces.packet", Raw: json.RawMessage(raw)})
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	te := srv.parseTraceTopicEvent(events.Event{Topic: "traces.packet", Data: recs[0]})
	if te.TraceID != "ffffffffffffffff123456789abcdef1" {
		t.Errorf("TraceID = %q, want ffffffffffffffff123456789abcdef1", te.TraceID)
	}
	if te.Src != "10.9.0.1:1" || te.Dst != "10.9.0.2:2" || te.Protocol != "TCP" || te.PacketLen != 60 {
		t.Errorf("trace = %+v", te)
	}
}
