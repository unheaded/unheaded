# ADR-038: Kanban Task GUID → Git Commit Audit Trail

## Status: ACCEPTED

## Date: 2026-04-05
## Decision Makers: Stevie Bellis (Principal), Claude (Advisor)

---

## Context

Every Kanban task has a GUID. Every git commit has a hash. Linking them
creates an audit trail: which commits delivered which tasks. This enables
traceability from requirement → implementation → verification.

## Decision

Add a `commits` JSONB column to `kanban_tasks`. When committing, include
the task GUID in the commit message trailer. A post-commit hook or periodic
scanner maps commit hashes back to task GUIDs in PostgreSQL.

### Commit Format

```
feat(wotan): add replication proto

Task: f543319f-c65d-484d-b074-a66cf619b66c
Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
```

### Schema

```sql
ALTER TABLE kanban_tasks ADD COLUMN IF NOT EXISTS commits JSONB DEFAULT '[]';
-- Each entry: {"hash": "abc123", "message": "feat(...)", "date": "2026-04-05T..."}
```

### Scanner

`scripts/kanban-git-link.sh` runs periodically or as post-commit hook:
1. `git log --format='%H %s' --grep='Task:' | extract GUID + hash`
2. UPDATE kanban_tasks SET commits = commits || new_entry WHERE guid = $1

### API

- `GET /api/v1/tasks/:id` — includes `commits` array in response
- Detail modal shows linked commits with hash (click to copy)

## Consequences

### Positive
- Full audit trail: task → commits → code changes
- Searchable: "which commits implemented this feature?"
- Reverse lookup: "which task does this commit belong to?"

### Negative
- Requires discipline: include `Task: <guid>` in commit messages
- Scanner needs periodic execution (hook or cron)
