// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"unheaded/pkg/cartridge"
	"unheaded/pkg/logger"
	"unheaded/services/anomaly"
)

func scoreEvent(l3 uint8, score uint32) cartridge.Event {
	e := cartridge.Event{Kind: ScoreKind, A: score, B: 0xd0ee2b6b, Slot: 2, Stage: cartridge.StageTick}
	e.Flow.L3, e.Flow.Proto, e.Flow.Sport, e.Flow.Dport = l3, 17, 41002, 40000
	if l3 == 4 {
		copy(e.Flow.Src[:], []byte{10, 0, 0, 1})
		copy(e.Flow.Dst[:], []byte{10, 0, 0, 2})
	} else {
		e.Flow.Src[0], e.Flow.Src[15] = 0xfd, 1
		e.Flow.Dst[0], e.Flow.Dst[15] = 0xfd, 2
	}
	return e
}

func TestScoreFromEvent(t *testing.T) {
	m, ok := scoreFromEvent(scoreEvent(4, 99))
	if !ok || m.FlowKey.SrcIP != "10.0.0.1" || m.FlowKey.DstIP != "10.0.0.2" || m.FlowKey.DstPort != 40000 ||
		m.FlowKey.Proto != 17 || m.Score != 99 || m.ModelID != 0xd0ee2b6b || m.Source != "cartridge:2" {
		t.Fatalf("v4: %+v %v", m, ok)
	}
	m, ok = scoreFromEvent(scoreEvent(6, 3))
	if !ok || m.FlowKey.SrcIP != "fd00::1" || m.FlowKey.DstIP != "fd00::2" || m.Score != 3 {
		t.Fatalf("v6: %+v %v", m, ok)
	}
	skip := map[string]cartridge.Event{}
	e := scoreEvent(4, 50)
	e.Stage = cartridge.StageXDP
	skip["xdp-stage event"] = e
	e = scoreEvent(4, 50)
	e.Kind = 0x53 // sampler 'S'
	skip["sample event"] = e
	skip["score > 100"] = scoreEvent(4, 101)
	skip["no l3"] = scoreEvent(0, 50)
	for name, e := range skip {
		if _, ok := scoreFromEvent(e); ok {
			t.Errorf("%s: forwarded", name)
		}
	}
}

// TestBridgeIntoService: events -> anomaly.scores JSON -> the service's
// intake -> its top-anomalies list. Only score events pass.
func TestBridgeIntoService(t *testing.T) {
	svc := anomaly.NewService(logger.New(io.Discard), nil)
	evs := []cartridge.Event{scoreEvent(4, 99), scoreEvent(6, 2)}
	other := scoreEvent(4, 77)
	other.Kind = 0x46
	evs = append(evs, other)
	var payloads []string
	n, err := bridge(context.Background(), evs, func(_ context.Context, b []byte) error {
		payloads = append(payloads, string(b))
		return svc.IngestScore(b)
	})
	if err != nil || n != 2 {
		t.Fatalf("bridged %d, err %v", n, err)
	}
	for _, p := range payloads {
		if strings.Contains(p, "payload") {
			t.Fatalf("payload field on the wire: %s", p)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(p), &m); err != nil {
			t.Fatal(err)
		}
	}
	got := svc.GetFlowScores("cartridge:2")
	if len(got) != 2 {
		t.Fatalf("%d flows recorded", len(got))
	}
	// A failing sink stops the batch and reports it.
	if _, err := bridge(context.Background(), evs, func(context.Context, []byte) error { return errors.New("down") }); err == nil {
		t.Fatal("sink error swallowed")
	}
}
