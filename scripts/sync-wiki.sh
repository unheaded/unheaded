#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# sync-wiki.sh — push the in-repo wiki/ folder up to the GitHub Wiki repo.
#
# The canonical source of truth lives at `wiki/` inside this repo so it
# moves in lockstep with code changes (Librarian discipline — see the
# Cross-Document Update Protocol). GitHub's actual Wiki UI reads from a
# separate `<repo>.wiki.git` repo, so this script copies + commits +
# pushes the in-repo wiki/ to that remote.
#
# Usage:
#   ./scripts/sync-wiki.sh           # sync wiki/ to github.com/unheaded/unheaded.wiki
#   ./scripts/sync-wiki.sh --dry-run # show what would change without pushing
#
# Requires: SSH key loaded for git@github.com (the wiki repo uses the
# same auth as the main repo).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WIKI_SRC="$REPO_ROOT/wiki"
WIKI_REMOTE="${UNHEADED_WIKI_REMOTE:-git@github.com:unheaded/unheaded.wiki.git}"
WIKI_CLONE="${WIKI_CLONE_DIR:-/tmp/unheaded-wiki-sync}"
DRY_RUN=0

for arg in "$@"; do
    case "$arg" in
        --dry-run|-n) DRY_RUN=1 ;;
        --help|-h)
            sed -n '3,18p' "$0"
            exit 0
            ;;
        *)
            echo "unknown arg: $arg" >&2
            exit 1
            ;;
    esac
done

if [ ! -d "$WIKI_SRC" ]; then
    echo "ERROR: source wiki dir $WIKI_SRC not found" >&2
    exit 1
fi

echo "Source:  $WIKI_SRC"
echo "Remote:  $WIKI_REMOTE"
echo "Clone:   $WIKI_CLONE"
[ "$DRY_RUN" = 1 ] && echo "Mode:    DRY RUN (no push)"
echo

# Fresh clone — the wiki repo is small enough that clone-from-scratch is
# simpler than maintaining a long-lived working copy.
rm -rf "$WIKI_CLONE"
git clone --depth 1 "$WIKI_REMOTE" "$WIKI_CLONE"

cd "$WIKI_CLONE"

# Mirror wiki/ → wiki repo root. Preserve only .git/ on the destination.
find . -mindepth 1 -maxdepth 1 -not -name '.git' -exec rm -rf {} +
cp -a "$WIKI_SRC"/. ./

# What changed?
git add -A
if git diff --cached --quiet; then
    echo "✓ Wiki already up to date — nothing to push."
    exit 0
fi

echo
echo "=== Pending changes ==="
git diff --cached --stat
echo

if [ "$DRY_RUN" = 1 ]; then
    echo "DRY RUN — not committing or pushing. Re-run without --dry-run to apply."
    exit 0
fi

MAIN_COMMIT="$(cd "$REPO_ROOT" && git rev-parse --short HEAD)"
MAIN_BRANCH="$(cd "$REPO_ROOT" && git rev-parse --abbrev-ref HEAD)"

git commit -m "sync from $MAIN_BRANCH @ $MAIN_COMMIT"
git push origin master

echo
echo "✓ Wiki synced from main-repo $MAIN_COMMIT to $WIKI_REMOTE"
