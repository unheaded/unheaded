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
