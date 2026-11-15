# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""Runs tests/scripts/test_zhen_lan_proof.sh, so the job that runs this
directory also runs the planning half of scripts/zhen-lan-proof.sh."""

import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def test_the_lan_proof_plan():
    r = subprocess.run(['bash', str(ROOT / 'tests' / 'scripts' / 'test_zhen_lan_proof.sh')],
                       capture_output=True, text=True, timeout=60, check=False)
    assert r.returncode == 0, r.stdout + r.stderr
    assert r.stdout.startswith('PASS: zhen-lan-proof plan')
