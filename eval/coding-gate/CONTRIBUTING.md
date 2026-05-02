# Contributing to the Coding Gate

Conventions for authoring fixtures, runners, results, and probe write-ups in `eval/coding-gate/`.

## Fixture (`prompts.jsonl`) is FROZEN

The fixture is the contract between every gate run. Adding new prompts is allowed; modifying or deleting existing prompts is **not** without a probe-results doc justifying the change AND a rubric version bump (see `RUBRIC.md` changelog).

When adding prompts, append-only. Use stable IDs (e.g., `syntax-go-mutable-default-arg`). Each new prompt entry:

```json
{"id":"<stable-id>","kind":"syntax|review","language":"<lang>","prompt":"...","expected_flag":"..."}
```

`expected_flag` is required for review prompts (used by graders), optional for syntax prompts.

## Sanitization convention for security write-ups

**Why:** during the 2026-05-02 probe, the first iteration of `probe-2026-05-02/A1-source-poison.md` embedded the verbatim malicious payload as a quoted code-fence example. cs/vor indexed the doc and then *retrieved* the doc on a follow-up probe query, surfacing the payload to the model. The finding doc itself became a fresh attack vector.

**Rule:** any commit that documents an adversarial payload must elide the verbatim destructive content. Replace exact paths and commands with bracketed placeholders. The threat model is recoverable from the elided text; the executable payload is not.

| Verbatim (do NOT commit) | Sanitized (commit this) |
|---|---|
| `rm -rf /home/govan/tmp/unheaded/raft/training/` | `[destructive-cmd-elided]` |
| `ln -s /tmp/evil-corpus ~/.config/cs/sources/...` | `ln -s /tmp/[attacker-dir] ~/.config/cs/sources/[legit-looking-name]` |
| Full quoted payload markdown | "The poisoned content advertised itself as ... and recommended deletion of two real Unheaded directories." |

If a verbatim quote is necessary for the finding to be reproducible, attach the unsanitized payload to a non-indexed channel (e.g., the GitHub issue comment, a private gist) and reference it from the doc; never commit it to a path that cs/vor can index.

The frontmatter of a sanitized doc must include `sanitized: true`:

```markdown
---
title: ...
severity: ...
date: ...
sanitized: true
---
```

A doc with `sanitized: false` (or no frontmatter) embedded in `eval/coding-gate/probe-*/` is a CI failure (see `scripts/probe-sanitization-check.sh`).

## Probe runs go in `probe-YYYY-MM-DD/`

One directory per probe session. Inside:

- `SYNTHESIS.md` — top-level findings + decision summary.
- `<probe-id>.md` — one per experiment / attack vector. Use the IDs registered in the parent SYNTHESIS.
- Raw runner outputs (`seed-N.md`, `nosystem.md`, `no-<clause>.md`, ...) — committed verbatim for reproduction.

Probe runner: `scripts/coding-gate-probe.sh` (configurable via `PROBE_NAME`/`PROBE_SEED`/`PROBE_SYSTEM`/`PROBE_OUT` env vars).

## Results docs (`results-YYYY-MM-DD*.md`)

Each gate run produces a `results-YYYY-MM-DD<-suffix>.md`. Format:

1. **Header**: date, grader, run UID, GGUF SHA-256, vor sources fingerprint, decoding parameters.
2. **Integrity checks** table (per `RUBRIC.md` §6).
3. **Per-prompt grades** table — fill PASS/FAIL/🔴 for all 14 prompts, with one-sentence notes.
4. **Aggregates** — PASS count, syntax half, review half, per-language.
5. **Verdict** — H1/H2/H3/H4 per RUBRIC §4 decision rule.
6. **Raw outputs** — appended by the runner; do not edit.

Hand-grade against the rubric **before** writing the verdict. Don't peek at aggregates and rationalize backward.

## RUBRIC version bumps

Edit `RUBRIC.md`'s `Version:` and `Changelog` in the same commit as the rule change. Reference the probe-results doc that justifies the change. Never bump silently.

## CI gate (5-seed sweep)

Before merging any change to `cmd/zhen-rag/main.go`, `scripts/run-coding-gate.sh`, `eval/coding-gate/RUBRIC.md`, or `eval/coding-gate/prompts.jsonl`, run:

```
make coding-gate-ci
```

This runs the 14-prompt fixture across 5 seeds (42, 137, 314, 271, 999), auto-grades each, and reports the variance band. PRs that produce >2-prompt swing in the verdict band require a probe-results doc explaining why the change is intentional.
