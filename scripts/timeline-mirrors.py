#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""timeline-mirrors.py: keep references/timeline.{json,yaml,toml} in step with timeline.md.

references/timeline.md is the record people edit. The JSON, YAML and TOML files
beside it carry the same milestones for programs. They were kept by hand and
fell behind (S-RESIDUE, backlog row D-26), so this script writes them.

  timeline-mirrors.py            same as --check
  timeline-mirrors.py --check    exit 1 and say what differs; write nothing
  timeline-mirrors.py --write    bring the three mirrors in step

What comes from where:
  timeline.md     which milestones exist, their text, their order, their phase
                  (the "### Age N" heading above the bullet) and whether they
                  are done (a bullet under a "**Completed ...:**" label is
                  completed, 100; any other bullet is pending, 0)
  timeline.json   everything else: version, status, the phases' names, status
                  and progress, metadata, and the names of milestones it
                  already holds

A milestone is a top-level "- " bullet under an Age heading. Its id is
phase-<N>-item-<first 8 hex of sha256 over the text after "- ">, so editing a
bullet's text changes its id. The script pairs the old id with the new one by
position and reports it as "text changed" instead of adding a second entry.

A new milestone's name is the bullet's bold lead without its closing
"(date, branch)." or, for a plain bullet, the text before the first " (".

YAML and TOML are written from the JSON data. --check parses all three and
compares the data; last_updated is compared as an instant.

Exit: 0 in step (or written), 1 drift found by --check, 2 usage or a file
that cannot be read.

Needs PyYAML (pinned in .github/ci-python/requirements.in). Everything else
is the standard library. tests/scripts/test_timeline_mirrors.py pins this.
"""
import argparse
import datetime
import difflib
import hashlib
import json
import os
import pathlib
import re
import sys
import tomllib

import yaml

DEFAULT_DIR = pathlib.Path(__file__).resolve().parents[1] / "references"
AGE = re.compile(r"^### Age (\d+):")
LABEL = re.compile(r"^\*\*[^*]+:\*\*\s*$")
MILESTONE_KEYS = ("id", "name", "status", "progress", "description")


class UsageError(Exception):
    """A file is missing or not in the shape this script writes."""


def name_of(text):
    """The short name of a new milestone, from its bullet text."""
    if text.startswith("**"):
        end = text.find("**", 2)
        lead = text[2:end] if end > 0 else text[2:]
        lead = lead.rstrip().rstrip(".").rstrip()
        return re.sub(r"\s*\([^()]*\)$", "", lead)
    return text.lstrip("*").split(" (", 1)[0]


def read_bullets(path):
    """The milestones timeline.md holds, in file order."""
    bullets = []
    phase = None
    done = False
    with open(path, encoding="utf-8") as fh:
        for number, raw in enumerate(fh, 1):
            line = raw.rstrip("\n")
            age = AGE.match(line)
            if age:
                phase, done = int(age.group(1)), False
            elif LABEL.match(line):
                done = line.startswith("**Completed")
            elif line.startswith("- "):
                if phase is None:
                    raise UsageError(f"{path.name}:{number}: a bullet above the first '### Age N:' heading")
                text = line[2:]
                digest = hashlib.sha256(text.encode()).hexdigest()[:8]
                bullets.append({
                    "id": f"phase-{phase}-item-{digest}",
                    "text": text,
                    "phase": f"phase-{phase}",
                    "status": "completed" if done else "pending",
                    "progress": 100 if done else 0,
                    "line": number,
                })
    ids = [b["id"] for b in bullets]
    for dup in sorted({i for i in ids if ids.count(i) > 1}):
        raise UsageError(f"{path.name}: two bullets with the same text ({dup})")
    return bullets


def plan(bullets, current):
    """What the JSON data should be, and what that changes.

    Returns (wanted, notes): wanted is the full data (last_updated untouched),
    notes is one line per difference between timeline.md and the JSON.
    """
    held = {m["id"]: m for m in current["milestones"]}
    wanted_ids = [b["id"] for b in bullets]
    held_ids = list(held)
    new = [i for i in wanted_ids if i not in held]
    gone = [i for i in held_ids if i not in wanted_ids]

    # An edited bullet shows up as one id gone and one new at the same place.
    retext = {}
    matcher = difflib.SequenceMatcher(a=held_ids, b=wanted_ids, autojunk=False)
    for tag, a0, a1, b0, b1 in matcher.get_opcodes():
        if tag == "replace":
            olds = [i for i in held_ids[a0:a1] if i in gone]
            news = [i for i in wanted_ids[b0:b1] if i in new]
            retext.update(zip(news, olds, strict=False))

    notes = []
    milestones = []
    for b in bullets:
        where = f"timeline.md:{b['line']}"
        if b["id"] in held:
            m = dict(held[b["id"]])
            if (m["status"], m["progress"]) != (b["status"], b["progress"]):
                notes.append(f"status changed: {b['id']} is {b['status']} in timeline.md, {m['status']} in the mirrors ({where})")
        elif b["id"] in retext:
            old = held[retext[b["id"]]]
            m = dict(old)
            if old["name"] == name_of(old["description"]):
                m["name"] = name_of(b["text"])
            notes.append(f"text changed: {old['id']} -> {b['id']} ({where})  {m['name']}")
        else:
            m = {"name": name_of(b["text"])}
            notes.append(f"{where} not in the mirrors: {b['id']}  {m['name']}")
        m.update(id=b["id"], status=b["status"], progress=b["progress"], description=b["text"])
        milestones.append({k: m[k] for k in MILESTONE_KEYS})
    for i in gone:
        if i not in retext.values():
            notes.append(f"in the mirrors but not in timeline.md: {i}  {held[i]['name']}")

    phases = []
    for p in current["phases"]:
        p = {k: v for k, v in p.items() if k != "milestones"}
        mine = [b["id"] for b in bullets if b["phase"] == p["id"]]
        if mine:
            p["milestones"] = mine
        phases.append(p)
    known = {p["id"] for p in phases}
    for b in bullets:
        if b["phase"] not in known:
            raise UsageError(f"timeline.md:{b['line']}: {b['phase']} has no entry under \"phases\" in timeline.json")

    wanted = dict(current)
    wanted["phases"] = phases
    wanted["milestones"] = milestones
    if not notes and wanted != current:
        notes.append("the phases' milestone lists or the milestone order differ from timeline.md")
    return wanted, notes


def instant(value):
    """last_updated as an aware datetime, whichever parser produced it."""
    if isinstance(value, str):
        value = datetime.datetime.fromisoformat(value)
    if value.tzinfo is None:
        value = value.replace(tzinfo=datetime.UTC)
    return value


def stamp(value):
    """An instant in the form the mirrors carry: 2026-10-28T21:31:46.444736Z."""
    return instant(value).astimezone(datetime.UTC).isoformat(timespec="microseconds").replace("+00:00", "Z")


def comparable(data):
    out = dict(data)
    out["last_updated"] = instant(out["last_updated"])
    return out


def first_difference(want, got, path=""):
    """Where two parsed mirrors first differ, as a short path."""
    if isinstance(want, dict) and isinstance(got, dict):
        for key in want:
            if key not in got:
                return f"{path}.{key} is missing"
            found = first_difference(want[key], got[key], f"{path}.{key}")
            if found:
                return found
        extra = [k for k in got if k not in want]
        return f"{path}.{extra[0]} is extra" if extra else None
    if isinstance(want, list) and isinstance(got, list):
        for index, (a, b) in enumerate(zip(want, got, strict=False)):
            label = a.get("id", index) if isinstance(a, dict) else index
            found = first_difference(a, b, f"{path}[{label}]")
            if found:
                return found
        return None if len(want) == len(got) else f"{path} has {len(got)} entries, not {len(want)}"
    return None if want == got else f"{path} differs"


# --- emitters ---------------------------------------------------------------

def to_json(data):
    return json.dumps(data, indent=2, ensure_ascii=False) + "\n"


class _Stamp(str):
    """A timestamp the YAML mirror writes unquoted."""


class _Dumper(yaml.SafeDumper):
    def increase_indent(self, flow=False, indentless=False):
        return super().increase_indent(flow, False)


_Dumper.add_representer(_Stamp, lambda dumper, value: dumper.represent_scalar("tag:yaml.org,2002:timestamp", str(value)))


def to_yaml(data):
    data = dict(data)
    data["last_updated"] = _Stamp(stamp(data["last_updated"]))
    return yaml.dump(data, Dumper=_Dumper, sort_keys=False, allow_unicode=True, width=1 << 30, default_flow_style=False)


_TOML_ESCAPES = {"\\": "\\\\", '"': '\\"', "\b": "\\b", "\t": "\\t", "\n": "\\n", "\f": "\\f", "\r": "\\r"}


def _toml_value(value):
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, int):
        return str(value)
    if isinstance(value, str):
        out = []
        for ch in value:
            if ch in _TOML_ESCAPES:
                out.append(_TOML_ESCAPES[ch])
            elif ord(ch) < 0x20 or ord(ch) == 0x7F:
                out.append(f"\\u{ord(ch):04X}")
            else:
                out.append(ch)
        return '"' + "".join(out) + '"'
    if isinstance(value, list) and all(isinstance(v, str) for v in value):
        return "[" + ", ".join(_toml_value(v) for v in value) + "]"
    raise UsageError(f"timeline.json holds a value the TOML mirror cannot carry: {value!r}")


def to_toml(data):
    """TOML for this file's shape only: scalars, [[phases]], [[milestones]], [metadata]."""
    shape = {"version", "last_updated", "status", "phases", "milestones", "metadata"}
    if set(data) != shape:
        raise UsageError(f"timeline.json's top-level keys are {sorted(data)}, not {sorted(shape)}")
    lines = [
        f"version = {_toml_value(data['version'])}",
        f"last_updated = {stamp(data['last_updated'])}",
        f"status = {_toml_value(data['status'])}",
    ]
    for table in ("phases", "milestones"):
        for entry in data[table]:
            lines += ["", f"[[{table}]]"]
            lines += [f"  {key} = {_toml_value(value)}" for key, value in entry.items()]
    lines += ["", "[metadata]"]
    lines += [f"  {key} = {_toml_value(value)}" for key, value in data["metadata"].items()]
    return "\n".join(lines) + "\n"


# --- commands ---------------------------------------------------------------

def load_json(directory):
    path = directory / "timeline.json"
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, ValueError) as err:
        raise UsageError(f"{path}: {err}") from err
    for key in ("last_updated", "phases", "milestones"):
        if key not in data:
            raise UsageError(f"{path}: no \"{key}\"")
    return data


def mirror_problems(directory, want):
    """Why timeline.yaml or timeline.toml does not carry the same data as `want`."""
    problems = []
    loaders = {
        "timeline.yaml": lambda fh: yaml.safe_load(fh.read().decode("utf-8")),
        "timeline.toml": tomllib.load,
    }
    for name, loader in loaders.items():
        try:
            with open(directory / name, "rb") as fh:
                got = loader(fh)
            if not isinstance(got, dict) or "last_updated" not in got:
                raise ValueError("not a mapping with last_updated")
            found = first_difference(comparable(want), comparable(got))
        except (OSError, ValueError, yaml.YAMLError) as err:
            problems.append(f"{name} does not parse: {str(err).splitlines()[0]}")
            continue
        if found:
            problems.append(f"{name} does not match timeline.json: {found}")
    return problems


def write_file(path, text):
    scratch = path.with_name(path.name + ".tmp")
    scratch.write_text(text, encoding="utf-8")
    os.replace(scratch, path)


def main(argv=None):
    parser = argparse.ArgumentParser(description="Keep timeline.{json,yaml,toml} in step with timeline.md.")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--check", action="store_true", help="say what differs and exit 1 (the default)")
    mode.add_argument("--write", action="store_true", help="write the three mirrors")
    parser.add_argument("--dir", type=pathlib.Path, default=DEFAULT_DIR, help="the directory holding timeline.md (default: references/)")
    args = parser.parse_args(argv)

    try:
        current = load_json(args.dir)
        bullets = read_bullets(args.dir / "timeline.md")
        wanted, notes = plan(bullets, current)
        if not args.write:
            notes += mirror_problems(args.dir, current)
            for note in notes:
                print(note)
            if notes:
                print(f"{len(notes)} difference(s). Run scripts/timeline-mirrors.py --write, then commit the three mirrors.")
                return 1
            print(f"timeline mirrors in step with timeline.md ({len(bullets)} milestones)")
            return 0

        for note in notes:
            print(note)
        if notes:
            wanted["last_updated"] = stamp(datetime.datetime.now(datetime.UTC))
        texts = {"timeline.json": to_json(wanted), "timeline.yaml": to_yaml(wanted), "timeline.toml": to_toml(wanted)}
        written = []
        for name, text in texts.items():
            path = args.dir / name
            if not path.exists() or path.read_text(encoding="utf-8") != text:
                write_file(path, text)
                written.append(name)
        left = mirror_problems(args.dir, wanted)
        if left:
            for problem in left:
                print(problem)
            print("the mirrors were written but do not read back the same; this is a bug in this script")
            return 1
        print(f"wrote {', '.join(written)}" if written else "nothing to write")
        return 0
    except UsageError as err:
        print(f"timeline-mirrors: {err}", file=sys.stderr)
        return 2
    except OSError as err:
        print(f"timeline-mirrors: {err}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
