<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# REAL-STEEL's parked packages, one month on (2026-11-21)

A dated follow-up to [REAL-STEEL-UNREACHED.tsv](REAL-STEEL-UNREACHED.tsv), written under
`docs/battle-plans/DECISION-PACK-2026-11-18.md` row 41 (M-223.3). It changes no code and decides
nothing about any package. The TSV, [REAL-STEEL-DECISIONS.md](REAL-STEEL-DECISIONS.md) and
[TRUE-WIRE-DECISIONS.md](TRUE-WIRE-DECISIONS.md) are dated records and are not edited. The living
list is [docs/UNREACHED-GO-PACKAGES.md](../UNREACHED-GO-PACKAGES.md).

## Why this page

The inventory of 2026-11-18 said the 37 packages the sweep parked "have no follow-up row". Most
of them have one, in four different places. This page puts the 37 on one sheet with where each
follow-up lives, so the ones whose parking reason has gone stale can be seen.

## How it was read

- The 37 rows of the TSV whose verdict is `PARK` (34 from groups G7, G8 and G9, three flagged rows).
- The tree on develop at `08f4f961`: does the directory exist.
- "Reached": the package is in `go list -deps` of some `main` package of the root module
  (tests do not count), the same rule `docs/UNREACHED-GO-PACKAGES.md` uses.

Result: **9 are gone, 4 are reached by a binary, 24 are still parked and unreached.**

## The 28 that exist

| Package | Group | Reached today | Where the parking is recorded | What would bring it into use |
|---|---|---|---|---|
| `pkg/baremetal` | G7 | no | REAL-STEEL-DECISIONS G7: Stevie's standing "parked" | Stevie's word; bare metal provisioning is not in any plan |
| `pkg/baremetal/image` | G7 | no | as above | as above |
| `pkg/baremetal/inventory` | G7 | no | as above | as above |
| `pkg/baremetal/ipmi` | G7 | no | as above | as above |
| `pkg/baremetal/pxe` | G7 | no | as above | as above |
| `services/sabatons` | G7 | no | as above; TRUE-WIRE-DECISIONS C9, "stays parked" | as above |
| `pkg/monad` | G8 | no (tests in `pkg/crypto/pqc`, `pkg/protocol/bpfschema`, `tests/pqc` import it) | `docs/protocol/G8-REFERENCE-RULINGS.md`: KEEP-AFTER, reference for pqc-authentication | The RFC Editor's TW-2 answer (`docs/protocol/RFC-EDITOR-REQUESTS-TW.md`), then keep the value layout and pseudo-header files and fold or drop `flags.go` |
| `pkg/protocol/errors` | G8 | no (only `pkg/protocol/tlv` imports it) | G8 rulings: KEEP-AFTER; rewritten to the registry in S-TRUE-WIRE | It is reference code by ruling; a binary that reports registry codes from Go would import it |
| `pkg/protocol/tlv` | G8 | no | G8 rulings: KEEP-AFTER | Reference code by ruling; a Go TLV parser on a live path |
| `pkg/protocol/integrity` | G8 | no (one test in `pkg/protocol/encoding` imports it) | G8 rulings, "Applied 2026-11-04": the HMAC half stays unwired, the Monad has no tag field; ADR-125 open question 2 | A tag on the wire, which is `-02` work (pack row 37); until then ADR-125's question 2 (does it stay) is unanswered |
| `pkg/protocol/migration` | G8 | no | TRUE-WIRE-DECISIONS R4, "Applied 2026-11-04": stays parked; `ValidateToken` is a stub that must not be wired | A retry token on the wire; none is defined |
| `pkg/protocol/sophiasync` | G8 | no (one Lich harness imports it) | G8 rulings: WIRE IN at S-TRUE-WIRE Phase 8 (signed deltas, observe mode) | A binary that sends or applies Sophia dictionary deltas; none does |
| `pkg/protocol/bpfschema` | G8 | **yes**: chaos-controller, compliance-evidence, trace-collector-go, unheaded-cli | left the unreached list on 2026-10-26 (S-REAL-STEEL Phase 13) | in use |
| `pkg/protocol/encoding` | G8 | **yes**: ten binaries, trace-collector-go and dashboard-backend among them | left the unreached list on 2026-10-26 | in use |
| `pkg/pqc_policy` | F3 | no (its own tests and `tests/pqc` import it) | REAL-STEEL-DECISIONS F3: "Park; let that sprint decide" (S-QUANTUM-ARMOR, ADR-105) | **No owner line today**: S-QUANTUM-ARMOR ran on 2026-10-23 and no ruling on this package was found in its plan, its progress files or ADR-105 |
| `services/anomaly` | G9 | **yes**: `cmd/anomaly` | left the unreached list on 2026-10-23 | in use (port 19018) |
| `services/compliance` | G9 | **yes**: `cmd/compliance-evidence wire` | ADR-127 (Accepted 2026-11-18; off unless `--serve`) | in use, off by default |
| `services/firewall` | G9 | no | ADR-128 (Proposed, parked) | Stevie's reading of ADR-128 |
| `services/qos` | G9 | no | ADR-129 (Proposed, parked) | Stevie's reading of ADR-129 |
| `services/nfv` | G9 | no | ADR-130 (Proposed, parked) | Stevie's reading of ADR-130 |
| `services/selfheal` | G9 | no | ADR-131 (Proposed, parked) | Stevie's reading of ADR-131 |
| `services/loadbalancer` | F6 | no | ADR-132 (Proposed, parked) | Stevie's reading of ADR-132 |
| `services/version` | G9 | no | ADR-133 (Proposed, parked) | Stevie's reading of ADR-133 |
| `services/keymgmt` | G9 | no | ADR-134 (Proposed, parked) | Stevie's reading of ADR-134 |
| `services/keymgmt/handler` | G9 | no | with `services/keymgmt` | as above |
| `services/keymgmt/lifecycle` | G9 | no | with `services/keymgmt` | as above |
| `services/keymgmt/store` | G9 | no | with `services/keymgmt` | as above |
| `services/wotan/internal/compute` | F7 | no | REAL-STEEL-DECISIONS F7: "Park for S-UPC-WIRE-COMPUTE"; that plan names it for Phase 18 (Step 433), which is deferred with east as a node | S-UPC-WIRE-COMPUTE Phase 18, if it is ever run; nothing schedules it |

Ten packages under `services/` hold their own `alerts.critical` listener, nine of them in this table; seven
of the ten are imported by no binary (ADR-144, "Update 2026-11-21").

## The 9 that are gone

Deleted on 2026-11-04 (DP-715, S-WIRE-HONEST; `docs/UNREACHED-GO-PACKAGES.md`, "Applied 2026-11-04"):
`pkg/protocol/amplification`, `dos`, `flowtype`, `intermediary`, `lifecycle`, `prefetch`, `registry`,
`sequence` and `settings`.

## Rows whose parking reason no longer says who decides

These are parked "until" something that has since happened or is not scheduled. Nothing is proposed
here; they are listed so that a later sitting can give each an owner line or a decision.

1. `pkg/pqc_policy`: parked for a sprint that has finished without ruling on it.
2. `services/wotan/internal/compute`: parked for a phase that is deferred with no date.
3. `pkg/protocol/integrity`: ADR-125's open question 2 waited on RFC item 7; item 7 was answered on
   2026-11-10 (the HMAC option moves to future work) and the question is still open.
4. `pkg/protocol/sophiasync`: ruled "wire in" and signed since S-TRUE-WIRE, and still imported by no binary.

Deleting any parked package is not a row 41 call: dropped work moves to a sibling repository on
Stevie's word.
