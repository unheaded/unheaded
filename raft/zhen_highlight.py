# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""Server-side syntax highlighting for Zhen's chat answers (Pygments).

The page renders every fenced block as escaped plain text first, then asks
POST /api/v1/highlight for the blocks that name a language and swaps in the
HTML that comes back. A block this module cannot or will not highlight comes
back as None and stays plain, so highlighting never loses text.

Pygments escapes everything it emits; the only markup in the output is its
own <span class="hl-*"> tags. No language guessing: an unlabelled block stays
plain, which is predictable and cheap.
"""

from typing import ClassVar

from pygments import highlight
from pygments.formatters import HtmlFormatter
from pygments.lexers import get_lexer_by_name, get_lexer_for_filename
from pygments.style import Style
from pygments.token import (
    Comment,
    Error,
    Generic,
    Keyword,
    Name,
    Number,
    Operator,
    String,
    Token,
)
from pygments.util import ClassNotFound

MAX_BLOCKS = 32
MAX_BLOCK_BYTES = 64 * 1024
MAX_TOTAL_BYTES = 256 * 1024

# Info-string prefixes whose remainder is a filename, not a language:
# ```file:path (the zhen-eval answer convention) and ```download:name.
_FILENAME_PREFIXES = ('file:', 'download:')


class ZhenStyle(Style):
    """The page's dark palette (raft/static/index.html :root), lifted for
    contrast on the code background."""
    background_color = 'transparent'
    styles: ClassVar[dict] = {
        Token: '#c9c9c9',
        Comment: 'italic #777777',
        Comment.Preproc: 'noitalic #8a6aaa',
        Keyword: '#ff7a2e',
        Keyword.Type: '#d9a066',
        Keyword.Constant: '#b394d6',
        Name.Builtin: '#8fb0d0',
        Name.Function: '#8fb0d0',
        Name.Class: 'bold #8fb0d0',
        Name.Decorator: '#b394d6',
        Name.Tag: '#ff7a2e',
        Name.Attribute: '#d9a066',
        Name.Variable: '#c9a0c9',
        String: '#8fbf7f',
        String.Escape: '#d9a066',
        String.Interpol: '#d9a066',
        Number: '#b394d6',
        Operator.Word: '#ff7a2e',
        Generic.Inserted: '#8fbf7f',
        Generic.Deleted: '#e07a7a',
        Generic.Heading: 'bold #c9c9c9',
        Generic.Subheading: '#8fb0d0',
        Generic.Prompt: '#777777',
        Error: '#e07a7a',
    }


_FORMATTER = HtmlFormatter(style=ZhenStyle, nowrap=True, classprefix='hl-')


def stylesheet():
    """CSS for the hl-* classes, scoped to code inside Zhen's answers. Token
    rules only: get_style_defs() would also emit page-wide pre/linenos rules."""
    return '\n'.join(_FORMATTER.get_token_style_defs('.msg.zhen pre code'))


def lexer_for(info):
    """The lexer for a fence info string, or None to leave the block plain.

    ``info`` is everything after the opening ```: a language name ("go",
    "c++"), or "file:<path>" / "download:<name>" (lexer from the filename).
    """
    info = (info or '').strip()
    if not info:
        return None
    try:
        for prefix in _FILENAME_PREFIXES:
            if info.startswith(prefix):
                return get_lexer_for_filename(info[len(prefix):].strip(), stripnl=False)
        return get_lexer_by_name(info.split()[0].lower(), stripnl=False)
    except ClassNotFound:
        return None


def highlight_blocks(blocks):
    """Highlight [{'lang': str, 'code': str}, ...] into a same-length list of
    HTML strings, None where the block stays plain (unknown language, over a
    size limit, or past MAX_BLOCKS)."""
    out = []
    total = 0
    for i, block in enumerate(blocks):
        code = block.get('code') if isinstance(block, dict) else None
        lang = block.get('lang') if isinstance(block, dict) else None
        if i >= MAX_BLOCKS or not isinstance(code, str) or not isinstance(lang, str):
            out.append(None)
            continue
        size = len(code.encode('utf-8'))
        total += size
        lexer = lexer_for(lang)
        if lexer is None or size > MAX_BLOCK_BYTES or total > MAX_TOTAL_BYTES:
            out.append(None)
            continue
        # Pygments ends output with a newline the plain rendering doesn't have.
        out.append(highlight(code, lexer, _FORMATTER).rstrip('\n'))
    return out
