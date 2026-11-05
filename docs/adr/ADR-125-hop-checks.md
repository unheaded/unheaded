<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-125: Hop checks: validators, HMAC-key presence and retry state move from Shield ingress to hop-ebpf

**Status:** Proposed. DP-716 (a) decided the move; this ADR records the semantics that answer did not spell out. Stevie accepts it.
**Date:** 2026-11-04
**Related:** FND-025, DP-716, DP-715, ADR-012 (verifier budget), ADR-119 (the PQC fast path; its fact 2 on flow labels), `docs/protocol/RFC-EDITOR-REQUESTS-WH.md`, S-TRUE-WIRE progress "E8".

## The pitch (decision pack, DP-716)

> HMAC, retry and hop checks sit at XDP ingress where packets carry no Monad yet. (a) Move them to hop-ebpf. Answer: Shield checks move to hop-ebpf.

## Context

At Shield ingress the Monad does not exist yet: Shield creates it. So `HMAC_KEYS`, `RETRY_TOKENS` and `HOP_VALIDATORS` had nothing to key on, nothing wrote them, and the checks were never built. hop-ebpf sees a Monad on every hop. Four facts decide what can move:

1. **No HMAC can be verified.** v0x01 has no tag field, foundation-06 leaves "the optional HMAC field" undefined, and eBPF has no HMAC helper. Only the presence and expiry of a key can be checked.
2. **No retry token is on the wire either.** `RETRY_TOKENS` can only say that a source was validated before and that the validation is still live.
3. **The flow label cannot key anything.** Shield draws it at random per packet. `HMACKeyKey.FlowID` is documented as "from Monad header", and the Monad has no such field.
4. **Budget.** `hop_xdp` is 153,835 verified instructions. The ratchet fails above 169,218 unless growth is reviewed and recorded.

## Decision

- **Maps.** `HMAC_KEYS`, `RETRY_TOKENS` and `HOP_VALIDATORS` move to `ebpf/hop-ebpf`; Rust first, `bpfschema` follows. `AUTHORITY` and its discarded lookup are deleted from shield-ebpf: it keyed `qos_class` as a dictionary id and no packet carries an owner. FND-025's verify text allows "enforce or gone".
- **Keys.** `flow_id` = `src_service_id | dst_service_id << 8`, the `CIRCUIT_ERRORS` pair. `namespace` = `deploy_ring`. `hop_id` = CONFIG key 0. Layouts match the Go structs byte for byte. Shield leaves service ids at 0, so Shield-born packets share one bucket until something resolves them; a validator on that bucket applies to all of them.
- **Order.** After the hop-limit guard, before the Monad is cloned: look up `HOP_VALIDATORS[(hop_id, namespace)]`. No validator: no check runs. With one: version in range (`allowed_version_min` and `max` both 0 means no version rule); hop_count at or under `max_hop_count` (0 means no cap, as CONFIG key 3 does); if `require_crc`, a CUSTOM packet fails (it has no CRC to check); if `require_hmac`, a live `HMAC_KEYS` entry; if flag bit 0, a live `RETRY_TOKENS` entry for (flow_id, low 48 bits of source). `allowed_dict_ids` is not evaluated: no dictionary id is on the wire.
- **Time.** `expires_at` is CLOCK_BOOTTIME seconds, read with `bpf_ktime_get_boot_ns`, 0 = never. The writer converts. ADR-119 D6.4(c)'s one-clock rule.
- **Modes.** CONFIG key 4: 0 off (default), 1 observe, 2 enforce. Observe counts and emits ANOMALY; enforce also drops. STATS 15 `checks_run`, 16 `checks_failed`, 17 `checks_enforced`.
- **Writer.** `unheaded hop rules apply` over `pkg/hoprules` and `pkg/ebpf.PinnedMap`. Key bytes come from 0400 files named in the rules file, never from the file itself.

## What this is not

This is an authorization check on hop-local state. It does not prove that a packet came from who it says. Integrity against a compromised intermediate node stays with foundation-06's options (a) ESP and (c) ML-DSA at Shield boundaries, or ADR-119 if it is ever built. The residual gap is a new finding, FND-031.

## Consequences

Default behaviour is unchanged: mode 0 and empty maps. Nothing loads hop-ebpf in production today. `cmd/ebpf-loader --only hop-ebpf` is new and is not in the default program list. Growth over the ratchet is recorded with `check-ebpf-insns.sh --update` and the reason.

## Open questions

1. Should the validator also key on `dst_service_id` alone, so one rule can guard a service?
2. Does `pkg/protocol/integrity` stay once RFC item 7 is ruled?
