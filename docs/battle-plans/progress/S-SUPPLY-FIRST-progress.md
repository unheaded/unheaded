<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-SUPPLY-FIRST progress

Plan: `docs/battle-plans/S-SUPPLY-FIRST-battle-plan.md`.

## For Stevie

1. ChromaDB has no release free of PYSEC-2026-311, -3813, -3814, -3815 (`last_affected` 1.5.9). All four are server endpoints or the RBAC provider; Grimoire runs the embedded client. Default taken: `chromadb==1.5.9`, tracked as FND-032. The file stays outside `check-python-audit.sh` because five other lines are ranges, so `python-audit-accepted.txt` does not grow. Appendix D of the plan pins those five and adds four accepted lines if you say so; the other way out is replacing ChromaDB in Grimoire. Dependabot #119 and #120 stay open either way.
2. Veto list: `docs/legal/RELICENSE-CANDIDATES-2026-11-04.md`, untouched until you answer.
3. Grimoire's full hash lock needs 101 ADR-096 admissions, or a CPU-index decision that drops the 19 CUDA wheels.
4. golang.org/x/crypto is on an untagged pseudo-version until the next tag (no tag carries the GO-2026-5932 fix; `openpgp/` removal is the only difference from v0.58.0).
5. `tomb/ansible/playbook-grimoire.yml` deploys `files/rag-query.py`, which does not exist; `failed_when: false` hides it. Pre-existing, left alone.
6. torch stays on 2.10.0 in `raft/` and the pip freeze although 2.14.1 audits clean, against DP-722's wording: DP-25.7 put the ROCm move and its training smoke after E1. Grimoire's CPU floor rose to 2.14.1.
7. This plan runs eight lane branches inside one plan branch, past the letter of rule 1, on S-LOW-FRUIT's precedent.
8. ruff, pytest and oxlint are not vulnerability bumps, so rule 4's approval does not literally cover them.
9. The embedded Grimoire scripts use Chroma's default embedding, which downloads an ONNX model on first use and cannot work on an air-gapped VM. Probably older than this bump, not verified.

## Stuck

(none yet)

## Lane reports

- Phase 5 (lane `ci`, Steps 57-62): exit gate passed on `s-supply-first-ci` at `fdcb4fbe` (signed). Step 57 DP-415 confirmed (docker.yml 2, security.yml 3, legacy pin 0). Dependabot cooldowns read back: github-actions 14, docker 7, gomod 14, cargo 14; no auto-merge. ADR-004 has "Brand alignment" and "Hiring friction" amended; ADR-004 grep count 1 (the amendment note); all 149 action uses pinned.
- Phase 7 (Steps 69-71, lane `licence`): DONE. `docs/legal/RELICENSE-CANDIDATES-2026-11-04.md` written (ten rows); Step 70 verify matched expected output exactly; committed signed as `6805264a` on `sprint/s-supply-first-licence`. Nothing outside the list was edited.
- Phase 10 (Steps 83-88, lane `scanner`): DONE. Red as predicted, green 4 PASS + ok, gosec-scan built, committed signed as `9d5fdc69` on `sprint/s-supply-first-scanner`; tree clean.

- Phase 2 (rust, Steps 25-29): DONE 2026-11-04. Step 26 clean before (`TOTAL locks=24 failing=0`, rc=0); Step 28 clean without the ignores (same output). Three audit.toml files removed, commit `08fa9b8a` (signed). cmd/trace-collector/.cargo/audit.toml left in place for DP-210. Phase 2 exit gate: passed.

- Phase 8 (Steps 72-78, lane `licence`, haiku agent): PARTIAL, exit gate NOT passed. Commit `231b77e2` (signed) on `sprint/s-supply-first-licence` covers Steps 74-75 only: 10 comment-header edits (`check` OK 10 replacements) and 22 prose edits (OK 22 replacements). Step 77 green: 36 monitoring and observability YAML files parse, `check-spdx.sh --langs go,rs --forbid MIT` OK, scope diff 0 lines.

## Stuck

- **[STUCK] Step 72** (25 image labels). Output: `26` where the step expects `25`. Files: the 25 `docker/` Dockerfiles plus `docs/battle-plans/S-SUPPLY-FIRST-battle-plan.md`, which quotes the label string. The step's `git grep` does not exclude `docs/battle-plans/`. Recovery: `git checkout -- docker` and `git checkout -- docs/battle-plans/S-SUPPLY-FIRST-battle-plan.md` in the licence worktree; the worktree is clean at `231b77e2`. No label was changed.
- Knock-on: Step 73 and Step 76 cannot pass while the labels are unchanged. Step 76 measured 30 remaining MIT files (25 docker labels plus the 5 expected `lxd/containers/` files). Step 78's exit gate is not met, so the Step 78 commit was not made as written. Steps 74-75 were committed as above instead, under the Stuck Protocol.
- Fix for the plan author (not applied here): Step 72's grep needs `-- docker` or a `:!docs/battle-plans` pathspec, then the count should be 25.

## For Stevie

- Phase 8 Step 72's grep count is 26, not 25: the plan document quotes the label string. The 25 docker labels still need the MIT to GPL change once Step 72 is corrected and rerun.

- Phase 8 RESCUE (Opus, Steps 72-73, 76-78, lane `licence`): DONE, exit gate passed, supersedes the PARTIAL entry and the Step 72 [STUCK] above. Commit `e5258724` (signed) on `sprint/s-supply-first-licence`; tree clean. Re-checked `231b77e2` first: all 32 spec edits landed, `p8a.spec` and `p8b.spec` match the plan byte for byte, the commit touches exactly the spec's files. Literal text changed, plan file not edited:
  - Step 72: `git grep -l '...' -- . ':!docs/battle-plans'` instead of no pathspec. Output `25`; `labels.txt` rewritten with those 25, all under `docker/`. The plan document was the only other hit.
  - Step 73: same pathspec on the first check, which the stuck report missed: as written it prints `1` even on a correct tree, because the plan quotes the label four times. Output `0`, then `25 +...GPL-3.0-or-later"` and `25 -...MIT"`. Unnormalised diff: no changed line other than the label.
  - Step 78: same file list, subject changed, because the commit holds only the labels; headers and prose were already in `231b77e2`.
  - Step 76 as written: exactly the five `lxd/containers/` files. Step 77 as written: `yaml ok 36`, SPDX gate OK, scope diff `0`.

- Phase 9 (Steps 79-82, lane `licence`, haiku agent): DONE, exit gate passed. Commit `5caf44f4` (signed) on `sprint/s-supply-first-licence`; tree clean. Step 79: `OK 19 replacements`. Step 80: `zhend`, `zhenai-forge`, `collector` all read GPL-3.0-or-later; 0 GPL-3.0-only left in `crates/zhend` and `crates/zhenai-forge`; 0 Cargo.lock changes; 0 non-GPL-3.0 changed lines in `zhend` src/tests/proto. Step 81: four GPL-2.0-only fields untouched. The commit used the plan helper unchanged, so its trailer names Opus 5.5 as the series rule 2 requires; the harness attribution for this session names Haiku 5.5. Not in unsigned.txt.
- Phase 1 (lane `go`, Steps 15-24): DONE. Commit `4d1062f7` on `sprint/s-supply-first-go`, signed. govulncheck clean, go test -race SAME as baseline, dependency-baseline 590 PASS, sbom go_components 149, dep-staleness OK (3 stale), mod-integrity OK, tree clean.

- Phase 4 (lane `grimoire`, Steps 42-56): DONE 2026-11-04, exit gate passed on `sprint/s-supply-first-grimoire`. Step 42 listed the 10 measured advisories, no extras; Step 45 left only the four chromadb ids. Throwaway non-root container smoke (Step 49) matched line for line, rc=0, so Step 50 did not run: index, re-index, list, query and missing-collection paths all work on chromadb 1.5.9, sentence-transformers 6.2.0, transformers 5.20.2, torch 2.14.1 CPU. Step 51: 590 packages in baseline, 2 accepted vulnerabilities, python_components 77. FND-032 opened; findings delta=1 (154 to 155, read with `-evidence` per rule 16). Commits `b3543873` and the Steps 53-56 commit on the lane branch, both signed. No live host touched.
- Phase 4 limit, for Stevie: the smoke ran `rag-index.py` and `rag-query.py` only. The scripts embedded in `tomb/ansible/playbook-grimoire.yml` and `tomb/provision.sh` moved to `PersistentClient` too, but Step 47 only compiled them; they have not run against chromadb 1.5.9 (see For Stevie 9 on their default embedding). Lane tip: `0cc85a74`.

- Phase 3 (lane `pyci`, Steps 30-41, sonnet): DONE, exit gate passed. Commits `20022e41` (ruff 0.17.3, pytest 9.2.0, re-lock; no transitive moves) and `1a2f16d5` (oxlint 1.90.0), both signed, on `sprint/s-supply-first-pyci`; tree clean. Freshness rc=0, no STALE; ruff 0 findings; pip-audit 2 accepted; oxlint 21 files no findings; both gates-can-fail PASS.

- Phase 11 (Steps 89-94, lane `scanner`, sonnet): DONE, exit gate passed. Red as predicted (`build failed`, `undefined: ReadGoMod`); green 5 PASS + ok; dependency baseline `[PASS] 590`, sbom trend 3 OK. Commit `c0a17bfa` (signed) on `sprint/s-supply-first-scanner`; tree clean.

- Phase 12 (Steps 95-101, lane `scanner`, sonnet): DONE, exit gate passed. Red as predicted (`build failed`, `undefined: config`); green 3 PASS + ok x5; golangci-lint `0 issues.`, gosec=0; binary output matched (rc=2 without --osv-dir; 4 records, 0 findings, 20 not an exact pin). Commit `3a1f1b55` (signed) on `sprint/s-supply-first-scanner`; tree clean.

- Phase 13 (Steps 102-108, lane `scanner`, haiku agent): DONE, exit gate passed on `sprint/s-supply-first-scanner`. Step 102: `vulnmatch: N components, 4 records, 0 findings (0 in KEV), 20 not an exact pin`. Step 103: `OK: 58 man pages in man/, 1 written`; only `?? man/vulnmatch.1` left. Step 104 MAN GATE: `OK: man/ matches N binaries`. Step 105: `| cmd/vulnmatch | TOOL |` and `[PASS] live-path inventory matches docs/LIVE-PATHS.md`. Step 106: `OK 2 replacements` (wiki/SBOM.md, wiki/Security.md). Step 107 NOTHING OUTBOUND: 0 network imports; `kev.Parse` only; `Native CVE matcher` count 1. Step 108 commit signed as `1fd606f4`; tree clean. Scanner lane is now complete through Phase 13.

- Phase 6 (lane `images`, Steps 63-68): exit gate passed on `sprint/s-supply-first-images` at `852a6c1a` (signed): 29 of 29 images scanned, 0 failed, tree clean. Step 67 is [STUCK] on its hadolint half only (below). Five images moved at patch level: coredns 1.12.4, loki 3.0.1 and 2.9.17, promtail 2.9.17, node-exporter v1.8.2, prometheus v3.15.1. Record: `docs/security/image-scan-2026-11-04.md`. `check-prometheus-rules.sh` green on v3.15.1 (12 rule files, 12 test files); Loki and promtail configs validated offline on the new tags; coredns 1.12.4 started on the repo Corefile with `--network none`. Nothing live touched. `check-image-tags.sh`: exactly the three predicted stale rows.
- Phase 6 went past Step 63's list in three read-only ways: scanned the candidate tags (Step 65 needs "removes a finding" proved), scanned the open-webui digest the tree pins, and fetched the public CISA KEV catalogue to `/var/tmp/s-supply-first/p6/kev.json` for the Step 65 override. `scripts/check-prometheus-rules.sh` was edited (its `CURRENT_IMAGE` digest) because Step 66 says every occurrence; it is outside the swarm table's "Dockerfiles' FROM, compose tags", and the gate's coverage did not shrink, though it now tests the same version twice until the next bump. `docs/monitoring/TELEMETRY_STACK.md` got the two version strings.
- Phase 14 note: `deploy/sophia-eye/docker-compose.yml` is also rewritten by S-CLEAN-CUT (`a29524e8`), which keeps the prometheus line. Git may conflict on that one line; the answer is `prom/prometheus:v3.15.1`.

## Stuck

- **[STUCK] Step 67, hadolint half.** `check-hadolint.sh` exits 1, and it does on the untouched tree at `f1cf9670` too: `DL3008 in docker/firewall/{ipfire,opnsense}/Dockerfile went down to 0 (baseline 1)`. Both Dockerfiles are gone, so the rows are stale, the same cause as the two `ubuntu:24.04` rows the plan predicted for `check-image-tags.sh`; the plan just did not predict it here. No image edit caused it (Phase 6 changed no Dockerfile), so "revert that image's edit" has nothing to revert. Not fixed here: `docs/policy/hadolint-baseline.tsv` is not this phase's file, rule 17 forbids an unrelated baseline edit, and S-CLEAN-CUT Step 118 owns exactly those two rows, committed in `f3cd77ac`. Proof: a throwaway clone of `f1cf9670` with only those two rows removed gives `[PASS] hadolint: 35 Dockerfiles parsed`. Expect it to clear when Phase 14 merges develop with S-CLEAN-CUT in; re-run it there.

## For Stevie

- **URGENT, KEV.** The open-webui digest pinned in `deploy/sophia-eye/docker-compose.yml` (`sha256:4860ba00...`, built 2026-10-16) carries CRITICAL CVE-2026-111497 (Google Brotli, in `libbrotli1`, `brotli` and `brotlicffi`), on CISA KEV since 2026-10-27, due date 2026-11-10. Unauthenticated remote, crafted Brotli stream. The compose comment says this is the build west holds. No open-webui container is running on west now; east and the Tomb were not checked. Step 65's rule says record, not bump: today's `main` (2026-11-03) is free of it and scans 4/28 against 14/137, but it is one day old and a branch build. Your call: take a newer digest early, or keep it down until a digest passes 14 days.
- Step 63 scans open-webui by tag, which measures `main`, not what the tree pins; that is how the plan's list would have missed the item above. Any future scan list should use the digest.
- Five images are in neither Step 63's list nor its exclusions, so they are unmeasured: `grafana/grafana:10.4.0` (the `monitoring/` stack's Grafana), `grafana/loki:2.9.0`, `prom/prometheus:v2.51.0`, `victoriametrics/victoria-metrics:v1.99.0`, `lscr.io/linuxserver/wireguard:1.0.20250521`. Left unscanned rather than widen the list. Say the word and they take a few minutes.
- Images waiting on you, critical/high: postgres 16.15 1/25 (ADR-102 drill first), grafana 11.5.1 9/140 and 10.3.3 9/139, clickhouse 0/5, victoria-metrics 3/59, qdrant 2/11, nginx 3/17, haproxy 2/23, traefik 4/76, vector 7/38, prometheus v2.50.1 8/114, text-embeddings-inference 8/49. All are at the last tag of their series or named by the plan as ones to leave. alertmanager v0.34.2 becomes eligible on 2026-11-12 (0/12 today).
- Grafana patch tags do not help: 11.5.10 scans 9/150 and 10.3.12 scans 8/141, so a minor move is the only real gain.
- Three bumps raise the raw finding count and were taken anyway: coredns 2/58 to 2/66, loki 3.0.1 3/62 to 4/83, loki 2.9.17 3/63 to 4/74. Each removes real findings and satisfies Step 65. The rises look like scanner visibility, not regressions: old Loki sits on end-of-life Alpine 3.18.5 with OpenSSL 3.1.4, and coredns 1.12.0 reports no module version so its own CVEs never matched. That reading is an inference, not a test. If you would rather have a net-count rule, revert those two images. The new Loki bases do show 2 CRITICAL (CVE-2026-31789, `libcrypto3`/`libssl3`).
- coredns needs a minor move too: nine CVEs against it are fixed only in 1.14.x.
- Six images run on Alpine releases past end of life (nginx, haproxy, traefik, victoria-metrics, grafana, loki 3.0.x), so their counts probably understate.

## Phase 14 (Steps 109-123, Opus)

- Step 109: every lane committed work that passed its own [V] steps. Open items going in: Phase 6 Step 67's hadolint half only. No `unsigned.txt`: no unsigned commit in this plan.
- Step 111: 8 lane merges, signed, no conflict. The likely sites (`wiki/Security.md`, Dockerfile labels, `monitoring/docker-compose.yml`) auto-merged.
- Step 112: develop at `59a712e2` conflicted in `docs/LIVE-PATHS.md` only. Rule 15's generated-file exception applied: regenerated with `scripts/live-path-inventory.sh --report`, not hand-merged, `[PASS]` after. So this one merge ran by hand instead of through `safe-merge.sh`, which aborts on any conflict. `deploy/sophia-eye/docker-compose.yml` merged clean and keeps `prom/prometheus:v3.15.1`, so Phase 6's predicted conflict did not happen.
- Develop moved once during the phase: `42d11e2f` (ADR-123, docs only, one file), merged clean as `99636d7b`. Steps 114-117 below ran after that merge.
- Step 113: nothing printed. All seven residue paths are gone, no FND collision.
- Step 114: CBOM regenerated with no change; `check-crypto-inventory` and `check-pq-kex` pass.
- Phase 6 Step 67 hadolint half: CLEARED. `[PASS] hadolint: 34 Dockerfiles parsed, no (rule, file) above its baseline (63 pairs)`. `check-image-tags` passes too.
- Step 115, read this one: the FIRST full `go test -race` run did not meet the bar. `TestFileStore_WatchDelete` in `pkg/secrets/store` failed on a data race: `runtime.closechan` from `FileStore.Close` (`file.go:356`) against `runtime.chansend` from `notifyWatchers` (`file.go:266`). It is not this plan's: neither this branch nor develop changes `pkg/secrets`, and it reproduces without this plan's bumps (see For Stevie). The second full run, on the final tree, printed `SAME`; gofmt and vet clean, gosec ratchet rc=0. `SAME` here means the race did not fire on that run, not that it is gone.
- Step 116, the final vulnerable list, on the final tree:
  - Go: `No vulnerabilities found.`
  - Rust: `TOTAL locks=21 failing=0` (was 24; S-CLEAN-CUT deleted the `cmd/trace-collector`, `cmd/waf` and `ebpf/af-xdp` locks).
  - Python: `[PASS] 3 pinned requirements files; 2 known vulnerabilities, all accepted with a reason`, the two torch ids PYSEC-2025-194 and PYSEC-2026-139 (FND-013). ChromaDB's four ids stay outside this gate under FND-032, For Stevie 1.
  - Tool freshness: STALE count 0.
  - vulnmatch fixture: `672 components, 4 records, 0 findings (0 in KEV), 16 not an exact pin`.
- Step 117: `sbom/go-dependencies.json` changed on two rows only (`golang.org/x/crypto`, `golang.org/x/net` v0.62.0), because S-CLEAN-CUT regenerated it today in `f3cd77ac`. `rust-dependencies.json` unchanged. Pip freeze kept.
- Image record for Phase 6: `docs/security/image-scan-2026-11-04.md`.

## For Stevie (Phase 14, continues the list above)

- Real race, not from this plan, left unfixed: `pkg/secrets/store/file.go`. `notifyWatchers` takes the channel slice under `RLock`, then sends after releasing it, while `Close` and `removeWatcher` close those channels. Besides failing `-race`, this can panic with send on closed channel in a running service. `TestFileStore_WatchDelete` failed 2 of 30 on the merged tree, 1 of 60 at `08fa9b8a` (pre-plan Go code), 1 of 60 on develop at `42d11e2f`. Any `-race` CI job can flake on it. Not fixed here because `pkg/secrets` is outside this plan's files and Step 115's fix clause covers only failures this plan caused. Fixing it means deciding who owns closing a watcher channel, so it wants its own change with a test.
- `scripts/generate-sbom.sh` lists `sbom/SBOM.md` as an output but never writes it, so that file still reads 2026-03-15 with 99 Go modules while the JSON holds 149. Step 117 expected a diff there; there is none. Left as is.
- `docs/policy/trivy-misconfig-baseline.tsv` carries 13 stale rows (`d955f5a2` deleted the manifests), so `check-trivy-misconfig` fails on develop today, and the security.yml job that reads it should too. `--update` is the fix, in whichever plan owns the rows; not done here under rule 17. Counts: 312 lines in the file, 13 to drop.
- Superseded by develop, no action: the URGENT open-webui KEV row above (`f11da98d`, on your word, pin now `sha256:975e3965...`) and the wireguard entry in the unmeasured-images row (`3295d8da`). The other four images in that row are still unmeasured.

## Phase 14, Step 120 and after

- Step 120: `PASS: 51 gate(s) proven to fail when violated, 24 skipped.`, rc=0, 0 INCONCLUSIVE, tree clean. `check-image-tags` now bites, as predicted once S-CLEAN-CUT Step 118 landed.
- Limit of that pass: `--quick` says nothing about its 24 slow gates. Run with `--only` because this plan touched them: `check-hadolint` PASS, `check-prometheus-rules` PASS, `check-clippy` PASS, `check-trivy-misconfig` INCONCLUSIVE (next line). Not run, this plan touched nothing they read: `bpf-verifier-check`, `check-ebpf-insns`, `check-ebpf-loads`, `check-haproxy-cfg`, `test-vector-config`, `lich-cargo-fuzz`, `monitoring/test-alert-path` (the plan changed no Alertmanager config) and the Well suites.
- `check-trivy-misconfig` is INCONCLUSIVE, so it is NOT proven to bite on this tree. Not from this plan: 13 stale rows in `docs/policy/trivy-misconfig-baseline.tsv`, for `kubernetes/manifests/base/haproxy-ingress/{controller,ingressclass,networkpolicy,rbac}.yaml` and `kubernetes/manifests/base/loadbalancers/ingress.yaml`, all deleted by `d955f5a2` (2026-11-03). Develop at `42d11e2f` fails with the identical 13 lines. Against a scratch copy of the baseline without those rows, this tree gives `[PASS] trivy misconfig: 345 failed checks in 297 (rule, file) pairs, all at baseline (274 config files scanned)`, so this plan added no finding. The repo's baseline is untouched (rule 17).
