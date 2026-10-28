<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# TRUE-WIRE E3: delete, keep or un-park (one pass)

S-TRUE-WIRE Phase 6 (Steps 238-247), 2026-10-28, on `sprint/true-wire`. The
open NEXT-7 decisions 42-44 in one table. Same answer letters as
[REAL-STEEL-DECISIONS.md](REAL-STEEL-DECISIONS.md):

| Letter | Means |
|---|---|
| Y | take the default |
| N | keep it as it is |
| P | park it (keep, with a one-line "unwired: why" in UNREACHED-GO-PACKAGES.md) |
| W | wire it (un-park): a `cmd/` per the `cmd/anomaly` recipe, a unit, a uid and a port |

A blank Answer means the Default. Leaving the whole table blank applies every
default. Nothing below is deleted until this is answered (Step 248 runs only
after E3).

**Evidence** (Step 238, regenerated 2026-10-28): 68 Go packages are reached by
no `main` package (`go list -deps` over every main); for each row, the count of
non-test and test files, the last commit, and the eBPF program that is its
other half. `cmd/*` rows are binaries: their status is
[LIVE-PATHS.md](../LIVE-PATHS.md) (ORPHAN = no unit, no compose service). The
eBPF load gate's known-failing list is empty: every eBPF program loads.

## Protocol reference code (NEXT-7 decision 42 context; G8 rulings)

| # | Path | Evidence | Default | Consequence of the default | Answer |
|---|---|---|---|---|---|
| R1 | pkg/protocol/amplification | unreached; 1 file + 1 test; last 2026-03-15 | DELETE (G8 Step 69) | none in a binary; the draft text is unaffected | |
| R2 | pkg/protocol/dos | unreached; 1+1; last 2026-05-10 | DELETE (G8) | same | |
| R3 | pkg/protocol/intermediary | unreached; 1+1; last 2026-05-07 | DELETE (G8) | same | |
| R4 | pkg/protocol/migration | unreached; 1+1; last 2026-05-10; G8 ruled it "decided in Phase 5": it generates the retry tokens Shield's RETRY_TOKENS map would hold | PARK until E8 (P) | E8 decides whether Shield keeps retry tokens; delete with RETRY_TOKENS if not | |
| R5 | pkg/protocol/prefetch | unreached; 1+1; last 2026-07-29 | DELETE (G8) | same | |
| R6 | pkg/protocol/lifecycle | unreached; 1+1; GOAWAY tracker for a frame no current draft defines | DELETE (Phase 4 Step 150) | Shield's GOAWAY map already removed (Step 207) | |
| R7 | pkg/protocol/settings | unreached; 1+1; foundation-06 has no SETTINGS registry | DELETE (Step 150; overrides the plan's KEEP, whose premise was false) | answer N to keep it as a conformance reference | |
| R8 | pkg/protocol/flowtype | unreached; 1+1; last 2026-05-07; packs a 2-bit flow type into Monad flag bits 1:0, which foundation-06 registers as CUSTOM and RSVD | DELETE (G8 ruling, overrides Step 69's KEEP) | none; no importer | |
| R9 | pkg/protocol/sequence | unreached; 1+1; last 2026-05-10; not the PQC draft's per-flow SeqNum rule | DELETE (G8 ruling) | none; no importer | |
| R10 | pkg/protocol/registry | unreached; 1+1; last 2026-03-15; a generic code-point type no draft specifies | DELETE (G8 ruling) | none; no importer | |

## Control planes with no binary (NEXT-7 decision 44; G9)

Their eBPF halves stay either way (rows E1-E6): the load gate tests them, and
`qos-ebpf` is the reference the cartridge rate limiter is checked against.

| # | Path | Evidence | Default | Consequence of the default | Answer |
|---|---|---|---|---|---|
| C1 | services/compliance | unreached; 1+2; last 2026-07-29; half: ebpf/compliance-ebpf | DELETE | compliance-ebpf keeps loading in the gate with nothing feeding it (already true) | |
| C2 | services/firewall | unreached; 1+1; half: ebpf/firewall-ebpf | DELETE | same for firewall-ebpf | |
| C3 | services/nfv | unreached; 1+1; half: ebpf/nfv-ebpf | DELETE | same for nfv-ebpf | |
| C4 | services/qos | unreached; 1+2; last 2026-05-07; half: ebpf/qos-ebpf | DELETE | qos-ebpf stays (cartridge parity reference) | |
| C5 | services/selfheal | unreached; 1+1; half: failover-ebpf | DELETE | same | |
| C6 | services/version | unreached; 1+1; half: ebpf/version-ebpf | DELETE | same | |
| C7 | services/keymgmt (+handler, lifecycle, store) | unreached; 1+2 plus three sub-packages; last 2026-07-29 | KEEP-PARKED (P) | ADR-103's issuer and key distribution may want it; un-parking is also what decision 43's "wire OPTIMISTIC" needs | |
| C8 | services/loadbalancer | unreached; 2+4; last 2026-10-27 (S24 touched it); control plane for ebpf/maglev-ebpf (F6) | KEEP-PARKED (P) | maglev Phase 2 | |
| C9 | services/sabatons | unreached; 1+1; bare metal (G7) | KEEP-PARKED (P) | your standing "parked" (REAL-STEEL G7); the plan suggested DELETE, not taken without you | |

## eBPF halves (listed so a "delete the service" answer is not read as "delete the program")

| # | Program | Evidence | Default | Consequence | Answer |
|---|---|---|---|---|---|
| E1 | ebpf/compliance-ebpf | loads (gate PASS) | KEEP | none | |
| E2 | ebpf/firewall-ebpf | loads | KEEP | none | |
| E3 | ebpf/nfv-ebpf | loads | KEEP | none | |
| E4 | ebpf/qos-ebpf | loads; cartridge parity reference | KEEP | none | |
| E5 | ebpf/failover-ebpf | loads | KEEP | none | |
| E6 | ebpf/version-ebpf | loads | KEEP | none | |

## Libraries and binaries (NEXT-7 decisions 42-43)

| # | Path | Evidence | Default | Consequence of the default | Answer |
|---|---|---|---|---|---|
| L1 | pkg/afxdp | builds only with `-tags afxdp` (cgo + a Rust library); nothing in the repo passes that tag; no importer; last 2026-03-25 | DELETE (retire Decision 5's AF_XDP product path) | docs/BENCHMARKS.md's AF_XDP line gets "(Python sendto ceiling; AF_XDP path retired)" (Step 255); the supervised XDP/TC path is unaffected | |
| L2 | internal/bpfmap | unreached; 1+2; REAL-STEEL F5 parked it for Phase 13's writer, which uses aya | DELETE | none | |
| L3 | pkg/ebpf/maploader | unreached; 1+3; F5, same | DELETE | its docs go with it | |
| L4 | cmd/ebpf-exporter | ORPHAN | DELETE | none deployed | |
| L5 | cmd/pqc-verifier | ORPHAN; `POST /verify` and `/verify/sovereign` on :19008 | DELETE (decision 43 default: until keymgmt is un-parked) | wiki/PQC-Authentication.md drops its row (Steps 232-234 already say "design only" for the XDP fast path) | |
| L7 | ebpf/pqc-common | no consumer since shield-ebpf dropped it (Step 233); the draft's PqcValue layout and flag helpers, 8 host tests | KEEP-PARKED (P) | it is what "wire OPTIMISTIC" (decision 43) would build on | |
| L6 | cmd/shield + services/shield | ORPHAN; an HTTP WAF daemon unrelated to shield-ebpf, with no auth on its rule API (found at Step 183, E8) | ASK | not in the plan's set; listed because E8 needs to know whether it stays | |

## Trees (NEXT-7 decision 42)

| # | Path | Evidence | Default | Consequence of the default | Answer |
|---|---|---|---|---|---|
| T1 | pkg/waf (+detection, inspection, ip, ratelimit, response, rules) | unreached; the WAF that runs is the Rust `cmd/waf`; REAL-STEEL G10e answered Y but it stayed because `tests/s77/s77_verification_test.go:73-76` asserts `pkg/waf/rules/engine.go` and its client-IP test exist | ASK (decision 42) | delete the 7 packages and the four s77 assertions together | |
| T2 | lxd/ (90 files) | container definitions superseded by Cuirass (`cmd/unheaded-daemon` on LXD); last touched 2026-10-27 | ASK | `scripts/east-bootstrap.sh` points at `lxd/hosts/host-b/` templates (WireGuard, launch-minimal.sh): deleting lxd/ breaks that script's references; move the two templates first | |
| T3 | docker/hosts/{host-a,host-b} (17 files) | the per-host compose stacks of FND-009 D11 | ASK | FND-009's D11 host-network decision closes with the deletion | |

## Dry run of the default set (Steps 241-243, 2026-10-28)

Every DEFAULT=DELETE row (R1-R3, R5-R10, C1-C6, L1-L5), and R4 (a superset:
R4's default is now PARK) removed in a scratch worktree
of `sprint/true-wire`, together with the one file outside them that imports a
deleted package: `tomb/lich/harnesses/lich_006_real_lifecycle_test.go`
(LICH-006's real-target harness for `pkg/protocol/lifecycle`; it goes with R6).

| Check | Result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go test ./...` vs the Phase 0 baseline | 218 ok, no FAIL (the baseline's `pkg/secrets/store` flake did not recur); 17 fewer packages than the full tree, exactly the deleted ones |
| `go mod tidy -diff` | empty: no module leaves go.mod |
| R8-R10 added afterwards | build, vet, `go test ./pkg/protocol/... ./tomb/...` and tidy clean again (no importers) |
| other references | `scripts/check-gosec-ratchet.sh` names `internal/bpfmap/bpfmap.go:21` in a comment only (an example); nothing in workflows, units or compose references a deleted path |

Rows marked ASK (L6, T1-T3) were not part of the dry run.
