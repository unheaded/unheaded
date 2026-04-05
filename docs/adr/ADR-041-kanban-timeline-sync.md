# ADR-041: Kanban ↔ Timeline Bidirectional Sync

## Status: PLANNED

## Date: 2026-04-05
## Decision Makers: Stevie Bellis (Principal), Claude (Advisor)

---

## Context

Kanban tasks live in PostgreSQL (The Well). Timeline lives in
references/timeline.md (source of truth in Git). These are disconnected —
creating a task in Kanban doesn't update the timeline, and timeline changes
don't create Kanban tasks.

The Meta Moment requires these to be synchronized: the Kanban board should
reflect the actual project timeline, and timeline updates should flow to
the board.

## Decision

### Bidirectional Sync via Wotan Events

```
timeline.md (Git)                    kanban_tasks (PostgreSQL)
      │                                       │
      ├── timeguru parses ──→ Wotan topic ──→ Kanban creates/updates tasks
      │   timeline.updates                    (matching by title or GUID)
      │                                       │
      └── Kanban publishes ←── Wotan topic ←─┘
          task.status.changed                 (writes back to timeline.md)
```

### Sync Rules

1. **Timeline → Kanban**: timeguru watches timeline.md for changes.
   On change: parse milestones/tasks → publish to Wotan `timeline.updates` →
   Kanban subscriber creates/updates matching tasks.

2. **Kanban → Timeline**: When task status changes (drag-and-drop),
   Kanban publishes to Wotan `task.status.changed` → timeguru subscriber
   updates the corresponding checkbox in timeline.md → git commit.

3. **GUID linking**: Each timeline item gets a GUID when first synced.
   Stored in both timeline.md (as HTML comment) and kanban_tasks.guid.
   Prevents duplicate creation on re-sync.

4. **Conflict resolution**: Git (timeline.md) wins. If both changed,
   timeline.md is source of truth. Kanban reflects Git state.

### Implementation

- `services/timeguru/sync.go` — timeline.md parser + Wotan publisher
- `cmd/kanban-app/timeline.go` — already has TimelineManager, extend with sync
- Wotan topics: `timeline.updates`, `task.status.changed`
- Sync interval: on file change (fsnotify) + periodic (5 min)

## Consequences

### Positive
- Single source of truth (Git) with live board reflection
- The Meta Moment: Kanban shows Unheaded building Unheaded, live
- GUID linking enables audit trail across both systems

### Negative
- Sync complexity — race conditions if both update simultaneously
- Git commits from auto-sync need careful commit messages
- timeguru must be running for sync to work
