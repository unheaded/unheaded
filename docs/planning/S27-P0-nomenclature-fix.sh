#!/usr/bin/env bash
# =============================================================================
# S27 P0: Mad-Maria Nomenclature Fix
# =============================================================================
# WHAT: Replace MATRIARCH/PATRIARCH → Mad-Maria in 2 read-only skill files
# WHY:  Cultural debt — Mad-Maria is canonical (S26 Round Table decision)
# WHO:  Lore (owner), verified by Kingdom
# WHEN: First action of S27 — blocks nothing but fixes cultural drift
# =============================================================================
set -euo pipefail

REPO_ROOT="${REPO_ROOT:-$HOME/tmp/unheaded}"
SKILL_BASE="${REPO_ROOT}/.skills/skills"
FIXED=0
FAILED=0

echo "═══════════════════════════════════════════"
echo "  P0: MAD-MARIA NOMENCLATURE FIX"
echo "═══════════════════════════════════════════"

# --- TARGET FILES ---
FILES=(
  "${SKILL_BASE}/unheaded-moatghost/SKILL.md"
  "${SKILL_BASE}/unheaded-kingdom/SKILL.md"
)

# --- PRE-FLIGHT CHECK ---
echo ""
echo "[PRE-FLIGHT] Checking file access..."
for f in "${FILES[@]}"; do
  if [[ ! -f "$f" ]]; then
    echo "  FAIL: File not found: $f"
    echo "  ┗━ RECOVERY: Check if skills are mounted. Try: ls ${SKILL_BASE}/"
    echo "  ┗━ RECOVERY: If skills moved, find them: find ${REPO_ROOT} -name 'unheaded-moatghost' -type d"
    FAILED=$((FAILED + 1))
    continue
  fi
  if [[ ! -w "$f" ]]; then
    echo "  WARN: File not writable: $f"
    echo "  ┗━ RECOVERY: Try chmod u+w $f"
    echo "  ┗━ RECOVERY: If git-controlled: git checkout -- $f && chmod u+w $f"
    echo "  ┗━ RECOVERY: If NixOS-managed: Edit in nix config, not directly"
  else
    echo "  OK: $f (readable + writable)"
  fi
done

if [[ $FAILED -gt 0 ]]; then
  echo ""
  echo "[ABORT] $FAILED file(s) not found. Fix paths before proceeding."
  exit 1
fi

# --- BACKUP ---
echo ""
echo "[BACKUP] Creating backups..."
for f in "${FILES[@]}"; do
  cp "$f" "${f}.bak.$(date +%Y%m%d%H%M%S)"
  echo "  OK: Backed up $f"
done

# --- VERIFY PATTERN EXISTS ---
echo ""
echo "[VERIFY] Checking for MATRIARCH/PATRIARCH occurrences..."
TOTAL_MATCHES=0
for f in "${FILES[@]}"; do
  COUNT=$(grep -c "MATRIARCH\|PATRIARCH" "$f" 2>/dev/null || true)
  echo "  $f: $COUNT occurrence(s)"
  TOTAL_MATCHES=$((TOTAL_MATCHES + COUNT))
done

if [[ $TOTAL_MATCHES -eq 0 ]]; then
  echo ""
  echo "[SKIP] No MATRIARCH/PATRIARCH found — already fixed or pattern changed."
  echo "  ┗━ VERIFY: grep -ri 'matriarch\|patriarch' ${SKILL_BASE}/"
  echo "  ┗━ VERIFY: grep -ri 'mad-maria\|mad.maria' ${SKILL_BASE}/"
  exit 0
fi

# --- EXECUTE FIX ---
echo ""
echo "[FIX] Replacing MATRIARCH/PATRIARCH → MAD-MARIA..."
for f in "${FILES[@]}"; do
  # The specific line from S26 handoff:
  # ║    👑 THE MATRIARCH/PATRIARCH 👑     ║
  # Replace with:
  # ║         👑 MAD-MARIA 👑              ║
  if sed -i 's/║    👑 THE MATRIARCH\/PATRIARCH 👑     ║/║         👑 MAD-MARIA 👑              ║/g' "$f"; then
    echo "  OK: Fixed $f"
    FIXED=$((FIXED + 1))
  else
    echo "  FAIL: sed failed on $f"
    echo "  ┗━ RECOVERY: Manual edit — open $f, find MATRIARCH/PATRIARCH, replace with MAD-MARIA"
    echo "  ┗━ RECOVERY: Restore backup: cp ${f}.bak.* $f"
    FAILED=$((FAILED + 1))
  fi
done

# --- POST-FIX VERIFICATION ---
echo ""
echo "[POST-VERIFY] Checking no MATRIARCH/PATRIARCH remains..."
REMAINING=0
for f in "${FILES[@]}"; do
  COUNT=$(grep -c "MATRIARCH\|PATRIARCH" "$f" 2>/dev/null || true)
  if [[ $COUNT -gt 0 ]]; then
    echo "  WARN: $f still has $COUNT occurrence(s)"
    echo "  ┗━ SHOW: grep -n 'MATRIARCH\|PATRIARCH' $f"
    REMAINING=$((REMAINING + COUNT))
  else
    echo "  OK: $f clean"
  fi
done

echo ""
echo "[VERIFY] Checking MAD-MARIA is present..."
for f in "${FILES[@]}"; do
  COUNT=$(grep -c "MAD-MARIA" "$f" 2>/dev/null || true)
  echo "  $f: $COUNT MAD-MARIA occurrence(s)"
done

# --- SUMMARY ---
echo ""
echo "═══════════════════════════════════════════"
echo "  P0 SUMMARY"
echo "  Fixed:    $FIXED file(s)"
echo "  Failed:   $FAILED file(s)"
echo "  Remaining MATRIARCH/PATRIARCH: $REMAINING"
echo "═══════════════════════════════════════════"

if [[ $FAILED -gt 0 || $REMAINING -gt 0 ]]; then
  echo "  STATUS: PARTIAL — manual intervention needed"
  exit 1
else
  echo "  STATUS: COMPLETE ✅"
  echo ""
  echo "  NEXT: git add ${FILES[*]} && git commit -m 'fix(lore): replace MATRIARCH/PATRIARCH with Mad-Maria in skill files'"
fi
