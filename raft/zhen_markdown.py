# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""Markdown to HTML for the source viewer (/view/source).

A sheet can come from an `external` source, which the trust model calls
"possibly poisoned", so the Markdown is hostile input. The rules:

- Every byte of the sheet that reaches the output goes through html.escape.
  Raw HTML in a sheet is shown as text, comments included.
- The output uses a fixed set of tags. The only attributes are a vetted
  `href`, a generated `id` (always `md-...`), fixed `class` names, `rel`,
  an escaped `title`, and a numeric `start` on ordered lists.
- A link is followed only when it is http, https or mailto, or an in-page
  `#anchor`. Everything else fails closed and is shown as text (the rule in
  docdreamer's src/links.rs). Images are never fetched: the alt text and the
  address are shown instead.
- Nothing is dropped. A construct this module does not know (reference links,
  footnotes, math, HTML blocks) stays as readable escaped text. An HTML
  comment, which a browser would hide, is shown as a marked plain block.
- Work is bounded: a size limit, a nesting limit, a per-run inline limit, a
  highlighting budget and a time budget. Past a limit the rest is shown plain.

Borrowed from docdreamer: control and text-direction characters are removed
(src/document.rs), YAML front matter is folded away instead of shown as
content, and `---` is a rule after a blank line but a heading underline
directly under text (src/pagination.rs).

Standard library only, plus zhen_highlight (Pygments) for fenced code.
tests/test_zhen_markdown.py parses the output and fails on any tag or
attribute outside the allow-list.
"""

import bisect
import hashlib
import html
import re
import threading
import time
import unicodedata
from collections import OrderedDict
from urllib.parse import unquote

import zhen_highlight

MAX_DOC_CHARS = 2 * 1024 * 1024
MAX_SECONDS = 2.0                 # the slowest real sheet takes about 0.25 s
MAX_DEPTH = 16                    # nested quotes and lists; deeper shows plain
MAX_INLINE_CHARS = 64 * 1024      # one paragraph, heading or table cell
MAX_DELIMS = 2000                 # emphasis runs in one inline run
MAX_BRACKETS = 1000               # open [ in one inline run
MAX_DEST_CHARS = 2000             # a link destination, a link title, a bare URL
MAX_HL_BLOCK_CHARS = 64 * 1024
MAX_HL_TOTAL_CHARS = 768 * 1024
MAX_OUTLINE = 600
MAX_TABLE_COLS = 64
MAX_FRONT_MATTER_LINES = 200
ID_PREFIX = 'md-'

# What the output may contain; the tests hold the renderer to this.
ALLOWED_TAGS = frozenset({
    'p', 'br', 'hr', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'a', 'strong', 'em',
    'del', 'code', 'pre', 'blockquote', 'ul', 'ol', 'li', 'table', 'thead',
    'tbody', 'tr', 'th', 'td', 'div', 'span', 'details', 'summary',
})
ALLOWED_ATTRS = frozenset({'href', 'id', 'class', 'rel', 'title', 'start'})
ALLOWED_SCHEMES = ('http', 'https', 'mailto')

_ASCII_PUNCT = frozenset('!"#$%&\'()*+,-./:;<=>?@[\\]^_`{|}~')

# C0/C1 controls (not \n, \t), text-direction overrides (Trojan Source,
# CVE-2021-42574), lone surrogates, BOM.
_CTRL = re.compile('[\x00-\x08\x0b-\x1f\x7f-\x9f\u200e\u200f\u202a-\u202e'
                   '\u2066-\u2069\ud800-\udfff\ufeff]')

_FENCE = re.compile(r'^( {0,3})(`{3,}|~{3,})(.*)$')
_ATX = re.compile(r'^ {0,3}(#{1,6})(?:[ \t]+(.*))?$')
_HR = re.compile(r'^ {0,3}(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$')
_SETEXT = re.compile(r'^ {0,3}(=+|-+)[ \t]*$')
_BQ = re.compile(r'^ {0,3}>')
_ITEM = re.compile(r'^( {0,3})([-*+]|\d{1,9}[.)])(?:([ \t]+)(.*)|[ \t]*)$')
_TASK = re.compile(r'^\[([ xX])\](?:[ \t]+|$)')
_DELIM_ROW = re.compile(r'^[ \t|:\-]+$')
_DELIM_CELL = re.compile(r'^:?-+:?$')
_PIPE = re.compile(r'(?<!\\)\|')
_YAML_KEY = re.compile(r'^[A-Za-z_][\w .\-]*:(?:\s|$)')

_SPECIAL = re.compile(r'[\\`*_~\[\]!<&\n]|https?://')
_TICKS = re.compile(r'`+')
_AUTOLINK = re.compile(r'<([A-Za-z][A-Za-z0-9+.\-]{1,31}:[^\s<>]{0,' + str(MAX_DEST_CHARS) + r'})>')
_BARE_URL = re.compile(r'https?://[^\s<>\[\]"`]{1,' + str(MAX_DEST_CHARS) + r'}')
_ENTITY = re.compile(r'&(?:#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]{1,31});')
_ESCAPED = re.compile(r'\\([!-/:-@\[-`{-~])')
_SCHEME = re.compile(r'^([A-Za-z][A-Za-z0-9+.\-]*):')
_WEB = re.compile(r'^[A-Za-z]+://[^\s/\\?#]')
_TAG = re.compile(r'<[^>]*>')
_SLUG_DROP = re.compile(r'[^\w\- ]')

_REL = 'rel="noopener noreferrer"'


def _esc(s):
    return html.escape(s, quote=True)


# ── link policy ─────────────────────────────────────────────────────────

def classify_link(dest):
    """(kind, href) for a link destination. kind is 'web', 'mail' or 'anchor'
    when the link may be followed (href is then safe to emit once escaped),
    'relative' for a schemeless path, 'none' for an empty one, and 'blocked'
    for everything else. Fails closed: only a scheme positively on the
    allow-list is followed."""
    d = dest.strip()
    if not d:
        return 'none', ''
    if any(c.isspace() or c < ' ' or c == '\x7f' for c in d):
        return 'blocked', d
    if d.startswith('#'):
        return 'anchor', '#' + ID_PREFIX + slug(unquote(d[1:]))
    m = _SCHEME.match(d)
    if not m:
        # No scheme. "//host/path" would take the page's scheme: not a link.
        return ('blocked' if d.startswith(('//', '\\')) else 'relative'), d
    scheme = m.group(1).lower()
    if scheme in ('http', 'https') and _WEB.match(d):
        return 'web', d
    if scheme == 'mailto' and '@' in d:
        return 'mail', d
    return 'blocked', d


def slug(text):
    """GitHub-style heading slug: lower case, punctuation dropped, spaces to
    hyphens."""
    return _SLUG_DROP.sub('', text.strip().lower()).replace(' ', '-')[:80]


# ── inline ──────────────────────────────────────────────────────────────
#
# Nodes are finished HTML strings, emphasis runs (dict), open brackets (list)
# and deferred links (tuple). A deferred link becomes an <a> only at the top
# level: inside another link's text it is shown as text, so anchors never nest.

def _is_punct(c):
    if c.isalnum() or c.isspace():
        return False
    return unicodedata.category(c)[0] in 'PS'


def _deferred(node, nolink):
    if node[0] == 'auto':
        _, href, shown = node
        if nolink:
            return shown
        return f'<a href="{_esc(href)}" {_REL}>{shown}</a>'
    _, alt, dest = node  # 'img': never fetched
    out = f'<span class="md-image">image: {alt}</span>' if alt else '<span class="md-image">image</span>'
    kind, href = classify_link(dest)
    if kind == 'web' and not nolink:
        return f'{out} <a href="{_esc(href)}" {_REL}>{_esc(href)}</a>'
    if dest.strip():
        return f'{out} <span class="md-dest">{_esc(dest.strip()[:300])}</span>'
    return out


def _join(nodes, nolink):
    out = []
    for x in nodes:
        t = type(x)
        if t is str:
            out.append(x)
        elif t is dict:
            out.append(x['ch'] * x['n'])
        elif t is list:
            out.append('![' if x[1] else '[')
        else:
            out.append(_deferred(x, nolink))
    return ''.join(out)


def _emphasis(nodes, nolink):
    """CommonMark's delimiter pass over nodes, in place."""
    bottoms = {}
    i = 0
    while i < len(nodes):
        d = nodes[i]
        if type(d) is not dict or not d['close']:
            i += 1
            continue
        key = (d['ch'], d['open'], d['orig'] % 3)
        bottom = bottoms.get(key, -1)
        j = i - 1
        while j > bottom:
            o = nodes[j]
            if type(o) is dict and o['open'] and o['ch'] == d['ch']:
                odd = (o['close'] or d['open']) and (o['orig'] + d['orig']) % 3 == 0 \
                    and not (o['orig'] % 3 == 0 and d['orig'] % 3 == 0)
                if not odd:
                    break
            j -= 1
        if j <= bottom:
            bottoms[key] = i - 1
            if not d['open']:
                nodes[i] = d['ch'] * d['n']
            i += 1
            continue
        o = nodes[j]
        use = 2 if (o['n'] >= 2 and d['n'] >= 2) else 1
        tag = 'del' if d['ch'] == '~' else ('strong' if use == 2 else 'em')
        nodes[j + 1:i] = [f'<{tag}>{_join(nodes[j + 1:i], nolink)}</{tag}>']
        o['n'] -= use
        d['n'] -= use
        i = j + 2
        for k, b in bottoms.items():
            if b > j:
                bottoms[k] = j
        if o['n'] == 0:
            del nodes[j]
            i -= 1
            for k, b in bottoms.items():
                if b >= j:
                    bottoms[k] = j - 1
        if d['n'] == 0:
            del nodes[i]


def _finish(nodes, nolink):
    _emphasis(nodes, nolink)
    return _join(nodes, nolink)


def _skip_ws(text, i):
    n = len(text)
    while i < n and text[i] in ' \t\n':
        i += 1
    return i


def _parse_dest(text, p):
    """Parse "(dest "title")" at text[p]. (dest, title, end) or None."""
    if p >= len(text) or text[p] != '(':
        return None
    # A destination or title that never closes must not be scanned to the end
    # of the run once per "](": look no further than this.
    n = min(len(text), p + 2 * MAX_DEST_CHARS + 16)
    i = _skip_ws(text, p + 1)
    if i < n and text[i] == '<':
        j = i + 1
        while j < n and text[j] not in '<>\n':
            j += 2 if text[j] == '\\' else 1
        if j >= n or text[j] != '>':
            return None
        dest = text[i + 1:j]
        i = j + 1
    else:
        depth = 0
        j = i
        while j < n:
            c = text[j]
            if c == '\\' and j + 1 < n and text[j + 1] in _ASCII_PUNCT:
                j += 2
                continue
            if c in ' \t\n' or c < ' ':
                break
            if c == '(':
                depth += 1
                if depth > 32:
                    return None
            elif c == ')':
                if depth == 0:
                    break
                depth -= 1
            j += 1
        if depth:
            return None
        dest = text[i:j]
        i = j
    k = _skip_ws(text, i)
    title = ''
    if k < n and k > i and text[k] in '"\'(':
        closer = ')' if text[k] == '(' else text[k]
        e = text.find(closer, k + 1, n)
        while e > 0 and text[e - 1] == '\\' and text[e - 2] != '\\':
            e = text.find(closer, e + 1, n)
        if e < 0:
            return None
        title = text[k + 1:e]
        k = _skip_ws(text, e + 1)
    if k < n and text[k] == ')':
        return _ESCAPED.sub(r'\1', dest), _ESCAPED.sub(r'\1', title), k + 1
    return None


def _link(dest, title, inner):
    kind, href = classify_link(dest)
    if kind in ('web', 'mail'):
        t = f' title="{_esc(title[:300])}"' if title else ''
        return f'<a href="{_esc(href)}" {_REL}{t}>{inner or _esc(href)}</a>'
    if kind == 'anchor':
        return f'<a href="{_esc(href)}">{inner or _esc(dest.strip())}</a>'
    if kind == 'relative':
        # A path inside the source's own tree. The viewer has no tree to
        # resolve it against, so it is text with the path on hover.
        return f'<span class="md-nolink" title="{_esc(href[:300])}">{inner or _esc(href)}</span>'
    if kind == 'none':
        return f'<span class="md-nolink">{inner}</span>'
    return (f'<span class="md-nolink">{inner}</span> '
            f'<span class="md-blocked">[link not followed: {_esc(href[:300])}]</span>')


def _trim_url(url):
    """GFM's rule for where a bare URL ends: trailing punctuation and an
    unbalanced ")" belong to the sentence."""
    while url:
        c = url[-1]
        if c in '?!.,:;*_~\'"' or (c == ')' and url.count(')') > url.count('(')):
            url = url[:-1]
        else:
            break
    return url


def inline(text):
    """One run of inline Markdown (a paragraph, heading or cell) to HTML."""
    if _SPECIAL.search(text) is None or len(text) > MAX_INLINE_CHARS:
        return _esc(text)
    nodes = []
    brackets = []
    ticks = None
    ndelim = 0
    pos = 0
    n = len(text)
    while pos < n:
        m = _SPECIAL.search(text, pos)
        if m is None:
            nodes.append(_esc(text[pos:]))
            break
        s = m.start()
        if s > pos:
            nodes.append(_esc(text[pos:s]))
        ch = text[s]
        if ch == '\\':
            nxt = text[s + 1] if s + 1 < n else ''
            if nxt == '\n':
                nodes.append('<br>\n')
                pos = s + 2
            elif nxt and nxt in _ASCII_PUNCT:
                nodes.append(_esc(nxt))
                pos = s + 2
            else:
                nodes.append('\\')
                pos = s + 1
        elif ch == '`':
            e = s
            while e < n and text[e] == '`':
                e += 1
            run = e - s
            if ticks is None:
                ticks = {}
                for t in _TICKS.finditer(text):
                    ticks.setdefault(t.end() - t.start(), []).append(t.start())
            starts = ticks.get(run, ())
            k = bisect.bisect_left(starts, e)
            if k >= len(starts):
                nodes.append('`' * run)
                pos = e
            else:
                close = starts[k]
                code = text[e:close].replace('\n', ' ')
                if len(code) > 1 and code[0] == ' ' and code[-1] == ' ' and code.strip(' '):
                    code = code[1:-1]
                nodes.append(f'<code>{_esc(code)}</code>')
                pos = close + run
        elif ch in '*_~':
            e = s
            while e < n and text[e] == ch:
                e += 1
            run = e - s
            pos = e
            if (ch == '~' and run != 2) or ndelim >= MAX_DELIMS:
                nodes.append(ch * run)
                continue
            before = text[s - 1] if s else '\n'
            after = text[e] if e < n else '\n'
            pb, pa = _is_punct(before), _is_punct(after)
            sb, sa = before.isspace(), after.isspace()
            left = not sa and (not pa or sb or pb)
            right = not sb and (not pb or sa or pa)
            if ch == '_':
                can_open = left and (not right or pb)
                can_close = right and (not left or pa)
            else:
                can_open, can_close = left, right
            if not (can_open or can_close):
                nodes.append(ch * run)
                continue
            ndelim += 1
            nodes.append({'ch': ch, 'n': run, 'orig': run, 'open': can_open, 'close': can_close})
        elif ch == '[' or ch == '!':
            image = ch == '!'
            if image and not text.startswith('[', s + 1):
                nodes.append('!')
                pos = s + 1
            elif len(brackets) >= MAX_BRACKETS:
                nodes.append('![' if image else '[')
                pos = s + (2 if image else 1)
            else:
                brackets.append(len(nodes))
                nodes.append(['[', image, True])
                pos = s + (2 if image else 1)
        elif ch == ']':
            pos = s + 1
            if not brackets:
                nodes.append(']')
                continue
            idx = brackets.pop()
            mark = nodes[idx]
            parsed = _parse_dest(text, s + 1) if mark[2] else None
            if parsed is None:
                nodes[idx] = '![' if mark[1] else '['
                nodes.append(']')
                continue
            dest, title, pos = parsed
            inner = nodes[idx + 1:]
            del nodes[idx:]
            if mark[1]:
                nodes.append(('img', _finish(inner, True), dest))
            else:
                nodes.append(_link(dest, title, _finish(inner, True)))
                for b in brackets:   # no links inside links
                    if not nodes[b][1]:
                        nodes[b][2] = False
        elif ch == '<':
            a = _AUTOLINK.match(text, s)
            if a and classify_link(a.group(1))[0] in ('web', 'mail'):
                nodes.append(('auto', a.group(1), _esc(a.group(1))))
                pos = a.end()
            else:
                nodes.append('&lt;')
                pos = s + 1
        elif ch == '&':
            ent = _ENTITY.match(text, s)
            if ent:
                nodes.append(_esc(_CTRL.sub('', html.unescape(ent.group(0)))))
                pos = ent.end()
            else:
                nodes.append('&amp;')
                pos = s + 1
        elif ch == '\n':
            nodes.append('<br>\n' if text.endswith('  ', 0, s) else '\n')
            pos = s + 1
        else:  # "http://" or "https://"
            bare = None if (s and text[s - 1].isalnum()) else _BARE_URL.match(text, s)
            url = _trim_url(bare.group(0)) if bare else ''
            if url and classify_link(url)[0] == 'web':
                nodes.append(('auto', url, _esc(url)))
                pos = s + len(url)
            else:
                nodes.append(_esc(ch))
                pos = s + 1
    return _finish(nodes, False)


# ── block helpers ───────────────────────────────────────────────────────

def _indent_of(line):
    """Columns of leading whitespace; a tab runs to the next 4-column stop."""
    if not line or line[0] not in ' \t':
        return 0
    col = 0
    for c in line:
        if c == ' ':
            col += 1
        elif c == '\t':
            col += 4 - col % 4
        else:
            break
    return col


def _strip_cols(line, cols):
    """line without its first `cols` columns of leading whitespace."""
    col = 0
    i = 0
    n = len(line)
    while i < n and col < cols:
        c = line[i]
        if c == ' ':
            col += 1
        elif c == '\t':
            w = 4 - col % 4
            if col + w > cols:
                return ' ' * (col + w - cols) + line[i + 1:]
            col += w
        else:
            break
        i += 1
    return line[i:]


def _fence_open(line):
    m = _FENCE.match(line)
    if not m:
        return None
    run, info = m.group(2), m.group(3)
    if run[0] == '`' and '`' in info:
        return None
    return len(m.group(1)), run[0], len(run), info.strip()


def _fence_close(line, ch, length):
    s = line.strip()
    return len(s) >= length and s.count(ch) == len(s) and _indent_of(line) <= 3


def _track(fence, line):
    """Fence state after line: (char, length) inside a fenced block, or None."""
    if fence is None:
        f = _fence_open(line)
        return (f[1], f[2]) if f else None
    return None if _fence_close(line, *fence) else fence


def _is_block_start(line):
    """True when line ends a paragraph above it without a blank line."""
    if _indent_of(line) >= 4:
        return False
    if _fence_open(line) or _ATX.match(line) or _BQ.match(line) or _HR.match(line) \
            or line.lstrip(' ').startswith('<!--'):
        return True
    m = _ITEM.match(line)
    if m and m.group(4):
        marker = m.group(2)
        return not marker[0].isdigit() or marker[:-1] == '1'
    return False


def _split_row(line, code_aware):
    """Cells of a table row. `\\|` is a literal pipe. With code_aware a pipe
    inside a code span does not split (GitHub splits there; sheets written by
    hand often expect it not to)."""
    s = line.strip()
    if not code_aware:
        cells = _PIPE.split(s)
        if cells and not cells[0].strip():
            cells = cells[1:]
        if cells and not cells[-1].strip():
            cells = cells[:-1]
        return [c.replace('\\|', '|') for c in cells]
    cells = []
    cur = []
    i = 1 if s.startswith('|') else 0
    n = len(s)
    while i < n:
        c = s[i]
        if c == '\\' and i + 1 < n and s[i + 1] == '|':
            cur.append('|')
            i += 2
        elif c == '`':
            e = i
            while e < n and s[e] == '`':
                e += 1
            close = s.find(s[i:e], e)
            if close < 0:
                cur.append(s[i:e])
                i = e
            else:
                cur.append(s[i:close + e - i].replace('\\|', '|'))
                i = close + e - i
        elif c == '|':
            cells.append(''.join(cur))
            cur = []
            i += 1
        else:
            cur.append(c)
            i += 1
    last = ''.join(cur)
    if last.strip() or not cells:
        cells.append(last)
    return cells


def _table_at(lines, i):
    """(header cells, alignments) when a table starts at lines[i]."""
    line = lines[i]
    if '|' not in line or i + 1 >= len(lines):
        return None
    d = lines[i + 1]
    if '|' not in d or '-' not in d or not _DELIM_ROW.match(d) or _indent_of(d) >= 4:
        return None
    delims = [c.strip() for c in _split_row(d, False)]
    if not delims or len(delims) > MAX_TABLE_COLS or not all(_DELIM_CELL.match(c) for c in delims):
        return None
    head = _split_row(line, False)
    if len(head) != len(delims) and len(line) <= MAX_INLINE_CHARS:
        head = _split_row(line, True)
        if len(head) != len(delims):
            return None
    aligns = []
    for c in delims:
        left, right = c.startswith(':'), c.endswith(':')
        aligns.append('c' if left and right else 'r' if right else 'l' if left else '')
    return head, aligns


# ── block ───────────────────────────────────────────────────────────────

class _Renderer:
    def __init__(self, budget_s):
        self.deadline = time.monotonic() + budget_s
        self.timed_out = False
        self.too_deep = False
        self.hl_chars = 0
        self.hl_skipped = 0
        self.lexers = {}
        self.outline = []
        self.ids = {}
        self.gap = False

    def over_budget(self):
        if not self.timed_out and time.monotonic() > self.deadline:
            self.timed_out = True
        return self.timed_out

    @staticmethod
    def plain(lines):
        return '<pre class="md-plain"><code>' + _esc('\n'.join(lines)) + '</code></pre>'

    @staticmethod
    def emit(blocks, tight=False):
        return '\n'.join(h if kind != 'p' or tight else f'<p>{h}</p>' for kind, h in blocks)

    def blocks(self, lines, depth):
        """[(kind, html)] for a container's lines. A 'p' entry is the bare
        inline HTML, so a tight list item can leave the <p> off."""
        out = []
        if depth > MAX_DEPTH:
            self.too_deep = True
            self.gap = False
            return [('pre', self.plain(lines))]
        n = len(lines)
        i = 0
        gap = False     # a blank line sits between two of this container's blocks
        while i < n:
            line = lines[i]
            if not line.strip():
                i += 1
                continue
            if out and not lines[i - 1].strip():
                gap = True
            if self.over_budget():
                out.append(('pre', self.plain(lines[i:])))
                break
            if _indent_of(line) >= 4:
                j = i
                block = []
                while j < n and (not lines[j].strip() or _indent_of(lines[j]) >= 4):
                    block.append(_strip_cols(lines[j], 4))
                    j += 1
                while not block[-1].strip():
                    block.pop()
                out.append(('pre', '<pre><code>' + _esc('\n'.join(block)) + '</code></pre>'))
                i = j
                continue
            f = _fence_open(line)
            if f:
                indent, ch, length, info = f
                j = i + 1
                body = []
                while j < n and not _fence_close(lines[j], ch, length):
                    body.append(_strip_cols(lines[j], indent) if indent else lines[j])
                    j += 1
                out.append(('code', self.code(body, info)))
                i = j + 1
                continue
            if line.lstrip(' ').startswith('<!--'):
                # An HTML comment: a browser would hide it. Hidden text is where
                # an instruction to the reader's tools would sit, so it is shown,
                # as written, and nothing inside it is rendered.
                j = i
                while j < n - 1 and '-->' not in lines[j][4 if j == i else 0:]:
                    j += 1
                out.append(('pre', '<pre class="md-plain md-comment"><code>'
                            + _esc('\n'.join(lines[i:j + 1])) + '</code></pre>'))
                i = j + 1
                continue
            m = _ATX.match(line)
            if m:
                text = (m.group(2) or '').rstrip()
                bare = text.rstrip('#')
                if bare != text and (not bare or bare[-1] in ' \t'):
                    text = bare.rstrip()
                out.append(('h', self.heading(len(m.group(1)), text)))
                i += 1
                continue
            if _HR.match(line):
                out.append(('hr', '<hr>'))
                i += 1
                continue
            if _BQ.match(line):
                i = self.quote(lines, i, depth, out)
                continue
            m = _ITEM.match(line)
            if m:
                i = self.list(lines, i, depth, out)
                continue
            t = _table_at(lines, i)
            if t:
                i = self.table(lines, i, t, out)
                continue
            i = self.paragraph(lines, i, out)
        self.gap = gap  # read by list() straight after the call
        return out

    def paragraph(self, lines, i, out):
        n = len(lines)
        para = [lines[i].lstrip()]
        level = 0
        j = i + 1
        while j < n:
            line = lines[j]
            if not line.strip():
                break
            if _indent_of(line) < 4:
                m = _SETEXT.match(line)
                if m:
                    level = 1 if m.group(1)[0] == '=' else 2
                    j += 1
                    break
                if _is_block_start(line) or _table_at(lines, j):
                    break
            para.append(line.lstrip())
            j += 1
        text = '\n'.join(para).rstrip()
        if level:
            out.append(('h', self.heading(level, text)))
        else:
            out.append(('p', inline(text)))
        return j

    def heading(self, level, text):
        inner = inline(text)
        plain = html.unescape(_TAG.sub('', inner)).strip()
        base = slug(plain) or 'section'
        seen = self.ids.get(base, 0)
        self.ids[base] = seen + 1
        hid = ID_PREFIX + (base if not seen else f'{base}-{seen}')
        if len(self.outline) < MAX_OUTLINE:
            self.outline.append({'level': level, 'id': hid, 'text': plain[:200]})
        return (f'<h{level} id="{_esc(hid)}">{inner}'
                f'<a class="md-anchor" href="#{_esc(hid)}">#</a></h{level}>')

    def code(self, body, info):
        code = '\n'.join(body)
        inner = None
        if info:
            size = len(code)
            if size > MAX_HL_BLOCK_CHARS or self.hl_chars + size > MAX_HL_TOTAL_CHARS or self.over_budget():
                self.hl_skipped += 1
            else:
                if info not in self.lexers:
                    self.lexers[info] = zhen_highlight.lexer_for(info)
                lexer = self.lexers[info]
                if lexer is not None:
                    try:
                        inner = zhen_highlight.highlight_one(code, lexer)
                        self.hl_chars += size
                    except Exception:  # a lexer bug must not lose the block
                        inner = None
        if inner is None:
            inner = _esc(code)
        head = f'<div class="md-code-head"><span class="md-lang">{_esc(info[:120])}</span></div>'
        return f'<div class="md-code">{head}<pre><code>{inner}</code></pre></div>'

    def quote(self, lines, i, depth, out):
        n = len(lines)
        inner = []
        fence = None
        j = i
        while j < n:
            line = lines[j]
            m = _BQ.match(line)
            if m:
                s = line[m.end():].removeprefix(' ')
                inner.append(s)
                fence = _track(fence, s)
            elif line.strip() and fence is None and self.lazy_ok(inner, line):
                inner.append(line)
            else:
                break
            j += 1
        out.append(('bq', '<blockquote>\n' + self.emit(self.blocks(inner, depth + 1)) + '\n</blockquote>'))
        return j

    @staticmethod
    def lazy_ok(got, line):
        """A line with no container marker still continues the paragraph that
        the container's last line belongs to."""
        if not got or not got[-1].strip() or _is_block_start(line):
            return False
        # A deeply indented line is code only when it does not continue a
        # paragraph, so look back (a bounded way) for the line it hangs from.
        k = len(got) - 1
        floor = max(0, k - 64)
        while k > floor and _indent_of(got[k]) >= 4 and got[k - 1].strip():
            k -= 1
        last = got[k]
        if _indent_of(last) >= 4:
            return False
        return not (_fence_open(last) or _ATX.match(last) or _HR.match(last))

    def list(self, lines, i, depth, out):
        n = len(lines)
        first = _ITEM.match(lines[i])
        marker = first.group(2)
        ordered = marker[0].isdigit()
        kind = marker[-1]
        items = []
        loose = False
        j = i
        while True:
            m = _ITEM.match(lines[j])
            width = len(m.group(1)) + len(m.group(2))
            rest = m.group(4)
            if not rest:
                offset, head = width + 1, ''
            else:
                gap = _indent_of(m.group(3) + 'x')
                if gap > 4:
                    offset, head = width + 1, ' ' * (gap - 1) + rest
                else:
                    offset, head = width + gap, rest
            item = [head]
            fence = _track(None, head)
            blanks = 0
            sibling = False
            j += 1
            while j < n:
                line = lines[j]
                if not line.strip():
                    blanks += 1
                    j += 1
                    continue
                ind = _indent_of(line)
                if ind >= offset:
                    item.extend([''] * blanks)
                    blanks = 0
                    s = _strip_cols(line, offset)
                    item.append(s)
                    fence = _track(fence, s)
                    j += 1
                    continue
                m2 = _ITEM.match(line) if ind < 4 and not _HR.match(line) else None
                if m2 and m2.group(2)[-1] == kind and m2.group(2)[0].isdigit() == ordered:
                    sibling = True
                elif not blanks and fence is None and not m2 and self.lazy_ok(item, line):
                    item.append(line.lstrip())
                    j += 1
                    continue
                break
            items.append(item)
            if not sibling:
                break
            if blanks:
                loose = True
        rendered = []
        for item in items:
            task = None
            mt = _TASK.match(item[0])
            if mt:
                task = mt.group(1) != ' '
                item[0] = item[0][mt.end():]
            rendered.append((task, self.blocks(item, depth + 1)))
            # A blank line between two blocks of an item makes the list loose;
            # one inside a nested list or quote does not.
            if self.gap:
                loose = True
        parts = []
        for task, kids in rendered:
            body = self.emit(kids, tight=not loose)
            if task is None:
                parts.append(f'<li>{body}</li>')
            else:
                box = '<span class="md-task md-done">x</span>' if task else '<span class="md-task"></span>'
                parts.append(f'<li class="md-task-item">{box} {body}</li>')
        if ordered:
            start = int(marker[:-1])
            tag = 'ol' if start == 1 else f'ol start="{start}"'
            out.append(('list', f'<{tag}>\n' + '\n'.join(parts) + '\n</ol>'))
        else:
            out.append(('list', '<ul>\n' + '\n'.join(parts) + '\n</ul>'))
        return j

    def table(self, lines, i, t, out):
        head, aligns = t
        cols = len(aligns)
        n = len(lines)

        def cell(tag, text, k):
            cls = f' class="md-al-{aligns[k]}"' if aligns[k] else ''
            return f'<{tag}{cls}>{inline(text.strip())}</{tag}>'

        rows = ['<thead><tr>' + ''.join(cell('th', c, k) for k, c in enumerate(head)) + '</tr></thead>', '<tbody>']
        j = i + 2
        while j < n:
            line = lines[j]
            # As GitHub does: a table runs to the first blank line or the start
            # of another block, so a wrapped line without a pipe is still a row.
            if not line.strip() or _indent_of(line) >= 4 or _is_block_start(line):
                break
            cells = _split_row(line, False)
            if len(cells) != cols and len(line) <= MAX_INLINE_CHARS:
                aware = _split_row(line, True)
                if len(aware) == cols:
                    cells = aware
            if len(cells) > cols:   # never drop text: fold the extras into the last cell
                cells = cells[:cols - 1] + [' | '.join(cells[cols - 1:])]
            cells += [''] * (cols - len(cells))
            rows.append('<tr>' + ''.join(cell('td', c, k) for k, c in enumerate(cells)) + '</tr>')
            j += 1
        rows.append('</tbody>')
        out.append(('table', '<div class="md-table"><table>\n' + '\n'.join(rows) + '\n</table></div>'))
        return j


def _front_matter(lines):
    """Index of the first body line when lines open with YAML front matter
    (a `---` first line, closed by a later `---`), else 0."""
    if not lines or lines[0].rstrip() != '---':
        return 0
    for k in range(1, min(len(lines), MAX_FRONT_MATTER_LINES + 2)):
        s = lines[k].rstrip()
        if s == '---':
            block = lines[1:k]
            if any(_YAML_KEY.match(b) for b in block) and not any(_fence_open(b) for b in block):
                return k + 1
            return 0
    return 0


def _render(source, budget_s):
    if not isinstance(source, str):
        source = ''
    if len(source) > MAX_DOC_CHARS:
        return {'html': None, 'outline': [], 'timed_out': False,
                'notes': ['This sheet is too large to render. The raw text is shown.']}
    text = source.replace('\r\n', '\n').replace('\r', '\n')
    clean = _CTRL.sub('', text)
    dropped = len(text) - len(clean)
    lines = clean.split('\n')
    r = _Renderer(budget_s)
    parts = []
    body = _front_matter(lines)
    if body:
        parts.append('<details class="md-front"><summary>Front matter</summary><pre><code>'
                     + _esc('\n'.join(lines[1:body - 1])) + '</code></pre></details>')
    parts.append(r.emit(r.blocks(lines[body:], 0)))
    notes = []
    if dropped:
        notes.append(f'{dropped} control or text-direction character(s) were removed from the '
                     'rendered view. The raw view still has them.')
    if r.timed_out:
        notes.append('Rendering ran out of time. The rest of the sheet is shown as plain text.')
    if r.too_deep:
        notes.append('Lists or quotes nested past the limit are shown as plain text.')
    if r.hl_skipped:
        notes.append(f'{r.hl_skipped} code block(s) are not coloured (size limit).')
    return {'html': '\n'.join(p for p in parts if p), 'outline': r.outline,
            'notes': notes, 'timed_out': r.timed_out}


_CACHE = OrderedDict()
_CACHE_MAX = 24
_CACHE_LOCK = threading.Lock()


def render(source, budget_s=MAX_SECONDS):
    """Render a sheet. Returns {'html', 'outline', 'notes', 'timed_out'}:
    html is None when the sheet is too large to render (show it raw);
    outline is [{'level', 'id', 'text'}] for the headings; notes are plain
    sentences for the reader. The result is shared: do not change it."""
    key = hashlib.sha256(source.encode('utf-8', 'surrogatepass')).hexdigest() \
        if isinstance(source, str) else ''
    with _CACHE_LOCK:
        hit = _CACHE.get(key)
        if hit is not None:
            _CACHE.move_to_end(key)
            return hit
    out = _render(source, budget_s)
    if not out['timed_out']:   # a slow moment must not stick
        with _CACHE_LOCK:
            _CACHE[key] = out
            while len(_CACHE) > _CACHE_MAX:
                _CACHE.popitem(last=False)
    return out
