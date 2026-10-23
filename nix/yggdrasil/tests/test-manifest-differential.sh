#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# test-manifest-differential.sh — Go validator vs Python jsonschema, same fixtures.
#
# `yggdrasil-evidence validate` mirrors schema/manifest-v1.yaml in Go (no
# JSON-Schema dependency, ADR-004 / ADR-105). This proves the two agree on every
# fixture under cmd/yggdrasil-evidence/testdata/manifests/: Python jsonschema
# (Draft 2020-12, FormatChecker) over yaml.safe_load, against the Go binary.
#
# Documented disagreements (Go stricter, never laxer):
#   unknown-field.yaml  the schema has no additionalProperties: false, so
#                       jsonschema accepts an unknown key; Go's strict decode
#                       refuses it. Changing the schema is a Round Table call.
#
# jsonschema checks `date-time` only when rfc3339-validator is installed, and
# neither the local nor the CI (--no-deps, hash-pinned) environment has it. This
# script registers an RFC 3339 checker itself so the format is compared rather
# than silently skipped on the Python side.
#
# Usage: YE=/path/to/yggdrasil-evidence nix/yggdrasil/tests/test-manifest-differential.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SCHEMA="${ROOT}/nix/yggdrasil/evidence-pack/schema/manifest-v1.yaml"
FIXTURES="${ROOT}/cmd/yggdrasil-evidence/testdata/manifests"
: "${YE:?set YE to a built yggdrasil-evidence binary (go build -o \"\$YE\" ./cmd/yggdrasil-evidence)}"
[ -x "${YE}" ] || { echo "FAIL: ${YE} is not executable"; exit 1; }

GO_ONLY="unknown-field"

py_verdict() {
    python3 -I - "${SCHEMA}" "$1" <<'PY'
import datetime, re, sys
import yaml
from jsonschema import Draft202012Validator, FormatChecker

schema = yaml.safe_load(open(sys.argv[1], encoding="utf-8"))
fc = FormatChecker()
RFC3339 = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$")

@fc.checks("date-time", raises=ValueError)
def _date_time(v):
    if not isinstance(v, str):
        return True
    if not RFC3339.fullmatch(v):
        return False
    datetime.datetime.fromisoformat(v.replace("Z", "+00:00"))
    return True

try:
    doc = yaml.safe_load(open(sys.argv[2], encoding="utf-8"))
except yaml.YAMLError:
    print("INVALID")
    sys.exit(0)
errors = list(Draft202012Validator(schema, format_checker=fc).iter_errors(doc))
print("INVALID" if errors else "VALID")
PY
}

go_verdict() {
    local rc=0
    "${YE}" validate "$1" >/dev/null 2>&1 || rc=$?
    case "${rc}" in
        0) echo VALID ;;
        1) echo INVALID ;;
        *) echo "ERROR(rc=${rc})" ;;
    esac
}

total=0; agree=0; documented=0; fail=0
for f in "${FIXTURES}"/valid/*.yaml "${FIXTURES}"/invalid/*.yaml; do
    name="$(basename "${f}" .yaml)"
    expect="VALID"; case "${f}" in */invalid/*) expect="INVALID" ;; esac
    py="$(py_verdict "${f}")"; go="$(go_verdict "${f}")"
    total=$((total + 1))
    if [ "${go}" != "${expect}" ]; then
        echo "FAIL ${name}: Go says ${go}, fixture expects ${expect}"; fail=$((fail + 1)); continue
    fi
    if [ "${py}" = "${go}" ]; then
        agree=$((agree + 1))
    elif [ "${name}" = "${GO_ONLY}" ] && [ "${py}" = "VALID" ] && [ "${go}" = "INVALID" ]; then
        documented=$((documented + 1)); agree=$((agree + 1))
        echo "NOTE ${name}: documented Go-only rejection (jsonschema VALID, Go INVALID)"
    else
        echo "FAIL ${name}: jsonschema ${py}, Go ${go}"; fail=$((fail + 1))
    fi
done

# The documented exception must still be an exception: if jsonschema starts
# rejecting it (schema gained additionalProperties: false), update this script.
if [ "${documented}" -ne 1 ]; then
    echo "FAIL: expected exactly 1 documented Go-only rejection, saw ${documented}"; fail=$((fail + 1))
fi

if [ "${fail}" -ne 0 ]; then
    echo "DISAGREE: ${fail} of ${total}"
    exit 1
fi
echo "AGREE ${agree}/${total} (${documented} documented Go-only rejection)"
