# eBPF Programs

eBPF-based packet-level observability from L2 to L7. XDP ingress, TC egress, kprobe/tracepoint instrumentation.

## Toolchain

| Layer | Tool | Role |
|-------|------|------|
| Kernel programs | **Aya** (Rust) | XDP/TC/kprobe/tracepoint eBPF programs |
| Userspace loader | **Aya** (Rust) | Program loading, map pinning, ring buffer reading |
| Go userspace | **cilium/ebpf** (Go) | BPF map operations, program attachment from Go services |
| Host library | **libbpf** | System-level BPF support (bpftool, BTF) |

## Programs

Workspace members of `ebpf/Cargo.toml` (GPL-2.0-only):

| Program | Hook | Purpose |
|---------|------|---------|
| `packet-marker` | XDP | Trace ID injection at ingress |
| `flow-tracker` | TC | Connection tracking |
| `latency-probe` | kprobe / kretprobe | TCP latency (RTT) |
| `syscall-tracer` | raw tracepoint | Syscall auditing |
| `shield-ebpf` | XDP + TC | Protocol Shield: Monad stamp/validate at the boundary |
| `hop-ebpf` | XDP | Per-hop Monad processing; validator, key and retry checks, off by default; enforce runs as observe unless built with `hop-enforce`, which only the test object is (ADR-125) |
| `yaldabaoth-ebpf` | TC egress | Chaos: corrupt, delay, duplicate or truncate registered flows |
| `monad-cpu-ebpf` | XDP | The UPC: MBC interpreter (Doom, xv6) |
| `firewall-ebpf`, `maglev-ebpf`, `canary-ebpf`, `qos-ebpf`, `failover-ebpf`, `compliance-ebpf`, `nfv-ebpf`, `version-ebpf`, `anomaly-ebpf` | XDP | Monad CPU applications (apps 03-12) |

Shared crates: `common`, `monad-common`, `pqc-common`. `trace-collector-go` loads packet-marker, flow-tracker and latency-probe on west and east ([[Systemd Units|Systemd-Units]]).

### `unheaded hop`

The CLI writes hop-ebpf's check maps, pinned under `/sys/fs/bpf/unheaded/hop-ebpf` (`--pin-dir` changes it). Both commands need CAP_BPF.

| Command | Does |
|---------|------|
| `unheaded hop rules apply --file <rules.yaml>` | Adds or replaces the validators, HMAC keys and retry entries in a rules file (`pkg/hoprules` gives the format). Without `--prune` nothing is removed. The check mode stays as it is, so rules written while it is off do nothing. |
| `... --prune` | Off by default. After the file's entries have landed, deletes what `HOP_VALIDATORS`, `HMAC_KEYS` and `RETRY_TOKENS` hold and the file does not, validators first, and prints one `deleted` line per entry. `CONFIG` (the mode) is never pruned. The flow-0 check then reads the file alone. Deleting a validator removes a check: it happens only on this flag. |
| `... --dry-run` | Runs every check, prints `would apply` with the counts and, with `--prune`, one `would delete` line per entry, and changes nothing. Run it before a first `--prune` on a host. |
| `unheaded hop mode <off\|observe\|enforce>` | Sets the check mode (CONFIG key 4). `enforce` is refused until `hoprules.EnforceBlockers` is empty (today FND-031, FND-033, FND-035), and the packaged object runs enforce as observe anyway (ADR-125). |

#### Flow 0 (FND-035)

Shield leaves both service ids at 0, so every Shield-born packet is flow 0 (and ring 1). A validator with `require_hmac` or `require_retry` therefore fails all of them unless flow 0 has a live entry, and an enforcing hop would drop them. `hop rules apply` refuses such a file before it writes anything: each `require_hmac` validator needs an `hmac_keys` entry with `flow: 0` on its namespace, and each `require_retry` validator a `retry` entry with `flow: 0`, either in the file or already in the maps and not expired. The refusal names every validator at fault. If the entry that satisfies the check has a `ttl`, the command prints a warning: nothing renews it, and when it lapses the same packets fail. `--allow-no-flow0` skips the check, for a ring that carries no Shield-born packets; the command says on stderr that it was skipped.

#### What `hop rules apply` accepts

A correctly set up host passes every check below unchanged; anything else is refused before a map is opened, with a message naming the file and the reason.

| File | Correct | Refused |
|------|---------|---------|
| Rules file (`--file`) | A regular file that its group and everyone else cannot write (`0600`, `0640`, `0644`). | Group- or world-writable (`chmod go-w` fixes it); not a regular file. |
| Key file (`key_file:`) | A regular file of exactly 32 bytes, mode `0400` or `0600`, owned by root or by the user running the command, with one hard link, named by its real absolute path. | Any other mode or size; another user's file; a second hard link; a symlink at the file or at any directory above it (name the real path instead: on Linux the open uses `openat2` with `RESOLVE_NO_SYMLINKS`). |

| Pin (`--pin-dir`/`<MAP>`), for `hop rules apply` and `hop mode` | The pin and the directory holding it are real files owned by root (or by the user running the command), closed to group and world writes, as `ebpf-loader --pin-maps` leaves them (`0755` directory, `0600` pins); and the kernel's own name for the map behind the pin is the pin's file name (`HOP_VALIDATORS`, `HMAC_KEYS`, `RETRY_TOKENS`, `CONFIG`). | A symlink at the pin or its directory; another user's pin or directory; a group- or world-writable one; a map the kernel knows under another name (a stale or wrong pin of the same sizes), with both names in the message. Directories above the pin directory are not checked. |

The key file is read and checked, and its bytes go no further: the `HMAC_KEYS` entry is written with a zero key field (2026-11-21). hop-ebpf reads an entry's presence and expiry, never the key, because v0x01 carries no tag to verify (ADR-125, FND-031), so `bpftool map dump` on the pin shows no key material. The bytes return to the map the day a tag exists.

A secret manager that hands out its files through a symlinked directory (`/run/secrets` under some of them) needs the directory's real path in `key_file`.

### `ebpf-loader` and signals

`cmd/ebpf-loader` holds the programs' link handles until it is told to stop. SIGINT, SIGTERM and SIGHUP all run the same cleanup: the handles are dropped, which detaches the programs, and the `--pid-file` is removed. Before 2026-11-21 only SIGINT did, so a unit stopped by systemd (SIGTERM) skipped the cleanup and left its PID file. SIGKILL still skips it.

### `ebpf-loader` and pins left by an earlier load

Pins outlive the loader: stopping it detaches the programs and leaves the pin files. None of these objects' maps is pinned by name, so the next load makes new maps, and a pin still there names a map no program writes. For `shield-ebpf` and `hop-ebpf` the loader refuses that before it parses, loads or attaches anything (since 2026-11-21 for Shield, where it used to log `already pinned` and go on):

- anything already in `<pin base>/shield-ebpf/` or `<pin base>/hop-ebpf/` (the pin base is `--map-pin-path`, or `/sys/fs/bpf/unheaded` with `--pin-maps`);
- a `<pin base>/trace_events` link that points anywhere but `<pin base>/shield-ebpf/ANAMNESIS`;
- a map that cannot be pinned, or a link that cannot be made, after the load.

The message names the pin. The loader never removes pins itself, because another loader may be running on them. Before a start, and in any deploy step that carries this loader to a host for the first time: check that no loader is running, then `rm -r` the program's pin directory (`ls /sys/fs/bpf/unheaded/shield-ebpf` shows whether there is anything to remove). No unit runs `ebpf-loader` today; it is a tool.

## Not tracing ourselves

Two loops of the tracer watching its own output, both cut:

- **Dashboard viewers** (2026-10-18): each WebSocket frame to a browser is a packet the tracer reports, sent on as another frame. dashboard-backend drops packets and flows of its own live viewer connections, in batches too (`traceRecords`, 2026-10-23).
- **The collector's own publishes** (2026-10-23): every batch `trace-collector-go` sends to Wotan is packets it reports in the next batch. It now reads its own TCP sockets from `/proc/self` (fd inodes, `net/tcp`, `net/tcp6`) every 5 s and leaves out packets and flows with one of them as an endpoint; `trace_collector_self_dropped_total` counts them. On east that day the whole packet stream was loopback traffic to Wotan's 18000/18001.

Other services' traffic to Wotan is still traced: it is real Kingdom traffic.

## Trace topics: shapes, clocks, units

`trace-collector-go --unified` publishes every topic as a JSON array (a batch), each event once. Audited end to end 2026-10-23 against east's live payloads. The `ebpf.packet.events`, `ebpf.flow.events` and `ebpf.latency.events` mirrors, which carried the same events a second time, are gone (ADR-147 Phase 10): dashboard-backend's eBPF ingestor holds the one subscription to each `traces.*` topic and hands each message to both of its views (flow graph and trace view).

| Topic | Record | `timestamp_ns` clock |
|-------|--------|----------------------|
| `traces.packet` | `trace_id {high,low}`, `flow_key {src_addr,dst_addr,src_port,dst_port,protocol number}`, `packet_len`, `action`, `direction` | ktime |
| `traces.flow` | `flow_key`, `flow_state {trace_id {high,low}, start_ns, last_seen_ns, packets_*, bytes_*, state}`, `event_type` (new/state_change/expired) | ktime (all three) |
| `traces.latency` | `latency_ns`, `min_rtt_ns`, `max_rtt_ns`, `samples`, `operation` | wall clock |
| `traces.correlated` | `forward`/`reverse {five_tuple {SrcIP,DstIP: [16]byte}, first_seen_ns, ...}`, `rtt_ns` | ktime |
| `traces.stats` | flow_tracker's `FLOW_STATS` counters under its `STAT_*` names, totals since the program loaded: `packets_ingress`, `packets_egress`, `flows_new`, `flows_closed`, `tcp_syns`, `tcp_fins`, `tcp_rsts`, `migrations`, `migration_expired`, `cancelled` (keys 7 and 8, the `FLOW_EVENTS` ring's sent and dropped, are retired). One snapshot every `--stats-interval` (default 10s) while flow_tracker is loaded; no subscriber yet | none |

- **Flow maps (ADR-147 decisions 19 and D6).** packet-marker's `FLOW_STATE` is an LRU hash (65,536 entries) that nothing reads: eviction bounds it, and a failed insert is counted in its `STATS` key 8 and exported as `trace_collector_flow_state_insert_failures{program="packet_marker"}` each stats tick (no series while packet_marker is not attached). It was a plain hash whose inserts failed silently once full (east's was full; west's filled in about 30 h). flow-tracker's `FLOW_EVENTS` ring is removed: nothing drained it and 95% of its events were dropped. Flows reach userspace through flow-tracker's `FLOWS` map and its batch reader.
- **Packet traces come only from packet_marker's `PACKET_EVENTS` ring buffer.** With packet_marker enabled and no ring buffer (not loaded, not attached, or the ring will not open), the collector refuses to start and says why; `--enable-packet-marker=false` runs it without packet traces. The poll fallback was removed 2026-11-09: it read packet_marker's 8-byte `STATS` counters as 68-byte traces, so every poll counted decode errors and published nothing, and it was also the only path that published `traces.stats`, which therefore never went out in ringbuf mode.

- **ktime** is `bpf_ktime_get_ns()`, CLOCK_MONOTONIC: ns since boot, not a date. Ages in the collector use `ktimeNow()` on the same clock; the dashboard's `observedAt` uses a timestamp only when it reads as recent wall time, else the arrival time.
- **trace_id**: packet_marker reads it from a TCP option (kind 253, 18 bytes) or, failing that, an IPv4 option (type 0x1F, 18 bytes), copying the option area onto the stack with `bpf_xdp_load_bytes` (S-TRUE-WIRE Step 217; it was disabled as `TODO(E1)` because the pointer walk could not be verified). Packets without either option, and every IPv6 packet (packet_marker is IPv4-only), still carry all zeros; nothing writes `TRACE_INJECT` yet. The dashboard treats all zeros as no trace ID.
- **flow_key from flow_tracker is normalized** (both directions share one key, ordered by address then port), not client to server; `packets_out` counts src to dst. The dashboard's flow graph keys packets by their own direction, so one connection can appear under both orders.
- **Flow events and dead entries.** Nothing deletes a flow from flow_tracker's `FLOWS` map: an entry keeps its last `last_seen_ns` until the LRU evicts it, so after a day most of the map is flows that ended. The collector reports a flow `expired` once, when it has been quiet for 60 s, and passes the dead entry over from then on; a flow that was already dead when the collector first saw it is not reported at all. A flow that takes a packet again is `new` again, with the entry's running totals. Until 2026-11-15 every dead entry was reported `new` and `expired` on every poll.
- **Correlated RTT** is first response minus first request of a 5-tuple pair, emitted once per pair. packet_marker sees ingress only, so a pair matches only where both directions arrive on the traced interface (loopback on east).
- **Latency** is the time spent inside `tcp_sendmsg`/`tcp_recvmsg`/`tcp_connect` (kprobe to kretprobe), not a network RTT. latency-probe's `LATENCY_MAP` is keyed by (trace ID, pid, tid), 24 bytes, one entry per thread under one trace ID, with send, recv and connect merged under one key, so the collector cannot tell which operation a sample came from (it labels all `tcp_send`) and publishes no `flow_key` for it.

## The latency sweep

`LATENCY_MAP` is an LRU hash of 65,536 entries: when it is full the kernel evicts the least recently used entry, so a thread that exited makes room for a new one. `trace-collector-go` reads it in batches of 4,096 every `--latency-sweep-interval` (default `2s`, refused under `500ms`) and publishes only the entries that took a new sample since the last sweep. Until 2026-11-12 the map was a plain hash read two system calls per entry every 200 ms (FND-044, ADR-144): it filled, refused every new thread without a word, and cost most of a core.

The probe and the collector ship together. The collector checks the object's `LATENCY_MAP` before loading it (LRU hash, 24-byte key, 40-byte value) and refuses any other layout with one error at start: `programs_loaded{program="latency_probe"}` stays 0 and the log names the sizes. To check a built object without root:

```
LATENCY_PROBE_OBJ=<path to latency-probe> go test ./cmd/trace-collector-go -run TestBuiltLatencyProbeSpec -count=1
```

Series on the collector's `/metrics`:

| Series | Reads as |
|---|---|
| `trace_collector_latency_sweep_seconds` | histogram, the time one complete sweep took |
| `trace_collector_latency_map_entries` | entries in the map at the last complete sweep; 65,536 is full, which is normal for an LRU after some days |
| `trace_collector_latency_decode_errors_total` | entries whose key or value was not the size the binary reads; anything but 0 means the object and the binary are from different builds |
| `trace_collector_latency_probe_events_total{stat}` | the probe's own counters (`LATENCY_STATS`), read at scrape time: `send_enter`, `send_exit`, `recv_enter`, `recv_exit`, `connect_enter`, `connect_exit`, `events_sent`, `events_dropped`, `map_updates`, `map_insert_failed` |
| `trace_collector_latency_stats_read_errors_total` | scrapes that could not read `LATENCY_STATS` (the family is left out of that scrape) |
| `latency_reader_errors_total{operation="sweep"}` | sweeps that did not finish; the next one starts over |
| `trace_collector_flow_map_entries` | entries in flow_tracker's `FLOWS` map at the last complete read (same batched pass, every second; an LRU of 65,536) |
| `trace_collector_map_unbatched_sweeps_total{map}` | passes over `latency` or `flows` that fell back to one entry at a time (two `bpf()` calls per entry); anything but 0 means the collector's CPU grows with that map |

Reading `map_insert_failed`: with the LRU it should stay 0, because a full map evicts instead of refusing. A rising value means the kernel refused inserts for another reason (memory pressure on the map's allocator) and new threads are losing samples. `events_dropped` rises steadily today: the probe also writes every sample to the `LATENCY_EVENTS` ring buffer, which no program reads, so it is full (parked in ADR-144's 2026-11-12 update).

## Verifier gates

Three gates, all on west (they need root to load programs):

| Gate | What it proves |
|---|---|
| `scripts/check-ebpf-loads.sh` | every program (and feature variant: `monad-cpu-ebpf[ascend-linux]`, `shield-ebpf[debug-stats]`, `hop-ebpf[hop-enforce]`) passes the kernel verifier; `docs/policy/ebpf-load-known-failing.txt` may only shrink (empty since 2026-09-25) |
| `scripts/check-ebpf-insns.sh` | ADR-012's budget, with the kernel's own counts: no program grows more than 10 % over `docs/policy/ebpf-insns-baseline.tsv`, none exceeds 900,000 unless listed in `docs/policy/ebpf-insns-over-budget.txt` (ascend at 926,542, shrink-only); `--update` re-seeds after a reviewed change |
| `scripts/upc-regression.sh` | the UPC corpus (xv6, Doom, cartridges) still boots, with its verifier numbers |

The baseline is kernel-specific: on another kernel `check-ebpf-insns.sh` exits 2 (not comparable). CI loads every program and publishes its counts in the job summary for comparison, but the ratchet gates on west (`scripts/ci-local.sh`, `scripts/check-gates-can-fail.sh`). In CI, `check-gates-can-fail.sh` proves the gate's logic on the recorded numbers and its row says so; a host outside CI on another kernel gets a refusal, not that shortcut. `cmd/ebpf-load-check` prints `<program> OK verified N insns`.

## Hand-run scripts

Lab and debugging tools, run as root by hand; nothing in the stack calls them. The supervised path is `trace-collector-go` (`unheaded-trace-collector.service`) and `ebpf-loader`.

| Script | What |
|---|---|
| `scripts/load-ebpf.sh [--dry-run] [--verbose]` | Loads packet-marker, flow-tracker, latency-probe and syscall-tracer and pins their maps under `/sys/fs/bpf/unheaded/` |
| `scripts/unload-ebpf.sh` | Detaches XDP and the TC filters from the interface and removes the pinned programs and maps |
| `scripts/shield-monitor.sh [SECS]` | Prints shield-ebpf's `STATS`, `ERROR_COUNTERS` and ANAMNESIS ring positions every few seconds (the Phase 10 tap-tomb lab) |
| `scripts/stress-cannon.py` | Floods IPv6 with Monad Hop-by-Hop options through kernel UDP sockets, so the packets cross XDP and TC (`--rate`, `--mode ramp\|baseline`, `--threads`) |
| `scripts/fast_inject.py [COUNT]` | Sends COUNT raw IPv6 frames with flow label 0xDE on an AF_PACKET socket, with no delay |
| `scripts/ringbuf-drain.py [PIN]` | Drains a BPF ring buffer (default shield-ebpf's ANAMNESIS) without parsing, the floor for how fast a consumer can go |

## Licensing

- **Aya/aya-ebpf**: MIT / Apache-2.0
- **cilium/ebpf**: Apache-2.0
- **Doom engine** (id Software linuxdoom-1.10): GPL-2.0, compiled from a checkout outside the repo by `demos/doom/Makefile` in `unheaded-doom` into `doom.elf` for the UPC ([[SBOM]])

See [LICENSES/THIRD_PARTY.md](../LICENSES/THIRD_PARTY.md) for full attribution.

---

> **Source:** [ebpf/](../ebpf/) · [docs/RUST_COMPONENTS.md](../docs/architecture/RUST_COMPONENTS.md) · [LICENSES/THIRD_PARTY.md](../LICENSES/THIRD_PARTY.md)
