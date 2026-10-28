<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Severity ladder

What each severity means for shipping, and the rule for turning an
observe-mode signature check into an enforcing one. ADR-062 (Open) asked for
this document. ADR-098 owns the register, the due dates and the acceptance
rules; this page does not change them. It says what each level blocks.

## The ladder

| Severity | Meaning (exposure × impact, MoatGhost rubric) | What it blocks | Due (ADR-098) |
|---|---|---|---|
| critical | Exploitable as deployed, with severe impact: remote code execution, credential or key disclosure, auth bypass on a reachable listener, forged signed data accepted | Blocks promotion (develop → staging → main, ADR-089) and any live rollout of the affected component | 7 days |
| high | Exploitable with a precondition the deployment does not reliably prevent, or severe impact with low exposure | Blocks promotion | 30 days |
| medium | Real weakness with a compensating control in place, or limited impact | Does not block. Logged in the register **with the compensating control named** | 90 days |
| low | Hardening gap, defence in depth, or reachable only by someone who already holds what it protects | Does not block. Tracked in the register | 180 days |
| untriaged | A failing source with no register entry (ADR-098) | Treated as high until triaged | none until triaged |

Rules:

- The severity in `compliance/findings/register.yaml` is the one that counts. Its `exposure` field records the
  assumption behind it. If the assumption stops holding (a LAN-only service gets a public address, a parked
  package gets a caller), the severity is re-rated in the same commit that changes the exposure.
- A critical or high finding gets past the promotion block only through an ADR-098 acceptance: reason,
  compensating control, expiry within 90 days, and Stevie's explicit word in the commit that records it. An
  accepted finding is still open.
- No inflation and no deflation. One blocker is a blocker. "It is only a lab" is a property of where something
  runs, not of the finding, and it expires when that changes (CLAUDE.md, Secrets Management).
- Findings from the fuzz, red-team and pentest framework (ADR-062, the Lich) enter the register at the severity
  the reproducer proves, not the one the campaign hoped for.

## Signature rollout: observe, then enforce

A new signature check starts in **observe** mode when making it mandatory would refuse traffic that is legitimate
today (ADR-062 recorded this for Sophia dictionary deltas). Observe mode accepts the unsigned input, counts it
and logs it. A bad signature is never observe-only: a signed input that fails verification, uses an untrusted key
or is stale is refused in both modes.

The check flips to **enforce** when all of these hold:

1. A first-party producer signs every input of that kind, with a key enrolled per deployment (S-IRON-GATE key
   layout: ADR-103, `unheaded-keygen -algo mldsa65`, mode 0400, the service's own account).
2. The live stack has a consumer that runs the check, so the counter below measures real traffic. A counter
   that stays at zero because nothing reads the input proves nothing, and the 7-day clock does not start.
3. Seven consecutive days with zero growth of the unsigned counter on every host that runs the consumer.

The flip is a commit that sets the enforcing environment variable in the unit or compose file, with the counter
evidence in its message. The default in code never changes silently: a default flip is a separate, reviewed
change.

| Check | Observe-mode counter | Failure counter | Enforce switch | State (2026-10-27) |
|---|---|---|---|---|
| Sophia dictionary deltas (`pkg/protocol/sophiasync`, M-149) | `unheaded_sophia_delta_unsigned_total` | `unheaded_sophia_delta_signature_failures_total` | `SOPHIA_REQUIRE_DELTA_SIGNATURE=true` | Observe. No service applies deltas yet (G8 reference package), so condition 2 does not hold. |
| Wotan WAL replication (`services/wotan/internal/cluster`, FND-020) | none: no observe mode | `unheaded_wotan_replication_rejected_total{reason}` | always enforcing | Enforcing in code. Cluster mode does not start yet (ADR-064). |
| Wotan signed topic prefixes (ADR-043 #2, ADR-105) | none: no observe mode | publish refused (InvalidArgument unsigned, PermissionDenied bad signature) | always enforcing | Enforcing. |

Checks without an observe mode are ones where nothing legitimate was ever unsigned. They do not need one.
