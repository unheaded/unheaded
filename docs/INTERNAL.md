# Internal Documentation

The `docs/internal/` directory contains documentation that is not intended for public consumption. These files support internal development workflows, security assessments in progress, and session-to-session handoff notes.

## Structure

```
docs/internal/
  security/         Security assessment results with incomplete data
  sessions/         Session handoff notes and agent prompts
  battle-plans/     Sprint planning and execution documents
```

## What belongs here

- **Security assessments** that contain placeholder/template data (not yet executed)
- **Session handoffs** between development sprints
- **Battle plans** for internal sprint planning and coordination
- **Agent prompts** and orchestration instructions

## What does NOT belong here

- Published protocol specifications (keep in `docs/protocol/`)
- Architecture decisions (keep in `docs/adr/`)
- Research findings (keep in `docs/research/`)
- User-facing documentation (keep in `docs/`)

## Active Battle Plans

The currently active battle plan (`battle-plan-S73-public-launch`) is also available at `docs/battle-plans/` for convenience during execution.
