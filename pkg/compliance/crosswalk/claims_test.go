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
	// A1 said a finding that could let someone sign as the owner is never
	// accepted, while agents' commits are signed with the owner's key.
	{"compliance/policies/risk-management-strategy.md", "A finding that could let someone sign as the owner"},
	{"compliance/policies/single-operator-posture.md", "a warm gpg-agent"},
	// The 2026-07-29 secrets decision, replaced on 2026-11-06: the repository is public.
	{"docs/security/PRE-PUBLIC-BLOCKERS.md", "RESOLVED BY RISK DECISION"},
	{"docs/security/PRE-PUBLIC-BLOCKERS.md", "Rotating them buys little"},
	{"CLAUDE.md", "no rotation scramble"},
	{"scripts/check-secrets-baseline.sh", "non-internet-facing"},
	{".github/workflows/security.yml", "excised from history"},
	{"docs/adr/ADR-098-findings-register-and-baseline-modes.md", "holds reviewed false positives"},
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
	{"docs/security/PRE-PUBLIC-BLOCKERS.md", "every value in history counts as burned"},
	{"CLAUDE.md", "every value in history counts as burned"},
	{"docs/compliance/SBOM.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"docs/compliance/LICENSE-AUDIT.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"LICENSES/THIRD_PARTY.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"compliance/policies/risk-management-strategy.md", "signed with Stevie's key and carry `Co-Authored-By` and `Claude-Session` trailers"},
	{"compliance/policies/single-operator-posture.md", "signed with Stevie's key and carry `Co-Authored-By` and `Claude-Session` trailers"},
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
