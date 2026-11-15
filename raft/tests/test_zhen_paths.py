# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""zhen_app.py finds the project from one ZHEN_PROJECT_ROOT (D-05).

The app hard-coded a home directory in six places, so it could only run as the
person whose home that was. A unit under its own account (ProtectHome=yes) sets
ZHEN_PROJECT_ROOT; unset, every path keeps the value it had.

The paths are fixed at import, so each case imports the app in a fresh
interpreter with sentence-transformers stubbed, as the other suites do in
process. Nothing listens and no model loads.
"""

import json
import os
import re
import subprocess
import sys
from pathlib import Path

import pytest

RAFT = Path(__file__).resolve().parents[1]

_PROBE = r'''
import json, sys, types
st = types.ModuleType('sentence_transformers')
st.SentenceTransformer = lambda *a, **kw: object()
sys.modules['sentence_transformers'] = st
sys.path.insert(0, sys.argv[1] + '/scripts')
sys.path.insert(0, sys.argv[1])
import zhen_app
keys = zhen_app._load_model_keys.__code__.co_names
print('PATHS=' + json.dumps({
    'project_root': zhen_app.PROJECT_ROOT,
    'runbooks': zhen_app.RUNBOOKS_DIR,
    'skills': str(zhen_app.SKILLS_DIR),
    'index': str(zhen_app.index_dir),
    'corpus': str(zhen_app.corpus_dir),
    'champion': list(zhen_app.CHAMPION_ALLOWED),
    'model_keys_reads_project_root': 'PROJECT_ROOT' in keys,
}))
'''


def _paths(env_root, home=None):
    env = {k: v for k, v in os.environ.items()
           if k not in ('ZHEN_PROJECT_ROOT', 'UNHEADED_ROOT', 'ZHEN_DB_PASSWORD', 'AUTH_ISSUER_URL')}
    if env_root is not None:
        env['ZHEN_PROJECT_ROOT'] = env_root
    if home is not None:
        env['HOME'] = home
    r = subprocess.run([sys.executable, '-c', _PROBE, str(RAFT)], env=env,
                       capture_output=True, text=True, timeout=120, check=False)
    line = [ln for ln in r.stdout.splitlines() if ln.startswith('PATHS=')]
    assert line, f'the app did not import (exit {r.returncode}): {r.stderr[-800:]}'
    return json.loads(line[0][len('PATHS='):])


def test_everything_sits_under_zhen_project_root():
    p = _paths('/srv/zhen-root')
    assert p['project_root'] == '/srv/zhen-root'
    assert p['runbooks'] == '/srv/zhen-root/runbooks'
    assert p['skills'] == '/srv/zhen-root/skills'
    assert p['index'] == '/srv/zhen-root/raft/index'
    assert p['corpus'] == '/srv/zhen-root/raft/corpus'
    assert p['champion'][0] == '/srv/zhen-root/'
    assert '/var/zhen/' in p['champion']
    assert p['model_keys_reads_project_root']
    assert not [v for v in json.dumps(p).split('"') if v.startswith('/home/')]


def test_unset_keeps_todays_paths(tmp_path):
    home = str(tmp_path)
    p = _paths(None, home=home)
    root = os.path.join(home, 'tmp', 'unheaded')   # ~/tmp/unheaded, the checkout the app has always run from
    assert p['project_root'] == root
    assert p['runbooks'] == root + '/runbooks'
    assert p['skills'] == root + '/skills'
    assert p['index'] == root + '/raft/index'
    assert p['champion'][0] == root + '/'


def test_a_trailing_slash_is_the_same_root():
    assert _paths('/srv/zhen-root/')['runbooks'] == '/srv/zhen-root/runbooks'


@pytest.mark.parametrize('bad', ['relative/root', './x', '~/tmp/unheaded'])
def test_a_root_that_is_not_absolute_stops_the_app(bad):
    env = dict(os.environ, ZHEN_PROJECT_ROOT=bad)
    r = subprocess.run([sys.executable, '-c', _PROBE, str(RAFT)], env=env,
                       capture_output=True, text=True, timeout=120, check=False)
    assert r.returncode != 0
    assert 'ZHEN_PROJECT_ROOT' in r.stderr


def test_no_home_directory_outside_the_default():
    """One function computes the default; nothing else names a home."""
    src = (RAFT / 'zhen_app.py').read_text()
    assert 'Path.home()' not in src
    assert '/home/' not in re.sub(r'#.*', '', src), 'a literal home path outside a comment'
    hits = [m.start() for m in re.finditer(r"expanduser\('~", src)]
    assert len(hits) == 1
    fn = src[src.index('def _project_root('):]
    fn = fn[:fn.index('\n\n\n')]
    assert "expanduser('~" in fn
