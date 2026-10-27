# ADR-028: Zhenai Scheduler & Heartbeat — From Reactive to Proactive

**Status:** Superseded by zhen-agentd -scheduler (S-CHAMPION-HARDENING Step 160, 2026-10-27). Was Accepted (implemented 2026-04-04: `raft/zhen_scheduler.py`, heartbeat + schedules); that file is deleted.
**Date:** 2026-04-03

## Superseded (2026-10-27)

`raft/zhen_scheduler.py` ran by hand, with no unit, user, Well record or
alert, and the web UI stopped it with `pkill -f`. S-CHAMPION-HARDENING
Step 160 folded the scheduler into `cmd/zhen-agentd` (`-scheduler`), which
already had the unit, the nologin user, the Well role and the Champion's
audit path. What changed from this ADR:

- Schedule: `configs/zhen-schedules.yaml` (installed as
  `/etc/unheaded/zhen/schedules.yaml`) replaces `raft/schedule.json`; cadences
  are `<N>m|<N>h`, `daily@HH:MM`, `weekly@<day>@HH:MM`. Only approved
  runbooks (or ones with no status) may be scheduled (ADR-024).
- Execution: `runbook_execute` through the Champion gate (manifest hash,
  destructive scan, audit row); a non-zero exit, including a failed
  `verification:` section, is a failed run.
- Record: one Kanban card (type `scheduled-run`) and one `zhen_agent_runs`
  row (mode `scheduled`, migration 037) per run, instead of nothing.
- Heartbeat: a 60 s tick sets `zhen_scheduler_heartbeat_timestamp_seconds`
  and publishes `zhen.scheduler.heartbeat`; `ZhenSchedulerStalled` alerts
  when it stops. The service/disk/swap pulse below is not carried over:
  Akira, the Watchdog and the host rules cover it.
- Chat commands call `POST /api/v1/scheduler/{add,remove,pause,list}` (the
  mutating tool `schedule_update`); `emergency stop` pauses the scheduler
  instead of killing processes.

See `wiki/Zhen-AI.md` "Scheduler". The rest of this record is the original
decision, kept for history.
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
