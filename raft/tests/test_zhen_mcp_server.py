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
import json
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

    BAD_NAMES = ("*", "../evil", "infra/good-one", "Good-One", "-x", "", "a" * 65,
                 "good-one\n", "good*", "[g]ood-one")

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
            for name in (*self.BAD_NAMES, "linked"):
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


READ_ONLY_TOOLS = {"corpus_search", "file_read", "service_health", "runbook_list", "runbook_show", "local_llm"}


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
            self.assertEqual(len(_listed()), 9)

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


class _FakeLlama:
    """A stdlib llama-server stand-in on 127.0.0.1, in a thread.

    `reply` is the JSON body (dict) or raw bytes to send; `status` the HTTP
    status; `delay` seconds to sleep first. Requests land in `requests`.
    """

    def __init__(self, reply=None, status=200, delay=0.0):
        import http.server
        import threading

        self.reply = reply if reply is not None else {
            "model": "qwen2.5-coder-7b-instruct",
            "choices": [{"index": 0, "finish_reason": "stop",
                         "message": {"role": "assistant", "content": "local says hi"}}],
        }
        self.status, self.delay, self.requests = status, delay, []
        fake = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                n = int(self.headers.get("Content-Length", "0"))
                fake.requests.append((self.path, json.loads(self.rfile.read(n))))
                if fake.delay:
                    import time
                    time.sleep(fake.delay)
                body = fake.reply if isinstance(fake.reply, bytes) else json.dumps(fake.reply).encode()
                try:
                    self.send_response(fake.status)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                except (BrokenPipeError, ConnectionResetError):
                    pass

            def log_message(self, *a):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = True
        self.url = f"http://127.0.0.1:{self.server.server_address[1]}"
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()


def _closed_port_url():
    import socket
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return f"http://127.0.0.1:{port}"


def _llm_env(url, **extra):
    env = {k: v for k, v in os.environ.items()
           if not k.startswith("ZHEN_LOCAL_LLM_") and k != "ZHEN_CAPTURE_DIR"}
    env["ZHEN_LOCAL_LLM_URL"] = url
    env.update(extra)
    return mock.patch.dict(os.environ, env, clear=True)


@unittest.skipUnless(HAVE_MCP, "mcp package not installed")
class TestLocalLLM(unittest.TestCase):
    def setUp(self):
        self.fake = _FakeLlama()
        self.addCleanup(self.fake.close)

    def test_listed_by_default_and_read_only(self):
        with _gate_env():
            self.assertIn("local_llm", _listed())
        self.assertNotIn("local_llm", zms.GATED_TOOLS)

    def test_answer_and_request_shape(self):
        with _llm_env(self.fake.url):
            res = _call("local_llm", {"prompt": "hello", "system": "be brief",
                                      "max_tokens": 99999, "temperature": 0.7})
        self.assertFalse(res.isError, _text(res))
        text = _text(res)
        self.assertIn("local says hi", text)
        self.assertIn("Untrusted model output", text)
        self.assertIn("qwen2.5-coder-7b-instruct", text)
        path, body = self.fake.requests[0]
        self.assertEqual(path, "/v1/chat/completions")
        self.assertEqual(body["messages"], [{"role": "system", "content": "be brief"},
                                            {"role": "user", "content": "hello"}])
        self.assertEqual(body["max_tokens"], zms.LOCAL_LLM_MAX_TOKENS_CAP)
        self.assertEqual(body["temperature"], 0.7)
        self.assertFalse(body["stream"])
        self.assertEqual(body["model"], "local")

    def test_default_url(self):
        with _llm_env(""):
            url, model, timeout = zms.local_llm_config()
        self.assertEqual((url, model, timeout), ("http://127.0.0.1:8081", "local", 120.0))

    def test_server_down_is_clean_error(self):
        with _llm_env(_closed_port_url()):
            res = _call("local_llm", {"prompt": "hello"})
        self.assertTrue(res.isError)
        self.assertIn("local LLM unavailable", _text(res))
        self.assertIn("nothing was started", _text(res))

    def test_timeout_is_error(self):
        slow = _FakeLlama(delay=2.0)
        self.addCleanup(slow.close)
        with _llm_env(slow.url, ZHEN_LOCAL_LLM_TIMEOUT="0.3"):
            res = _call("local_llm", {"prompt": "hello"})
        self.assertTrue(res.isError)
        self.assertIn("did not answer within 0.3s", _text(res))

    def test_http_error_and_bad_bodies(self):
        cases = [
            (_FakeLlama(reply={"error": "loading model"}, status=503), "HTTP 503"),
            (_FakeLlama(reply=b"not json"), "not an OpenAI chat completion"),
            (_FakeLlama(reply={"choices": []}), "not an OpenAI chat completion"),
            (_FakeLlama(reply={"choices": [{"message": {"content": "  "}}]}), "empty answer"),
            (_FakeLlama(reply=b"x" * (zms.LOCAL_LLM_MAX_BODY_BYTES + 10)), "exceeds"),
        ]
        for fake, want in cases:
            self.addCleanup(fake.close)
            with _llm_env(fake.url):
                res = _call("local_llm", {"prompt": "hello"})
            self.assertTrue(res.isError, want)
            self.assertIn(want, _text(res))

    def test_long_answer_is_cut(self):
        long = _FakeLlama(reply={"choices": [{"message": {"content": "a" * (zms.LOCAL_LLM_MAX_ANSWER_CHARS + 500)}}]})
        self.addCleanup(long.close)
        with _llm_env(long.url):
            res = _call("local_llm", {"prompt": "hello"})
        self.assertFalse(res.isError)
        self.assertIn("[truncated by zhen_mcp_server]", _text(res))
        self.assertLess(len(_text(res)), zms.LOCAL_LLM_MAX_ANSWER_CHARS + 1000)

    def test_bad_arguments_never_reach_the_server(self):
        bad = [
            {"prompt": ""}, {"prompt": "   "}, {"prompt": 5},
            {"prompt": "x", "system": 3},
            {"prompt": "x", "max_tokens": 0}, {"prompt": "x", "max_tokens": True},
            {"prompt": "x", "temperature": 3}, {"prompt": "x", "temperature": "hot"},
            {"prompt": "x" * (zms.LOCAL_LLM_MAX_PROMPT_BYTES + 1)},
        ]
        with _llm_env(self.fake.url):
            for args in bad:
                res = _call("local_llm", args)
                self.assertTrue(res.isError, repr(args)[:60])
        self.assertEqual(self.fake.requests, [])

    def test_bad_config_is_error(self):
        for env in ({"ZHEN_LOCAL_LLM_URL": "file:///etc/passwd"},
                    {"ZHEN_LOCAL_LLM_TIMEOUT": "abc"},
                    {"ZHEN_LOCAL_LLM_TIMEOUT": "9999"}):
            with _llm_env(self.fake.url, **env):
                res = _call("local_llm", {"prompt": "hello"})
            self.assertTrue(res.isError, env)
        self.assertEqual(self.fake.requests, [])


def _fake_secret(prefix, n, fill):
    # Assembled at run time: no secret-shaped literal in the tree.
    return prefix + fill * n


@unittest.skipUnless(HAVE_MCP, "mcp package not installed")
class TestRedact(unittest.TestCase):
    """Same patterns file as Go's pkg/zhencapture; same expectations."""

    def test_patterns_compile_and_match_go_cases(self):
        self.assertGreaterEqual(len(zms.load_redact_patterns()), 10)
        pem = ("-----BEGIN " + "RSA PRIVATE KEY-----\nMIIEow" + "A" * 40
               + "\n-----END " + "RSA PRIVATE KEY-----")
        cases = [
            ("key " + _fake_secret("sk-ant-api03-", 30, "x"), "key [REDACTED:anthropic_key]"),
            ("id " + _fake_secret("AKIA", 16, "Q") + ".", "id [REDACTED:aws_access_key]."),
            ("before\n" + pem + "\nafter", "before\n[REDACTED:private_key]\nafter"),
            ("Authorization: Bearer " + "t" * 24, "Authorization: Bearer [REDACTED:bearer]"),
            ("postgres://app_zhen:" + "w" * 12 + "@west:5432/db",
             "postgres://app_zhen:[REDACTED:url_password]@west:5432/db"),
            ('PGPASSWORD="' + "p" * 10 + '" psql', 'PGPASSWORD="[REDACTED:assignment]" psql'),
            ("the password policy is strict", "the password policy is strict"),
            ("password=abc", "password=abc"),
        ]
        for raw, want in cases:
            got, _ = zms.redact(raw)
            self.assertEqual(got, want)

    def test_idempotent(self):
        once, n1 = zms.redact("password=" + "q" * 12)
        twice, n2 = zms.redact(once)
        self.assertEqual((n1, n2), (1, 0))
        self.assertEqual(once, twice)


@unittest.skipUnless(HAVE_MCP, "mcp package not installed")
class TestCapture(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.dir = os.path.join(self._tmp.name, "cap")
        self.fake = _FakeLlama(reply={"model": "qwen", "choices": [{"finish_reason": "stop", "message": {
            "content": "use " + _fake_secret("sk-ant-", 30, "k") + " here"}}]})
        self.addCleanup(self.fake.close)

    def _lines(self):
        out = []
        for p in sorted(Path(self.dir).glob("zhen-capture-*.jsonl")):
            self.assertEqual(os.stat(p).st_mode & 0o777, 0o600)
            out += [json.loads(line) for line in p.read_text().splitlines()]
        return out

    def test_off_by_default(self):
        with _llm_env(self.fake.url):
            res = _call("local_llm", {"prompt": "hi"})
        self.assertFalse(res.isError)
        self.assertFalse(os.path.exists(self.dir))

    def test_writes_redacted_record(self):
        prompt = "my password=" + "h" * 12
        with _llm_env(self.fake.url, ZHEN_CAPTURE_DIR=self.dir):
            res = _call("local_llm", {"prompt": prompt, "system": "sys"})
        self.assertFalse(res.isError, _text(res))
        self.assertEqual(os.stat(self.dir).st_mode & 0o777, 0o700)
        lines = self._lines()
        self.assertEqual(len(lines), 1)
        rec = lines[0]
        raw = json.dumps(rec)
        self.assertNotIn("hhhhhhhh", raw)
        self.assertNotIn("kkkkkkkk", raw)
        self.assertEqual(rec["schema"], "zhen-capture/v1")
        self.assertEqual(rec["source"], "zhen-mcp/local_llm")
        self.assertEqual(rec["route"], "local")
        self.assertEqual(rec["system"], "sys")
        self.assertEqual(rec["local"]["model"], "qwen")
        self.assertIn("[REDACTED:anthropic_key]", rec["final"])
        self.assertEqual(rec["redactions"], 3)  # prompt, local.answer, final
        self.assertFalse(rec["truncated"])
        self.assertEqual(len(rec["id"]), 32)

    def test_linking_fields(self):
        # S-TWO-BRAINS Step 351: every call gets its own conversation_id;
        # task_family is passed through when the caller names a valid one.
        with _llm_env(self.fake.url, ZHEN_CAPTURE_DIR=self.dir):
            _call("local_llm", {"prompt": "a", "task_family": "repo-task"})
            _call("local_llm", {"prompt": "b", "task_family": "Not A Family!"})
            _call("local_llm", {"prompt": "c"})
        a, b, c = self._lines()
        self.assertEqual(a["task_family"], "repo-task")
        self.assertNotIn("task_family", b)
        self.assertNotIn("task_family", c)
        ids = {r["conversation_id"] for r in (a, b, c)}
        self.assertEqual(len(ids), 3)
        for r in (a, b, c):
            self.assertRegex(r["conversation_id"], r"^[0-9a-f]{32}$")
            self.assertNotIn("author_kind", r)

    def test_failure_is_captured_too(self):
        with _llm_env(_closed_port_url(), ZHEN_CAPTURE_DIR=self.dir):
            res = _call("local_llm", {"prompt": "hi"})
        self.assertTrue(res.isError)
        rec = self._lines()[0]
        self.assertIn("unavailable", rec["local"]["error"])
        self.assertEqual(rec["final"], "")

    def test_refused_inside_repo_but_answer_still_returned(self):
        inside = REPO_ROOT / "raft" / "tests" / "capture-should-not-exist"
        with _llm_env(self.fake.url, ZHEN_CAPTURE_DIR=str(inside)):
            res = _call("local_llm", {"prompt": "hi"})
        self.assertFalse(res.isError, _text(res))
        self.assertIn("capture failed", _text(res))
        self.assertIn("work tree", _text(res))
        self.assertFalse(inside.exists())

    def test_refuses_open_permissions_and_symlinked_file(self):
        os.makedirs(self.dir, mode=0o755)
        os.chmod(self.dir, 0o755)
        with self.assertRaisesRegex(ValueError, "chmod 700"):
            zms.check_capture_dir(self.dir)
        os.chmod(self.dir, 0o700)
        from datetime import UTC, datetime
        day = datetime.now(UTC).strftime("%Y-%m-%d")
        target = os.path.join(self._tmp.name, "elsewhere")
        os.symlink(target, os.path.join(self.dir, f"zhen-capture-{day}.jsonl"))
        with self.assertRaises(OSError):
            zms.write_capture(self.dir, {"prompt": "x"})
        self.assertFalse(os.path.exists(target))

    def test_caps_long_strings(self):
        rec = zms.write_capture(self.dir, {"prompt": "é" * (zms.CAPTURE_MAX_STRING_BYTES)})
        self.assertTrue(rec["truncated"])
        self.assertLessEqual(len(rec["prompt"].encode()), zms.CAPTURE_MAX_STRING_BYTES)
        self.assertTrue(rec["prompt"].endswith(zms.CAPTURE_TRUNCATION_MARKER))


if __name__ == "__main__":
    unittest.main()
