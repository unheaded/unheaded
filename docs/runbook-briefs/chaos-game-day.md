<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: chaos-game-day (runbooks/observe/chaos-game-day.yaml)

**Scenario.** A planned, attended game day on west: inject one known fault
at a time, watch the Kingdom detect and survive it, restore it, write it up.
East is out of scope unless Stevie says so.

**Owner** blackmage. **Risk** high. **requires_sudo** true (docker/systemd).
Duration about 2h.

**Facts:**
- `cmd/chaos-controller` (Yaldabaoth), port 19012: fault rules (DROP, DELAY,
  CORRUPT, DUPLICATE) with a duration after which a rule expires;
  `POST /api/v1/chaos/inject`, `GET /api/v1/chaos/rules`,
  `DELETE /api/v1/chaos/rules/<id>` (the kill switch: delete every active
  rule), `GET /api/v1/chaos/events` (audit trail), `GET /health`.
- Akira (`cmd/akira`, port 19100) votes service health into consensus bands
  OK/WARN/ERROR/CRITICAL/PANIC (CLAUDE.md table); in compose it detects only.
- Alert path: `curl -s 127.0.0.1:19130/v1/outbox | jq` (alert-mailer, mode
  dry-run since 2026-10-20; `wiki/Alerting.md`), Alertmanager on 127.0.0.1:9093.
- Silence window for planned work: `scripts/monitoring/maintenance.sh 2h "chaos game day"`;
  end with `scripts/monitoring/maintenance.sh --end` (ADR-100).
- Steady state before anything: `runbooks/observe/service-health-sweep.yaml`
  (or its curl loop), Akira's bands, `curl -s 127.0.0.1:18000/health`.

**Shape (in this order):**
1. Announce (manual): date, scope (west only), who is watching; start the
   silence window.
2. Steady state (shell, read-only): health sweep, Akira bands, alert outbox
   count; save to a dated directory (parameter `GAMEDAY_DIR` default
   `$HOME/gameday/$(date -u +%F)`).
3. Abort criteria (manual note, before the first injection): any service
   other than the one injected goes CRITICAL; the Well reports data errors;
   a restore does not bring health back within 5 minutes. On abort: kill
   switch (delete all chaos rules), run every restore step, end the silence.
4. Injections, ONE at a time, each an `action: manual` step that names its
   hypothesis and is IMMEDIATELY followed by its restore step (manual), then an
   observation step (shell, read-only):
   - stop Wotan (`docker stop unheaded-wotan`): hypothesis: clients retry
     and rejoin, logagg keeps forwarding after Wotan returns, nothing is
     lost except buffered messages. Restore: `docker start unheaded-wotan`.
   - stop the Well (`docker stop unheaded-postgres`): hypothesis: kanban,
     architect and micromanager go read-only (503 on writes), reads of cached
     data keep working. Restore: `docker start unheaded-postgres`.
   - kill trace-collector (a systemd unit, not a container:
     `sudo systemctl kill unheaded-trace-collector`): hypothesis: the
     dashboard shows "--", not zeros. Restore:
     `sudo systemctl start unheaded-trace-collector`.
5. Write-up (manual): a dated doc per game day: each hypothesis, held or not,
   evidence, follow-up findings; end the silence window.

**Rollback:** the restore steps; the kill switch for any chaos rule.
