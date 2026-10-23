# SPDX-License-Identifier: GPL-3.0-or-later
"""Tests for thresholds.py: the PREREG rule at its boundaries, the hold-out
simulation and strict loading. Run: python -m pytest scripts/two-brains/"""
import importlib.util
import json
import os

import pytest

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("thresholds", os.path.join(HERE, "thresholds.py"))
th = importlib.util.module_from_spec(spec)
spec.loader.exec_module(th)


@pytest.mark.parametrize("L,C,want", [
    (0.29, 0.9, "claude_direct"),
    (0.30, 0.9, "local_first"),
    (0.84, 0.84, "local_first"),
    (0.85, 0.85, "local_only"),
    (0.85, 0.94, "local_only"),   # +9 pp
    (0.85, 0.95, "local_first"),  # +10 pp: Claude is >= 10 pp higher
    (1.0, None, "local_only"),
    (0.0, None, "claude_direct"),
])
def test_decide_boundaries(L, C, want):
    assert th.decide(L, C) == want


def row(task, backend, ok, cost=0, model="m"):
    return {"task": task, "backend": backend, "model": model, "pass": ok, "cost_nano_usd": cost, "latency_ms": 10}


def test_simulate():
    l_ok, l_bad = row("t", "local", True), row("t", "local", False)
    c_ok = row("t", "claude", True, 100)
    assert th.simulate("local_only", l_bad, c_ok) == (False, 0)
    assert th.simulate("local_first", l_ok, c_ok) == (True, 0)
    assert th.simulate("local_first", l_bad, c_ok) == (True, 100)
    assert th.simulate("claude_direct", l_ok, c_ok) == (True, 100)


def test_verify_rejects_a_route_that_loses_passes():
    split = {"a": {"id": "a", "family": "hard", "split": "holdout"}, "b": {"id": "b", "family": "hard", "split": "holdout"}}
    rows = [row("a", "local", False), row("b", "local", False), row("a", "claude", True, 10), row("b", "claude", True, 10)]
    fams = {"hard": {"route": "local_only"}}
    rep = th.verify(fams, rows, split, ("local", "m"), ("claude", "m"))
    assert rep["hard"]["adopted"] is False
    fams = {"hard": {"route": "local_first"}}
    rep = th.verify(fams, rows, split, ("local", "m"), ("claude", "m"))
    # local_first == claude here (all escalate): passes equal, but cost not <= 50%.
    assert rep["hard"]["checks"]["routed cost <= 50% claude"] is False
    assert rep["hard"]["adopted"] is False


def test_verify_adopts_a_cheaper_equal_route():
    split = {t: {"id": t, "family": "hard", "split": "holdout"} for t in "abcd"}
    rows = [row(t, "local", t != "d") for t in "abcd"] + [row(t, "claude", True, 10) for t in "abcd"]
    rep = th.verify({"hard": {"route": "local_first"}}, rows, split, ("local", "m"), ("claude", "m"))
    assert rep["hard"]["routed_pass"] == 4 and rep["hard"]["adopted"] is True


def test_textbook_pooling_and_latest_result_wins():
    split = {"syntax-x": {"id": "syntax-x", "family": "syntax", "split": "tune"},
             "review-y": {"id": "review-y", "family": "review", "split": "tune"}}
    rows = [row("syntax-x", "local", False), row("syntax-x", "local", True), row("review-y", "local", True)]
    g = th.group(rows, split, "tune")
    assert set(g[("local", "m")]) == {"textbook"}
    assert th.rate(g[("local", "m")]["textbook"]) == 1.0


def test_load_results_strict(tmp_path):
    p = tmp_path / "r.jsonl"
    p.write_text(json.dumps(row("a", "local", True)) + "\nnot json\n")
    with pytest.raises(SystemExit):
        th.load_results([str(p)])
