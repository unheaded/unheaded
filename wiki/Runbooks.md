# Runbooks

Operational procedures. `runbooks/` holds YAML runbooks that Zhen's Champion can execute and author ([[Zhen AI|Zhen-AI]], ADR-024); `docs/runbooks/` holds prose incident runbooks.

## runbooks/ (YAML, Champion-executable)

| Area | Runbooks |
|------|----------|
| `data/` | postgresql-backup-restore, postgresql-migration, conversation-memory-maintenance, corpus-rebuild, faiss-index-status, raft-training-pipeline, zhen-index-rebuild, well-pg18-upgrade, well-restore-from-east-offsite |
| `deploy/` | full-stack-deploy, canary-deploy, rolling-upgrade, emergency-rollback, start-east-services, branch-promotion-develop-staging-main, resign-unpushed-commits |
| `infra/` | west-startup-order, host-bootstrap, service-deploy, systemd-service-debug, docker-compose-stack, lxd-container-lifecycle, kernel-upgrade, package-management, storage-health, ntp-time-sync, user-audit, akira-health-daemon, apt-repo-server, jenkins-server, netbox-setup, kanban-git-link, east-nat-internet |
| `network/` | west-east-network, wireguard-overlay, firewall-rules, dns-pihole-lxd, network-diagnostics, vpn-status, bpf-flow-graph, air-gap-egress-validation, bgp-bfd-session-down |
| `observe/` | service-health-sweep, dashboard-restart, prometheus-scrape-check, slo-error-budget-burn, gpu-monitoring, swap-monitoring, log-rotation, load-test, compliance-well-fallback, chaos-game-day |
| `pki/` | generate-kingdom-certs, rotate-service-cert, akira-cert-expiry-check, ca-compromise-root-rotation |
| `security/` | incident-response, secrets-rotation, tls-cert-rotation, ssh-hardening, firewall-audit, api-key-audit, container-security-scan, security-audit, compliance-evidence, compliance-profile-switch, claude-api-key-rotation-spend-breach, git-history-secret-purge, lich-mcp-operate, mcp-write-exec-enablement |
| `wotan/` | cluster-setup, cluster-failover, active-active-cutover, oom-state-recovery, pending-subscription-approval |
| `doom/` | doom-rebuild, doom-ring-setup, doom-debug, doom-performance |

79 runbooks. Lifecycle (`metadata.status` draft → approved, only Stevie approves), the checker `scripts/check-runbooks.sh` and its shrink-only baseline `runbooks/.check-baseline`: ADR-024 §6. The eleven S-TWO-BRAINS drafts each have a dry-run test in `tests/runbooks/`.

Plans in prose: `runbooks/security/ir-plan-v1-2026-05-06.md` (incident response), `runbooks/security/vuln-management-2026-05-06.md` (vulnerability management).

The Well's runbooks (backup-restore, migration, secrets-rotation) follow the migration ledger (ADR-101): no `DROP DATABASE`, no passwords in SQL. Backup locations are in [[Well Backups|Well-Backups]].

## docs/runbooks/ (incident prose)

| Runbook | Trigger |
|---------|---------|
| `dns-failure.md` | DNS resolution failure (Greaves) |
| `ebpf-collector-failure.md` | eBPF collector down |
| `high-latency.md` | P99 > 2 s |
| `pleg-stall.md` | Node NotReady / PLEG stall (k8s) |
| `vault-seal.md` | Vault sealed |

---

> **Source:** [runbooks/](../runbooks/) · [docs/runbooks/](../docs/runbooks/)
