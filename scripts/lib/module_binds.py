# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""The module bind rule (ADR-149 decision 6, S-ZHEN-SWITCH D5 (a)), the same
order of checks as pkg/modules.CheckBind: a module's listener is loopback, or
a private address on the host's allowlist; a wildcard, a link-local and a
globally routable address are refused always.

Two callers, one rule:
  module_binds.py units ROOT               the static gate (scripts/check-module-binds.sh):
                                           every listen flag and *_ADDR/*_LISTEN value in a
                                           module's units
  module_binds.py guard SRC TEXTFILE HOST  the live guard (scripts/monitoring/module-listener-guard.sh):
                                           every listening socket on a module's ports, from ss

Standard library and PyYAML only."""
import ipaddress
import os
import re
import subprocess
import sys
import time
from pathlib import Path

import yaml

LISTEN_FLAGS = ("-host", "--host", "-listen", "--listen", "-metrics-addr", "--metrics-addr", "--metrics")


def parse_allow(prefixes):
    """Prefixes from module.yaml; a bad or non-private one is an error."""
    out = []
    for p in prefixes or []:
        net = ipaddress.ip_network(str(p), strict=False)
        if not net.is_private or net.is_link_local or net.prefixlen == 0:
            raise ValueError(f"bind_allow {p}: not private")
        out.append(net)
    return out


def split_hostport(value):
    """(host, port) from host, host:port, [v6]:port, *:port; port may be None."""
    v = value.strip()
    if v.startswith("["):
        host, _, rest = v[1:].partition("]")
        port = rest[1:] if rest.startswith(":") else None
        return host, port
    if v.count(":") == 1:
        host, port = v.split(":")
        return host, port
    return v, None  # a bare host, v4 or v6


def classify(value, allow):
    """None when value may be bound, else the reason it may not."""
    host, _ = split_hostport(value)
    if host in ("", "*"):
        return "wildcard address"
    host = host.split("%", 1)[0]
    try:
        addr = ipaddress.ip_address(host)
    except ValueError:
        return "not an IP literal"
    if isinstance(addr, ipaddress.IPv6Address) and addr.ipv4_mapped:
        addr = addr.ipv4_mapped
    if addr.is_unspecified:
        return "wildcard address"
    if addr.is_loopback:
        return None
    if addr.is_link_local:
        return "link-local address"
    if is_private(addr):
        if any(addr in net for net in allow):
            return None
        return "private address not on this host's allowlist"
    return "globally routable address"


def is_private(addr):
    """RFC 1918 and RFC 4193 only (as Go's netip.Addr.IsPrivate)."""
    if isinstance(addr, ipaddress.IPv4Address):
        return any(addr in ipaddress.ip_network(n) for n in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"))
    return addr in ipaddress.ip_network("fc00::/7")


def modules_of(src):
    for manifest in sorted(Path(src).glob("*/module.yaml")):
        yield manifest.parent.name, yaml.safe_load(manifest.read_text(encoding="utf-8")) or {}


def unit_values(text):
    """(flag, value) pairs from ExecStart= lines (continuations joined) and
    Environment= values whose name ends in _ADDR or _LISTEN."""
    joined = re.sub(r"\\\n\s*", " ", text)
    out = []
    for line in joined.splitlines():
        line = line.strip()
        if line.startswith("ExecStart="):
            words = line[len("ExecStart="):].split()
            for i, w in enumerate(words):
                for f in LISTEN_FLAGS:
                    if w == f and i + 1 < len(words):
                        out.append((f, words[i + 1]))
                    elif w.startswith(f + "="):
                        out.append((f, w[len(f) + 1:]))
        elif line.startswith("Environment="):
            for kv in line[len("Environment="):].split():
                name, _, val = kv.strip('"').partition("=")
                if name.endswith("_ADDR") or name.endswith("_LISTEN"):
                    out.append((name, val))
    return out


def check_units(root):
    root = Path(root)
    fails, units, addrs = [], 0, 0
    for name, m in modules_of(root / "modules"):
        allow = []
        for prefixes in (m.get("bind_allow") or {}).values():
            allow += parse_allow(prefixes)
        for u in m.get("units") or []:
            path = root / "deploy/systemd" / u["unit"]
            if not path.is_file():
                fails.append(f"[FAIL] module {name}: unit {u['unit']} not found")
                continue
            units += 1
            for flag, value in unit_values(path.read_text(encoding="utf-8")):
                if value.startswith("$"):
                    continue  # set at run time: the binaries' own rule holds it
                addrs += 1
                why = classify(value, allow)
                if why:
                    fails.append(f"[FAIL] module {name}: {u['unit']} {flag} {value}: {why}")
    for f in fails:
        print(f)
    if fails:
        return 1
    print(f"[PASS] module binds: {units} units, {addrs} addresses, none public")
    return 0


def read_listeners():
    """ss -ltnH's output. A failed ss raises: the guard then writes nothing,
    the old textfile stays and its last-run timestamp ages, so
    ModuleListenerGuardNotRunning fires rather than a fresh public=0 (REV-4)."""
    r = subprocess.run(["ss", "-ltnH"], capture_output=True, text=True, check=False, timeout=30)
    if r.returncode != 0:
        raise RuntimeError(f"ss -ltnH exited {r.returncode}: {r.stderr.strip()[:200]}")
    return r.stdout


def guard(src, textfile, host):
    lines = []
    total = {}
    try:
        ss = read_listeners()
    except (OSError, RuntimeError, subprocess.TimeoutExpired) as e:
        print(f"module-listener-guard: {e}; textfile left as it was", file=sys.stderr)
        return 1
    listening = [ln.split()[3] for ln in ss.splitlines() if len(ln.split()) >= 4]
    for name, m in modules_of(src):
        ports = {str(p) for p in m.get("ports") or []}
        allow = parse_allow((m.get("bind_allow") or {}).get(host, []))
        total[name] = 0
        seen = set()
        for local in listening:
            h, port = split_hostport(local)
            if port not in ports:
                continue
            why = classify(local, allow)
            if not why or (port, h) in seen:
                continue
            seen.add((port, h))
            total[name] += 1
            lines.append(f'unheaded_module_listener_public{{module="{name}",port="{port}",addr="{h}"}} 1')
    out = Path(textfile) / "module_listeners.prom"
    tmp = Path(textfile) / ".module_listeners.prom.tmp"
    body = ["# HELP unheaded_module_listener_public A module listener on a public or unlisted address (ADR-149).",
            "# TYPE unheaded_module_listener_public gauge", *lines,
            "# HELP unheaded_module_listener_public_total Module listeners on a public or unlisted address.",
            "# TYPE unheaded_module_listener_public_total gauge"]
    body += [f'unheaded_module_listener_public_total{{module="{n}"}} {c}' for n, c in sorted(total.items())]
    body += ["# HELP unheaded_module_listener_guard_last_run_timestamp_seconds When the module listener guard last ran.",
             "# TYPE unheaded_module_listener_guard_last_run_timestamp_seconds gauge",
             f"unheaded_module_listener_guard_last_run_timestamp_seconds {int(time.time())}"]
    tmp.write_text("\n".join(body) + "\n", encoding="utf-8")
    os.chmod(tmp, 0o644)
    os.replace(tmp, out)
    return 0


def main(argv):
    if len(argv) == 2 and argv[0] == "units":
        return check_units(argv[1])
    if len(argv) == 4 and argv[0] == "guard":
        return guard(argv[1], argv[2], argv[3])
    print("usage: module_binds.py units ROOT | guard SRC TEXTFILE HOST", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
