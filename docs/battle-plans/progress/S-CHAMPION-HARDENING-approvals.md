<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-CHAMPION-HARDENING E1: runbook approval packet

Prepared 2026-10-27 (Steps 132-134). Two questions for Stevie:

1. Is the authorship policy acceptable as written in `docs/adr/ADR-024-zhen-runbook-automation.md`, section "Authorship and approval"?
2. Which runbooks below do you approve now? Approving sets `status: approved`, `approved_by: stevie`, `approved_at: <date>` and regenerates `runbooks/MANIFEST.sha256` (coordinator does the edit, Steps 137-141).

No recommendation on question 2: approval is the operator's signature on runbooks that may run with sudo on west or east.

## The eleven drafts

Every one dry-runs to `[SUCCESS] Runbook completed.` (`scripts/run-runbook.py --dry-run`, 2026-10-27) and has its own test under `tests/runbooks/` (101 passed).

| runbook | description | risk | sudo | dry-run test |
|---|---|---|---|---|
| `runbooks/data/well-restore-from-east-offsite.yaml` | Restore the Well from a verified offsite backup set on east | high | True | `tests/runbooks/test_well_restore_from_east.py` |
| `runbooks/deploy/branch-promotion-develop-staging-main.yaml` | Promote a reviewed batch from develop to staging, then staging to main | high | False | `tests/runbooks/test_branch_promotion.py` |
| `runbooks/deploy/resign-unpushed-commits.yaml` | Re-sign unpushed commits in a repository | medium | False | `tests/runbooks/test_resign_unpushed.py` |
| `runbooks/network/bgp-bfd-session-down.yaml` | Diagnose and potentially fix a down or flapping BGP session with BFD | medium | True | `tests/runbooks/test_bgp_bfd_session_down.py` |
| `runbooks/observe/chaos-game-day.yaml` | Planned, attended game day on west to inject one known fault at a time, watch the Kingdom detect and surviv... | high | True | `tests/runbooks/test_chaos_game_day.py` |
| `runbooks/pki/ca-compromise-root-rotation.yaml` | Replace a compromised Kingdom root CA, reissue what it signed, then stop trusting it | critical | True | `tests/runbooks/test_ca_root_rotation.py` |
| `runbooks/security/claude-api-key-rotation-spend-breach.yaml` | Stop Claude API spending, analyse the breach, rotate the key, resume after review | high | False | `tests/runbooks/test_claude_key_rotation.py` |
| `runbooks/security/git-history-secret-purge.yaml` | Remove secrets from git history and tags | critical | False | `tests/runbooks/test_git_history_purge.py` |
| `runbooks/security/mcp-write-exec-enablement.yaml` | Enable and disable MCP write and exec tools for a single session. | high | False | `tests/runbooks/test_mcp_enablement.py` |
| `runbooks/wotan/oom-state-recovery.yaml` | Recovery steps for Wotan after an OOM event, crash, or down service | medium | True | `tests/runbooks/test_wotan_oom_recovery.py` |
| `runbooks/wotan/pending-subscription-approval.yaml` | Approve pending Wotan subscriptions | medium | True | `tests/runbooks/test_wotan_pending_approval.py` |

## Eight legacy runbooks now refused for live Champion execution

Since Phase 4 (`9c1c4e25`) a live `runbook_execute` refuses a runbook that is not approved when its commands match the destructive-verb pattern (`pkg/zhenrag`). These predate the status field; running them by hand with `scripts/run-runbook.py` is unchanged. Approve any you want Zhen to be able to run live; it needs a status block (`status`, `authored_by`, `reviewed_by`, `approved_by`, `approved_at`, `dry_run_test`) and a dry-run test, which the coordinator would add first.

| runbook | description | risk | sudo | dry-run test |
|---|---|---|---|---|
| `runbooks/infra/apt-repo-server.yaml` | Set up local APT repository for Kingdom package distribution | medium | True | `-` |
| `runbooks/infra/kernel-upgrade.yaml` | Upgrade Linux kernel with eBPF compatibility verification | high | True | `-` |
| `runbooks/network/dns-pihole-lxd.yaml` | Pi-hole LXD migration — Docker to LXD container (ADR-022) | high | True | `-` |
| `runbooks/pki/generate-kingdom-certs.yaml` | Generate CA + per-service mTLS certificates for the Kingdom | high | True | `-` |
| `runbooks/pki/rotate-service-cert.yaml` | Rotate a single service's mTLS certificate (zero downtime) | medium | True | `-` |
| `runbooks/security/secrets-rotation.yaml` | Rotate API keys, database passwords, and service tokens | high | True | `-` |
| `runbooks/security/security-audit.yaml` | CIS benchmark audit and Linux hardening verification | low | True | `-` |
| `runbooks/security/ssh-hardening.yaml` | SSH hardening — key rotation, config lockdown, fail2ban | high | True | `-` |

## E2: the first unattended night (ADR-106 Q2)

Prepared 2026-10-27 (Phase 13). Nothing is enabled. What exists:
`deploy/systemd/zhen-agentd-overnight.{service,timer}` (01:00, `-worker -overnight`, window 01:00-07:00,
max 5 cards, `-claude fake`), the kill switch `runbooks/observe/zhen-overnight-abort.yaml` (draft),
`scripts/zhen-morning-report.sh`. The worker's overnight tests pass (20).

Questions:
1. Which night?
2. Claude leg: stay `fake` (no spend, changes nothing) or `live` with which daily cap (default $5; overnight's own ledger defaults to $3)?
3. Which cards: today the worker takes Kanban cards owned by `zhen` with a ```zhen block. Any further filter?
4. Well-recorded (needs the E-A batch 030-041 applied first, and `WELL_APP_ZHEN_DSN`, `WELL_KANBAN_DSN`) or memory-only (branches still land; runs are not recorded)?
5. The shared GPU lease path (`GPU_LOCK`) for zhen-agentd and the operator's `switch-model.sh`: `/run/unheaded/gpu.lock` with a shared group?

Why yours: it spends money and writes branches while nobody watches.

## E3: ADR-019 Level 2 amendment (branch-sandbox writes)

Filed by Phase 5 in `docs/adr/ADR-019-*.md` (status Proposed, with what accepting and rejecting each change).
Note: branch-sandbox mode is already live with no off switch; rejecting it stops routed tasks with a check from
applying drafts unattended.
