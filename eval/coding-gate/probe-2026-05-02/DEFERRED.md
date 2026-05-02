# Deferred — pre-D-A blockers requiring multi-repo coordination

These three blockers from `SYNTHESIS.md` cannot land cleanly in a single unheaded commit because they require coordination with the upstream `cs/vor` repo (https://github.com/bellistech/vor or its post-rename successor) and/or design work that depends on cross-cutting source-provenance plumbing.

## D-pre.B1 — Source provenance plumbing (IMPLEMENTED 2026-05-02)

**Status:** retrieval-layer half landed in cs (`harden/api-dos-and-traversal` branch, commit `e7f57dd` on top of `fa46000`); zhen-rag pass-through landed in unheaded as part of the same B1 unheaded commit.

**What shipped (cs side, awaiting upstream PR):**

- `SourceKind` enum (`embedded` | `user-custom` | `user-source`) with `Trust()` method (`canonical` | `local` | `external`).
- `SourceSpec{FS, Kind, Path, Label}` constructor input.
- `registry.NewWithSources([]SourceSpec, []fs.FS)` — preferred entry point. `NewWithDetails([]fs.FS, []fs.FS)` retained for back-compat (everything tagged `SourceEmbedded`).
- `Sheet.SourceKind`, `Sheet.SourcePath`, `Sheet.SourceLabel` fields.
- `/api/topics/:name` JSON: `source_kind`, `source_trust`, `source_path` (omitempty), `source_label` (omitempty).
- `/api/search` results: same four fields per hit.
- `sources.Load()` now returns `[]Source{FS, Path, Label}` — labelled per discovered symlink.

**What shipped (unheaded side):**

- `vorTopic` / `vorSearchHit` structs gained the four new fields.
- References-block in zhen-rag now formats each topic with a trust prefix: `--- [canonical] category/name ---` or `--- [external] ./topic (source: <label>) ---`.
- System prompt has a new `SOURCE-TRUST LABELS` clause: any answer that relies on an `[external]` reference for a specific Unheaded claim must be prefixed with a verify-out-of-band note. Defense-in-depth alongside (not instead of) the destructive-verb filter.
- `-show-context` stderr output displays the trust label and source label for each retrieved topic.

**Backward compat:**

- Pre-B1 vor builds: `vorTopic.SourceTrust` will be empty; zhen-rag falls back to `[canonical]` so old retrieval still works without warnings.
- cs `cscore` (mobile) caller path unchanged (still uses `NewWithDetails`).

**Smoke verified:** poisoned `~/.config/cs/sources/<name>` symlink content correctly surfaces as `[external]` in zhen-rag's references-block and as `source_trust=external` in vor's JSON. Destructive-verb filter still fires when poisoned content recommends destructive shell verbs. Embedded `bash` cheatsheet correctly tagged `[canonical]`.

**Search ranking weights (LANDED 2026-05-02 on cs branch):**

The B1 design called out a per-kind weight (default 1.0/0.8/0.5) so embedded ranks above user-source for tied relevance. Implemented as a tie-breaker comparator in `cs/internal/registry/registry.go` (commit `a6043b0` on `harden/api-dos-and-traversal`):

- `SourceKind.RankWeight()` returns 100 / 80 / 50 for embedded / user-custom / user-source.
- The Search comparator inserts the weight just before the existing lex-name tie-break, so high-relevance user-source still beats low-relevance embedded, but tied-relevance always favors embedded.
- Three new tests cover the hierarchy, the prefer-embedded-on-tie behavior, and the doesn't-dominate-when-other-signal-wins case.

This closes the last piece of B1's design list. The cs harden branch now has 3 commits to PR upstream:

```
a6043b0 source-kind tie-breaker (this)
e7f57dd source provenance schema
fa46000 vor DoS + path-traversal harden
```

## D-pre.B2 — Champion tool-call gating on source provenance (IMPLEMENTED 2026-05-02)

**Status:** the gate function and decision matrix tests landed in `pkg/champion/toolcall.go` + `pkg/champion/toolcall_test.go`. The agent-layer caller (Phase D-A's ReAct loop) is the only remaining piece — and it's the consumer of this gate, not part of B2 itself.

**What shipped:**

- `ToolCall{Name, Args, Justification, EmittedBy}` envelope — what the agent layer hands to Champion before any underlying dispatch.
- `Reference{Topic, Category, SourceKind, SourceTrust, SourcePath, SourceLabel, Excerpt}` — mirrors B1 cs/vor schema.
- `MutatingTools` / `ReadOnlyTools` whitelists, with **fail-closed default** for unknown tool names (treated as mutating).
- `IsMutating()`, `HasUntrustedJustification()`, `HasDestructiveVerb()` predicates.
- `(*Champion).AcceptToolCall(ctx, ToolCall) → (ToolCallDecision, error)` — the gate.
- Three rules in priority order:
  1. **Rule 3 (highest priority)** — destructive shell verb in any string arg → hard deny (regardless of justification trust).
  2. **Rule 2** — mutating tool + external-trust justification → deny pending out-of-band user confirmation. `ToolCallDecision.PendingConfirmation = true`.
  3. **Rule 1** — existing path-allowlist gate (delegates to `validatePath` for `path`-bearing tools).
- Every gate decision is logged to `ActionStore` with a status string: `accepted` | `denied_destructive` | `denied_untrusted_justification` | `denied_path`.

**Destructive-verb pattern:** word-boundary regex covering `rm -rf`/`rm -r`/`rm /`, `drop table`/`drop database`, `mkfs(.ext4|.xfs|...)`, `dd if=`/`dd of=`, `wipe`, `shutdown`, `reboot`, `kill -9`, `truncate`, `unlink`, `git push --force`, `git reset --hard`, `chmod 000`, `> /dev/sd*`. Recurses into nested `map[string]any` and `[]any` so an attacker can't smuggle a destructive command via a nested argv slice.

**Test coverage (all green, race-clean):**

- IsMutating: 17 cases — every named mutating tool, every named read-only tool, 3 unknown-tool fail-closed cases.
- HasUntrustedJustification: 6 cases — empty, all-canonical, all-local, mixed canonical+local, single-external, buried-external.
- HasDestructiveVerb: 9 cases — empty, clean string, `rm -rf`, `drop table` buried in SQL, argv slice smuggling, nested map smuggling, `mkfs.ext4`, `git reset --hard`, clean file write.
- AcceptToolCall full matrix: allowed-readonly, allowed-mutating-trusted, denied-untrusted-mutating (with audit verification), allowed-readonly-even-if-untrusted, denied-destructive-beats-trust, destructive-precedence-over-untrusted, denied-path-outside-allowlist, unknown-tool-fail-closed-pending, audit-logged-always.

**What remains for Phase D-A (out of B2 scope):**

- The ReAct loop that constructs `ToolCall` envelopes from LLM JSON output.
- The system prompt that documents the tool-call schema to the model.
- The pending-confirmation flow itself: token issuance, single-use enforcement, 5-minute timeout, replay protection. The B2 design specs the contract (`ConfirmPendingToolCall(ctx, token)`) but doesn't implement it — the implementation belongs with whatever surfaces the user-facing prompt (CLI / TUI / Slack / etc.).
- Wiring `AcceptToolCall` into `WriteFile`/`PatchFile`/etc — currently those entry points use the old direct path-allowlist. The new gate is a strict superset; the migration is a follow-up commit (rename existing entry points to `*Internal` and add new `*ViaToolCall(ctx, ToolCall)` shims).

## D-pre.B3 — vor DoS hardening (upstream cs PR — DRAFTED 2026-05-02)

**Status:** patch drafted on cs topic branch `harden/api-dos-and-traversal` (commit `fa46000`). Awaiting Stevie's review and upstream PR to bellistech/cs.

**Owner cs side:** add `MaxQueryLength` (default 4096 chars) to the `/api/search` handler; reject queries beyond that with HTTP 413 Payload Too Large. Add path-traversal hardening to `/api/topics/:name` — explicitly reject names containing `..`, `/`, `\\`, or `\0` before the fuzzy resolver runs.

**Probe-finding citation:** `A3-vor-fuzz.md` F1 (DoS) and F2 (lax fuzzy resolver).

**Smoke results from patched build:**

```
/api/search?q=bash                         → 200 OK (legit)
/api/search?q=<5000-char-query>            → 413 query too long (NEW)
/api/topics/..                             → 404 (Go mux strips '..')
/api/topics/..%2Fetc%2Fpasswd              → 400 invalid topic name (NEW)
/api/topics/%2e%2e%2fetc                   → 400 invalid topic name (NEW)
/api/topics/A%00B                          → 400 invalid topic name (NEW)
/api/topics/bash                           → 200 OK (legit unaffected)
```

The patched cs build is running locally on 127.0.0.1:9876 from the topic branch; unheaded probes against vor benefit from the hardening immediately. Upstream merge is the official close-out.

---

## What landed in this commit (D-pre.A1–A5, A7)

- A1 destructive-verb filter clause in zhen-rag's default system prompt
- A2 RUBRIC §2 correction (don't-know=FAIL for textbook syntax)
- A3 sanitization convention (`CONTRIBUTING.md`, `probe-sanitization-check.sh`)
- A4 D-pre verify gate run (H1 verdict at seed=42 under v2 rubric)
- A5 5-seed CI gate (`scripts/coding-gate-ci.sh`, `make coding-gate-ci`)
- A7 fixture expansion (14 hard prompts, RUBRIC v2.1)

The B1/B2/B3 blockers are the remaining gate to Phase D-A (agent runtime). They are not in this commit; they require multi-repo coordination and are tracked here for future scheduling.
