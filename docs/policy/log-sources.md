<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# Log sources: the inventory and its gate

`config/logging/sources.yaml` lists every log source the Kingdom expects to
ship and how to prove it does. It is the S-LOG-HARVEST sprint's source of
truth (ADR-092 says what must be kept; this file says where each stream is
and whether it arrives).

## One entry per source

```yaml
  - id: west/nginx-access          # host/name, unique
    host: west                     # west | east | pihole | device:<name>
    kind: file                     # journal-unit | journal-container | file | lxd-file | lxd-journal | syslog-device
    match: /var/log/nginx/access.log   # unit file name, container name, path, or sender IP
    retention: operational         # audit | operational (retention-policy-v1)
    max_silence: 6h                # how long quiet is normal; n/a for oneshots and exclusions
    canary: http-get               # logger | http-get | dns-query | "none: <reason>"
    owner: architect
    status: gap                    # shipped | gap | blocked:<D#|FND-#> | excluded:<reason>
    note: "free text"              # optional
```

- `journal-unit` matches a systemd unit file name (`foo.service`, templates as `foo@.service`); the host journal carries it.
- `journal-container` matches a compose service by its `container_name` (`unheaded-<service>`); journald driver.
- `file` is a path outside the journal; `lxd-file` / `lxd-journal` live inside an LXD container; `syslog-device` is a LAN device that sends syslog to the relay.
- `excluded:` needs a reason. Platform units (dbus, polkit, …) ride the journal source with no expectation of their own; rsyslog's `/var/log` copies of the journal are not sources.

## The gate

`./scripts/check-log-sources.sh` runs `pkg/logsources`'s tests:

| Test | Red when |
|---|---|
| `TestEveryUnitHasASource` | a `deploy/systemd/*.service` has no `journal-unit` entry |
| `TestEveryComposeServiceHasASource` | a `docker-compose.yml` service has no `journal-container` entry |
| `TestEveryHostHasASource` | a `compliance/hosts/*.yaml` host has no entry at all |
| `TestNoDuplicateIDs`, `TestRetentionClassKnown`, `TestExcludedNeedsReason` | the file itself is wrong |

CI runs it in `security.yml` next to the compose log-caps gate, and
`scripts/check-gates-can-fail.sh` provokes it (a unit file with no entry) so
a silently passing gate is caught.

## Adding a source

Add a new unit, service or host and CI names it. Add the entry with the
honest status: `gap` until a shipping path exists, `blocked:<D#>` when a
decision in the sprint plan holds it, `excluded:<why>` only with a reason.
Then the runtime coverage prober (Phase 9) turns `max_silence` and `canary`
into alerts.

## Privacy

The Pi-hole query log names every LAN client and every domain it looks up;
nginx access logs carry client IPs. Both are personal data under
`docs/policy/retention-policy-v1-2026-05-06.md`. They ship to ClickHouse on
west (loopback only, one database user) and nowhere else.
