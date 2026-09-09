<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-090 — The Total Source Sweep: file-by-file slop removal

| | |
|---|---|
| **Status** | **Proposed** (activates after the Staging Ladder sprint completes) |
| **Date** | 2026-08-03 |
| **Origin** | Stevie, 2026-08-03: *"systematically iterate through every source code file 1 by 1 (I MEAN EVERY FILE) to look for ways we can optimize and cut down lines of code. i feel we have some true slop I would like to eventually remove."* |
| **Related** | ADR-089 (promotion workflow — the sweep ships through it), ADR-073 (lint policy), ADR-048 (ForgeBackend trait — an example of the duplication this sweep hunts) |

---

## Context

The tree has grown fast and in bursts. Multi-agent swarms landed 81 files in a sprint.
Waves 10-17 each added a subsystem. Ages 0 through 3 each layered new abstractions over
old ones. Nothing has ever swept back through.

The result is what Stevie calls **slop**: code that is not wrong, not flagged by any
linter, and not doing anything useful. Linters cannot find it because it is syntactically
fine and often *locally* reasonable. It is only visible when a human reads the file and
asks "why does this exist?"

Three concrete pieces of evidence that slop is real here, not suspected:

- **`cmd/waf` and `crates/zhenai-forge` had 133 combined `dead_code` warnings** that were
  invisible for months purely because they were binary crates without a `lib.rs`. After
  the lib/bin split (`6e5f7589`, `25b40e62`) the real number was 3. The rest was noise —
  but nobody knew which was which until someone looked.
- **`shield-ebpf`'s PQC fast path has neither a producer nor a consumer.** An entire
  enforcement path that has never executed once.
- **The Mistral training path in `train.rs` is retained as stale functions**, deliberately,
  under a "side stuff" exemption. That is a documented decision — but undocumented
  versions of the same thing are certainly elsewhere.

The scale: **1,836 tracked source files.**

| Language | Files | | Language | Files |
|---|---|---|---|---|
| Go (production) | 685 | | Nix | 92 |
| Go (test) | 548 | | Python | 70 |
| Rust | 197 | | JS | 40 |
| Shell | 159 | | HTML/CSS/SQL/proto | 45 |

A sweep of 1,836 files is not a sprint. It is a long-running background campaign that
must survive dozens of sessions, context resets, and interruptions.

---

## Decision

Sweep **every** source file exactly once, in a fixed order, recording a verdict per file
in a durable ledger. The ledger — not any session's memory — is the state of the campaign.

### The ledger

`docs/sweeps/TOTAL-SOURCE-SWEEP.tsv`, one row per file, committed to the repo:

```
path <TAB> status <TAB> verdict <TAB> commit <TAB> notes
```

- `status` ∈ `PENDING` | `SWEPT` | `SKIPPED` | `DEFERRED`
- `verdict` ∈ `CLEAN` | `TRIMMED` | `DELETED` | `MERGED` | `FLAGGED`
- `commit` — the SHA that carried the change, or empty
- `notes` — one line, only when the verdict is not `CLEAN`

Generated once from `git ls-files`, then only ever updated in place. **A file is swept
exactly once.** Resuming a session means reading the first `PENDING` row. This is the
single most important design property: the campaign must be resumable by a session with
no memory of the previous one.

### Sweep order

Fixed, so progress is meaningful and the order is not re-litigated each session:

1. **Vendored/generated first — to exclude them.** `llama.cpp/`, `crates/xv6-mbc/upstream/`,
   `target/`, `node_modules/`, protobuf-generated `.pb.go`, anything with a
   "DO NOT EDIT" header. Marked `SKIPPED` with the reason. Editing generated code is a
   defect, not a cleanup.
2. **Shell (159)** — smallest surface, highest operational risk, most likely to hold
   copy-paste. Good calibration for the rubric.
3. **Python (70)** — already understood from the Staging Ladder work.
4. **Rust non-eBPF (~130)**.
5. **Go production (685)** — the bulk. By package, not alphabetically, so related files
   are read together and duplication between them is visible.
6. **Go test (548)** — swept *after* their production counterparts, because a test that
   covers deleted code is itself deletable.
7. **Rust eBPF (~67)** — last and most carefully. Verifier budget makes every change
   consequential and the 901,888-byte artifact check applies.
8. **Nix / HTML / CSS / SQL / proto (~140)**.

### The rubric — what counts as slop

Applied to each file, in order. Anything not on this list is **not** in scope.

1. **Dead code** — unreferenced functions, types, constants, whole files. Verified by
   search *plus* build, never by search alone. The `waf`/`forge` episode is the warning:
   a naive dead-code signal was 97% false.
2. **Commented-out code** — delete. Git remembers it; the comment does not.
3. **Vestigial compatibility shims** — wrappers kept for a caller that no longer exists,
   `_with_backend` style shims whose non-generic form has no remaining users.
4. **Copy-paste duplication** — the same 20 lines in four files. Extract *only* if the
   abstraction is honest; four similar-looking blocks that will diverge are better left
   alone than fused into a function with five boolean parameters.
5. **Over-abstraction** — the opposite failure. An interface with one implementation, a
   factory that constructs one type, a config struct with one field. Collapse it.
6. **Redundant error wrapping** — `fmt.Errorf("read: %w", err)` inside a function already
   named `read`, three layers deep. Keep the layer that adds context; drop the echoes.
7. **Stale TODOs and dead feature flags** — a flag with one value in every deployment is
   a constant. A TODO older than a year is a decision that was made by not doing it.
8. **Redundant tests** — tests asserting the same behaviour, tests of deleted code,
   `#[ignore]`d tests with no path back to running (`zhenai-forge` has 28).

### What is explicitly NOT slop — do not touch

This section exists because a LOC-reduction campaign, left unbounded, damages code.

- **Defensive input validation.** Nil checks, bounds checks, and explicit error handling
  are the house style and a security property. They are lines that exist on purpose.
  Removing them is the single worst outcome this ADR could produce.
- **Test coverage.** Deleting tests to reduce line count inverts the goal entirely.
  Tests are only removed when what they test is removed.
- **Clarity.** A five-line `if` chain that reads clearly beats a one-line nested ternary.
  **This is a slop-removal campaign, not a code-golf campaign.** Density is not the
  objective; absence of unnecessary code is.
- **Comments explaining *why*.** The commit messages and headers in this repo carry real
  reasoning (see `static-analysis.yml`). That is institutional memory, not verbosity.
  Comments restating *what* the next line does are fair game; comments explaining why a
  decision was made are load-bearing.
- **Documented deliberate retentions** — e.g. the Mistral path's "side stuff" exemption.
  Re-decide those with Stevie, do not unilaterally reverse them.

### Per-file procedure

```
1. Read the whole file. Not a grep — the whole file.
2. Apply the rubric. Record a verdict even when it is CLEAN.
3. If a change is warranted:
   a. Make it.
   b. Build the affected package/crate.
   c. Run the tests that cover it.
   d. If it is eBPF-adjacent: confirm monad-cpu-ebpf --features ascend-linux
      is still 901,888 bytes.
   e. Confirm every gating check is still green (clippy 0, gosec ratchet, etc).
4. Update the ledger row.
5. Commit per COHERENT UNIT — usually one package, never 200 files.
```

### Verification standard

**Behavioural equivalence is the bar, and the burden of proof is on the deletion.**

- Every change ships through ADR-089's `develop` → `staging` → `main` path.
- Deletions of anything reachable get a test demonstrating the behaviour survived — or
  an explicit note that no test exists and the deletion is reasoned, which is the same
  honesty standard the af-xdp wraparound fix was held to.
- **"The build passes" is not sufficient evidence** that removing a function was safe.
  Reflection, build tags, `cfg` features, FFI symbol exports, and BPF map references are
  all invisible to the compiler. Check for them explicitly before deleting anything with
  a `pub`/exported name.

### Cadence

Batched by package, not by session. A session sweeps whatever it can finish cleanly and
commits at package boundaries. There is no deadline — an unfinished sweep with an
accurate ledger is a healthy state, and the ledger makes "unfinished" cheap.

---

## On the metric

Stevie set the target: cut lines, remove slop. Worth one line of precision so the metric
does not get misapplied later — **lines *removed* is a reasonable target for a deletion
campaign; lines *written* remains a bad measure of delivered value.** The reporting for
this sweep is: files swept, files changed, lines removed, and — the one that actually
matters — **zero behavioural regressions**. The last number is the one that makes the
first three worth anything.

---

## Consequences

**Good**
- Slop that no linter can see gets found, because a human (or an agent reading carefully)
  looks at every file exactly once.
- The ledger doubles as the first complete inventory of what is actually in this tree.
- Dead subsystems like the unwired PQC fast path surface as a class rather than as
  one-off discoveries.
- Smaller tree → faster builds, cheaper review, less surface for the eventual public
  release.

**Costs**
- 1,836 files is a long campaign. It will span many sessions and must not be rushed.
- Genuine risk of over-deletion. The "NOT slop" list and the behavioural-equivalence bar
  exist to bound it, and the per-package commit granularity exists so any single bad
  deletion is revertible in isolation.
- Reading every file properly is slow. The campaign is deliberately not time-boxed,
  because a time-boxed version becomes a grep-and-delete pass, which is the failure mode.

**Risks and mitigations**

| Risk | Mitigation |
|---|---|
| Deleting reflection/FFI/BPF-referenced code | Explicit non-compiler check before any exported-symbol deletion |
| Deleting defensive checks to cut lines | Named in "NOT slop"; treat as a defect in review |
| Over-abstracting during "deduplication" | Only extract when the abstraction is honest; prefer leaving duplication |
| Campaign stalls half-done | Ledger makes partial completion a valid, resumable state |
| Agent bulk-edits without reading | Per-file verdict required — `CLEAN` rows are the proof a file was read |

---

## Open questions for Stevie

1. **Vendored code** — `llama.cpp/` and `crates/xv6-mbc/upstream/` are skipped by default.
   Confirm. (The xv6 upstream is *already* being progressively replaced by Unheaded-authored
   code per Phases 2.2-2.4, so its status may be special.)
2. **`docs/`** — 345K lines of documentation, far larger than any code surface. In scope
   for a separate sweep, or out of scope entirely? This ADR covers **source only**.
3. **The 28 `#[ignore]`d `zhenai-forge` tests** — restore (needs the right Gemma-4 GGUF
   re-acquired) or delete? Currently they are neither running nor removed, which is the
   worst of both.
