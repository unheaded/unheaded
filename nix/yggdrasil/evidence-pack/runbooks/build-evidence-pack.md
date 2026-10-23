# Runbook — Build Evidence Pack (operator-side, packer pipeline)

**Audience**: packer build runner (task #65 implementation).
**Schedule**: Once per Yggdrasil image build, immediately after the ISO is emitted.

---

## Prerequisites

- Yggdrasil ISO has been built by packer (`build/yggdrasil-<version>.iso` exists).
- The build key seed is available at `/etc/yggdrasil/build.key` (32-byte ML-DSA-65 seed, mode 0600; `yggdrasil-evidence sign` refuses anything group- or world-readable). Where that seed lives, who holds it, how `build.pubkey` is published and how it rotates is ADR-105 open question **E4** (Stevie's custody decision). Until E4 is settled there is no production key: packs are signed only with throwaway test keys, and nothing in the repo generates or commits a key.
- Trivy ≥ 0.50 installed.
- Lynis ≥ 3.1 installed.
- yamllint installed (optional; `yggdrasil-evidence validate` is the gate).
- `yggdrasil-evidence` built from `cmd/yggdrasil-evidence/` and on PATH.

## Steps

### 1. Stage the manifest skeleton

```bash
WORK=$(mktemp -d -t yggdrasil-evidence-XXXXXX)
ISO_PATH="build/yggdrasil-${YGG_VERSION}.iso"
ISO_SHA=$(sha256sum "$ISO_PATH" | awk '{print $1}')
mkdir -p "$WORK/manifest" "$WORK/manifest/overlay-patches"
```

### 2. Compute anchor metadata

```bash
ANCHOR_CODENAME=$(grep release_codename nix/yggdrasil/anchor.nix | cut -d'"' -f2)
ANCHOR_VERSION=$(grep release_version nix/yggdrasil/anchor.nix | cut -d'"' -f2)
ANCHOR_DATE=$(grep release_date nix/yggdrasil/anchor.nix | cut -d'"' -f2)
ANCHOR_COMMIT=$(grep debian_commit nix/yggdrasil/anchor.nix | cut -d'"' -f2)
```

### 3. Inventory overlay patches

```bash
PATCH_COUNT=$(ls nix/yggdrasil/overlay/patches/*.patch | wc -l)
TOTAL_LOC=0
for p in nix/yggdrasil/overlay/patches/*.patch; do
  LOC=$(diffstat -p1 -t "$p" | tail -n+2 | awk -F, '{sum += $2 + $3} END {print sum+0}')
  TOTAL_LOC=$((TOTAL_LOC + LOC))
  cp "$p" "$WORK/manifest/overlay-patches/"
done
```

### 4. Generate SBOM (SPDX + CycloneDX)

```bash
trivy rootfs --format spdx-json --output "$WORK/manifest/sbom.spdx.json" "$ISO_PATH"
trivy rootfs --format cyclonedx --output "$WORK/manifest/sbom.cyclonedx.json" "$ISO_PATH"
PKG_COUNT=$(jq '.packages | length' "$WORK/manifest/sbom.spdx.json")
```

### 5. Run CVE scan — fail build on any CRITICAL/HIGH

```bash
trivy rootfs --severity CRITICAL,HIGH,MEDIUM,LOW --format json --output "$WORK/manifest/cve-scan.json" "$ISO_PATH"
CRIT=$(jq '[.Results[].Vulnerabilities[]? | select(.Severity == "CRITICAL")] | length' "$WORK/manifest/cve-scan.json")
HIGH=$(jq '[.Results[].Vulnerabilities[]? | select(.Severity == "HIGH")]     | length' "$WORK/manifest/cve-scan.json")
if [ "$CRIT" -gt 0 ] || [ "$HIGH" -gt 0 ]; then
  echo "BUILD FAILED: $CRIT critical + $HIGH high CVEs (waivers must be in chain_of_custody before retry)" >&2
  exit 1
fi
```

### 6. Run CIS benchmark via Lynis

```bash
mount -o loop "$ISO_PATH" /mnt/yggdrasil-iso
lynis audit system --no-colors --quiet --report-file "$WORK/lynis.dat"
HARDENING_SCORE=$(grep '^hardening_index' "$WORK/lynis.dat" | cut -d= -f2)
# Custom CIS benchmark check (per docs/security/CIS-mapping.md, TBD)
yggdrasil-cis-check --iso "$ISO_PATH" --json > "$WORK/manifest/cis-benchmark.json"
SCORE=$(jq '.score_percent' "$WORK/manifest/cis-benchmark.json")
if [ "$(echo "$SCORE < 95" | bc)" -eq 1 ]; then
  echo "BUILD FAILED: CIS score $SCORE% < 95%" >&2
  exit 1
fi
```

### 7. Run upc-doctor against a freshly-booted instance

```bash
qemu-system-x86_64 -m 2G -smp 2 -boot d -cdrom "$ISO_PATH" \
  -display none -monitor none -serial stdio \
  -netdev user,id=net0,hostfwd=tcp::2222-:22 -device virtio-net,netdev=net0 \
  -snapshot &
QEMU_PID=$!
sleep 60  # let it boot
ssh -p 2222 -o StrictHostKeyChecking=no root@localhost \
    "yggdrasil-doctor upc --json" > "$WORK/manifest/upc-doctor.json"
DOCTOR_EXIT=$(jq '.exit_code' "$WORK/manifest/upc-doctor.json")
kill $QEMU_PID
if [ "$DOCTOR_EXIT" != "0" ]; then
  echo "BUILD FAILED: yggdrasil-doctor upc exit code $DOCTOR_EXIT (image regression)" >&2
  exit 1
fi
```

### 8. Assemble manifest.yaml from gathered data

Use a templating tool (yq, gomplate, or a small Go program in `cmd/yggdrasil-evidence/`) to populate `$WORK/manifest/manifest.yaml` per the `schema/manifest-v1.yaml` schema.

### 9. Sign the new chain_of_custody entry, then validate

Every custody entry is signed (hash-chained to the entry before it). The build's own entry is the last one, so its `signature` line is appended at the end of the file:

```bash
N=$(( $(grep -c '^  - timestamp_utc:' "$WORK/manifest/manifest.yaml") - 1 ))
yggdrasil-evidence sign --key /etc/yggdrasil/build.key --purpose custody-entry \
  --custody-index "$N" --in "$WORK/manifest/manifest.yaml" --out "$WORK/custody.sig"
printf '    signature: "%s"\n' "$(cat "$WORK/custody.sig")" >> "$WORK/manifest/manifest.yaml"

yamllint -d relaxed "$WORK/manifest/manifest.yaml"   # optional
# --schema makes validate refuse to run if this checkout's schema differs from the rules built into the binary
yggdrasil-evidence validate --schema nix/yggdrasil/evidence-pack/schema/manifest-v1.yaml "$WORK/manifest/manifest.yaml"
```

### 10. Sign manifest.yaml + image.iso.sha256

Signatures are gungnir evidence seals (JSON). `sign --purpose manifest` refuses a manifest that does not validate.

```bash
yggdrasil-evidence sign --key /etc/yggdrasil/build.key --purpose manifest \
  --in "$WORK/manifest/manifest.yaml" --out "$WORK/manifest/manifest.yaml.sig"
echo "$ISO_SHA  $(basename "$ISO_PATH")" > "$WORK/manifest/image.iso.sha256"
yggdrasil-evidence sign --key /etc/yggdrasil/build.key --purpose iso-sha256 \
  --in "$WORK/manifest/image.iso.sha256" --out "$WORK/manifest/image.iso.sha256.sig"
```

### 11. Tarball, self-check, emit

```bash
tar -czf "build/evidence-pack-${ISO_SHA}.tar.gz" -C "$WORK" manifest/
yggdrasil-evidence pubkey --key /etc/yggdrasil/build.key --out "$WORK/build.pubkey"
yggdrasil-evidence verify --pubkey "$WORK/build.pubkey" --iso "$ISO_PATH" "build/evidence-pack-${ISO_SHA}.tar.gz"
echo "Evidence pack: build/evidence-pack-${ISO_SHA}.tar.gz"
```

### 12. Cleanup

```bash
rm -rf "$WORK"
```

## On failure

Any non-zero exit at steps 5, 6, 7, 9 fails the build entirely. Do not ship the ISO without a valid evidence pack. Append the failure to `chain-of-custody.txt` for the NEXT build attempt.
