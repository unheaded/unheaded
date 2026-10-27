<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Risk Management Strategy

**Version:** 1 (draft 2026-10-27)
**Owner:** Stevie Bellis. Reviewed with the [information security policy](information-security-policy.md) (UH-POL-01 attestation).
**Covers:** 800-53 PM-9, RA-2, RA-3, PM-11; CSF 2.0 GV.RM; ISO 27001:2022 clauses 6.1.2, 6.1.3 (the Statement of Applicability); SOC 2 CC3.1-CC3.4.

## 1. How risk is handled

- **Identified** by evidence, not opinion: the catalog's controls (`compliance/catalog/controls.yaml`) are checked by collectors; a failing source is a finding in the ADR-098 register (`compliance/findings/register.yaml`) with severity = exposure x impact.
- **Assessed** per finding (severity, lockout risk, remediation step) and per system (section 3).
- **Responded to** in one of three ways: fix (default), accept with an end date (`accepted`, at most 90 days, still counted as open), or avoid (remove the component). No risk is transferred: there is no insurance and no contracted provider (decision in [decisions for Stevie](../../docs/compliance/privacy/decisions-for-stevie.md)).
- **Monitored** continuously by the collectors and alert rules (`ComplianceNewSevereFinding`, `ComplianceAcceptanceExpired`, `ComplianceFindingsOverdue`).
- **Reported** in the generated SSP (`docs/compliance/SSP-generated.md`) and the generated section of ADR-098.

## 2. Risk appetite

Defaults, written so they can be held to. **Open for Stevie (S-WELL-SPRING E-E):** each is a default until he confirms or replaces it in [decisions for Stevie](../../docs/compliance/privacy/decisions-for-stevie.md).

| # | Statement (default) | Holds while |
|---|---|---|
| A1 | **Keys and source integrity: no appetite.** A finding that could let someone sign as the owner, push to the repository, or forge evidence or drift events is fixed, not accepted. | always |
| A2 | **Availability: lab appetite.** Outages within the SLO targets (99.5 % / 99 %, ADR-100) are accepted; budgets are not frozen when spent. A *silent* failure is not accepted: every outage must be detectable (Watchdog, heartbeat, SLO pages). | the hosts serve only the operator |
| A3 | **Host hardening deviations: accepted while LAN-only.** CIS deviations on west and east may be accepted for up to 90 days at a time (ADR-098 answer 10: until 2026-12-30) because the hosts are reachable from the home LAN and the P2P link only. Any internet exposure ends every such acceptance and requires rotating the baselined lab credentials first. | no internet-facing listener on either host |

## 3. System categorization

FIPS 199 style: confidentiality, integrity, availability rated low / moderate / high for the information each service holds or controls; the service takes the highest of the three. "Moderate" here means a loss would seriously hurt this project's operation or trust in its evidence; nothing here is rated high, as no third party's data or safety depends on it.

| Service | C | I | A | Category | Why |
|---|---|---|---|---|---|
| The Well (PostgreSQL) | moderate | moderate | moderate | **moderate** | holds Kanban, Zhen captures and conversations, audit and compliance history; integrity of append-only history is evidence |
| Wotan | low | moderate | moderate | **moderate** | every service's bus; forged events (drift, health) mislead detection |
| Dashboard backend | low | moderate | moderate | **moderate** | the operator's live view; a wrong view hides incidents |
| Kanban, timeguru, captain, architect, micromanager, monad, sophia | low | low | low | low | planning data, rebuildable from git and The Well |
| unheaded-daemon (Cuirass), Sword, chaos-controller | low | low | low | low | lab deploys and chaos inside an allowlist (ADR-107) |
| unheaded-auth, unheaded-keygen | low | low | low | low | proposed raise to moderate integrity (issues tokens): decision for Stevie |
| trace-collector, Heimdall, Akira, kev-poller, alert-mailer | low | low | low | low | detection and telemetry; loss is visible (SLOs, Watchdog) |
| Monitoring stack, Vector, ClickHouse | low | low | low | low | history; ClickHouse holds incident evidence (contingency plan gap C2) |
| Zhen (zhen-agentd, MCP, llama-server) | low | low | low | low | local models; writes need confirmation; captures are redacted |

## 4. Criticality

Recovery and protection order, highest first (the contingency plan's recovery order follows it):

1. Signing keys and the repository (GitHub, the east mirror): everything else is rebuilt from them.
2. The Well and its backups.
3. The alert path: Prometheus, Alertmanager, alert-mailer, Watchdog and heartbeat.
4. Wotan.
5. The dashboard and Kanban.
6. Everything else.

## 5. Statement of Applicability

Generated, not written: the `## SoA` section of `docs/compliance/SSP-generated.md` (`go run ./cmd/compliance-evidence ssp -out docs/compliance/SSP-generated.md`). Every framework in `compliance/catalog/frameworks/` is listed with the publisher's full requirement count and how many requirements the controls evidence, fail, leave incomplete, or do not map. Requirements outside a single-operator lab (personnel, training, dual control) are excluded with reasons on the [single-operator posture](single-operator-posture.md) page, not dropped from the counts.

## 6. Review

Yearly with the policy, after any change to the scope (a second person, an internet-facing listener, an adopter's data), and when an appetite statement is broken.
