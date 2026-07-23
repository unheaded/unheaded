# ADR-083: Terminology conventions (rolling)

- **Status:** Accepted
- **Date:** 2026-07-23
- **Type:** Rolling — append new terminology decisions as a dated subsection; do
  not supersede this ADR for each one.

## Context

Some of our internal engineering vocabulary is precise and load-bearing for how
we design, but reads oddly when quoted to an end user or adopter. Compliance and
security frameworks already have recognized names for the same properties, and
adopters trust those names. We want to keep the sharp internal language for
ourselves while presenting adopters the framework-standard framing.

This ADR is the single home for those internal-vs-user-facing naming decisions,
so the convention is discoverable in one place rather than scattered.

## Decision

Maintain a two-audience terminology policy:

- **Internal surfaces** (`CLAUDE.md`, `docs/GEMINI.md`, ADRs, code comments,
  design notes): keep the precise internal term.
- **User-facing surfaces** (the wiki served at `:20002`, public architecture
  docs, README/adopter messaging): use the framework-standard name.

Do not rewrite historical records (`docs/history/`, `docs/archive/`, past session
logs) — they are dated artifacts.

### Terminology register

| Internal term | User-facing term | Notes |
|---|---|---|
| zero user data access | **NIST 800-207 Zero Trust Architecture** | Same guarantee — the infrastructure is architecturally isolated from user data. Adopters recognize the NIST name; the internal phrase stays in code/ADRs/CLAUDE.md because it is precise about *what* is enforced. |

## 2026-07-23 — zero user data access → NIST 800-207 Zero Trust Architecture

Replaced the phrase on user-facing surfaces only:

- `wiki/Architecture.md`, `wiki/Security.md`, `wiki/README.md`
- `docs/ARCHITECTURE.md`, `docs/architecture/ARCHITECTURE.md`

Kept the internal phrase in `CLAUDE.md` (with a note pointing here),
`docs/GEMINI.md`, existing ADRs, and `cmd/lich-security` code, and untouched in
`docs/history/` and `docs/archive/`.

## Consequences

- Adopters see compliance-framework language; engineers keep the precise term.
- New terminology decisions append here as a dated subsection + a register row.
- When adding user-facing copy, check the register before quoting an internal
  term verbatim.
