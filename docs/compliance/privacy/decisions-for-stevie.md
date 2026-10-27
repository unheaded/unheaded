<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Decisions for Stevie (compliance and privacy)

Open calls from S-WELL-SPRING Phase 12 (M-100..M-104, M-109), escalation E-E (decision 28 in `docs/sweeps/OUTSTANDING-2026-10-27.md`). Each has a default that stands until you answer. To answer: fill **Answer** with your call and the date; the docs that depend on it are listed so they can be updated in the same change.

| # | Question | Default (stands until answered) | Why this default | Depends on it | Answer |
|---|---|---|---|---|---|
| D1 | **MFA scope.** Where is a second factor required? | GitHub account now (2FA on the owner account, which holds push and release rights). Dashboard and Kanban login when the ADR-103 login lands for them (M-055). Host SSH stays key-only, no second factor (lab, LAN-only). | GitHub is the only internet-facing account that can change the code; the hosts are not reachable from the internet (FND-004) | information security policy section 4 | |
| D2 | **External penetration test.** | Lich (the in-repo adversary: campaigns under `tomb/lich/`, fuzzing) now; an external test before any internet exposure of a host or service | no internet-facing surface today, so an external test would mostly test the LAN | IR plan v2 section 9; risk appetite A3 | |
| D3 | **Host anti-malware / IDS.** | No AV. Detection = Heimdall drift (signed `drift.*`), the detection rules (`detection.yml`, S-WELL-SPRING Phase 11), auditd and the KEV poller. Suricata stays removed (2026-10-22) | two Ubuntu servers with no mail, browsing or untrusted uploads; AV adds a privileged process with little to catch | IR plan v2 section 1 | |
| D4 | **FIPS build variant.** | Not now. When needed: Go services built with Go's FIPS 140-3 module (`GOFIPS140`), Rust TLS on aws-lc-rs's FIPS feature; both as a second build, not a replacement | no adopter needs FIPS; a FIPS build constrains the hybrid post-quantum key exchange (ADR-105) | ADR-105; information security policy section 10 | |
| D5 | **Independent assessment** (SOC 2, ISO certification, a third-party review of the crosswalk). | Not now. The crosswalk review (`docs/compliance/crosswalk-review-2026-10-27.md`) is self-review and says so | the attestations are self-attested by design (ADR-097); an assessor needs operating history first | single-operator posture (accepted gap) | |
| D6 | **Counsel / DFIR retainer.** | None engaged. Two options if you want one: (a) pay-as-you-go: a privacy/breach attorney found through the State Bar of Texas lawyer referral service when an incident needs one, no retainer; (b) a cyber-insurance policy that includes a breach-coach hotline, which brings counsel and DFIR together. No specific firm has been vetted | the deployment holds the operator's own data only, so few incidents would start a legal clock | IR plan v2 sections 3 and 7; risk strategy section 1 ("no risk is transferred") | |
| D7 | **Risk appetite.** Confirm or replace the three statements | A1 keys and source integrity: no appetite (fix, never accept). A2 availability: lab appetite, SLO targets 99.5 % / 99 %, no silent failures. A3 host hardening deviations: accepted up to 90 days at a time while LAN-only; internet exposure ends every acceptance | written from your earlier calls (ADR-100 targets; ADR-098 answer 10) | risk management strategy section 2 | |
| D8 | **Enforcing-mode strictness** (ADR-098 Q5). Refuse to *start* services on an out-of-baseline host, or only refuse installs? | Only refuse installs (ADR-098 draft: availability is also a control) | asked again at S-WELL-SPRING Step 275 with the enforcing code path; recorded here | ADR-098; `cmd/unheaded-baseline` | |
| D9 | **Categorization of unheaded-auth / unheaded-keygen.** Low (as drafted) or moderate integrity? | Low | nothing outside the LAN trusts its tokens yet; raise it when a login is exposed | risk management strategy section 3 | |
| D10 | **Attestation expiry.** The signing helper sets `expires_at` = the control's freshness window: 365 days for the policy and IR plan, **90 days for the access review** (the plan said 365 for all three) | Freshness window | one date per attestation: after 90 days the access review would be stale anyway, and an expired one fails loudly instead of sitting STALE | `scripts/compliance/sign-attestation.sh` | |

## Signatures waiting on you (E-C)

Not decisions, but yours alone. After reading what each names (`docs/compliance/drafts/attestations/README.md`):

```bash
scripts/compliance/sign-attestation.sh security-policy-review
scripts/compliance/sign-attestation.sh ir-plan-review
scripts/compliance/sign-attestation.sh access-review
```


## E-E (S-WELL-SPRING Step 275): enforcing mode and the open paperwork decisions

Context: this file, `docs/compliance/risk-management-strategy.md` and `docs/compliance/enforcing-soak.md`.

1. Q5: may enforcing mode refuse to START Unheaded services on an out-of-baseline host, or only refuse installs (the draft default, built as the package preinst gate)?
2. MFA scope.
3. External pentest timing.
4. Host AV/IDS stance (D3 above).
5. Counsel/DFIR retainer (D6).
6. The three risk-appetite statements (D7).
7. Flip prerequisite: `unheaded-baseline` runs unprivileged, so enforcing needs a root drop-in for `unheaded-baseline.service`, measured on east in the window first. Approve that drop-in per host?

Why yours: availability vs integrity, spend, and legal posture. Until answered every mode file stays `audit`.
