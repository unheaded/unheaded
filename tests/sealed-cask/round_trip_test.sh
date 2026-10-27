#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Round trip for the signed Sealed Cask (S-IRON-GATE Steps 335-337): a cask
# built by scripts/build-sealed-cask.sh with a throwaway ML-DSA-65 build key
# (made at run time in a temp dir, never committed, never the real build key)
# verifies with its public key, and is refused when a byte is appended, when
# it is unsigned, and under another key. Ignored material under configs/
# (dev keys, certs) must not be staged. --no-build: the Go and eBPF builds are
# not what this tests.
# shellcheck disable=SC2015 # pass and bad always succeed, so A && pass || bad is if/else
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
T="$(mktemp -d)"
PROBE="${ROOT}/configs/keys/.cask-leak-probe-$$"
trap 'rm -rf "$T"; rm -f "$PROBE"; rmdir "$(dirname "$PROBE")" 2>/dev/null || true' EXIT
fail=0
pass() { echo "[PASS] $*"; }
bad() { echo "[FAIL] $*"; fail=1; }

cd "$ROOT"
for b in yggdrasil-evidence cask-verify unheaded-keygen; do
	go build -o "$T/bin/$b" "./cmd/$b"
done
export YGGDRASIL_EVIDENCE="$T/bin/yggdrasil-evidence" CASK_VERIFY="$T/bin/cask-verify"
"$T/bin/unheaded-keygen" -algo mldsa65 -dir "$T/keys" build >/dev/null
"$T/bin/unheaded-keygen" -algo mldsa65 -dir "$T/keys" other >/dev/null
KEY="$T/keys/build.key" PUB="$T/keys/build.pub" OTHER="$T/keys/other.pub"

# An ignored (untracked) file that must never reach a cask.
mkdir -p "$(dirname "$PROBE")"
echo "not for shipping" >"$PROBE"

UNHEADED_BUILD_KEY="$KEY" scripts/build-sealed-cask.sh --no-build --no-encrypt --output "$T/cask.tar.gz" >"$T/build.log" 2>&1 ||
	{ cat "$T/build.log"; bad "signed build"; }
scripts/build-sealed-cask.sh --no-build --no-encrypt --output "$T/unsigned.tar.gz" >"$T/build2.log" 2>&1 ||
	{ cat "$T/build2.log"; bad "unsigned build"; }
grep -q "UNSIGNED CASK (set UNHEADED_BUILD_KEY)" "$T/build2.log" && pass "unsigned build says so" || bad "unsigned build banner"

if "$CASK_VERIFY" verify --cask "$T/cask.tar.gz" --pubkey "$PUB" >"$T/v.log" 2>&1; then
	pass "signed cask VERIFIED"
else
	cat "$T/v.log"
	bad "signed cask refused"
fi
scripts/verify-binding-rune.sh --pubkey "$PUB" "$T/cask.tar.gz" >"$T/vr.log" 2>&1 || true
grep -q "BINDING RUNE INTACT" "$T/vr.log" && pass "verify-binding-rune delegates" || { cat "$T/vr.log"; bad "verify-binding-rune"; }

tar tzvf "$T/cask.tar.gz" >"$T/list" || bad "cask does not list"
grep -q '/BINDING_RUNE.sig$' "$T/list" && pass "cask carries BINDING_RUNE.sig" || bad "no BINDING_RUNE.sig"
grep -q '/runbooks/security/' "$T/list" && pass "tracked runbooks staged" || bad "runbooks missing"
if grep -q 'cask-leak-probe' "$T/list"; then bad "ignored configs/keys file shipped"; else pass "ignored configs/keys not staged"; fi
if grep -q '^[lhbcp]' "$T/list"; then bad "cask holds a link or device"; else pass "files and directories only"; fi

cp "$T/cask.tar.gz" "$T/tampered.tar.gz"
printf 'x' >>"$T/tampered.tar.gz"
if "$CASK_VERIFY" verify --cask "$T/tampered.tar.gz" --pubkey "$PUB" >/dev/null 2>&1; then bad "appended byte accepted"; else pass "tamper refused"; fi
"$CASK_VERIFY" verify --cask "$T/unsigned.tar.gz" --pubkey "$PUB" >"$T/u.log" 2>&1 || true
grep -q "UNSIGNED CASK" "$T/u.log" && pass "unsigned refused" || bad "unsigned accepted"
if "$CASK_VERIFY" verify --cask "$T/cask.tar.gz" --pubkey "$OTHER" >/dev/null 2>&1; then bad "wrong key accepted"; else pass "wrong key refused"; fi
if scripts/verify-binding-rune.sh --pubkey "$OTHER" "$T/cask.tar.gz" >/dev/null 2>&1; then bad "verify-binding-rune wrong key"; else pass "verify-binding-rune refuses wrong key"; fi
scripts/verify-binding-rune.sh --insecure-sha-only "$T/cask.tar.gz" >"$T/ins.log" 2>&1 || true
grep -q "INSECURE" "$T/ins.log" && grep -q "origin NOT verified" "$T/ins.log" && pass "--insecure-sha-only banner" || bad "--insecure-sha-only"

"$CASK_VERIFY" extract --cask "$T/cask.tar.gz" --pubkey "$PUB" --dest "$T/out" >/dev/null &&
	cmp -s "$T/out/runbooks/security/heimdall-enrol-drift-key.yaml" runbooks/security/heimdall-enrol-drift-key.yaml &&
	pass "extract writes the signed bytes" || bad "extract"

exit "$fail"
