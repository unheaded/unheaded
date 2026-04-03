# ADR-024: Zhen Runbook Automation — Champion-Executable Infrastructure Playbooks

**Status:** Draft
**Date:** 2026-04-02
**Deciders:** Architect, Developer, Captain, Micromanager
**Depends on:** ADR-019 (Zhen Champion Agent), ADR-018 (RAFT training)

## Context

Unheaded's operational surface is growing: 34 services, 23 eBPF programs, dual bare metal hosts (WEST + EAST), PostgreSQL multi-DB, WireGuard overlays, FAISS index rebuilds, model fine-tuning pipelines. Each of these has operational procedures that are currently:

1. **Scattered** — across CLAUDE.md, raft.txt, memory files, session logs, and developer heads
2. **Implicit** — experienced operators know the steps, but the knowledge isn't codified
3. **Fragile** — one missed step (e.g., `HSA_OVERRIDE_GFX_VERSION=11.0.0`) breaks the whole pipeline
4. **Human-dependent** — only humans can execute them today

The Zhen Champion (ADR-019) is designed to act, not just advise. But it needs structured, machine-readable runbooks to act on — not prose documentation.

## Decision

### 1. Runbook Format: Structured YAML with Verification Gates

Every operational procedure becomes a **runbook** — a YAML file with explicit steps, preconditions, verification gates, and rollback procedures.

```yaml
# runbooks/zhen-index-rebuild.yaml
apiVersion: runbook/v1
kind: Runbook
metadata:
  name: zhen-index-rebuild
  description: Rebuild Zhen FAISS index from corpus
  owner: zhen
  estimated_duration: 45m
  risk: low
  requires_sudo: false

preconditions:
  - check: "test -f ~/tmp/unheaded/raft/corpus/ring_all.jsonl"
    description: "Corpus file exists"
  - check: "pgrep -f zhen_app.py"
    description: "Zhen is currently running (will need restart)"
    optional: true

env:
  HSA_OVERRIDE_GFX_VERSION: "11.0.0"
  VENV: "~/.venv/zhen-rocm"

steps:
  - name: activate-venv
    action: shell
    command: "source ${VENV}/bin/activate"
    verify: "python3 -c 'import torch; assert torch.cuda.is_available()'"
    on_fail: "See ADR-024 troubleshooting: ROCm PyTorch setup"

  - name: run-embedding
    action: shell
    command: "cd ~/tmp/unheaded/raft && python3 scripts/16_embed_v2.py"
    timeout: 3600
    verify: "test -f ~/tmp/unheaded/raft/index/v2.index"
    progress:
      log: /tmp/embed.log
      pattern: '\[.*\] .*/1,762,687'

  - name: symlink-index
    action: shell
    command: |
      cd ~/tmp/unheaded/raft/index
      ln -sf v2.index active.index
      ln -sf v2_ids.json active_ids.json
    verify: "readlink ~/tmp/unheaded/raft/index/active.index | grep v2"

  - name: restart-zhen
    action: shell
    command: |
      pkill -f zhen_app.py || true
      sleep 2
      cd ~/tmp/unheaded/raft
      source ${VENV}/bin/activate
      nohup python3 zhen_app.py > /tmp/zhen.log 2>&1 &
    verify: "curl -sf http://localhost:20103/api/v1/stats | python3 -c 'import sys,json; d=json.load(sys.stdin); assert d[\"total_chunks\"] > 1700000'"
    timeout: 30

rollback:
  - name: restore-previous-index
    action: shell
    command: |
      cd ~/tmp/unheaded/raft/index
      ln -sf v1.index active.index
      ln -sf v1_ids.json active_ids.json
    when: "step.symlink-index.completed AND step.restart-zhen.failed"
```

### 2. Runbook Registry

All runbooks live in `runbooks/` at the repo root. Categories:

| Category | Examples |
|----------|----------|
| **infra** | Host bootstrap, WireGuard setup, LXD container lifecycle |
| **data** | FAISS index rebuild, corpus ingestion, RAFT training |
| **deploy** | Service deploy, rollback, canary promotion |
| **observe** | Dashboard restart, Prometheus scrape check, log rotation |
| **security** | Certificate rotation, API key regeneration, audit log export |
| **doom** | Doom ring setup/teardown, loader sequence, bridge start |

### 3. Kanban Integration — The Pipe Dream Made Real

Each runbook execution maps to a Kanban task lifecycle:

```
Kanban Column    | Runbook Phase     | Zhen Action
─────────────────┼───────────────────┼──────────────────────────
Backlog          | Triggered         | Create task from runbook
In Progress      | Executing steps   | Update task with step progress
Review           | Verification      | Run verify gates, report results
Done             | Completed         | Mark task done, record in Well
Blocked          | Failed/Rollback   | Flag for human intervention
```

**Implementation path:**

1. **Phase 1 (Now):** Runbooks are YAML files. Zhen reads them, executes steps via shell, reports to Kanban via API (`POST /api/v1/tasks`).

2. **Phase 2 (Age 3):** Kanban board gets a "Runbooks" view. Clicking a runbook creates a task with sub-tasks for each step. Progress is live-updated via Wotan `kanban.task.update` messages.

3. **Phase 3 (Age 4):** Scheduled runbooks. Zhen checks a cron-like schedule, auto-triggers runbooks, creates Kanban tasks, executes, and reports. Humans approve via Kanban review column.

### 4. Runbook Authoring from Session Logs

Key insight: **every debugging session IS a runbook draft.** When a human + Claude solve a problem (like today's ROCm GPU acceleration fix), the session contains:

- The problem diagnosis
- The steps taken
- The verification commands
- The gotchas discovered

Zhen should be able to ingest a session summary and draft a runbook YAML from it. The human reviews and approves. This creates a flywheel:

```
Problem → Session → Runbook Draft → Human Review → Approved Runbook → Zhen Executes Next Time
```

### 5. Detail Requirements for Champion Execution

Runbooks MUST be detailed enough that Zhen can execute them without human interpretation:

- **No ambiguity**: Every command is exact, with all env vars and paths explicit
- **Verification gates**: Every step has a machine-checkable success condition
- **Failure handling**: Every step specifies what to do on failure (retry, rollback, escalate)
- **Timeouts**: Every long-running step has an explicit timeout
- **Idempotency**: Running a runbook twice should be safe (guards against re-execution)
- **Context**: Each runbook documents WHY it exists, not just WHAT it does

## Consequences

### Positive
- Operational knowledge is codified and version-controlled
- Zhen can autonomously handle routine operations
- New operators (human or AI) can execute complex procedures reliably
- Kanban provides visibility into infrastructure operations
- Session-to-runbook flywheel continuously improves operational coverage

### Negative
- Runbook authoring has upfront cost per procedure
- YAML format may be too rigid for some edge cases (escape hatch: inline scripts)
- Kanban integration requires API extensions to kanban-app

### Risks
- Over-automation: some operations genuinely need human judgment
- Stale runbooks: environment changes can silently break steps
- Mitigation: runbooks have `last_verified` dates, Zhen can dry-run periodically

## Implementation Priority

1. **Phase 1 COMPLETE (2026-04-03):** 31 runbooks written across 7 categories
2. **Age 3:** Kanban API for task creation from runbooks
3. **Age 3:** Session-to-runbook extraction tooling
4. **Age 4:** Scheduled autonomous execution

## Runbook Registry (31 runbooks, 2026-04-03)

### infra/ (6 runbooks)
| Runbook | Description | Risk | cs/ Sources |
|---------|------------|------|-------------|
| service-deploy | Generic service build/test/deploy/health-check | medium | - |
| host-bootstrap | Bootstrap new bare metal host | high | systemd, cloud-init, ssh |
| systemd-service-debug | Debug failed systemd units | low | systemd, journalctl |
| kernel-upgrade | Kernel upgrade with eBPF compat check | high | kernel, grub |
| lxd-container-lifecycle | LXD create/start/stop/snapshot/migrate | medium | lxd |
| docker-compose-stack | Full Docker Compose stack lifecycle | medium | docker, docker-compose |
| east-nat-internet | EAST internet via WEST NAT | medium | iptables, ip |

### network/ (5 runbooks)
| Runbook | Description | Risk | cs/ Sources |
|---------|------------|------|-------------|
| wireguard-overlay | WireGuard IPv6 overlay (fd00:dead:beef::/48) | high | wireguard |
| firewall-rules | Kingdom default-deny firewall setup | high | iptables, nftables |
| dns-pihole-lxd | Pi-hole Docker→LXD migration (ADR-022) | high | dnsmasq, bind, lxd |
| network-diagnostics | Debug WEST↔EAST connectivity | low | tcpdump, mtr, ss |
| bpf-flow-graph | eBPF flow graph setup + cross-host | medium | ebpf, bpftrace |

### security/ (6 runbooks)
| Runbook | Description | Risk | cs/ Sources |
|---------|------------|------|-------------|
| tls-cert-rotation | TLS cert rotation for gateway+services | high | openssl, certbot, pki |
| ssh-hardening | SSH lockdown + fail2ban | high | ssh, fail2ban |
| secrets-rotation | API key + DB password rotation | high | vault |
| incident-response | Kingdom IR playbook | critical | incident-response, forensics |
| security-audit | CIS benchmark + hardening check | low | auditd, hardening-linux |
| container-security-scan | CVE scan for Docker/LXD | low | container-security |

### data/ (5 runbooks)
| Runbook | Description | Risk | cs/ Sources |
|---------|------------|------|-------------|
| zhen-index-rebuild | FAISS index rebuild with ROCm GPU | low | - |
| postgresql-backup-restore | The Well backup/restore/verify | medium | postgresql |
| postgresql-migration | Apply schema migrations safely | high | postgresql |
| raft-training-pipeline | Full RAFT: QA→train→quantize→deploy | medium | - |
| corpus-rebuild | Rebuild ring_all.jsonl from all sources | low | - |

### observe/ (4 runbooks)
| Runbook | Description | Risk | cs/ Sources |
|---------|------------|------|-------------|
| prometheus-scrape-check | Verify all /metrics endpoints | low | prometheus |
| log-rotation | Configure logrotate for Kingdom services | low | logrotate, rsyslog |
| dashboard-restart | Restart dashboard + Wotan in order | low | - |
| service-health-sweep | Health check all 34 services | low | - |

### doom/ (3 runbooks)
| Runbook | Description | Risk | cs/ Sources |
|---------|------------|------|-------------|
| doom-ring-setup | Full ring lifecycle: setup→load→inject→bridge | medium | - |
| doom-debug | Debug CPU state, screen, tearing, perf | low | ebpf, strace |
| doom-rebuild | doom.elf→doom.mbc→doom_data.bin pipeline | medium | - |

### deploy/ (1 runbook)
| Runbook | Description | Risk | cs/ Sources |
|---------|------------|------|-------------|
| full-stack-deploy | Deploy entire Kingdom to bare metal | high | ansible, github-actions |

### Knowledge Source
Commands sourced from `~/tmp/projects/cs/sheets/` — 200 cheatsheets across 32 categories (44K+ lines). Each runbook adapts generic commands to Unheaded-specific ports, paths, and service names.
