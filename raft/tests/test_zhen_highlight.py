# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""Unit tests for raft/zhen_highlight.py (no Flask, no RAG, no network)."""

import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import zhen_highlight as zh  # noqa: E402


def _text(html):
    """The visible text of highlighted HTML: tags dropped, entities decoded."""
    s = re.sub(r'<[^>]+>', '', html)
    return s.replace('&lt;', '<').replace('&gt;', '>').replace('&quot;', '"') \
            .replace('&#39;', "'").replace('&amp;', '&')


def test_languages_zhen_answers_use():
    for lang in ('bash', 'sh', 'python', 'go', 'rust', 'html', 'css', 'js',
                 'yaml', 'json', 'diff', 'sql', 'c++', 'ini', 'nginx', 'systemd'):
        assert zh.lexer_for(lang) is not None, lang


def test_filename_info_strings():
    assert zh.lexer_for('file:cmd/x/main.go').name == 'Go'
    assert zh.lexer_for('download:runbook.yaml').name == 'YAML'
    assert zh.lexer_for('file:no-extension-here') is None


def test_unknown_or_empty_stays_plain():
    assert zh.lexer_for('') is None
    assert zh.lexer_for('   ') is None
    assert zh.lexer_for('not-a-language') is None
    assert zh.highlight_blocks([{'lang': '', 'code': 'x'}]) == [None]


def test_output_is_spans_only_and_keeps_the_text():
    code = 'if a < b && c > "d" { return `<script>alert(1)</script>` }\n'
    [html] = zh.highlight_blocks([{'lang': 'go', 'code': code}])
    assert '<script' not in html
    assert set(re.findall(r'<(/?\w+)', html)) <= {'span', '/span'}
    assert 'class="hl-' in html
    assert _text(html) == code.rstrip('\n')


def test_leading_blank_lines_kept():
    [html] = zh.highlight_blocks([{'lang': 'python', 'code': '\n\nx = 1'}])
    assert _text(html) == '\n\nx = 1'


def test_limits_and_bad_input():
    big = 'x' * (zh.MAX_BLOCK_BYTES + 1)
    assert zh.highlight_blocks([{'lang': 'python', 'code': big}]) == [None]
    many = [{'lang': 'python', 'code': 'x'}] * (zh.MAX_BLOCKS + 2)
    out = zh.highlight_blocks(many)
    assert len(out) == len(many)
    assert out[zh.MAX_BLOCKS - 1] is not None and out[zh.MAX_BLOCKS:] == [None, None]
    assert zh.highlight_blocks(['nope', {'lang': 1, 'code': 'x'}, {'lang': 'go'}]) == [None, None, None]


def test_stylesheet_scoped_to_answers():
    css = zh.stylesheet()
    assert '.msg.zhen pre code .hl-k' in css
    for line in css.splitlines():
        assert line.startswith('.msg.zhen pre code .hl-'), line
