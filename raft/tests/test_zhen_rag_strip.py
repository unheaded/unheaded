# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""_strip_tool_call_json (S-CHAMPION-HARDENING Step 182, RT-14/16): a prior
turn re-sent to the model must not carry a {"tool_call": ...} object, at any
nesting depth, while the prose around it survives."""
import sys
import types
from pathlib import Path

if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))

import zhen_rag

strip = zhen_rag._strip_tool_call_json
MARK = "[tool_call payload stripped]"


def test_trailing_object_removed_prose_kept():
    out = strip('Sure, running it now. {"tool_call": {"name": "file_write", "args": {}}}')
    assert out == "Sure, running it now. " + MARK


def test_embedded_object_removed():
    out = strip('before {"tool_call": {"name": "x"}} after')
    assert out == "before " + MARK + " after"


def test_nested_args_one_level():
    out = strip('{"tool_call": {"name": "runbook_execute", "args": {"path": "a.yaml"}}}')
    assert out == MARK


def test_nested_args_deep():
    payload = '{"tool_call": {"name": "file_write", "args": {"path": "x", "meta": {"a": {"b": 1}}}}}'
    assert strip("ok " + payload + " done") == "ok " + MARK + " done"


def test_braces_inside_strings_do_not_end_the_object():
    payload = '{"tool_call": {"name": "file_write", "args": {"content": "func f() { return } }}"}}}'
    assert strip(payload + " tail") == MARK + " tail"


def test_two_objects_both_removed():
    out = strip('{"tool_call": {"name": "a"}} and {"tool_call": {"name": "b", "args": {"x": {"y": 2}}}}')
    assert out == MARK + " and " + MARK


def test_spacing_and_newlines_inside():
    out = strip('x {\n  "tool_call" :\n  {"name": "a"}\n} y')
    assert out == "x " + MARK + " y"


def test_bare_brace_and_plain_json_left_alone():
    for text in ["a { b", '{"name": "not a tool call"}', "if x { y }", ""]:
        assert strip(text) == text


def test_unterminated_tool_call_is_still_cut():
    # A truncated payload must not reach the model as a template either.
    assert strip('text {"tool_call": {"name": "file_write", "args": {"path": "x"') == "text " + MARK
