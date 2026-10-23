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
