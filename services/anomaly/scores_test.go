// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package anomaly

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func validScore() ScoreMessage {
	return ScoreMessage{
		FlowKey: FlowKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 41001, DstPort: 80, Proto: 6},
		Score:   99, ModelID: 0xd0ee2b6b, Source: "cartridge:2", TimestampMS: 1_700_000_000_000,
	}
}

func TestIngestScore_RecordsAndSurfaces(t *testing.T) {
	t.Parallel()
	s := newTestService()
	b, err := json.Marshal(validScore())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.IngestScore(b); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/top", nil))
	var top struct {
		Flows []FlowScore `json:"flows"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &top); err != nil {
		t.Fatal(err)
	}
	if top.Count != 1 || top.Flows[0].AnomalyScore != 99 || top.Flows[0].FlowKey.DstPort != 80 || top.Flows[0].ServiceID != "cartridge:2" {
		t.Fatalf("top = %+v", top)
	}
	// Above 80: an alert, as for any recorded score.
	if len(s.GetAlerts()) != 1 {
		t.Fatalf("alerts = %d", len(s.GetAlerts()))
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "anomaly_scores_ingested_total 1") {
		t.Fatalf("metrics:\n%s", rec.Body.String())
	}
}

func TestIngestScore_RejectsHostileInput(t *testing.T) {
	t.Parallel()
	s := newTestService()
	mut := func(f func(*ScoreMessage)) []byte {
		m := validScore()
		f(&m)
		b, _ := json.Marshal(m)
		return b
	}
	cases := map[string][]byte{
		"empty":         nil,
		"not json":      []byte("{"),
		"trailing data": append(mut(func(*ScoreMessage) {}), []byte(" {}")...),
		"unknown field": []byte(`{"flow_key":{"src_ip":"10.0.0.1","dst_ip":"10.0.0.2","src_port":1,"dst_port":2,"proto":6},"score":5,"payload":"AAAA"}`),
		"score > 100":   mut(func(m *ScoreMessage) { m.Score = 101 }),
		"score < 0":     mut(func(m *ScoreMessage) { m.Score = -1 }),
		"bad src ip":    mut(func(m *ScoreMessage) { m.FlowKey.SrcIP = "not-an-ip" }),
		"bad dst ip":    mut(func(m *ScoreMessage) { m.FlowKey.DstIP = "" }),
		"long source":   mut(func(m *ScoreMessage) { m.Source = strings.Repeat("x", 65) }),
		"oversize":      append([]byte(`{"source":"`), append([]byte(strings.Repeat("x", MaxScoreMessage)), []byte(`"}`)...)...),
	}
	for name, b := range cases {
		if err := s.IngestScore(b); !errors.Is(err, ErrBadScore) {
			t.Errorf("%s: err = %v, want ErrBadScore", name, err)
		}
	}
	if n := len(s.GetFlowScores("")); n != 0 {
		t.Fatalf("%d scores recorded from hostile input", n)
	}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "anomaly_scores_rejected_total 10") {
		t.Fatalf("metrics:\n%s", rec.Body.String())
	}
}

func TestIngestScore_IPv6AndConcurrent(t *testing.T) {
	t.Parallel()
	s := newTestService()
	m := validScore()
	m.FlowKey.SrcIP, m.FlowKey.DstIP = "fd00::1", "fd00::2"
	b, _ := json.Marshal(m)
	done := make(chan error, 50)
	for i := 0; i < 50; i++ {
		go func() { done <- s.IngestScore(b) }()
	}
	for i := 0; i < 50; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if n := len(s.GetFlowScores("")); n != 1 {
		t.Fatalf("one flow expected, got %d", n)
	}
}
