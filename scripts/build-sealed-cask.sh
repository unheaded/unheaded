#!/bin/bash
# build-sealed-cask.sh — Build a deterministic, immutable Unheaded deployment image
# ADR-010: Sealed Cask Deployment (Soul Vessel Model)
#
# Creates a tarball with one top-level sealed-cask-<stamp>/ directory holding:
#   - The unit binaries (scripts/build-binaries.sh)
#   - All eBPF programs
#   - Configuration files, NixOS definitions, runbooks, migrations: tracked
#     files only (git ls-files), so gitignored dev keys, certs and users under
#     configs/ never ship
#   - BINDING_RUNE.sha256: a `sha256sum` manifest of every file
#   - BINDING_RUNE.sig: the build key's ML-DSA-65 Gungnir seal over that
#     manifest (S-IRON-GATE Phase 13), when UNHEADED_BUILD_KEY is set
#
# Verify before deployment with:
#   cask-verify verify --cask <cask.tar.gz> --pubkey <build.pubkey>
#   (scripts/verify-binding-rune.sh <cask.tar.gz> wraps it)
#
# Usage:
#   ./scripts/build-sealed-cask.sh [--output /path/to/cask.tar.gz] [--no-build] [--no-encrypt]
#     --no-build    skip the Go and eBPF builds: stage what is already built
#                   (tests and dry runs)
#     --no-encrypt  skip the age phase
#
# Environment:
#   UNHEADED_BUILD_KEY   path to the ML-DSA-65 build key seed (custody per
#                        runbooks/security/build-key-ceremony.yaml). Unset:
#                        the cask is built UNSIGNED and cask-verify refuses it.
#   UNHEADED_BUILD_SIGN_AS
#                        run the signer as this account (sudo -n -u), e.g.
#                        unheaded-buildkey on the custody host, which owns
#                        the seed; the rune goes over stdin, the seal stdout
#   YGGDRASIL_EVIDENCE   the yggdrasil-evidence binary (default: on PATH, else
#                        `go run ./cmd/yggdrasil-evidence`)

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TIMESTAMP=$(date -u +%Y%m%d-%H%M%S)
OUTPUT=""
BUILD=1
ENCRYPT=1
while [ $# -gt 0 ]; do
    case "$1" in
        --output) OUTPUT="${2:?--output needs a path}"; shift 2 ;;
        --output=*) OUTPUT="${1#--output=}"; shift ;;
        --no-build) BUILD=0; shift ;;
        --no-encrypt) ENCRYPT=0; shift ;;
        -h|--help) sed -n '2,34p' "$0"; exit 0 ;;
        -*) echo "build-sealed-cask: unknown option $1" >&2; exit 2 ;;
        *) [ -z "${OUTPUT}" ] || { echo "build-sealed-cask: one output path" >&2; exit 2; }
           OUTPUT="$1"; shift ;;
    esac
done
OUTPUT="${OUTPUT:-${PROJECT_ROOT}/deploy/cask-${TIMESTAMP}.tar.gz}"

# A private staging parent: mktemp -d is 0700 and unpredictable, so no other
# user can pre-create or swap the directory the manifest is computed over.
STAGING_PARENT=$(mktemp -d "${TMPDIR:-/tmp}/sealed-cask.XXXXXXXX")
trap 'rm -rf "${STAGING_PARENT}"' EXIT
STAGING="${STAGING_PARENT}/sealed-cask-${TIMESTAMP}"
MANIFEST="${STAGING}/BINDING_RUNE.sha256"
SIG="${STAGING}/BINDING_RUNE.sig"

echo "=============================================="
echo "SEALED CASK BUILD — ADR-010"
echo "=============================================="
echo "Project: ${PROJECT_ROOT}"
echo "Output:  ${OUTPUT}"
echo "Time:    $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

mkdir -p "${STAGING}"/{bin,ebpf,configs,nix,runbooks,db}
cd "${PROJECT_ROOT}"

# --- Phase 1: Build binaries ---
if [ "${BUILD}" -eq 1 ]; then
    echo "=== Phase 1: Build Go binaries ==="
    ./scripts/build-binaries.sh --out "${STAGING}/bin"
    echo "  Go build: OK"
    echo "=== Phase 2: Build eBPF programs ==="
    (cd "${PROJECT_ROOT}/ebpf" && cargo build --release 2>&1 | tail -3)
    echo "  eBPF build: OK"
else
    echo "=== Phases 1-2: builds SKIPPED (--no-build) ==="
fi

# --- Phase 3: Stage artifacts ---
echo "=== Phase 3: Stage artifacts ==="

# eBPF objects (regular files only; a link never enters a cask)
echo "  Staging eBPF objects..."
EBPF_OUT="${PROJECT_ROOT}/ebpf/target/bpfel-unknown-none/release"
if [ -d "${EBPF_OUT}" ]; then
    find "${EBPF_OUT}/" -maxdepth 1 -type f -exec cp {} "${STAGING}/ebpf/" \;
else
    echo "  (none built: ${EBPF_OUT} does not exist)"
fi

# Tracked files only: configs/keys, configs/certs, configs/auth and other
# gitignored material stay out. Each is copied as a regular file.
stage_tracked() {
    local src="$1" dst="$2"
    git -C "${PROJECT_ROOT}" ls-files -z -- "${src}" | while IFS= read -r -d '' f; do
        [ -f "${PROJECT_ROOT}/${f}" ] || continue
        rel="${f#"${src}"/}"
        mkdir -p "${dst}/$(dirname "${rel}")"
        cp -L "${PROJECT_ROOT}/${f}" "${dst}/${rel}"
    done
}
echo "  Staging configs..."
stage_tracked configs "${STAGING}/configs"
cp "${PROJECT_ROOT}/docker-compose.yml" "${STAGING}/configs/"
echo "  Staging NixOS definitions..."
stage_tracked nix "${STAGING}/nix"
echo "  Staging runbooks..."
stage_tracked runbooks "${STAGING}/runbooks"
echo "  Staging migrations..."
stage_tracked db/migrations "${STAGING}/db/migrations"

if [ -n "$(find "${STAGING}" ! -type f ! -type d -print -quit)" ]; then
    echo "ERROR: staging holds something other than files and directories" >&2
    exit 1
fi

# --- Phase 4: Generate Binding Rune (SHA256 manifest) ---
echo "=== Phase 4: Generate Binding Rune ==="
cd "${STAGING}"
find . -type f ! -name BINDING_RUNE.sha256 ! -name BINDING_RUNE.sig -print0 \
    | LC_ALL=C sort -z | xargs -0 -r sha256sum > "${MANIFEST}"
TOTAL=$(wc -l < "${MANIFEST}")
RUNE_HASH=$(sha256sum "${MANIFEST}" | awk '{print $1}')
echo "  Files: ${TOTAL}"
echo "  Rune hash: ${RUNE_HASH}"

# --- Phase 4b: Seal the Binding Rune with the build key ---
SIGNED=0
if [ -n "${UNHEADED_BUILD_KEY:-}" ]; then
    echo "=== Phase 4b: Seal Binding Rune (ML-DSA-65 build key) ==="
    if [ -n "${YGGDRASIL_EVIDENCE:-}" ]; then
        YE=("${YGGDRASIL_EVIDENCE}")
    elif command -v yggdrasil-evidence >/dev/null 2>&1; then
        YE=(yggdrasil-evidence)
    else
        YE=(go -C "${PROJECT_ROOT}" run ./cmd/yggdrasil-evidence)
    fi
    if [ -n "${UNHEADED_BUILD_SIGN_AS:-}" ]; then
        # The seed is 0400 to its custody account, which cannot read this
        # 0700 staging dir: hand it the rune on stdin, take the seal on stdout.
        [ -n "${YGGDRASIL_EVIDENCE:-}" ] || command -v yggdrasil-evidence >/dev/null 2>&1 ||
            { echo "ERROR: UNHEADED_BUILD_SIGN_AS needs an installed yggdrasil-evidence (or YGGDRASIL_EVIDENCE)" >&2; exit 1; }
        # shellcheck disable=SC2024 # the redirects run as the caller on purpose
        sudo -n -u "${UNHEADED_BUILD_SIGN_AS}" "${YE[@]}" cask sign --key "${UNHEADED_BUILD_KEY}" \
            --manifest - --out - < "${MANIFEST}" > "${SIG}"
    else
        "${YE[@]}" cask sign --key "${UNHEADED_BUILD_KEY}" --manifest "${MANIFEST}" --out "${SIG}"
    fi
    SIGNED=1
else
    echo "=== Phase 4b: UNSIGNED CASK (set UNHEADED_BUILD_KEY) ==="
    echo "  cask-verify refuses an unsigned cask; this one is for development only."
fi

# --- Phase 5: Package ---
echo "=== Phase 5: Package ==="
mkdir -p "$(dirname "${OUTPUT}")"
tar czf "${OUTPUT}" -C "${STAGING_PARENT}" "$(basename "${STAGING}")"
SIZE=$(du -h "${OUTPUT}" | awk '{print $1}')
echo "  Output: ${OUTPUT} (${SIZE})"

# --- Phase 6: Encrypt with age (ADR-010 + Sprint H2) ---
AGE_KEY="${PROJECT_ROOT}/secrets/cask.age.key"
ENCRYPTED="${OUTPUT}.age"

if [ "${ENCRYPT}" -eq 0 ]; then
    echo "=== Phase 6: Encryption SKIPPED (--no-encrypt) ==="
elif which age >/dev/null 2>&1; then
    echo "=== Phase 6: Encrypt ==="
    if [ ! -f "${AGE_KEY}" ]; then
        echo "  Generating age key pair..."
        mkdir -p "$(dirname "${AGE_KEY}")"
        age-keygen -o "${AGE_KEY}" 2>/dev/null
        chmod 600 "${AGE_KEY}"
        echo "  Key saved to: ${AGE_KEY} (PROTECT THIS FILE)"
    fi

    AGE_PUB=$(grep "^age1" "${AGE_KEY}" || age-keygen -y "${AGE_KEY}")
    age -r "${AGE_PUB}" -o "${ENCRYPTED}" "${OUTPUT}"
    ENC_SIZE=$(du -h "${ENCRYPTED}" | awk '{print $1}')
    echo "  Encrypted: ${ENCRYPTED} (${ENC_SIZE})"
    echo "  Decrypt:   age -d -i ${AGE_KEY} ${ENCRYPTED} > cask.tar.gz"
else
    echo "=== Phase 6: Encryption SKIPPED (age not installed) ==="
    echo "  Install with: apt install age"
fi

echo ""
echo "=============================================="
if [ "${SIGNED}" -eq 1 ]; then
echo "SEALED CASK BUILT (signed)"
else
echo "SEALED CASK BUILT (UNSIGNED CASK: development only)"
fi
echo "=============================================="
echo "  Package:    ${OUTPUT}"
echo "  Size:       ${SIZE}"
echo "  Files:      ${TOTAL}"
echo "  Rune hash:  ${RUNE_HASH}"
echo "  Verify:     cask-verify verify --cask ${OUTPUT} --pubkey <build.pubkey>"
if [ -f "${ENCRYPTED}" ] && [ "${ENCRYPT}" -eq 1 ]; then
echo "  Encrypted:  ${ENCRYPTED} (${ENC_SIZE})"
echo "  Decrypt:    age -d -i ${AGE_KEY} ${ENCRYPTED} > cask.tar.gz"
fi
echo ""
