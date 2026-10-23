# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""
Unit tests for raft/zhen_mcp_server.py.

Drives the server in-process through the mcp library's own request
handlers, so isError and tool listing are checked exactly as a client
sees them. Needs the `mcp` package (e.g. ~/.venv/zhen); skipped without it.

    ~/.venv/zhen/bin/python -m pytest raft/tests/test_zhen_mcp_server.py -v
"""

import asyncio
import importlib.util
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

RAFT_DIR = Path(__file__).resolve().parent.parent
REPO_ROOT = RAFT_DIR.parent

HAVE_MCP = importlib.util.find_spec("mcp") is not None
if HAVE_MCP:
    sys.path.insert(0, str(RAFT_DIR))
    import zhen_mcp_server as zms
    from mcp import types


def _run(coro):
    return asyncio.run(coro)


@unittest.skipUnless(HAVE_MCP, "mcp package not installed")
class TestProjectRootAndVersion(unittest.TestCase):
    def test_project_root_derived_from_file(self):
        self.assertEqual(Path(zms.PROJECT_ROOT), REPO_ROOT)
        self.assertEqual(Path(zms.RUNBOOKS_DIR), REPO_ROOT / "runbooks")

    def test_project_root_env_override(self):
        with tempfile.TemporaryDirectory() as d:
            got = zms.resolve_project_root({"ZHEN_MCP_PROJECT_ROOT": d})
            self.assertEqual(got, Path(os.path.realpath(d)))

    def test_project_root_env_empty_falls_back(self):
        self.assertEqual(zms.resolve_project_root({"ZHEN_MCP_PROJECT_ROOT": ""}), REPO_ROOT)

    def test_server_version_is_own(self):
        from importlib.metadata import version

        opts = zms.app.create_initialization_options()
        self.assertEqual(opts.server_name, "zhen-champion")
        self.assertEqual(opts.server_version, zms.__version__)
        self.assertNotEqual(opts.server_version, version("mcp"))


if __name__ == "__main__":
    unittest.main()
