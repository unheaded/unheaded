# Skill Updates

These are updates to the Unheaded team skills. Apply these to keep skills in sync with project reality.

## ⚠️ CRITICAL: Timeline Update

**`timeline-UPDATED.md`** - This is the UPDATED timeline that reflects today's session progress!

**How to apply:**
1. Copy `timeline-UPDATED.md` to replace `~/.skills/skills/unheaded-timeguru/references/timeline.md`
2. Or re-export/reinstall the unheaded-timeguru.skill with this content

**Why this matters:** The installed skill's timeline was stale. This session shipped:
- Kanban Frontend (40% → 75%)
- Busboy Go Client Library
- Mock Client for Testing
- CI/CD Pipelines

## Updates Summary

| Skill | File | Description |
|-------|------|-------------|
| **unheaded-timeguru** | **timeline-UPDATED.md** | **CRITICAL - Full timeline with session progress** |
| unheaded-developer | developer-proto-patterns.md | Proto patterns for gRPC services (add to references/) |
| unheaded-messagebus | messagebus-SKILL.md | NEW skill for Busboy message bus infrastructure |

## Important Clarifications

**`unheaded-busboy`** = Coordinator skill (office manager, helps navigate between skills)
**`unheaded-messagebus`** = NEW - Infrastructure message bus skill (the actual Busboy server code in `/busboy/`)

These are DIFFERENT things! The coordinator skill already exists. The messagebus skill is new and optional.

## Application Instructions

### For Timeline (MOST IMPORTANT)
```bash
cp skill-updates/timeline-UPDATED.md ~/.skills/skills/unheaded-timeguru/references/timeline.md
```

### For Developer Proto Patterns
Add `developer-proto-patterns.md` content to:
`~/.skills/skills/unheaded-developer/references/proto-patterns.md`

### For NEW messagebus skill
```bash
mkdir -p ~/.skills/skills/unheaded-messagebus/
cp skill-updates/messagebus-SKILL.md ~/.skills/skills/unheaded-messagebus/SKILL.md
```

## Session Lesson Learned

**ALWAYS invoke TimeGuru at session start!**

The TimeGuru skill tells you to read `references/timeline.md` FIRST. I didn't do this and ended up out of sync with project reality. Don't repeat my mistake!

## Files in This Directory

```
skill-updates/
├── README.md                    # This file
├── timeline-UPDATED.md          # CRITICAL - Updated timeline with session progress
├── developer-proto-patterns.md  # Proto/gRPC patterns for Developer skill
└── messagebus-SKILL.md          # NEW skill for message bus infrastructure
```
