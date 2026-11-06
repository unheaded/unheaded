// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package crosswalk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Written claims the evidence does not support (ADR-097: the evidence decides).
// Each was found false on 2026-11-05 and corrected; a claim comes back only by
// changing this list, in a commit that says what made it true.
var retiredClaims = []struct{ file, claim string }{
	{"compliance/policies/single-operator-posture.md", "Agents cannot push or sign"},
	{"compliance/policies/single-operator-posture.md", "every human statement is signed and lapses"},
	{"compliance/policies/single-operator-posture.md", "CI gates proven able to fail;"},
	{"compliance/catalog/controls.yaml", "not a CI\n      gate: it fails today"},
	{"compliance/catalog/controls.yaml", "title: First-party key exchange is hybrid post-quantum"},
	{"scripts/compliance/check-service-auth.sh", "not a CI gate: it fails"},
	{"pkg/compliance/crosswalk/collect/cisprobes.go", "the system default is 022"},
	{"docs/compliance/enforcing-soak.md", "Every host's mode file says"},
	{"compliance/catalog/controls.yaml", "with the rule set the auditd-rules\n      fix installs loaded"},
	{"pkg/compliance/crosswalk/collect/cisprobes.go", "lines cover all %d keys"},
	{"compliance/catalog/controls.yaml", "alert-mailer and the Well each answer"},
	{"pkg/compliance/crosswalk/collect/collect.go", "and the Well all answer"},
	{"README.md", "gRPC with mTLS by default"},
	{"VISION.md", "It gets the hosting evidence for free"},
	{"wiki/Security-Audit.md", "default-deny firewall, services talk via Wotan, uniform container hardening, isolated compose network | PASS |"},
}

// Documents that state a rule or an old result keep their text and say so up
// front, so a reader does not take it for today's state.
var requiredNotes = []struct{ file, note string }{
	{"compliance/policies/information-security-policy.md", "## 13. Where practice falls short"},
	{"docs/compliance/README.md", "Historical snapshot"},
	{"docs/compliance/SECRETS-SCAN.md", "Historical snapshot"},
	{"docs/security/SECURITY_AUDIT.md", "Historical snapshot"},
	{"VISION.md", "not today's state"},
	{"compliance/findings/register.yaml", "only the signed reprepro repo"},
}

func TestRetiredClaimsStayRetired(t *testing.T) {
	read := func(rel string) string {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	for _, c := range retiredClaims {
		if strings.Contains(read(c.file), c.claim) {
			t.Errorf("%s still says %q", c.file, c.claim)
		}
	}
	for _, n := range requiredNotes {
		if !strings.Contains(read(n.file), n.note) {
			t.Errorf("%s lacks %q", n.file, n.note)
		}
	}
}
