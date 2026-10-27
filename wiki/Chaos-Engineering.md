# Chaos Engineering: Yaldabaoth

Yaldabaoth (`services/yaldabaoth`) is the chaos engine; `chaos-controller` (`cmd/chaos-controller`, port 19012) is the one process that runs it. Every fault is an `Experiment` that passes a blast-radius allowlist, writes its revert recipe to a journal, is checked against the Kingdom SLOs, and is reverted when it ends, is aborted, times out, or the controller dies. ADR-107 (Proposed) is the envelope; S-REAL-STEEL Phases 10-14 built and proved it. Game day 1 (2026-10-26) ran every injector live: [[Game Days|Game-Days]].

Before 2026-10-23 none of this touched anything: the injectors logged and returned nil, probes always succeeded, and the controller announced "injected" for rules that reached nothing. Those paths are gone (`TestNoFakeInjectorSourcesLeft` scans for them).

## The envelope (ADR-107, D-16..D-20)

| | |
|---|---|
| One engine (D-16) | `chaos-controller` builds `yaldabaoth.Service`; the legacy `/api/v1/chaos/inject` rules and GameDay stages run as experiments through it. No experiment type has a built-in injector; a type without one is refused (501 `no_injector`), never reported as a success. **Dry run is the default**: only `CHAOS_DRY_RUN=false` injects anything |
| Allowlist (D-17) | `/etc/unheaded/chaos/allowlist.yaml` (seed `configs/chaos/allowlist.yaml`, strict YAML: unknown fields are errors) is the only source of what may be broken. Targets are resolved, never named: an LXD instance through Cuirass (managed, project `unheaded`, label `chaos=allowed`), a lab netns/interface by anchored pattern (`^steel[0-9]+$`, `^steel[0-9]+-h$`; may not match a host interface), a systemd unit from the list and only on host `east`. `pihole`, `*.target`, `ssh*`, `systemd-*`, `dbus*` are refused even if listed. Limits: `max_concurrent 1`, `max_duration 10m`, `max_loss_percent 50`, `max_delay 2s`. Checked before injection and again by the injector; every refusal is audited, counted (`chaos_refusals_total{reason}`) and published on `chaos.refused` (≤ 10/min). Widening it is a host edit plus a restart, never a request field |
| Abort, three paths (D-18) | `POST /api/v1/chaos/abort` (auth); Wotan `chaos.abort` (an unparsable message still aborts); the file `/run/unheaded/chaos/ABORT`, polled every second, which needs neither Wotan nor auth and refuses new experiments while it exists (the unit's `RuntimeDirectoryPreserve=yes` keeps it across restarts). A stat error on the file counts as present |
| Revert journal (D-19) | Before an injector acts, its revert recipe goes to `CHAOS_STATE_DIR/journal.json` (0600, atomic). Reverted at the end, on abort, by TTL (duration + 30 s), at controller start (replay), and by `chaos-controller revert-all` from the unit's `ExecStopPost` (runs after a crash or SIGKILL). An entry is removed only after the injector verifies the fault is gone; otherwise `chaos.revert_failed`, a High finding, and `revert-all` exits non-zero. A damaged journal stops the controller; no state dir means nothing runs |
| Steady state (D-20) | Probes are real HTTP GETs to allowlisted instances at the address Cuirass reports (loopback, link-local, unspecified, multicast refused; no redirects, no proxy; at most 10 per experiment); their success ratio must reach `min_success_ratio` before injection and they abort only below `abort_below`. The Kingdom SLO guard (`pkg/sloguard`, shared with Sword): every `page: true` SLO's `slo:error_ratio:5m` under 14.4 × (1 − objective). A burning or missing series, or Victoria down, refuses the start; during the run it aborts; after the revert it is part of the verdict. No `VM_URL`: nothing starts |

## Experiment types

| Type | Injector | Target kind | Host |
|---|---|---|---|
| `network_delay`, `latency_spike` (alias), `network_loss`, `network_partition` | `tc qdisc replace dev <veth> root handle c4a0: netem ...` on the instance's **host-side veth**, resolved from Cuirass's actual state (`host_veth`), never taken from the request (D-21); or a lab `steel<N>-h`. Revert deletes the root qdisc only if its handle is `c4a0:`, verified with `tc -j qdisc show`; an interface that is gone (instance deleted) counts as reverted | `lxd`, lab iface | west |
| `container_kill` (`stop` or `kill`) | `POST $CUIRASS/api/v1/containers/{id}/chaos`; refused unless Cuirass reports `enforce: true` and `chaos: true` (no enforce loop means no self-heal). Revert = wait until Cuirass reports `running` + `healthy` within the MTTR budget (default 90 s), else High finding + `chaos.revert_failed`; since `4fd3f877` a target the rolling update deleted meanwhile closes the entry as target gone | `lxd` | west |
| `service_stop` (stop), `process_kill` (`kill --signal=SIGKILL`) | fixed argv `systemctl --no-ask-password {stop|kill|start} <unit>` as `unheaded-chaos`; authorization is a polkit rule rendered from the allowlist (`chaos-controller render-polkit`, `deploy/polkit/60-unheaded-chaos.rules.tmpl`; a test fails if they diverge). Refused unless `os.Hostname()` is `east`. Revert = `start`, verified with `is-active`; after a kill it leans on `Restart=` and starts only if still inactive after 30 s | `systemd` | east |
| `packet_chaos` | loads `yaldabaoth-ebpf` (`YALD_OBJ`, default `/usr/lib/unheaded/ebpf/yaldabaoth-ebpf`), attaches TCX egress on a lab interface matching `^steel[0-9]+-h$`, pins `CHAOS_TARGETS`/`CONFIG`/`STATS` under `/sys/fs/bpf/unheaded/chaos/<nonce>/`, builds the netem tree `c4a0:` prio / `fw` filter on skb mark `0xC4A05` / band 3 netem (D-24). Only packets whose flow label is enrolled get the Monad CHAOS flag (bit 7, octet 0x07, frozen v0x01) and the mark; nothing else is shaped. Never on a physical NIC. Lab path: `chaos-controller packet-chaos --iface steel0-h --flow-label 0x5EE1 --mode marker --loss 50 --duration 5m [--dry-run]` | lab iface | west |
| `http_error` | **refuses**: "not supported: deploy a faulty build instead" (steel-echo `STEEL_ERROR_RATE`, D-22) | | |
| `cpu_stress`, `memory_stress`, `disk_stress`, `dns_failure`, `time_skew`, `node_drain`, `resource_exhaustion`, `custom` | **refuse** "not implemented" (501). They had no injector; now they say so | | |

Request shape (`POST /api/v1/chaos/experiments`, `X-API-Key`):

```json
{"name":"rs-netem-1","type":"network_delay",
 "target":{"kind":"lxd","id":"steel-echo-v2-0"},
 "parameters":{"delay_ms":200,"jitter_ms":20},
 "schedule":{"duration":"120s"},
 "rollback":{"automatic":true}}
```

Unknown parameters are refused. The record carries `status` (`running|completed|aborted|failed`), `resolved` (what the allowlist resolved the target to), and `results{success, hypothesis_valid, reverted, steady_state_probes, guard_before, guard_after, findings, abort_reason}`.

## Measured live (west and east, 2026-10-26)

| What | Number | Where recorded |
|---|---|---|
| netem 200 ms ± 20 on one replica's host veth | `/health` RTT 0.38-0.41 s on the target, 0.2 ms on the others; qdisc gone and journal empty after 120 s | plan Step 306 |
| crash revert: `kill -9 $MainPID` with a 10 min netem in place | qdisc gone **0.12 s** after the kill (`ExecStopPost revert-all`); unit back at +6 s | Step 308 |
| same drill with `systemctl kill -s SIGKILL` | kills the whole cgroup, ExecStopPost included; the qdisc went 5.15 s later when the restarted controller replayed the journal (D-19's start path). Use `kill -9 $MainPID` for the crash drill | Step 307 |
| container kill → Cuirass restart (MTTR) | 24.4 s (Phase 11, a sync boundary away); 1.7 s (game day B) | Steps 311, 368 |
| negatives: `pihole`, a managed instance without `chaos=allowed` | both 403, `chaos_refusals_total{reason="allowlist"}` 2, zero requests reached Cuirass | Step 311 |
| `service_stop` timeguru on east 60 s | inactive at +10 s, east dashboard `degraded`; active and `/health` 200 at +80 s; journal empty | Steps 325-326 |
| `process_kill` SIGKILL on east | `Restart=` brought it back at +8 s, `NRestarts` 0→1 | Step 328 |
| least privilege on east | as `unheaded-chaos`, `systemctl stop ssh.service` and `stop unheaded-kev-poller.service` both "Interactive authentication required"; empty `CapabilityBoundingSet` | Step 324 |
| abort of a 10 min netem, HTTP path / file path | reverted in **0.12 s** / **0.88 s**; `chaos_aborts_total{path="http"}`, `{path="file"}` | game day D |
| abort of a packet experiment by the file | TCX program gone 0.42 s after `touch`; `/sys/fs/bpf/unheaded/chaos` empty; re-sent 1000 frames, CHAOS bit clear | Step 349 |
| packet chaos, `marker` + loss 50 % on flow 0x5EE1, 1000 + 1000 frames | 0x5EE1: 474 received, all CHAOS-flagged, CRC ok; 0x5EE2: 1000/1000, flag clear; netem dropped 526; `STATS packets_total=2000 targets_hit=1000`. Non-enrolled Monad bytes identical before/during; enrolled differs only at the flags octet and the CRC | Steps 346, 351 |
| packet chaos, delay 100 ms (game day E) | +104 ms on 0x5EE1 only | Step 371 |
| Kingdom `slo:error_ratio:5m` wotan / dashboard / well | 0 before and after every scenario | game day |

## Running it

**Install.** `scripts/install-chaos-west.sh` and `scripts/install-chaos-east.sh`: dry run by default, `--apply` as root. They install the root-owned binary (`/usr/local/bin/chaos-controller`), the sysusers account `unheaded-chaos` (uid 16763, `pkg/uids.Yaldabaoth`), `deploy/systemd/unheaded-chaos-controller.service` (full sandbox block, `StateDirectory=unheaded-chaos` 0700, `RuntimeDirectory=unheaded/chaos`, `ExecStopPost=... revert-all`, `Restart=on-failure`), the allowlist and the SLO policy. West adds the drop-in `west-packet-lab.conf` (`CAP_NET_ADMIN` + `CAP_BPF`); east's `east.conf` grants no capability at all and the installer writes the rendered polkit rule to `/etc/polkit-1/rules.d/60-unheaded-chaos.rules`. East has no Go: build on west, rsync the bundle, `--apply --binary=...`. Neither script writes `/etc/unheaded/chaos/chaos.env` (0640 root:unheaded-chaos, by hand): `AUTH_API_KEYS`, `CUIRASS_API_KEY`, `VM_URL`, `WOTAN_ADDR`, `CHAOS_DRY_RUN=false`. Then `systemctl daemon-reload && systemctl enable --now unheaded-chaos-controller`.

Environment: `HTTP_ADDR` (default `:19012`), `CHAOS_ALLOWLIST`, `CHAOS_STATE_DIR`, `CHAOS_ABORT_FILE`, `CHAOS_SLO_POLICY`, `CUIRASS_URL`, `CUIRASS_API_KEY`, `VM_URL`, `WOTAN_ADDR`, `WOTAN_GRPC_ADDR`, `CHAOS_DRY_RUN`, `YALD_OBJ`. Routes: `POST/GET /api/v1/chaos/experiments[/{id}]`, `POST /api/v1/chaos/abort`, legacy `POST /api/v1/chaos/inject` and `/api/v1/chaos/gameday[/status]`, `/health` (lists the registered injector types), `/ready`, `/metrics` (`chaos_experiments_total{type,result}`, `chaos_active_injections`, `chaos_aborts_total{path}`, `chaos_refusals_total{reason}`). Mutating routes are 403 unless `AUTH_ENABLED=true`.

**A window** (west): Cuirass with `CUIRASS_ENFORCE=true CUIRASS_PRUNE=true CUIRASS_CHAOS=true` (see [[Containers]]), the monitoring compose stack up so `slo:error_ratio:5m` exists, then experiments. `max_concurrent: 1` means a `container_kill` holds the slot for its revert window (up to 120 s) and the next injection is refused meanwhile (game day A2).

**Stopping it, any of:**

```bash
sudo touch /run/unheaded/chaos/ABORT          # reverts everything; refuses new experiments until removed
curl -X POST -H "X-API-Key: $CHAOS_API_KEY" http://127.0.0.1:19012/api/v1/chaos/abort
sudo systemctl stop unheaded-chaos-controller # ExecStopPost runs revert-all
# dry run again: CHAOS_DRY_RUN=true in chaos.env, restart
# Cuirass side: restart without CUIRASS_CHAOS (the chaos endpoint answers 403 outside a window)
```

Residue check: `tc qdisc show | grep -c c4a0` (0), `bpftool net show | grep -i yald` (nothing), `ls /sys/fs/bpf/unheaded/chaos/` (empty), `GET /api/v1/chaos/experiments` journal `entries: null`; on east `systemctl list-units "unheaded-*"` equals the baseline. By hand if anything remains: `tc qdisc del dev <if> root handle c4a0:`, `bpftool net detach tcx_egress dev <if>` then `rm -rf /sys/fs/bpf/unheaded/chaos/<id>`, `systemctl start <unit>` on east.

## Known gaps

- **FND-023** (ADR-098 register): `steel-echo load --discover` follows Cuirass's actual state, which follows its 30 s sync, so a killed replica keeps its share of the traffic until the next sync (availability 0.9343 in Phase 11, 0.9829 in game day B, against a 0.99 bar). Fix candidates: probe targets between refreshes, or publish health transitions on Wotan.
- **FND-024**: the monitoring compose stack that records `slo:error_ratio:5m` was started by nobody; with it down the guard refuses everything (correct, fail closed). Since 2026-10-27 `scripts/kingdom-startup.sh` starts it first (Phase 0a, before the warm-up silence) and waits up to 180 s for the series (Phase 1b). Started by hand: `sudo docker compose -f monitoring/docker-compose.yml up -d`.
- East's Wotan does not auto-approve `chaos-controller`, so `chaos.refused` events are not published from east (follow-on in the handoff).
- An allowlisted instance chooses its own address, so a compromised one could point probes at another routable host; only a status bit comes back. Follow-on: restrict probe addresses to the lxdbr0 subnet (Phase 4/10 BlackMage residual, low).
- `shield-ebpf` no longer reuses the CHAOS flag for blocked-source events, and the Anamnesis event codes moved to the spec's values (E4: `EVENT_CHAOS 0x04`, `EVENT_ANOMALY 0x08`, `6ae71c3f`). The BORN/COMPUTED/DIED codes still differ from the draft; separate escalation.

---

> **Source:** [services/yaldabaoth](../services/yaldabaoth) · [cmd/chaos-controller](../cmd/chaos-controller) · [configs/chaos/allowlist.yaml](../configs/chaos/allowlist.yaml) · [deploy/systemd/unheaded-chaos-controller.service](../deploy/systemd/unheaded-chaos-controller.service) · [deploy/polkit](../deploy/polkit) · [ebpf/yaldabaoth-ebpf](../ebpf/yaldabaoth-ebpf) · [ADR-107](../docs/adr/ADR-107-chaos-safety-envelope.md) · [S-REAL-STEEL battle plan](../docs/battle-plans/S-REAL-STEEL-battle-plan.md) (Phases 10-14, Appendix A)
