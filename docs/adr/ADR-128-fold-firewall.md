<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-128: Fold the firewall plane into Cuirass, as intent only

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-127, ADR-129 to ADR-132, TRUE-WIRE C2 and E2.

## The pitch (decision pack, DP-203)

> Firewall ... into cuirass.

## What is there

`services/firewall`: HTTP on 19010, no auth; rules, `/api/v1/connections`, `/api/v1/stats`, `/api/v1/policy`, a conntrack aggregator publishing `firewall.conntrack`. No BPF map is touched: the aggregator summarises an in-memory table nothing fills. `ebpf/firewall-ebpf`: `firewall_xdp`, 33,795 verified instructions, CONNTRACK 1,000,000 entries, POLICY_CACHE, attached by nothing.

## Verdict: right for intent, wrong for enforcement

Cuirass holds desired state, so policy intent belongs there: it has auth, TLS and `/api/v1/state/desired`. But four facts stop it being the whole answer:

1. **Cuirass cannot write maps.** `deploy/systemd/unheaded-daemon.service` has an empty `CapabilityBoundingSet` and `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`. Adding `CAP_BPF` to the process that holds the LXD client certificate widens the worst blast radius in the Kingdom.
2. **Cuirass runs once; XDP runs per node.** east needs rules too.
3. **Routes collide.** firewall and qos both register `/api/v1/policy` and `/api/v1/stats`; all five planes register `/health`, `/ready`, `/metrics`. Each plane must expose a handler and mount under a prefix, `/api/v1/firewall/...`.
4. **`Start` binds an open listener.** A fold must not call it.

## The node writer question (shared by ADR-128 to ADR-132)

Intent in Cuirass needs a per-node process with `CAP_BPF` to write the maps. Five planes have maps to write: firewall, qos, loadbalancer, failover and nfv. It gates four folds, or five if NFV stays in Cuirass (ADR-130), so decide it once. Candidates:

| Candidate | For | Against |
|---|---|---|
| ADR-125's Go writer (`pkg/hoprules`, `unheaded hop rules apply`) over `pkg/ebpf.PinnedMap`, generalised | No new module; loads nothing, only writes pins; a `Type=oneshot` unit on a timer with `CAP_BPF` alone | A unit and a uid per node; polls desired state rather than reacting; exists only after S-WIRE-HONEST |
| `cmd/ebpf-loader --watch` | It already pins maps, and it stays resident: it blocks on `rx.recv()` to hold its attachments | It is a TOOL: no unit runs it. Rust, with no Wotan or HTTP client: a watch mode needs new crates, a rule 4 confirm for Stevie |
| `cmd/trace-collector-go` | Runs live with `CAP_BPF` | A tracer should not enforce |

**Recommendation:** the Go writer: nothing new enters the tree. It is Stevie's call.

## Decision direction

Flag `CUIRASS_FEATURE_FIREWALL=true`, default off: mounts policy routes under `/api/v1/firewall/`, behind the existing auth. The conntrack aggregator does not run in Cuirass; conntrack is a node fact. When no data is in, the API says so (ADR-127's rule).

## Open questions

1. The node writer, above.
2. Does firewall-ebpf overlap the host nftables work (DP-22)? One default-deny source of truth?
