# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""raft/zhen_redact.py: the Python side of the shared redactor.

The patterns file is Go's (pkg/zhencapture, RE2); these cases mirror
pkg/zhencapture TestRedact so the two readers stay in step.
"""

import json
import sys
from pathlib import Path

import pytest

RAFT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(RAFT))

import zhen_redact


@pytest.fixture(scope='module')
def pats():
    return zhen_redact.load()


def fake(prefix, n, ch):
    return prefix + ch * n


@pytest.mark.parametrize('text,gone,keep,count', [
    ('key ' + fake('sk-ant-api03-', 30, 'x') + ' end', ['sk-ant-api03-xxx'], ['key [REDACTED:anthropic_key] end'], 1),
    ('postgres://app_zhen:' + fake('', 12, 'w') + '@west:5432/db', ['wwwwww'],
     ['postgres://app_zhen:[REDACTED:url_password]@west:5432/db'], 1),
    ('PGPASSWORD="' + fake('', 10, 'p') + '" psql', ['pppppp'], ['PGPASSWORD="[REDACTED:assignment]" psql'], 1),
    ('ask jane.doe+ops@mail.example.co.uk today', ['jane.doe', 'example.co.uk'], ['ask [REDACTED:email] today'], 1),
    ('@stevie and user@localhost', [], ['@stevie and user@localhost'], 0),
    ('the password policy is strict', [], ['the password policy is strict'], 0),
])
def test_redact_matches_go(pats, text, gone, keep, count):
    out, n = zhen_redact.redact(text, pats)
    for g in gone:
        assert g not in out
    for k in keep:
        assert k in out
    assert n == count


def test_idempotent(pats):
    once, _ = zhen_redact.redact('password=' + fake('', 12, 'q') + ' mail a@b.example', pats)
    twice, n = zhen_redact.redact(once, pats)
    assert twice == once and n == 0


def test_bad_file_raises(tmp_path):
    p = tmp_path / 'p.json'
    p.write_text(json.dumps({'version': 2, 'patterns': []}))
    with pytest.raises(ValueError):
        zhen_redact.load(p)
    with pytest.raises(FileNotFoundError):
        zhen_redact.load(tmp_path / 'absent.json')


def test_redact_json(pats):
    pem = '-----BEGIN ' + 'PRIVATE KEY-----\nMIIE' + 'B' * 30 + '\n-----END ' + 'PRIVATE KEY-----'
    doc = json.dumps([{'preview': 'mail a.b@example.org', 'n': 3}, {'preview': 'x "' + pem + '" y'}])
    out = zhen_redact.redact_json(doc, pats)
    parsed = json.loads(out)
    assert parsed[0]['preview'] == 'mail [REDACTED:email]' and parsed[0]['n'] == 3
    assert 'MIIE' not in out and '[REDACTED:private_key]' in parsed[1]['preview']
    assert zhen_redact.redact_json('not json', pats) == '[]'
