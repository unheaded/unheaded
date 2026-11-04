<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-119: The Shield XDP post-quantum signature fast path, designed with both ends

**Status:** Proposed (pitch, parked under ADR-112). Design complete on paper; nothing is built. The removal of 2026-10-28 stands.
**Date:** 2026-11-03
**Grounded on:** `develop` at `dd70a4da` (the removed code read at `233b6451`, the last commit that carried it).
**Related:**
- ADR-105 (hybrid ML-KEM key exchange, crypto inventory; "authentication stays classical", "PQC-00
  Layer 1 is frozen and out of scope", lines 22-27)
- ADR-112 (plan freeze; this is a parked pitch ADR)
- ADR-116 (monitor the monitors; the "verifier alive" expectation below)
- ADR-103 (service identities, `trust.d`)
- ADR-012 (the 900,000 verified-instruction budget) and `scripts/check-ebpf-insns.sh`
- `docs/security/shield-ebpf-pqc-fast-path-unwired-2026-08-03.md` (what existed and why it was
  removed)
- S-TRUE-WIRE Steps 231-237 (`docs/battle-plans/progress/S-TRUE-WIRE-progress.md:260-266`) and its
  E8 addendum (lines 239-242)
- DECISION-PACK-2026-11-03 DP-203 and DP-204
  (`docs/battle-plans/DECISION-PACK-2026-11-03.md:194-203`)
- `docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md` (PQC-00),
  `draft-bellis-unheaded-protocol-foundation-06.md`, `draft-bellis-unheaded-sophia-dictionary-03.md`

## The pitch (Stevie, 2026-11-03, verbatim)

> don't lift freeze but do spawn additional agent to design it properly and leave the parked and pending till after the freeze

Read with DP-204: the removal is confirmed (option A), and the second choice, "design PQC-at-XDP
properly", is done now on paper so that when the freeze lifts the sprint starts from a complete
design with a producer and a consumer, not from a dangling map.

## Context

### What was there

At `233b6451`, `ebpf/shield-ebpf/src/main.rs:876-930` declared:

- `PQC_SIG_STATUS`, a `HashMap<u32, PqcSigStatusEntry>` with 65,536 entries, keyed by SigRef (lines
  880-882);
- `pqc_fast_path_check(flags, sig_ref)`: if `is_pqc_active(flags)` (S and CUSTOM both set), look the
  SigRef up and map `Valid` to `CachedValid`, `Invalid` and `Expired` to `CachedInvalid`, anything
  else or a miss to `NeedUserspace` (lines 897-913);
- `PqcFastPathResult { NotPqc, CachedValid, CachedInvalid, NeedUserspace }` (lines 921-930), with
  `CachedInvalid` documented as "drop packet (PESSIMISTIC) or warn (OPTIMISTIC)".

The entry type lived in `ebpf/pqc-common/src/lib.rs:128-151` at the same commit: `status u8, algo_id
u8, tier u8, _pad u8, verified_at u32`, 8 bytes, `#[repr(C, packed)]`, with the status enum `Pending
0, Valid 1, Invalid 2, Expired 3, Unsupported 4` (lines 59-73).

### Why it was dead

Both ends were missing (`docs/security/shield-ebpf-pqc-fast-path-unwired-2026-08-03.md`, "What is
missing"): nothing called the check from `try_shield_xdp`, and nothing in any language wrote the
map. Every lookup would have returned `NeedUserspace`, and there was no userspace. The risk recorded
there was not wrong drops but the belief that XDP-layer PQC enforcement existed. S-TRUE-WIRE Steps
231-237 removed the map, the function, the enum and the entry type; Shield XDP stayed at 1,502
verified instructions and TC at 976 (progress log lines 260-263). `ebpf/pqc-common` now carries only
the 12-byte `PqcValue` and the flag test (`ebpf/pqc-common/src/lib.rs:30-62, 78-87`) and nothing
consumes it.

Also recorded at Step 234 (progress log lines 239-242, the E8 addendum): PQC-00 §4 assumes packets
*arrive* at Shield ingress carrying a Monad whose value field holds SigRef and KeyRef, and Shield
verifies and strips it; the reference Shield instead *creates* the Monad at ingress
(`main.rs:262-280`) and strips it at TC egress (`main.rs:497-509`). The only XDP branch that sees an
incoming Monad today is step 4, "already a Kingdom packet, pass through without re-stamping"
(`main.rs:246-260`). This design has to say which packets it keys on. It does, below.

### What exists that the design can use

- **Signing and verification in Go.** `pkg/crypto/pqc/ml_dsa.go:92-100` (`NewMLDSAVerifier`,
  `Verify(message, signature, publicKey)`), `hash_pfx.go:14-23` (`HashPrefix`, `VerifyHashPrefix`),
  `security.go:98-170` (`SigRefRateLimiter`: allocations per second, utilization, eviction
  threshold). ML-DSA-65 is the signature algorithm the Kingdom already uses for Wotan topic signing,
  Gungnir seals and evidence (ADR-105 decisions 5 and 8), and `unheaded-keygen -algo mldsa65`
  already makes the keys (CLAUDE.md, "Wotan Topic Signing").
- **A userspace verification pipeline with no caller.** `services/shield/pqc_verify.go:83-120`
  (`PQCVerifier.Verify`, a 7-step check), `:235-278` (signature and key caches, a per-flow sliding
  replay window), `services/shield/pqc_security.go:17-20, 95-96` (algo-confusion guard, SigRef rate
  limiter, a TOCTOU cache that pins a verification result). `cmd/pqc-verifier/main.go:4-12` wraps it
  as an HTTP service on port 19008 (`pkg/ports/ports.go:42`) that nothing calls.
- **A parked key-lifecycle control plane.** `services/keymgmt/keymgmt.go:4-11`: generation, rotation
  with a 60-second grace period, revocation; keys referenced by 2-byte KeyRefs; port 19007
  (`pkg/ports/ports.go:41`); Wotan topics `pqc.key.{generated,rotated,revoked}`.
  `lifecycle/grace.go:4-6` and `lifecycle/revocation.go:12-19` (a revocation cascades to the SigRefs
  signed under the key). `store/sophia_store.go:4-16` is an in-memory stand-in for the Sophia maps,
  not a BPF writer. DP-203's recommendation is to delete all eight parked planes; this ADR needs the
  *design* of keymgmt, not its code.
- **Per-flow PQC state in Wotan.** `pkg/wotan/state.go:149-155` (`PQCStateManager`).
- **Key trust the ADR-103 way.** `pkg/auth/keys.go:25, 213-225`: a verifier accepts a peer's public
  key only if `/etc/unheaded/trust.d/<name>.pub` exists; the file name binds the identity.
- **Map mirror discipline.** `pkg/protocol/bpfschema/doc.go:4-15`: Rust is the source of truth;
  `rust_source_test.go` parses the `#[repr(C)]` structs and `#[map]` declarations and fails when the
  Go mirror's size, offsets, map name or key/value size differ; "add a Go mirror only after the Rust
  struct or map exists; change the Rust first, then follow it here."
- **Pin path precedent.** `/sys/fs/bpf/unheaded/anamnesis_events`
  (`pkg/ebpf/anamnesis_reader_linux.go:19`), `/sys/fs/bpf/unheaded/cart`
  (`pkg/cartridge/cartridge.go:42`). PQC-00 §6.1 names `/sys/fs/bpf/sophia/pqc_sigs` for the
  *full-signature* map, which is a different map from the status cache designed here.
- **The verifier ratchet.** `scripts/check-ebpf-insns.sh:7-17, 37-38`: budget 900,000; a program
  that grows more than 10% over its baseline row fails; `--update` records reviewed growth. Baseline
  rows (`docs/policy/ebpf-insns-baseline.tsv`, kernel 6.17.0-41, 2026-10-28):
  `shield-ebpf/shield_xdp` 1,502 (default and `debug-stats`), `shield_tc` 976 default, 1,084
  `debug-stats`.

### Two wire facts the design cannot paper over

1. **The 12-byte PQC value does not fit the frozen 20-byte Monad.** Foundation-06 (`:1620-1650`)
   places the value "in the Monad scratch bytes (offsets 0x0E-0x13)", but `Monad.scratch` is 4 bytes
   (`ebpf/monad-common/src/lib.rs:609-655`) and 0x12-0x13 is the checksum (or exponent keys under
   CUSTOM). Only the 32-bit SigRef fits, in `scratch[0..4]`. KeyRef, HashPfx and SeqNum have no room
   on the v0x01 wire. PQC-00 §5 (`:280-327`) describes a 24-bit SigRef and 24-bit KeyRef in a
   12-byte layout; Foundation-06 and `pqc-common` say 32-bit SigRef and 16-bit KeyRef. The drafts
   disagree with each other and with the register.
2. **Flow identity at XDP is weak.** Shield assigns the flow label from `bpf_get_prandom_u32()` per
   packet (`main.rs:~266`). Any per-flow state at XDP must key on addresses and ports, not the flow
   label.

This design therefore keys the XDP cache on SigRef alone, takes everything else (key id, algorithm,
expiry) from the map value written by userspace, and treats the wire-format reconciliation as an RFC
Editor item for the sprint (open question 1).

## Decision direction (Claude's recommendations, springboard)

### D1. Who verifies: one userspace verifier, the only map writer

**Recommendation:** revive `cmd/pqc-verifier` (port 19008 is already reserved) as the ring-buffer
consumer and the sole writer of the status map, and drop its HTTP `/verify` API; keep `/health`,
`/ready`, `/metrics`. It runs as its own nologin uid (`pkg/uids`), with `CAP_BPF` only, opening
pinned maps that the Shield loader created and chowned to it. It links `services/shield.PQCVerifier`
with `pkg/crypto/pqc.MLDSAVerifier` as the `SetCryptoVerify` function.

**Keys.** Peer public keys are ML-DSA-65 `.pub` files in `/etc/unheaded/trust.d/` (the ADR-103
pattern; `pkg/auth/keys.go:25`), one file per signing peer, read at start and on SIGHUP. The
`key_id` written into the map is the first 4 bytes of `KeyID(pub)` as `pkg/auth` already computes
it. Rotation, grace and revocation follow the `services/keymgmt` design (`lifecycle/grace.go`,
`revocation.go`): a revoked key cascades to every SigRef it signed, which the verifier rewrites to
`Invalid`. Whether keymgmt the *service* exists is DP-203's call; the verifier only needs the trust
directory and a revocation list file in the first stage.

**What gets verified.** The verifier needs the full signature and the signed pseudo-header. Neither
is on the v0x01 wire. They come from the Sophia signature store (`PQC_SIG_MAP`, Sophia-03
`:698-720`; `pkg/sophia/pqc_control.go:13-40` is the userspace mirror), loaded by the control plane
when a peer Shield announces a new SigRef on a signed Wotan topic (`pqc.sig.announce.<node_id>`,
signed under `pkg/wotansig` like `drift.*`). The pseudo-header is PQC-00 §7 (immutable fields only).
Inferred: the first version signs per (src prefix, dst prefix, key epoch, SigRef) rather than per
packet, which is what signature-by-reference means.

**Rate limits.** `pkg/crypto/pqc.SigRefRateLimiter` caps SigRef allocation; a token bucket per
source /64 caps `NeedUserspace` requests the verifier will act on (default 200 per second per
source, 5,000 per second total; inferred, to be measured). Requests above the cap are counted and
dropped from the queue, never verified; the packet's fate was already decided in XDP (D3). ML-DSA-65
verification cost in Go on west is unmeasured; the sprint measures it before the caps are set.

**The flow it keys on.** The check runs only on the step-4 branch of `try_shield_xdp`
(`main.rs:246-260`): an inbound IPv6 packet that already carries a Monad HBH option. That is
Kingdom-to-Kingdom ingress (east to west over `wg0` and any future peer Shield), never Shadow
ingress, which has no Monad. This matches PQC-00 §4's model for the one place the reference Shield
actually sees an arriving Monad, and it answers E8 for this check only: the PQC path verifies a peer
Shield's stamp, not an end user's packet.

### D2. The BPF map contract

Rust first (`ebpf/pqc-common/src/lib.rs`), then the Go mirror in `pkg/protocol/bpfschema` with a
`rust_source_test` row; never the reverse (`bpfschema/doc.go:12-15`).

```
Map:        PQC_SIG_STATUS     HashMap<u32, PqcSigStatusEntry>, 65,536 entries
Key:        SigRef u32         Monad.scratch[0..4], big-endian, as the wire carries it
Value:      PqcSigStatusEntry  #[repr(C, packed)], 16 bytes
              status      u8    0 Pending, 1 Valid, 2 Invalid, 3 Expired, 4 Unsupported
              algo_id     u8    0x02 ML-DSA (Foundation-06 algorithm registry)
              _reserved   u16   zero; verifier refuses to load an entry with it set
              expires_at  u32   unix seconds; XDP treats Valid past this as Expired
              key_id      u32   first 4 bytes of KeyID(pub) of the signing key
              verified_at u32   unix seconds of the last userspace verdict
Writer:     cmd/pqc-verifier only (uid-owned pin, 0600)
Reader:     shield_xdp (lookup only; it never updates the map)
Pin:        /sys/fs/bpf/unheaded/shield/pqc_sig_status
Eviction:   userspace sweep every 1 s removes entries past expires_at + 60 s;
            at 90 % utilization the verifier evicts least-recently-verified
            Valid entries first and never evicts Invalid (SigRefRateLimiter.NeedsEviction)
Lifetime:   Valid entries get expires_at = min(key expiry, now + 300 s)
```

```
Map:        PQC_VERIFY_REQ     RingBuf, 256 KiB (Anamnesis is 8 MiB, main.rs:58;
                               Heimdall rings are 64 KiB, crates/heimdall-bpf/src/execve.rs:35)
Record:     48 bytes: sig_ref u32, flow_label u32, src [u8;16], dst [u8;16],
            ts_ns u64 (bpf_ktime_get_ns), mode u8, reason u8, _pad u16
Producer:   shield_xdp on NeedUserspace (reserve; on failure count STAT_PQC_RING_FULL)
Consumer:   cmd/pqc-verifier
```

```
Map:        SHIELD_CONFIG (exists, main.rs:121-122; key 0 is the AF_XDP toggle)
  key 1:    pqc_mode        0 OFF (default when absent), 1 OPTIMISTIC, 2 PESSIMISTIC
  key 2:    verifier_alive  unix seconds of the verifier's last heartbeat, written every 2 s
```

Why a plain `HashMap` and not `LruHashMap`: kernel LRU would evict an `Invalid` entry under load and
turn a known-bad SigRef back into `NeedUserspace`, which passes in OPTIMISTIC. Eviction policy
belongs to the one process that knows which entries are safe to lose.

Why `expires_at` is checked in XDP and not only by the sweeper: if the verifier dies, cached `Valid`
entries stop passing on their own. The data plane does not depend on a userspace timer to stop
trusting a stale verdict.

STATS keys (`main.rs:106-107`, 32 entries, inferred free keys 16-23): `pqc_seen`,
`pqc_cached_valid`, `pqc_cached_invalid`, `pqc_need_userspace`, `pqc_ring_full`, `pqc_expired`,
`pqc_mode_drop`, `pqc_verifier_dead`. The exporter publishes them as `unheaded_shield_pqc_*_total`.

### D3. XDP behaviour per state

Evaluated at step 4 (`main.rs:246-260`), after the blocklist (step 3) and before the pass-through.
`flags = Monad.flags` at 0x07; `sig_ref = be32(Monad.scratch[0..4])`.

| State | How it is reached | OFF | OPTIMISTIC | PESSIMISTIC |
|---|---|---|---|---|
| NotPqc | S and CUSTOM not both set | pass | pass | pass (open question 4: should a boundary that requires PQC drop unsigned Monads?) |
| NotPqc, SigRef 0 with S set | PQC-00 §5 "MUST reject" | pass | count, pass | drop |
| CachedValid | entry.status 1 and now < expires_at | pass | pass | pass |
| CachedInvalid | status 2, 3, 4, or Valid past expires_at | pass | count `pqc_cached_invalid`, emit Anamnesis ANOMALY, pass | drop, emit Anamnesis ERROR |
| NeedUserspace | miss or status 0 | pass | reserve ring record, count, pass | reserve ring record; drop |
| NeedUserspace, ring full | reserve failed | pass | count `pqc_ring_full`, pass | count, drop |
| Verifier dead | now - verifier_alive > 10 s | pass | count `pqc_verifier_dead`, pass | CachedValid still passes; NeedUserspace drops (fail closed) |

PESSIMISTIC here means "drop until verified" rather than PQC-00 §9.1's "buffer until verified": XDP
cannot hold a packet, and a retransmit arrives in milliseconds. The first packet of a new SigRef is
lost once per SigRef lifetime. That is the cost of a real control and is stated here so nobody is
surprised by it. The verifier health timeout (10 s) is PQC-00 §8.3's number.

The check never touches the map from XDP (no `insert`, no `update`). A compromised or buggy data
plane path cannot promote a SigRef.

### D4. The mode switch and its default

- The switch is `SHIELD_CONFIG[1]`, written by the Shield loader from `/etc/unheaded/shield.yaml`
  (`pqc_mode: off | optimistic | pessimistic`) and never by the verifier.
- **Default: OFF while the feature is absent from the build (stage 0-1), OPTIMISTIC when the map
  first ships (stage 2), PESSIMISTIC only after a soak** of at least 14 days in OPTIMISTIC on the
  live east-west link during which `pqc_cached_invalid_total` stays at zero, `pqc_ring_full_total`
  stays at zero, and `pqc_verifier_dead_total` stays at zero. The soak result is recorded in the
  sprint's progress file before the config line changes.
- PQC-00 §9.3 says an implementation MUST default to PESSIMISTIC when no policy is configured. We
  satisfy that by always writing an explicit mode from config: an absent key is OFF, which is "no
  check", not "a permissive check". Whether the draft wants an amendment to say so is open question
  5.
- **ADR-116 expectations** (registry rows the sprint adds): `pqc-verifier` heartbeat
  (`SHIELD_CONFIG[2]` moves every 2 s; max silence 10 s; absence means every new SigRef is dropped
  in PESSIMISTIC); ring consumption (`pqc_need_userspace_total` rises while the verifier's
  `verified_total` does not: the verifier is stuck); `pqc_cached_invalid_total` non-zero in
  OPTIMISTIC is an alert with a runbook, not a log line. "Verifier alive" is the first expectation;
  a silent verifier in PESSIMISTIC is a self-inflicted outage and must page.

### D5. Verifier budget estimate

Baseline `shield_xdp` is 1,502 verified instructions (`ebpf-insns-baseline.tsv`). The check adds,
per the verifier's path exploration (both branches of every test are walked):

| Piece | Estimate (verified insns) | Basis |
|---|---|---|
| Flag test and bounds-checked 4-byte scratch read | 15-30 | the existing step-4 opt_type read is of this shape (`main.rs:250-256`) |
| `SHIELD_CONFIG` lookup for mode and heartbeat (2 lookups) | 40-60 | one `HashMap::get` with null check is 15-25 in this program (inferred from `increment_stat`, `main.rs:868-883`) |
| `PQC_SIG_STATUS` lookup, status match, expiry compare | 40-80 | |
| `bpf_ktime_get_ns` and two compares | 10-15 | |
| Ring buffer reserve, 48-byte write, submit, plus the discard branch | 60-120 | Heimdall's Gjallarhorn XDP observer with one ring submit verifies at 292 total (CLAUDE.md, S5 update) |
| Up to three `increment_stat` calls on the taken paths | 60-100 | |
| Anamnesis emit on CachedInvalid | 40-80 | `emit_anamnesis` exists (`main.rs:814-838`) |

Total growth: roughly 270-490, landing `shield_xdp` at about 1,800-2,000 verified instructions,
under 0.25% of the 900,000 budget and three orders of magnitude below `monad_cpu` ascend (868,290)
or `anomaly_xdp` (386,952). It will exceed the 10% growth ratchet (150 over 1,502), so the sprint
records the new row with `check-ebpf-insns.sh --update` and the reviewed reason, as the ratchet
intends. The number is checked, not trusted: stage 1's gate is the measured count, under both
feature variants, on west's kernel.

### D6. Threat model

1. **Replay of a cached-valid SigRef.** An attacker who captures one Monad-bearing packet from a
   peer Shield can replay its SigRef on packets of their own. With SigRef the only wire field, XDP
   alone cannot tell them apart. Mitigations: the SigRef verdict is bound to the source prefix the
   verifier saw (`src` is in the ring record; the verifier refuses to mark Valid a SigRef seen from
   two sources and rewrites it Invalid); Valid lifetime is 300 s; the step-3 blocklist and `wg0`
   itself (a replayed packet from outside the tunnel does not arrive as a Kingdom packet). This is
   weaker than PQC-00 §8.1's per-packet SeqNum check, which the v0x01 register cannot carry.
   Alternative C below closes it better.
2. **Cache poisoning by a userspace compromise.** The verifier is the only writer; its uid owns the
   pin; it has no listener but `/health`, `/ready`, `/metrics` on loopback; keys are read-only
   `trust.d` files it cannot write. A compromised verifier can mark anything Valid. That is the same
   trust as today's `wotansig` verifier, and it is bounded by expiry (300 s) and by the audit line
   every write produces (`pqc.verdict.<node_id>`, signed). A compromised *loader* could flip the
   mode; the loader already owns every Shield map, so this adds no new principal.
3. **DoS via NeedUserspace flood.** An attacker sends Monad-bearing packets with fresh SigRefs. XDP:
   ring reserve fails at 256 KiB (about 5,000 records in flight), counted, and in PESSIMISTIC the
   packet is dropped anyway, so the flood costs XDP one lookup and one failed reserve per packet.
   Verifier: per-source and global token buckets drop requests before any ML-DSA-65 work;
   `SigRefRateLimiter` caps how many SigRefs can ever enter the map; `Invalid` entries are never
   evicted by the flood. The remaining cost is the attacker's own packets being dropped, which is
   the goal. In OPTIMISTIC the flood passes; that is why OPTIMISTIC is a soak mode, not a
   destination.
4. **Time-of-check gaps.** (a) Between the verifier's verdict and the map write: a packet in that
   window is `NeedUserspace`, dropped in PESSIMISTIC, passed in OPTIMISTIC; bounded by the
   verifier's write latency, measured in the sprint. (b) Key revocation to cache rewrite: the
   revocation cascade rewrites every SigRef under the key to `Invalid` in one batch update before it
   publishes `pqc.key.revoked`; until that batch lands, Valid entries pass for at most their
   remaining lifetime (300 s). (c) Clock: `expires_at` is unix seconds from the verifier against
   `bpf_ktime_get_boot_ns`-derived time in XDP; the sprint uses one clock (the verifier writes
   `verifier_alive` from the same source XDP compares against) or the comparison is wrong.
   `services/shield/pqc_security.go:95-96` already names a TOCTOU cache for the userspace side.
5. **Downgrade.** A peer that stops setting S and CUSTOM falls into NotPqc and passes. Only open
   question 4 (require PQC per boundary) closes this.

### D7. What the Lich campaign must cover

A new campaign, `tomb/lich/LICH-018-pqc-fast-path/`, in the LICH-012 shape (sub-campaigns, a verdict
table, a signed "no exploit" line where nothing is found;
`tomb/lich/LICH-012-config-convergence/README.md:1-50`):

- **L18a replay:** capture one valid Monad on `wg0` in the lab netns, replay its SigRef from another
  source prefix; expected: verifier flips it Invalid within one write latency; measure the window.
- **L18b poison:** attempt a map write from a non-verifier uid (pin permissions), from inside XDP
  (there is no code path; confirm by reading the object), and via a forged `pqc.sig.announce`
  without a `wotansig` signature (Wotan must refuse).
- **L18c flood:** 1M packets per second of fresh SigRefs through `scripts/stress-cannon.py` with the
  CUSTOM bit set; expected: `pqc_ring_full` climbs, verifier CPU stays under its unit ceiling
  (`MemoryMax`, `CPUQuota`), no `Valid` entry appears, the existing Doom and trace rings are
  unaffected.
- **L18d liveness:** `systemctl kill` the verifier in PESSIMISTIC; expected: CachedValid still
  passes, NeedUserspace drops, the ADR-116 expectation pages within 10 s; then restart and confirm
  recovery without a Shield reload.
- **L18e verifier bombs:** a fuzz harness over the 48-byte ring record and over `PqcSigStatusEntry`
  bytes (status values 5-255, expires_at 0 and 0xFFFFFFFF, reserved non-zero) against the XDP state
  table, as a `tomb/lich/harnesses/` target like `fuzz_mbc_execute`.
- **L18f budget:** load the object on both feature variants on west and east kernels; a load failure
  or a count over the recorded baseline row is a finding.

### D8. How it lands in stages behind a cargo feature

- **Stage 0 (this ADR).** Nothing in the tree. `ebpf/pqc-common` stays as it is.
- **Stage 1: types and gate, no map.** `PqcSigStatusEntry` and the status enum return to
  `ebpf/pqc-common` under feature `pqc-fast-path`; the Go mirror and `rust_source_test` row land in
  `bpfschema`; `check-ebpf-insns.sh` gains a `shield_xdp pqc-fast-path` variant row. The default
  build of `shield-ebpf` has no new map, no new ring and the same 1,502 count. Gate: both variants
  load on west; the variant count is recorded.
- **Stage 2: both ends, OPTIMISTIC, lab only.** The maps, the step-4 check, `cmd/pqc-verifier` as
  the consumer, the loader writing `SHIELD_CONFIG[1]=1`, the signed announce topic, metrics, the
  ADR-116 expectations and the three runbooks (verifier dead, cached-invalid non-zero, ring full).
  Runs in `scripts/lab-two-node.sh` namespaces. Gate: a signed SigRef goes Pending to Valid, a
  tampered one goes Invalid, counters match the packet counts, LICH-018 a-f run with no open
  finding.
- **Stage 3: OPTIMISTIC on the live east-west link, 14-day soak.** Feature on in the build that
  ships to both hosts; mode OPTIMISTIC in config. Gate: the three counters at zero for the whole
  soak, the daily ADR-116 line names the verifier.
- **Stage 4: PESSIMISTIC on one boundary.** Config change only, east first (the staging host), with
  the chaos-guard style revert path (`ExecStopPost` sets mode OFF). Gate: no drop of legitimate
  traffic in the Steel Echo probes for 7 days.

Anything that fails a gate rolls back one stage by config, not by code.

## Alternatives

**A. Never verify at XDP; verify only at the service.** Every service already authenticates callers
with Ed25519 service tokens over hybrid-KEM TLS (ADR-103, ADR-105), and ADR-105 lines 22-27 records
that authentication "stays classical" and Layer 1 is out of scope. Under this option the Shield
never learns a signature verdict, the drafts' Layer 1 stays a design, and DP-204 A is the end of the
story. Cost: nothing. What it gives up: dropping known-bad Kingdom-to-Kingdom traffic before the
stack touches it, and the only place a per-packet PQC claim could ever be true. Honest reading: this
is the right answer until the Kingdom has a second peer Shield that is not reachable over `wg0`;
today's single tunnel already authenticates the peer with a Noise handshake plus a rotated PSK.

**B. Verify in the TC hook instead of XDP.** `shield_tc` already loads the Monad from the skb
(`main.rs:497-509`) and has `ctx.load`, so the SigRef read is simpler and the verifier budget is
already paid for the parse. But TC egress runs on packets *leaving* the Kingdom, which are the ones
we stamped ourselves; the verdict matters on ingress. A TC *ingress* classifier would see the same
step-4 packets after XDP and after the kernel allocated an skb, so a drop there costs more per
packet than at XDP and the DoS argument in D6.3 weakens. Keep B as the fallback if the XDP budget
surprises us (D5), and note that `bpf_loop` for the deferred HMAC/retry/hop checks (FND-025) is a
TC-shaped problem too.

**C. A per-flow session token after one userspace verification.** Instead of caching a verdict per
SigRef, the verifier marks the *flow* (src /64, dst /64, src port, dst port, next header) as
verified in a `PQC_FLOW_OK` map with its own expiry, and XDP checks the flow key, not the SigRef.
This closes D6.1 (a replayed SigRef from another flow is not in the map) and matches what
`services/shield/pqc_verify.go:269-278` already keeps per flow in userspace. Costs: a 5-tuple parse
at XDP that step 4 does not do today (inferred 100-200 more verified instructions), a bigger map
(flows outnumber SigRefs), and the flow label cannot be the key (D-context fact 2). It also changes
what the cache *means*: "this flow was verified once" rather than "this signature is good", which is
closer to a session than to signature-by-reference and is what the drafts' Wotan per-flow state
(PQC-00 §12.1) describes anyway. Honest comparison: C is the stronger control against replay and the
weaker match to the drafts' wording; the per-SigRef design above is the direct successor of the
removed code. The brainstorm should pick one or layer them (SigRef verdict gates the first packet,
flow token gates the rest). The recommendation here is to prototype C's flow key in stage 2 behind
the same feature and measure both before stage 3, because the difference is one map and one parse,
and the replay gap is the threat the drafts care most about.

## Open questions for the brainstorm

1. **Wire format.** The 12-byte PQC value cannot sit in a 20-byte v0x01 Monad (only the 32-bit
   SigRef fits in `scratch[0..4]`), and PQC-00 §5 (24-bit SigRef) disagrees with Foundation-06 and
   `pqc-common` (32-bit). Does the sprint ask the RFC Editor for a PQC-00 revision that keys Layer 1
   on SigRef alone, or does the wire freeze stay and this ADR's "SigRef only" becomes the spec?
2. **Which model does Shield follow (E8)?** This ADR keys on step 4 (arriving Kingdom packets) and
   leaves Shadow ingress untouched. Is that the E8 answer for the PQC check, or does E8 need the
   general answer first?
3. **Per-SigRef verdict, per-flow token, or both** (alternative C)? And if both, is the first packet
   of a flow dropped in PESSIMISTIC acceptable, or must stage 4 wait for a buffering design?
4. **Require PQC per boundary?** Should a boundary in PESSIMISTIC also drop Monads with S and CUSTOM
   *unset* (closing D6.5), and is that a per-peer setting in `shield.yaml`?
5. **Default mode and the draft's MUST.** Is "absent key means OFF, config always writes an explicit
   mode" an acceptable reading of PQC-00 §9.3, or does the draft get a sentence?
6. **keymgmt's fate (DP-203).** Delete all eight planes and rebuild the lifecycle inside
   `cmd/pqc-verifier` from `trust.d` plus a revocation file, or keep `services/keymgmt` as the
   control plane with its own uid and port 19007? The design above works either way; the second adds
   a service to run and monitor.
7. **Where the full signature lives.** Sophia `PQC_SIG_MAP` as a kernel map (Sophia-03 `:698-720`,
   65,536 entries of multi-kilobyte ML-DSA-65 signatures is tens of MB of kernel memory) or a
   userspace store the verifier reads (`pkg/sophia/pqc_control.go` is already a mirror)? Nothing in
   XDP needs the signature bytes, so the kernel map may be the wrong place.
8. **Soak numbers.** 14 days OPTIMISTIC, 300 s Valid lifetime, 10 s verifier timeout, 256 KiB ring,
   200/5,000 requests per second: accept as defaults to tune from, or set them from a measurement
   first?

## Note 2026-11-04 (DP-715, TRUE-WIRE L5)

`cmd/pqc-verifier` was deleted with the other TRUE-WIRE defaults. D1's "revive" now means restoring it from git (last commit carrying it: the parent of the S-WIRE-HONEST deletion). Port 19008 stays reserved in `pkg/ports`; `services/shield` stays until TRUE-WIRE L6 is answered.

## Decision

None yet. Parked under ADR-112. The 2026-10-28 removal stands; no map, feature, branch or code
exists for this design until the freeze lifts and this ADR competes for a plan slot.
