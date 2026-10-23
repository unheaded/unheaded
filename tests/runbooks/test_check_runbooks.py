# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Tests for scripts/check-runbooks.py (S-TWO-BRAINS Steps 261-264).

Every rule has a fixture that breaks it; a good fixture passes; the
baseline ratchet lets known problems through, refuses new ones, and
reports an entry that no longer fails as removable.
Run: python -m pytest tests/runbooks/test_check_runbooks.py
"""
import os
import subprocess
import sys
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parents[2]
CHECKER = REPO / "scripts" / "check-runbooks.py"

GOOD_LEGACY = """\
apiVersion: runbook/v1
kind: Runbook
metadata:
  name: {name}
  description: A legacy runbook with no status
  owner: micromanager
  estimated_duration: 2m
  risk: low
  requires_sudo: false
steps:
  - name: look
    action: shell
    command: "echo hi"
    verify: "true"
  - name: by-hand
    action: manual
    command: |
      edit <the file> by hand
"""

GOOD_DRAFT = """\
apiVersion: runbook/v1
kind: Runbook
metadata:
  name: {name}
  description: A new runbook
  owner: micromanager
  estimated_duration: 5m
  risk: medium
  requires_sudo: false
  status: draft
  authored_by: zhen-local
  reviewed_by: claude-opus-5-5
  approved_by: ""
  dry_run_test: tests/runbooks/test_{stem}.py
steps:
  - name: read
    action: shell
    host: west
    command: |
      set -euo pipefail
      echo one
      echo two
    verify: "test -d /"
  - name: push
    action: manual
    irreversible: true
    command: "git push origin develop"
rollback:
  - name: undo
    action: manual
    command: "git revert <sha>"
"""


def write(root, rel, text):
    p = root / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text)
    return p


def make_draft(root, name="good-draft", text=None, with_test=True):
    stem = name.replace("-", "_")
    body = (text or GOOD_DRAFT).format(name=name, stem=stem)
    p = write(root, f"runbooks/data/{name}.yaml", body)
    if with_test:
        write(root, f"tests/runbooks/test_{stem}.py", "def test_x():\n    pass\n")
    return p


def run(root, *args):
    r = subprocess.run(
        [sys.executable, "-B", str(CHECKER), "--root", str(root), *args],
        capture_output=True, text=True, timeout=120, check=False,
    )
    return r.returncode, r.stdout + r.stderr


def rules(out):
    """The rule ids reported, from lines 'path: rule: where: message'."""
    found = set()
    for line in out.splitlines():
        parts = line.split(": ")
        if len(parts) >= 3 and parts[0].endswith(".yaml"):
            found.add(parts[1])
    return found


def test_good_legacy_and_draft_pass(tmp_path):
    write(tmp_path, "runbooks/data/good-legacy.yaml", GOOD_LEGACY.format(name="good-legacy"))
    make_draft(tmp_path)
    code, out = run(tmp_path, str(tmp_path / "runbooks"))
    assert code == 0, out
    assert "2 runbook(s)" in out


@pytest.mark.parametrize("mutate, rule", [
    (lambda s: s.replace("kind: Runbook\n", ""), "required-key"),
    (lambda s: s.replace("apiVersion: runbook/v1", "apiVersion: runbook/v2"), "api-version"),
    (lambda s: s.replace("  owner: micromanager\n", ""), "metadata-key"),
    (lambda s: s.replace("risk: low", "risk: spicy"), "risk"),
    (lambda s: s.replace("action: shell", "action: python"), "action"),
    (lambda s: s.replace('command: "echo hi"', 'command: "echo $(("'), "bash-n"),
    (lambda s: s.replace("- name: by-hand", "- name: look"), "step-name-dup"),
    (lambda s: s.replace('    verify: "true"\n', '    verify: "true"\n    host: north\n'), "host"),
    (lambda s: s.replace("  - name: by-hand\n", "  - action: manual\n"), "step-key"),
    (lambda s: s.replace("{name}", "other-name", 1), "name-mismatch"),
    (lambda s: "a: [1, 2\n", "parse"),
    (lambda s: s.replace('command: "echo hi"', 'command: "echo [REDACTED:host#1]"'), "redaction-placeholder"),
])
def test_each_legacy_rule_fails(tmp_path, mutate, rule):
    text = mutate(GOOD_LEGACY)
    write(tmp_path, "runbooks/data/fx.yaml", text.format(name="fx"))
    code, out = run(tmp_path, "--no-baseline", str(tmp_path / "runbooks"))
    assert code == 1, out
    assert rule in rules(out), out


def test_host_may_name_a_declared_parameter(tmp_path):
    text = GOOD_LEGACY.replace('    verify: "true"\n', '    verify: "true"\n    host: "${{HOST}}"\n')
    text += "parameters:\n  - name: HOST\n    description: west or east\n"
    write(tmp_path, "runbooks/data/fx.yaml", text.format(name="fx"))
    code, out = run(tmp_path, "--no-baseline", str(tmp_path / "runbooks"))
    assert code == 0, out


@pytest.mark.parametrize("mutate, rule", [
    (lambda s: s.replace("status: draft", "status: final"), "status"),
    (lambda s: s.replace("authored_by: zhen-local", "authored_by: gpt"), "authored-by"),
    (lambda s: s.replace("  reviewed_by: claude-opus-5-5\n", ""), "reviewed-by"),
    (lambda s: s.replace('approved_by: ""', "approved_by: stevie"), "approved-by"),
    (lambda s: s.replace("  dry_run_test: tests/runbooks/test_{stem}.py\n", ""), "dry-run-test"),
    (lambda s: s.replace("      set -euo pipefail\n", ""), "set-euo"),
    (lambda s: s.replace("      echo one\n", "      ! grep -q x /etc/hostname\n"), "negated-command"),
    (lambda s: s.replace("    action: manual\n    irreversible: true", "    action: shell\n    irreversible: true"),
     "irreversible-manual"),
    (lambda s: s.replace('verify: "test -d /"', 'verify: "git push --force origin main"'),
     "irreversible-manual"),
])
def test_each_draft_rule_fails(tmp_path, mutate, rule):
    make_draft(tmp_path, text=mutate(GOOD_DRAFT))
    code, out = run(tmp_path, "--no-baseline", str(tmp_path / "runbooks"))
    assert code == 1, out
    assert rule in rules(out), out


def test_draft_test_file_must_exist(tmp_path):
    make_draft(tmp_path, with_test=False)
    code, out = run(tmp_path, "--no-baseline", str(tmp_path / "runbooks"))
    assert code == 1, out
    assert "dry-run-test" in rules(out), out


def test_approved_needs_approver(tmp_path):
    make_draft(tmp_path, text=GOOD_DRAFT.replace("status: draft", "status: approved"))
    code, out = run(tmp_path, "--no-baseline", str(tmp_path / "runbooks"))
    assert code == 1 and "approved-by" in rules(out), out
    make_draft(tmp_path, text=GOOD_DRAFT.replace("status: draft", "status: approved")
               .replace('approved_by: ""', "approved_by: stevie"))
    code, out = run(tmp_path, "--no-baseline", str(tmp_path / "runbooks"))
    assert code == 0, out


def test_single_line_shell_needs_no_set_euo(tmp_path):
    text = GOOD_DRAFT.replace("command: |\n      set -euo pipefail\n      echo one\n      echo two\n",
                              'command: "echo one"\n')
    make_draft(tmp_path, text=text)
    code, out = run(tmp_path, "--no-baseline", str(tmp_path / "runbooks"))
    assert code == 0, out


def test_comment_lines_before_set_euo_are_allowed(tmp_path):
    text = GOOD_DRAFT.replace("      set -euo pipefail\n", "      # why\n\n      set -euo pipefail\n")
    make_draft(tmp_path, text=text)
    code, out = run(tmp_path, "--no-baseline", str(tmp_path / "runbooks"))
    assert code == 0, out


def test_baseline_lets_known_through_and_refuses_new(tmp_path):
    bad = GOOD_LEGACY.replace("risk: low", "risk: spicy")
    write(tmp_path, "runbooks/data/fx.yaml", bad.format(name="fx"))
    base = tmp_path / "runbooks" / ".check-baseline"
    code, out = run(tmp_path, "--write-baseline", str(base), str(tmp_path / "runbooks"))
    assert code == 0, out
    assert base.read_text().count("\n") >= 1
    code, out = run(tmp_path, "--baseline", str(base), str(tmp_path / "runbooks"))
    assert code == 0, out
    # a new problem in the same file is not covered by the old entry
    write(tmp_path, "runbooks/data/fx.yaml",
          bad.replace("action: shell", "action: python").format(name="fx"))
    code, out = run(tmp_path, "--baseline", str(base), str(tmp_path / "runbooks"))
    assert code == 1 and "action" in rules(out), out


def test_baseline_entry_that_no_longer_fails_is_removable(tmp_path):
    bad = GOOD_LEGACY.replace("risk: low", "risk: spicy")
    write(tmp_path, "runbooks/data/fx.yaml", bad.format(name="fx"))
    base = tmp_path / "runbooks" / ".check-baseline"
    run(tmp_path, "--write-baseline", str(base), str(tmp_path / "runbooks"))
    write(tmp_path, "runbooks/data/fx.yaml", GOOD_LEGACY.format(name="fx"))
    code, out = run(tmp_path, "--baseline", str(base), str(tmp_path / "runbooks"))
    assert code == 1, out
    assert "removable" in out, out


def test_write_baseline_refuses_to_grow(tmp_path):
    write(tmp_path, "runbooks/data/fx.yaml", GOOD_LEGACY.replace("risk: low", "risk: spicy").format(name="fx"))
    base = tmp_path / "runbooks" / ".check-baseline"
    run(tmp_path, "--write-baseline", str(base), str(tmp_path / "runbooks"))
    write(tmp_path, "runbooks/data/fy.yaml", GOOD_LEGACY.replace("risk: low", "risk: spicy").format(name="fy"))
    code, out = run(tmp_path, "--write-baseline", str(base), str(tmp_path / "runbooks"))
    assert code == 1 and "grow" in out, out


def test_deleting_a_baseline_line_bites(tmp_path):
    write(tmp_path, "runbooks/data/fx.yaml", GOOD_LEGACY.replace("risk: low", "risk: spicy").format(name="fx"))
    base = tmp_path / "runbooks" / ".check-baseline"
    run(tmp_path, "--write-baseline", str(base), str(tmp_path / "runbooks"))
    lines = [ln for ln in base.read_text().splitlines() if ln and not ln.startswith("#")]
    base.write_text("\n".join(lines[1:]) + "\n")
    code, out = run(tmp_path, "--baseline", str(base), str(tmp_path / "runbooks"))
    assert code == 1, out


def test_single_file_ignores_other_files_baseline_entries(tmp_path):
    write(tmp_path, "runbooks/data/fx.yaml", GOOD_LEGACY.replace("risk: low", "risk: spicy").format(name="fx"))
    write(tmp_path, "runbooks/data/ok.yaml", GOOD_LEGACY.format(name="ok"))
    base = tmp_path / "runbooks" / ".check-baseline"
    run(tmp_path, "--write-baseline", str(base), str(tmp_path / "runbooks"))
    code, out = run(tmp_path, "--baseline", str(base), str(tmp_path / "runbooks/data/ok.yaml"))
    assert code == 0, out


def test_default_baseline_is_root_runbooks_check_baseline(tmp_path):
    write(tmp_path, "runbooks/data/fx.yaml", GOOD_LEGACY.replace("risk: low", "risk: spicy").format(name="fx"))
    run(tmp_path, "--write-baseline", str(tmp_path / "runbooks" / ".check-baseline"), str(tmp_path / "runbooks"))
    code, out = run(tmp_path, str(tmp_path / "runbooks"))
    assert code == 0, out


def test_missing_path_is_usage_error(tmp_path):
    code, out = run(tmp_path, str(tmp_path / "nope"))
    assert code == 2, out


@pytest.mark.skipif(not (REPO / "runbooks" / ".check-baseline").exists(), reason="baseline not written yet")
def test_repo_corpus_passes_with_its_baseline():
    r = subprocess.run([sys.executable, "-B", str(CHECKER), str(REPO / "runbooks")],
                       capture_output=True, text=True, timeout=300, check=False, cwd=REPO,
                       env={**os.environ})
    assert r.returncode == 0, r.stdout + r.stderr

