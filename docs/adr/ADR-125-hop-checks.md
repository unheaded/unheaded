<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-125: Hop checks: validators, HMAC-key presence and retry state move from Shield ingress to hop-ebpf

**Status:** Accepted (Stevie Bellis, 2026-11-18; `docs/battle-plans/DECISION-PACK-2026-11-18.md` round 5). Previously: Proposed. DP-716 (a) decided the move; this ADR records the semantics that answer did not spell out. Stevie accepts it.
**Date:** 2026-11-04
**Amended:** 2026-11-09: the object refuses enforce unless built with `hop-enforce`.
**Amended:** 2026-11-21: the Scope ruling for FND-033 is recorded under "Scope ruling 2026-11-21"; it is not built.
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
- **Scope.** Enforce covers the packets that reach the checks. Four classes do not, and which of them stay is Stevie's ruling (FND-033): a packet with a bad CRC, a packet with the CHAOS flag and a packet at the CONFIG key 3 hop limit, which leave before the checks, and a deploy ring with no validator, where no check runs.
- **Time.** `expires_at` is CLOCK_BOOTTIME seconds, read with `bpf_ktime_get_boot_ns`, 0 = never. The writer converts. ADR-119 D6.4(c)'s one-clock rule.
- **Modes.** CONFIG key 4: 0 off (default), 1 observe, 2 enforce. Observe counts and emits ANOMALY; enforce also drops. STATS 15 `checks_run`, 16 `checks_failed`, 17 `checks_enforced`, 18 `checks_enforce_refused`. `unheaded hop mode` is the one writer in the tree (2026-11-06, Stevie): it refuses enforce until FND-031, FND-033 and FND-035 close (`hoprules.EnforceBlockers`, held to the register by a test); `TestConfigMap_OneWriter` fails on a second writer in the tree.
- **The object refuses too (2026-11-09, Stevie).** Root on the pins is outside any writer, so `hop_xdp` refuses enforce unless built with the cargo feature `hop-enforce`, off by default. Without it, key 4 = 2 runs as observe: checks run, count and emit ANOMALY, a failed check counts STATS 18 instead of 17, and nothing drops. Every package, release and Makefile target builds that default object; only `scripts/check-ebpf-loads.sh` builds the `hop-enforce` variant (`target/load-gate/hop-enforce`), for `tests/hopchecks`, and `TestHopEnforce_OnlyTheTestPath` (pkg/hoprules) fails on any other build file naming the feature. `tests/hopchecks` writes the map itself: its enforce rows run on the `hop-enforce` object, and `TestDefaultObjectRefusesEnforce` runs them all on the default object and wants no drop. The `hop-enforce` object is byte-identical to the object before this change. The default object's verified count moves and the variant is a new row, so `check-ebpf-insns.sh` fails until a root run on west records both with `--update`.
- **Writer.** `unheaded hop rules apply` over `pkg/hoprules` and `pkg/ebpf.PinnedMap`. Key bytes come from 0400 files named in the rules file, never from the file itself.

## What this is not

This is an authorization check on hop-local state. It does not prove that a packet came from who it says. Integrity against a compromised intermediate node stays with foundation-06's options (a) ESP and (c) ML-DSA at Shield boundaries, or ADR-119 if it is ever built. The residual gap is a new finding, FND-031.

## Consequences

Default behaviour is unchanged: mode 0 and empty maps. Nothing loads hop-ebpf in production today. `cmd/ebpf-loader --only hop-ebpf` is new and is not in the default program list. Growth over the ratchet is recorded with `check-ebpf-insns.sh --update` and the reason.

## Open questions

1. Should the validator also key on `dst_service_id` alone, so one rule can guard a service?
2. Does `pkg/protocol/integrity` stay once RFC item 7 is ruled?

## Decisions 2026-11-18 (decision pack, round 5)

From `docs/battle-plans/DECISION-PACK-2026-11-18.md`.

- Accepted as built (Stevie Bellis: "Accept all eight"): the maps and checks in hop-ebpf, mode 0 by default, and a default object that refuses enforce. Acceptance turns nothing on. Enforce stays behind the `hop-enforce` feature and the findings the Decision names (FND-031, FND-033, FND-035); the Scope ruling (FND-033) and the two open questions above stay open.

## Scope ruling 2026-11-21 (FND-033; decision pack row 41, M-197.1)

This section records a ruling. It changes no code: at develop `08f4f961` the three early exits in `ebpf/hop-ebpf/src/main.rs` still return before `run_checks`, and an unmatched deploy ring still runs no check. The text above is not edited; the Decision's "Scope" paragraph stays as the record of what was open.

**The question, as asked** (`docs/battle-plans/progress/S-WIRE-HONEST-progress.md`, "For Stevie", item 5):

> FND-033, your ruling: should enforce also cover a bad CRC, CHAOS and the hop limit, and what should a `deploy_ring` with no validator do? Today all four pass unchecked.

**Who ruled, and on what authority.** Not Stevie in person. `docs/battle-plans/DECISION-PACK-2026-11-18.md`, round 11, row 41, asks whether Claude may settle the smaller technical calls "on the seats' written recommendation and list each for his later review", "hop enforcement and its tooling" among them, and his answer is:

> **Yes, except weakening or irreversible.** Each call is logged below under "Decided by Claude, reviewable". Held and asked: anything that weakens security, deletes data, adds third-party software, changes the frozen wire format, or cannot be undone.

**The seats' written recommendation** (`~/tmp/pending-2026-11-21/row41-recommendations.md` on west, entry M-197.1, written 2026-11-21):

> (b) for all four: under enforce a bad CRC, a CHAOS-flagged packet and a packet at the hop limit are dropped and counted, and a deploy ring with no validator is dropped. CLAUDE.md's network rule is "explicit allow, default deny", and an enforce mode with four silent exceptions is the thing the finding says cannot be relied on. Observe mode is unchanged. Enforce stays refused until FND-031 and FND-035 close.

**The ruling.** As recommended, for each of the four classes the Scope paragraph lists: none stays outside an enforcing hop.

| Class (the Scope paragraph's words) | Under enforce | Under off and observe |
|---|---|---|
| a packet with a bad CRC | dropped and counted | as today |
| a packet with the CHAOS flag | dropped and counted | as today |
| a packet at the CONFIG key 3 hop limit | dropped and counted | as today |
| a deploy ring with no validator | dropped | as today: no check runs |

Why it is inside row 41: every row tightens (the hold list names weakening, not tightening), nothing on the wire changes, no data is deleted, nothing third-party is added, and the default object never enforces, so a revert of the later code restores today's behaviour.

**What this does not do.**

- It does not allow enforce. `hoprules.EnforceBlockers` still names FND-031, FND-033 and FND-035, and the packaged object still runs key 4 = 2 as observe.
- It does not close FND-033. Its verify text wants three things: the ruling, this ADR stating each result (done here), and "tests/hopchecks holds an enforce case for each one asserting it" (not done). The verify text also says "Stevie rules": whether a row 41 ruling meets that is his to confirm at review.
- It does not settle the two open questions above, nor the meaning of `max_hop_count` against foundation-06's limit (the same progress file, item 4).

**What the code change will have to carry** (repository code, a root run in the lab; not part of this section): `run_checks` moves ahead of the three early exits under the `hop-enforce` feature only; one enforce case per class in `tests/hopchecks`; `check-ebpf-insns.sh --update` with the growth and its reason. Two consequences to name when it is built: an enforcing hop with no validator for a ring drops that whole ring, so a rules file must list every ring the hop is meant to carry before the mode is set; and a CHAOS-flagged packet no longer crosses an enforcing hop, so a `packet_chaos` experiment and an enforcing hop do not share a path.

**Undo.** Add a dated section that withdraws this one; no code or host state depends on it.
