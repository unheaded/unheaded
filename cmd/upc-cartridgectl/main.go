// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// upc-cartridgectl — loads only signed, validated MBC cartridges into the
// cartridge-ebpf interpreter (ADR-108), resolving them by name through the
// cartridge registry (Sophia, or a file-backed registry while the stack is
// down), swaps slots, drives the tick stage, and serves metrics.
//
//	upc-cartridgectl [global flags] up                     load + pin the interpreter
//	upc-cartridgectl [global flags] down                   unpin (unloads it)
//	upc-cartridgectl sign --key SEED [--ttl D] FILE.cart   write FILE.cart.seal
//	upc-cartridgectl [global flags] register NAME FILE.cart
//	upc-cartridgectl [global flags] load NAME --slot N
//	upc-cartridgectl [global flags] unload --slot N
//	upc-cartridgectl [global flags] swap NAME --from A --to B
//	upc-cartridgectl [global flags] list | stats
//	upc-cartridgectl [global flags] bench [--repeat N] [--dport P]
//	upc-cartridgectl [global flags] pump [--max N]
//	upc-cartridgectl [global flags] serve [--metrics-addr 127.0.0.1:26610] [--xdp-iface IF]
//	upc-cartridgectl weights publish --wotan URL --cart FILE.cart --sign-key SEED [--model NAME]
//	upc-cartridgectl weights fetch --wotan URL --model NAME --pubkey PUB --out DIR
//	                                                       DATA over Wotan, sealed (weights.go)
//
// Global flags: --registry-dir (default /var/lib/unheaded/cartridges),
// --sophia URL (+ SOPHIA_TOKEN), --pubkey FILE, --lab, --pin-dir, --obj,
// --cart (the `cart` CLI used as the validator).
package main
