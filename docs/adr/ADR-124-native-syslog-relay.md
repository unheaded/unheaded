<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-124: Native syslog relay, `cmd/syslog-relay`

**Status:** Proposed (D2 answered native, Stevie 2026-11-03). Carried by S-LOG-HARVEST Phase 5. Amended 2026-11-09 (Stevie's answer #18): socket activation instead of `CAP_NET_BIND_SERVICE`; Vector files relayed lines by their row.
**Date:** 2026-11-04
**Related:** ADR-092 (log shipping), ADR-087 (NOC, network-device syslog planned), ADR-123 (uniform logs, parked), ADR-117 (ClickHouse is the only log store), ADR-111 (command-line shape), `config/logging/sources.yaml`.

## Context

ADR-092 says every log ships to long-term storage. Vector's `journald` source does that for whatever reaches the journal. Two kinds of source cannot reach it:

| Source | Where it logs today | What it can send |
|---|---|---|
| Pi-hole (LXD `pihole`, 10.10.20.252): `pihole.log`, `FTL.log`, `webserver.log`, its own journal | files and a journal inside the container | rsyslog `omfwd` (D3) |
| Router/AP 192.168.69.129 and other LAN devices (D7) | their own buffers | syslog, if the device has it |

Neither has a receiver: nothing on west listens for syslog. A receiver takes bytes from the network and writes them where audit records live, so everything it reads is hostile input.

## Decision

A first-party receiver: RFC 5424 and RFC 3164 over UDP and TCP in, the journal's native protocol out, so the shipping path stays the one Vector already has. `pkg/syslog` and `pkg/journald` use the standard library only. `cmd/syslog-relay` adds first-party packages (`pkg/logsources`, `pkg/cmdline`, `pkg/metrics`) and the zerolog logger every service uses; no new module.

**Listeners.** `--listen udp://10.10.20.1:514`, `--listen tcp://10.10.20.1:514`, later the LAN address (D7). There is no default listener, and a wildcard address (`0.0.0.0`, `::`) is refused.

**Socket activation (answer #18, 2026-11-09).** systemd binds 514 in `deploy/systemd/unheaded-syslog-relay.socket` and passes the sockets; the relay holds no capability (`CapabilityBoundingSet=` empty, `PrivateUsers=yes`, `SocketBindAllow=tcp:9599` only). It reads `LISTEN_PID`, `LISTEN_FDS` and `LISTEN_FDNAMES` itself (`cmd/syslog-relay/activation.go`, standard library only, no go-systemd). Each passed socket must be a UDP socket or a listening TCP socket bound to exactly one `--listen`, and each `--listen` must arrive; otherwise, or when `LISTEN_PID` names another process, the relay refuses to start rather than ignore the fds as `sd_listen_fds(3)` would. With no `LISTEN_*` set it binds `--listen` itself (tests, dev). The socket unit carries its own `IPAddressAllow`, since systemd applies the service's list only to sockets the service opens. The sockets outlive a relay restart, so datagrams queue in the kernel meanwhile. Exposure (`systemd-analyze security --offline=yes`): 1.0, was 1.3 with the capability; kev-poller is 1.2.

**Output.** `/run/systemd/journal/socket`, one datagram per message, `KEY=value\n` pairs; a value holding a newline uses the binary length form.

| Field | From | Trusted? |
|---|---|---|
| `REMOTE_ADDR` | the socket | as far as the transport is (below) |
| `UNHEADED_SOURCE` | the `sources.yaml` row for the sender, or `unknown` | yes: the relay decides it |
| `UNHEADED_SOURCE_HOST`, `UNHEADED_RETENTION` | that row's `host` and `retention`; `unknown` and `operational` with no row | yes: the relay decides them (2026-11-09) |
| `MESSAGE` | the message | no |
| `PRIORITY`, `SYSLOG_FACILITY` | PRI | no. `PRIORITY` never goes below 1: journald sends emerg entries to every login tty |
| `UNHEADED_SEVERITY_CLAIMED` | PRI, only when the relay raised `PRIORITY` | no |
| `SYSLOG_IDENTIFIER` | APP-NAME or the 3164 tag | no |
| `REMOTE_HOSTNAME_CLAIMED` | HOSTNAME | no; the name says so |
| `SYSLOG_TIMESTAMP`, `SYSLOG_PID`, `SYSLOG_MSGID`, `SYSLOG_STRUCTURED_DATA` | the header, when present | no. Kept so a queued replay keeps the sender's time and a 5424 message keeps its structured data. |
| `UNHEADED_TRUNCATED=1` | the relay | yes |
| `UNHEADED_MALFORMED=<kind>` | the relay, when the header would not parse | yes |

The relay never writes a field starting with `_`; those are journald's own, and `_SYSTEMD_UNIT`, `_UID` and `_PID` on every relayed entry name the relay.

**Hostile input.**

- `--max-msg`, default 8192 (RFC 5424 §6.1 asks that at least 2048 be accepted), caps the message. Anything longer is truncated and flagged.
- TCP frames are capped at twice `--max-msg`. A frame between the two is truncated and flagged, because closing on it would make rsyslog's `omfwd` resend the same message forever: its header sits on top of an 8 KiB `maxMessageSize` line. A frame over the cap closes the connection; the length is read before any buffer is allocated.
- Output is escaped after parsing, in every field: C0, DEL and C1 controls, line and paragraph separators and Unicode format characters (bidi overrides included) become `\xHH` or `\uHHHH`, invalid UTF-8 byte by byte. A backslash is left alone, so a sender can write text that looks like an escape; it cannot write a control byte.
- Per-sender token bucket, default 200 msg/s, burst 1000, in front of the parser. Over the limit: dropped and counted. A row may lower it (`rate:`, `burst:`), then SIGHUP.
- TCP: `--max-conns` (default 256), `--max-conns-per-sender` (default a sixteenth of it), and a deadline per frame (`--idle-timeout`, default 5 m), so dribbling a byte a minute does not hold a slot.
- State is bounded by the allowlist. Buckets exist only for rows in `sources.yaml`; with `--accept-unknown` all unknown senders share one bucket; a metric label is only ever a row id, never a claimed name or an unknown address.

**Malformed is not dropped.** A line whose header will not parse (`<192>`, a cut 5424 header) from a known sender is shipped as text with `UNHEADED_MALFORMED` set, at user.notice, and counted. A log line that fails to parse is still evidence (ADR-123 takes the same line).

**Allowlist.** A sender is a `kind: syslog-device` row whose `match` is `<ip>` or `<ip> <tag>`. A row with `status: excluded:` is not a sender: 192.168.69.132 is excluded on purpose, so it is dropped like a stranger. Unknown senders are counted (`syslog_relay_unknown_sender_total`) and dropped; a TCP connection from one is closed at accept. `--accept-unknown` is for the inventory phase only. The allowlist is checked per message, so a SIGHUP that removes a row also stops an open connection. A sender can only claim tags on its own address.

**Metrics.** `--metrics`, repeatable, default `127.0.0.1:9599`: `/metrics`, `/health`, `/ready`. `syslog_relay_received_total{transport}`, `syslog_relay_dropped_total{reason}`, `syslog_relay_unknown_sender_total`, `syslog_relay_malformed_total`, `syslog_relay_truncated_total`, `syslog_relay_reload_failures_total`, `syslog_relay_tcp_connections`, `syslog_relay_source_last_seen_timestamp_seconds{source}`. Prometheus runs in a container and cannot reach the host's loopback, so the unit also binds the overlay address, as `zhen-agentd` does. A metrics listener that cannot bind is logged and retried and does not stop the relay: logs matter more than the graph of them, and `up == 0` reports it.

**Identity.** Own nologin account `unheaded-syslog-relay`, uid 16739, `pkg/uids` and `deploy/sysusers.d/unheaded.conf`; house systemd hardening.

**Package.** The `unheaded-syslog-relay` .deb (`scripts/lib/deb-packages.sh`) ships the binary, the service and its socket, the account's sysusers line and `/etc/unheaded/log-sources.yaml` as a conffile. Like every package it enables nothing; removal stops the socket before the service.

## Alternatives

- **rsyslog `imudp` on west.** Already installed, but it widens use of a third-party tool while its verdict is on hold (2026-10-29), and its rate limiting, allowlist and field mapping would be configuration we cannot test the way we test code.
- **`systemd-journal-remote`.** A new package, so a third-party freeze ask, and it takes the journal export format, not syslog: devices cannot speak it.

## Limits

- **UDP source addresses can be forged** by anything on the same segment. `REMOTE_ADDR` on a UDP message is a strong hint, not proof; TCP's handshake is better, which is why Pi-hole sends over TCP. There is no TLS: rsyslog in the container and most devices would need certificates, and both legs stay on one host or one LAN.
- **`--max-conns` is global; `--max-conns-per-sender` is not.** One sender holds at most a sixteenth of the slots by default (`reason="sender_conns"` past that). Several compromised senders together can still fill the table until the idle deadline. Accepted: the allowlist holds a handful of rows.
- **LF-framed TCP cannot carry a newline.** A sender that lets someone else's text reach its LF-framed output lets them start a new line, under that sender's name. Octet counting avoids this; D3's `omfwd` config uses it.
- **Kernel socket drops are not counted.** A UDP flood past the receive buffer never reaches the relay. journald's own rate limit (per service, so per relay) can also suppress lines, and it says so only in the journal.
- **A journal stall drops rather than blocks.** A write that cannot finish within a second is dropped and counted (`reason="journal"`), so the UDP read loop keeps moving.

## Consequences

Relayed entries reach `config/vector/vector.yaml` like any other. Three things in `journal_rows` and `journal_volume` read differently for them. As written on 2026-11-04 they were left to Phase 6; Stevie's answer #18 of 2026-11-09 settled them, and all three are built (below each).

1. **`unit` is the relay.** `_SYSTEMD_UNIT` is `unheaded-syslog-relay.service` for every relayed line, so the `unit` column will not hold `pihole-dns` as Step 160 expects. Vector should take `unit` from `UNHEADED_SOURCE` or `SYSLOG_IDENTIFIER` for this one unit. Taking the identifier means a sender names its own unit, so `UNHEADED_SOURCE` is the safer choice.
   *Built:* for an entry whose `_SYSTEMD_UNIT` is the relay and whose `_TRANSPORT` is `journal` (journald's own fields, which no other process can set), `unit` is `UNHEADED_SOURCE`, so rows read `pihole/dns`. The relay's own log lines arrive on stdout and keep its unit.
2. **`retention` follows a claimed facility.** `audit = facility == 4 || facility == 10`. A sender picks its PRI, so it can file a line under either class. The row's own `retention` in `sources.yaml` should decide for relayed lines.
   *Built:* the relay stamps the row's class as `UNHEADED_RETENTION`, and Vector takes it for relayed lines; the claimed facility stays in `fields`. A missing or unknown stamp is `audit` (never shorten evidence on a bad field).
3. **Auth-failure counting matches text.** `journal_volume` matches `^Failed password for` on every row, so a relayed line counts towards `AuthFailureBurst` on west. A device's failed logins are worth counting, but they should not read as west's, and an allowlisted sender can raise the alert at will. The SSH login events are already safe: they read the trusted fields, not `SYSLOG_IDENTIFIER`.
   *Built:* a relayed line's `host` is the row's `host` (`UNHEADED_SOURCE_HOST`: `pihole`, `device:<ip>`), so its auth failures count under `host="device:..."`, `unit="<row id>"`, and `AuthFailureBurst` (`sum by (host, kind)`) never adds them to west's. The receiving host is kept as `fields.relay_host`. Tests: `config/vector/tests/relay.yaml`.

**ADR-123.** No conflict. The relay writes journal fields, not the JSON log contract, and sets no `level` or `service` key; its own stdout follows the contract. Under ADR-123's `format` key these rows are `syslog`, and `level` keeps coming from `PRIORITY`, which here is the sender's claim. If ADR-123 lands, the relay's rows need no change.
