<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-118: Pi-hole twice (Docker and LXD) behind one DNS front on west

**Status:** Proposed (pitch, parked under ADR-112). Captured during the 2026-11-03 unattended churn; nothing is built.
**Date:** 2026-11-03
**Grounded on:** west as of 2026-11-03 05:30 UTC (Pi-hole v6.5/FTL 6.8 in the LXD container at 10.10.20.252; host DNS cut over to it the same morning; nft DNAT of `wlp17s0:53` → 10.10.20.252 from April).
**Related:** ADR-116 (monitor the monitors), ADR-098 step 6 (host firewall), S-LOG-HARVEST D3/D7 (Pi-hole logs, LAN devices), the 2026-11-03 DNS outage notes in `~/tmp/next.md`, third-party policy of 2026-10-29.

## The pitch (Stevie, 2026-11-03, verbatim)

> maybe we run pihole on docker and lxd - both as backends in haproxy with dns running on 0.0.0.0:53 of west

## Context

- LAN DNS has one instance: the LXD container. When it was wrong (DNSSEC refusing everything while west's clock was 6 h behind, 2026-11-02) the LAN had no DNS, and west itself could not reach NTP to fix the clock. The loop is broken as of 2026-11-03 by listing public resolvers after Pi-hole in west's own `resolv.conf`; the LAN still has one Pi-hole.
- HAProxy already runs on west for 80/443 and has health checks, stats and the IPS rules; it is the natural "front" in Stevie's head.
- West's port 53 is not free on every address: LXD's `dnsmasq` listens on `10.10.20.1:53` (DHCP + DNS for the containers), and `wlp17s0:53` is DNAT'd to the container. `0.0.0.0:53` would collide with the first and bypass the second.

## What holds and what does not (Claude's reading)

**Two Pi-holes: yes.** A second instance on a different substrate (Docker) means a LXD or snap failure does not take LAN DNS down, and the two can be upgraded one at a time. Pi-hole v6 has no built-in sync, so the gravity lists, local DNS records and the FTL config must be kept identical from the repo (`deploy/pihole/`, Teleporter export as the gate), or the two instances answer differently.

**HAProxy as the front: only for TCP.** HAProxy balances TCP streams; it does not balance UDP, and LAN DNS is UDP first (TCP only on truncation and zone transfers). HAProxy can front DNS-over-TCP (53/tcp) and DNS-over-TLS (853) with health checks, which is worth having for the devices that use them, but it cannot be the front for the UDP path. Three native-first options for the UDP path:

| Option | Health checks | New third party | Notes |
|---|---|---|---|
| **A. nftables DNAT to two backends** (`dnat to numgen inc mod 2 map { 0 : lxd, 1 : docker }`) | none: a dead backend still gets half the queries | no | extends the existing April rule; clients retry, so a dead backend halves speed, not availability |
| **B. A small native UDP+TCP DNS front** (`cmd/dns-front`, Go stdlib: forward to the first healthy backend, probe each with a real query every few seconds, metrics on loopback) | yes | no | same shape as the decided `cmd/syslog-relay`; ~300 lines; fits ADR-116 (the front *is* the expectation: "Pi-hole answered in the last minute") |
| C. `dnsdist` | yes | **yes** | the industry answer; needs the third-party sitting |

Recommendation: **B**, with HAProxy fronting 53/tcp and 853 if those are wanted, and A as the interim if B waits on the freeze. The front binds `192.168.69.184:53`, `192.168.13.2:53` (east) and `wg0`, never `0.0.0.0`; LXD's dnsmasq keeps `10.10.20.1`.

**Keep west's own escape hatch.** Whatever fronts the LAN, west's `resolv.conf` keeps a public resolver after the front, so chrony can always fix the clock (the 2026-11-02 lesson). The same applies to east.

## Open questions for the brainstorm

1. Docker instance: on the main compose stack (one more service there) or its own compose file under `deploy/pihole/`, so it survives a main-stack outage (ADR-116's reasoning)?
2. Sync mechanism: Teleporter export committed to the repo and imported on both, or the FTL `pihole.toml` + gravity list files mounted read-only from the repo?
3. Does anything on the LAN use DNS over TCP or DoT today, or is the HAProxy leg skipped until something does?
4. Does the front count as the "LAN DNS expectation" for ADR-116, with the Pi-holes behind it as the signal sources?
5. Query log privacy with two instances: both ship to the relay (S-LOG-HARVEST D3), or only the front logs clients?

## Decision

None yet. Parked under ADR-112.

## Decisions 2026-11-03 (decision pack, round 2)

From `docs/battle-plans/DECISION-PACK-2026-11-03.md` (DP ids in parentheses).

- Idea only (Stevie: "maybe not", "just adr as idea"). Q1-Q6 are DP-208, still open.

## Decisions 2026-11-04 (decision pack, round 3)

- DP-208: own compose file under `deploy/pihole/`, Teleporter sync, no DoT leg until a device needs it, the front is ADR-116's LAN DNS expectation, both instances ship logs. Still an idea only.
