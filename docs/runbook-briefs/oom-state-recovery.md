<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: oom-state-recovery (runbooks/wotan/oom-state-recovery.yaml)

**Scenario.** Wotan was OOM-killed (or crashed) and restarted, or is down.
Precedent: OOM-killed 2026-10-18 23:25 UTC; root cause (per-topic budgets let
~50 log topics hold 290 MiB of heap) fixed by `0fea41f1` (per-topic byte
budget) and `71a29277` (one budget for all topics, `--buffer-bytes-total`
64 MiB). The store is `type: memory` (`configs/wotan.yaml`), so after any
restart every topic, member and buffered message is gone.

**Owner** wotan. **Risk** medium. **requires_sudo** true. Duration about 15m.

**What recovers on its own (fixed in pkg/wotan-client and pkg/logagg):**
clients rejoin when Wotan has forgotten them (`7998692b`), gRPC streams
resume from the right seq after a restart (`8bbcaf9b`), HTTP pollers page
with `seq`/`after_seq` and never redeliver at or below their cursor
(`40ea00d7`), and a Wotan down at service start no longer switches log
forwarding off (`c3f91ba1`). Messages buffered before the kill are lost.

**Detection (read-only, first):**
- kernel: `journalctl -k --since -6h --no-pager | grep -i 'killed process'`
- compose: `docker inspect -f '{{.State.OOMKilled}} {{.State.Status}} {{.RestartCount}}' unheaded-wotan`
  (container limit 256M, `GOMEMLIMIT=200MiB` in `docker-compose.yml`)
- deb: `systemctl status unheaded-wotan --no-pager` and
  `journalctl -u unheaded-wotan --since -6h --no-pager | tail -50`
- metrics: `curl -s "$WOTAN_URL/metrics" | grep -E '^unheaded_chat_(memory_allocated_bytes|members_approved|members_pending|rooms_total)'`
- Akira's consensus band for wotan (`cmd/akira`, port 19100).
Parameter `WOTAN_URL`, default `http://127.0.0.1:18000`.

**Recovery:**
1. If down: restart (manual, irreversible: true only in the sense that
   nothing in memory survives; it already did not). Compose:
   `docker compose up -d wotan`; deb: `sudo systemctl restart unheaded-wotan`.
2. Verify `/health` answers: `curl -sf "$WOTAN_URL/health" | jq -e '.status == "healthy"'`.
3. Verify the allowlisted services re-joined: `unheaded_chat_members_approved`
   back above zero within a few minutes (a shell step that polls `/health`'s
   `total_members` for up to 120 s: `for i in $(seq 1 60); do ...; sleep 2; done`),
   and `unheaded_chat_members_pending` not piling up (a pile means a name
   missing from `auto_approve`: see `runbooks/wotan/pending-subscription-approval.yaml`).
4. Verify flow: Kanban and the dashboard log tail show new entries (manual
   look); `curl -s "$WOTAN_URL/api/v1/topics"` lists topics again.

**Prevention (notes only, not steps):** the compose `mem_limit`/`deploy.resources`
256M stays; switching `store.type` to `hybrid` (WAL + PostgreSQL) so state
survives a restart is a Stevie decision, not a runbook step.

**Rollback:** none needed; the runbook changes nothing but restarting a dead
service.
