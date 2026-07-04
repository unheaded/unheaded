#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# sync-wiki.sh — propagate the wiki from its SOURCE OF TRUTH to the GitHub mirror.
#
#   SOURCE OF TRUTH:  unheaded/wiki/            (this repo, versioned with the code)
#        │  rsync (this script)
#        ▼
#   MIRROR:           ~/tmp/unheaded-wiki/      (separate repo: unheaded.wiki.git)
#        │  git push origin
#        ▼
#   PUBLISHED:        GitHub wiki + standalone wiki on :20002
#
# ALWAYS edit pages in unheaded/wiki/. NEVER edit the mirror directly — the next
# sync overwrites it. See the CLAUDE.md at the top of each wiki dir for the flow.
#
# The mirror is NOT a byte-for-byte clone: a handful of pages currently live only
# in the mirror (see PRESERVE / the orphan report). So the default is
# NON-DESTRUCTIVE — it copies/updates every source page but never deletes a
# mirror-only page unless you pass --prune. This prevents silently wiping a
# published page that hasn't been migrated back into the source yet.
#
# Usage:
#   scripts/sync-wiki.sh                 # sync source -> mirror; report orphans; no push
#   scripts/sync-wiki.sh --dry-run       # show what would change; touch nothing
#   scripts/sync-wiki.sh --prune         # also delete mirror-only pages (opt-in)
#   scripts/sync-wiki.sh --push          # sync, then commit + push the mirror to GitHub
#   scripts/sync-wiki.sh --prune --push  # full mirror + publish
#
# Env overrides:
#   WIKI_SRC   (default: <repo>/wiki)
#   WIKI_DEST  (default: ~/tmp/unheaded-wiki)

set -euo pipefail

# ── Resolve paths ────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SRC="${WIKI_SRC:-$REPO_ROOT/wiki}"
DEST="${WIKI_DEST:-$HOME/tmp/unheaded-wiki}"

# Per-repo meta files (NOT wiki pages): each dir keeps its own copy, they are
# never synced between dirs, never reported as stray, and never pruned. CLAUDE.md
# is the agent guide at the top of each wiki dir; README.md is a mirror guard if
# present.
PRESERVE=("CLAUDE.md" "README.md")

# ── Flags ────────────────────────────────────────────────────────────────────
DRY_RUN=0 PRUNE=0 PUSH=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --prune)   PRUNE=1 ;;
    --push)    PUSH=1 ;;
    -h|--help) grep -E '^#( |$)' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $arg (try --help)" >&2; exit 2 ;;
  esac
done

# ── Preflight ────────────────────────────────────────────────────────────────
[ -d "$SRC" ]  || { echo "ERROR: source wiki not found: $SRC" >&2; exit 1; }
[ -d "$DEST" ] || { echo "ERROR: mirror not found: $DEST" >&2; exit 1; }
[ -d "$DEST/.git" ] || { echo "ERROR: mirror is not a git repo: $DEST" >&2; exit 1; }
command -v rsync >/dev/null || { echo "ERROR: rsync not installed" >&2; exit 1; }

echo "== wiki sync =="
echo "  source (truth): $SRC"
echo "  mirror  (repo): $DEST"
[ "$DRY_RUN" = 1 ] && echo "  MODE: dry-run (no changes written)"

# ── 1. Copy / update every source .md into the mirror ────────────────────────
# -r recursive, -c checksum (copy only on real content change → clean git diffs,
# no spurious churn from mtimes), --itemize-changes to show each write.
# The two --exclude rules for the meta files come FIRST (first match wins) so
# CLAUDE.md/README.md are never propagated; then --include '*.md' --exclude '*'
# restricts the rest to markdown. Both dirs are flat.
RSYNC_FLAGS=(-rc --itemize-changes --exclude='CLAUDE.md' --exclude='README.md' --include='*.md' --exclude='*')
[ "$DRY_RUN" = 1 ] && RSYNC_FLAGS+=(--dry-run)

echo
echo "-- copy/update (source -> mirror) --"
CHANGED="$(rsync "${RSYNC_FLAGS[@]}" "$SRC"/ "$DEST"/ | grep -E '^[<>ch]' || true)"
if [ -n "$CHANGED" ]; then echo "$CHANGED"; else echo "  (mirror already up to date)"; fi

# ── 2. Orphan report: pages in the mirror but not in the source ──────────────
in_preserve() { local f="$1"; for p in "${PRESERVE[@]}"; do [ "$f" = "$p" ] && return 0; done; return 1; }

echo
echo "-- mirror-only pages (in mirror, NOT in source) --"
ORPHANS=()
while IFS= read -r f; do
  [ -z "$f" ] && continue
  in_preserve "$f" && continue
  ORPHANS+=("$f")
done < <(comm -13 <(cd "$SRC" && ls -1 ./*.md 2>/dev/null | sed 's|^\./||' | sort) \
                  <(cd "$DEST" && ls -1 ./*.md 2>/dev/null | sed 's|^\./||' | sort))

if [ "${#ORPHANS[@]}" -eq 0 ]; then
  echo "  (none)"
else
  printf '  %s\n' "${ORPHANS[@]}"
  if [ "$PRUNE" = 1 ]; then
    echo "  --prune: deleting the above from the mirror"
    if [ "$DRY_RUN" = 0 ]; then (cd "$DEST" && rm -f -- "${ORPHANS[@]}"); fi
  else
    echo "  (kept — pass --prune to delete, or migrate them into $SRC to keep them)"
  fi
fi

# ── 3. Optionally commit + push the mirror to GitHub ─────────────────────────
if [ "$PUSH" = 1 ]; then
  echo
  echo "-- publish (commit + push mirror) --"
  if [ "$DRY_RUN" = 1 ]; then
    echo "  dry-run: would 'git add -A && git commit && git push' in $DEST"
  else
    (
      cd "$DEST"
      if [ -z "$(git status --porcelain)" ]; then
        echo "  nothing to publish (mirror unchanged)"
      else
        git add -A
        # --no-gpg-sign: an automated publish must never stall on a gpg passphrase
        # prompt (the mirror sets commit.gpgsign=true but its history is unsigned).
        git commit -q --no-gpg-sign -m "wiki sync from unheaded/wiki @ $(cd "$REPO_ROOT" && git rev-parse --short HEAD)"
        git push origin HEAD
        echo "  pushed to $(git remote get-url origin)"
      fi
    )
  fi
else
  echo
  echo "Mirror updated locally. Review with:  (cd $DEST && git status && git diff)"
  echo "Publish with:  scripts/sync-wiki.sh --push   (add --prune to drop mirror-only pages)"
fi

echo
echo "== done =="
