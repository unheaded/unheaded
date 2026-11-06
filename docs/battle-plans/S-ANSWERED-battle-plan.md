<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-ANSWERED: the work from Stevie's answers of 2026-11-06

**Date**: 2026-11-06
**Authority**: Stevie's word of 2026-11-06 lifts ADR-112's freeze for this plan only.
**Prerequisite**: develop at `2c701542`, pushed.
**Target**: every unattended answer built, reviewed and merged to develop; the red jobs of the first push explained and fixed or named.
**Mode**: unattended, repo only. Rules: `SPRINT-SERIES-2026-11-04.md`.
**Lanes**: three at a time, each built, independently reviewed, then closed and merged under the develop lock.

## Wave 1

| Lane | Work |
|---|---|
| `redci` | Find and fix the meta-gate shard 2/3 failure and the `fuzz_mbc_execute` nightly failure of the first push. Remove the stale `helm.yml` entry, drop arm64 from `docker.yml`, delete the 16 dead Dockerfiles in `docker/services/`, turn off `plan.yml` and `apply.yml`. Make `docker.yml` ready to run, since Stevie is enabling it. Tidy: Jenkinsfile docker socket, `Jenkinsfile.protocol` buf version, Trivy ignore-unfixed, raft demo docs. Doom drawio cells, PDF and the `clean-artifacts` WAD warning move to `unheaded-doom`. |
| `zhen` | Zhen web UI login through the existing issuer. A run is tainted once it reads tool output; signed manifest; the validatePath deny becomes write-only; dry runs, pause and DispatchStanding stay allowed. Card opt-in becomes a flag only an authenticated operator can set. Password floor 15; rooms get a quota. |
| `retrieval` | DP-719 option A, replacing the 2026-11-04 answer: delete `tomb/grimoire/rag/`, keep the threat-model and MITRE corpus, serve search through Zhenai's Go retrieval. ChromaDB leaves the tree; FND-032 closes with proof. |

## Wave 2

| Lane | Work |
|---|---|
| `handlers` | One shared JSON helper that returns 500 when a value will not marshal, across the 112 handlers. huginn refuses a wildcard bind. `AUTH_AUDIT_LOG` rotation and the unit path. Health reports take the reporter from the token (with H6); `--remediate` stays off; Akira's names are capped. |
| `deploy` | Object trust check in the Rust loaders, with an opt-out flag. The deb that loads the yaldabaoth and heimdall objects installs them root-owned. West upgrades from the built deb inside `rolling-upgrade.yaml`; repo compared with build before any host moves; a real rollback. Nix: drop the dead firewall rules, fix the Loki URL. Pin the registry's current digests, except the Well's, which waits for a passing drill. |
| `evidence` | Mark the old license documents superseded; a version bump fails until `sbom/` is regenerated; note INCOMPLETE on current matrices only. Generalise the register text; add register rows for the public repo and branch protection; reopen FND-004; rewrite FND-025's verify for hop-ebpf; rewrite A1 to match how commits are really signed; add a `declared` status; delete `audit_completeness`; hop-ebpf key 4 refuses enforce until FND-031, 033 and 035 close. Correct `CLAUDE.md`. Change `.github/BRANCH_PROTECTION.md` to required checks with no review count. Rewrite the 2026-07-29 secrets decision: every value in history counts as burned. Parked ADRs: BIND zone files and a locked-down certificate authority (136); the Doom Range port check and dashboard links move to `uhl` once it boots with a websocket TTY (137). |

## Not in this plan

At S-E1-LIVE, with Stevie: Well verify-full, protocol-api on loopback, credential rotation, the BTF attr fix, one downgrade per host, west to lab-hardened after east, `wotan-ctl boot` and load-rom. Waiting on ADR 136: chaos-controller's certificate. Waiting on M6: FND-026's verify. On GitHub, Stevie's hands: enable CI, Docker, eBPF & Rust and Release; branch protection after the first green CI.
