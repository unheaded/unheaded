# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""Runs tests/scripts/test_write_zhen_app_env.sh, so the job that runs this
directory also holds scripts/write-zhen-app-env.sh to never printing the
password it copies."""

import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def test_the_env_writer_never_prints_the_password():
    r = subprocess.run(['bash', str(ROOT / 'tests' / 'scripts' / 'test_write_zhen_app_env.sh')],
                       capture_output=True, text=True, timeout=60, check=False)
    assert r.returncode == 0, r.stdout + r.stderr
    assert r.stdout.startswith('PASS: write-zhen-app-env')
