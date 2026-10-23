<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# cartridges/

MBC cartridge sources (ADR-108, Cartridge ABI v0). Assemble with `cart`
(`crates/upc-cartridge`), sign with `upc-cartridgectl`, load through the
registry. Wiki: [UPC Cartridges](../wiki/UPC-Cartridges.md).

| File | Stage | What |
|---|---|---|
| `pass.s` | xdp | VERDICT PASS on every packet (plumbing) |
| `drop-udp-9.s` | xdp | DROP UDP to port 9 (discard), PASS the rest (plumbing) |
| `ratelimit.s` + `ratelimit.data.bin` | xdp | per-flow token bucket, qos-ebpf semantics (DATA: rate_mbps 100, cap 12,500 B, divide magic) |
| `sampler.s` + `sampler.data.bin` | xdp | 1:N flow sampler (N = 8), queues an 8-word feature snapshot |
| `scorer.s` + `scorer.data.bin` | tick | int8 MLP anomaly scorer (`--loop-bound 16`); `scorer.model.json` = float weights, scales, metrics (trainquant, synthetic data) |

## Build

```
cargo build --release --manifest-path crates/upc-cartridge/Cargo.toml
cart build --stage xdp --asm cartridges/pass.s -o pass.cart
cart validate pass.cart
```

## Lab signing

Lab keys live outside the repo, mode 0600, and are never committed
(ADR-043 #4). Where a non-lab key lives is the operator's decision.

```
head -c 32 /dev/urandom > "$S5/lab.seed" && chmod 600 "$S5/lab.seed"
yggdrasil-evidence pubkey --key "$S5/lab.seed" --out "$S5/lab.pub"
upc-cartridgectl sign --key "$S5/lab.seed" --ttl 24h pass.cart      # writes pass.cart.seal
```

## Register, load, swap (root: map writes need CAP_BPF)

```
upc-cartridgectl up
upc-cartridgectl --pubkey "$S5/lab.pub" register pass pass.cart
upc-cartridgectl --pubkey "$S5/lab.pub" load pass --slot 0
upc-cartridgectl --pubkey "$S5/lab.pub" swap drop9 --from 0 --to 1
upc-cartridgectl list ; upc-cartridgectl stats
upc-cartridgectl serve --metrics-addr 127.0.0.1:26610 [--xdp-iface IF]
upc-cartridgectl unload --slot 1 ; upc-cartridgectl down
```

`load` refuses an image without a seal, a seal over other bytes, a seal
from another key, an expired seal, or bytes the validator rejects. `--lab`
accepts unsigned images and marks the slot `lab="1"` on every metric.
