// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	cebpf "github.com/cilium/ebpf"

	"unheaded/pkg/logger"
	"unheaded/pkg/monadpkt"
	"unheaded/pkg/protocol/bpfschema"
	"unheaded/services/compliance"
)

const (
	// Emergency procedures in the S-G9-FOLD plan name these two paths exactly.
	ringTestDir = "/sys/fs/bpf/g9-ring-test"
	ringTestPin = ringTestDir + "/AUDIT_LOG"

	complianceObj = "../../ebpf/target/load-gate/default/bpfel-unknown-none/release/compliance-ebpf"

	xdpDrop = 1
	xdpPass = 2

	policyActionBlock = 0x02
	allowedSrc        = 1
	allowedDst        = 2
	blockedSrc        = 3
	blockedDst        = 4
)

func wireScore(t *testing.T, svc *compliance.WireComplianceService) compliance.ComplianceScore {
	t.Helper()
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/compliance/score", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("score status = %d", rec.Code)
	}
	var sc compliance.ComplianceScore
	if err := json.Unmarshal(rec.Body.Bytes(), &sc); err != nil {
		t.Fatal(err)
	}
	return sc
}

// A real packet through compliance_xdp, out of the ring and into the score
// (needs root; run the compiled test binary under sudo). This is the proof the
// evidence can fail: a blocked packet lowers the score.
func TestRingBlockedPacketMovesScore(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	coll, err := cebpf.LoadCollection(complianceObj)
	if err != nil {
		t.Fatalf("load %s: %v", complianceObj, err)
	}
	t.Cleanup(coll.Close)
	prog, policies, ring := coll.Programs["compliance_xdp"], coll.Maps["SERVICE_POLICIES"], coll.Maps["AUDIT_LOG"]
	if prog == nil || policies == nil || ring == nil {
		t.Fatalf("object lacks compliance_xdp, SERVICE_POLICIES or AUDIT_LOG: %v %v", coll.Programs, coll.Maps)
	}
	if err := os.Mkdir(ringTestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(ringTestPin)
		_ = os.Remove(ringTestDir)
	})

	key := uint32(blockedSrc) | uint32(blockedDst)<<16
	if err := policies.Put(key, [8]byte{policyActionBlock}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cfg := compliance.DefaultConfig()
	cfg.AggregationWindow = 10 * time.Millisecond
	svc := compliance.NewService(logger.New(io.Discard), nil, cfg)
	if err := svc.StartBackground(ctx); err != nil {
		t.Fatal(err)
	}

	// A pin that is not a ring is refused, not consumed until ctx ends.
	if err := policies.Pin(ringTestPin); err != nil {
		t.Fatal(err)
	}
	if err := consumeAuditRing(ctx, ringTestPin, svc); err == nil || !strings.Contains(err.Error(), "not a ring buffer") {
		t.Fatalf("hash map pin: err = %v, want a refusal", err)
	}
	if err := policies.Unpin(); err != nil {
		t.Fatal(err)
	}

	if err := ring.Pin(ringTestPin); err != nil {
		t.Fatal(err)
	}
	consumed := make(chan error, 1)
	consumeCtx, stop := context.WithCancel(ctx)
	t.Cleanup(stop)
	go func() { consumed <- consumeAuditRing(consumeCtx, ringTestPin, svc) }()

	send := func(label uint32, src, dst uint8, want uint32) {
		t.Helper()
		in, err := monadpkt.Build(monadpkt.Frame{FlowLabel: label, SrcSvc: src, DstSvc: dst})
		if err != nil {
			t.Fatal(err)
		}
		verdict, err := prog.Run(&cebpf.RunOptions{Data: in, DataOut: make([]byte, len(in))})
		if err != nil {
			t.Fatalf("test run: %v", err)
		}
		if verdict != want {
			t.Fatalf("services %d to %d: verdict = %d, want %d", src, dst, verdict, want)
		}
	}
	send(1, allowedSrc, allowedDst, xdpPass)
	send(2, blockedSrc, blockedDst, xdpDrop)

	var sc compliance.ComplianceScore
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		// Violations is the aggregator's copy: set means it ran after both.
		if sc = wireScore(t, svc); sc.Allowed+sc.Blocked == 2 && sc.Violations == 1 {
			break
		}
	}
	stop()
	select {
	case err := <-consumed:
		if err != nil {
			t.Fatalf("consumeAuditRing at ctx end = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumeAuditRing did not return when ctx ended")
	}

	if sc.Allowed != 1 || sc.Blocked != 1 || sc.NoData {
		t.Fatalf("score = %+v, want Allowed 1, Blocked 1, NoData false", sc)
	}
	if sc.ScorePct != 50 {
		t.Fatalf("ScorePct = %v, want 50: one of two packets blocked", sc.ScorePct)
	}
}

func TestRingRefusedRecordIsCountedNotFatal(t *testing.T) {
	svc := compliance.NewService(logger.New(io.Discard), nil, nil)
	var log bytes.Buffer
	sink := auditRingSink{svc: svc, log: &log}

	sink.take(make([]byte, 3)) // wrong size
	rec := make([]byte, bpfschema.ComplianceAuditRecordSize)
	rec[12] = 0x7f // unknown decision
	for i := 1; i < auditRefusedLogEvery; i++ {
		sink.take(rec)
	}
	if sink.refused != auditRefusedLogEvery {
		t.Fatalf("refused = %d, want %d", sink.refused, auditRefusedLogEvery)
	}
	if got := strings.Count(log.String(), "\n"); got != 2 {
		t.Fatalf("log lines = %d, want 2 (the first and the %dth):\n%s", got, auditRefusedLogEvery, log.String())
	}

	rec[12] = 0x01 // allow: the ring goes on after a refusal
	sink.take(rec)
	if sc := wireScore(t, svc); sc.Allowed != 1 || sc.TotalPackets != 1 {
		t.Fatalf("score = %+v, want the one good record alone", sc)
	}
}

func TestRingMissingPinIsAnError(t *testing.T) {
	svc := compliance.NewService(logger.New(io.Discard), nil, nil)
	if err := consumeAuditRing(context.Background(), ringTestDir+"/no-such-pin", svc); err == nil {
		t.Fatal("consumeAuditRing on a missing pin = nil, want an error")
	}
}
