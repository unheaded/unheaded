# Branch Audit — `public-release-cleanup`

**Audit date**: 2026-04-27
**Auditor**: Cowork-on-Macbook session
**Audited at HEAD**: `2e01fc09` (main)

## Summary

| Field | Value |
|---|---|
| Branch | `public-release-cleanup` |
| Origin | Local + remote (`origin/public-release-cleanup`) |
| Commits ahead of main | **1017** |
| Commits behind main | **515** |
| Merge base with main | **NONE** (`fatal: no merge base`) |
| Most recent commit on branch | `ce4ff171 docs(protocol): add IETF submission XML and text outputs` |
| Notable content | Heavy overlap with `docs/s73-public-launch-planning` (shared commits) + UPC weeks 5-12 (MBC Linux boot) + DOOM rendering wins |
| Verdict | **DEFER — LIKELY DUPLICATE / PARTIAL OVERLAP WITH `docs/s73-public-launch-planning`** |

## Why "no merge base"

Same parallel-history pattern. Branch traces to `aceb1b51` initial commit but no shared merge base with main.

## Critical observation: shared commits with `docs/s73-public-launch-planning`

This branch's top commits **exactly mirror** many on `docs/s73-public-launch-planning`:

| Commit | Also on s73 branch? |
|---|---|
| `ce4ff171 docs(protocol): add IETF submission XML and text outputs` | ✅ YES |
| `aaa7ffe3 security: fix pre-public audit findings` | ✅ YES |
| `47f826a2 docs: polish documentation for public release` | ✅ YES |
| `97e3316a chore: prepare repository for public release` | ✅ YES |
| `c5072695 chore: remove tracked binaries, update .gitignore` | ✅ YES |
| `bbf5a6ea feat(zhen): hybrid inference, context benchmark` | ✅ YES |
| `e8ebdc18 feat(zhen): Wikipedia embedded into FAISS — 1.67M vectors live` | ✅ YES |
| `d50c7a63 feat(doom): SCREEN_BASE alignment fix — DOOM RENDERS ON THE UPC!` | ✅ YES |
| `01c3ed98 feat(doom): full doomgeneric cross-compilation pipeline` | ✅ YES |
| `ac382983 feat(network): WireGuard IPv6 overlay design + configs` | ✅ YES |
| `93cb0826 docs: polish README + create demo video script` | ✅ YES |

**The two branches are likely siblings or one is a snapshot of the other.** No need to triage both independently.

## Headline content unique to this branch (or notable)

Top of the branch (most recent commits) does NOT include the s73-branch's most-recent items (`0d476ba3`, `66f4b2b4`, `304360be`, `a5cd996e`). This suggests:

**`docs/s73-public-launch-planning` is the SUPERSET; `public-release-cleanup` is older or a partial subset.**

Lower in the branch, this one includes the **UPC Linux Boot work** (Weeks 5-12 of S-LINUX):
- `c88239ce feat(upc): arch/mbc process management + memory init — Week 4 of S-LINUX`
- `72c349db feat(upc): arch/mbc nommu memory + root filesystem — Weeks 5-6 of S-LINUX`
- `47f85c6e feat(upc): Weeks 7-8 — init + rootfs + boot script = MBC LINUX BOOTS`
- `d6a72260 feat(upc): Weeks 9-10 — shell utilities + enhanced rootfs`
- `55021833 docs(upc): Weeks 11-12 — performance analysis + demo materials`

Per CLAUDE.md, "UPC Dream Ladder: All Level 4 OS primitives + Level 5 boot + Level 6 arch/mbc (37 files)" is on main — these commits are likely already merged.

## Risk classification

- **PROBABLE DUPLICATE** of `docs/s73-public-launch-planning` (subset).
- **MEDIUM-LOW priority** — triage subordinate to s73-branch triage; whatever happens to s73 governs this one.
- **No unique content identified** beyond what's on s73-branch (verify in triage).

## Recommended next action (DEFER + LIKELY DELETE-AFTER-VERIFY)

1. **Preserve** with archive tag:
   ```
   git tag archived/public-release-cleanup-2026-04-27 public-release-cleanup
   ```
2. **Verify-and-delete after s73 triage**: after `docs/s73-public-launch-planning` triage completes, confirm this branch contains zero unique content via `git log public-release-cleanup --not docs/s73-public-launch-planning --oneline`. If empty → safe to delete.
3. **If unique content found** → cherry-pick into the s73 triage workflow.

**This branch should NOT survive past the s73 triage sprint.**

## Linux-side execution checklist

```bash
cd ~/tmp/unheaded
git fetch --all
git tag archived/public-release-cleanup-2026-04-27 public-release-cleanup
git push origin archived/public-release-cleanup-2026-04-27

# Verify subset relationship:
git log public-release-cleanup --not docs/s73-public-launch-planning --oneline | tee /tmp/unique-to-cleanup.txt | wc -l
# If 0 → confirmed subset → delete safely after s73 closes.

# After s73 triage closes:
git branch -D public-release-cleanup
git push origin :public-release-cleanup
```

## Sign-off

- [ ] Marshal — DEFER + verify-subset-then-delete acknowledged
- [ ] Captain — informational only; subordinate to s73 disposition
- [ ] Stevie — final disposition

---
*Audited from Cowork-on-Macbook 2026-04-27. Likely duplicate of s73-public-launch-planning.*
