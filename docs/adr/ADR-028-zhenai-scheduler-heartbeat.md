# ADR-028: Zhenai Scheduler & Heartbeat — From Reactive to Proactive

**Status:** Planned
**Date:** 2026-04-03
**Deciders:** Captain, Architect, Micromanager
**Inspired by:** OpenClaw heartbeat/cron system (competitive analysis)

## Context

Zhenai currently only acts when asked. The user must open the web UI or send a command. Real NOC/sysadmin teams are proactive — they run scheduled health checks, morning briefings, overnight monitoring, and alert on anomalies without being asked.

OpenClaw's competitive advantage is exactly this: cron jobs ("every morning at 7AM, brief me") and heartbeat ("check for urgent emails every 30 minutes"). We need equivalent capability.

## Decision

### Cron-Based Runbook Scheduling

Zhenai can schedule runbooks to run automatically at specified times:

```
schedule service-health-sweep every 30m
schedule log-rotation daily at 3am
schedule postgresql-backup-restore weekly on sunday at 2am
schedule security-audit monthly on 1st at midnight
unschedule service-health-sweep
list schedules
```

Implementation: systemd timers or Python `schedule` library, writing results to The Well.

### Heartbeat Monitor

A lightweight background loop that runs every N minutes:
- Check service health (all 34 services)
- Check disk space (alert if >85%)
- Check memory (alert if swap >50%)
- Check for failed systemd units
- If anomaly detected → log to The Well + send alert

NOT a full runbook execution — just a quick health pulse. Full runbooks triggered only when anomalies detected.

### Alert Channels

When heartbeat or cron detects an issue:
1. Log to The Well (zhen_actions table)
2. Write to `/tmp/zhenai-alerts.log`
3. Future: push notification to Kanban mobile app (ADR-025)
4. Future: email/webhook notification

### Emergency Stop

New command: `stop all` or `emergency stop`
- Kills all running runbooks
- Cancels all scheduled jobs
- Logs the emergency stop to The Well
- Returns control to manual-only mode

## Implementation

### Phase 1: Chat commands for scheduling
```
schedule <runbook> every <interval>
schedule <runbook> daily at <time>
unschedule <runbook>
list schedules
```

### Phase 2: Background scheduler daemon
- Python process alongside zhen_app.py
- Reads schedule from The Well or config file
- Executes runbooks via run-runbook.py
- Logs results to The Well

### Phase 3: Heartbeat monitor
- Lightweight health pulse every 5 minutes
- Alerts on anomalies
- Configurable thresholds

## Consequences

### Positive
- Zhenai becomes proactive — detects issues before the human notices
- Scheduled maintenance happens reliably (backups, log rotation, health checks)
- Emergency stop provides safety valve

### Negative
- Background processes consume resources (mitigate: heartbeat is lightweight)
- Scheduled runbooks could conflict with manual operations (mitigate: lock mechanism)
