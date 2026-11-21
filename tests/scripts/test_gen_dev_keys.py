# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/gen-dev-keys.sh makes a service key for every first-party compose
service of both stacks. alert-mailer (monitoring/docker-compose.yml) reads
its key from its own secrets directory, not a compose secret, so its key is
made in a directory of its own that can be mounted alone
(ALERT_MAILER_SECRETS_DIR); its public half goes in the trust directory
Wotan reads, as every other identity's does.

The script is run here with stand-ins for unheaded-keygen and unheaded-auth
(KEYGEN, AUTHBIN) that record their arguments: nothing is built and no real
key is made."""
import os
import pathlib
import stat
import subprocess

import yaml

REPO = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts/gen-dev-keys.sh"

KEYGEN_STUB = """#!/usr/bin/env bash
# Stand-in for unheaded-keygen: -dir D -trust-dir T [-print-kid] NAME
set -euo pipefail
echo "$*" >>"$STUB_LOG"
dir="" trust="" print=0 name=""
while [[ $# -gt 0 ]]; do
	case "$1" in
	-dir) dir="$2"; shift 2 ;;
	-trust-dir) trust="$2"; shift 2 ;;
	-print-kid) print=1; shift ;;
	*) name="$1"; shift ;;
	esac
done
if [[ $print -eq 1 ]]; then
	[[ -e "$dir/$name.key" ]] || { echo "no key for $name in $dir" >&2; exit 1; }
	echo "kid-$name"
	exit 0
fi
mkdir -p "$dir" "$trust"
[[ ! -e "$dir/$name.key" ]] || { echo "key exists" >&2; exit 2; }
echo "not a key" >"$dir/$name.key"
echo "not a key" >"$trust/$name.pub"
"""

AUTH_STUB = """#!/usr/bin/env bash
# Stand-in for unheaded-auth: user add ... -users-file F | user list -users-file F
set -euo pipefail
file=""
while [[ $# -gt 0 ]]; do
	case "$1" in
	-users-file) file="$2"; shift 2 ;;
	*) shift ;;
	esac
done
[[ -e "$file" ]] && echo dev || { cat >/dev/null; echo "[]" >"$file"; }
"""


def run(tmp_path):
    for name, text in (("keygen", KEYGEN_STUB), ("auth", AUTH_STUB)):
        p = tmp_path / name
        p.write_text(text, encoding="utf-8")
        p.chmod(p.stat().st_mode | stat.S_IXUSR)
    env = dict(os.environ, KEYGEN=str(tmp_path / "keygen"), AUTHBIN=str(tmp_path / "auth"), STUB_LOG=str(tmp_path / "log"))
    proc = subprocess.run(
        ["bash", str(SCRIPT), "--keys-dir", str(tmp_path / "keys/dev"), "--trust-dir", str(tmp_path / "trust"),
         "--users-file", str(tmp_path / "auth-dir/users.json")],
        capture_output=True, text=True, check=False, env=env, cwd=tmp_path,
    )
    assert proc.returncode == 0, proc.stderr
    return proc.stdout


def test_alert_mailer_gets_a_key_in_a_directory_of_its_own(tmp_path):
    out = run(tmp_path)
    keys, trust = tmp_path / "keys/dev", tmp_path / "trust"
    assert (keys / "alert-mailer/alert-mailer.key").is_file()
    assert not (keys / "alert-mailer.key").exists()
    assert (trust / "alert-mailer.pub").is_file()
    rows = [line.split() for line in out.splitlines()]
    assert ["alert-mailer", "kid-alert-mailer"] in [r[:2] for r in rows if len(r) >= 2]


def test_the_main_stacks_keys_stay_where_compose_secrets_read_them(tmp_path):
    run(tmp_path)
    keys = tmp_path / "keys/dev"
    compose = yaml.safe_load((REPO / "docker-compose.yml").read_text(encoding="utf-8"))
    secret_files = [pathlib.Path(s["file"]).name for s in compose["secrets"].values()]
    assert "wotan.key" in secret_files
    for name in secret_files:
        assert (keys / name).is_file(), name
    for extra in ("unheaded-cli", "unheaded-auth"):
        assert (keys / f"{extra}.key").is_file()
    # Only the monitoring stack's services get a directory.
    assert sorted(p.name for p in keys.iterdir() if p.is_dir()) == ["alert-mailer"]


def test_every_first_party_monitoring_service_is_covered(tmp_path):
    """A second first-party service in the monitoring stack gets a key too,
    without this script being told its name."""
    run(tmp_path)
    monitoring = yaml.safe_load((REPO / "monitoring/docker-compose.yml").read_text(encoding="utf-8"))
    built = sorted(n for n, s in monitoring["services"].items() if "build" in s)
    assert "alert-mailer" in built
    for name in built:
        assert (tmp_path / "keys/dev" / name / f"{name}.key").is_file(), name
        assert (tmp_path / "trust" / f"{name}.pub").is_file(), name


def test_a_second_run_keeps_the_keys(tmp_path):
    run(tmp_path)
    log = tmp_path / "log"
    first = log.read_text(encoding="utf-8").splitlines()
    log.write_text("", encoding="utf-8")
    run(tmp_path)
    second = log.read_text(encoding="utf-8").splitlines()
    assert any("-print-kid" not in line and line.endswith(" alert-mailer") for line in first)
    assert second and all("-print-kid" in line for line in second), second


def test_it_says_what_alerts_critical_still_needs(tmp_path):
    out = run(tmp_path)
    assert "-algo mldsa65" in out
    assert "alerts-critical" in out
    assert "ALERT_MAILER_SECRETS_DIR" in out
    assert "wiki/Wotan-Topic-Signing.md" in out
    # It makes no signing key and enrols nothing.
    assert "mldsa65" not in (tmp_path / "log").read_text(encoding="utf-8")
