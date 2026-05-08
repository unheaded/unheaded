# 2026-05-12 — Tuesday

## Scheduled

### Round Table: Carry-Over Architectural Decisions (~30 min)

**Convene:** Stevie + unheaded-architect (+ unheaded-developer in advisory).
**Trigger:** Marshal drain shift 2026-05-08 (per `references/marshal-parked-2026-05-07.md` carry-forward + Stevie's 16 rapid-fire decisions). Stevie chose "Schedule a 30-min Round Table" disposition.
**Goal:** Pick definitive architectural directions on 5 carry-over items so each can land in the next code-touching sprint.

#### Agenda

1. **C4 / heimdall-daemon TODO line 72 — GungnirSeal verify before trusting manifest.**
   - API exists (`gungnir.Verify(payload, seal, pubKey)` in `pkg/gungnir/gungnir.go`).
   - Decision needed: where does the trusted root pubkey come from? Pinned in CLAUDE.md? In `pkg/auth/wg-peer-registry.json`? Per-host SOPS file?
   - Owner: Stevie (key-mgmt UX) + Architect.

2. **C4 / heimdall-daemon TODO line 135 — Wotan ML-DSA-65 signing for `drift.*` topics.**
   - Today: `config.*` topics are signed (`services/wotan/internal/signing/`). `drift.*` is not.
   - Decision needed: extend `config.*` enforcement to `drift.*`, or define a new signing class with its own key/policy?
   - Owner: Architect + Developer.

3. **C4 / heimdall-daemon TODO lines 147 + 148 — BPF ringbuf reader + Gjallarhorn XDP listener.**
   - Both are Linux-only Aya kernel-side scaffolding work.
   - Decision needed: pair them in a single bare-metal session on WEST, gated on the Round Table outcome from #1 and #2 (since the listener will publish signed events).
   - Owner: Developer (with bare-metal Linux for verification).

4. **D5 / `crates/zhend/src/pu/codec.rs` `encode_for_gossip` wire-format versioning.**
   - Today: stub calls `encode()` verbatim (no-op for backward compat).
   - Decision needed: which Fragment fields are "local-only" (currently noted: `access_count`, `tier`)? How is the versioning byte encoded — as a leading byte? Or via a separate `encode_for_gossip_v1`?
   - Owner: Architect + Developer.

5. **D6 / `crates/doom-runner/src/main.rs:624` `RingStatus` struct shape.**
   - Today: the `ring::status` action prints "TODO" without reading any actual state.
   - Decision needed: what fields belong on `RingStatus`? Per-hop state? Packet counts? Last error? Reading from pinned eBPF maps requires the map shape decision.
   - Owner: Developer (with bare-metal Linux for verification).

#### Out of scope

- **D4 / zhend pilgrimage roadmap** — flagged 2026-05-06 as "intentional design intent, leave as-is per design." Do not relitigate; sequence when Architect is ready.

#### Output expectations

- 4-6 ADR drafts (or definitive defer decisions) so the next code-touching sprint can pick them up without re-asking.
- Each output ADR includes: problem statement, options considered, chosen option, smoke-test recipe.

#### Pre-reads (15 min before round table)

- `references/marshal-parked-2026-05-06.md` — D4/D5/D6 entries (line 224-232).
- `references/marshal-parked-2026-05-07.md` — carry-over section (line 222-234).
- `references/battle-plan-marshal-drain-2026-05-08.md` — context for why this moment.

---

## References

- `cmd/heimdall-daemon/main.go` lines 72, 135, 147, 148 (the 4 TODOs).
- `crates/zhend/src/pu/codec.rs` (D5).
- `crates/doom-runner/src/main.rs:624` (D6).
- `pkg/gungnir/gungnir.go` (existing seal-verify API for #1).
- `services/wotan/internal/signing/` (existing ML-DSA-65 enforcement for #2).
