# Deferred — pre-D-A blockers requiring multi-repo coordination

These three blockers from `SYNTHESIS.md` cannot land cleanly in a single unheaded commit because they require coordination with the upstream `cs/vor` repo (https://github.com/bellistech/vor or its post-rename successor) and/or design work that depends on cross-cutting source-provenance plumbing.

## D-pre.B1 — Source provenance plumbing

**Owner cs side:** label every retrieval result with `source_kind` (`embedded` | `user_symlink:<target>`) and per-source weight. Requires:

- Adding a `Source` field to `internal/registry/Sheet` in cs that records whether the sheet came from `go:embed`, `~/.config/cs/sheets/`, or `~/.config/cs/sources/<symlink>` (and which symlink).
- Plumbing that field through `/api/topics/:name` and `/api/search` JSON responses.
- A configurable per-source weight in BM25-like ranking so embedded sheets can rank above user-added sources by default.

**Owner unheaded side (downstream):** zhen-rag passes `source_kind` from the retrieval JSON into the references block presented to the model. The system prompt is updated to instruct the model to flag any answer that depends on `user_symlink` content as "untrusted-derived; verify."

**Why deferred:** changing cs's response schema is a public-API change. Coordinate with Stevie and the cs maintainers; cut a cs minor release; bump cs version pin in the Makefile or docs; verify backward compat for older zhen-rag callers. Not a half-day fix.

## D-pre.B2 — Champion tool-call gating on source provenance

**Owner unheaded side (`pkg/champion/`):** every Champion tool call (`WriteFile`, `PatchFile`, etc.) must accept a `justification` argument naming the references it was based on. The Champion's `Config.AllowedPaths` allowlist is consulted, BUT — and this is the new gate — if the justification chain includes any retrieval result with `source_kind=user_symlink`, the tool call refuses unless the user has explicitly out-of-band confirmed the action.

**Why deferred:** depends on B1 landing first (the `source_kind` field has to exist before Champion can gate on it). Also requires designing the agent-loop tool-call envelope (Phase D-A scope), which we explicitly haven't started.

## D-pre.B3 — vor DoS hardening (upstream cs PR)

**Owner cs side:** add `MaxQueryLength` (default 4096 chars) to the `/api/search` handler; reject queries beyond that with HTTP 413 Payload Too Large. Add path-traversal hardening to `/api/topics/:name` — explicitly reject percent-encoded `..`, `/`, `\0` before the fuzzy resolver runs.

**Probe-finding citation:** `A3-vor-fuzz.md` F1 (DoS) and F2 (lax fuzzy resolver).

**Why deferred:** upstream PR; doesn't require unheaded-side changes. Open as a cs issue with the A3-vor-fuzz.md findings linked.

---

## What landed in this commit (D-pre.A1–A5, A7)

- A1 destructive-verb filter clause in zhen-rag's default system prompt
- A2 RUBRIC §2 correction (don't-know=FAIL for textbook syntax)
- A3 sanitization convention (`CONTRIBUTING.md`, `probe-sanitization-check.sh`)
- A4 D-pre verify gate run (H1 verdict at seed=42 under v2 rubric)
- A5 5-seed CI gate (`scripts/coding-gate-ci.sh`, `make coding-gate-ci`)
- A7 fixture expansion (14 hard prompts, RUBRIC v2.1)

The B1/B2/B3 blockers are the remaining gate to Phase D-A (agent runtime). They are not in this commit; they require multi-repo coordination and are tracked here for future scheduling.
