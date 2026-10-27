<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Information Security Policy

**Version:** 1 (draft 2026-10-27)
**Owner and approver:** Stevie Bellis
**Approval:** a signed attestation, `compliance/attestations/security-policy-review.yaml` (control UH-POL-01). Until it exists this policy is a draft.
**Review:** yearly, and after any change to the scope below. The attestation expires after 365 days; an expired one fails UH-POL-01.

Each section names the catalog controls (`compliance/catalog/controls.yaml`) that evidence it. A section with no control is a statement of practice only.

## 1. Scope

- **Systems:** the hosts west and east, the services and containers of this repository running on them, The Well (PostgreSQL), Wotan, the monitoring stack, and the GitHub repository `unheaded/unheaded`.
- **Data:** source code, configuration, operational telemetry (metrics, logs, packet headers and flow metadata), and the operator's own records. The platform does not read packet payloads or adopters' application data (NIST 800-207 Zero Trust Architecture; [privacy notice](../../docs/compliance/privacy/privacy-notice.md)).
- **People:** one operator. What that means for separation of duties is in the [single-operator posture](single-operator-posture.md).
- **Out of scope:** adopters' own deployments. They inherit the controls by running the same code, not this policy.

## 2. Roles

| Role | Held by | Duties |
|---|---|---|
| Owner, approver | Stevie Bellis | approves this policy, accepts risk, signs attestations, approves runbooks (`metadata.status: approved`), pushes to GitHub |
| Operator | Stevie Bellis | runs hosts and services, applies migrations and fixes, responds to incidents |
| Automated agents | Claude Code sessions, Zhen | write code and docs on branches and commit; never push, never sign attestations, never change hosts outside an answered escalation |

## 3. Acceptable use

- West and east run this project's workloads and the operator's development. No unrelated internet-facing service runs on them.
- Credentials are never written into the repository, chat transcripts or logs. They live in environment files outside git or in files readable only by the service account that needs them.
- Lab-only gaps (for example unencrypted backup disks) are recorded and audited, not hidden; they become findings before any internet exposure.

Supports: UH-SECRET-01.

## 4. Access control

- **Hosts:** SSH with keys. Each third-party and first-party service runs as its own nologin system account with a fixed uid (`pkg/uids`); no service runs as root or as a shared account.
- **Source:** GitHub account of the owner, with signed commits.
- **The Well:** TLS-only; one login role per writer with the least grants it needs; owner roles cannot log in (migration 025). Passwords in `.env` outside git.
- **Services:** Ed25519 service tokens and user tokens from the built-in issuer (ADR-103).
- **Review:** every 90 days, attested (`compliance/attestations/access-review.yaml`).

Supports: UH-AC-01, UH-SSH-01, UH-ACCT-01, UH-ACCT-02, UH-SUDO-01, UH-PASS-01, UH-PAM-01, UH-SESS-01, UH-AUTHN-01, UH-CTR-01, UH-CFG-02.

## 5. Change management

- All change is in git. Work lands on `develop`, is promoted to `staging` and then `main` (ADR-089); promotion is the owner's call.
- Commits on `develop` are signed.
- Schema changes are ledgered migrations (`db/migrations/MANIFEST`, ADR-101); an applied migration is never edited.
- CI gates run on every push and are proven able to fail (`scripts/check-gates-can-fail.sh`). A gate's baseline may only shrink.
- Host changes go through `cmd/unheaded-remediate`, which journals every apply and keeps an undo.

Supports: UH-SIGN-01, UH-GATE-01, UH-SAST-01, UH-VULN-01, UH-SBOM-01, UH-FUZZ-01, UH-LIC-01, UH-REMED-01, UH-CFG-01.

## 6. Logging and monitoring

- Host and container logs go to the systemd journal and from there through Vector (ADR-092). Log storage is bounded.
- Service level objectives, alert urgency and the single mail sender are defined in `observability/alerting/policy.yaml` (ADR-100). Every alert names a runbook.
- A dead-man's switch (Watchdog, west/east heartbeat) proves the alert path itself is alive.
- Security-relevant host events are recorded by auditd. Clocks are synchronized.

Supports: UH-LOG-01, UH-LOG-02, UH-AUDIT-01, UH-TIME-01, UH-FIM-01, UH-BASE-01.

## 7. Incident response

Incidents are handled under the [IR plan v2](../../runbooks/security/ir-plan-v2-2026-10-27.md). Every incident over the ADR-100 threshold gets a blameless postmortem in `docs/incidents/`. The plan is reviewed yearly and after each tabletop, attested (`compliance/attestations/ir-plan-review.yaml`).

Supports: UH-IR-01.

## 8. Vulnerability management

- Dependencies: govulncheck, cargo-audit, pip-audit and grype on the SBOM fail the build on known-vulnerable versions.
- Hosts: unattended security updates; the CISA KEV feed is polled (`cmd/kev-poller`, ADR-055) and a stale feed alerts.
- Host configuration: the CIS-derived baseline (`compliance/baseline/baseline.yaml`) is checked by `unheaded-baseline` on each host and by the central collector; failures are findings in the ADR-098 register.
- Detail: `runbooks/security/vuln-management-2026-05-06.md`.

Supports: UH-VULN-01, UH-SBOM-01, UH-PATCH-01, UH-HOST-01, UH-HOST-02, UH-HOST-03, UH-HOST-04, UH-SVC-01, UH-SVC-02, UH-CRON-01, UH-NET-01, UH-FW-01, UH-MAC-01, UH-BOOT-01, UH-BANNER-01, UH-RADIO-01.

## 9. Backup and recovery

The Well is dumped nightly to a second disk on west and copied to east; a restore drill runs weekly and alerts when it has not passed for 8 days. Recovery order and objectives are in the [contingency plan](contingency-plan.md).

Supports: UH-BACKUP-01.

## 10. Cryptography

- First-party TLS is 1.3 only and negotiates hybrid post-quantum key exchange (`X25519MLKEM768`, ADR-105). The edge never serves plain HTTP.
- Evidence and drift events are sealed with ML-DSA-65.
- The cryptographic inventory (`compliance/crypto/cbom.cdx.json`) is regenerated and gated for staleness and classical key exchange.
- Keys are held by the owner; custody is listed in the contingency plan.

Supports: UH-TLS-01, UH-PQC-01, UH-PQC-02, UH-PQC-03, UH-EVID-01.

## 11. Exceptions

An exception is an `accepted` entry in the ADR-098 findings register (`compliance/findings/register.yaml`): reason, compensating control, the date accepted and an `until` at most 90 days later. An accepted finding still counts as open; it is not hidden from any figure. Exceptions are renewed or fixed, never left to lapse silently.

## 12. Review

This policy, the [single-operator posture](single-operator-posture.md) and the [risk management strategy](risk-management-strategy.md) are reviewed together once a year. The review is the signed `security-policy-review` attestation (`scripts/compliance/sign-attestation.sh security-policy-review`).
