# SPDX-License-Identifier: GPL-3.0-or-later
"""Minimal runbook/v1 shape check (ADR-024) for the two-brains eval."""
import sys

import yaml

def fail(msg):
    print("invalid runbook:", msg)
    sys.exit(1)

try:
    doc = yaml.safe_load(open(sys.argv[1]))
except yaml.YAMLError as e:
    fail("YAML does not parse: %s" % e)
if not isinstance(doc, dict):
    fail("top level is not a mapping")
if doc.get("apiVersion") != "runbook/v1" or doc.get("kind") != "Runbook":
    fail("apiVersion must be runbook/v1 and kind Runbook")
meta = doc.get("metadata") or {}
for k in ("name", "description", "risk"):
    if not isinstance(meta.get(k), str) or not meta[k]:
        fail("metadata.%s missing" % k)
steps = doc.get("steps")
if not isinstance(steps, list) or not steps:
    fail("steps must be a non-empty list")
for i, s in enumerate(steps):
    if not isinstance(s, dict):
        fail("step %d is not a mapping" % i)
    for k in ("name", "action", "command", "verify"):
        if not isinstance(s.get(k), str) or not s[k].strip():
            fail("step %d (%s) lacks %s" % (i, s.get("name"), k))
print("ok")
