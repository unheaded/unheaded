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


def _call(name, arguments=None):
    """tools/call through the library handler; returns the CallToolResult."""
    req = types.CallToolRequest(
        method="tools/call",
        params=types.CallToolRequestParams(name=name, arguments=arguments),
    )
    return _run(zms.app.request_handlers[types.CallToolRequest](req)).root


def _text(result):
    return "".join(c.text for c in result.content if c.type == "text")


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


@unittest.skipUnless(HAVE_MCP, "mcp package not installed")
class TestUnknownTool(unittest.TestCase):
    def test_unknown_tool_is_error(self):
        res = _call("no_such_tool", {})
        self.assertTrue(res.isError)
        self.assertIn("Unknown tool: no_such_tool", _text(res))


@unittest.skipUnless(HAVE_MCP, "mcp package not installed")
class TestRunbookNames(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        base = Path(os.path.realpath(self._tmp.name))
        self.rb = base / "runbooks"
        (self.rb / "infra").mkdir(parents=True)
        (self.rb / "infra" / "good-one.yaml").write_text("name: good-one\n")
        (base / "evil.yaml").write_text("name: evil\n")
        os.symlink(base / "evil.yaml", self.rb / "infra" / "linked.yaml")
        self._patches = [
            mock.patch.object(zms, "RUNBOOKS_DIR", self.rb),
            mock.patch.dict(os.environ, {"ZHEN_MCP_ALLOW_EXEC": "1"}),
        ]
        for p in self._patches:
            p.start()

    def tearDown(self):
        for p in reversed(self._patches):
            p.stop()
        self._tmp.cleanup()

    def test_real_runbook_names_match_pattern(self):
        names = [p.stem for p in (REPO_ROOT / "runbooks").rglob("*.yaml")]
        self.assertTrue(names)
        for n in names:
            self.assertTrue(zms.RUNBOOK_NAME_RE.fullmatch(n), n)

    def test_show_valid(self):
        res = _call("runbook_show", {"name": "good-one"})
        self.assertFalse(res.isError, _text(res))
        self.assertIn("name: good-one", _text(res))

    def test_show_not_found_is_error(self):
        res = _call("runbook_show", {"name": "nope"})
        self.assertTrue(res.isError)

    BAD_NAMES = ["*", "../evil", "infra/good-one", "Good-One", "-x", "", "a" * 65,
                 "good-one\n", "good*", "[g]ood-one"]

    def test_show_rejects_bad_names(self):
        for name in self.BAD_NAMES:
            res = _call("runbook_show", {"name": name})
            self.assertTrue(res.isError, f"{name!r}: {_text(res)[:80]}")
            self.assertNotIn("name: evil", _text(res))
            self.assertNotIn("name: good-one", _text(res))

    def test_show_rejects_symlink_out_of_runbooks(self):
        res = _call("runbook_show", {"name": "linked"})
        self.assertTrue(res.isError)
        self.assertNotIn("name: evil", _text(res))

    def test_execute_rejects_bad_names_without_running(self):
        with mock.patch("subprocess.run") as run:
            for name in self.BAD_NAMES + ["linked"]:
                res = _call("runbook_execute", {"name": name, "dry_run": True})
                self.assertTrue(res.isError, f"{name!r}: {_text(res)[:80]}")
            run.assert_not_called()

    def test_execute_valid_runs_resolved_path(self):
        with mock.patch("subprocess.run") as run:
            run.return_value = mock.Mock(returncode=0, stdout="ok", stderr="")
            res = _call("runbook_execute", {"name": "good-one", "dry_run": True})
        self.assertFalse(res.isError, _text(res))
        cmd = run.call_args.args[0]
        self.assertEqual(cmd[-1], str(self.rb / "infra" / "good-one.yaml"))
        self.assertIn("--dry-run", cmd)


READ_ONLY_TOOLS = {"corpus_search", "file_read", "service_health", "runbook_list", "runbook_show"}


def _gate_env(**flags):
    """Patch os.environ with both gate flags removed, then `flags` set."""
    env = {k: v for k, v in os.environ.items()
           if k not in ("ZHEN_MCP_ALLOW_WRITE", "ZHEN_MCP_ALLOW_EXEC")}
    env.update(flags)
    return mock.patch.dict(os.environ, env, clear=True)


def _listed():
    res = _run(zms.app.request_handlers[types.ListToolsRequest](None)).root
    return {t.name for t in res.tools}


@unittest.skipUnless(HAVE_MCP, "mcp package not installed")
class TestWriteExecGate(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.root = Path(os.path.realpath(self._tmp.name))
        self.target = self.root / "f.txt"
        self.target.write_text("old")
        self._patch = mock.patch.object(zms, "ALLOWED_PATHS", [str(self.root)])
        self._patch.start()

    def tearDown(self):
        self._patch.stop()
        self._tmp.cleanup()

    def test_default_lists_read_only_tools(self):
        with _gate_env():
            self.assertEqual(_listed(), READ_ONLY_TOOLS)

    def test_flags_list_gated_tools(self):
        with _gate_env(ZHEN_MCP_ALLOW_WRITE="1"):
            self.assertEqual(_listed(), READ_ONLY_TOOLS | {"file_write", "file_patch"})
        with _gate_env(ZHEN_MCP_ALLOW_EXEC="1"):
            self.assertEqual(_listed(), READ_ONLY_TOOLS | {"runbook_execute"})
        with _gate_env(ZHEN_MCP_ALLOW_WRITE="1", ZHEN_MCP_ALLOW_EXEC="1"):
            self.assertEqual(len(_listed()), 8)

    def test_only_exact_one_enables(self):
        for v in ("", "0", "true", "yes", " 1"):
            with _gate_env(ZHEN_MCP_ALLOW_WRITE=v, ZHEN_MCP_ALLOW_EXEC=v):
                self.assertEqual(_listed(), READ_ONLY_TOOLS, repr(v))

    def test_write_disabled_is_error(self):
        with _gate_env():
            res = _call("file_write", {"path": str(self.root / "new.txt"), "content": "x"})
            self.assertTrue(res.isError)
            self.assertIn("ZHEN_MCP_ALLOW_WRITE=1", _text(res))
            res = _call("file_patch", {"path": str(self.target), "old_text": "old", "new_text": "new"})
            self.assertTrue(res.isError)
        self.assertFalse((self.root / "new.txt").exists())
        self.assertEqual(self.target.read_text(), "old")

    def test_exec_disabled_is_error(self):
        with _gate_env(ZHEN_MCP_ALLOW_WRITE="1"), mock.patch("subprocess.run") as run:
            res = _call("runbook_execute", {"name": "service-health-sweep", "dry_run": True})
            self.assertTrue(res.isError)
            self.assertIn("ZHEN_MCP_ALLOW_EXEC=1", _text(res))
            run.assert_not_called()

    def test_write_enabled_works(self):
        with _gate_env(ZHEN_MCP_ALLOW_WRITE="1"):
            res = _call("file_patch", {"path": str(self.target), "old_text": "old", "new_text": "new"})
            self.assertFalse(res.isError, _text(res))
        self.assertEqual(self.target.read_text(), "new")


if __name__ == "__main__":
    unittest.main()
