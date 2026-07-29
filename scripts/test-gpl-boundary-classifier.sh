#!/bin/bash
# SPDX-License-Identifier: MIT
# test-gpl-boundary-classifier.sh — unit tests for verify-gpl-boundary.sh's
# SPDX expression classifier.
#
# The classifier decides whether a third-party dependency imposes a copyleft
# obligation on us. Getting it wrong in the permissive direction hides real
# AGPL contamination; getting it wrong in the copyleft direction fails CI on
# dual-licensed crates we take under MIT/Apache. Both directions are tested
# here so a future "just make it pass" edit to the classifier breaks a test
# instead of silently retiring the gate.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GPL_BOUNDARY_LIB_ONLY=1 . "${SCRIPT_DIR}/verify-gpl-boundary.sh"

PASS=0
FAIL=0

expect_class() {
    local expr="$1" want="$2" got
    got="$(spdx_class "${expr}")"
    if [ "${got}" = "${want}" ]; then
        printf '  PASS  %-42s -> %s\n' "${expr}" "${got}"
        PASS=$((PASS + 1))
    else
        printf '  FAIL  %-42s -> %s (want %s)\n' "${expr}" "${got}" "${want}"
        FAIL=$((FAIL + 1))
    fi
}

echo "--- spdx_class: permissive ---"
expect_class 'MIT'                                    PERMISSIVE
expect_class 'Apache-2.0'                             PERMISSIVE
expect_class 'Apache-2.0 WITH LLVM-exception'         PERMISSIVE
expect_class 'BSD-3-Clause'                           PERMISSIVE
expect_class 'ISC'                                    PERMISSIVE

echo "--- spdx_class: dual-licensed with permissive branch ---"
# The r-efi case: an LGPL alternative we never elect must not trip the gate.
expect_class 'Apache-2.0 OR LGPL-2.1-or-later OR MIT' PERMISSIVE
expect_class 'MIT OR Apache-2.0'                      PERMISSIVE
expect_class 'GPL-3.0-only OR MIT'                    PERMISSIVE

echo "--- spdx_class: copyleft that must NOT be downgraded ---"
# These are the regressions that matter: if any of these classify PERMISSIVE,
# real contamination walks through the gate unnoticed.
expect_class 'AGPL-3.0-only'                          AGPL
expect_class 'AGPL-3.0-or-later OR Commercial'        AGPL
expect_class 'GPL-2.0-only OR AGPL-3.0'               AGPL
expect_class 'GPL-3.0-only'                           GPL
expect_class 'GPL-3.0-or-later'                       GPL
expect_class 'LGPL-2.1-or-later'                      LGPL

echo "--- spdx_class: GPL-2.0 one-way incompatibility (Barrister 2026-07-29) ---"
# GPL-2.0-only cannot be combined with a GPL-3.0-or-later project. -or-later
# can, because we elect 3.0. Conflating the two is the latent hole this closes.
expect_class 'GPL-2.0-only'                           GPL2_ONLY
expect_class 'GPL-2.0'                                GPL2_ONLY
expect_class 'GPL-2.0-or-later'                       GPL
expect_class 'GPL-2.0+'                               GPL
# A permissive branch still rescues it — we simply elect MIT and never
# take on the GPL-2.0 obligation at all.
expect_class 'GPL-2.0-only OR MIT'                    PERMISSIVE

echo
echo "passed: ${PASS}  failed: ${FAIL}"
[ "${FAIL}" -eq 0 ] || exit 1
echo "RESULT: PASS"
