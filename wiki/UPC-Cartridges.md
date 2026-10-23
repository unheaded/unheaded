# UPC Cartridges

Small signed MBC network functions run per flow by the UPC interpreter on
the packet path. Decision record: [ADR-108](../docs/adr/ADR-108-mbc-cartridges.md)
(Proposed). Third client of the [UPC programmatic API](UPC-Overview), after
Doom and xv6.

## What

- **Image** (`.cart`): 52-byte header (magic `MBCC`, stage, budget, sizes,
  entry, loop bound, flags, sha256) + ROM words + read-only DATA words.
- **Stages**: `xdp` runs per packet, forward branches only, at most 64
  instructions; `tick` runs per queued flow off the packet path, at most
  256 instructions per call, resumable, loops bounded by `loop_bound`.
- **Inputs**: enumerated header fields only (ports, protocol, length, TTL,
  TCP flags, inter-arrival bucket, flow counts, three Monad fields) and a
  clock. No syscall reads packet payload.
- **Outputs**: a verdict (PASS/DROP), EMIT records (ringbuf), QUEUE jobs for
  the tick stage.
- **Faults** pass the packet and count; 64 in one second disable the slot.
- **Engine**: `ebpf/cartridge-ebpf` (`cart_xdp`, `cart_tick`), its own BPF
  object and verifier budget. The step function is one `no_std` source,
  `monad_common::cart`, shared with the host reference, so runs replay
  bit-for-bit.

## Build

```
cargo build --release --manifest-path crates/upc-cartridge/Cargo.toml
cart build --stage xdp --asm cartridges/drop-udp-9.s -o drop9.cart
cart validate drop9.cart
cart run drop9.cart --inputs packets.log      # reference run
```

## Sign

```
upc-cartridgectl sign --key LAB.seed --ttl 24h drop9.cart   # drop9.cart.seal
```

ML-DSA-65 (Gungnir). Keys 0600, never in the repo. Recipe:
[`cartridges/README.md`](../cartridges/README.md).

## Register and load

```
upc-cartridgectl up
upc-cartridgectl --pubkey LAB.pub register drop9 drop9.cart
upc-cartridgectl --pubkey LAB.pub load drop9 --slot 0
upc-cartridgectl --pubkey LAB.pub swap pass --from 0 --to 1
upc-cartridgectl down
```

The registry is Sophia (`--sophia URL`, namespace `x-cartridge`,
provisional) or a file-backed directory (`--registry-dir`, default
`/var/lib/unheaded/cartridges`). Every `load` verifies the seal and runs the
validator on the exact bytes it then writes.

## Metrics

`upc-cartridgectl serve` on `127.0.0.1:26610`: `/metrics`, `/health`, `/ready`.

| Metric | Labels |
|---|---|
| `unheaded_cart_packets_total` | slot, verdict, lab |
| `unheaded_cart_faults_total`, `unheaded_cart_insns_total`, `unheaded_cart_runs_total`, `unheaded_cart_emits_total`, `unheaded_cart_queue_drops_total` | slot, lab |
| `unheaded_cart_queue_depth`, `unheaded_cart_scores_total`, `unheaded_cart_xdp_packets_total`, `unheaded_cart_breaker_trips_total`, `unheaded_cart_events_dropped_total` | — |
| `unheaded_cart_slot_info` | slot, name, sha256, gen, lab |

## Cartridges

| Source | Stage | What | Oracle / check |
|---|---|---|---|
| `cartridges/ratelimit.s` | xdp | per-flow token bucket; DATA rate_mbps, cap bytes | `ebpf/qos-ebpf` bucket math, verdict for verdict |
| `cartridges/sampler.s` | xdp | 1:N sampler; EMIT `S`/`F`, QUEUEs an 8-word feature snapshot; always PASS | independent model, exact 1:N |
| `cartridges/scorer.s` | tick | int8 MLP 8-16-1 over the snapshot; EMIT `A` {score 0..100, model id} | `QModel::score` bit for bit |
| `cartridges/pass.s`, `drop-udp-9.s` | xdp | plumbing | — |

Rate limiter rule: `cap <= 524287 * rate_mbps`, `rate_mbps <= 8000` (32-bit
refill math). Scorer model: `cartridges/scorer.model.json` (float weights,
scales, train/eval metrics); retrain with `trainquant gen` + `trainquant fit`
(synthetic flows only).

```
cart build --stage tick --loop-bound 16 --asm cartridges/scorer.s \
           --data cartridges/scorer.data.bin -o scorer.cart
cart tick scorer.cart --jobs jobs.log     # reference run, one "label f0..f7" per line
upc-cartridgectl pump                     # drive cart_tick until SCORE_QUEUE is empty
```

## Scores to services/anomaly

`cmd/anomaly` runs the anomaly service (`127.0.0.1:19018`,
`/api/v1/anomalies/top`) and reads score events from the pinned interpreter's
ring buffer. With `--wotan host:18000` it publishes `anomaly.scores`
(provisional topic) and the service consumes it; without, it hands the same
JSON to the service directly. The message is flow key + score + model id;
unknown fields are refused.

## Replay (time travel)

```
upc-cartridgectl record --out run.anam --every 1000 --duration 10s   # trace 1 run in 1000
upc-cartridgectl replay run.anam --slot 0                            # every run, every state hash
upc-cartridgectl replay run.anam --slot 2 --run 4 --watch r1 --to 120
cart replay scorer.cart run.anam --steps                             # same, image given directly
```

Tracing is off by default. A traced run is a head (state + FIELD table +
clock), one record per instruction (pc, word, step code, state hash) and an
end record; replay re-executes with the same step function and reports
BIT-EXACT or the first divergent instruction. Traces carry header fields and
state, never payload (files 0600). Contract:
[`docs/architecture/cartridge-replay.md`](../docs/architecture/cartridge-replay.md).

## Budgets and costs (2026-10-23)

| Program | Verifier processed insns | Gate |
|---|---|---|
| `cart_xdp` | 42,743 | 500,000 |
| `cart_tick` | 146,897 | 500,000 |

`scripts/upc-regression.sh` load-tests the object as its third variant.
One interpreted instruction costs about 4-7 ns on west (4 steps per
`bpf_loop` callback). Measured: rate limiter 343 ns/pkt (native qos-ebpf
bucket path 299), sampler 360, scorer 251,572 flows/s on one core.
