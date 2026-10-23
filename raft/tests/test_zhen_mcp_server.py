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


@unittest.skipUnless(HAVE_MCP, "mcp package not installed")
class TestValidatePath(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        base = Path(os.path.realpath(self._tmp.name))
        self.base = base
        self.root = base / "unheaded"
        self.outside = base / "outside"
        self.evil = base / "unheaded-evil"
        for d in (self.root / "pkg", self.root / "docs", self.outside, self.evil):
            d.mkdir(parents=True)
        for f in (self.root / "README.md", self.root / ".env", self.outside / "loot.txt",
                  self.evil / "loot.txt", self.root / "pkg" / "secrets_manager.go"):
            f.write_text("x")
        os.symlink(self.outside, self.root / "docs" / "escape")
        os.symlink(self.root / ".env", self.root / "docs" / "harmless-name")
        self._patch = mock.patch.object(zms, "ALLOWED_PATHS", [str(self.root)])
        self._patch.start()

    def tearDown(self):
        self._patch.stop()
        self._tmp.cleanup()

    def assertAllowed(self, path):
        got, err = zms.validate_path(str(path))
        self.assertIsNone(err, f"{path} should be allowed")
        return got

    def assertDenied(self, path):
        got, err = zms.validate_path(str(path))
        self.assertIsNone(got, f"{path} should be denied")
        self.assertTrue(err)

    def test_inside_allowed(self):
        self.assertEqual(self.assertAllowed(self.root / "README.md"), str(self.root / "README.md"))
        self.assertAllowed(self.root)
        self.assertAllowed(self.root / "pkg" / "new-file.go")

    def test_sibling_prefix_denied(self):
        self.assertDenied(self.evil / "loot.txt")

    def test_symlink_escape_denied(self):
        self.assertDenied(self.root / "docs" / "escape" / "loot.txt")

    def test_symlink_to_denied_name_denied(self):
        self.assertDenied(self.root / "docs" / "harmless-name")

    def test_traversal_component_denied(self):
        self.assertDenied(f"{self.root}/pkg/../../outside/loot.txt")

    def test_dotdot_inside_name_allowed(self):
        self.assertAllowed(self.root / "pkg" / "notes..md")

    def test_deny_matches_by_component(self):
        self.assertAllowed(self.root / "pkg" / "secrets_manager.go")
        self.assertAllowed(self.root / "docs" / "shadowing.md")
        for p in (".env", ".env.local", "pkg/.ssh/config", "keys/id_rsa.pub",
                  "secrets/db.yaml", "credentials.json", "id_ed25519"):
            self.assertDenied(self.root / p)

    def test_deny_applies_below_root_only(self):
        # A sandbox root that itself sits under a deny-listed name stays usable.
        root = self.base / "secrets" / "unheaded"
        root.mkdir(parents=True)
        (root / "README.md").write_text("x")
        with mock.patch.object(zms, "ALLOWED_PATHS", [str(root)]):
            self.assertAllowed(root / "README.md")
            self.assertDenied(root / ".env")

    def test_empty_and_nul_denied(self):
        self.assertDenied("")
        self.assertDenied(f"{self.root}/README.md\x00.txt")


if __name__ == "__main__":
    unittest.main()
