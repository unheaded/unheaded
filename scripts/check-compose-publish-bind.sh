#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Every compose publish names the address it binds. A publish with no address
# listens on every interface, IPv4 and IPv6, through Docker's own NAT rules, so
# a host firewall's INPUT chain never sees that traffic.
#
# A publish passes when its address is internal: loopback, a Docker bridge
# (172.16.0.0/12), the west-east point-to-point link (192.168.13.0/24) or the
# WireGuard overlay (fd00:dead:beef::/48). An address that comes from a
# variable is judged by the default written in the file, so ${X:-127.0.0.1}
# passes and ${X} with no default fails: what it binds is not in the file.
#
# A publish that really is an edge is listed in
# docs/security/compose-publish-allowlist.txt as file:service:port/proto with
# a reason, and still has to write its address out (0.0.0.0 on purpose, never
# by omission). An entry that matches no such publish fails too.
#
# Covers every tracked compose file outside tomb/, in short and long form.
# Reads the files as written: the guarantee must be in the declared
# configuration, not in a host's .env.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ALLOWLIST="${COMPOSE_PUBLISH_ALLOWLIST:-${REPO_ROOT}/docs/security/compose-publish-allowlist.txt}"
[[ -f "$ALLOWLIST" ]] || { echo "check-compose-publish-bind: missing $ALLOWLIST" >&2; exit 2; }

if [[ $# -gt 0 ]]; then
	files=("$@")
else
	mapfile -t files < <(cd "$REPO_ROOT" && git ls-files | grep -E '(^|/)(docker-)?compose[^/]*\.ya?ml$' | grep -v '^tomb/' | sort)
fi
[[ ${#files[@]} -gt 0 ]] || { echo "check-compose-publish-bind: no compose files; refusing to pass vacuously" >&2; exit 2; }

python3 - "$REPO_ROOT" "$ALLOWLIST" "${files[@]}" <<'PY'
import ipaddress, os, re, sys, yaml

root, allowlist, files = sys.argv[1], sys.argv[2], sys.argv[3:]
INTERNAL = [ipaddress.ip_network(n) for n in
            ("127.0.0.0/8", "::1/128", "172.16.0.0/12", "192.168.13.0/24", "fd00:dead:beef::/48")]
WILDCARDS = {"0.0.0.0", "::"}

allowed = {}
for line in open(allowlist):
    line = line.strip()
    if not line or line.startswith("#"):
        continue
    key, sep, reason = line.partition(" # ")
    if not sep or not reason.strip():
        print(f"[FAIL] {allowlist}: {line}: an entry needs a reason after ' # '")
        sys.exit(1)
    allowed[key.strip()] = reason.strip()


def split_short(entry):
    """Split on colons that are outside ${...} and [...]."""
    parts, cur, depth = [], "", 0
    for ch in entry:
        if ch in "{[":
            depth += 1
        elif ch in "}]":
            depth -= 1
        if ch == ":" and depth == 0:
            parts.append(cur)
            cur = ""
        else:
            cur += ch
    return parts + [cur]


def parse(entry):
    """Return (address text or None, container port, protocol)."""
    if isinstance(entry, dict):
        host_ip = entry.get("host_ip")
        return (str(host_ip) if host_ip is not None else None,
                str(entry.get("target")), str(entry.get("protocol") or "tcp"))
    text = str(entry)
    proto = "tcp"
    if re.search(r"/(tcp|udp|sctp)$", text):
        text, proto = text.rsplit("/", 1)
    parts = split_short(text)
    return (parts[0] if len(parts) == 3 else None), parts[-1], proto


def resolved(address):
    """Literal address the file promises, or None if the file promises none."""
    m = re.fullmatch(r"\$\{[A-Za-z_][A-Za-z0-9_]*:?-([^}]*)\}", address)
    if m:
        address = m.group(1)
    elif "$" in address:
        return None
    return address.strip("[]") or None


fail, total, seen = False, 0, set()
for f in files:
    rel = os.path.relpath(os.path.join(root, f) if not os.path.isabs(f) else f, root)
    try:
        services = (yaml.safe_load(open(os.path.join(root, rel))) or {}).get("services") or {}
    except yaml.YAMLError as e:
        print(f"[FAIL] {rel}: not parseable YAML: {e}")
        fail = True
        continue
    for name, spec in sorted(services.items()):
        for entry in (spec or {}).get("ports") or []:
            total += 1
            address, target, proto = parse(entry)
            key = f"{rel}:{name}:{target}/{proto}"
            if (spec or {}).get("network_mode") == "host":
                print(f"[FAIL] {key}: network_mode: host ignores ports; drop the entry and bind the listener")
                fail = True
                continue
            if address is None:
                print(f"[FAIL] {key}: '{entry}' names no address, so it binds every interface")
                fail = True
                continue
            literal = resolved(address)
            if literal is None:
                print(f"[FAIL] {key}: '{entry}' has no default address in the file")
                fail = True
                continue
            try:
                ip = ipaddress.ip_address(literal)
            except ValueError:
                print(f"[FAIL] {key}: '{literal}' is not an IP address")
                fail = True
                continue
            if any(ip in n for n in INTERNAL if n.version == ip.version):
                continue
            seen.add(key)
            if key not in allowed:
                kind = "every interface" if literal in WILDCARDS else f"{literal}, which is not internal"
                print(f"[FAIL] {key}: binds {kind} and is not in {allowlist}")
                fail = True

for key in sorted(set(allowed) - seen):
    print(f"[FAIL] {allowlist} lists {key}, which no longer binds outside the internal set: remove it")
    fail = True
if total == 0:
    print("check-compose-publish-bind: no publishes found; refusing to pass vacuously")
    sys.exit(2)
if fail:
    sys.exit(1)
print(f"[PASS] {total} publishes across {len(files)} compose files name their address; "
      f"{len(allowed)} allowlisted edge publishes")
PY
