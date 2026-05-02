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

**Remaining work (search ranking weights):**

The B1 design called out a configurable per-kind weight (default 1.0/0.8/0.5) so embedded ranks above user-source for tied relevance. NOT implemented in this commit — empirically the trust-label clause is sufficient for the LLM-layer behavior we cared about. Defer until a real concrete failure shows up where the model would have made a different decision had ranking been weighted.

## D-pre.B2 — Champion tool-call gating on source provenance

**Owner unheaded side (`pkg/champion/`):** every Champion tool call (`WriteFile`, `PatchFile`, etc.) must accept a `justification` argument naming the references it was based on. The Champion's `Config.AllowedPaths` allowlist is consulted, BUT — and this is the new gate — if the justification chain includes any retrieval result with `source_kind=user_symlink`, the tool call refuses unless the user has explicitly out-of-band confirmed the action.

**Why deferred:** depends on B1 landing first (the `source_kind` field has to exist before Champion can gate on it). Also requires designing the agent-loop tool-call envelope (Phase D-A scope), which we explicitly haven't started.

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
