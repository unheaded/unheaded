#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# No compose file holds a credential. The value of a secret-named key comes
# from outside the file: the caller's environment or a file on the host.
#
# gitleaks did not see the Grafana admin password that sat in
# tomb/dark-mirror/docker-compose.yml until 2026-11-21: a short lab word has
# no entropy and no vendor prefix for a rule to match. This gate goes by the
# key's name instead of the value's shape.
#
# A secret-named key is one whose name ends in PASSWORD, PASSWD, PASSPHRASE,
# SECRET, TOKEN or KEY (any case). It is read in each service's environment
# (map and list form, anchors and merge keys resolved) and build args. Its
# value passes when it is:
#   - absent or empty (list form "- NAME" passes the caller's value through);
#   - made only of variables that carry no literal: ${X}, $X, ${X:?message},
#     ${X?message}, ${X:-} and ${X-};
#   - a file reference: an absolute path under /run, /etc, /var, /opt or /srv.
# Everything else fails: a literal, a literal default (${X:-admin}), a literal
# replacement (${X:+admin}), text around a variable, $$X (a literal dollar).
#
# Under any key, a URL that carries a password (scheme://user:password@host)
# fails unless the password is such a variable. A top-level secret or config
# with its value inline (content:) fails.
#
# Covers every tracked compose file, tomb/ and test fixtures included: there is
# no list of exceptions. A fixture that needs a secret-named key takes it from
# a variable like any other file.
#
# Reads the files as written, like check-compose-hardening.sh: what a host's
# .env holds is not in the repository and is not this gate's business.
# Exit 1: a finding. Exit 2: the gate could not judge (no file, no key seen).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [[ $# -gt 0 ]]; then
	files=("$@")
else
	mapfile -t files < <(cd "$REPO_ROOT" && git ls-files | grep -E '(^|/)(docker-)?compose[^/]*\.ya?ml$' | sort)
fi
[[ ${#files[@]} -gt 0 ]] || { echo "check-compose-secrets: no compose files; refusing to pass vacuously" >&2; exit 2; }

python3 - "$REPO_ROOT" "${files[@]}" <<'PY'
import os, re, sys, yaml

root, files = sys.argv[1], sys.argv[2:]

SECRET_NAME = re.compile(r"(PASSWORD|PASSWD|PASSPHRASE|SECRET|TOKEN|KEY)$", re.IGNORECASE)
NAME = r"[A-Za-z_][A-Za-z0-9_]*"
# A variable that brings no text of its own: ${X}, ${X:?msg}, ${X?msg},
# ${X:-}, ${X-}, $X. A message may not hold a closing brace.
OUTSIDE = re.compile(r"\$\{" + NAME + r"(?::?\?[^}]*|:?-)?\}|\$" + NAME)
FILE_ROOTS = ("/run/", "/etc/", "/var/", "/opt/", "/srv/")
URL_PASSWORD = re.compile(r"[A-Za-z][A-Za-z0-9+.-]*://[^/@\s:]*:([^@\s]+)@")
ESCAPED_DOLLAR = "\x00"


def literal_left(value):
    """What the file itself contributes to the value, after its variables."""
    text = value.replace("$$", ESCAPED_DOLLAR)
    return OUTSIDE.sub("", text).replace(ESCAPED_DOLLAR, "$$")


def file_reference(value):
    return value.startswith(FILE_ROOTS) and not re.search(r"[\s$]", value)


def why_not(key, value):
    """Empty when the pair passes; else the reason. The value is never printed."""
    if value is None:
        return ""
    text = str(value).strip()
    if text == "":
        return ""
    if SECRET_NAME.search(key):
        if file_reference(text) or literal_left(text) == "":
            return ""
        if "$" in text:
            return "has a literal beside or inside its variable (a default, a replacement or surrounding text)"
        return "is a literal"
    for m in URL_PASSWORD.finditer(text.replace("$$", ESCAPED_DOLLAR)):
        if literal_left(m.group(1).replace(ESCAPED_DOLLAR, "$$")) != "":
            return "is a URL with a password written in it"
    return ""


def pairs(block):
    """(key, value) from an environment or args block in either form."""
    if isinstance(block, dict):
        return [(str(k), v) for k, v in block.items()]
    out = []
    for entry in block or []:
        key, sep, value = str(entry).partition("=")
        out.append((key, value if sep else None))
    return out


fail, judged = False, 0
for f in files:
    path = f if os.path.isabs(f) else os.path.join(root, f)
    rel = os.path.relpath(path, root)
    if not os.path.isfile(path):
        print(f"check-compose-secrets: no such file: {rel}", file=sys.stderr)
        sys.exit(2)
    try:
        doc = yaml.safe_load(open(path)) or {}
    except yaml.YAMLError as e:
        print(f"[FAIL] {rel}: not parseable YAML: {e}")
        fail = True
        continue
    if not isinstance(doc, dict):
        continue
    for name, spec in sorted((doc.get("services") or {}).items()):
        spec = spec or {}
        blocks = [("environment", spec.get("environment"))]
        build = spec.get("build")
        if isinstance(build, dict):
            blocks.append(("build.args", build.get("args")))
        for where, block in blocks:
            for key, value in pairs(block):
                if SECRET_NAME.search(key):
                    judged += 1
                reason = why_not(key, value)
                if reason:
                    print(f"[FAIL] {rel}:{name}: {where} {key} {reason}")
                    fail = True
    for kind in ("secrets", "configs"):
        for name, spec in sorted((doc.get(kind) or {}).items()):
            judged += 1
            if isinstance(spec, dict) and str(spec.get("content") or "").strip():
                if literal_left(str(spec["content"]).strip()) != "":
                    print(f"[FAIL] {rel}: {kind}.{name} has its value inline (content:)")
                    fail = True

if fail:
    print("Take the value out of the file: ${NAME:?set NAME - credentials are not stored in this repo},")
    print("or a file under /run/secrets. A value that was committed is in git history: rotate it.")
    sys.exit(1)
if judged == 0:
    print("check-compose-secrets: no secret-named key in any file; refusing to pass vacuously")
    sys.exit(2)
print(f"[PASS] {judged} secret-named keys and top-level secrets across {len(files)} compose files "
      f"take their value from outside the file")
PY
