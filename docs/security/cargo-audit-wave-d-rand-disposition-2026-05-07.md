# Cargo Audit Wave D — RUSTSEC-2026-0097 (rand) Disposition

**Auditor:** Marshal-led re-engagement Phase 21
**Advisory:** [RUSTSEC-2026-0097](https://rustsec.org/advisories/RUSTSEC-2026-0097) — *"Rand is unsound with a custom logger using `rand::rng()`"*
**Crates flagged:** `rand 0.8.5` (transitive via tonic 0.10 + tungstenite); `rand 0.9.2` (transitive via quinn-proto, proptest, fastbloom)
**Disposition:** **NOT APPLICABLE** to this codebase — recommend `cargo audit ignore`.

---

## The advisory in one sentence

`rand::rng()` (the top-level function introduced in `rand 0.9`) is reentrancy-unsound when combined with a *custom global logger* that itself calls `rand::rng()` during a log-emission path. The unsoundness is a TLS / thread-local-storage interaction.

## Audit findings against this codebase

### Q1: Does the codebase call `rand::rng()` directly?

```bash
grep -rn "rand::rng()" --include="*.rs"
# (zero hits)
```

**No.** The only randomness call sites in our Rust code use the older `rand::thread_rng()` API:

| File | Line | Context |
|------|------|---------|
| `crates/zhend/src/crypto/envelope.rs` | 68 | `rand::thread_rng().fill_bytes(&mut nonce_bytes);` (XChaCha nonce gen) |
| `crates/zhend/src/crypto/kem.rs` | 47 | `x25519_dalek::StaticSecret::random_from_rng(rand::thread_rng());` |
| `crates/zhend/src/crypto/kem.rs` | 119 | same pattern (ephemeral secret) |

All three are deliberate cryptographic randomness, sourced in non-logger paths. None of them is on a log-emission code path.

### Q2: Does the codebase install a custom global logger?

The Rust crates initialize logging via standard frameworks:

```bash
grep -rln "tracing_subscriber\|env_logger\|simplelog" --include="*.rs"
# cmd/ebpf-collector/collector/src/main.rs
# cmd/trace-collector/src/main.rs
# cmd/ebpf-loader/src/main.rs
# crates/zhend/src/main.rs
# crates/doom-runner/src/main.rs
```

All five entry points use **`tracing_subscriber`** (the standard library-blessed logger), with no custom emission hooks. `tracing_subscriber` does not call `rand::rng()` during emission — it serializes `tracing::Event`s using its own formatter pipeline, with no randomness involved.

### Q3: Do any transitive deps install a custom logger that calls `rand::rng()`?

The deps that bring in `rand 0.9.2` (quinn-proto, proptest, fastbloom) are themselves libraries that consume randomness, not loggers that produce log lines. None of them register a global logger.

The deps that bring in `rand 0.8.5` (tungstenite, tonic) — same: neither is a logger.

## Conclusion

**The unsoundness condition (custom logger calling `rand::rng()` reentrantly during emission) is structurally impossible in this codebase.** The two preconditions are not met:

1. We do not call `rand::rng()` (only the older `thread_rng()`).
2. We do not install a custom global logger that calls rand.

## Recommended ignore

Add to each affected workspace's `audit.toml` (or pass via `cargo audit --ignore`):

```toml
[advisories]
ignore = [
    "RUSTSEC-2026-0097",  # rand unsoundness — N/A: codebase does not call rand::rng() and uses standard tracing_subscriber logger
]
```

Or, lighter-weight, cite this disposition doc in any future audit-status review and re-evaluate when one of these changes:
- We adopt a custom global logger.
- We migrate from `rand::thread_rng()` to `rand::rng()`.
- A patched rand version ships and the advisory is closed upstream.

## Compliance audit trail

This disposition should be cross-referenced from `docs/security/cargo-audit-2026-05-07.md` Wave D row when daytime ratifies it. Marshal recommends adoption: ~10 minutes to add the ignore config and re-run `cargo audit` to confirm clean exit on the affected workspaces.
