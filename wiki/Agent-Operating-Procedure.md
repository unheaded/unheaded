# Agent Operating Procedure

How every AI agent session on Unheaded starts and ends, so each one picks up exactly where the last stopped. Coding standards: [[Developer Guide|Developer-Guide]].

**Rule:** every session inherits from the last and hands off to the next.

## Start

1. **Read the handoff.** `~/tmp/next.md` is the cross-session briefing (newest section on top). Big sprints also keep a session log in the repo (`docs/sessions/`, `references/`). No handoff → reconstruct from `git log --oneline -20`.
2. **Load context.** Check out `develop`; read `CLAUDE.md`, the active battle plan in `docs/battle-plans/`, `references/timeline.md`, relevant ADRs. Load the skills the work needs (Marshal oversees UPC/Doom work).
3. **Check the environment.**
   ```bash
   git status -sb && git log --oneline -5      # matches the handoff? how far ahead of origin?
   go version                                  # matches the go.mod `toolchain` line (go1.27.3)
   go build ./... && go test -race ./...
   ```
4. **Confirm goals** with Stevie, then work. Check in before long or expensive runs.

## End

1. **Write the handoff** to `~/tmp/next.md` (overwrite stale sections): what was done with commit SHAs, what is not done and why, blockers with root cause, environment notes, open questions for Stevie, priority order for next time.
2. **Update the document web**: timeline, wiki, CLAUDE.md where the change touches them ([[Home]] → Librarian rules).
3. **Commit, don't push.** Claude commits, GPG-signed; Stevie pushes. If the GPG cache has timed out during unattended work, commit with `--no-gpg-sign` and re-sign later with `scripts/resign-unpushed.py`.
4. **Final check:** build and tests still green, tree clean.

## Differences from the 2026-02 source

The source (S26 Round Table) has agents writing `docs/sessions/S<N>-*-handoff.md`, loading all twelve skills every time, and running `git push origin main`. Current practice: the handoff is `~/tmp/next.md`, work lands on `develop` (ADR-089 promotion to `staging` and `main`), and only Stevie pushes.

---

> **Source:** [docs/workflow/AGENT-OPERATING-PROCEDURE.md](../docs/workflow/AGENT-OPERATING-PROCEDURE.md) (v1.0, 2026-02-20) · `CLAUDE.md`
