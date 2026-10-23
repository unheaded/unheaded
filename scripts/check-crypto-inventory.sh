#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-crypto-inventory.sh — the checked-in crypto inventory (CBOM) must
# match the tree (ADR-105 decision 4).
#
# compliance/crypto/cbom.cdx.json is GENERATED: `go run ./cmd/crypto-inventory
# scan --out compliance/crypto/cbom.cdx.json` walks every file git knows about
# (tracked, plus untracked files that are not ignored) and the read-only host
# facts in compliance/crypto/hosts/*.json, and writes a deterministic CycloneDX
# 1.6 document: sorted, no timestamp, no serial number. So "fresh" has a
# precise meaning: regenerating gives the same bytes. This gate regenerates in
# memory and fails (exit 1) when they differ, naming each component added (+),
# removed (-) or changed (~). A new TLS config, rustls provider, sshd KEX list,
# WireGuard peer, postgres image or signature import therefore cannot land
# without the inventory saying so in the same commit.
#
# With --schema (or CI=true) it also validates the file against the published
# CycloneDX 1.6 JSON schema (bom-1.6 + spdx + jsf-0.82, tag 1.6.1), fetched by
# sha256 into the tool cache and never vendored, with Python jsonschema
# (Draft-07, local $ref resolution, no network after the fetch). The CI job
# installs jsonschema from .github/ci-python/requirements.txt (hashed).
#
# Host facts are refreshed by hand, read-only, never by this gate:
#   go run ./cmd/crypto-inventory collect-host --host east --ssh govan@east \
#     --out compliance/crypto/hosts/east.json
#
# Exit: 0 fresh (and schema-valid when asked), 1 stale or schema-invalid,
# 2 could not run.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CBOM="compliance/crypto/cbom.cdx.json"
cd "$REPO_ROOT" || exit 2

SCHEMA=0
[[ "${CI:-}" == "true" ]] && SCHEMA=1
for arg in "$@"; do
	case "$arg" in
	--schema) SCHEMA=1 ;;
	*)
		echo "usage: $0 [--schema]" >&2
		exit 2
		;;
	esac
done

go run ./cmd/crypto-inventory check --cbom "$CBOM"
rc=$?
[[ $rc -eq 0 ]] || exit "$rc"
[[ $SCHEMA -eq 1 ]] || exit 0

# CycloneDX/specification tag 1.6.1, schema/ directory. A bump changes the
# tag and every sha256 together; take them from the tagged files.
SCHEMA_TAG="1.6.1"
SCHEMA_FILES=(
	"bom-1.6.schema.json efc54d749e32a6e16abd19394b80b4c67d846e12c782e04505130375f94ea541"
	"spdx.schema.json c41917196639055e9f9670811bac23ef777732144f3ff5a2f39686f61580dbe6"
	"jsf-0.82.schema.json 8bae002c25e723db7ee1f26afde680ae1a2b1a8f6b4b4b0fd65dc3becb090aae"
)
cache="${XDG_CACHE_HOME:-$HOME/.cache}/unheaded-tools/cyclonedx-schema-${SCHEMA_TAG}"
mkdir -p "$cache" || exit 2
for entry in "${SCHEMA_FILES[@]}"; do
	name="${entry%% *}"
	sum="${entry##* }"
	f="$cache/$name"
	if [[ ! -f "$f" ]]; then
		curl -fsSL --retry 3 -o "$f.part" \
			"https://raw.githubusercontent.com/CycloneDX/specification/${SCHEMA_TAG}/schema/${name}" || {
			echo "[FAIL] could not fetch ${name} (CycloneDX ${SCHEMA_TAG})" >&2
			rm -f "$f.part"
			exit 2
		}
		mv "$f.part" "$f"
	fi
	echo "${sum}  ${f}" | sha256sum -c --quiet - >&2 || {
		echo "[FAIL] ${name} does not match its pinned sha256; deleted" >&2
		rm -f "$f"
		exit 2
	}
done

python3 -I - "$CBOM" "$cache" <<'PY'
import json, sys
from pathlib import Path
try:
    import jsonschema
    from referencing import Registry, Resource
except ImportError as e:
    print(f"[FAIL] python jsonschema/referencing not importable: {e}", file=sys.stderr)
    sys.exit(2)

cbom, cache = sys.argv[1], Path(sys.argv[2])
resources = []
for name in ("bom-1.6.schema.json", "spdx.schema.json", "jsf-0.82.schema.json"):
    doc = json.loads((cache / name).read_text())
    resources.append((doc["$id"], Resource.from_contents(doc)))
registry = Registry().with_resources(resources)
root = json.loads((cache / "bom-1.6.schema.json").read_text())
validator = jsonschema.Draft7Validator(root, registry=registry)
errors = sorted(validator.iter_errors(json.loads(Path(cbom).read_text())), key=lambda e: list(e.absolute_path))
if errors:
    print(f"[FAIL] {cbom} is not valid CycloneDX 1.6 ({len(errors)} error(s)); first 5:", file=sys.stderr)
    for e in errors[:5]:
        print(f"  at /{'/'.join(map(str, e.absolute_path))}: {e.message[:300]}", file=sys.stderr)
    sys.exit(1)
print(f"[PASS] {cbom} validates against the CycloneDX 1.6 schema")
PY
