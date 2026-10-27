# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The shared redactor for Python writers (retention policy v1 §11).

Reads pkg/zhencapture/redact_patterns.json, the one rule file the Go
capture writer and well-retention's archive use (secrets, tokens, keys, URL
passwords, email addresses), and applies it the same way: in order, group 1
kept before the tag and group 2 after it. No second rule set.

zhen_app.py's _pg_log runs every conversation row through redact() before
the INSERT into zhen_conversations; when the rules cannot be loaded it
persists nothing (load() raises, the caller refuses).
"""

import json
import os
import re
from pathlib import Path

DEFAULT_PATTERNS = Path(__file__).resolve().parents[1] / 'pkg' / 'zhencapture' / 'redact_patterns.json'


def load(path=None):
    """Compile the patterns file: [(name, regex)]. Raises on any problem."""
    p = Path(path or os.environ.get('ZHEN_REDACT_PATTERNS') or DEFAULT_PATTERNS)
    data = json.loads(p.read_text(encoding='utf-8'))
    if data.get('version') != 1 or not data.get('patterns'):
        raise ValueError(f'{p}: version {data.get("version")!r} with {len(data.get("patterns") or [])} patterns')
    out = []
    for pat in data['patterns']:
        out.append((pat['name'], re.compile(pat['regex'])))
    return out


def _replacer(name):
    def repl(m):
        before = (m.group(1) or '') if m.re.groups >= 1 else ''
        after = (m.group(2) or '') if m.re.groups >= 2 else ''
        return f'{before}[REDACTED:{name}]{after}'
    return repl


def redact(text, patterns):
    """Return (redacted text, number of replacements)."""
    n = 0
    for name, rx in patterns:
        text, k = rx.subn(_replacer(name), text)
        n += k
    return text, n


def _walk(v, patterns):
    if isinstance(v, str):
        return redact(v, patterns)[0]
    if isinstance(v, list):
        return [_walk(x, patterns) for x in v]
    if isinstance(v, dict):
        return {k: _walk(x, patterns) for k, x in v.items()}
    return v


def redact_json(text, patterns):
    """Redact every string inside a JSON document and re-encode it; a
    document that does not parse becomes '[]' rather than passing through."""
    try:
        doc = json.loads(text)
    except (TypeError, ValueError):
        return '[]'
    return json.dumps(_walk(doc, patterns))
