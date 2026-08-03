# Shield eBPF: the PQC authentication fast-path is not wired up (2026-08-03)

**Component:** `ebpf/shield-ebpf` (THE SHIELD, XDP data plane)
**Severity:** Medium — a declared enforcement path that never executes
**Found by:** clippy `dead_code`, while working the warning ratchet down from 348
**Status:** NOT fixed — needs a decision, see below

## What is there

`ebpf/shield-ebpf/src/main.rs:873-914` declares a complete post-quantum
signature fast-path for the XDP layer:

- `PQC_SIG_STATUS` — a `HashMap<u32, PqcSigStatusEntry>` BPF map, 65536 entries,
  keyed by SigRef.
- `pqc_fast_path_check(flags, sig_ref) -> PqcFastPathResult` — checks whether a
  packet carries PQC flags (`S|CUSTOM`) and, if so, consults the cached
  verification result.
- `PqcFastPathResult` — `NotPqc` / `CachedValid` / `CachedInvalid` /
  `NeedUserspace`, where `CachedInvalid` is documented as
  *"drop packet (PESSIMISTIC) or warn (OPTIMISTIC)"*.

## What is missing

Both ends. This is not a case of "written ahead of its caller":

1. **No consumer.** `pqc_fast_path_check` is never called. The XDP entry point
   does not consult it, so no packet is ever classified, and in particular no
   packet is ever dropped for carrying a signature already known to be invalid.
2. **No producer.** A tree-wide grep for `PQC_SIG_STATUS` returns only the
   declaration and the read inside the dead function. Nothing in userspace —
   no loader, no Go service, no Rust binary — ever writes an entry. Even if the
   check were called, every lookup would miss and return `NeedUserspace`.

The only thing that kept this visible at all was a `dead_code` warning in a job
that was non-gating until recently.

## Why it matters

The risk is not that packets are wrongly dropped — nothing is dropped. It is
the belief that XDP-layer PQC enforcement exists. `PqcSigStatus::Invalid` and
`::Expired` both map to `CachedInvalid`, which reads as "the data plane rejects
packets whose signatures failed or expired." It does not. Anything relying on
that property — a threat model, an ADR, a compliance claim about
authentication at the packet layer — is relying on code that has never run.

Same class as the WAF inert config toggles in
`waf-inert-config-toggles-2026-07-31.md`: a control that reads as configured
but cannot fire. The difference is that the WAF toggles failed in the safe
direction (protection stayed *on*); this one is a protection that was never on.

## Not fixed on purpose

Wiring this up is not a cleanup. It means:

- calling the check on the XDP hot path, which changes the verifier
  instruction budget for `shield-ebpf` (the repo gates this via
  `scripts/bpf-verifier-check.sh`);
- choosing PESSIMISTIC vs OPTIMISTIC on `CachedInvalid` — i.e. deciding whether
  the Shield starts dropping production traffic on a cache hit;
- building the userspace side that populates `PQC_SIG_STATUS`, including entry
  expiry and eviction under a 65536-entry cap.

That is a feature with a blast radius, not a warning fix, and the choice of
drop-vs-warn is Stevie's. Left annotated in place.

## Options

1. **Wire it, OPTIMISTIC first** — call the check, log/count `CachedInvalid`
   without dropping, and confirm the counter stays at zero in normal traffic
   before switching to PESSIMISTIC. Safest path to a real control.
2. **Delete it** — if PQC verification is intended to live entirely in
   userspace, remove the map, the function, and the enum so nothing claims an
   XDP-layer guarantee that is not there.
3. **Leave and document** — current state, acceptable only while the Shield is
   not carrying real traffic.

Option 3 stops being acceptable at the same moment as the credential baseline
in CLAUDE.md: when this is exposed to the public internet.
