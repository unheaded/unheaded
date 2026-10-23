# S-QUANTUM-ARMOR progress log

Plan: `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`. This file is the session log
the plan refers to, and where `[STUCK]` / `[ESCALATE]` markers live.

## Track: Phases 0-4 (Coordinator + Developer), worktree off develop `424edf47`

Skip Protocol acknowledgment: "I will skip any step after 3x its estimate or 2 failed
debug attempts, mark it [STUCK] with what I tried, commit known-good state first, continue
with the next step that does not depend on it, and finish with a STUCK/ESCALATE report
ordered by how many steps each item unblocks." [ESCALATE] steps are never attempted; they
are reported.

### Plan amendments (Marshal Pattern 6)

- Step 1: `$SCRATCH` is a private session scratch directory, not `mktemp` under `/tmp`
  (a parallel agent shares the session scratchpad; each track uses its own subdirectory).
- Step 2: `443de3f82` is not an ancestor of develop: it is the pre-re-sign copy of
  `0a605108` (same subject and content), which is. Base used: `424edf47`.
- Step 12: the local `postgres:18-alpine` image (PG 18.3) was probed instead of pulling
  `18.0-alpine`; Phase 8 picks the exact tag.
- Steps 27-31: in a linked worktree, go.mod's `replace ... => ../projects/doomgeneric/unheaded`
  does not resolve (only `cmd/wotan-ctl` imports it). Baselines ran with a scratch
  `GOWORK` that points that replace at the real sibling checkout; clean clones get a
  scratch `projects` symlink next to them. No tracked file changed for this.
- Commits: `--no-gpg-sign` always (lead re-signs at merge).

### Phase 0 (Steps 1-25): done

19/19 hypotheses TRUE. Table: `docs/security/pq-preflight-2026-10-23.md`. No container,
key or password left behind; hosts read only.

### Phase 1 (Steps 26-33): done, baseline captured

- Tools: go, cargo, docker (sudo), openssl, ssh, python3, jq, git all present.
- Go `go test -count=1 ./...`: 275 ok / 0 FAIL.
- Race (`pkg/tls`, `pkg/transport`, `pkg/mesh`, `pkg/loadbalancer`, `pkg/gungnir`,
  `pkg/wotan-client`, `services/wotan`, `cmd/heimdall-daemon`, `cmd/yggdrasil-evidence`,
  `pkg/compliance`): 46 ok / 0 FAIL.
- Rust (the plan's `tail -1` keeps the doc-test line): waf, zhend, trace-collector,
  upc-bootctl all build and pass; waf lib 52 + 1, upc-bootctl 25.
- Gates: check-image-tags, check-dependency-baseline, check-manifest-yaml,
  check-secrets-baseline all rc=0.
- Meta-gate `--quick` in a clean clone: rc=1, three gates INCONCLUSIVE (already failing
  on a clean tree), all pre-existing, none ours:
  - `live-path-inventory`: `cmd/zhen-claude` (ADR-104) was added without regenerating
    `docs/LIVE-PATHS.md`. Step 79's regeneration fixes it.
  - `check-ruff`, `check-python-audit`: ruff and pip-audit are not installed on west.
    With the CI-pinned tools in a scratch venv (`--require-hashes` from
    `.github/ci-python/requirements.txt`) they run; later meta-gate runs use that venv.

### Phase 2 (Steps 34-53): done

ADR-105 (Proposed) with eleven decisions, all the plan's pre-seeded recommendations; no
"Override ONLY if" condition was met (decision 8 checked: no publisher uses a `drift.`
topic prefix; `pkg/state`'s `drift.detected` is an event type on `cuirass.reconciliation`).
ADR-INDEX row added (totals 102, Proposed 17). Plan note: Step 34 cites "ADR-099 §2" for
sealed snapshots; in ADR-099 §2 is "Profiles switch controls", sealed snapshots are in its
2026-10-01 amendment.

### Phase 3 (Steps 54-82): done

- `cmd/crypto-inventory` (scan, check, summary, collect-host) + fixtures; CBOM
  `compliance/crypto/cbom.cdx.json`, 101 components: classical 16, classical-signature 13,
  hybrid 7, hybrid-opportunistic 25, n/a 15, pq-psk-static 7, pq-signature 12, unknown 6.
- Host facts (read-only) `compliance/crypto/hosts/{west,east}.json`: sshd mlkem first with
  classical fallback; `wg0` one peer, PSK set (file dated 2026-03-25). No address, no key.
- Gate `scripts/check-crypto-inventory.sh`: rc=0 fresh, rc=1 with one component deleted;
  `--schema` validates against CycloneDX 1.6 (tag 1.6.1, sha256-pinned, Python jsonschema).
- Exit gate (clean clone of `63400e8b`): gate rc=0, `--list` 1, LIVE-PATHS 1; meta-gate
  `--quick` rc=0, `check-crypto-inventory  OK — bites when provoked (exit 1)`.
- [D] Step 81: the first meta-gate run found `check-compose-hardening` and `check-well-lint`
  failing on a clean tree because they glob the whole repo and saw the scanner's fixtures
  (`docker-compose.yml`, `postgresql.conf`). Fixed by renaming the fixtures (`63400e8b`).

Plan amendments in Phase 3:
- The CI step is its own `crypto-inventory` job in security.yml (Go + the hashed Python
  lock); the gitleaks job that runs check-image-tags has neither.
- The tree is `git ls-files --cached --others --exclude-standard` (not tracked-only), so
  the meta-gate's untracked probes and a not-yet-committed file are seen.
- The gate scripts build the command once instead of `go run` (go run maps exit 2 to 1).

### Phase 4 (Steps 83-95): done

- Lint rules + ratchet in `cmd/crypto-inventory/lint.go`; `scripts/check-pq-kex.sh`;
  baseline `docs/policy/pq-kex-baseline.txt` = 15 entries (17 lines with the header).
- Amendment: a baselined key the tree no longer has FAILS ("resolved: remove from
  baseline") instead of only printing, as check-image-tags does, so a fix lands with its
  baseline line removed and the violation cannot return unnoticed.
- Step 89: rc=0 on the tree; rc=1 with an untracked `pkg/tls/zz_pq_probe.go` pinning X25519.
- Exit gate: `check-pq-kex.sh` rc=0 (`[PASS] 15 classical key-exchange entries, all in the
  baseline`); meta-gate `--quick` in a clean clone of `12bfce16`: rc=0, both
  `check-crypto-inventory` and `check-pq-kex` "OK — bites when provoked (exit 1)".
- Step 83 note: the lint tests were written before lint.go but first run after it existed
  (no separate red run).

### STUCK / ESCALATE (Phases 0-4)

None stuck. No [ESCALATE] step lies in Phases 0-4; E1-E5 belong to Phases 8-12.

### Notes for Phases 5-9 and 12

- Phase 5: `tls.MLKEM1024` needs go1.27 under `go vet` (go.mod says `go 1.26.0`), so
  `pkg/tls/pq_test.go` cannot name it in a normal file; use a `//go:build go1.27` file
  (as the inventory fixture does) or `tls.CurveID(514)`.
- Phase 5: the baseline also lists `services/wotan/cmd/wotan/main.go tls-floor-below-1.3`
  (gRPC `NewServerTLSFromFile`, Step 106), `pkg/http/server.go tls-floor-below-1.3`
  (empty `tls.Config{}`, Step 113) and `pkg/loadbalancer/config.go tls-max-below-1.3`
  (the optional `max_version: "1.2"`). The scanner does not know `ApplyStrict` /
  `ApplyPreferred` yet: a literal that sets MinVersion 1.3 clears the floor rule, but the
  CBOM will call it `hybrid`, not `hybrid-strict`, until the scanner learns those calls.
  `pkg/mesh/mtls` literals take MinVersion from a variable and show `unknown`.
- Phase 7: lint keys are `cmd/waf/Cargo.toml` (ring-only features) and
  `crates/zhend/src/api/quic.rs` (ring provider calls). zhend's Cargo.toml already compiles
  aws-lc-rs through default features; dropping `ring` leaves it hybrid.
- Phase 8: both `docker-compose.yml` and `kubernetes/manifests/base/the-well/statefulset.yaml`
  carry `postgres-pre18` (both mount db/postgresql.conf with ssl on).
- Phase 9: the inventory also shows two WireGuard peers configured without a PSK
  (`runbooks/network/wireguard-overlay.yaml`, `scripts/firewall/setup-ipfire.sh`), not
  linted. Hosts use `wg0`; east keeps the PSK in `preshared.key` and `wg0.conf`, west inline.
- Phase 12 / everyone: `scripts/check-gates-can-fail.sh` REGISTRY gained two rows after
  `check-image-tags`; merge by keeping all rows.

## Track: Phases 13-14 (Coordinator: MoatGhost, Librarian, BlackMage, Marshal), branch `sprint1/wrap` off develop `6884d105`

Skip Protocol acknowledged as above. Go runs with a scratch `GOWORK` (doomgeneric
replace); commits `--no-gpg-sign`. Baseline at `6884d105` (scratch clone, before any
change): `go test -count=1 ./...` 278 ok / 0 FAIL.

### Phase 13 (Steps 249-260): done

- 249: every requirement ID exists (800-53 CM-8, SC-8, SC-8(1), SC-12, SC-13, SI-7, AU-10,
  SR-4; CSF ID.AM-02, PR.DS-02; 800-171 03.13.08; SSDF PS.2; SOC 2 CC6.7, CC8.1; ISO A.5.9,
  A.8.24).
- 250: `UH-PQC-01` (CBOM current), `UH-PQC-02` (first-party KEX hybrid), `UH-PQC-03` (host
  sshd hybrid-only; FAILS today, E2), `UH-EVID-01` (signed, verifiable build evidence).
  Amendment: UH-PQC-01 maps ISO A.5.9 as well as A.8.24, because
  `TestMappingsCorroborated` requires NIST's ISO reference for ID.AM-02 (A.5.9).
  `compliance/baseline/baseline.yaml` regenerated (+2 host probes, `sshd-kex-pq`).
- 251: `go test ./pkg/compliance/...` 11 ok; no test pins the control count.
- 252: FND-015 (Well P-256, E1), `host-probe:{west,east}:sshd-kex-pq` (E2), FND-016
  (static PSK, E3), FND-017 (plaintext internal links), plus, beyond the plan: FND-018
  (no production evidence key, E4), FND-019 (no drift keys, E5), FND-020 (replication
  drops signatures), FND-021 (ring byte budget ignores signatures), FND-022
  (rustls-pemfile unmaintained), FND-023 (the wotan-client flake, resolved by `c2ca5e74`).
  ADR-098's generated section NOT regenerated (develop unsigned; regenerate after re-sign).
- Flake (parked item, fixed): `TestGRPCClient_GetMessages` / `_AfterSeq` failed 8 in 40
  runs at base. Root cause: at the 200 ms query timeout the server's own deadline ends the
  stream and grpc can deliver that (`DeadlineExceeded` via RST_STREAM CANCEL, or
  `Canceled`) to `Recv` before `queryCtx`'s timer has set `Err()`, so `queryEnded` said
  "Wotan failed". Fix: a `DeadlineExceeded`/`Canceled` at or after our deadline is the
  normal end. `TestQueryEnded_ServerReportsOurDeadlineFirst` red then green; after: 0
  failures in 200 `-race` runs.
- `cmd/crypto-inventory` (lead item from the KEX-Go track): gosec 10 + staticcheck 1 -> 0.
  G204/G702: git now runs with constant arguments and `cmd.Dir`. G703: every file the tool
  reads or writes goes through an `os.Root` at the repository top (paths are
  repository-relative; a path or symlink out of the repo is refused; red test first).
  G101/G402 false positives: leading `#nosec`. QF1001: De Morgan.
- 255-256: SECURITY.md Cryptography rewritten; ADR-105 gained "Decisions made during
  implementation" (evidence seals, custody, waivers, validator strictness, signed sender,
  prefix rules, Heimdall identity, ErrInvalidSignature, aws-lc-rs, Well dump/restore, LB
  opt-ins, PSK wait 200 s; decision 9 amended 60 s -> 200 s), status still Proposed;
  wiki `Post-Quantum-Key-Exchange` (Home + _Sidebar), The-Well, Overlay-Network (PSK ops
  facts), ADR-Index mirror (+104, +105); zhen-pq-threat-model rec 13 done;
  `nix/yggdrasil/README.md` #68 implemented; CLAUDE.md What's Done line. No CHANGELOG
  entry: CHANGELOG.md is not kept for develop. LIVE-PATHS: no `cmd/` main package added.
- 257: consistency grep empty. 258: `sync-wiki.sh --dry-run` only (nothing written).
- 260 EXIT GATE: controls 4, findings 5, `pkg/compliance` 0 FAIL. PASS.

### Phase 14 (Steps 261-277)

- 261: `go build ./...` ok; `go test -count=1 ./...` 278 ok / 0 FAIL, the same package set
  as the base. ZERO NEW FAILURES. (Run at `4cd32c28`; the two later Go/script fixes were
  re-tested on their own: `go test -race ./pkg/logagg/...` ok, clippy gate PASS.)
- 262: `-race` on every touched package: 55 ok / 0 FAIL.
- 263: Rust (`cargo test --quiet`, summed): waf 58 passed, zhend 163, trace-collector 163
  (1 ignored), upc-bootctl 25; 0 failed anywhere; identical to the Phase 7 post-change counts.
  Clippy: `scripts/check-clippy.sh` FAILED on develop (since Phase 3): it treats every
  tracked Cargo.lock as a workspace, and `cmd/crypto-inventory/testdata/rust/Cargo.lock` is
  a fixture with no Cargo.toml. The meta-gate's `--quick` skips this slow gate, so no
  track saw it. Fixed (`cf6da70e`): lockfiles under `testdata/` are skipped. After:
  `PASS: clippy 0/0 warnings, 0 errors` over 19 workspaces.
  `cargo audit` (all 20 lockfiles, as CI): 0 vulnerabilities; warnings unmaintained only
  (waf RUSTSEC-2025-0134 = FND-022; zhend instant/paste/fxhash/bincode/pqcrypto-*, as
  before the sprint).
- 264: gosec ratchet PASS; gitleaks PASS (5362 tracked files); secrets baseline PASS (20);
  govulncheck: 0 reachable (1 in a required module, golang.org/x/crypto GO-2026-5932, not
  called, no fix released). gosec `./...` as CI runs it found 3 G115 in
  `pkg/logagg/ringbuffer.go` (`d8d916f9`, another session's commit of 2026-10-23, so the
  develop gosec job was red); fixed without conversions (`4e8d9c46`), after: 0 issues.
- 265: meta-gate `--quick` in a clean clone of `4e8d9c46` (sibling `projects` symlink for the
  doomgeneric replace; ruff 0.17.1 and pip-audit 2.10.1 from a scratch venv installed with
  `--require-hashes` from `.github/ci-python/requirements.txt`; `GOWORK=off`): rc=0,
  `PASS: 33 gate(s) proven to fail when violated, 11 skipped` (slow tier), 6 not provable
  here (sudo / live XDP / images). The sprint's rows: check-crypto-inventory, check-pq-kex,
  check-evidence-pack, wireguard/test-rotate-psk, check-ruff, check-python-audit,
  live-path-inventory all "bites when provoked"; check-clippy and well/test-pq-groups are
  slow (skipped by `--quick`; clippy run directly above). A first run reported 5
  INCONCLUSIVE: the clone sat under a scratch dir holding a `go.work`, which Go picked up
  from the parent; not a repo problem.
- 266: PQ baseline holds 4 lines, each with its reason (2 LB opt-ins accepted, 2 Postgres
  held until E1). CBOM fresh and schema-valid. First-party classical TLS in the CBOM: the
  Well (3 components, FND-015) and the LB `max_version` opt-in (accepted). Also classical,
  not TLS: `pkg/secrets/encryption/age.go` (age X25519 recipients) and two WireGuard peers
  without a PSK in repo configs that are not deployed
  (`runbooks/network/wireguard-overlay.yaml`, `scripts/firewall/setup-ipfire.sh`): noted,
  not triaged here.
- 267: nothing regenerated (CBOM and LIVE-PATHS unchanged).
- 268-272 red team (`53df15c5`, details in `docs/security/pq-preflight-2026-10-23.md`
  "Post-sprint verification" and LICH-012):
  - Downgrade: Strict refuses X25519-only, P-256-only and TLS 1.2; Preferred falls back
    only for a client with no hybrid at all. With a classical group listed first, Go
    sends a HelloRetryRequest and still negotiates X25519MLKEM768 (written into ADR-105
    Consequences).
  - Evidence tamper: `../evil`, symlink and hardlink to /etc/passwd, duplicate manifest,
    absolute entry, a seal from another key, a same-key seal over another manifest: all
    rc=1; strace shows no access outside the tarball.
  - Key hygiene: clean (one bare `BEGIN PRIVATE KEY` header string in a WAF
    leak-detection test, pre-existing, no key).
  - New findings: none at medium or above; two low (FND-020, FND-021).
- 273: `~/tmp/next.md` NOT edited by this track: the lead keeps it (S17c already tracks
  Sprint 1 and the open decisions). Everything Step 273 asks for is in the closing
  summary below.
- 274-277: shift report below; never pushed; the plan file was not ticked in place.

### Marshal shift report (Phases 13-14)

- Steps: 249-272 and 274-277 done; 273 deviated (next.md left to the lead, content here);
  0 STUCK; 0 ESCALATE attempted (E1-E5 remain Stevie's).
- Citations: 1 (S2, plan deviation, logged): two pre-existing develop breaks fixed outside
  Phases 13-14 because the final regression could not pass without them (clippy gate
  fixture, logagg G115). Parked, not done: the age X25519 and PSK-less WireGuard repo
  configs; ADR-098 regeneration.
- Plan compliance: 28 of 29 steps as written (97%).

## Sprint 1 closing summary

**Status: DONE pending Stevie's decisions.** All 15 phases (277 steps) executed across
seven tracks; nothing STUCK; every phase exit gate passed. develop is unsigned from this
sprint (re-sign before push); nothing pushed; no host changed.

What exists now:
- Every first-party Go and Rust TLS surface negotiates `X25519MLKEM768` and a handshake
  test asserts it; Strict links (Wotan gRPC, wotan-client, mtls, mesh) refuse classical.
- `compliance/crypto/cbom.cdx.json` (CycloneDX 1.6, 111 components) from
  `cmd/crypto-inventory`; gates `check-crypto-inventory` and `check-pq-kex` (baseline
  15 -> 4) in CI, both proven to bite by the meta-gate.
- rustls on aws-lc-rs (WAF, zhend QUIC); unused TLS features dropped; dependency baseline
  764 -> 747.
- The Well's PG 18 hybrid proven in throwaway containers (24/24) with an upgrade runbook.
- sshd strict probe; `rotate-psk.sh` + harness + (disabled) weekly timer.
- `yggdrasil-evidence`: all subcommands implemented, ML-DSA-65 evidence seals, signed
  custody, gate `check-evidence-pack`.
- Signed `drift.*`: `pkg/wotansig`, per-prefix keys in Wotan, signatures delivered to
  subscribers, Heimdall publishes signed drift.
- Compliance: UH-PQC-01..03, UH-EVID-01; FND-015..023 plus the two sshd-kex-pq sources.

Final regression (tip `sprint1/wrap`): Go 278 ok / 0 FAIL (= base); race 55 ok / 0 FAIL;
Rust 409 passed / 0 failed (waf 58, zhend 163, trace-collector 163 + 1 ignored,
upc-bootctl 25); clippy 0/0 (19 workspaces); cargo audit 0 vulnerabilities (20 lockfiles);
govulncheck 0 reachable; gosec 0 issues; gitleaks, secrets baseline, gosec ratchet PASS;
CBOM fresh + schema-valid; PQ-KEX PASS (4); meta-gate `--quick` in a clean clone rc=0 (33 proven, 11 slow skipped).

**Stevie's open decisions (one list):**
1. E1: upgrade the live Well PG 16 -> 18 with `ssl_groups` (FND-015;
   `runbooks/data/well-pg18-upgrade.yaml`).
2. E2: make sshd hybrid-only on west and east, east first (lockout risk; UH-PQC-03;
   `docs/security/pq-host-escalations.md`).
3. E3: first live WireGuard PSK rotation (`rotate-psk.sh --apply`) and enabling
   `unheaded-wg-psk-rotate.timer` (FND-016).
4. E4: the production evidence build key: where the seed lives, who holds it, how
   `build.pubkey` is published, the rotation ceremony (FND-018).
5. E5: a Heimdall drift key per node, its public key in Wotan's `drift.` keys, a Heimdall
   unit (FND-019).
6. Accept ADR-105 (Proposed).
7. Sign off the `aws-lc-rs` 1.18.2 / `aws-lc-sys` 0.46.0 admission (ADR-004).
8. Sign off the `anthropic-sdk-go` v1.82.0 admission (ADR-004, ADR-104).

Housekeeping for whoever merges: re-sign the unsigned commits
(`scripts/resign-unpushed.py`) before pushing; regenerate ADR-098's generated findings
section after the re-sign; merge `sprint1/wrap` into develop.

Parked (Appendix B.3 plus this track): TLS on for internal links (FND-017) · edge TLS ·
PQ certificates · Monad wire-level PQC (frozen) · Zhen hybrid-KEM combiner review ·
Heimdall BPF ringbuf / Gjallarhorn listener · root -> build key chain · Rosenpass · CNSA
2.0 · merging the TLS helper packages · FND-014 password rotation · east deb rebuild ·
`cmd/ebpf-collector` proto field 12 · replication signatures (FND-020) · ring byte budget
(FND-021) · rustls-pemfile (FND-022) · Heimdall metrics not scraped (no listener) ·
`TopicStreamClient.PublishSigned` · WAF does not log the negotiated group · age X25519 in
`pkg/secrets/encryption` · PSK-less WireGuard peers in two undeployed repo configs.
