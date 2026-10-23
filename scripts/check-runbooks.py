#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""check-runbooks.py: validate runbook/v1 YAML (ADR-024), ratcheted.

Every runbook:
  required-key     apiVersion, kind, metadata, steps present; steps a list
  api-version      apiVersion: runbook/v1, kind: Runbook
  metadata-key     metadata has name description owner estimated_duration
                   risk requires_sudo
  name-mismatch    metadata.name == the file's stem
  risk             low | medium | high | critical
  step-key         every step (and rollback entry) has name, action, command
  action           shell | manual
  host             west | east, or ${VAR} for a declared parameter/env key
  step-name-dup    step names unique
  bash-n           `bash -n` accepts every shell command, verify, rollback
                   command and precondition check (manual text is prose
                   with placeholders, so it is not parsed)
  parse            the file is YAML and a mapping
  redaction-placeholder
                   no "[REDACTED:" text: a privacy-filter placeholder a model
                   copied from its filtered prompt into the runbook

A runbook with metadata.status (the S-TWO-BRAINS ones) also:
  status           draft | approved
  authored-by      zhen-local | claude | human
  reviewed-by      present
  approved-by      present; "" while draft, a name once approved
  dry-run-test     a path, relative to the repo root, that exists
  set-euo          a multi-line shell command or verify starts (after
                   comments) with `set -euo pipefail`
  irreversible-manual
                   a step marked `irreversible: true`, or whose shell text
                   pushes, force-pushes, drops, restores over, shreds or
                   prunes, is action: manual

Known problems of the existing corpus are listed in runbooks/.check-baseline
("path|rule|where" per line). The baseline may only SHRINK: a problem not
in it fails, and an entry that no longer fails is reported as removable and
fails too, so a fix removes its line in the same commit and the problem
cannot quietly come back. --write-baseline refuses to add entries to an
existing baseline (same ratchet as scripts/check-secrets-baseline.sh).

  scripts/check-runbooks.py runbooks/                        # check
  scripts/check-runbooks.py runbooks/network/x.yaml          # one file
  scripts/check-runbooks.py --write-baseline runbooks/.check-baseline runbooks/
Exit: 0 clean, 1 findings, 2 usage.
"""
import argparse
import os
import re
import subprocess
import sys

import yaml

TOP_REQUIRED = ("apiVersion", "kind", "metadata", "steps")
META_REQUIRED = ("name", "description", "owner", "estimated_duration", "risk", "requires_sudo")
RISKS = ("low", "medium", "high", "critical")
ACTIONS = ("shell", "manual")
HOSTS = ("west", "east")
STATUSES = ("draft", "approved")
AUTHORS = ("zhen-local", "claude", "human")

# Shell text that cannot be taken back. In a runbook with a status, a step
# that does this must be manual: the operator runs it, the runner never does.
IRREVERSIBLE = [
    (re.compile(r"\bgit\s+push\b"), "git push"),
    (re.compile(r"\bpg_restore\b"), "pg_restore"),
    (re.compile(r"\bdropdb\b"), "dropdb"),
    (re.compile(r"\bDROP\s+(TABLE|DATABASE|ROLE|SCHEMA)\b", re.I), "DROP"),
    (re.compile(r"\bshred\b"), "shred"),
    (re.compile(r"\brm\s+-[a-zA-Z]*r[a-zA-Z]*f|\brm\s+-[a-zA-Z]*f[a-zA-Z]*r"), "rm -rf"),
    (re.compile(r"\bmkfs(\.\w+)?\b"), "mkfs"),
    (re.compile(r"\bwipefs\b"), "wipefs"),
    (re.compile(r"\breflog\s+expire\b"), "git reflog expire"),
    (re.compile(r"\bgc\s+--prune\b"), "git gc --prune"),
    (re.compile(r"\breset\s+--hard\b"), "git reset --hard"),
    (re.compile(r"\bbranch\s+-D\b"), "git branch -D"),
]

ROOT_DEFAULT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


class Finding:
    __slots__ = ("path", "rule", "where", "msg")

    def __init__(self, path, rule, where, msg):
        self.path, self.rule, self.where, self.msg = path, rule, where, msg

    def key(self):
        return f"{self.path}|{self.rule}|{self.where}"

    def __str__(self):
        return f"{self.path}: {self.rule}: {self.where}: {self.msg}"


def bash_n(text):
    """Return bash's complaint about text, or None if it parses."""
    r = subprocess.run(["bash", "-n"], input=text, capture_output=True, text=True,
                       timeout=20, check=False)
    if r.returncode == 0:
        return None
    return (r.stderr.strip().splitlines() or ["syntax error"])[-1]


def starts_with_set_euo(text):
    for line in text.splitlines():
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        return bool(re.match(r"set\s+-euo\s+pipefail\b", s))
    return True


def entries(rb, section):
    v = rb.get(section)
    if isinstance(v, dict):  # active-active-cutover nests rollback: {steps: [...]}
        v = v.get("steps")
    return v if isinstance(v, list) else []


def check_file(path, rel, root):
    out = []

    def add(rule, where, msg):
        out.append(Finding(rel, rule, where, msg))

    try:
        with open(path, encoding="utf-8") as f:
            rb = yaml.safe_load(f)
    except (yaml.YAMLError, UnicodeDecodeError) as e:
        add("parse", "file", str(e).splitlines()[0] if str(e) else "unparseable")
        return out
    if not isinstance(rb, dict):
        add("parse", "file", "not a mapping")
        return out
    with open(path, encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            if "[REDACTED:" in line:
                # pkg/claudebridge/privacy placeholders: a model saw a filtered
                # prompt and wrote the placeholder back. Never a real value.
                add("redaction-placeholder", f"line {n}", "privacy-filter placeholder written into the runbook")

    for k in TOP_REQUIRED:
        if k not in rb:
            add("required-key", k, f"top-level '{k}' missing")
    if rb.get("apiVersion", "runbook/v1") != "runbook/v1":
        add("api-version", "apiVersion", f"{rb.get('apiVersion')!r} is not runbook/v1")
    if rb.get("kind", "Runbook") != "Runbook":
        add("api-version", "kind", f"{rb.get('kind')!r} is not Runbook")
    steps = rb.get("steps")
    if "steps" in rb and not isinstance(steps, list):
        add("required-key", "steps", "steps is not a list")
        steps = []
    steps = steps or []

    meta = rb.get("metadata") if isinstance(rb.get("metadata"), dict) else {}
    for k in META_REQUIRED:
        if k not in meta:
            add("metadata-key", k, f"metadata.{k} missing")
    stem = os.path.splitext(os.path.basename(path))[0]
    if "name" in meta and meta.get("name") != stem:
        add("name-mismatch", "metadata.name", f"{meta.get('name')!r} != file stem {stem!r}")
    if "risk" in meta and meta.get("risk") not in RISKS:
        add("risk", "metadata.risk", f"{meta.get('risk')!r} not in {'/'.join(RISKS)}")

    declared = set((rb.get("env") or {}).keys()) if isinstance(rb.get("env"), dict) else set()
    for p in rb.get("parameters") or rb.get("params") or []:
        if isinstance(p, dict) and p.get("name"):
            declared.add(p["name"])

    strict = "status" in meta
    if strict:
        check_strict_meta(meta, root, add)

    seen = {}
    for section in ("steps", "rollback"):
        for i, st in enumerate(steps if section == "steps" else entries(rb, "rollback")):
            check_step(section, i, st, declared, strict, add, seen if section == "steps" else None)

    for i, pc in enumerate(entries(rb, "preconditions")):
        if isinstance(pc, dict) and isinstance(pc.get("check"), str):
            err = bash_n(pc["check"])
            if err:
                add("bash-n", f"preconditions[{i}].check", err)
    return out


def check_strict_meta(meta, root, add):
    status = meta.get("status")
    if status not in STATUSES:
        add("status", "metadata.status", f"{status!r} not in {'/'.join(STATUSES)}")
    if meta.get("authored_by") not in AUTHORS:
        add("authored-by", "metadata.authored_by",
            f"{meta.get('authored_by')!r} not in {'/'.join(AUTHORS)}")
    if not isinstance(meta.get("reviewed_by"), str):
        add("reviewed-by", "metadata.reviewed_by", "missing (a reviewer id, or \"\")")
    if "approved_by" not in meta or not isinstance(meta.get("approved_by"), str):
        add("approved-by", "metadata.approved_by", "missing")
    elif status == "draft" and meta["approved_by"] != "":
        add("approved-by", "metadata.approved_by", "must be \"\" while status is draft")
    elif status == "approved" and not meta["approved_by"].strip():
        add("approved-by", "metadata.approved_by", "status approved needs an approver")
    t = meta.get("dry_run_test")
    if not isinstance(t, str) or not t:
        add("dry-run-test", "metadata.dry_run_test", "missing")
    elif os.path.isabs(t) or ".." in t.split("/"):
        add("dry-run-test", "metadata.dry_run_test", f"{t!r} must be repo-relative")
    elif not os.path.isfile(os.path.join(root, t)):
        add("dry-run-test", "metadata.dry_run_test", f"{t} does not exist")


def check_step(section, i, st, declared, strict, add, seen):
    if not isinstance(st, dict):
        add("step-key", f"{section}[{i}]", "not a mapping")
        return
    name = st.get("name")
    where = f"{section}:{name}" if isinstance(name, str) and name else f"{section}[{i}]"
    for k in ("name", "action", "command"):
        if k not in st:
            add("step-key", where, f"'{k}' missing")
    action = st.get("action", "shell")
    if action not in ACTIONS:
        add("action", where, f"{action!r} not in {'/'.join(ACTIONS)}")
    if seen is not None and isinstance(name, str):
        if name in seen:
            add("step-name-dup", where, f"name repeats step {seen[name] + 1}")
        else:
            seen[name] = i
    host = st.get("host")
    if host is not None:
        m = re.fullmatch(r"\$\{([A-Za-z_]\w*)\}", str(host))
        if not (host in HOSTS or (m and m.group(1) in declared)):
            add("host", where, f"{host!r} not west/east or a declared ${{PARAM}}")
    if action != "shell":
        return  # manual text is prose with <placeholders>; an unknown action was reported above
    fields = ("command", "verify") if section == "steps" else ("command",)
    texts = [(f, st.get(f)) for f in fields if isinstance(st.get(f), str) and st.get(f).strip()]
    for f, text in texts:
        err = bash_n(text)
        if err:
            add("bash-n", f"{where}.{f}", err)
    if not strict:
        return
    for f, text in texts:
        if "\n" in text.strip() and not starts_with_set_euo(text):
            add("set-euo", f"{where}.{f}", "multi-line shell must start with `set -euo pipefail`")
    if st.get("irreversible") is True:
        add("irreversible-manual", where, "irreversible: true needs action: manual")
    for f, text in texts:
        for rx, what in IRREVERSIBLE:
            if rx.search(text):
                add("irreversible-manual", f"{where}.{f}", f"{what} in a shell step: make it action: manual")
                break


def collect(paths):
    files = []
    for p in paths:
        if os.path.isdir(p):
            for d, dirs, names in os.walk(p):
                dirs.sort()
                files.extend(os.path.join(d, n) for n in sorted(names) if n.endswith((".yaml", ".yml")))
        elif os.path.isfile(p):
            files.append(p)
        else:
            raise FileNotFoundError(p)
    return files


def relpath(path, root):
    a = os.path.abspath(path)
    r = os.path.relpath(a, root)
    return a if r.startswith("..") else r


def read_baseline(path):
    if not path or not os.path.exists(path):
        return set()
    with open(path, encoding="utf-8") as f:
        return {ln.strip() for ln in f if ln.strip() and not ln.startswith("#")}


def main(argv=None):
    ap = argparse.ArgumentParser(description="Validate runbook/v1 YAML, ratcheted by a shrink-only baseline.")
    ap.add_argument("paths", nargs="+", help="runbook files or directories")
    ap.add_argument("--root", default=ROOT_DEFAULT, help="repo root (paths in output and baseline are relative to it)")
    ap.add_argument("--baseline", help="baseline file (default <root>/runbooks/.check-baseline)")
    ap.add_argument("--no-baseline", action="store_true", help="report every finding")
    ap.add_argument("--write-baseline", metavar="FILE", help="write the current findings as the baseline; refuses to grow an existing one")
    a = ap.parse_args(argv)
    root = os.path.abspath(a.root)

    try:
        files = collect(a.paths)
    except FileNotFoundError as e:
        print(f"check-runbooks: no such file or directory: {e}", file=sys.stderr)
        return 2
    findings = []
    for f in files:
        findings.extend(check_file(f, relpath(f, root), root))
    current = {x.key() for x in findings}
    checked = {relpath(f, root) for f in files}

    if a.write_baseline:
        old = read_baseline(a.write_baseline)
        if os.path.exists(a.write_baseline):
            grow = current - old
            if grow:
                print("check-runbooks: refusing to grow the baseline (it may only shrink); new:", file=sys.stderr)
                for k in sorted(grow):
                    print(f"  {k}", file=sys.stderr)
                return 1
        keep = sorted(k for k in old if k.split("|", 1)[0] not in checked) + sorted(current)
        with open(a.write_baseline, "w", encoding="utf-8") as f:
            f.write("# runbooks/.check-baseline: known runbook/v1 problems (scripts/check-runbooks.py).\n")
            f.write("# May only shrink. Fix a runbook, then delete its lines. path|rule|where\n")
            for k in sorted(set(keep)):
                f.write(k + "\n")
        print(f"check-runbooks: {len(files)} runbook(s), baseline written with {len(current)} entr{'y' if len(current) == 1 else 'ies'} for them")
        return 0

    base = set() if a.no_baseline else read_baseline(a.baseline or os.path.join(root, "runbooks", ".check-baseline"))
    new = [x for x in findings if x.key() not in base]
    stale = sorted(k for k in base if k.split("|", 1)[0] in checked and k not in current)
    for x in new:
        print(x)
    for k in stale:
        print(f"removable baseline entry (no longer fails; delete the line): {k}")
    known = len(findings) - len(new)
    print(f"check-runbooks: {len(files)} runbook(s), {len(new)} new finding(s), {known} baselined, {len(stale)} removable")
    return 1 if new or stale else 0


if __name__ == "__main__":
    sys.exit(main())
