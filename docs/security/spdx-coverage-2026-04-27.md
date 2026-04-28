# SPDX Coverage Snapshot — 2026-04-27

**Run from**: Cowork-on-Macbook (Linux sandbox, pure grep)
**Source-of-truth ADR**: ADR-052 (drift policy) + ADR-INDEX (license tracking)
**Project license**: GPL-3.0-or-later (per LICENSE file)

## Summary

| Language | Total files | Files missing SPDX | Coverage % |
|---|---:|---:|---:|
| Go | 1,162 | **6** | 99.48% |
| Rust | 208 | **156** | 25.0% |

## Go gap (6 files — small, mechanical fix)

```
./cmd/routing-health/main_extended_test.go
./cmd/routing-health/main.go
./cmd/routing-health/main_test.go
./cmd/test_batch/main.go
./services/wotan/proto/topic.pb.go
./<one more — full list at /tmp/spdx-missing-go.txt during the session>
```

**Disposition**:
- Files in `cmd/routing-health/` and `cmd/test_batch/` likely added since the last SPDX sweep — backfill in `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md` Section D.
- `services/wotan/proto/topic.pb.go` is **auto-generated** from `.proto`. SPDX should be added to the `.proto` file's `option go_package` block, or via post-gen sed in CI. Direct edit may break protoc round-trip.
- Estimated fix wall-clock: 5 minutes once on a Linux box.

## Rust gap (156 files — 75% missing, separate sprint)

Sample missing files:
```
./cmd/ebpf-collector/collector/build.rs
./cmd/ebpf-collector/collector/src/main.rs
./cmd/ebpf-collector/ebpf-programs/src/packet_counter.rs
./cmd/ebpf-collector/ebpf-programs/src/tcp_latency.rs
./cmd/ebpf-collector/common/src/lib.rs
… 151 more
```

**Disposition**:
- Significant gap. Per CLAUDE.md S52 ("SPDX headers on 838 Go files, GPL boundary documented"), **Rust SPDX backfill was deferred** at the time of S52.
- Now that the codebase has 208 Rust files (vs. ~50–100 at S52), the gap is large enough to need its own sprint.
- Recommended approach:
  1. Author script `scripts/spdx-add-rust.sh` (mirror of any existing Go variant)
  2. Dry-run on the 156 files; manually inspect 5–10 for header placement (top of file, before any `//!` doc comment)
  3. Apply, run `cargo build --workspace` to verify nothing breaks
  4. Commit as a single `chore(legal): backfill SPDX headers on 156 Rust files (GPL-3.0-or-later)` patch
- Estimated wall-clock: 1–2 hours focused work.
- **Lane I addition**: append to `battle-plan.md` Lane I as I6 (or fold into Track decision propagation).

## CI implications

The Jenkinsfile `Static Analysis → SPDX Headers` stage currently checks **Go only** (per the sed/find at line 95). To enforce Rust SPDX in CI:
- Extend the existing stage with a Rust grep block, OR
- Add a sibling `Rust SPDX Headers` stage in the parallel block.

Defer this CI extension until the 156-file backfill is complete (else CI is permanently red).

## Cross-references

- ADR-052 (drift policy) — generally tracks freshness, not coverage; this snapshot is informational.
- COMPLIANCE-REMOTE-PACKET-2026-04-27.md Section D — Go backfill commands.
- `THIRD_PARTY.md` — license inventory for upstream deps (separate from our own SPDX headers).

---

*SPDX coverage snapshot forged 2026-04-27 from Cowork-on-Macbook. Go gap is small (6); Rust gap is large (156) and gets its own sprint slot.*
