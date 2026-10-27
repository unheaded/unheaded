<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Single-Operator Posture

**Version:** 1 (draft 2026-10-27)
**Owner:** Stevie Bellis. Approved with the [information security policy](information-security-policy.md) (UH-POL-01 attestation).

One person owns, operates, reviews and approves everything here. Several framework requirements assume an organization: a personnel function, a training program, two people for sensitive actions. This page states which of those cannot be met, accepts the gap, and names what compensates. It does not claim the gap is closed.

## Accepted gaps

| Gap | Framework requirements | Why it cannot be met | Accepted |
|---|---|---|---|
| Awareness and role training (AT) | 800-53 AT-1..AT-4; ISO A.6.3; SOC 2 CC1.4, CC2.2 | no workforce to train; the operator writes the runbooks and the plans | yes |
| Personnel security (PS) | 800-53 PS-1..PS-9; ISO A.6.1, A.6.2, A.6.4, A.6.5 | no hiring, transfer or termination; no screening of oneself | yes |
| Separation of duties | 800-53 AC-5; ISO A.5.3; SOC 2 CC5.1 | the author, reviewer, approver and operator are the same person | yes |
| Dual control (two-person rule) | 800-53 AC-3(2), CM-5(4); PCI DSS 3.7.6 (split knowledge, dual control) | there is no second person | yes |
| 24/7 incident coverage | 800-53 IR-7 (assistance); PCI DSS 12.10.3 | one operator sleeps; mail is the only pager ([IR plan v2](../../runbooks/security/ir-plan-v2-2026-10-27.md) section 2) | yes |
| Independent assessment | 800-53 CA-2(1), CA-7(1); SOC 2 (the report itself) | no independent assessor engaged; open decision in [decisions for Stevie](../../docs/compliance/privacy/decisions-for-stevie.md) | yes, pending |

Agents (Claude Code, Zhen) are not a second person: they act on the operator's instructions and their work is approved by the same operator.

## Compensating controls

These make one person's actions recorded, attributable and hard to undo quietly. They do not replace a second reviewer.

| Control | What it gives | Evidence |
|---|---|---|
| Signed commits on `develop` | every change attributable to a key; history cannot be rewritten without it showing | UH-SIGN-01 |
| Gates that cannot be silenced | CI gates proven able to fail; baselines only shrink; a new finding cannot be waived by appending it | UH-GATE-01, UH-SECRET-01 |
| Attestations that expire | every human statement is signed and lapses (90 or 365 days); a lapsed one fails its control instead of standing forever | UH-POL-01, UH-IR-01, UH-AC-01 |
| Append-only journals | `audit_events`, `service_health_transitions` and the compliance history tables refuse UPDATE, DELETE and TRUNCATE for every role (migrations 014, 015, 016); host fixes are journalled with undo (`unheaded-remediate`) | UH-REMED-01 |
| Ledgered migrations | every schema change is a reviewed file in `db/migrations/` with a ledger row | (ADR-101) |
| Exceptions with an end date | accepted findings carry an `until` at most 90 days out and still count as open | ADR-098 register |
| Agents cannot push or sign | the owner pushes; attestation signing refuses any identity but the owner's (`scripts/compliance/sign-attestation.sh`) | (practice) |

## When this changes

A second person with commit or host access ends this posture: separation of duties, dual control for key and migration changes, and access review of that person become required, and this page is rewritten.
