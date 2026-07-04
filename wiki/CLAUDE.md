# CLAUDE.md — Wiki source of truth

**This directory (`unheaded/wiki/`) is the SINGLE SOURCE OF TRUTH for the
Unheaded wiki.** Edit pages here. Everything downstream is generated from it.

## The flow

```
unheaded/wiki/            ← YOU ARE HERE. Edit pages here. Versioned with the code.
      │   scripts/sync-wiki.sh   (rsync, from the repo root)
      ▼
~/tmp/unheaded-wiki/      ← generated mirror (repo: unheaded.wiki.git). Do NOT edit.
      │   git push origin
      ▼
GitHub wiki  +  standalone wiki on :20002   ← published
```

Never edit `~/tmp/unheaded-wiki/` by hand — the sync overwrites it.

## Publishing

From the repo root:

```bash
scripts/sync-wiki.sh              # copy source → mirror, report orphans, no push
scripts/sync-wiki.sh --dry-run    # preview only
scripts/sync-wiki.sh --push       # sync, then commit + push the mirror to GitHub
scripts/sync-wiki.sh --prune --push   # also delete mirror-only pages, then publish
```

Because the two dirs are kept standardized (below), mirroring is a plain
filename-preserving `rsync` — the script just wraps it with an orphan report and
a non-destructive default (it never deletes a mirror-only page unless you pass
`--prune`).

## Naming conventions — keep these so the sync stays a trivial rsync

- **One flat `.md` per page.** No subdirectories. The filename (minus `.md`) is
  the GitHub wiki page name.
- **Kebab-case, Title-Cased words:** `ADR-020-Kanban-Bug-Fixes.md`,
  `Draft-Shim-00.md`, `The-Well.md`. Match the exact casing of existing pages.
- **Special GitHub-wiki files:** `Home.md` (landing page), `_Sidebar.md` (nav),
  `_Footer.md`. Keep them in sync with the page set.
- **Every page must be linked** in both `Home.md` and `_Sidebar.md` — no orphan
  pages. When you add/rename/remove a page, update both.
- **Renames:** `git mv Old-Name.md New-Name.md`, update every `[[link]]` to it,
  then run the sync with `--prune` so the old name is dropped from the mirror.
- `CLAUDE.md` (this file) and `README.md` are per-repo meta — the sync never
  copies or prunes them.

## Cross-doc ripple (Librarian rule)

A wiki page is one layer of an 8-layer document web. If a change here reflects a
code/architecture change, also check `CLAUDE.md` (repo root), `docs/`,
`references/timeline.md`, and `README.md`. Don't update the wiki in isolation.
