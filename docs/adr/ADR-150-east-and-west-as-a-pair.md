<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-150: East and west as a pair: what fails over, and how

**Status:** Proposed (parked; draft for Stevie's reading, decides nothing). No plan, no branch, no code, no host changed. Every choice that is Stevie's is under "To settle" with its options and one recommendation.
**Date:** 2026-11-17
**Related:** ADR-147 (decisions 1, 2, 3, 13, 14, 15, 17, 18, 21, 25 and the 2026-11-16 update), `docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md` row 6 (S-EAST-PAIR), ADR-016 and its 2026-10-15 amendment (the Well, RPO 24 h), ADR-029 (health consensus), ADR-035 and ADR-064 (Wotan redundancy), ADR-69420 (Sleipnir), ADR-100 and ADR-116 (heartbeat, monitor the monitors), ADR-101 (migration ledger), ADR-103 (issuer, service tokens), ADR-107 (chaos envelope), ADR-117 (one Grafana, VictoriaMetrics for metrics), ADR-131 (failover actuator), ADR-142 (Wotan speed, parked), ADR-143 (cloud template), ADR-149 (Zhen is a module), `docs/battle-plans/S-E1-LIVE-battle-plan.md` Evenings 10B, 11 and 15.

## Why this ADR

Row 6 of the series says S-EAST-PAIR "needs a design ADR for the pair first (what fails over, and how)" (`docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md:28`). This is that ADR, as a draft. It is written from the repository and from the seven seat reports on west, which stay off the repository (decision 25); their conclusions are used at the level of design only. A statement about code or configuration carries `path:line`. "Not established" marks what those sources do not say; the marks are collected at the end.

## What is already decided

| Decision (ADR-147) | What it fixes for the pair | Where |
|---|---|---|
| 3 | East watches west. The rule over everything: "Highly available and scalable design is the end goal with everything - build it as if we have a real production target". East needs its own way out to say west is gone | `docs/adr/ADR-147-every-service-earns-its-place.md:384` |
| 17 | East is a second home "as far as it can be": its own uplink, it watches west, it can run the once-per-environment services (Wotan, alerting, a copy of the Well) when west is gone. A real pair. Its hardware (4 cores, 7 GiB, 61 to 68 °C against 70) sets what it carries. The trace collector fix first; east rebuilt first (Evening 10B: its own Wotan back, each service on its own account with limits, the role services removed); loaded only with a temperature stop | `:389`, `:406`, `:407` |
| 18 | Two wired paths (a NIC or a switch, which Stevie adds). BGP with BFD announces real service prefixes and fails over between the paths. The idle FRR daemons go | `:390` |
| 1 | Restart policy stays `no`. So nothing in this design restarts or promotes a stateful service by itself | `:377` |
| 21 | East and west are treated as two production servers. Ten or more nodes in a public cloud before 1.0 | `:402-404` |
| 13 | No numbers for resource shares before measurement. This ADR holds none | `:399` |
| 14, 15 | A tenant is an LXD instance. Zhen is a module behind a switch, never reachable from the public side, and not part of what the platform needs to survive | `:391`, `:394-397` |
| 2 | Mail is on | `:380` |
| 2026-11-16 update | After an unplanned power-up west's stack stays down and nothing outside west notices; the alert path watching itself must be seen from east | `:420-427` |

Older words that still bind:

- **The Well's recovery point is 24 h** by nightly dumps, chosen 2026-10-15; `wal_level = replica` was left on so a stronger mechanism is one restart away (`docs/adr/ADR-016-postgres-the-well.md:85-114`). It is the only recovery number on record.
- **Wotan:** ADR-035 is Accepted as the two-host design (west primary, east standby by WAL replication, Akira-driven failover); "a third node reopens ADR-064" (`docs/adr/ADR-035-wotan-active-passive-redundancy.md:3`).
- **ADR-116's answers of 2026-11-03** (the ADR is parked, the questions are answered): the out-of-band last resort is all three of east mailing directly, a CI job that expects a daily signed report, and an outside check-in service (third party, through the sitting); alert-mailer and the heartbeat receiver become host units outside compose (`docs/adr/ADR-116-monitor-the-monitors.md:162-171`).

## Starting point

### What each host runs

| | West | East |
|---|---|---|
| Containers | The compose stack: traefik, victoria, postgres, clickhouse, vector, grafana, coredns, wotan, timeguru, akira, dashboard-backend, kanban-app, auth, logstore-writer; cuirass and sword behind profiles (`docker-compose.yml:167-1385`). The monitoring project: prometheus, alertmanager, alert-mailer (`monitoring/docker-compose.yml:63,113,166`) | None (`docs/adr/ADR-147-every-service-earns-its-place.md:53`) |
| Host units | Huginn, node_exporter, trace collector, kev-poller, chaos controller, wiki-server, vor, the backup, drill and offsite units (`wiki/Systemd-Units.md:5-22`) | Huginn (pushes to west's VictoriaMetrics), node_exporter, kev-poller, chaos controller, trace collector, and from debs wotan, timeguru, akira, dashboard-backend, kanban-app, unheaded-daemon and the three old role units, the offsite check timer (`wiki/Systemd-Units.md:26-37`); the heartbeat sender (`deploy/systemd/unheaded-heartbeat.service:2-5`) |
| Routing | FRR, AS 65001 (`routing/frr/west.conf:29`) | BIRD, AS 65002 (`routing/bird/east.conf:76-79`) |
| State of note | Every stateful service of the environment | Wotan and timeguru disabled since 2026-11-10 (`docs/battle-plans/progress/S-E1-LIVE-progress.md:15`); five services have had no bus since (`docs/adr/ADR-147-every-service-earns-its-place.md:291`) |

The repository already names the two sets: "what runs on every host" and "what runs once per environment (the containers, wiki, kev-poller)" (`deploy/systemd/unheaded-host.slice:4`, `deploy/systemd/unheaded-env.slice:4`). Everything once-per-environment runs on west today. Six of those services also exist on east as host units from the debs, so a second instance is an install that has been done before, not a new packaging.

### What links them

- One cable. WireGuard `wg0` runs on it. One eBGP session runs inside the tunnel, bound to one multihop BFD session at 300 ms times 3 (`wiki/Overlay-Network.md:3-12,20`; `routing/frr/west.conf:29-37,52-55`; `routing/bird/east.conf:44-57,76-96`).
- Each side announces one `/64` that is a blackhole on the host announcing it (`routing/frr/west.conf:18,44`; `routing/bird/east.conf:67`). If the session falls, no flow changes (`docs/adr/ADR-147-every-service-earns-its-place.md:307`).
- The repository's WireGuard and BIRD files differ from what is deployed (`:308`; `docs/battle-plans/S-WEST-GROUND-battle-plan.md:36`). S-WEST-GROUND makes west's files true and leaves east's to this plan (`docs/battle-plans/S-WEST-GROUND-battle-plan.md:61`).
- East has no way out of its own: west's uplink is wifi and east reaches DNS, time and the internet only through west (`docs/adr/ADR-147-every-service-earns-its-place.md:306`). East's ruleset is written for that shape (`deploy/nftables/east.nft:7-10`) and is not applied (`docs/battle-plans/S-WEST-GROUND-battle-plan.md:33`).
- Three flows dial the cable's IPv4 addresses and bypass the tunnel: east's heartbeat (`deploy/systemd/unheaded-heartbeat.service:26`), Huginn's push (`wiki/Systemd-Units.md:31`), the Well's offsite copy (`deploy/systemd/unheaded-well-offsite.service:35`). SSH uses the same link on purpose, so an overlay change cannot cut it (`wiki/Overlay-Network.md:12`).

### What already reaches east

| What | How | Where |
|---|---|---|
| The Well | A nightly dump set, copied by SFTP after each good backup to an account that can add files and never remove them; newest 14 kept; east checks them daily | `wiki/Well-Backups.md:7-12,28-35`; `deploy/east/sshd-well-offsite.conf:10-14` |
| The repository | Every ref mirrored to a bare repository on east every 30 minutes | `wiki/Well-Backups.md:99-101` |
| "East is alive" | East posts a heartbeat to west's alert-mailer every two minutes; ten quiet minutes is `HeartbeatMissing`, a page | `deploy/systemd/unheaded-heartbeat.timer:8`; `monitoring/prometheus/rules/heartbeat.yml:23-25`; `observability/alerting/policy.yaml:71-72` |
| ClickHouse and VictoriaMetrics sets | Not yet: local on west, "offsite with plan 6" | `wiki/Well-Backups.md:66-75` |

Nothing travels the other way. "West dying is not covered: that needs a receiver outside both hosts" (`monitoring/prometheus/rules/heartbeat.yml:16-17`).

### What has not been done

`docs/battle-plans/progress/S-E1-LIVE-progress.md` records Evening 0, a test cut of Evenings 4 and 5, and parts of Evenings 1, 13 and 16. It has no entry for a step of Evening 6 (east's filter), 10, 10B (east's debs and TLS), 11 (east as a node) or 15 (PostgreSQL 18). This ADR assumes none of those five has run.

## The two-node problem

Two hosts have no majority. When east stops hearing west, east cannot tell "west is dead" from "the links between us are cut and west is still serving". If east promotes a stateful service in the second case, two primaries accept writes and their histories diverge: split brain. The same holds for a vote: a reporter that stops reporting stops voting (`pkg/health/consensus.go:44`), so with west silent the ballot on east is east's reporters alone, which is one observer and not a consensus. ADR-035 says so itself: "Two-node cluster can't do true consensus" (`docs/adr/ADR-035-wotan-active-passive-redundancy.md:68`).

| Option | What it is | What it can prove | Cost |
|---|---|---|---|
| (a) A person promotes | East only alerts. A person confirms west is down, or stops it, then promotes by a runbook | Everything a person checks | Outage lasts until a person acts. Fits decision 1 |
| (b) A third vote | A small witness outside both hosts breaks the tie | A majority exists | A third place to run and to trust. orion is Stevie's personal server and outside Unheaded: usable only if he approves it. A third small machine in the house shares the house's power and uplink. An outside service is third party |
| (c) Fencing by paths | East promotes only when both wired paths are dark and west does not answer on its uplink address either | That west is unreachable by every path east has. Not that west is stopped: west could be up and cut off, still writing for its own local services | No new part. A real fence needs power control of the other host; whether either machine has it is not established |
| (d) Automatic for stateless, manual for stateful | What holds no state, or rebuilds it from its publishers, runs on both hosts or moves by itself. The Well moves by a person | Nothing needs proving for the stateless set; the stateful set is (a) | Smallest risk; the Well's outage is (a)'s |

**Recommendation for now: (d).** Routing between the paths, east's watching and mailing, east's own Wotan and its every-host set are automatic. The Well, and what cannot work without it, are promoted by a person from a rehearsed runbook. Reasons: decision 1 already says a stateful thing that stops stays down until a person starts it; (c) cannot rule out the case that does the damage; (b) is a choice only Stevie can make.

Three rules follow, whoever promotes:

1. **One role marker per failover unit per host** (primary or standby), written only by the promote and demote runbooks. The unit that holds a service address and the path that starts the service both read it.
2. **Fence before promote:** west's instance is confirmed stopped, or is stopped, and its address withdrawn, before east's is promoted.
3. **A returning host does not resume as primary.** PostgreSQL does not stop an old primary from starting again on its old timeline. Today nothing starts at boot on west, so this holds by accident. If the stack is ever started at boot (the question the 2026-11-16 update leaves open), the start path checks the marker and asks the other host first.

**What ten nodes change.** A quorum exists with three or more voters. The role marker is then written by a quorum instead of a person (Akira decides, Cuirass acts: ADR-131); the Well can commit synchronously to "any one of several" standbys without stalling on one loss; ADR-064's three-node Wotan reopens. The mechanisms here are the ones that stage needs: a role marker, a service address announced by the holder of the role, clients that dial the address, streaming replication. Only the member count and who writes the marker change.

## What fails over, and how

One row per once-per-environment thing. "Stays down" means: down until west returns or a person starts it on east. No row sets a recovery target; ADR-016's 24 h is the only one on record.

| Thing | State it holds | How the state reaches east | When west is gone | Loss and outage, in words | Fail-back |
|---|---|---|---|---|---|
| The Well (PostgreSQL) | The platform's three databases and roles (`wiki/The-Well.md:20-27`) | Today: nightly sets. Proposed: asynchronous streaming replication to a hot standby, and the sets stay | Read-only on east until a person promotes (manual) | With replication: the commits not yet shipped when west stopped; if replication was already broken, everything since it broke, which is why lag alerts. Without it: back to last night's set. Outage until a person promotes and the address moves | West is re-seeded from east as a standby, then a planned switch back |
| Wotan | Nothing durable: rings in memory, a sequence counter (`wiki/Service-Wotan.md:36,48`) | None, by design | East's own instance is already running for east's services (automatic) | What west's bus buffered, the same as a Wotan restart. Publishers refill it: discovery every 15 s (`pkg/discovery/discovery.go:27`), health every 30 s (`pkg/health/akira.go:37`), the critical-alert snapshot every 5 min (`cmd/alert-mailer/critical.go:42`) | West's instance starts empty |
| Alert path: Prometheus rule evaluation, Alertmanager, alert-mailer | 30 days of rule series (the SLO windows are 28 days: `wiki/Alerting.md:9,15`), silences and the notification log, the mailer's cap count and open tickets, the SMTP credential on the host | None | East's own small watcher mails "west is gone" and east's local checks (automatic, stateless). Environment-wide rule evaluation stays down, or is started on east by a person with empty history | Silences, open tickets and error-budget history stay on west's disk and return with it. On east the SLO burn alerts cannot be trusted until their windows refill; cause alerts work at once | East's promoted evaluator is stopped; west's resumes with a hole for the outage |
| VictoriaMetrics | 30 days of series for both hosts (`docker-compose.yml:316`) | Nightly snapshot sets, offsite with this plan (`wiki/Well-Backups.md:75`) | Stays down. East's Huginn has nowhere to push | The outage's samples for both hosts, unless east keeps its own (To settle 12). History returns with west or from the last set. Whether Huginn holds samples while it cannot push is not established | West's instance starts on its own data; the gap stays |
| Log store (ClickHouse today; the choice waits on the second log week, decision 5) | Operational logs 90 days, audit logs 455 days (`config/clickhouse/schema/001-logs.sql:31-32`) | Nightly sets, offsite with this plan (`wiki/Well-Backups.md:66-75`). No replica is designed before the log store is chosen | Stays down. Each host's journal is the buffer; east's shipper is prepared, not installed (`wiki/Log-Aggregation.md:56`) | None for east's lines if the outage is shorter than east's journal is deep (depth not established). West's lines not yet in a set if west's disk is lost | The shippers drain from their journal cursors |
| Issuer `unheaded-auth` and its signing key | An Ed25519 signing key and a users file, both host files outside the repository and the Well (`wiki/Service-Auth.md:14-15`). The issuer's name is in every token (`docker-compose.yml:1257`) | None today | No new human login until an issuer answers under the same name on east (manual start). Service-to-service tokens are not affected: each binary mints its own | Sessions end with their tokens, an hour at most by default (`pkg/auth/jwks.go:149`). No data is lost if east's users file is current | East's issuer is stopped; west's answers again |
| Dashboard and Kanban | None of their own: the board is in the Well, sessions are in memory (`wiki/Service-Auth.md:85`) | Installed on east by the debs | East's instances run already. The board is read-only until the Well is promoted ("No DB = read-only mode", `wiki/The-Well.md:80`) | Sessions | Nothing to do |
| Akira and health consensus | Votes that expire when their reporter goes quiet (`pkg/health/consensus.go:44`); one derived row per service in the Well | None needed | East's Akira keeps checking east. It does not decide a promotion (the two-node problem). Whether a ballot across both hosts has ever run is not established (`docs/adr/ADR-029-wotan-consensus-health-remediation.md:25-27`) | None | Nothing to do |
| timeguru | The timeline, in the Well (`wiki/The-Well.md:37`), from `references/timeline.md` in git | With the Well | Follows the Well | The Well's | The Well's |
| Wiki and vor | Wiki: pages shipped as files (`deploy/systemd/unheaded-wiki-server.service:20`). vor: sheets and an index under west's `/var/zhen` (`deploy/systemd/unheaded-vor.service:43-44`) | None | Stays down | None: both are derived. How long vor's index takes to rebuild elsewhere is not established | Nothing to do |
| The apt repo | The package tree on west (`deploy/nginx/unheaded-apt.conf:17-21`) and its signing key | None | Stays down. East runs what it has installed | East cannot install or upgrade until it is back. Packages rebuild from tags; the signing key does not rebuild | Nothing to do |
| kev-poller | The catalog and 30 days of snapshots, per host (`wiki/Service-KEV-Poller.md:11`) | None needed: it is installed on both hosts (`wiki/Systemd-Units.md:51`), though the slice files it under once per environment (`deploy/systemd/unheaded-env.slice:4`) | East's keeps polling once east has its own uplink (automatic) | None | Nothing to do |
| The backups themselves | 14 Well sets on west's other disk, 14 on east; the git mirror on east | SFTP and git push, as today | East holds the copies. No new set until a primary exists. If east is promoted the backup job must run there, and there is then one site, not two | No set has been restored on east as far as the record shows: the drill runs on west and the written restore copies a set back to west first (`wiki/Well-Backups.md:28-33,58`) | West's timers resume; east's sets taken while primary are copied to west |
| DNS and Pi-hole | Pi-hole is a house service in an LXD container on west (`deploy/pihole/60-unheaded-forward.conf:2`) | n/a | Out of scope as a service. In scope as a dependency: east must resolve names and keep time without west | n/a | n/a |
| Zhen | Its own database, backup set and units (`docs/adr/ADR-149-zhen-is-a-module-behind-one-switch.md:34`) | Not failed over. Its database rides a physical standby as bytes if it shares the Well's cluster; nothing starts it on east | Stays down. Never part of the survival set | A module's outage | Its switch on west |

Grafana, Traefik and CoreDNS hold nothing that is not provisioned from the repository; they start on east only in a deliberate promotion.

## Who watches whom

| Case | What exists | What is missing |
|---|---|---|
| East dead | West pages on `HeartbeatMissing`; the sender ran on 2026-11-10 (`docs/battle-plans/progress/S-E1-LIVE-progress.md:55`). The mailer goes back to `send` in S-SEE-AND-SURVIVE Phase 15 (`wiki/Alerting.md:98`) | Nothing |
| West's rule evaluation or Alertmanager down, mailer up | The mailer mails when Alertmanager's pipeline receipt has been silent 20 minutes (`cmd/alert-mailer/mailer.go:367-378`; `observability/alerting/policy.yaml:62-63`) | Nothing, on this branch |
| West up, its whole stack down (the unplanned power-up) | Nothing: the mailer is in the stack | East must see it |
| West dead | Nothing | East must see it and mail |
| The links cut, both alive | West's half: `HeartbeatMissing` for east | East's half. Two mails, one from each host, then mean "a link"; one mail means "a host" |
| East's watcher dead | Nothing | West watches it |
| Both dead, or the house dark | Nothing inside can say so | ADR-116's answer 1 (an outside check-in, a CI job expecting a daily report): parked there, not designed here |

**What east must have and run:**

1. **Its own uplink**, with name resolution and time that do not pass through west.
2. **A default-deny filter that knows the new interface**, applied before the uplink comes up. East's ruleset assumes one link and is not applied.
3. **alert-mailer as a host unit** with the heartbeat receiver (ADR-116's answer 2), and an SMTP credential in a host file. Stevie's hands; nothing about it enters the repository.
4. **The "quiet too long" judgement inside the mailer.** Today that judgement for a host is a Prometheus rule on west (`monitoring/prometheus/rules/heartbeat.yml:23-25`) and east has no Prometheus. The mailer already makes the same judgement for the pipeline receipt by itself (`cmd/alert-mailer/mailer.go:367-378`). The same shape for host heartbeats is a small first-party change and puts no Prometheus on east.
5. **Two beats from west.** A host beat from a systemd timer (the unit exists and is parameterised: `deploy/systemd/unheaded-heartbeat.service:26-27`), and an alert-path beat that west's mailer forwards on each pipeline receipt, so it stops when rule evaluation, Alertmanager or the mailer stops. Host beat present and alert-path beat missing: "west is up, its alert path is down". Both missing: east tries each wired path and west's uplink address and says in the mail which answered.
6. **A short list of local checks on east**, on timers, posting to east's mailer: temperature, disk, east's Wotan, the standby's lag. Every rule is evaluated on west today (`monitoring/prometheus/rules/resources.yml:79` for temperature), so with west gone nobody reads east's heat.

West watches east as now, plus a beat or a scrape for east's mailer.

Rule evaluation on east, three options: (E1) the mailer-only watcher above; (E2) the monitoring project on east as well, permanently: every rule twice, a container runtime on east, the cost to be measured; (E3) `vmalert` (third-party: needs Stevie's explicit yes; ADR-117 turned it down, `docs/adr/ADR-117-one-grafana-one-metrics-store-one-log-store.md:104`). Recommended: E1.

## The network

### Which daemon

Both third-party daemons are in use, one per host. Sleipnir, the project's own speaker, is not built: ADR-69420's Phase 1 is deferred (`docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md:5`) and no file in the tree carries its name.

| Option | For | Against |
|---|---|---|
| FRR on west, BIRD on east, as today | The session, graceful restart and BFD between the two are up and their one interoperation fault is recorded and fixed (`wiki/Overlay-Network.md:20`). Changing paths and prefixes without also changing a daemon changes one thing at a time. Two implementations agreeing tests the configuration harder than one talking to itself. BIRD is one process on the small host | Two configuration languages; `cmd/routing-health` and the runbook already carry both (`cmd/routing-health/main.go:27-30`; `runbooks/network/bgp-bfd-session-down.yaml:16`) |
| BIRD on both | One process per host, one language | BIRD 2.17 has no EVPN channel (`routing/bird/bird.conf:119`); the repository's reference fabric is written for FRR (`routing/frr/frr.conf:59-148`) |
| FRR on both | Covers that reference fabric | Several daemons per host; idle ones are how decision 18's finding arose |

**Recommended: as today for the pair**, with west's FRR cut to the daemons the design uses. `routing/frr/daemons:11` still sets `isisd=yes` and `deploy/host-settings/west.tsv:18` holds west's `/etc/frr/daemons` to that file, so the idle daemons go only when that file changes. One daemon for all nodes is a question for the ten-node stage (To settle 5).

### Which addresses move

| Kind | Moves | Announced by | Used for |
|---|---|---|---|
| Host loopback, one `/128` per host | Never | Its host, on every wired path | Router ID, WireGuard endpoint, session source, the host-scoped flows that dial the cable's IPv4 addresses today |
| Path addresses: IPv6 link-local on each wire; the cable's IPv4 `/30` stays for SSH | Never | Nobody | Next hops; the lockout guard |
| Service address, one `/128` per failover unit, from one `/64` of the existing `/48` set aside for environment services (`docs/architecture/WIREGUARD_OVERLAY.md:71-75`; which `/64` is the plan's to pick) | With the role | The host whose role marker says primary, inside the tunnel | What clients dial: the Well first |
| The same `/128` on both hosts, for a stateless service that runs on both | No | Both | Each host reaches its own; the survivor's keeps answering |
| The two per-host `/64`s announced today | n/a | Dropped | They are blackholes |

**How an address follows its service.** One systemd unit per service address adds the `/128` to a dummy interface and removes it on stop. It starts only where the role marker says primary. The daemon exports connected `/128`s inside the service `/64` and nothing else. A promotion is: write the markers, stop the unit on one host, start it on the other. No VRRP and no extra daemon.

### Where WireGuard sits

| | W1: a tunnel per path | W2: one tunnel between the loopbacks, over a routed underlay |
|---|---|---|
| Underlay | Nothing routed; each wire carries one tunnel | One eBGP session and one single-hop BFD session per wire, over IPv6 link-local, announcing only the two loopbacks |
| Overlay | A session and a BFD session per tunnel; service prefixes over both | Today's one session inside the tunnel, carrying only service `/128`s |
| A path fails | The overlay session on it drops; routes move to the other tunnel | The underlay moves the loopback route; the tunnel and the overlay session do not notice |
| At ten nodes | Tunnels times paths | Underlay is the fabric, overlay a mesh between loopbacks |

**Recommended: W2.** Paths change in the underlay, roles change in the overlay, and one layer never has to explain the other. It is also the smaller step: the overlay session is today's. To prove in the plan, not asserted here: that WireGuard sends from the loopback when its endpoint is a loopback (a preferred source on the loopback route), and that FRR and BIRD at the installed versions hold a link-local session with BFD.

If path B crosses a switch shared with the house, the sessions on it are authenticated and hop-limit checked, the filter admits BGP and BFD only from the peer on that interface, and everything else between the hosts is inside the tunnel. TCP MD5 exists in both daemons; whether the installed versions both speak TCP-AO is not established.

### What a client sees

- **A path fails:** flows between the hosts continue on the other path after BFD's detection time (about 0.9 s at today's timers, `wiki/Overlay-Network.md:20`). TCP sessions survive.
- **A role moves:** the address is withdrawn on one host and announced by the other. Open connections break, because the new holder has no state for them. Clients reconnect: the Go database pool redials; Wotan clients retry from 1 s doubling to 30 s (`wiki/Service-Wotan.md:39`); a person logs in again if the issuer moved.
- **A client on the same host as the service** reaches the address locally and does not depend on BGP.
- **A browser in the house** learns none of this: service prefixes live inside the pair. It reaches each host's front at that host's own address, and after a promotion the person opens east's.

### Proving it

Cut one path three ways, attended: pull the cable, set the interface down on one end, or drop it with the chaos controller's netem injector (an allowlist change on the host, Stevie's: `docs/adr/ADR-107-chaos-safety-envelope.md:89`). During the cut: a steady probe to a service address counts lost replies; `iperf3` across the pair shows the dip and the recovery; the daemons' logs give detection and reroute times. Session and BFD state become textfile series through Huginn, read from `vtysh` and `birdc`; no exporter is added. Then the other path; then both, which must produce east's mail. The baseline per path is the `iperf3` measurement S-WEST-GROUND hands over (`docs/battle-plans/S-WEST-GROUND-battle-plan.md:908`).

## The Well

Erda's part. Nothing here was executed; the proofs are named in "Order of work".

**As it is.** PostgreSQL 16.15 in a container on west, data checksums on, published on loopback only (`docker-compose.yml:385,405,420`). `wal_level = replica` (`db/postgresql.conf:36`), no archive. `pg_hba.conf` admits the data network and the host and has no replication line (`db/pg_hba.conf:18-28`). Roles are created by migrations through the ledger (ADR-101); passwords never enter the repository.

**Mechanism: PostgreSQL's own streaming replication**, physical, to a hot standby on east. No tool is added.

| Question | Options | Recommended, and why |
|---|---|---|
| Asynchronous or synchronous | Synchronous loses no acknowledged commit, and with one standby every write on west waits whenever east or the link is down | **Asynchronous.** East is the smaller, hotter, attended host; making west's writes wait on it lowers availability. The loss on failover is the lag at that moment, so lag is a series and an alert. Synchronous to "any one of several" belongs to the ten-node stage |
| How west keeps WAL for a standby that is away | A replication slot keeps everything, without bound, while east is off. No slot, and a long absence forces a re-seed without warning | **A slot with PostgreSQL's cap on what it may hold** (`max_slot_wal_keep_size`). Past the cap PostgreSQL invalidates the slot and east is re-seeded; the alert fires before. The cap comes from measurement. It matters more after S-WEST-GROUND: WAL lives inside the data directory, on the volume five stores will share (`docs/adr/ADR-147-every-service-earns-its-place.md:383`) |
| What runs on east | The same pinned image as west (a container runtime on east); the distribution's PostgreSQL on both hosts; logical replication | **The same image on both ends.** A physical standby must be the same major version and should be the same build: west's image is Alpine, and a standby built on another C library can sort text differently and corrupt indexes. The cluster's locale provider is not established, so "same build" removes the question. Whether east has a container runtime is not established (To settle 9) |
| Where east's copy lives | East's SSD; east's `/var` | **The SSD.** The seats found east's `/var` on a spinning disk, the shape S-WEST-GROUND moves west off |

**The link.** A dedicated login role with `REPLICATION` and a connection limit, made by a migration. One `hostssl` replication line in `pg_hba.conf`. TLS 1.3 as for every client, inside the tunnel. The Well must then listen where east can reach it: on its service address, never on the house LAN. Host connections reach the container as the bridge gateway today (`db/pg_hba.conf:9-11`); what source address the Well would see for east through a published port is not established, so the host filter, not `pg_hba.conf` alone, limits who reaches that listener.

**Settings the standby cannot lower.** A hot standby needs `max_connections`, `max_worker_processes`, `max_wal_senders`, `max_prepared_transactions` and `max_locks_per_transaction` at least as large as the primary's. Its cache may be smaller. What replay costs east is measured after S-WASTE-OUT removes the health-report commits (`docs/adr/ADR-147-every-service-earns-its-place.md:247`), with the temperature stop.

**Promotion, by hand.** Fence west (confirm it is stopped, or stop it; withdraw its address). Promote the standby with PostgreSQL's own command. Write the role markers. Start the address unit on east. Check the ledger with `well-migrate status`. Set east's backup job running. Each step has a check; the runbook is the plan's.

**Re-seeding and fail-back.** A promoted east is on a new timeline and west's old directory cannot simply rejoin. Two native ways: rewind west (`pg_rewind`, possible because checksums are on) or take a fresh base backup from east. **Recommended: always the base backup.** At the Well's size it is short, and it is one path to rehearse instead of two. Then a planned switch back: writers frozen, lag zero, promote west, move the address, re-seed east.

**What the nightly sets and the drill still cover.** Replication copies a mistake as fast as it copies anything: a dropped table, a bad migration and a wrong `UPDATE` reach the standby at once. The sets are the only protection against those, and against losing both hosts. They are logical, so they restore into another major version. They stay as they are, with one addition: the restore drill also runs on east, since "east alone can rebuild the Well" has never been shown. Point-in-time recovery (ADR-016's option B) is a separate, deferred choice; a standby is not a history.

**PostgreSQL 18 (Evening 15).** The upgrade is a dump and restore into a new volume (`docs/battle-plans/S-E1-LIVE-battle-plan.md:1824-1828`). A physical standby cannot follow a major upgrade: it is rebuilt from the new primary, and both ends change their pinned image together. So either Evening 15 runs first and the standby is built once, or the standby is built on 16 and rebuilt after. The upgrade window has no standby; the sets cover it. On 18 the replication link gets the hybrid key exchange every other client gets (`wiki/The-Well.md:71`).

**How services find the Well after a promotion.**

| Option | What changes at a promotion | Note |
|---|---|---|
| The service address, announced by BGP | Nothing in any client | Needs the server certificate on both hosts to name the service, not the host; and compose services on west to dial the address instead of the compose name (`wiki/The-Well.md:73`) |
| Both hosts in each client's connection string, asking for the writable one | Nothing in any client | The driver supports it (`go.mod:35`); `pkg/database` takes one host and sets no such attribute (`pkg/database/config.go:93-95,116`). Every non-Go client needs the same |
| A DNS name repointed | The record | The resolvers are on west; DNS would have to be paired first |
| Edit each connection string and restart | Every client, by hand | Works today; slowest, and done under stress |

**Recommended: the service address.** It is the first real prefix for decision 18's BGP to carry, and no client learns that a promotion happened.

**Zhen's database.** ADR-149 gives Zhen its own database inside the same cluster (`docs/adr/ADR-149-zhen-is-a-module-behind-one-switch.md:34`). Physical replication copies a whole cluster, so Zhen's bytes reach east with it. Nothing on east serves them. A cluster of its own for Zhen is plan 7's question.

## Wotan

**What the code does today.**

- Wotan's message buffer is memory by default; a write-ahead log is an opt-in flag (`services/wotan/cmd/wotan/main.go:528`). A restart loses what is buffered (`wiki/Service-Wotan.md:48`).
- **Cluster mode does not start.** Any `--cluster-*` flag ends in "cluster mode is not yet implemented in this binary; refusing to start" (`services/wotan/cmd/wotan/main.go:193-201`).
- The cluster package exists and is tested alone: a replication server that streams log entries by sequence and needs a write-ahead log to stream from (`services/wotan/internal/cluster/replication_server.go:33`); a client that verifies each entry's signature and halts on a rejected one (`services/wotan/internal/cluster/replication_client.go:186-213`; `wiki/Wotan-Topic-Signing.md:104`; `docs/security/severity-ladder.md:54`. FND-020, which this closed, has no row in `compliance/findings/register.yaml` on this branch); a promote and demote state machine (`services/wotan/internal/cluster/failover.go:40-41,87`) that elects nothing itself and is "driven externally" (`docs/adr/ADR-064-wotan-active-active-cluster-k8s-native.md:18`).
- **A standby cannot be seeded from a snapshot:** the transfer call answers Unimplemented, "catch up from seq 0" (`services/wotan/internal/cluster/replication_server.go:95-104`).
- **Nothing durable holds the cluster epoch** since the PostgreSQL store was withdrawn: split-brain prevention "needs an epoch that does not live in PG, to be decided when cluster mode is wired" (`docs/adr/ADR-035-wotan-active-passive-redundancy.md:91-92`).

**What "its own Wotan back on east" can mean.**

| Reading | What it is | Cost |
|---|---|---|
| A. A bus per host | Each host runs its own Wotan for its own services, always. Nothing is replicated. When either host or the link is lost, each bus keeps working | The bus is per host, not once per environment. What must cross hosts goes by other roads: metrics to VictoriaMetrics, logs to the log store, heartbeats between the mailers. A critical alert on west does not reach east's services, and Akira on one host does not see the other's announcements, since a process holds one Wotan client (`pkg/wotan-client/policy_test.go:204`) |
| B. One bus at an address that moves | East's instance runs empty. East's services use west's bus over the tunnel; when west is gone the bus's service address moves to east and clients reconnect to an empty bus | No new code in Wotan: a failover is a restart somewhere else, which the stack already has to survive. But east's every-host set then depends on the link to publish; a client holds 1,000 messages for two minutes (`pkg/wotan-client/fallback.go:22-26`). What a client holding a high sequence number does against a fresh instance is not established |
| C. ADR-035 as accepted | West primary, east standby, log replication, failover driven by Akira | Wiring that refuses to start today, the write-ahead log switched on, a durable home for the epoch designed, the snapshot written, and the two-node problem for the promotion |

**Recommended: A now.** It is the plain reading of decision 17 and what east's units already dial. In-memory state has nothing to replicate, and losing a host or the link costs each side nothing it would not lose in a restart. This departs from ADR-035's accepted text, so it is Stevie's to settle (To settle 6). B's moving address can be added later for a consumer that needs one environment-wide bus. C and ADR-064 return with a third node. Making Wotan a Redis-class store is ADR-142, parked, and is not designed here.

Late joiners under A or B rebuild from their publishers, with no log: discovery within 15 s, health within 30 s, critical alerts at the next 5-minute snapshot.

## Order of work

Stages for S-EAST-PAIR to be forged from. Each leaves the pair working and each is proven by breaking something. East is attended throughout (series rule 18).

| Stage | What | The pair afterwards | Proven by |
|---|---|---|---|
| 0 | Before this plan: plans 1, 2 and 4 closed; Stevie's hardware in place; "To settle" answered | As today | n/a |
| 1 | East rebuilt (Evening 10B, with S-WASTE-OUT's handoffs: `docs/battle-plans/progress/S-WASTE-OUT-progress.md:129-134`): the collector fix, its own Wotan, each service on its own account with limits, the role units gone, current eBPF objects, Akira's config. East's idle temperature read | East is a sound single host with a bus | Stopping east's Wotan is reported by east's services; east's trace events arrive; the temperature is on record |
| 2 | East's uplink, resolver and time; east's filter applied; the mailer on east; west's two beats; east's local checks | Either host can say the other is gone | West's monitoring stopped: east mails "alert path down". West booted without its stack: the same mail. West powered off: east mails "west gone" and names the paths it tried. A cable pulled: both hosts mail |
| 3 | The second wired path; underlay sessions and BFD per wire; the tunnel between loopbacks; host loopbacks, then one stateless service address; the three IPv4 flows moved off the cable's addresses; idle FRR daemons off; the repository's files equal to both hosts' | Two paths carrying real routes | Each path cut in turn under a probe and `iperf3`: detection time, lost probes and recovery on record. Both cut: east's mail |
| 4 | The Well's standby on east; the slot and its cap; lag series and alerts; the restore drill on east | The Well has a live copy | The standby stopped for an hour: west unaffected, the lag alert fires, it catches up. The cap passed on a scratch pair: the slot is invalidated, the re-seed works. A set restored on east |
| 5 | The Well's service address; clients dial it; the promotion and fail-back runbooks; the issuer, board and dashboard on east in the same drill | East can become the home, by hand | A planned switch east and back with writers frozen: nothing lost. An unplanned one under a small write load, west's Well killed: the rows lost are counted against the lag recorded. West re-seeded, then back |
| 6 | ClickHouse and VictoriaMetrics sets offsite on east; their drills on east; the written list of what stays down with west | Every store has a copy off west | Each set restored on east into a throwaway |
| 7 | Hand to S-PROVING-RUN: the lost-link, lost-host and power-cut conditions reuse these drills, on east under the temperature stop | | |

Stages 3 and 4 do not depend on each other; 5 needs both.

## Out of scope, and not decided here

- Any recovery point or recovery time target. ADR-016's 24 h stands until Stevie restates it.
- Every resource share (decision 13). What east can carry when promoted is measured, not written here.
- Automatic promotion of the Well; a witness; a fence by power control.
- Replication for logs: the log store is not chosen yet (decision 5).
- Wotan's cluster mode, ADR-064 and ADR-142.
- Sleipnir, EVPN-VXLAN and the fabric for more than two nodes.
- Pi-hole and the house's DNS; the house router; anything on orion.
- Tenants and Zhen: they are placed by plan 7 and are not failed over.
- A public front for either host.
- Whether west's stack starts at boot: the 2026-11-16 update's question. This ADR only states what the start path must check if it does.
- The battle plan.

## Alternatives considered

| Alternative | Why not, or when |
|---|---|
| Static routes instead of BGP | Two static routes with a tracked next hop would move traffic between two paths. Decision 18 chose BGP. Static routes also do not carry "this service is here now": a role change would be an edit on both hosts instead of one announcement, and the configuration would be thrown away at ten nodes |
| An external HA manager (keepalived, Patroni, corosync and pacemaker) | Each is third-party: needs Stevie's explicit yes. Patroni also needs a consensus store (etcd, third-party), which is the third vote by another name. A manager automates the promotion this ADR recommends leaving to a person while there are two hosts |
| VRRP for the service address | A second failure detector beside BFD, and it decides by itself who holds the address: automatic promotion by the back door |
| Synchronous replication now | Stalls west whenever east is away |
| Logical replication for the standby | Carries no DDL and no roles; every migration would run on both sides and the ledger would have two masters. Useful later for an upgrade with a short stop |
| pgBackRest or another backup tool for seeding and archive | Third-party: needs Stevie's explicit yes. `pg_basebackup` ships with PostgreSQL |
| The sets only, no live standby | Meets "a copy of the Well" as decision 17's words stand. A promotion then restores last night's set: up to a day lost, as ADR-016 accepted. Listed as To settle 7's alternative |
| East points all its services at west's Wotan and runs none | Leaves east with no bus when west or the link is gone; decision 17 says its own |
| A second full monitoring stack on east | Doubles the cost on the small host to cover one case the mailer can cover alone |
| orion as witness or outside watcher | Outside Unheaded; only on Stevie's word |

## Consequences

- East holds a live copy of every row in the Well, not only last night's set. It already holds 14 sets; its filter, accounts and disk are now part of the Well's trust boundary.
- A second alert sender exists. A cut link produces two mails. The caps are per host.
- West's writes never wait on east. A promotion can lose the last moments of commits; how many is read from the lag series, not guessed.
- Nothing stateful moves without a person. An outage of west lasts as long as it takes a person to read a mail and run a runbook.
- The repository's WireGuard, FRR and BIRD files become the source for both hosts, checked for drift on both.
- East needs a container runtime, or the Well's packaging changes on both hosts.
- The promote, demote and re-seed runbooks become things that are drilled, like the restore.
- Zero user data access is unchanged: replication, heartbeats and routes carry platform state only.
- At ten nodes the same parts are reused. What is replaced is the person.

## To settle (Stevie)

1. **Who promotes the Well.** (a) a person; (b) a third vote; (c) fencing by paths; (d) automatic for the stateless set, a person for the Well. Recommended: (d).
2. **A third vote or outside watcher.** None; orion; a third small machine in the house; an outside check-in service (third-party: needs your explicit yes). Recommended: none for promotion while there are two hosts; for alerting, your ADR-116 answer 1 stands and waits for the third-party sitting.
3. **The shape of the second path and east's uplink.** (H1) a second direct cable, and a separate port or wifi for east's uplink; (H2) a switch that both hosts and the router join, with the existing cable kept as path A. Recommended: H2. It gives west a wired uplink, east its uplink and the pair its second path in one piece of hardware, and a third node later is one more cable. Its cost: path B and the wired uplinks share the switch, so path B is treated as untrusted.
4. **A way out through the other host.** Each host announces a default route to the other at low preference, so east leaves through west when its own uplink is down, and the reverse. Yes or no. Recommended: yes, after stage 3 is proven; it is a second real route for BGP to carry.
5. **The routing daemon.** FRR on west and BIRD on east; BIRD on both; FRR on both. Recommended: as today for the pair; decide one daemon when the ten-node stage is designed.
6. **Wotan on the pair.** (A) a bus per host; (B) one bus at a moving address; (C) ADR-035's replication. Recommended: A. It departs from ADR-035's accepted text; if you choose A, ADR-035 gets a dated amendment.
7. **The Well on east.** A live standby, asynchronous; a live standby, synchronous; the nightly sets only. Recommended: asynchronous standby, sets kept. And: restate the recovery point, or leave ADR-016's 24 h as the written target with the standby as margin. Recommended: leave it until stage 5 has measured what a promotion loses.
8. **How services reach the Well after a promotion.** The service address; both hosts in every connection string; edit and restart. Recommended: the service address.
9. **What runs PostgreSQL on east.** Docker on east with the same pinned image; the distribution's PostgreSQL on both hosts; an LXD instance built to match. Recommended: the same image. Docker is not a new tool; it is a new install on east.
10. **The issuer on east.** The same signing key copied to east; or east's issuer has its own key and verifiers pick it up from the key set, which they refetch for an unknown key (`wiki/Service-Auth.md:40`). Recommended: its own key. No private key leaves its host; the cost is one login after each move.
11. **East's mail route.** A second credential for today's sender, kept on east; or a different route. Recommended: a second credential now; a different route with ADR-116's sitting.
12. **What east runs while it is alone.** The survival set of decision 17 (Wotan, the watcher, the Well once promoted) plus the issuer, board and dashboard; or also a VictoriaMetrics that scrapes east only. Recommended: the first; add VictoriaMetrics only if stage 5's measurement under the temperature stop leaves room.
13. **Rule evaluation on east.** E1 the mailer-only watcher; E2 a second monitoring project; E3 `vmalert` (third-party). Recommended: E1.
14. **Evening 15 before or after the standby.** Recommended: before, if it will run before stage 4; otherwise build on 16 and rebuild once.
15. **Does west's stack start at boot** (left open on 2026-11-16). Not answered here. If yes, the start path reads the role marker and asks the other host before the Well starts as primary.

## Not established

1. Whether a health ballot across both hosts (ADR-029 Phase 4) has ever run.
2. Whether any backup set has been restored on east.
3. Whether east has a container runtime installed.
4. The Well cluster's locale provider.
5. What source address the Well sees for a connection from east through a published port.
6. Whether Huginn holds samples while VictoriaMetrics is unreachable.
7. How deep east's journal is.
8. What a Wotan client holding a high sequence number does against a fresh instance.
9. Whether the installed FRR and BIRD both speak TCP-AO, and whether they hold a link-local session with BFD between them.
10. Whether WireGuard keeps the loopback as its source address in W2.
11. Whether either machine has power control that could serve as a fence.
12. East's idle temperature after the collector fix.
13. How long vor's index takes to rebuild on another host.
14. Whether Evenings 6, 10, 10B, 11 and 15 have run: the progress file has no entry for them.
15. Which ports are free on each host for the second path and the uplink: the plan reads them.

## Decision

None. Parked: a draft for Stevie's reading. S-EAST-PAIR is forged after he has read it and answered "To settle".
