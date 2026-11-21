# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""Runs tests/scripts/test_install_prerequisites.sh, so the job that runs this
directory also runs scripts/install-prerequisites.sh against a throwaway tree
with stubbed package, docker and systemd commands (no root, no network)."""

import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def test_the_prerequisites_installer():
    r = subprocess.run(['bash', str(ROOT / 'tests' / 'scripts' / 'test_install_prerequisites.sh')],
                       capture_output=True, text=True, timeout=120, check=False)
    assert r.returncode == 0, r.stdout + r.stderr
    assert 'PASS: install-prerequisites' in r.stdout
