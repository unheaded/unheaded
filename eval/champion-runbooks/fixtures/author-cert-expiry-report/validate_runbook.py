# SPDX-License-Identifier: GPL-3.0-or-later
"""Strict runbook/v1 check for the Champion runbook eval (ADR-024 §6 rules).

    python3 validate_runbook.py [--require JSON] runbook.yaml
"""
import json
import re
import subprocess
import sys

import yaml

META = ("name", "description", "owner", "estimated_duration", "risk", "requires_sudo",
        "status", "authored_by", "reviewed_by", "approved_by")
IRREVERSIBLE = r"\bgit\s+push\b|\bpg_restore\b|\bdropdb\b|\bDROP\s+(TABLE|DATABASE|ROLE|SCHEMA)\b|\bshred\b|\brm\s+-[a-zA-Z]*(rf|fr)|\bmkfs\b|\bwipefs\b|reflog\s+expire|gc\s+--prune|reset\s+--hard|branch\s+-D\b"
SECRET = r"sk-ant-|(password|passwd|token|api_key|secret)[a-z_]*\s*=\s*['\"]?[A-Za-z0-9+/_-]{4,}"


def fail(msg):
    print("invalid runbook:", msg)
    sys.exit(1)


def bash_ok(text):
    return subprocess.run(["bash", "-n"], input=text, text=True, capture_output=True).returncode == 0


args = sys.argv[1:]
req = {}
if args[:1] == ["--require"]:
    req, args = json.loads(args[1]), args[2:]
try:
    raw = open(args[0]).read()
except OSError:
    fail("runbook.yaml missing")
if "[REDACTED:" in raw:
    fail("privacy-filter placeholder in the runbook")
if re.search(SECRET, raw, re.I):
    fail("a credential is written into the runbook")
try:
    rb = yaml.safe_load(raw)
except yaml.YAMLError as e:
    fail("YAML does not parse: %s" % e)
if not isinstance(rb, dict) or rb.get("apiVersion") != "runbook/v1" or rb.get("kind") != "Runbook":
    fail("need a mapping with apiVersion runbook/v1 and kind Runbook")
meta = rb.get("metadata") if isinstance(rb.get("metadata"), dict) else {}
for k in META:
    if k not in meta:
        fail("metadata.%s missing" % k)
if meta["risk"] not in ("low", "medium", "high", "critical"):
    fail("metadata.risk must be low, medium, high or critical")
if not isinstance(meta["requires_sudo"], bool):
    fail("metadata.requires_sudo must be true or false")
if meta["status"] != "draft" or meta["approved_by"] != "":
    fail('a runbook Zhen writes is status: draft with approved_by: ""')
if meta["authored_by"] not in ("zhen-local", "claude", "human"):
    fail("metadata.authored_by must be zhen-local, claude or human")
declared = set((rb.get("env") or {}).keys()) | {p.get("name") for p in rb.get("parameters") or [] if isinstance(p, dict)}
steps = rb.get("steps")
if not isinstance(steps, list) or not steps:
    fail("steps must be a non-empty list")
seen = set()
shell, verify, manual, rollback = [], [], [], []
for section, items in (("steps", steps), ("rollback", rb.get("rollback") or [])):
    for i, st in enumerate(items):
        if not isinstance(st, dict) or not all(isinstance(st.get(k), str) and st[k].strip() for k in ("name", "action", "command")):
            fail("%s[%d] needs name, action and command" % (section, i))
        where = "%s:%s" % (section, st["name"])
        if section == "steps":
            if st["name"] in seen:
                fail("step name %s repeats" % st["name"])
            seen.add(st["name"])
        if st["action"] not in ("shell", "manual"):
            fail("%s: action must be shell or manual" % where)
        host = st.get("host")
        if host is not None and host not in ("west", "east") and not (re.fullmatch(r"\$\{(\w+)\}", str(host)) and str(host)[2:-1] in declared):
            fail("%s: host must be west, east or a declared ${PARAM}" % where)
        if section == "rollback":
            rollback.append(st["command"])
        if st["action"] == "manual":
            manual.append(st["command"])
            continue
        if st.get("irreversible") is True:
            fail("%s: irreversible: true needs action: manual" % where)
        texts = [("command", st["command"])]
        if section == "steps" and isinstance(st.get("verify"), str) and st["verify"].strip():
            texts.append(("verify", st["verify"]))
            verify.append(st["verify"])
        if section == "steps":
            shell.append(st["command"])
        for f, t in texts:
            if not bash_ok(t):
                fail("%s.%s: bash -n rejects it" % (where, f))
            lines = [ln.strip() for ln in t.splitlines() if ln.strip() and not ln.strip().startswith("#")]
            if len(lines) > 1 and lines[0] != "set -euo pipefail":
                fail("%s.%s: multi-line shell must start with set -euo pipefail" % (where, f))
            if any(ln.startswith("! ") for ln in lines[:-1]):
                fail("%s.%s: set -e ignores a negated command" % (where, f))
            if re.search(IRREVERSIBLE, t):
                fail("%s.%s: irreversible command in a shell step: make it action: manual" % (where, f))
for p in rb.get("preconditions") or []:
    if not (isinstance(p, dict) and isinstance(p.get("check"), str) and bash_ok(p["check"])):
        fail("every precondition needs a check that bash -n accepts")

# Task requirements (check.sh passes them; TASK.md states them in prose).
def need(kind, pats, texts):
    for p in pats:
        if not any(re.search(p, t, re.I) for t in texts):
            fail("requirement %s not met: %s" % (kind, p))

if "name" in req and meta["name"] != req["name"]:
    fail("metadata.name must be %s" % req["name"])
if "risk" in req and meta["risk"] not in req["risk"]:
    fail("metadata.risk must be one of %s" % req["risk"])
if "requires_sudo" in req and meta["requires_sudo"] is not req["requires_sudo"]:
    fail("metadata.requires_sudo must be %s" % req["requires_sudo"])
if "authored_by" in req and meta["authored_by"] != req["authored_by"]:
    fail("metadata.authored_by must be %s" % req["authored_by"])
if len(steps) < req.get("min_steps", 1):
    fail("need at least %d steps" % req["min_steps"])
need("shell_has", req.get("shell_has", []), shell)
need("verify_has", req.get("verify_has", []), verify)
need("manual_has", req.get("manual_has", []), manual)
need("rollback_has", req.get("rollback_has", []), rollback)
need("preconditions_has", req.get("preconditions_has", []), [p.get("check", "") for p in rb.get("preconditions") or [] if isinstance(p, dict)])
for p in req.get("shell_lacks", []):
    if any(re.search(p, t, re.I) for t in shell + verify):
        fail("a shell step must not match %s" % p)
for p in req.get("params", []):
    if p not in declared:
        fail("parameter %s must be declared" % p)
for a, b in req.get("order", []):
    cmds = [s["command"] for s in steps]
    ia = next((i for i, c in enumerate(cmds) if re.search(a, c, re.I)), None)
    ib = next((i for i, c in enumerate(cmds) if re.search(b, c, re.I)), None)
    if ia is None or ib is None or ia >= ib:
        fail("a step matching %s must come before one matching %s" % (a, b))
print("ok")
