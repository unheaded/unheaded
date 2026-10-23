<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# REAL-STEEL E1: what to do with code nothing runs (one pass)

S-REAL-STEEL Phase 0, 2026-10-23, at `f7d730e7`. Input for
[ADR-090](../adr/ADR-090-total-source-sweep-slop-removal.md); the package list
is [UNREACHED-GO-PACKAGES.md](../UNREACHED-GO-PACKAGES.md).

**Nothing has been deleted.** Phase 1 deletes only what this table says to.

## How to answer

Fill the **Answer** column with one letter per row:

| Letter | Means |
|---|---|
| Y | take the recommendation |
| N | keep it as it is |
| P | park it (keep, plus a one-line "unwired: why" note in UNREACHED-GO-PACKAGES.md) |
| W | wire it in this sprint |

A blank answer means the **Default** column. Every row has one, so leaving
the whole table blank is a valid answer: it applies every recommendation.

Evidence per package (importers, fake-success markers, reflection, last
commit, non-Go references) is in [REAL-STEEL-UNREACHED.tsv](REAL-STEEL-UNREACHED.tsv),
one row per package or file, 178 rows. The scan is
`scripts/sweep/unreached-evidence.sh`; the grouping rules are D-02 in the
[battle plan](../battle-plans/S-REAL-STEEL-battle-plan.md).

## Headline

| Recommendation | Rows |
|---|---|
| Delete (G1, G2, G3, G5, G10) | 99 |
| Merge into Sword, then delete (services/canary) | 1 |
| Wire this sprint (G4) | 3 |
| Keep: tests and test helpers (G6) | 15 |
| Park (G7 bare metal, G8 protocol reference, G9 control planes) | 34 |
| Flagged, answered one by one below | 26 |

## Groups

| # | Group | Packages / files (count) | Evidence | Recommendation | Default | Answer |
|---|---|---|---|---|---|---|
| G1 | Fake success, superseded | cmd/unheaded-daemon/internal/{config,ebpf,lxd,state}, pkg/lxd (+ real_client.go.dev), pkg/container/lxd/lxd.go.dev, pkg/nix (only caller of pkg/lxd), pkg/certs/acme (9) | Whole files read: pkg/lxd `RealClient` returns success without I/O; acme's account URL is `"placeholder"`, challenges are marked valid, the certificate is nil, revoke is a no-op; the daemon's internal/lxd `NewClient` ignores its config and returns a mock. Cuirass uses `lxd_rest.go`. | DELETE | Y | |
| G2 | Orphan dashboard scripts | 22 files in dashboard/js: anomaly-heatmap, canary-panel, chaos-panel, compliance-panel, failover-panel, firewall-panel, int-heatmap, latency-chart, lb-panel, metrics, monad-decode, nfv-panel, packet-flow-diagram, 5 x pqc-*, qos-panel, trace-search, trace-viewer, version-panel (22) | No .html or .js loads them. monad-decode.js invents register values, a random CRC and a random 5% "checksum invalid". (doom-viewport.js, the 23rd, is F7.) | DELETE (chaos/canary panels get rebuilt on real APIs later, not revived) | Y | |
| G3 | Deploy abstractions the sprint does not use | pkg/deploy (Engine), pkg/deploy/pipeline, pkg/deploy/artifact, pkg/container/{image,network,storage}.go.dev, services/canary (7) | `deployRolling` sleeps 100 ms per replica over the mock runtime; five "Simulate" pipeline stages; canary's metrics are hardcoded. One deploy core is `pkg/deploy/strategy` (D-05). pkg/container itself is F2. | DELETE; canary MERGE into Sword first (Step 233) | Y | |
| G4 | Wired by this sprint | pkg/deploy/strategy, pkg/deploy/rollback, services/sword (3) | Phases 7-8 | WIRE | W | |
| G5 | Wotan dead paths | services/wotan/internal/circuitbreaker (the only user of `sony/gobreaker`, so go.mod loses a module), `transport.Connect` in pkg/transport/cascade.go (2) | Zero callers. Connect is named only in two comments; CLAUDE.md's "all services use transport.Connect" is false. (services/loadbalancer moved to F6.) | DELETE | Y | |
| G6 | Tests and test helpers | tests/{e2e,integration,pqc,s77,doom-memwrite,security/doom}, tomb/lich/harnesses, nix/tests, deploy/nftables, scripts/{host,lib}, cmd/dashboard-backend/jstest, pkg/protocol/fuzz, pkg/metrics/metricstest, pkg/wotan-client/mock (15) | Test files only, or a helper that live tests import. Unreached by construction, not slop by this metric. | KEEP | N | |
| G7 | Bare metal / Sabatons | pkg/baremetal, pkg/baremetal/{image,inventory,ipmi,pxe}, services/sabatons (6) | Your standing "parked" | PARK | P | |
| G8 | Protocol reference code | pkg/protocol/{amplification,bpfschema,dos,encoding,errors,flowtype,integrity,intermediary,lifecycle,migration,prefetch,registry,sequence,settings,sophiasync,tlv}, pkg/monad (17) | Reference code for frozen specs; deleting it is a spec decision (RFC Editor) | PARK | P | |
| G9 | Control planes waiting for their eBPF half | services/{anomaly,compliance,firewall,nfv,qos,version} (matching ebpf/*-ebpf), services/selfheal (failover-ebpf), services/keymgmt + handler/lifecycle/store (shield-ebpf PQC key maps) (11) | A live eBPF program exists; nothing feeds or reads it from userspace yet | PARK + "unwired: control plane for ebpf/x" note | P | |
| G10a | Alerting engine | pkg/alerting, pkg/alerting/{manager,notify,rules} (4) | No caller; alerting is vmalert + alert-mailer (ADR-100) | DELETE | Y | |
| G10b | Certificate library | pkg/certs, pkg/certs/{ca,issue,rotation,store} (5) | No caller (acme is G1) | DELETE | Y | |
| G10c | Legacy compliance engine | pkg/compliance (engine.go), pkg/compliance/{audit,controls,report,standards} (5) | No caller; the live path is pkg/compliance/crosswalk and friends (ADR-097/098), which stay. Commit c5e3cdf1 already calls it "legacy". | DELETE | Y | |
| G10d | Service mesh (Hauberk) | pkg/mesh/{discovery,loadbalance,loadbalancer,mtls,observe,policy,proxy,resilience,routing}, services/hauberk (10) | No caller; goes with F2's pkg/mesh | DELETE | Y | |
| G10e | Go WAF reference | pkg/waf, pkg/waf/{detection,inspection,ip,ratelimit,response,rules} (7) | No caller; the WAF that runs is the Rust `cmd/waf` | DELETE | Y | |
| G10f | Config sub-packages | pkg/config/{merge,sources,template,watch} (4) | Imported only by pkg/config (F4); go with its answer | DELETE | Y | |
| G10g | Armory/Gnostic services with no live counterpart | services/{anamnesis,cape,cloak,cuirass,gauntlets,pauldrons,pleroma,tassets,telemetry,vambraces} (10) | No binary imports them. services/cuirass is superseded by Cuirass itself (`cmd/unheaded-daemon`). | DELETE | Y | |
| G10h | Unused libraries | internal/dashboard, internal/ebpf/collector, pkg/{eventbus,events,http,netlink,network,runtime,service,state,telemetry,tracing,validation,worker}, pkg/health/aggregator.go (15) | No caller. pkg/state is a second reconciler (S17 patched it for parity; still nothing runs it). pkg/service is the "service template" CLAUDE.md lists. pkg/telemetry has a `replace` line in go.mod. aggregator.go's `HealthStatus` is used by pkg/health/response.go, which is also uncalled: delete both. | DELETE | Y | |

## Flagged rows (one answer each)

| # | Package / file | What is unknown | Suggestion | Default | Answer |
|---|---|---|---|---|---|
| F1 | pkg/audit/export | The live pkg/audit's tests import it | Delete it and the pkg/audit test cases that only exercise it | Y | |
| F1 | pkg/audit/logger | same | same | Y | |
| F1 | pkg/audit/query | same | same | Y | |
| F1 | pkg/audit/storage | same | same | Y | |
| F1 | pkg/secrets/encryption | The live pkg/secrets's tests import it | Delete it and those test cases | Y | |
| F1 | pkg/secrets/rotation | same | same | Y | |
| F1 | pkg/secrets/store | same (also the known `TestFileStore_WatchDelete` race) | same | Y | |
| F1 | pkg/storage/cache | The live pkg/storage's tests import it | Delete it and those test cases | Y | |
| F1 | pkg/storage/kv | same | same | Y | |
| F1 | pkg/storage/object | same | same | Y | |
| F2 | pkg/container | tests/e2e (kept) imports it | Delete it and the e2e cases that only exercise it | Y | |
| F2 | pkg/dns | tests/e2e imports it | same | Y | |
| F2 | pkg/loadbalancer | tests/e2e imports it | same | Y | |
| F2 | pkg/mesh | tests/e2e imports it (its sub-packages are G10d) | same | Y | |
| F2 | pkg/scheduler | tests/e2e imports it | same | Y | |
| F2 | pkg/trace | tests/e2e imports it | same | Y | |
| F3 | pkg/pqc_policy | tests/pqc (kept) imports it; PQC policy is S-QUANTUM-ARMOR's ground (ADR-105) | Park; let that sprint decide | P | |
| F4 | pkg/config | Uses reflect (struct walking) | Delete: the reflect use is internal, no linkname, //export or cgo | Y | |
| F4 | pkg/testing | Uses reflect (assert helpers); no importer at all | Delete | Y | |
| F4 | services/kenoma | One reflect.DeepEqual | Delete | Y | |
| F5 | internal/bpfmap | Typed pinned-map access; Phase 13 needs a userspace writer for `CHAOS_TARGETS` and none exists | Park until Phase 13, delete then if unused | P | |
| F5 | pkg/ebpf/maploader | Fills BPF maps from Go structs; same Phase 13 question | Park until Phase 13 | P | |
| F6 | services/loadbalancer | The plan template put it in G5 (xdpsync publishes before joining Wotan), but it is the control plane for ebpf/maglev-ebpf (rule R8) | Park; Phase 2 Step 92 then fixes its join name and allowlist | P | |
| F7 | services/wotan/internal/compute | Wotan memory service for Doom-over-IPv6; Sprint 5 is compute on the wire | Park for S-UPC-WIRE-COMPUTE | P | |
| F7 | dashboard/js/doom-viewport.js | No page loads it, but dashboard-backend's doom.go serves frames for it | Park for S-UPC-WIRE-COMPUTE | P | |
| F8 | pkg/uids | No importer, but its own tests enforce the uid registry against compose and Helm (like pkg/ports); Sword (D-08) uses `uids.Sword` | Keep | N | |

## One more question

**ADR-090 status**: accept it now, or keep it Proposed? If accepted, D-01's
override applies: Phase 1 also creates `docs/sweeps/TOTAL-SOURCE-SWEEP.tsv`
and marks these packages' files. Default: keep Proposed (this table is a
package-level tranche; the 1,836-file campaign is a separate decision).

Answer: ______

## What Phase 1 does with the answers

- One commit per group or family, leaves first; `go build ./...`, `go vet` and
  the full test baseline after each. A new failure restores that family and
  flags it.
- Every non-Go reference in the ledger's notes (wiki, docs, Makefile, compose,
  .golangci.yml, CLAUDE.md) is edited in the same commit. Generated inventories
  (`docs/legal/scancode-full.json`, `compliance/crypto/cbom.cdx.json`) are
  regenerated, not hand-edited. History docs (`docs/archive`, `docs/history`,
  session logs) are left as written.
- Deleting G5's circuitbreaker removes `github.com/sony/gobreaker` from go.mod;
  deleting pkg/telemetry removes its `replace` line.
