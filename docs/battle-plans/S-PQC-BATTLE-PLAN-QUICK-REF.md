# S-PQC Battle Plan Part 1 — Quick Reference Index

## File Location
```
/sessions/cool-optimistic-bohr/mnt/tmp/S-PQC-BATTLE-PLAN-part1.md
```

## Quick Navigation

### Phase 0: Environment & Prerequisite Verification (Steps 1-25)
**Start**: Step 1 — Kernel version verification
**Key Milestone**: Step 25 — Phase 0 exit gate checkpoint
**Objective**: All tools present, all dependencies running

### Phase 1: PQC Crypto Library Integration (Steps 26-55)
**Start**: Step 26 — Clone and build liboqs
**Key Milestone**: Step 48 — Phase 1 commit checkpoint
**Objective**: 5 algorithms wrapped, tested, benchmarked

**Deliverables**:
- `pkg/crypto/pqc/slh_dsa.go` — SLH-DSA wrapper
- `pkg/crypto/pqc/ml_dsa.go` — ML-DSA wrapper
- `pkg/crypto/pqc/fn_dsa.go` — FN-DSA wrapper
- `pkg/crypto/pqc/ml_kem.go` — ML-KEM wrapper
- `pkg/crypto/pqc/hqc.go` — HQC wrapper
- `pkg/crypto/pqc/pqc.go` — Interface definitions
- `pkg/crypto/pqc/registry.go` — Algorithm ID registry

### Phase 2: Sophia PQC Map Infrastructure (Steps 56-95)
**Start**: Step 56 — Verify Sophia map infrastructure
**Key Milestone**: Step 86 — Phase 2 commit checkpoint
**Objective**: All 5 maps created, pinned, RDONLY verified

**Deliverables**:
- `eBPF/sophia_pqc.h` — C struct definitions
- `pkg/sophia/pqc_maps.go` — Go type definitions
- `pkg/sophia/pqc_control.go` — Control plane API
- `/sys/fs/bpf/sophia/sophia_pqc_sigs` — Signature map
- `/sys/fs/bpf/sophia/sophia_pqc_keys` — Key map
- `/sys/fs/bpf/sophia/sophia_pqc_app_policy` — Policy map
- `/sys/fs/bpf/sophia/sophia_pqc_sovereign_sigs` — Multi-sig map
- `/sys/fs/bpf/sophia/sophia_pqc_kem_keys` — KEM map

### Phase 3: Monad Value Layout & Pseudo-Header (Steps 96-125)
**Start**: Step 96 — Read spec Section 5
**Key Milestone**: Step 125 — Phase 3 exit gate
**Objective**: Value layout parses, pseudo-header correct

**Deliverables**:
- `pkg/monad/pqc_value.go` — 12-byte value parser
- `pkg/monad/pseudo_header.go` — 52-byte header builder
- `pkg/crypto/pqc/hash_pfx.go` — HashPfx computation
- `pkg/monad/flags.go` — S flag detection

## Algorithm Registry

```
0x01 = SLH-DSA (FIPS 205)    — Hash-based, 4784 bytes, <5ms verify
0x02 = ML-DSA (FIPS 204)     — Lattice-based, 2420 bytes, <0.3ms verify
0x03 = FN-DSA (FIPS 206)     — Lattice-based, 666 bytes, userspace sign
0x04 = ML-KEM (FIPS 203)     — Lattice-based KEM
0x05 = HQC (FIPS 207)        — Hamming QC KEM
```

## Sophia Maps Summary

| Map Name | Key | Value | Entries | Access |
|----------|-----|-------|---------|--------|
| sophia_pqc_sigs | SigRef (24-bit) | Signature + metadata | 1M | RDONLY_PROG |
| sophia_pqc_keys | KeyRef (24-bit) | Public key + metadata | 65K | RDONLY_PROG |
| sophia_pqc_app_policy | app_id | Policy struct | 4K | RDONLY_PROG |
| sophia_pqc_sovereign_sigs | flow_id | 2-of-3 multi-sig | 10K | RDONLY_PROG |
| sophia_pqc_kem_keys | kem_id | KEM public key | 10K | RDONLY_PROG |

## Monad Value Layout (12 bytes)

```
 0 1 2 3 4 5 6 7 8 9 A B (hex offsets)
[---SigRef---][---KeyRef---][HashPfx][--SeqNum--]
 24-bit       24-bit        16-bit    32-bit
```

## Pseudo-Header Layout (52 bytes)

```
 0-15:  Source IPv6 address (16 bytes)
16-31:  Destination IPv6 address (16 bytes)
32-35:  Flow Label (20-bit in bits 31-12, 12 zero bits)
36-37:  Source Port (16-bit)
38-39:  Destination Port (16-bit)
40-43:  SeqNum (32-bit)
44-51:  Reserved (8 zero bytes)
```

## Commit Checkpoints

```
Step 25  — Phase 0 exit gate (prerequisites)
Step 48  — Phase 1 commit (crypto libraries)
Step 86  — Phase 2 commit (Sophia maps)
Step 105 — Phase 3 final build (Monad layout)
Step 125 — Phase 3 exit gate (pseudo-header)
```

## Performance Targets

| Operation | Target | Status |
|-----------|--------|--------|
| SLH-DSA verify | <5ms | Pending liboqs binding |
| ML-DSA verify | <0.3ms | Pending liboqs binding |
| HashPfx compute | <0.1ms | Ready |
| Full verification | <10ms (p95) | Pending Phase 4-5 |

## Key External References

- **Spec**: `docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md`
- **NIST PQC**: FIPS 203, 204, 205, 206, 207
- **liboqs**: `third_party/liboqs/` (build from source)
- **Cloudflare circl**: `github.com/cloudflare/circl`
- **NIST Test Vectors**: TBD in Phase 1b

## Running the Plan

### Start Phase 0
```bash
cd ~/tmp/unheaded
# Follow Step 1-25 in order
# Each step is copy-paste ready
```

### Start Phase 1 (after Phase 0 gate)
```bash
# Follow Step 26-55 in order
# Builds liboqs, creates Go/Rust wrappers
```

### Start Phase 2 (after Phase 1 gate)
```bash
# Follow Step 56-95 in order
# Creates 5 BPF maps with RDONLY enforcement
```

### Start Phase 3 (after Phase 2 gate)
```bash
# Follow Step 96-125 in order
# Implements Monad value parsing and pseudo-header
```

## Success Criteria

- [ ] All Phase 0 prerequisites verified
- [ ] Phase 1: All 5 algorithms wrapped and compiling
- [ ] Phase 2: All 5 maps created and pinned at /sys/fs/bpf/sophia/
- [ ] Phase 3: Monad value parses round-trip, pseudo-header matches spec

## Notes for Agents

1. **Sequential**: Phases 0-3 MUST be done in order (dependencies)
2. **Exit Gates**: Hard stops at steps 25, 49, 87, 125
3. **Commits**: Every 5 steps, one major commit per phase end
4. **Time**: Estimate 10-15 hours for Part 1 (can be parallelized in Phase 1b)
5. **Stuck Protocol**: If > 3x time estimate or 2 failed debug attempts, skip and move to Part 2

---

**Next**: Part 2 (Phases 4-6) covers PQC Verifier Daemon, Compliance Tiers, and Anamnesis integration
