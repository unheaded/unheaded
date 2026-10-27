# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""zhen_scheduler_client: the web UI's calls into zhen-agentd -scheduler.

urllib.request.urlopen is replaced; nothing listens and nothing runs.
"""

import io
import json
import sys
import urllib.error
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import zhen_scheduler_client as sc


class _Resp:
    def __init__(self, status, body):
        self.status = status
        self._b = json.dumps(body).encode()

    def read(self):
        return self._b

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


@pytest.fixture
def sent(monkeypatch):
    calls = []

    def fake(req, timeout=None):
        calls.append(req)
        return _Resp(200, {'status': 'ok', 'result': {'name': 'service-health-sweep', 'runbook': 'observe/service-health-sweep'}})

    monkeypatch.setattr(sc.urllib.request, 'urlopen', fake)
    return calls


def test_add_is_a_gated_post_with_a_direct_user_justification(sent):
    code, body = sc.call('http://127.0.0.1:20105', {'X-API-Key': 'k'}, 'add', {'name': 'service-health-sweep', 'every': '6h'})
    assert code == 200 and body['result']['runbook'] == 'observe/service-health-sweep'
    req = sent[0]
    assert req.full_url == 'http://127.0.0.1:20105/api/v1/scheduler/add'
    assert req.get_method() == 'POST'
    assert req.headers.get('X-api-key') == 'k'
    payload = json.loads(req.data)
    assert payload['name'] == 'service-health-sweep' and payload['every'] == '6h'
    assert payload['emitted_by'] == 'zhen-web-ui'
    assert [r['source_trust'] for r in payload['justification']] == ['direct-user']


def test_list_is_a_get_without_a_body(sent):
    sc.call('http://127.0.0.1:20105', {}, 'list')
    assert sent[0].get_method() == 'GET' and sent[0].data is None
    assert sent[0].full_url.endswith('/api/v1/scheduler/list')


def test_emergency_stop_pauses(sent):
    sc.call('http://127.0.0.1:20105', {}, 'pause', {'paused': True})
    assert json.loads(sent[0].data)['paused'] is True
    assert sent[0].full_url.endswith('/api/v1/scheduler/pause')


def test_unknown_op_is_refused():
    with pytest.raises(ValueError):
        sc.call('http://127.0.0.1:20105', {}, 'exec', {})


def test_daemon_refusal_and_absence(monkeypatch):
    def refused(req, timeout=None):
        raise urllib.error.HTTPError(req.full_url, 500, 'x', {}, io.BytesIO(b'{"status":"error","reason":"runbook x: status \\"draft\\""}'))

    monkeypatch.setattr(sc.urllib.request, 'urlopen', refused)
    code, body = sc.call('http://127.0.0.1:20105', {}, 'add', {'name': 'x', 'every': '1h'})
    assert code == 500 and 'draft' in sc.reason(body)

    def down(req, timeout=None):
        raise urllib.error.URLError('connection refused')

    monkeypatch.setattr(sc.urllib.request, 'urlopen', down)
    code, body = sc.call('http://127.0.0.1:20105', {}, 'list')
    assert code == 503 and 'unreachable' in sc.reason(body)


def test_format_list():
    text = sc.format_list({'jobs': [
        {'name': 'a', 'runbook': 'observe/a', 'every': '6h', 'enabled': True, 'last_status': 'failed', 'last_error': 'exit 1: verification: 0/1 passed'},
        {'name': 'b', 'runbook': 'data/b', 'every': 'weekly@sun@03:00', 'enabled': False},
    ], 'paused': True, 'config': '/etc/unheaded/zhen/schedules.yaml'})
    assert 'Scheduled Jobs (2)' in text and 'verification: 0/1 passed' in text
    assert 'never run' in text and 'Paused' in text
