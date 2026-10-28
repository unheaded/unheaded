<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Go packages no binary imports (2026-10-26)

Input for [ADR-090](adr/ADR-090-total-source-sweep-slop-removal.md); the library
counterpart of [LIVE-PATHS.md](LIVE-PATHS.md), which covers binaries. Not a
deletion list: keeping an experiment is a fine decision, it should just be one.

On develop at 2026-10-26, after S-REAL-STEEL Phase 13: **64 of 250** packages in
the root module are imported by no `main` package (tests do not count). After
Phase 1 the same day it was 66 of 248; before Phase 1 (2026-10-23) 148 of 300.
**2026-10-27 (S24, after NEXT-7 Sprints 2-4): 68 of 265.** The four new entries are test-only by design and need no
verdict: `pkg/auth/authtest`, `pkg/bff/bfftest`, `tests/svctoken` (the cross-language token test) and
`tomb/lich/LICH-017-baseline-agent` (a campaign run with `go test`); `pkg/wotan-client/wotantest` was already a test
helper. Nothing previously unreached became reached.

Phase 13 added two reached packages (`services/yaldabaoth/packet`,
`pkg/monadpkt`) and, through them, reached `pkg/protocol/bpfschema` and
`pkg/protocol/encoding`, which left the list. Regenerate:

```bash
mains=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...)
comm -23 <(go list ./... | sort -u) <(go list -deps $mains | grep '^unheaded/' | sort -u)
```

Every package below has a verdict in
[sweeps/REAL-STEEL-UNREACHED.tsv](sweeps/REAL-STEEL-UNREACHED.tsv) (the
package ledger) and the deleted files are marked in
[sweeps/TOTAL-SOURCE-SWEEP.tsv](sweeps/TOTAL-SOURCE-SWEEP.tsv) (the ADR-090
file ledger). The decision table Stevie answered is
[sweeps/REAL-STEEL-DECISIONS.md](sweeps/REAL-STEEL-DECISIONS.md).

## Decided 2026-10-26 (E1)

Stevie: "Apply all defaults", ADR-090 Accepted. Applied in S-REAL-STEEL Phase 1
as 16 commits (`e38cddf9`..`36ca8714`), one per group or family, each gated on
`go build ./...`, `go vet ./...` and the full `go test ./...` suite against the
pre-deletion baseline (295 ok / 1 pre-existing FAIL in `scripts/lib` before,
216 ok / the same 1 FAIL after; zero new failures).

**Deleted (108 package rows + 22 dashboard scripts + 5 `.go.dev` files + 2 symbols):**
G1 fake-success clients (`pkg/lxd`, `pkg/nix`, `pkg/certs/acme`,
`cmd/unheaded-daemon/internal/*`), G2 orphan dashboard scripts, G3 deploy
abstractions and `services/canary` (merged into Sword), G5 `transport.Connect`
and `services/wotan/internal/circuitbreaker` (drops `sony/gobreaker`), G10a
alerting, G10b certs, G10c legacy compliance engine, G10d `pkg/mesh/*` +
`services/hauberk`, G10f `pkg/config/*`, G10g ten Armory/Gnostic services,
G10h unused libraries + `pkg/health/aggregator.go`, F1 `pkg/audit/{export,logger,query}`,
`pkg/secrets/rotation`, `pkg/storage/{cache,kv,object}`, F2 `pkg/{container,dns,loadbalancer,mesh,scheduler}`,
F4 `pkg/config`, `pkg/testing`, `services/kenoma`.

**Kept, with the reason (PARK / KEEP groups):**

- G6 tests and test helpers (15): unreached by construction. `tests/e2e` lost
  `integration_test.go`, which exercised only the deleted F2 packages.
- G7 bare metal / Sabatons (6): Stevie's standing "parked".
- G8 protocol reference code (17, `pkg/protocol/*`, `pkg/monad`): reference
  code for frozen specs; deleting it is an RFC Editor decision. **Ruled
  2026-10-27** in `docs/protocol/G8-REFERENCE-RULINGS.md`: reference, by ruling
  (KEEP or KEEP-AFTER): `errors`, `tlv`, `pkg/monad`; wired in later phases:
  `sophiasync`, `integrity` (HMAC); decided in Phases 4/5: `settings`,
  `lifecycle`, `migration`; E3 delete rows: `amplification`, `dos`,
  `intermediary`, `prefetch`, `flowtype`, `sequence`, `registry`. A
  regeneration does not re-ask these.
- G9 control planes waiting for their eBPF half (10): see the `unwired:` notes
  in the list. `services/anomaly` left the list on 2026-10-23 (`cmd/anomaly`).
- **One decision table for G9, F5, F6, the G8 delete rows, `pkg/afxdp`,
  `cmd/ebpf-exporter`, `cmd/pqc-verifier`, `pkg/waf`, `lxd/` and the host
  stacks:** [docs/sweeps/TRUE-WIRE-DECISIONS.md](sweeps/TRUE-WIRE-DECISIONS.md)
  (S-TRUE-WIRE E3, 2026-10-28, with a dry run of its defaults).
- F3 `pkg/pqc_policy`: parked for S-QUANTUM-ARMOR (ADR-105).
- F5 `internal/bpfmap`, `pkg/ebpf/maploader`: still parked. Phase 13 (Step 340)
  wrote the `CHAOS_TARGETS` loader on cilium/ebpf (already a dependency): these
  two wrap bpf(2) map ops on an fd and can neither load an object nor attach a
  TCX link.
- F6 `services/loadbalancer`: control plane for `ebpf/maglev-ebpf`; Phase 2
  Step 92 fixes its Wotan join name.
- F7 `services/wotan/internal/compute`, `dashboard/js/doom-viewport.js`:
  parked for S-UPC-WIRE-COMPUTE.
- F8 `pkg/uids`: its tests enforce the uid registry; Sword uses `uids.Sword`.

**FLAGGED in Phase 1 (default was delete; the evidence said otherwise):**

- `pkg/audit/storage`: reached since the Phase 0 scan, imported by live
  `cmd/zhen-claude` and `cmd/zhen-agentd` (S-TWO-BRAINS, 2026-10-23).
- `pkg/secrets/store`, `pkg/secrets/encryption`: `store.MemoryStore` is the
  only `Store` implementation, so the live `pkg/secrets` Manager suite
  (`TestManager*`, 20 cases) is built on it; deleting it deletes the Manager's
  tests, not cases that only exercise `store`. `store` imports `encryption`.
- `pkg/trace`: the live-pipeline e2e cases use it as their trace-ID fixture
  (`String`, `FlowLabelHint`); only two cases exercise it alone. Deleting it
  means rewriting those cases.
- `pkg/waf/*` (7, G10e): deleted, then restored under Step 62: `tests/s77`
  (G6 KEEP) asserts `pkg/waf/rules/engine.go`, `clientip_test.go` and
  `detection/bot_test.go` exist (`TestPhase1_BugFixes/Bug17_XFF_Spoofing`,
  `TestS77_AllDeliverables`). Needs Stevie's word on amending that kept test.

New since the Phase 0 scan and unreached by design: `pkg/tls/pqtest`
(S-QUANTUM-ARMOR test helper) and `pkg/deploy/cuirassdriver/cuirasstest`
(Sword test helper).

## The list

- `cmd/dashboard-backend/jstest` (G6)
- `deploy/nftables` (G6)
- `internal/bpfmap` (F5, parked; Phase 13 used cilium/ebpf)
- `nix/tests` (G6)
- `pkg/baremetal` (G7)
- `pkg/baremetal/image` (G7)
- `pkg/baremetal/inventory` (G7)
- `pkg/baremetal/ipmi` (G7)
- `pkg/baremetal/pxe` (G7)
- `pkg/deploy/cuirassdriver/cuirasstest` (test helper, by design)
- `pkg/ebpf/maploader` (F5, parked; Phase 13 used cilium/ebpf)
- `pkg/metrics/metricstest` (G6)
- `pkg/monad` (G8)
- `pkg/pqc_policy` (F3, S-QUANTUM-ARMOR)
- `pkg/protocol/amplification` (G8)
- `pkg/protocol/dos` (G8)
- `pkg/protocol/errors` (G8)
- `pkg/protocol/flowtype` (G8)
- `pkg/protocol/fuzz` (G6)
- `pkg/protocol/integrity` (G8)
- `pkg/protocol/intermediary` (G8)
- `pkg/protocol/lifecycle` (G8)
- `pkg/protocol/migration` (G8)
- `pkg/protocol/prefetch` (G8)
- `pkg/protocol/registry` (G8)
- `pkg/protocol/sequence` (G8)
- `pkg/protocol/settings` (G8)
- `pkg/protocol/sophiasync` (G8)
- `pkg/protocol/tlv` (G8)
- `pkg/secrets/encryption` (FLAGGED: imported by `pkg/secrets/store`)
- `pkg/secrets/store` (FLAGGED: the live Manager tests are built on it)
- `pkg/tls/pqtest` (test helper, by design)
- `pkg/trace` (FLAGGED: e2e trace-ID fixture)
- `pkg/uids` (F8, keep)
- `pkg/waf` (FLAGGED: `tests/s77` asserts its files)
- `pkg/waf/detection` (FLAGGED, with `pkg/waf`)
- `pkg/waf/inspection` (FLAGGED, with `pkg/waf`)
- `pkg/waf/ip` (FLAGGED, with `pkg/waf`)
- `pkg/waf/ratelimit` (FLAGGED, with `pkg/waf`)
- `pkg/waf/response` (FLAGGED, with `pkg/waf`)
- `pkg/waf/rules` (FLAGGED, with `pkg/waf`)
- `pkg/wotan-client/mock` (G6)
- `scripts/host` (G6)
- `scripts/lib` (G6)
- `services/compliance` (G9; unwired: control plane for ebpf/compliance-ebpf)
- `services/firewall` (G9; unwired: control plane for ebpf/firewall-ebpf)
- `services/keymgmt` (G9; unwired: control plane for shield-ebpf PQC key maps)
- `services/keymgmt/handler` (G9; with `services/keymgmt`)
- `services/keymgmt/lifecycle` (G9; with `services/keymgmt`)
- `services/keymgmt/store` (G9; with `services/keymgmt`)
- `services/loadbalancer` (F6; unwired: control plane for ebpf/maglev-ebpf)
- `services/nfv` (G9; unwired: control plane for ebpf/nfv-ebpf)
- `services/qos` (G9; unwired: control plane for ebpf/qos-ebpf)
- `services/sabatons` (G7)
- `services/selfheal` (G9; unwired: control plane for ebpf/failover-ebpf)
- `services/version` (G9; unwired: control plane for ebpf/version-ebpf)
- `services/wotan/internal/compute` (F7, S-UPC-WIRE-COMPUTE)
- `tests/doom-memwrite` (G6)
- `tests/e2e` (G6)
- `tests/integration` (G6)
- `tests/pqc` (G6)
- `tests/s77` (G6)
- `tests/security/doom` (G6)
- `tomb/lich/harnesses` (G6)
