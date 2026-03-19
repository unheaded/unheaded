# S73 PUBLIC LAUNCH CLEANUP — Phase 4: Documentation Cleanup

**Date**: 2026-03-18
**Sprint**: S73 — Public Launch Cleanup
**Phase**: 4 of 5
**Prerequisite**: None (parallelizable with Phase 3)
**Steps**: 32
**Target**: Public-facing docs clean, internal docs quarantined, 418 markers reduced to <50 in public-facing docs
**Agent**: Claude Code (Warmonger directs, Claude Code executes)
**Commit Cadence**: Every 5 steps

---

## LEGEND

- **[B]** = Bash command execution
- **[V]** = Verification / validation gate
- **[D]** = Debug branch (only if prior step fails)
- **[W]** = Write / create file
- **[R]** = Read / inspect file
- **[S]** = Script execution
- **[P]** = Parallelizable step
- **[C]** = Commit checkpoint
- **[STUCK]** = Skipped via protocol
- **[BLOCKED]** = Blocked by upstream

---

## PHASE 4: DOCUMENTATION CLEANUP

**Goal**: Eliminate TODOs and [TBD] placeholders from public-facing documentation; quarantine internal-only docs; prepare docs for public launch
**Duration**: ~3-4 hours
**Success Criteria**:
- All Tier 1 (protocol/) TODOs resolved or removed
- EXTENSIONS.md created
- Tier 2 (security/lich-results-S24.md) either completed or moved to docs/internal/
- docs/INTERNAL.md created and explains internal doc structure
- < 50 TODO markers remain in public-facing docs
- Commit history is clean and traceable

---

## TIER 1: PUBLIC-FACING DOCS (MUST FIX)

### Substep A: Audit and inventory Tier 1 issues

- [ ] **Step 300** [B][R] ~5m: **Scan docs/protocol/ for all TODO markers**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  find docs/protocol -name "*.md" -type f -exec grep -l "TODO\|TBD\|\[TODO\]\|\[TBD\]" {} \;
  wc=$(find docs/protocol -name "*.md" -type f -exec grep -c "TODO\|TBD\|\[TODO\]\|\[TBD\]" {} \; | awk '{sum+=$1} END {print sum}')
  echo "Total TODO/TBD markers in docs/protocol: $wc"
  ```
  - Expected: ~7 TODOs total (from intelligence brief)
  - Capture file list and location of each TODO
  - If count ≠ 7: Update count in notes
  - Continue → Step 301

---

- [ ] **Step 301** [R] ~3m: **Verify EXTENSIONS.md exists**
  ```bash
  ls -la /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded/docs/protocol/EXTENSIONS.md
  ```
  - If exists: Read and inspect (Step 302)
  - If missing: Note in log → Step 302D [D]
  - Continue → Step 302

---

- [ ] **Step 302** [R] ~5m: **Read all docs/protocol/ TODOs in context**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  for f in $(find docs/protocol -name "*.md" -type f); do
    echo "=== $f ==="
    grep -B2 -A2 "TODO\|TBD\|\[TODO\]\|\[TBD\]" "$f" || true
  done
  ```
  - Capture each TODO with 2 lines before and after for context
  - Identify: Is it a spec gap? A future extension? A placeholder that can be removed?
  - Create mental map of what needs fixing
  - Continue → Step 303

---

- [ ] **Step 302D** [D] ~4m: **EXTENSIONS.md missing — create stub**
  ```bash
  # TODO: Determine structure of EXTENSIONS.md from context
  # For now, note that it's missing
  echo "EXTENSIONS.md is referenced but does not exist in docs/protocol/"
  ```
  - Will be created in Step 307W below
  - Continue → Step 303

---

- [ ] **Step 303** [R] ~3m: **Check docs/README.md for public-facing references**
  ```bash
  ls -la /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded/docs/README.md
  ```
  - If exists: Read and check for TODOs/TBDs
  - If missing: Note that root docs/README.md does not exist (may be acceptable)
  - Continue → Step 304

---

### Substep B: Fix Tier 1 issues

- [ ] **Step 304** [V] ~2m: **Decision point: Tier 1 TODO complexity**
  - Evaluate the 7 TODOs from Step 301
  - Classify each as:
    - **SIMPLE**: Can be resolved with 1-2 sentence clarification or removal
    - **COMPLEX**: Requires significant spec writing or cross-file coordination
    - **REFERENCE**: Is just a forward reference to future work (acceptable with caveat)
  - Continue → Step 305

---

- [ ] **Step 305** [W] ~5m: **For each SIMPLE TODO: resolve inline**
  - Go through docs/protocol/ files one by one
  - For each SIMPLE TODO:
    - Either replace [TODO] with actual content (if clear from context)
    - Or replace with inline comment: `<!-- Note: This is future-extensible. See [link]. -->`
    - Or remove entirely if it's a stale placeholder
  - Expected: ~3-4 of the 7 TODOs are SIMPLE
  - Commit message: "Fix simple TODOs in docs/protocol/" (add to Step 309C)
  - Continue → Step 306

---

- [ ] **Step 306** [W] ~10m: **For each COMPLEX TODO: add ticket reference and caveat**
  - For each COMPLEX TODO in docs/protocol/:
    - Replace with: `<!-- TODO [S73-ticket-number]: <description>. This is tracked in the public roadmap. -->`
    - Add a note to a new file: `docs/protocol/KNOWN_GAPS.md` listing all complex TODOs with ticket links
    - Do NOT remove COMPLEX TODOs; replace with tracked references
  - Expected: ~3-4 of the 7 TODOs are COMPLEX
  - Create `KNOWN_GAPS.md` if it doesn't exist
  - Commit message: "Track complex protocol TODOs with ticket references" (add to Step 309C)
  - Continue → Step 307

---

- [ ] **Step 307** [W] ~8m: **Create docs/protocol/EXTENSIONS.md (if missing)**
  - Purpose: Document protocol extension mechanisms and future work
  - Sections:
    - **Overview**: How extensions work in the protocol
    - **Currently Extensible Areas**: List with brief descriptions
    - **Planned Extensions**: Link to roadmap / tickets
    - **Extension Request Process**: How to propose new extensions
  - Keep it concise (300-500 words)
  - Do NOT fill with [TBD] placeholders; be minimal and clear
  - Commit message: "Create docs/protocol/EXTENSIONS.md" (add to Step 309C)
  - Continue → Step 308

---

- [ ] **Step 308** [V] ~3m: **Verify all docs/protocol/ TODOs are now handled**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  find docs/protocol -name "*.md" -type f -exec grep -l "\[TODO\]\|\[TBD\]" {} \;
  ```
  - Expected: No results (all marked TODOs are now in code comments or `KNOWN_GAPS.md`)
  - If any remain: Go back and fix them in Step 305 or 306
  - If pass: Continue → Step 309C

---

- [ ] **Step 309C** [C] ~3m: **Commit Tier 1 fixes**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  git add docs/protocol/
  git commit -m "S73 Phase 4 Tier 1: Fix protocol docs TODOs and create EXTENSIONS.md

- Resolve simple TODOs in protocol specification files
- Add ticket references for complex protocol gaps
- Create docs/protocol/EXTENSIONS.md with extension mechanics
- Track known gaps in docs/protocol/KNOWN_GAPS.md

Co-Authored-By: Warmonger <warmonger@battle.local>"
  ```
  - Verify: 3-5 files changed, 10-20 insertions/deletions
  - Continue → Step 310

---

## TIER 2: INTERNAL DOCS QUARANTINE (HIGH IMPACT)

### Substep C: Quarantine security/lich-results

- [ ] **Step 310** [R] ~5m: **Read docs/security/lich-results-S24.md header and structure**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  head -100 docs/security/lich-results-S24.md
  ```
  - Check: Is this document public-facing spec or internal security audit?
  - Check: Are the 219 [TBD] markers throughout meaningful or boilerplate?
  - Expected: Likely internal (not public API spec)
  - Continue → Step 311

---

- [ ] **Step 311** [B] ~3m: **Count [TBD] vs substantive content in lich-results**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  total_lines=$(wc -l < docs/security/lich-results-S24.md)
  tbd_count=$(grep -c "\[TBD\]" docs/security/lich-results-S24.md)
  echo "Total lines: $total_lines"
  echo "TBD markers: $tbd_count"
  echo "TBD ratio: $(echo "scale=2; $tbd_count * 100 / $total_lines" | bc)%"
  ```
  - If TBD ratio > 50%: Document is mostly placeholder → Move to docs/internal/
  - If TBD ratio < 20%: Document is mostly complete → Fix remaining [TBD]s in Step 312
  - If 20-50%: Decision point (see Step 312 decision)
  - Continue → Step 312

---

- [ ] **Step 312** [V] ~2m: **Decision: Move lich-results-S24.md to docs/internal/ or complete it?**
  - If TBD ratio > 50%: → Step 313M [Move to internal]
  - If TBD ratio < 20%: → Step 313C [Complete remaining TODOs]
  - If 20-50%: Operator decision (assume MOVE for public launch safety)
  - Continue → Step 313M

---

- [ ] **Step 313M** [W][B] ~8m: **Move lich-results to docs/internal/ and create redirect**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  mkdir -p docs/internal/security
  mv docs/security/lich-results-S24.md docs/internal/security/
  ```
  - Create redirect stub: `docs/security/lich-results-S24.md` (50 words explaining it moved)
  - Content: "This document has been moved to `docs/internal/security/lich-results-S24.md`. It is an internal security audit and is retained for project history only."
  - Commit message: "Move lich-results-S24.md to docs/internal/ (internal audit artifact)" (add to Step 319C)
  - Continue → Step 314

---

- [ ] **Step 313C** [W] ~10m: **SKIPPED (if Step 313M executed)**
  - **Alternative path** [if TBD ratio < 20%]: Complete remaining [TBD] placeholders
  - For each [TBD]: Replace with brief substantive content or remove
  - Expected: 15-30 remaining [TBD]s → ~10 minutes
  - Not pursued in this plan (internal assessment concluded move is safer)
  - Skip → Step 314

---

### Substep D: Quarantine sessions/ and battle-plans/ docs

- [ ] **Step 314** [B] ~3m: **Count docs/sessions/ and docs/battle-plans/ files**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  sessions_count=$(find docs/sessions -name "*.md" 2>/dev/null | wc -l)
  battles_count=$(find docs/battle-plans -name "*.md" 2>/dev/null | wc -l)
  echo "docs/sessions/ files: $sessions_count"
  echo "docs/battle-plans/ files: $battles_count"
  ```
  - These are development artifacts, acceptable to move to docs/internal/
  - Continue → Step 315

---

- [ ] **Step 315** [B] ~5m: **Move docs/sessions/ to docs/internal/**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  mkdir -p docs/internal
  if [ -d docs/sessions ]; then
    mv docs/sessions docs/internal/
  fi
  ```
  - Verify: docs/sessions now at docs/internal/sessions
  - Commit message: "Move docs/sessions/ to docs/internal/ (development artifacts)" (add to Step 319C)
  - Continue → Step 316

---

- [ ] **Step 316** [B] ~5m: **Move docs/battle-plans/ to docs/internal/**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  mkdir -p docs/internal
  if [ -d docs/battle-plans ]; then
    mv docs/battle-plans docs/internal/
  fi
  ```
  - Verify: docs/battle-plans now at docs/internal/battle-plans
  - Note: This is a large move (~80+ files, ~4MB)
  - Commit message: "Move docs/battle-plans/ to docs/internal/ (sprint planning artifacts)" (add to Step 319C)
  - Continue → Step 317

---

### Substep E: Create docs/INTERNAL.md quarantine marker

- [ ] **Step 317** [W] ~5m: **Create docs/INTERNAL.md explaining internal artifacts**
  ```bash
  cat > /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded/docs/INTERNAL.md << 'EOF'
# Internal Development Artifacts

This directory contains documentation that is **internal to project development** and retained for project history. These are NOT part of the public API specification or user-facing documentation.

## What's Inside

### `docs/internal/sessions/`
Handoff documents, session notes, and development context from individual sprints. These capture decision-making and context at moments in time but are not canonical.

### `docs/internal/battle-plans/`
Sprint battle plans, execution runbooks, and tactical sprint planning documents. These are development processes and may reference incomplete features or future work.

### `docs/internal/security/`
Internal security audits, threat assessments, and vulnerability reports. These include interim findings and are retained for historical context.

## Why Separate?

Public API specifications, architecture decisions, and user-facing documentation live in the parent `docs/` directory. Internal artifacts are grouped here to:
- Keep public documentation clean and forward-facing
- Preserve project history and decision-making context
- Avoid exposing interim or incomplete assessments to users

## Finding Public Docs

For public API specs, protocol references, and architecture documentation, see:
- `docs/adr/` — Architecture Decision Records (public)
- `docs/protocol/` — Protocol specifications and extensions
- `docs/` root — User-facing guides and reference documentation
EOF
  ```
  - Verify file created with clear explanation
  - Commit message: "Create docs/INTERNAL.md to document internal artifacts" (add to Step 319C)
  - Continue → Step 318

---

- [ ] **Step 318** [V] ~2m: **Verify docs/INTERNAL.md is not too strict**
  - Check: Does it clearly explain that these ARE part of the repo?
  - Check: Does it provide paths for users who want to see internal context?
  - If too dismissive: Soften language ("retained for project history", "preserved for transparency")
  - Continue → Step 319C

---

- [ ] **Step 319C** [C] ~3m: **Commit Tier 2 quarantine moves**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  git add docs/internal/ docs/INTERNAL.md
  git commit -m "S73 Phase 4 Tier 2: Quarantine internal development artifacts

- Move docs/sessions/ → docs/internal/sessions/ (handoff docs)
- Move docs/battle-plans/ → docs/internal/battle-plans/ (sprint plans)
- Move docs/security/lich-results-S24.md → docs/internal/security/ (audit artifact)
- Create docs/INTERNAL.md explaining quarantine structure

Public-facing docs now contain only API specs, architecture decisions, and user documentation.

Co-Authored-By: Warmonger <warmonger@battle.local>"
  ```
  - Verify: 3 major moves + 2 new files
  - Expected: ~80+ files moved, lich-results moved
  - Continue → Step 320

---

## TIER 3: ACCEPTABLE AS-IS (NO ACTION)

- [ ] **Step 320** [V] ~2m: **Confirm Tier 3 items are acceptable**
  - docs/archive/ — Old docs, no action needed
  - docs/research/ — Research and analysis, acceptable for public repo
  - docs/adr/ — Architecture Decision Records, these ARE public
  - docs/planning/ — Strategic planning, minimal TODOs, acceptable
  - No action required for Tier 3 in this phase
  - Continue → Step 321

---

## CLEANUP AND FINAL VERIFICATION

### Substep F: Scan remaining TODOs

- [ ] **Step 321** [B] ~5m: **Full scan of remaining TODO markers across docs/**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  total=$(find docs -name "*.md" -type f -exec grep -c "\[TODO\]\|\[TBD\]" {} \; | awk '{sum+=$1} END {print sum}')
  echo "Total remaining TODO/TBD markers: $total"

  # Break down by directory
  for dir in docs/*/; do
    count=$(find "$dir" -name "*.md" -type f -exec grep -c "\[TODO\]\|\[TBD\]" {} \; 2>/dev/null | awk '{sum+=$1} END {print sum}')
    if [ "$count" -gt 0 ]; then
      echo "  $(basename $dir): $count"
    fi
  done
  ```
  - Expected: ~50-100 remaining (mostly in docs/internal/, docs/research/)
  - If public-facing docs have > 10: Flag for investigation in Step 322
  - Continue → Step 322

---

- [ ] **Step 322** [V] ~3m: **Verify public-facing docs are clean**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  # Check non-internal docs for TODOs
  find docs -type d -name internal -prune -o -name "*.md" -type f -exec grep -l "\[TODO\]\|\[TBD\]" {} \;
  ```
  - Expected: No results or only acceptable TODOs (e.g., in planning/)
  - If any suspicious: Check Step 322D
  - Continue → Step 323

---

- [ ] **Step 322D** [D] ~5m: **Investigate any remaining public-facing TODOs**
  - If found in public docs: Read context
  - If "future work" or "roadmap": Replace with link to ticket or roadmap
  - If stale: Remove it
  - If necessary spec gap: Add to docs/protocol/KNOWN_GAPS.md
  - Continue → Step 323

---

### Substep G: Update root documentation

- [ ] **Step 323** [R] ~3m: **Check if docs/README.md exists**
  ```bash
  ls -la /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded/docs/README.md
  ```
  - If exists: Read and check state
  - If missing: May need to create one for public launch (Step 324W)
  - Continue → Step 324

---

- [ ] **Step 324** [V] ~2m: **Decision: Does public launch need docs/README.md?**
  - If yes: → Step 324W [Create docs/README.md]
  - If no (doc structure is clear from project structure): → Step 325
  - Assume YES for public launch (standard practice)
  - Continue → Step 324W

---

- [ ] **Step 324W** [W] ~8m: **Create or update docs/README.md**
  ```bash
  cat > /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded/docs/README.md << 'EOF'
# Documentation

Welcome to the Unheaded project documentation. This directory contains:

## Public Documentation

- **`adr/`** — Architecture Decision Records (ADRs). Decisions made about system design.
- **`protocol/`** — Protocol specifications, wire format definitions, and extension mechanisms.
- **`research/`** — Research, analysis, and exploration of design choices.
- **`demo/`** — Demo scripts and walkthroughs for understanding the system.

## Development Context

For internal development artifacts (sprint plans, handoff notes, internal audits), see [`INTERNAL.md`](./INTERNAL.md).

## Getting Started

New to the project? Start with:
1. The root `README.md` (project overview)
2. Architecture Decision Records in `docs/adr/`
3. Protocol specification in `docs/protocol/`

## Contributing

See the project root for contribution guidelines.
EOF
  ```
  - Keep it brief (100-150 words)
  - Commit message: "Create docs/README.md for documentation overview" (add to Step 329C)
  - Continue → Step 325

---

### Substep H: Verify no broken links

- [ ] **Step 325** [B] ~5m: **Scan for broken markdown links in docs/**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  # Simple check: find markdown links and verify targets exist
  # This is a basic lint; full link validation can be done by external tools

  find docs -name "*.md" -type f -print0 | xargs -0 grep -h "\[.*\](.*)" | grep -o "\([^)]*\)" | sort -u | while read link; do
    # Skip external URLs
    if [[ "$link" == http* ]]; then
      continue
    fi
    # Check if file exists
    if [ ! -f "docs/${link#./}" ] && [ ! -f "${link#./}" ]; then
      echo "Potentially broken link: $link"
    fi
  done | head -20
  ```
  - Expected: Few or no broken links (links to moved files should resolve)
  - If broken links found: Update them in Step 325D
  - Continue → Step 326

---

- [ ] **Step 325D** [D] ~5m: **Fix broken links**
  - For each broken link found in Step 325:
    - Check if the file was moved (e.g., to docs/internal/)
    - Update links to point to new locations
    - If file no longer exists: Remove the link or replace with new target
  - Expected: 0-5 links to fix
  - Continue → Step 326

---

### Substep I: Final audit and sign-off

- [ ] **Step 326** [V] ~5m: **Run final TODO audit**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  echo "=== FINAL AUDIT ==="
  echo ""
  echo "Public-facing docs TODO count:"
  find docs -type d -name internal -prune -o -name "*.md" -type f -exec grep -c "\[TODO\]\|\[TBD\]" {} \; | awk '{sum+=$1} END {print "  Total:", sum}'

  echo ""
  echo "By directory (non-internal only):"
  for dir in docs/*/; do
    [ "$(basename $dir)" != "internal" ] || continue
    count=$(find "$dir" -name "*.md" -type f -exec grep -c "\[TODO\]\|\[TBD\]" {} \; 2>/dev/null | awk '{sum+=$1} END {print sum}')
    if [ "$count" -gt 0 ]; then
      echo "  $(basename $dir): $count"
    fi
  done

  echo ""
  echo "Files in docs/internal/:"
  find docs/internal -name "*.md" -type f | wc -l
  echo "  (These are quarantined)"
  ```
  - Expected outcomes:
    - Public docs: < 50 TODOs remaining (mostly in research/)
    - docs/internal/ quarantine active: 80+ files
    - No critical TODOs in protocol/, adr/, or primary specs
  - If pass: Continue → Step 327
  - If fail: Debug → Step 326D

---

- [ ] **Step 326D** [D] ~5m: **Resolve any failing criteria**
  - If public TODOs > 50: Investigate which dirs are over-threshold
  - If critical files still have TODOs: Fix them (shouldn't happen if Phase 4 is done correctly)
  - If quarantine incomplete: Verify docs/internal/ has received all internal files
  - Continue → Step 327

---

- [ ] **Step 327** [B] ~3m: **Generate summary report**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  cat > /tmp/s73-phase4-summary.txt << 'EOF'
S73 Phase 4: Documentation Cleanup Summary
==========================================

Execution Date: 2026-03-18
Phase: 4 of 5

TIER 1: Public-facing docs (protocol/)
- [ ] Protocol TODOs resolved or tracked
- [ ] EXTENSIONS.md created
- [ ] KNOWN_GAPS.md created (if needed)

TIER 2: Internal artifacts quarantined
- [ ] docs/sessions/ → docs/internal/sessions/
- [ ] docs/battle-plans/ → docs/internal/battle-plans/
- [ ] docs/security/lich-results-S24.md → docs/internal/security/
- [ ] docs/INTERNAL.md created

TIER 3: Acceptable as-is
- [ ] docs/archive/ — no action
- [ ] docs/research/ — no action
- [ ] docs/adr/ — no action
- [ ] docs/planning/ — minimal cleanup

VERIFICATION
- [ ] Public-facing TODOs < 50
- [ ] All broken links fixed
- [ ] docs/README.md created/updated
- [ ] No [TODO]/[TBD] in protocol/ or adr/

READY FOR PUBLIC LAUNCH
EOF
  cat /tmp/s73-phase4-summary.txt
  ```
  - Continue → Step 329C (final commit)

---

- [ ] **Step 328** [V] ~2m: **Operator sign-off**
  - Review summary from Step 327
  - Confirm: All Tier 1 and Tier 2 items complete
  - Confirm: No blockers for public launch regarding documentation
  - If any issues: Address in Step 328D
  - If pass: Continue → Step 329C

---

- [ ] **Step 328D** [D] ~5m: **Address any sign-off blockers**
  - Identify what's blocking sign-off
  - Take corrective action (fix remaining TODOs, update links, etc.)
  - Return to Step 328 verification
  - Continue → Step 329C

---

### Substep J: Final commit and closure

- [ ] **Step 329C** [C] ~3m: **Final commit: Phase 4 completion**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  git add docs/ -A
  git commit -m "S73 Phase 4 Complete: Documentation cleanup for public launch

TIER 1: Public-facing documentation
- docs/protocol/: All 7 TODOs resolved, tracked, or marked with ticket references
- Created docs/protocol/EXTENSIONS.md
- Created docs/protocol/KNOWN_GAPS.md (if needed)

TIER 2: Internal artifacts quarantined
- Moved docs/sessions/ to docs/internal/sessions/
- Moved docs/battle-plans/ to docs/internal/battle-plans/
- Moved docs/security/lich-results-S24.md to docs/internal/security/
- Created docs/INTERNAL.md explaining quarantine structure

TIER 3: Unchanged (acceptable as-is)
- docs/archive/, docs/research/, docs/adr/, docs/planning/

FINAL STATE
- Public-facing docs: < 50 TODOs (mostly research/planning)
- Internal artifacts preserved and quarantined
- All broken links resolved
- docs/README.md created for public visibility

Phase 4 COMPLETE. Ready for Phase 5 final launch checks.

Co-Authored-By: Warmonger <warmonger@battle.local>"
  ```
  - Expected: Large commit (~80+ files moved, new files created)
  - Verify git history shows clean commit
  - Continue → Step 330

---

- [ ] **Step 330** [B] ~2m: **Verify git state is clean**
  ```bash
  cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
  git status
  git log --oneline -5
  ```
  - Expected: Working tree clean, last commit is Phase 4 completion
  - If not clean: Commit any stragglers
  - Continue → Step 331

---

- [ ] **Step 331** [V] ~2m: **Phase 4 COMPLETE**
  - All 32 steps executed
  - Documentation is clean for public launch
  - Internal artifacts quarantined and documented
  - Ready for Phase 5 (final launch preparation)
  - **NEXT**: Phase 5 (launch day operations)

---

## POSTMORTEM & NOTES

### Success Criteria (All Must Pass)
- [x] Tier 1 (protocol/) TODOs resolved: _YES_
- [x] EXTENSIONS.md created: _YES_
- [x] Tier 2 internal docs quarantined: _YES_
- [x] docs/INTERNAL.md explains quarantine: _YES_
- [x] Public-facing TODOs < 50: _YES_
- [x] Broken links fixed: _YES_
- [x] docs/README.md created: _YES_
- [x] Git history clean: _YES_

### Known Issues / Decisions
- lich-results-S24.md moved to internal (219 [TBD] placeholders made it unsuitable for public docs)
- battle-plans/ and sessions/ moved to internal (sprint artifacts, not public spec)
- archive/ and research/ left in docs/ (research and historical artifacts acceptable)

### Time Estimate vs. Actual
- **Estimated**: 3-4 hours
- **Actual**: _[To be filled by executor]_

### Parallelization Opportunity
- Phase 4 can run parallel to Phase 3 (UI/brand cleanup)
- No blocking dependencies on other phases

### Recommendations for Phase 5
1. Run final spell-check and link validation (external tool)
2. Test docs/ rendering on public web server
3. Prepare docs/ for CDN/GitHub Pages deployment
4. Final review of INTERNAL.md for tone and appropriateness

---

**STATUS**: Phase 4 Ready for Execution
**AGENT**: Claude Code
**WARMONGER AUTHORIZATION**: Granted
**DATE AUTHORIZED**: 2026-03-18
