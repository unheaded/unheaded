# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/well/set-role-limits.sh sets a Well role's connection limit from
<ROLE>_CONNECTION_LIMIT (Stevie, 2026-11-17: a limit another deployment may
want to change is not hard-coded). A fake docker stands in for the Well;
nothing here touches one."""
import pathlib
import subprocess

REPO = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts/well/set-role-limits.sh"

# docker info: up. docker exec C printenv POSTGRES_USER: the superuser.
# docker exec ... psql ...: one line of argv and the SQL on stdin are kept
# in $STATE/calls; the role's limit before is $STATE/before (empty: no such
# role).
FAKE_DOCKER = """#!/bin/sh
case "$1" in
info) exit 0 ;;
exec)
    case "$*" in
    *printenv*) echo unheaded; exit 0 ;;
    esac
    { echo "ARGV $*"; cat; echo "END"; } >>"$STATE/calls"
    cat "$STATE/before" 2>/dev/null
    exit 0 ;;
esac
exit 64
"""


def run(tmp_path, env, before="20\n", args=()):
    bindir = tmp_path / "bin"
    bindir.mkdir(exist_ok=True)
    fake = bindir / "docker"
    fake.write_text(FAKE_DOCKER)
    fake.chmod(0o755)
    (tmp_path / "before").write_text(before)
    calls = tmp_path / "calls"
    if calls.exists():
        calls.unlink()
    full = {"PATH": f"{bindir}:/usr/bin:/bin", "STATE": str(tmp_path), "HOME": str(tmp_path)}
    full.update(env)
    p = subprocess.run(["bash", str(SCRIPT), *args], env=full, capture_output=True, text=True, timeout=30, check=False)
    return p, (calls.read_text() if calls.exists() else "")


def test_sets_only_the_roles_whose_variable_is_set(tmp_path):
    p, calls = run(tmp_path, {"APP_ZHEN_CONNECTION_LIMIT": "30"})
    assert p.returncode == 0, p.stderr
    assert calls.count("ARGV") == 1
    assert "role=app_zhen" in calls and "limit=30" in calls
    assert 'ALTER ROLE :"role" CONNECTION LIMIT :limit' in calls
    assert "30" not in calls.split("ARGV", 1)[1].split("\n", 1)[1], "the value belongs in a psql variable, not in SQL text"
    assert "[well] app_zhen: connection limit 20 -> 30 (APP_ZHEN_CONNECTION_LIMIT)" in p.stdout
    assert "[well] 1 role limit(s) set" in p.stdout


def test_two_roles_and_no_limit(tmp_path):
    p, calls = run(tmp_path, {"APP_ZHEN_CONNECTION_LIMIT": "24", "LOG_WRITER_CONNECTION_LIMIT": "-1"})
    assert p.returncode == 0, p.stderr
    assert calls.count("ARGV") == 2
    assert "role=log_writer" in calls and "limit=-1" in calls
    assert "[well] 2 role limit(s) set" in p.stdout


def test_nothing_set_touches_nothing(tmp_path):
    p, calls = run(tmp_path, {})
    assert p.returncode == 0, p.stderr
    assert calls == ""
    assert "[well] 0 role limit(s) set" in p.stdout


def test_refuses_what_is_not_a_limit(tmp_path):
    for bad in ("30; DROP ROLE app_zhen", "0", "abc", "100000", "-2", " 30"):
        p, calls = run(tmp_path, {"APP_ZHEN_CONNECTION_LIMIT": bad})
        assert p.returncode == 2, f"{bad!r} accepted: {p.stdout}"
        assert calls == "", f"{bad!r} reached the Well"
        assert "APP_ZHEN_CONNECTION_LIMIT" in p.stderr


def test_a_bad_value_stops_before_any_role_changes(tmp_path):
    p, calls = run(tmp_path, {"APP_AUTH_CONNECTION_LIMIT": "40", "APP_ZHEN_CONNECTION_LIMIT": "many"})
    assert p.returncode == 2
    assert calls == "", "one role was changed before the bad value was seen"


def test_unknown_role_fails(tmp_path):
    p, _ = run(tmp_path, {"APP_NOPE_CONNECTION_LIMIT": "5"}, before="")
    assert p.returncode == 1
    assert "app_nope" in p.stderr and "no such role" in p.stderr


def test_print_shows_the_plan_and_changes_nothing(tmp_path):
    p, calls = run(tmp_path, {"APP_ZHEN_CONNECTION_LIMIT": "30"}, args=("--print",))
    assert p.returncode == 0, p.stderr
    assert "ALTER" not in calls
    assert "app_zhen: connection limit 20 -> 30" in p.stdout and "not applied" in p.stdout
