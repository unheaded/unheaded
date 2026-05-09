#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# ascend-linux-smoke.sh — End-to-end smoke test for the ASCEND-LINUX pipeline.
#
# Verifies the full Phase 0/1.1 deliverable chain still functions:
#   1. monad-mbc tests (host-side opcode + translator + boot)
#   2. eBPF programs build (BPF target)
#   3. xv6 kernel builds (RV32 cross-compile + link + translate)
#   4. upc-bootctl validates the kernel image
#   5. doom-runner builds (regression: ABI v2 didn't break Doom)
#
# Run from repo root: bash scripts/ascend-linux-smoke.sh

set -e

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

PASS=0
FAIL=0

step() {
    local name="$1"
    shift
    printf "\n────────────────────────────────────────\n"
    printf "  %s\n" "$name"
    printf "────────────────────────────────────────\n"
    if "$@"; then
        printf "  ✓ %s\n" "$name"
        PASS=$((PASS + 1))
    else
        printf "  ✗ %s\n" "$name"
        FAIL=$((FAIL + 1))
    fi
}

# 1. monad-mbc lib tests (251 expected)
step "monad-mbc lib tests" \
    bash -c 'cd crates/monad-mbc && cargo test --release --lib 2>&1 | grep -q "251 passed; 0 failed"'

# 2. monad-mbc OS-primitive integration tests (55 expected: 46 original + 9 ASCEND)
step "monad-mbc OS-primitive tests (incl. 9 ASCEND-LINUX)" \
    bash -c 'cd crates/monad-mbc && cargo test --release --test os_primitives_test 2>&1 | grep -q "55 passed; 0 failed"'

# 3. eBPF programs build
step "eBPF workspace build (monad-cpu-ebpf with new opcodes)" \
    bash -c 'cd ebpf && cargo build --release 2>&1 | tail -1 | grep -q "Finished"'

# 4. BPF verifier check (gate at ≤25% budget)
step "BPF verifier check (gate ≤25% budget)" \
    bash -c 'bash scripts/bpf-verifier-check.sh 2>&1 | grep -q "GATE: PASSED"'

# 5. xv6-mbc workspace shim builds
step "xv6-mbc shim crate" \
    bash -c 'cd crates/xv6-mbc && cargo build --release 2>&1 | tail -1 | grep -q "Finished"'

# 6. xv6 kernel builds end-to-end (kernel.elf + xv6-mbc.mbc emit)
step "xv6 kernel build (kernel.elf + xv6-mbc.mbc emit)" \
    bash -c 'cd crates/xv6-mbc/upstream && \
             rm -f ../adapters/*.o kernel/*.o target/* 2>/dev/null; \
             make -f ../adapters/Makefile.mbc kernel 2>&1 | tail -3 | grep -q "Wrote.*xv6-mbc.mbc"'

# 7. upc-bootctl tool validates the kernel image
step "upc-bootctl validate xv6-mbc.mbc" \
    bash -c 'cd cmd/upc-bootctl && cargo build --release 2>&1 > /dev/null && \
             ./target/release/upc-bootctl validate \
               --kernel ../../crates/xv6-mbc/upstream/target/xv6-mbc.mbc 2>&1 | \
             grep -q "structurally valid"'

# 8. upc-bootctl boot --dry-run
step "upc-bootctl boot --dry-run" \
    bash -c 'cd cmd/upc-bootctl && \
             ./target/release/upc-bootctl boot \
               --kernel ../../crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
               --instance 222 --dry-run 2>&1 | \
             grep -q "dry-run complete"'

# 9. upc-tty-bridge builds
step "upc-tty-bridge Go build" \
    bash -c 'go build -o /tmp/upc-tty-bridge.smoke ./cmd/upc-tty-bridge/ && \
             test -x /tmp/upc-tty-bridge.smoke && \
             rm /tmp/upc-tty-bridge.smoke'

# 10. doom-runner builds (regression: ABI v2 MbcCpuState 136 bytes)
step "doom-runner builds with ABI v2 (regression)" \
    bash -c 'cd crates/doom-runner && cargo build --release 2>&1 | tail -1 | grep -q "Finished"'

# 11. GPL boundary check passes (no contamination from new ASCEND-LINUX crates)
step "GPL boundary check (no contamination from new code)" \
    bash -c 'bash scripts/verify-gpl-boundary.sh 2>&1 | grep -q "RESULT: PASS"'

# 12. govulncheck — zero kingdom vulns
step "govulncheck (zero kingdom vulnerabilities)" \
    bash -c '~/go/bin/govulncheck ./... 2>&1 | grep -q "affected by 0 vulnerabilities"'

printf "\n════════════════════════════════════════\n"
printf "  ASCEND-LINUX smoke test summary\n"
printf "    PASS: %d\n" "$PASS"
printf "    FAIL: %d\n" "$FAIL"
printf "════════════════════════════════════════\n"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
