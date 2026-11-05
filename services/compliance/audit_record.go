// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package compliance

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"time"

	"unheaded/pkg/protocol/bpfschema"
)

// Decisions and violation types of the compliance-ebpf audit record.
const (
	auditDecisionAllow = 0x01
	auditDecisionBlock = 0x02

	auditViolationPolicy     = 0x01
	auditViolationEncryption = 0x02
	auditViolationPIIEgress  = 0x03
	auditViolationZone       = 0x04
)

// IngestAuditRecord feeds one 64-byte compliance-ebpf AUDIT_LOG record into the
// score: the only path by which the plane sees a real packet. Hostile input:
// a record of the wrong size or an unknown decision is refused.
func (s *WireComplianceService) IngestAuditRecord(b []byte) error {
	if len(b) != bpfschema.ComplianceAuditRecordSize {
		return fmt.Errorf("audit record is %d bytes, want %d", len(b), bpfschema.ComplianceAuditRecordSize)
	}
	src := strconv.Itoa(int(binary.LittleEndian.Uint16(b[8:10])))
	dst := strconv.Itoa(int(binary.LittleEndian.Uint16(b[10:12])))
	trace := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b[16:20])), 16) // the flow label, host order
	class := DataClassification(b[14] & 0x03)
	zone := strconv.Itoa(int(b[20]))
	switch b[12] {
	case auditDecisionAllow:
		s.mu.Lock()
		s.appendAudit(AuditEntry{Timestamp: time.Now(), TraceID: trace, SrcServiceID: src, DstServiceID: dst,
			Action: ActionAllow, Classification: class, ZoneID: zone})
		s.mu.Unlock()
		s.RecordAllowed()
		return nil
	case auditDecisionBlock:
	default:
		return fmt.Errorf("audit record decision %#x", b[12])
	}
	var vt ViolationType
	switch b[13] {
	case auditViolationPolicy:
		vt = ViolationPolicy
	case auditViolationEncryption:
		vt = ViolationEncryption
	case auditViolationPIIEgress:
		vt = ViolationPIIEgress
	case auditViolationZone:
		vt = ViolationZone
	default:
		return fmt.Errorf("audit record violation type %#x", b[13])
	}
	s.RecordViolation(Violation{
		Timestamp:      time.Now(),
		SrcServiceID:   src,
		DstServiceID:   dst,
		ViolationType:  vt,
		Classification: class,
		ActionTaken:    ActionBlock,
		TraceID:        trace,
		ZoneID:         zone,
	})
	return nil
}
