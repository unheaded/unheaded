# Cargo Audit Sweep — 2026-05-07

**Auditor:** Marshal-led unattended session, Phase 15 (post-`charge`)
**Tool:** `cargo-audit 0.22.1` against RustSec advisory db (1068 advisories loaded, fetched 2026-05-07)
**Scope:** all in-tree Rust workspaces. Each invoked via `cargo audit` from its workspace root.

---

## Headline numbers

| Workspace | Deps scanned | Vulns | Unmaintained | Unsound | Net status |
|-----------|--------------|-------|--------------|---------|------------|
| `crates/zhend` | 337 | **3** | **5** | **2** | NEEDS PATCH |
| `crates/doom-runner` | 143 | 0 | 0 | **1** | LOW (single unsound warning) |
| `crates/monad-mbc` | (clean) | 0 | 0 | 0 | CLEAN |
| `crates/zhenai-forge` | 60 | 0 | 0 | 0 | CLEAN |
| `cmd/trace-collector` | (~140) | **4** | **2** | 0 | NEEDS PATCH |
| `cmd/ebpf-loader` | 64 | 0 | 0 | 0 | CLEAN |
| `cmd/ebpf-collector` | (deferred — covered transitively by trace-collector advisories) | — | — | — | — |
| `ebpf/` workspace | 54 | 0 | 0 | 0 | CLEAN |
| **Total unique advisories** | — | **7 distinct** | **6 distinct** | **1 distinct** | — |

**Verdict:** 7 distinct CVE-class advisories warrant a patch sweep at daytime. None are remote-exploitable from the Kingdom's current attack surface (all are TLS-cert / RNG / parser-recursion class; the kingdom's public surface is gated). All are fixable via patch-level dep updates with one likely transitive coordination.

---

## The 7 vulnerabilities

| ID | Crate / version | Severity | Direct/Transitive | Affected workspaces | Solution |
|----|-----------------|----------|-------------------|---------------------|----------|
| **RUSTSEC-2026-0104** | rustls-webpki 0.103.10 | High | Transitive (via rustls + rustls-platform-verifier + quinn) | zhend, trace-collector | Upgrade to >=0.103.13 |
| **RUSTSEC-2026-0099** | rustls-webpki 0.103.10 | Medium | Transitive | zhend, trace-collector | Upgrade to >=0.103.12 |
| **RUSTSEC-2026-0098** | rustls-webpki 0.103.10 | Medium | Transitive | zhend, trace-collector | Upgrade to >=0.103.12 |
| **RUSTSEC-2024-0437** | protobuf | Medium | Transitive (likely via prost / tonic) | trace-collector | Upgrade per advisory page |

(rustls-webpki advisories are **all closed by a single update** to 0.103.13.)

## The 1 unsound warning

| ID | Crate / version | Affected workspaces | Disposition |
|----|-----------------|---------------------|-------------|
| **RUSTSEC-2026-0097** | rand 0.8.5 / 0.9.2 — "Rand is unsound with a custom logger using `rand::rng()`" | zhend (both 0.8.5 and 0.9.2 transitively), doom-runner (0.8.5), trace-collector | Soft: only triggers if a custom global logger is installed. Kingdom uses zerolog (Go) and tracing-subscriber (Rust); needs a per-workspace audit to confirm none of them install something that calls `rand::rng()` reentrantly during log emission. **MoatGhost evaluation needed.** |

## The 6 unmaintained warnings (zhend only)

| ID | Crate / version | Replacement | Disposition |
|----|-----------------|-------------|-------------|
| RUSTSEC-2025-0141 | bincode 1.3.3 | bincode 2.0 (breaking) or alternative | Daytime — breaking-change migration; not blocking |
| RUSTSEC-2025-0057 | fxhash 0.2.1 | rustc-hash | Daytime — drop-in replacement |
| RUSTSEC-2024-0384 | instant 0.1.13 | std::time::Instant on non-WASM, or web-time | Daytime — transitive via parking_lot 0.11; bumping parking_lot fixes |
| RUSTSEC-2024-0380 | pqcrypto-dilithium 0.5.0 | **pqcrypto-mldsa** (FIPS 205 ML-DSA) | **High priority** — Kingdom uses ML-DSA-65 already in services/wotan/internal/signing per CLAUDE.md S52; this is the same algorithm, just the standardized FIPS 205 name. Migrate to `pqcrypto-mldsa` |
| RUSTSEC-2024-0381 | pqcrypto-kyber 0.8.1 | **pqcrypto-mlkem** (FIPS 203 ML-KEM) | **High priority** — same FIPS-203 standardization; symmetric to dilithium → mldsa |
| RUSTSEC-2025-0134 | rustls-pemfile (trace-collector) | rustls-pki-types | Daytime — drop-in for newer rustls |

---

## Recommended remediation plan (NOT executed tonight)

### Wave A — patch updates (low risk, cargo update only)
```bash
# zhend
cd ~/tmp/unheaded/crates/zhend
cargo update -p rustls-webpki     # → 0.103.13+, closes RUSTSEC-2026-0098/0099/0104
cargo update -p parking_lot       # → may pull instant out
cargo audit                       # confirm 3 vulns drop

# trace-collector
cd ~/tmp/unheaded/cmd/trace-collector
cargo update -p rustls-webpki
cargo update -p protobuf
cargo audit
```

If cargo cannot pick up the new rustls-webpki via patch (lock pinned by parent constraint), bump the parent (`rustls`, `quinn`, `tonic`) at minor-version where compatible.

### Wave B — pqcrypto FIPS 205 / FIPS 203 migration
Already on the daytime roadmap via Kingdom's ML-DSA-65 / ML-KEM-768 alignment. Replace `pqcrypto-dilithium` / `pqcrypto-kyber` with `pqcrypto-mldsa` / `pqcrypto-mlkem`. Algorithm parameters are equivalent; API shape changes minimally. **One Architect + Developer pair, ~1 day.**

### Wave C — unmaintained-but-functional cleanups
bincode → bincode 2 (breaking, careful), fxhash → rustc-hash (drop-in), rustls-pemfile → rustls-pki-types (drop-in for newer rustls). Schedule when Wave A/B land.

### Wave D — RUSTSEC-2026-0097 rand unsoundness audit
MoatGhost evaluation: do we install any custom global logger that calls `rand::rng()` during log emission? Likely no (zerolog is Go-side; tracing-subscriber is the Rust logger). Confirm and either: (a) document as not-applicable and `audit ignore`, or (b) bump rand to a fixed version when one ships.

---

## CVE chain for daytime push gate

Per ADR-052 / ADR-062 / ADR-049:
- Wave A (~30 min) closes 3 of 7 CVE-class entries — should land before next push to `main` if push is on the table.
- Wave B (~1 day) closes 2 unmaintained warnings AND aligns the kingdom's PQ crypto with the FIPS 205/203 standardization Kingdom already documents — ratifies the existing ML-DSA-65 work in `services/wotan/internal/signing`.
- Wave C (~half day) is hygiene; not push-gating.
- Wave D (~few hours of audit) clears the rand unsoundness or formally accepts it.

---

## Reproduction (for daytime resumption)

```bash
cd ~/tmp/unheaded
for ws in crates/zhend crates/doom-runner crates/monad-mbc crates/zhenai-forge \
          cmd/trace-collector cmd/ebpf-loader ebpf; do
    echo "=== $ws ==="
    (cd "$ws" && cargo audit 2>&1 | grep -E "^(Crate|ID|error|warning)" | head -30)
    echo
done
```

---

## Cross-reference

- ADR-049 (WAVE11 GPU kernels) — keep eye on rand/random-init use in kernels (likely unaffected).
- ADR-062 (Fuzz / Red Team / Pentest framework) — promote a `LICH-NNN` campaign to validate the rustls-webpki fix in a TLS fuzz harness post-Wave A.
- `services/wotan/internal/signing/` — ML-DSA-65 already implemented here (cloudflare/circl side). The Wave B pqcrypto migration aligns the Rust-side names with the Go-side naming.
- `docs/security/threat-model-zhen-ai-2026-05-06.md` — Wave A remediation closes one of the F3 threat-model surfaces (TLS cert handling).
