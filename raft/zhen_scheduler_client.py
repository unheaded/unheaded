# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""The web UI's side of zhen-agentd's scheduler (S-CHAMPION-HARDENING Step 165).

raft/zhen_scheduler.py (ADR-028) is retired: the scheduler runs inside
zhen-agentd (`-scheduler`), and the chat commands `schedule`, `unschedule`,
`list schedules` and `emergency stop` call its endpoints on loopback with
the daemon's auth:

    POST /api/v1/scheduler/add      {name, every, justification, emitted_by}
    POST /api/v1/scheduler/remove   {name, justification, emitted_by}
    POST /api/v1/scheduler/pause    {paused, justification, emitted_by}
    GET  /api/v1/scheduler/list

The changes are the Champion's mutating tool schedule_update; the typed
command is the user's own action, so it carries a direct-user justification
(the same shape the runbook and model-switch buttons send). stdlib only.
"""

import json
import urllib.error
import urllib.request

EMITTED_BY = 'zhen-web-ui'


def _justification(command):
    return [{
        'topic': 'user-typed-schedule-command',
        'category': 'user-action',
        'source_kind': 'user-action',
        'source_trust': 'direct-user',
        'source_label': command,
    }]


def call(agentd_url, headers, op, body=None, timeout=30):
    """One scheduler call. Returns (http_status, decoded_body).

    op is add, remove, pause (POST, gated) or list (GET). An unreachable
    daemon is (503, {...}); the caller shows the reason.
    """
    if op not in ('add', 'remove', 'pause', 'list'):
        raise ValueError(f'unknown scheduler op {op!r}')
    url = f'{agentd_url}/api/v1/scheduler/{op}'
    if op == 'list':
        req = urllib.request.Request(url, headers=headers, method='GET')
    else:
        payload = dict(body or {})
        payload['justification'] = _justification(f'chat: {op}')
        payload['emitted_by'] = EMITTED_BY
        req = urllib.request.Request(url, data=json.dumps(payload).encode('utf-8'), headers=headers, method='POST')
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode('utf-8') or '{}')
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read().decode('utf-8') or '{}')
        except (ValueError, OSError):
            return e.code, {'status': 'error', 'reason': str(e)}
    except (urllib.error.URLError, OSError) as e:
        return 503, {'status': 'error', 'reason': f'zhen-agentd unreachable at {agentd_url}: {e}'}


def reason(body):
    """The daemon's reason, for a chat answer."""
    return body.get('reason') or body.get('error') or json.dumps(body)[:300]


def format_list(view):
    """The list answer as chat Markdown."""
    jobs = view.get('jobs') or []
    lines = [f'**Scheduled Jobs ({len(jobs)}):**\n']
    for j in jobs:
        on = j.get('enabled')
        last = j.get('last_status') or 'never run'
        line = f'  {"●" if on else "○"} `{j.get("name")}` — `{j.get("runbook")}` {j.get("every")} [{"ENABLED" if on else "disabled"}] last: {last}'
        if j.get('last_error'):
            line += f' ({j["last_error"][:120]})'
        lines.append(line)
    if view.get('paused'):
        lines.append('\n**Paused** (emergency stop): no job starts until `resume schedules`.')
    lines.append('\nScheduler: `zhen-agentd -scheduler` (config ' + str(view.get('config', '?')) + ').')
    return '\n'.join(lines)
