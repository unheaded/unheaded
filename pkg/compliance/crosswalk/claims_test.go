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
	// The first rewrite of A1 narrowed all of it to people outside the project.
	// Only commit signing is excepted.
	{"compliance/policies/risk-management-strategy.md", "anyone outside this project"},
	// The 2026-07-29 secrets decision, replaced on 2026-11-06: the repository is public.
	{"docs/security/PRE-PUBLIC-BLOCKERS.md", "RESOLVED BY RISK DECISION"},
	{"docs/security/PRE-PUBLIC-BLOCKERS.md", "Rotating them buys little"},
	{"CLAUDE.md", "no rotation scramble"},
	{"compliance/catalog/controls.yaml", "passwordless"},
	// Checked against the code on 2026-11-07: a design rule is not a proof, the
	// code reads secrets from the environment, and the apt listener has a LAN address.
	{"CLAUDE.md", "Zero user data access (validated)"},
	{"CLAUDE.md", "Zero user data access - architectural isolation enforced"},
	{"CLAUDE.md", "✅ Zero user data access"},
	{"CLAUDE.md", "Secrets: never in code, environment, or logs"},
	{"CLAUDE.md", "127.0.0.1 + 192.168.13.2 only"},
	{"CLAUDE.md", "**Alpha Success Criteria (ALL MET):**"},
	{"scripts/check-secrets-baseline.sh", "non-internet-facing"},
	{".github/workflows/security.yml", "excised from history"},
	{"docs/adr/ADR-098-findings-register-and-baseline-modes.md", "holds reviewed false positives"},
	// Checked on 2026-11-07: sbom/VERSION was seeded by hand once, the gate reads
	// a stamp, and sbom/SBOM.md's verdict is REVIEW.
	{"scripts/check-sbom-version.sh", "is written only by a full"},
	{"wiki/Releases.md", "Only a full run of `scripts/generate-sbom.sh` writes that file"},
	{"CLAUDE.md", "553 dependencies audited, GPL boundary clean"},
	{"CLAUDE.md", "| Signed apt repo (reprepro, nginx)"},
	// FND-004 is open: these said as fact what it has not yet tested.
	{"docs/compliance/privacy/decisions-for-stevie.md", "the hosts are not reachable from the internet (FND-004)"},
	{"docs/compliance/privacy/supplier-inventory.md", "edge firewall drops unsolicited inbound (FND-004)"},
	{"compliance/policies/risk-management-strategy.md", "rotating the baselined lab credentials"},
	// Key 4 has one writer in the tree; the object itself does not refuse.
	{"pkg/hoprules/hoprules.go", "only SetMode\n// writes it"},
	{"pkg/hoprules/hoprules.go", "and only SetMode"},
	// 2026-11-09: hop_xdp refuses enforce unless built with hop-enforce, so
	// "the object honours whatever key 4 holds" is no longer true of what ships.
	{"docs/adr/ADR-125-hop-checks.md", "The refusal is in that writer, not in `hop_xdp`"},
	{"compliance/findings/register.yaml", "hop_xdp itself honours whatever key 4 holds"},
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
	{"compliance/catalog/controls.yaml", "INCOMPLETE as a license record"},
	{"docs/adr/ADR-136-bind-zones-and-locked-down-ca.md", "**Status:** Proposed (parked)"},
	{"docs/adr/ADR-136-bind-zones-and-locked-down-ca.md", "needs Stevie's confirmation"},
	{"docs/adr/ADR-137-doom-range-check-and-dashboard-links-to-uhl.md", "**Status:** Proposed (parked)"},
	{"docs/adr/ADR-INDEX.md", "(ADR-136-bind-zones-and-locked-down-ca.md)"},
	{"docs/adr/ADR-INDEX.md", "(ADR-137-doom-range-check-and-dashboard-links-to-uhl.md)"},
	{"docs/compliance/SBOM.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"docs/compliance/LICENSE-AUDIT.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"LICENSES/THIRD_PARTY.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"compliance/policies/risk-management-strategy.md", "signed with Stevie's key and carry `Co-Authored-By` and `Claude-Session` trailers"},
	{"compliance/policies/single-operator-posture.md", "signed with Stevie's key and carry `Co-Authored-By` and `Claude-Session` trailers"},
	{"compliance/policies/risk-management-strategy.md", "A finding that could let someone sign with Stevie's key, push to the repository, or forge evidence or drift events is fixed, not accepted."},
	{"compliance/policies/risk-management-strategy.md", "from `7a93b734` (2026-11-04) on"},
	{"compliance/policies/single-operator-posture.md", "`sign-attestation.sh` checks the git identity and that signing works"},
	{"scripts/compliance/sign-attestation.sh", "so an agent session passes it too (FND-038)"},
	{"docs/adr/ADR-125-hop-checks.md", "**Scope.** Enforce covers the packets that reach the checks. Four classes do not"},
	{"docs/adr/ADR-125-hop-checks.md", "the object refuses enforce unless built with `hop-enforce`"},
	{"compliance/findings/register.yaml", "2026-11-09: the object refuses too"},
	{"sbom-results/SBOM-CONSOLIDATED.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"sbom-results/SBOM-SUMMARY.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"sbom-results/COMPLIANCE_GATE.md", "**Superseded by `sbom/SBOM.md`.**"},
	{"scripts/check-sbom-version.sh", "cannot tell a full run from a hand edit"},
	{"wiki/Releases.md", "cannot tell a full run from a hand edit"},
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
