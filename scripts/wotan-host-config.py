#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""wotan-host-config.py: a host's copy of Wotan's topic config, with its keys.

The repository's configs/wotan.yaml ships every signed prefix with `keys: []`:
public keys are enrolled per deployment, never in the checked-in file
(wiki/Wotan-Topic-Signing.md). A host therefore runs a copy that differs from
the repository's file in exactly those lines. This script writes that copy
from the two inputs and says when it has drifted, so a change to
auto_approve or permissions in the repository reaches the host and a key is
never lost by copying the file by hand.

  scripts/wotan-host-config.py --keys KEYS --out OUT            # write OUT if it differs
  scripts/wotan-host-config.py --keys KEYS --out OUT --check    # compare only

  --source FILE   the repository's config (default: configs/wotan.yaml beside this script)
  --keys FILE     the host's enrolled public keys, one per line:
                      <prefix> <base64 ML-DSA-65 public key>   # optional comment
                  blank lines and lines starting with # are ignored; a prefix
                  may have several lines (several producers, or a rotation)
  --out FILE      the host's copy, the file Wotan reads there
  --check         write nothing; exit 4 when OUT is missing or differs

The keys file holds public keys only and may be world-readable. OUT is
written in place (same inode): Wotan's container mounts that one file, and a
rename would leave it reading the old one. Wotan does not apply a change of
signing keys on SIGHUP: restart it after a write.

Exit: 0 in step (or written), 1 refused (bad input), 2 usage, 4 drift (--check).
"""
import argparse
import base64
import binascii
import pathlib
import re
import sys

MLDSA65_PUBLIC_KEY_BYTES = 1952  # FIPS 204, ML-DSA-65

PREFIX_LINE = re.compile(r'^(\s*)-\s+prefix:\s+"([^"]+)"')
KEYS_LINE = re.compile(r"^(\s*)keys:\s*(\[.*\])\s*(#.*)?$")


class Refused(Exception):
    """An input this script will not render from."""


def parse_keys(text):
    """Return {prefix: [base64, ...]} in file order, refusing anything malformed."""
    enrolled = {}
    for n, raw in enumerate(text.splitlines(), 1):
        line = raw.split("#", 1)[0].strip()
        if not line:
            continue
        parts = line.split()
        if len(parts) != 2:
            raise Refused(f"keys line {n}: want '<prefix> <base64 public key>', got {len(parts)} field(s)")
        prefix, key = parts
        if not prefix.endswith("."):
            raise Refused(f"keys line {n}: prefix '{prefix}' must end in '.'")
        try:
            decoded = base64.b64decode(key, validate=True)
        except (binascii.Error, ValueError) as err:
            raise Refused(f"keys line {n}: not standard base64 ({err})") from err
        if len(decoded) != MLDSA65_PUBLIC_KEY_BYTES:
            raise Refused(
                f"keys line {n}: {len(decoded)} bytes, an ML-DSA-65 public key is {MLDSA65_PUBLIC_KEY_BYTES}"
            )
        if key in enrolled.setdefault(prefix, []):
            raise Refused(f"keys line {n}: the same key is listed twice for '{prefix}'")
        enrolled[prefix].append(key)
    return enrolled


def render(source, enrolled):
    """Return the source text with each enrolled prefix's `keys: []` filled."""
    lines = source.splitlines(keepends=True)
    out = []
    current = None  # the signed prefix whose keys line comes next
    seen = set()
    for line in lines:
        body = line.rstrip("\n")
        m = PREFIX_LINE.match(body)
        if m:
            current = m.group(2)
            out.append(line)
            continue
        k = KEYS_LINE.match(body)
        if k and current is not None:
            if k.group(2).replace(" ", "") != "[]":
                raise Refused(
                    f"the source already lists keys for '{current}': the repository's file ships keys: [] "
                    "and a host's keys belong in the keys file"
                )
            if current in enrolled:
                listed = ", ".join(f'"{key}"' for key in enrolled[current])
                out.append(f"{k.group(1)}keys: [{listed}]\n")
                seen.add(current)
            else:
                out.append(line)
            current = None
            continue
        out.append(line)
    missing = [p for p in enrolled if p not in seen]
    if missing:
        raise Refused(
            "the source has no signed prefix " + ", ".join(f"'{p}'" for p in missing)
            + ": a key enrolled for it would be dropped"
        )
    return "".join(out)


def main(argv=None):
    here = pathlib.Path(__file__).resolve().parent
    ap = argparse.ArgumentParser(description="Write or check a host's copy of Wotan's topic config.")
    ap.add_argument("--source", default=str(here.parent / "configs" / "wotan.yaml"))
    ap.add_argument("--keys", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--check", action="store_true")
    args = ap.parse_args(argv)

    try:
        source = pathlib.Path(args.source).read_text(encoding="utf-8")
        enrolled = parse_keys(pathlib.Path(args.keys).read_text(encoding="utf-8"))
        want = render(source, enrolled)
    except OSError as err:
        print(f"wotan-host-config: {err}", file=sys.stderr)
        return 1
    except Refused as err:
        print(f"wotan-host-config: refused: {err}", file=sys.stderr)
        return 1

    out = pathlib.Path(args.out)
    try:
        have = out.read_text(encoding="utf-8") if out.exists() else None
    except OSError as err:
        print(f"wotan-host-config: {err}", file=sys.stderr)
        return 1
    n_keys = sum(len(v) for v in enrolled.values())
    if have == want:
        print(f"wotan-host-config: {out} is in step ({n_keys} key(s) over {len(enrolled)} prefix(es))")
        return 0
    if args.check:
        what = "is missing" if have is None else "differs from the repository's file plus the enrolled keys"
        print(f"wotan-host-config: DRIFT: {out} {what}; run without --check, then restart Wotan", file=sys.stderr)
        return 4
    try:
        with open(out, "w", encoding="utf-8") as fh:  # in place: the container mounts this inode
            fh.write(want)
    except OSError as err:
        print(f"wotan-host-config: {err}", file=sys.stderr)
        return 1
    print(f"wotan-host-config: wrote {out} ({n_keys} key(s) over {len(enrolled)} prefix(es)); restart Wotan to apply")
    return 0


if __name__ == "__main__":
    sys.exit(main())
