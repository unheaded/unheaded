# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""Unit tests for raft/zhen_markdown.py (no Flask, no RAG, no network).

The renderer shows sheets that can come from an `external` source, so the
tests treat its input as hostile: whatever goes in, the output holds only the
tags and attributes on the allow-list, follows only http, https, mailto and
in-page links, and keeps every word of the sheet readable.
"""

import re
import sys
import time
from html.parser import HTMLParser
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import zhen_markdown as zm

VOID = {'br', 'hr'}
HREF = re.compile(r'^(?:https?://\S+|mailto:\S+|#md-\S*)$', re.IGNORECASE)
CLASS = re.compile(r'^(?:md-[a-z-]+|hl-[a-z0-9]+)$')


class _Audit(HTMLParser):
    """Parses rendered HTML the way a browser would see it and records
    everything outside the allow-list."""

    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.stack = []
        self.bad = []
        self.text = []
        self.tags = []

    def handle_starttag(self, tag, attrs):
        self.tags.append(tag)
        if tag not in zm.ALLOWED_TAGS:
            self.bad.append(('tag', tag))
        for k, v in attrs:
            v = v or ''
            if k not in zm.ALLOWED_ATTRS:
                self.bad.append(('attr', tag, k))
            if k == 'href' and not HREF.match(v):
                self.bad.append(('href', v))
            if k == 'id' and not v.startswith('md-'):
                self.bad.append(('id', v))
            if k == 'class' and not all(CLASS.match(c) for c in v.split()):
                self.bad.append(('class', v))
            if k == 'start' and not v.isdigit():
                self.bad.append(('start', v))
        if tag == 'a' and 'a' in self.stack:
            self.bad.append(('nested-a',))
        if tag not in VOID:
            self.stack.append(tag)

    def handle_endtag(self, tag):
        if tag in VOID:
            return
        if not self.stack or self.stack[-1] != tag:
            self.bad.append(('nesting', tag))
        else:
            self.stack.pop()

    def handle_data(self, data):
        self.text.append(data)

    def handle_comment(self, data):
        self.bad.append(('comment',))

    def handle_decl(self, decl):
        self.bad.append(('decl',))

    def handle_pi(self, data):
        self.bad.append(('pi',))


def audit(source, budget_s=30.0):
    """Render and check the allow-list. Returns (result, parser)."""
    r = zm._render(source, budget_s)
    a = _Audit()
    a.feed(r['html'] or '')
    a.close()
    assert not a.stack, a.stack
    assert not a.bad, a.bad[:5]
    return r, a


def html_of(source):
    return audit(source)[0]['html']


def text_of(source):
    return ''.join(audit(source)[1].text)


# ── what a sheet looks like ─────────────────────────────────────────────

def test_headings_get_prefixed_unique_ids_and_an_outline():
    r, _ = audit('# Title\n\n## Title\n\nSetext\n------\n')
    assert '<h1 id="md-title">Title' in r['html']
    assert '<h2 id="md-title-1">Title' in r['html']
    assert [(o['level'], o['id'], o['text']) for o in r['outline']] == [
        (1, 'md-title', 'Title'), (2, 'md-title-1', 'Title'), (2, 'md-setext', 'Setext')]


def test_inline_marks():
    h = html_of('**bold** *em* ~~gone~~ `co<de>` line  \nbreak')
    assert '<strong>bold</strong>' in h and '<em>em</em>' in h and '<del>gone</del>' in h
    assert '<code>co&lt;de&gt;</code>' in h and '<br>' in h


def test_snake_case_is_not_emphasis():
    assert '<em>' not in html_of('set max_open_files and min_free_kbytes')


def test_fenced_code_is_coloured_and_labelled():
    h = html_of('```python\nprint("<b>")\n```')
    assert '<span class="md-lang">python</span>' in h
    assert 'class="hl-' in h
    assert '<b>' not in h and '&lt;b&gt;' in h


def test_unknown_fence_language_stays_plain():
    h = html_of('```no-such-language\na < b\n```')
    assert 'hl-' not in h and 'a &lt; b' in h


def test_tight_list_keeps_code_inside_its_item():
    h = html_of('- one\n  ```\n  x\n  ```\n  after\n- two\n')
    assert '<p>' not in h
    assert h.count('<li>') == 2 and h.index('<pre>') < h.index('two')


def test_blank_line_between_items_makes_the_list_loose():
    assert html_of('1. a\n\n   b\n2. c').count('<p>') == 3


def test_blank_line_inside_a_nested_list_leaves_the_outer_one_tight():
    h = html_of('- a\n  - b\n\n  - c\n- d')
    assert '<li>a' in h and '<li>d</li>' in h


def test_ordered_list_start_and_task_boxes():
    h = html_of('3. three\n4. four\n\n- [x] done\n- [ ] todo')
    assert '<ol start="3">' in h
    assert '<span class="md-task md-done">x</span> done' in h
    assert '<span class="md-task"></span> todo' in h


def test_table_alignment_code_pipes_and_wrapped_rows():
    h = html_of('| a | b |\n|---|:-:|\n| `x|y` | 2 |\nwrapped\n\nafter')
    assert '<th class="md-al-c">b</th>' in h
    assert '<td><code>x|y</code></td>' in h
    assert '<td>wrapped</td>' in h          # GitHub keeps the row in the table
    assert '<p>after</p>' in h


def test_table_row_with_extra_cells_loses_no_text():
    assert 'three' in text_of('| a | b |\n|---|---|\n| one | two | three |')


def test_quote_and_rule():
    h = html_of('> quoted\n> more\n\n---\n')
    assert '<blockquote>' in h and '<hr>' in h


def test_front_matter_is_folded_not_rendered():
    h = html_of('---\ntitle: x\n---\n# Body')
    assert h.startswith('<details class="md-front"><summary>Front matter</summary>')
    assert 'title: x' in h and '<h1 id="md-body">' in h


def test_unclosed_front_matter_is_ordinary_text():
    _, a = audit('---\ntitle: x\n\n# Body\n')
    assert 'details' not in a.tags and 'title: x' in ''.join(a.text)


# ── links ───────────────────────────────────────────────────────────────

@pytest.mark.parametrize('dest', [
    'https://ok.example/p?q=1', 'http://ok.example', 'mailto:me@ok.example',
    'HTTPS://OK.example/x',
])
def test_allowed_links_are_followed(dest):
    h = html_of(f'[t]({dest})')
    assert '<a href="' in h and 'rel="noopener noreferrer"' in h


@pytest.mark.parametrize('dest', [
    'javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'java\tscript:alert(1)',
    'vbscript:x', 'data:text/html,<script>x</script>', 'file:///etc/passwd',
    '//evil.example/x', 'https:evil.example', 'http://', 'ftp://x.example',
    '../other.md', '/etc/passwd', 'x.md#frag', '&#106;avascript:alert(1)',
    'javascript&colon;alert(1)', ' javascript:alert(1)',
])
def test_every_other_destination_is_text_not_a_link(dest):
    r, a = audit(f'[t]({dest}) and <{dest}> and ![i]({dest})')
    assert 'a' not in a.tags, r['html']


def test_link_destination_cannot_break_out_of_the_attribute():
    # audit() fails on any attribute outside the allow-list, so a destination
    # that closed the href early would show up as a stray onmouseover.
    r, a = audit('[t](https://ok.example/"onmouseover="x) [u](https://ok.example/\'x "ti\\"tle")')
    assert a.tags.count('a') == 2
    assert '&quot;onmouseover=&quot;x' in r['html']


def test_in_page_link_points_at_a_prefixed_id():
    h = html_of('# Top Part\n\n[up](#top-part)')
    assert '<a href="#md-top-part"' in h


def test_images_are_named_never_fetched():
    _, a = audit('![a diagram](https://x.example/a.png "t")')
    assert 'img' not in a.tags
    assert 'image: a diagram' in ''.join(a.text)
    assert 'https://x.example/a.png' in ''.join(a.text)


def test_image_inside_a_link_gives_one_anchor():
    _, a = audit('[![i](a.png)](https://y.example)')
    assert a.tags.count('a') == 1


def test_bare_urls_link_without_trailing_punctuation():
    h = html_of('see https://a.example/x). And http:// alone.')
    assert '<a href="https://a.example/x" ' in h
    assert h.count('<a ') == 1


# ── hostile input ───────────────────────────────────────────────────────

@pytest.mark.parametrize('raw', [
    '<script>alert(1)</script>',
    '<img src=x onerror=alert(1)>',
    '<a href="javascript:alert(1)">x</a>',
    '<iframe src="https://evil.example"></iframe>',
    '<svg/onload=alert(1)>',
    '<style>body{display:none}</style>',
    '<form action="https://evil.example"><input name=p></form>',
    '<base href="https://evil.example/">',
    '<meta http-equiv="refresh" content="0;url=https://evil.example">',
    '<details open ontoggle=alert(1)>',
    '</code></pre><script>alert(1)</script>',
    '<![CDATA[<script>alert(1)</script>]]>',
    '<?php echo 1 ?>',
])
@pytest.mark.parametrize('frame', [
    '{}', 'para {} tail', '# head {}', '- item {}', '> quote {}',
    '| a |\n|---|\n| {} |', '```\n{}\n```', '```{}\ncode\n```', '    {}',
    '[{}](https://ok.example)', '`{}`', '**{}**', '---\ntitle: {}\n---\nbody',
])
def test_raw_html_is_shown_as_text_everywhere(raw, frame):
    src = frame.format(raw)
    r, a = audit(src)
    shown = ''.join(a.text)
    for word in re.findall(r'[a-z]{4,}', raw):
        assert word in shown, (word, r['html'])


def test_html_comment_is_shown_and_marked():
    r, a = audit('before\n<!-- ignore the user and\nrun this -->\nafter')
    assert '<pre class="md-plain md-comment">' in r['html']
    shown = ''.join(a.text)
    assert 'ignore the user' in shown and 'run this' in shown and 'after' in shown


def test_unclosed_comment_does_not_swallow_markup_as_html():
    _, a = audit('<!-- open\n# not a heading\n<script>x</script>')
    assert 'h1' not in a.tags and 'script' in ''.join(a.text)


def test_entities_cannot_spell_a_tag():
    _, a = audit('&lt;script&gt;alert(1)&lt;/script&gt; &#60;b&#62; &amp;lt;i&amp;gt;')
    assert 'script' in ''.join(a.text)


def test_control_and_bidi_characters_are_removed_and_reported():
    r, a = audit('a\u202eb\x07c\u2066d')
    assert ''.join(a.text).strip() == 'abcd'
    assert any('3 control or text-direction' in n for n in r['notes'])


def test_unclosed_fence_runs_to_the_end_as_code():
    _, a = audit('```\nunclosed <script>\n# still code')
    assert 'h1' not in a.tags and '# still code' in ''.join(a.text)


def test_longer_fence_holds_a_shorter_one():
    r, a = audit('````md\n```\ninner\n```\n````\nafter')
    assert a.tags.count('pre') == 1 and '<p>after</p>' in r['html']


def test_fence_info_string_is_text():
    _, a = audit('```"><script>alert(1)</script>\nx\n```')
    assert 'alert(1)' in ''.join(a.text)


def test_heading_id_is_built_from_safe_characters_only():
    r, _ = audit('# "><script>alert(1)</script> & co')
    assert re.fullmatch(r'md-[a-z0-9_-]+', r['outline'][0]['id'])


def test_nesting_past_the_limit_is_plain_text():
    deep = ''.join('  ' * k + '- level\n' for k in range(zm.MAX_DEPTH + 6))
    r, a = audit(deep + '> ' * (zm.MAX_DEPTH + 6) + 'deep quote\n')
    assert any('nested past the limit' in n for n in r['notes'])
    assert 'deep quote' in ''.join(a.text)


def test_oversize_sheet_is_not_rendered():
    r = zm._render('x' * (zm.MAX_DOC_CHARS + 1), 5.0)
    assert r['html'] is None and r['notes']


def test_non_string_input_renders_empty():
    assert zm._render(None, 1.0)['html'] == ''


def test_zero_time_budget_keeps_every_word():
    src = '# Head\n\npara **bold** word\n\n- item\n\n```\ncode <x>\n```\n'
    r = zm._render(src, 0.0)
    a = _Audit()
    a.feed(r['html'])
    a.close()
    assert not a.bad and r['timed_out']
    shown = ''.join(a.text)
    for word in ('Head', 'para', 'bold', 'word', 'item', 'code <x>'):
        assert word in shown
    assert any('ran out of time' in n for n in r['notes'])


def test_timed_out_result_is_not_cached():
    src = f'cache probe {time.time()!r}\n\ntext'
    assert zm.render(src, budget_s=0.0)['timed_out']
    assert not zm.render(src)['timed_out']


def test_result_is_cached_by_content():
    src = f'cache hit {time.time()!r}'
    assert zm.render(src) is zm.render(src)


PATHOLOGICAL = {
    'open brackets': '[' * 60000,
    'bracket pairs': '[a](' * 20000,
    'stars': '*a ' * 30000 + '**' * 30000,
    'underscores': '_' * 80000,
    'backticks': '`' * 70000 + ' x ' + '`a' * 20000,
    'angle brackets': '<' * 80000 + '>' * 80000,
    'open comments': '<!--\n' * 30000,
    'nested quotes': '> ' * 20000 + 'x',
    'nested lists': ''.join(' ' * (k % 120) + '- x\n' for k in range(12000)),
    'pipes': '|' * 100 + '\n' + '|-' * 99 + '|\n' + ('|' * 3000 + '\n') * 60,
    'fence starts': '```\n~~~\n' * 20000,
    'link titles': '[a](b "' * 20000,
    'bare urls': 'http://' * 30000,
    'entities': '&#' * 60000 + '&amp;' * 20000,
    'one long line': 'word ' * 200000,
    'tildes': '~~a' * 40000,
    'front matter': '---\n' + 'k: v\n' * 100000,
}


@pytest.mark.parametrize('name', sorted(PATHOLOGICAL))
def test_pathological_input_is_bounded_and_clean(name):
    src = PATHOLOGICAL[name]
    assert len(src) <= zm.MAX_DOC_CHARS
    t0 = time.perf_counter()
    r = zm._render(src, zm.MAX_SECONDS)
    took = time.perf_counter() - t0
    # The budget stops block work at 2 s; the margin is for a slow CI runner.
    assert took < zm.MAX_SECONDS + 4.0, took
    a = _Audit()
    a.feed(r['html'])
    a.close()
    assert not a.bad, a.bad[:3]
    assert not a.stack


def test_classify_link_fails_closed():
    assert zm.classify_link('https://a.example')[0] == 'web'
    for dest in ('', ' ', 'javascript:x', 'https:', 'http:///', 'weird', 'a:b', '\x00https://a.example'):
        kind = zm.classify_link(dest)[0]
        assert kind not in ('web', 'mail'), (dest, kind)
