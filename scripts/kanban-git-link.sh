#!/bin/bash
# kanban-git-link.sh — Scan git log for Task: GUID trailers and link to Kanban tasks in PG.
# Run as post-commit hook or periodically via cron.
#
# Usage: scripts/kanban-git-link.sh [--since "1 week ago"]

set -e
cd "$(git rev-parse --show-toplevel)"

SINCE="${1:---since=1 month ago}"
DB_USER="${WELL_USER:-unheaded}"
DB_NAME="${WELL_DB:-unheaded}"
DB_PASS="${WELL_PASSWORD:-unheaded_dev}"
DB_HOST="${WELL_HOST:-localhost}"
DOCKER_PG="${DOCKER_PG_CONTAINER:-unheaded-postgres}"

LINKED=0
SKIPPED=0

# Extract commits with Task: trailer
git log --format="%H|%s|%aI" --all "$SINCE" | while IFS='|' read -r hash message date; do
    # Check if this commit references a task GUID
    GUID=$(git log -1 --format="%b" "$hash" | grep -oP 'Task:\s*\K[0-9a-f-]{36}' | head -1)
    [ -z "$GUID" ] && continue

    # Check if already linked
    EXISTING=$(docker exec "$DOCKER_PG" psql -U "$DB_USER" -d "$DB_NAME" -tAc \
        "SELECT commits::text FROM kanban_tasks WHERE guid::text = '$GUID'" 2>/dev/null)

    if echo "$EXISTING" | grep -q "$hash"; then
        SKIPPED=$((SKIPPED + 1))
        continue
    fi

    # Escape message for JSON
    SAFE_MSG=$(echo "$message" | sed 's/"/\\"/g' | head -c 200)

    # Append commit to task's commits array
    docker exec "$DOCKER_PG" psql -U "$DB_USER" -d "$DB_NAME" -c \
        "UPDATE kanban_tasks SET commits = COALESCE(commits, '[]'::jsonb) ||
         jsonb_build_array(jsonb_build_object('hash', '$hash', 'message', '$SAFE_MSG', 'date', '$date'))
         WHERE guid::text = '$GUID'" 2>/dev/null

    LINKED=$((LINKED + 1))
    echo "  Linked: $hash -> $GUID ($SAFE_MSG)"
done

echo "Done. Linked: $LINKED, Skipped (already linked): $SKIPPED"
