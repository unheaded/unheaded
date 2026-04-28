# Branch Audit — `docs/s73-public-launch-planning`

**Audit date**: 2026-04-27
**Auditor**: Cowork-on-Macbook session
**Audited at HEAD**: `2e01fc09` (main)

## Summary

| Field | Value |
|---|---|
| Branch | `docs/s73-public-launch-planning` |
| Origin | Local + remote (`origin/docs/s73-public-launch-planning`) |
| Commits ahead of main | **1017** |
| Commits behind main | **525** |
| Merge base with main | **NONE** (`fatal: no merge base`) |
| Most recent commit on branch | `0d476ba3 chore: ignore xml2rfc generated .txt and .html build artifacts` |
| Notable content | Public-launch prep, RFC blockers fixed, MBC ISA + Shim Pipeline I-Ds, ADR-69420 (Sleipnir/Yggdrasil), README rewrites, IETF submission XML |
| Verdict | **ARCHIVE-TAG + DELETE** (revised 2026-04-27 per Stevie: only main has been used; this branch is stale-and-sitting). High-value items not on main (Sophia/Wotan draft-03, WireGuard IPv6, demo script) should be re-implemented fresh on main rather than cherry-picked from a 1017-commit divergence. |

## Why "no merge base"

Same parallel-history pattern as `claude/migrate-packages-github-V2Ctr` — both branches trace to `aceb1b51` (2026-01-26 initial commit) but diverged via separate trajectories that no longer share a common merge base with current main.

## Headline content (high signal)

This branch contains the **S73 public launch sprint** content. Some of it has clearly already landed on main (per CLAUDE.md, ADR-69420 is referenced). Other items are listed as "Age 2 remaining" in CLAUDE.md and may be exclusive to this branch:

- `0d476ba3 chore: ignore xml2rfc generated .txt and .html build artifacts`
- `66f4b2b4 docs: ADR-69420 Sleipnir (Kingdom BGP) + Yggdrasil (Unheaded OS)` *(ADR-69420 IS on main per `docs/adr/ADR-INDEX.md` — likely already merged)*
- `304360be docs(protocol): add MBC ISA and Shim Pipeline Internet-Drafts`
- `a5cd996e pre-public: fix RFC blockers, rewrite README, forge S73 battle plan`
- `c4a0fdcf docs(protocol): advance Foundation draft-06, Sophia draft-03, Wotan draft-03` *(per CLAUDE.md, Foundation draft-06 is mentioned as on main; Sophia/Wotan draft-03 listed as "Age 2 remaining" — may not be on main)*
- `ac382983 feat(network): WireGuard IPv6 overlay design + configs` *(WireGuard listed as "Age 2 Remaining Epics" in CLAUDE.md — likely **NOT** on main)*
- `93cb0826 docs: polish README + create demo video script` *(demo video listed as "Age 2 remaining" — likely NOT on main)*
- `aaa7ffe3 security: fix pre-public audit findings — remove credentials and binaries` *(security-relevant — must verify on main)*
- Heavy Doom rendering work: `d50c7a63 SCREEN_BASE alignment fix — DOOM RENDERS ON THE UPC!`, `bba3b242 non-fatal I_Error + fclose no-op — 8B insns, 140 frames`, etc.
- `e8ebdc18 feat(zhen): Wikipedia embedded into FAISS — 1.67M vectors live` *(per CLAUDE.md "1.52M vector corpus" on main — branch may have a slightly different snapshot)*

## Risk classification

- **HIGH SIGNAL content** — actual public-launch deliverables: WireGuard IPv6, Sophia/Wotan draft-03, demo video script, README polish, IETF submission XML.
- **HIGH OVERLAP risk** — many commits likely already on main (ADR-69420, Foundation draft-06, parts of UPC Doom, Zhen RAG corpus).
- **PARALLEL HISTORIES** — wholesale merge would mangle main.
- **Public-relevance** — this branch contains the credentials-removal + binaries-removal + .gitignore tightening that *was* the pre-public scrubbing pass. If main has not yet been scrubbed equivalently, this is a **public-launch blocker** that must be triaged before any public push.

## Recommended next action (DEFER + EXPEDITED TRIAGE)

This branch is more time-sensitive than the V2Ctr branch because of public-launch implications.

1. **Preserve** with archive tag immediately:
   ```
   git tag archived/s73-public-launch-planning-2026-04-27 docs/s73-public-launch-planning
   ```
2. **Expedited triage** as part of the Captain Track call (Phase 4 of this sprint):
   - If Track B (launch-first) or Track C (twin-track) is chosen, this branch's triage becomes P0.
   - If Track A (forge-first) is chosen, triage deferred to post-Track-A close.
3. **Specific commits to validate against main FIRST** (gating items for any public push):
   - `aaa7ffe3 security: fix pre-public audit findings`
   - `c5072695 chore: remove tracked binaries`
   - `51f5d7af chore(docs): remove personal references`
4. **Specific commits to consider cherry-picking** (high-value Age 2 remaining):
   - `c4a0fdcf docs(protocol): advance Foundation draft-06, Sophia draft-03, Wotan draft-03`
   - `ac382983 feat(network): WireGuard IPv6 overlay design + configs`
   - `93cb0826 docs: polish README + create demo video script`
   - `04672076 docs(protocol): add IETF Internet-Draft submission guide`

## Linux-side execution checklist

```bash
cd ~/tmp/unheaded
git fetch --all
git tag archived/s73-public-launch-planning-2026-04-27 docs/s73-public-launch-planning
git push origin archived/s73-public-launch-planning-2026-04-27

# Validate: are credentials/binaries present on current main?
git ls-tree -r --name-only main | grep -iE 'cred|secret|\.key$|\.pem$' || echo "NO CREDENTIALS ON MAIN"
git ls-tree -r --name-only main | grep -iE '\.bin$|\.so$|\.a$' | head

# Triage walk-through (focus on public-launch-blocking items first):
git log main..docs/s73-public-launch-planning --oneline > /tmp/triage-s73.txt
```

## Sign-off

- [ ] Captain — Track decision unblocks priority of this triage
- [ ] Marshal — verdict DEFER acknowledged
- [ ] Barrister — security-fixing commits flagged as gating
- [ ] Stevie — final disposition

---
*Audited from Cowork-on-Macbook 2026-04-27. Public-launch readiness depends on this triage.*
