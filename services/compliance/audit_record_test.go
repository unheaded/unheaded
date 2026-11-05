// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package compliance

import (
	"context"
	"encoding/binary"
	"testing"
)

func auditBytes(decision, violation, class byte) []byte {
	b := make([]byte, 64)
	binary.LittleEndian.PutUint16(b[8:10], 7)
	binary.LittleEndian.PutUint16(b[10:12], 9)
	b[12], b[13], b[14] = decision, violation, class
	binary.LittleEndian.PutUint32(b[16:20], 0xABCDE)
	b[20] = 3
	return b
}

func TestIngestAuditRecord_AllowAndBlock(t *testing.T) {
	svc := newTestService(t)
	if err := svc.IngestAuditRecord(auditBytes(auditDecisionAllow, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := svc.IngestAuditRecord(auditBytes(auditDecisionBlock, auditViolationPIIEgress, 3)); err != nil {
		t.Fatal(err)
	}
	svc.aggregateViolations(context.Background())
	score := svc.GetScore()
	if score.NoData || score.Allowed != 1 || score.Blocked != 1 || score.ScorePct != 50.0 {
		t.Fatalf("score = %+v, want one allowed, one blocked, 50%%", score)
	}
	if score.AuditCompleteness != 1.0 {
		t.Fatalf("AuditCompleteness = %.4f, want 1: every ring record is an audit record", score.AuditCompleteness)
	}
	v := svc.ListViolations()
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if v[0].SrcServiceID != "7" || v[0].DstServiceID != "9" || v[0].ViolationType != ViolationPIIEgress ||
		v[0].Classification != ClassSovereignPII || v[0].TraceID != "abcde" || v[0].ZoneID != "3" {
		t.Fatalf("violation = %+v", v[0])
	}
}

func TestIngestAuditRecord_HostileInputRefused(t *testing.T) {
	svc := newTestService(t)
	for name, b := range map[string][]byte{
		"short":           make([]byte, 63),
		"long":            make([]byte, 65),
		"nil":             nil,
		"zero decision":   auditBytes(0, 0, 0),
		"bad decision":    auditBytes(9, 0, 0),
		"bad violation":   auditBytes(auditDecisionBlock, 0x7f, 0),
		"none on a block": auditBytes(auditDecisionBlock, 0, 0),
	} {
		if err := svc.IngestAuditRecord(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if s := svc.GetScore(); s.TotalPackets != 0 {
		t.Fatalf("TotalPackets = %d after refused input, want 0", s.TotalPackets)
	}
}
