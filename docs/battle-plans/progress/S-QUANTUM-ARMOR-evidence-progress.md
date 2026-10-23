# S-QUANTUM-ARMOR: Evidence track progress (Phases 10-11)

Plan: `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`, Phases 10 and 11 only
(Steps 190-226). Branch based on develop `424edf47`. Decisions implemented as the
plan pre-seeds them for ADR-105 (decision 5 evidence seals, 6 validation, 7 pack
layout, Step 213 custody signatures); anything chosen here beyond the plan is
listed under "Choices ADR-105 must reflect".

Skip Protocol acknowledgment: I will skip any step after 3x its estimate or 2
failed debug attempts, mark it [STUCK] with what I tried, commit known-good state
first, continue with the next step that does not depend on it, and finish with a
STUCK/ESCALATE report ordered by how many steps each item unblocks. [ESCALATE]
steps are never attempted; they are reported.

## Preflight rows this track depends on

| # | Verdict | Evidence |
|---|---|---|
| H12 | TRUE | all 8 subcommands in `cmd/yggdrasil-evidence/main.go` print `TODO(task #65)` and exit 1 (read at 424edf47) |
| H13 | TRUE | `Verify` returns `ErrSealExpired` when `ExpiresAtUnix < now`; `SignFor` refuses ttl > 31 days (`pkg/gungnir/keys.go`) |
| H17 | PARTIAL | `jsonschema` 4.19.2 + `pyyaml` import locally; CI pins jsonschema 4.26.0 + pyyaml 6.0.3. Neither has `rfc3339-validator`, so jsonschema's `FormatChecker` does not check `date-time` at all |

## Baseline (before any change)

`go test -race -count=1 ./pkg/gungnir/... ./cmd/heimdall-daemon/... ./cmd/compliance-evidence/... ./cmd/yggdrasil-evidence/...`: 4 ok, 0 FAIL.

## Log

### Phase 10.1 (Steps 190-195)
- 190-191: `pkg/gungnir/evidence_test.go` written; red = build failure (no `SignEvidence`).
- 192-193: `pkg/gungnir/evidence.go`; green with `-race`; heimdall-daemon and compliance-evidence unchanged ok. `FuzzParseEvidenceSeal` 30 s: no crasher.
- 194: `git diff --stat HEAD -- pkg/gungnir/gungnir.go pkg/gungnir/keys.go` empty.
- 195: committed.

### Phase 10.2 (Steps 196-202)
- 196: 3 valid + 16 invalid fixtures (the plan's 14 plus `unquoted-date`, `yes-as-string` to pin YAML 1.1 scalar typing).
- 197-199: `manifest.go` = Go copy of the schema's rules (type/required/const/enum/min/max/pattern/format per JSON pointer) + a small engine over `yaml.Node`; typed decode with `KnownFields(true)` after it. `main.go` refactored to `run(args, stdout, stderr) int`. `TestSchemaDriftGuard` green and proven to fail on six schema edits. `FuzzValidateManifest` 45 s: no crasher.
- 200: differential: `AGREE 19/19 (1 documented Go-only rejection)`; a fake always-valid binary makes it `DISAGREE: 17 of 19`, rc=1.
- 201: `yggdrasil-verify.yml` runs `go test` + the differential (setup-go pinned as in ci.yml); YAML OK, PINNED_OK. Triggers now include `cmd/yggdrasil-evidence/**` and `pkg/gungnir/**` (otherwise a validator change would not run its own gate).
- 202: committed. 203 PHASE 10 EXIT GATE: `ok pkg/gungnir`, `ok cmd/yggdrasil-evidence` (+ differential AGREE 19/19).

### Phase 11.1 (Steps 204-208)
- 204-206: `pack.go` streaming tar.gz reader, in memory only; red then green. Refuses absolute, `..`, unclean (`//`, `./`), backslash/NUL names, symlink, hardlink, char device, fifo, entries outside `manifest/` (incl. `manifestX/`), duplicates, per-file/total/entry caps, non-gzip, gzip of non-tar, truncated, empty. Real caps (64 MiB file, 256 MiB total, 10 000 entries) asserted; a 1 GiB zero bomb (~2 MiB compressed, multi-member gzip) and a 64 MiB+1 file trip at the header. The real 256 MiB total cap is exercised with small limits (tripping it at full size means inflating 192+ MiB per run).
- 207: `FuzzReadPack` 120 s, 16.2M execs, no crasher (no `testdata/fuzz` corpus written).
- 208: committed.

### Phase 11.2 (Steps 209-212)
- 209: `fixture_test.go` (`buildFixturePack`: fake ISO, manifest from `valid/minimal.yaml` with the ISO's real sha/size, sha256sum line, trivy-shaped `cve-scan.json`, SPDX with N packages, CIS/doctor JSON, throwaway 0600 seed in `t.TempDir()`, signed through the CLI itself) + `sign_test.go`. Red = build failure.
- 210: `sign.go`: `sign` (`--purpose manifest|iso-sha256|custody-entry`, manifest must validate before it is signed, seal JSON 0644), `pubkey` (derive only), `verify-signature` (both seals, purpose-checked), `verify-iso` (seal on image.iso.sha256 first, then streamed ISO hash vs signed line vs manifest, plus iso_path name and iso_size_bytes). `pkg/gungnir` gained `SignEvidenceDetached`/`VerifyEvidenceDetached` (seal with issued_at 0, key id = KeyID(pub)) because `sign --purpose custody-entry` needs it; the custody chain code (`custody.go`) landed here for the same reason, its subcommand in 11.3.
- 211: green with `-race` (cmd/yggdrasil-evidence, pkg/gungnir). gosec clean. golangci-lint: 2 `unused` (checkCustody, farFuture) that 11.3 consumes.
- 212: committed.

### Phase 11.3 (Steps 213-218)
- 213 [DECIDE] custody signatures: RECOMMENDATION taken. Override condition ("a pack with signed custody already exists in another format") does not hold: H12, nothing implemented before. Implemented as: hex of a detached gungnir evidence signature (context `unheaded/gungnir/evidence/v1`, purpose `custody-entry`, implied issued_at 0, key id KeyID(pub)) over `lp("yggdrasil/custody/v1") lp(timestamp_utc) lp(actor) lp(action) lp(prev)`, prev = SHA-256 of the previous entry's message, 32 zero bytes for entry 0. Golden vector pinned and cross-checked with an independent Python computation.
- 214: `verify_test.go` red (7 tests failing on the stubs).
- 215: `gates.go` (verify-gates, verify-custody), `verify.go` (verify, diff), stubs removed from `main.go`.
- 216: green with `-race`; vet, gofmt, golangci-lint (0 issues), gosec clean. [D] 217 not needed.
- 218: committed.

### Phase 11.4 (Steps 219-224)
- 219: evidence-pack README (status implemented; `manifest/` layout; `build.pubkey` sole anchor; seal/custody/waiver semantics; subcommand + exit-code table; the Go-stricter-than-schema list), build runbook (`gungnir-sign` -> `yggdrasil-evidence sign --purpose ...`, custody-entry signing, key custody = E4, self-verify before emitting), verify runbook (`root.pubkey` -> `build.pubkey`, exit codes incl. `diff` 3, custody/attachment rows). `main.go` doc comment: "Scaffold only" removed.
- 220: build script signs custody entry + manifest + image.iso.sha256 when `YGGDRASIL_BUILD_KEY` is set (binary from PATH or `YGGDRASIL_EVIDENCE`), else keeps `.todo` markers and prints an "UNSIGNED PACK: not evidence" banner; `GPG_KEY_ID` dropped. Also fixed a pre-existing silent exit: `ls a/*.qcow2 a/*.iso` exits 2 when either glob is empty, which under `set -e -o pipefail` killed the script with no output when only an ISO existed (now `find`). Ran it: unsigned run rc=0 with banner, and `verify` rejects that pack (4/4 stages FAIL); with a throwaway key the scaffold manifest's `TBD` fields make `sign` refuse loudly (rc=1), as intended. The signing block itself was run verbatim against a valid manifest in a scratch harness: GNU `tar -czf ... manifest/` pack, verify-signature OK, verify-custody OK.
- 221: `scripts/check-evidence-pack.sh` (build, fixtures vs built binary, E2E test must actually run and pass); `provoke_evidence_pack` + REGISTRY row appended in `check-gates-can-fail.sh`.
- 222: gate green locally (rc=0).
- 223: committed.

### Step 225 [ESCALATE] E4: production evidence build key (NOT attempted)
Context: everything signs and verifies with throwaway seeds generated in `t.TempDir()` or a scratch dir (0600, never printed, never committed, deleted after use). A real pack needs a long-lived ML-DSA-65 seed, `build.pubkey` published where operators fetch it over a trusted channel, a rotation ceremony (ADR-043 #4: HSM/TPM/sealed or strict ACL + audit log, quarterly), and confirmation that it is a different key from the Heimdall baseline key (key separation). Why no recommendation: where key material lives and who holds it is Stevie's custody decision. The chain stops here; nothing else in Phases 10-11 depends on it.

## Choices ADR-105 must reflect (beyond the plan's pre-seeded text)

1. Evidence seal (decision 5): signed message = length-prefixed `format, purpose, algorithm, key_id, issued_at (int64 BE), payload_sha256`; `ParseEvidenceSeal` is strict (16 KiB cap, unknown/duplicate keys and trailing data refused); purposes are `^[a-z0-9][a-z0-9-]{0,63}$` in gungnir, the CLI allows only `manifest`, `iso-sha256`, `custody-entry`; signing refuses a signer whose key id is not `KeyID(pub)` (the `dev-spike` placeholder).
2. Detached evidence signatures (new gungnir API, needed for Step 213): `SignEvidenceDetached`/`VerifyEvidenceDetached` = an evidence seal with `issued_at_unix = 0` and `key_id = KeyID(pub)` whose signature bytes alone are stored.
3. Custody chain (Step 213): `lp("yggdrasil/custody/v1") lp(timestamp_utc) lp(actor) lp(action) lp(prev)`, prev = SHA-256 of the previous entry's message, genesis 32 zero bytes; the `signature` field is outside the chain. Golden vector in `TestCustodyMessageGolden`.
4. Validation (decision 6): beyond `additionalProperties`, Go is stricter on YAML anchors/aliases/tags/duplicate keys/multi-document, non-decimal integers and `.inf/.nan`, `$` before a trailing newline, and integer `license_summary` values. Scalars are typed with PyYAML's YAML 1.1 resolvers so "Go rejects everything jsonschema rejects" holds for scalar types too. The differential script supplies its own RFC 3339 `date-time` checker because jsonschema without `rfc3339-validator` (not installed locally or in CI) silently skips that format. `validate --schema PATH` now means "refuse to run if this schema differs from the compiled-in rules".
5. Waiver semantics: manifest `cve_scan.critical/high` are UNWAIVED counts (schema pins them to 0); a CRITICAL or HIGH finding in `cve-scan.json` is waived only by a `cve_scan.waivers` entry that is unexpired on the verification date (`expires_date` inclusive) and whose CVE ID appears in a `chain_of_custody` action. MEDIUM/LOW must match exactly.
6. verify-gates also cross-checks attachments the plan did not list explicitly: `cis-benchmark.json` `score_percent` (required) and `lynis_hardening_score` (if present), `upc-doctor.json` `exit_code`, and every overlay patch (listed == present, sha256 matches, no unlisted `overlay-patches/*`).
7. Exit codes: a pack that is malformed or unsafe to read is a verification failure (1), an unreadable path or missing pubkey is usage (2); `diff` returns 2 for anything it cannot read or validate and 3 for rewritten custody.
8. Pack reader: strict names (must already be clean; `./manifest/...` is refused), directories allowed, `TypeReg`/`TypeRegA` only for files.
9. `pubkey` is a new subcommand (plan C.7 lists it; the original CLI did not have it).

## Not done here (other lanes)
- `nix/yggdrasil/README.md` still lists #68 as "Scaffold complete": Phase 13 doc ripple (Librarian).
- UH-EVID-01 control row / crosswalk mapping: Phase 13 (MoatGhost).

## Step 224 meta-gate (`--quick`, clean clone of 915aeb58)
`check-evidence-pack  OK — bites when provoked (exit 1)`. Overall rc=1 with 4 INCONCLUSIVE ("already failing on a clean tree"):
- `check-well-lint`: MINE. `manifest.go`'s drift message `fmt.Sprintf("... missing from the Go rule table")` matched the Well lint's string-formatted-SQL heuristic (`FROM`). Reworded; gate PASS. Fixed in the next commit.
- `live-path-inventory`: pre-existing at 424edf47 (`cmd/zhen-claude` ORPHAN missing from `docs/LIVE-PATHS.md`; this sprint adds no main package and does not touch LIVE-PATHS).
- `check-ruff`, `check-python-audit`: environment (ruff / pip-audit not on PATH on west); they refuse to pass by skipping. Pre-existing, not touched.

## Step 226 PHASE 11 EXIT GATE
```
$ grep -c 'TODO(task #65)' cmd/yggdrasil-evidence/main.go; grep -rc 'gungnir-sign' nix/yggdrasil/evidence-pack | grep -v ':0' ; ./scripts/check-evidence-pack.sh >/dev/null && echo GATE11_OK
0
GATE11_OK
```
DoD: race green on gungnir + every importer (heimdall-daemon, compliance-evidence, unheaded-remediate, compliance/snapshot, yggdrasil-evidence); go vet, gofmt, golangci-lint (0 issues), gosec, govulncheck (no vulnerabilities) clean on touched packages; go.mod/go.sum unchanged (no new dependency); no listener; throwaway keys only, never printed or committed; E4 logged.

Re-run on 8a3a90a0 (clean clone): `check-evidence-pack` and `check-well-lint` both "OK — bites when provoked"; remaining INCONCLUSIVE = `check-python-audit`, `check-ruff` (tools absent on west), `live-path-inventory` (pre-existing `cmd/zhen-claude`). None introduced by this track.
