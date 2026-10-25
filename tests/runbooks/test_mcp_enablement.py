# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/security/mcp-write-exec-enablement.yaml (R10).

The runbook's own tool-list checks start the real MCP server through
`zhen-claude -dry-run` (no credential, nothing sent) with the write gate off
and on; the answers must flip. The repo's .mcp.json must keep both gates off."""
import json
import subprocess

import harness
import pytest

RB = "runbooks/security/mcp-write-exec-enablement.yaml"


def gated_checks():
    out = []
    for s in harness.steps(harness.load(RB)):
        if s["action"] != "shell":
            continue
        for field in ("command", "verify"):
            if "-tools mcp" in (s.get(field) or ""):
                out.append((s["name"], field))
    return out


def mcp_importable():
    for py in (str(harness.Path.home() / ".venv/zhen/bin/python"), "python3"):
        if subprocess.run([py, "-c", "import mcp"], capture_output=True, check=False).returncode == 0:
            return True
    return False


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(harness.load(RB))]


def test_repo_mcp_json_keeps_both_gates_off():
    cfg = json.loads((harness.REPO / ".mcp.json").read_text())
    for name, srv in cfg["mcpServers"].items():
        env = srv.get("env") or {}
        assert env.get("ZHEN_MCP_ALLOW_WRITE", "0") != "1", name
        assert env.get("ZHEN_MCP_ALLOW_EXEC", "0") != "1", name


def test_enable_and_disable_are_manual_and_the_work_is_attended():
    st = harness.steps(harness.load(RB))
    for s in st:
        c = s.get("command") or ""
        if "export ZHEN_MCP_ALLOW" in c or "unset ZHEN_MCP_ALLOW" in c:
            assert s["action"] == "manual", s["name"]
    assert "attended" in (harness.REPO / RB).read_text().lower()


def test_preflight_guard_passes_on_this_repo():
    rb = harness.load(RB)
    guards = [s["name"] for s in harness.steps(rb) if s["action"] == "shell" and ".mcp.json" in (s.get("command") or "")]
    assert guards, "no step checks the committed .mcp.json"
    for g in guards:
        harness.run_step(rb, g, env={"REPO": str(harness.REPO)}, cwd=harness.REPO)


@pytest.mark.skipif(not mcp_importable(), reason="the MCP server needs the mcp package (~/.venv/zhen)")
def test_tool_list_checks_flip_with_the_gate():
    rb = harness.load(RB)
    checks = gated_checks()
    assert checks, "no step lists the MCP tools"
    off, on = {}, {}
    for name, field in checks:
        base = {"REPO": str(harness.REPO), "ZHEN_MCP_ALLOW_WRITE": "", "ZHEN_MCP_ALLOW_EXEC": ""}
        off[(name, field)] = harness.run_step(rb, name, env=base, field=field, cwd=harness.REPO, check=False, timeout=300).returncode
        on[(name, field)] = harness.run_step(rb, name, env={**base, "ZHEN_MCP_ALLOW_WRITE": "1"}, field=field, cwd=harness.REPO,
                                             check=False, timeout=300).returncode
    assert any(off[k] != 0 and on[k] == 0 for k in checks), f"no check passes only when enabled: off={off} on={on}"
    assert any(off[k] == 0 and on[k] != 0 for k in checks), f"no check passes only when disabled: off={off} on={on}"
