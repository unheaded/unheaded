<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: pending-subscription-approval (runbooks/wotan/pending-subscription-approval.yaml)

**Scenario.** A service's Wotan subscription or publish stays "pending
approval": its publishes fail with `subscription pending approval`, its
messages never arrive. Wotan approves on subscribe only display names listed
in `topics.auto_approve`; every other name gets a pending member that
expires (default `--pending-approval-timeout` 1h).

**Owner** wotan. **Risk** medium. **requires_sudo** true (unit restart,
reading the journal / docker logs). Duration about 15m.

**The admin API cannot help today (known gap).** `GET /api/v1/admin/pending`
and `POST /api/v1/admin/approve` exist, but Wotan's admin key is never set
in its `main`, so every admin call answers 401 `admin authentication not
configured`; docs disagree on the variable name (`ADMIN_API_KEY` vs
`WOTAN_ADMIN_TOKEN`). The runbook must not rely on them; one read-only step
may show the 401 so the operator knows. Approve/deny would answer 404
(unknown member) or 409 (no longer pending) if they worked.

**Working path:**
1. Detect: the Wotan log line `topic_subscriber_pending_approval` (fields
   `topic`, `display_name`). Compose: `docker logs unheaded-wotan --since 1h`;
   deb/systemd: `journalctl -u unheaded-wotan --since -1h --no-pager`. Parameter
   `WOTAN_URL` default `http://127.0.0.1:18000`. The count, without names:
   `curl -s "$WOTAN_URL/metrics" | grep '^unheaded_chat_members_pending'`
   (refreshed every 15 s).
2. Decide: is the display name a known service? Display names come from the
   code (`discovery.Start(..., name, ...)` and each client's `display_name`):
   `grep -rn '"<name>"' cmd services pkg`. A name that matches no code is a
   typo or an unknown client: do NOT approve it. **An approved name can
   publish to any topic** (open finding), so approve only known service names.
3. Add the name to `topics.auto_approve` in every copy that is live:
   `configs/wotan.yaml` (compose mounts it read-only at
   `/etc/wotan/wotan.yaml`) and, on a deb install, `/etc/unheaded/wotan.yaml`
   (a conffile). Commit the repo change; never edit only the host copy.
   Validate the YAML (`python3 -c 'import yaml,sys; yaml.safe_load(open(sys.argv[1]))' <file>`).
4. Restart Wotan (manual, irreversible: true): the store is `type: memory`,
   so a restart drops every topic, member and buffered message. Compose:
   `docker compose up -d --force-recreate wotan`; deb:
   `sudo systemctl restart unheaded-wotan`.
5. Verify: `curl -sf "$WOTAN_URL/health"`; then the service's client re-asks
   on its own within 5 minutes (pkg/wotan-client `defaultPendingRecheck`),
   or restart that service. A read-only probe that the name is now
   allowlisted: subscribing as an allowlisted name returns the one approved
   member (idempotent, `EnsureApproved`):
   `curl -s -X POST "$WOTAN_URL/api/v1/topics/$TOPIC/subscribe" -H 'Content-Type: application/json' -d "{\"display_name\":\"$NAME\"}" | jq -e '.subscriber.status == "approved"'`
   (parameters `NAME`, `TOPIC`, no defaults).

**Rollback:** remove the line from `auto_approve`, restart Wotan (manual).
