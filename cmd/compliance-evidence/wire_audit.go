// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"unheaded/pkg/ebpf"
	"unheaded/services/compliance"
)

// auditRefusedLogEvery bounds the log: a program built against another record
// layout has every record refused, at wire speed.
const auditRefusedLogEvery = 4096

// auditRingSink feeds ring records into the score. A record the service
// refuses is counted and logged, and the ring goes on. Single goroutine: it
// runs inside ebpf.ConsumePinnedRing.
type auditRingSink struct {
	svc     *compliance.WireComplianceService
	log     io.Writer
	refused uint64
}

func (s *auditRingSink) take(rec []byte) {
	err := s.svc.IngestAuditRecord(rec)
	if err == nil {
		return
	}
	s.refused++
	if s.refused == 1 || s.refused%auditRefusedLogEvery == 0 {
		fmt.Fprintf(s.log, "compliance-evidence wire: audit ring: refused record %d: %v\n", s.refused, err)
	}
}

// consumeAuditRing runs the pinned compliance-ebpf AUDIT_LOG ring into svc
// until ctx ends. This is the only reader of AUDIT_LOG: a second one would take
// records away from the score.
func consumeAuditRing(ctx context.Context, path string, svc *compliance.WireComplianceService) error {
	sink := auditRingSink{svc: svc, log: os.Stderr}
	err := ebpf.ConsumePinnedRing(ctx, path, sink.take)
	if sink.refused > 0 {
		fmt.Fprintf(os.Stderr, "compliance-evidence wire: audit ring: %d records refused in all\n", sink.refused)
	}
	return err
}
