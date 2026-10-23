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
