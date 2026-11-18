# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Zhen's rows move from unheaded_app into unheaded_zhen through
scripts/well/zhen-copy-rows.sh (ADR-149; S-ZHEN-SWITCH Phase 8B, REV-6).
Without a database: its column map names every column 052 creates, a column
052 tightened gets its default when the old row held NULL, the tables are the
ones 053 drops, the record it writes is the one 053's guard reads, and the
fingerprint comparison passes only on the listed changes."""
import importlib.util
import pathlib
import re
import subprocess

import pytest

REPO = pathlib.Path(__file__).resolve().parents[2]
COPY = REPO / "scripts/well/zhen-copy-rows.sh"
SCHEMA = REPO / "db/migrations/052_zhen_schema.sql"
DROP = REPO / "db/migrations/053_app_drop_zhen.sql"
CHANGES = REPO / "scripts/well/zhen-schema-changes.txt"


def split_top(text):
    """Split at commas outside parentheses and quotes."""
    out, depth, quote, cur = [], 0, False, ""
    for ch in text:
        if ch == "'":
            quote = not quote
        elif not quote and ch == "(":
            depth += 1
        elif not quote and ch == ")":
            depth -= 1
        if ch == "," and depth == 0 and not quote:
            out.append(cur.strip())
            cur = ""
        else:
            cur += ch
    if cur.strip():
        out.append(cur.strip())
    return out


def schema_columns():
    """table -> the columns 052 creates that COPY can write (no STORED generated ones)."""
    sql = re.sub(r"--[^\n]*", "", SCHEMA.read_text(encoding="utf-8"))
    tables = {}
    for m in re.finditer(r"CREATE TABLE (\w+) \(", sql):
        depth, i = 1, m.end()
        while depth:
            depth += {"(": 1, ")": -1}.get(sql[i], 0)
            i += 1
        cols = []
        for el in split_top(sql[m.end():i - 1]):
            first = el.split()[0]
            if first.upper() in ("CONSTRAINT", "CHECK", "PRIMARY", "UNIQUE", "FOREIGN", "EXCLUDE"):
                continue
            if re.search(r"\bGENERATED\s+ALWAYS\s+AS\s*\(.*\bSTORED\b", el, re.DOTALL | re.IGNORECASE):
                continue
            cols.append(first)
        tables[m.group(1)] = cols
    return tables


def copy_map():
    """[(table, [target columns], [source expressions])] in the script's order."""
    text = COPY.read_text(encoding="utf-8")
    body = text[text.index("MAP=(") + len("MAP=("):]
    body = body[:body.index("\n)\n")]
    rows = []
    for line in body.splitlines():
        line = line.strip()
        if not line:
            continue
        table, cols, exprs = line.strip('"').split("|")
        rows.append((table, split_top(cols), split_top(exprs)))
    return rows


def changed_columns():
    out = set()
    for line in CHANGES.read_text(encoding="utf-8").splitlines():
        if line.startswith("column|"):
            out.add(tuple(line.split("|", 1)[1].split(".", 1)))
    return out


def test_map_names_every_column_052_creates():
    schema = schema_columns()
    rows = copy_map()
    assert [t for t, _, _ in rows] and {t for t, _, _ in rows} == set(schema)
    for table, cols, exprs in rows:
        assert len(cols) == len(exprs), table
        assert sorted(cols) == sorted(schema[table]), table


def test_tightened_columns_take_their_default_from_a_null():
    exprs = {(t, c): e for t, cols, es in copy_map() for c, e in zip(cols, es)}
    for table, col in changed_columns():
        if col == "id":            # identity: COPY writes the old value
            assert exprs[(table, col)] == "id"
            continue
        assert re.match(r"(coalesce|nullif)\(", exprs[(table, col)]), (table, col, exprs[(table, col)])


def test_tables_are_the_ones_053_drops():
    dropped = set(re.findall(r"^DROP TABLE (\w+);", DROP.read_text(encoding="utf-8"), re.MULTILINE))
    assert dropped == {t for t, _, _ in copy_map()}


def test_record_is_what_053_reads():
    # 053: regexp_match(rec, '(?:^|[ ,:])' || t || '=([0-9]+)(?:,|$)')
    pattern = re.search(r"'\(\?:\^\|\[ ,:\]\)' \|\| t \|\| '=\(\[0-9\]\+\)\(\?:,\|\$\)'",
                        DROP.read_text(encoding="utf-8"))
    assert pattern, "053's guard pattern changed: update this test with it"
    script = COPY.read_text(encoding="utf-8")
    assert "'copied $(date -u +%FT%TZ): $record'" in script
    rec = "copied 2026-11-17T20:00:00Z: zhen_action_snapshots=0,zhen_actions=3,zhen_memories=12"

    def read(t):
        m = re.search(r"(?:^|[ ,:])" + t + r"=([0-9]+)(?:,|$)", rec)
        return m and m.group(1)
    assert read("zhen_actions") == "3"
    assert read("zhen_action_snapshots") == "0"
    assert read("zhen_memories") == "12"
    assert read("zhen_captures") is None


def test_053_refuses_without_a_matching_record():
    sql = DROP.read_text(encoding="utf-8")
    guard = sql[sql.index("DO $$"):sql.index("END $$;")]
    assert "m IS NULL OR m[1]::bigint <> n" in guard
    assert "total > 0 AND rec = ''" in guard
    assert sql.index("END $$;") < sql.index("DROP TABLE")


def test_copy_script_is_valid_bash():
    subprocess.run(["bash", "-n", str(COPY)], check=True)


FAKE = '#!/bin/sh\necho "$0 $*" >>"$STATE/called"\nexit 1\n'


def run_copy(tmp_path, *args):
    """The script with a docker and a sudo that only record that they were
    called, and a container name no host has: nothing here reaches a Well."""
    bindir = tmp_path / "bin"
    bindir.mkdir(exist_ok=True)
    for name in ("docker", "sudo"):
        fake = bindir / name
        fake.write_text(FAKE)
        fake.chmod(0o755)
    env = {"PATH": f"{bindir}:/usr/bin:/bin", "STATE": str(tmp_path), "HOME": str(tmp_path),
           "WELL_CONTAINER": "qa-zhen-copy-none"}
    return subprocess.run(["bash", str(COPY), *args], env=env, capture_output=True, text=True, timeout=30, check=False)


def test_a_bad_command_line_is_refused_before_the_well_is_touched(tmp_path):
    # DECISION-PACK-2026-11-18 row 15: --null-bad-trace-id FILE is the only option.
    link = tmp_path / "link"
    link.symlink_to(tmp_path / "elsewhere")
    for args in (["--nope"], ["extra"], ["--null-bad-trace-id"], ["--null-bad-trace-id", ""],
                 ["--null-bad-trace-id", str(tmp_path / "missing" / "f")],
                 ["--null-bad-trace-id", str(tmp_path)], ["--null-bad-trace-id", str(link)],
                 ["--null-bad-trace-id", str(tmp_path / "f"), "extra"]):
        p = run_copy(tmp_path, *args)
        assert p.returncode == 2, f"{args}: exit {p.returncode}: {p.stderr}"
        assert "--null-bad-trace-id" in p.stderr, args
        assert not (tmp_path / "called").exists(), f"{args} reached docker"
    assert not (tmp_path / "f").exists(), "FILE was made by a run that was refused"


def test_the_nulling_expression_stands_in_for_the_maps():
    # With the option the map's trace_id expression is swapped for one that
    # stores NULL for a text that is not a uuid: the swap must find the map's
    # text, and one pattern must decide refusal, record and NULL alike.
    script = COPY.read_text(encoding="utf-8")
    exprs = {(t, c): e for t, cols, es in copy_map() for c, e in zip(cols, es)}
    m = re.search(r'^NULLIF_TRACE="(.+)"$', script, re.MULTILINE)
    assert m and m.group(1) == exprs[("zhen_actions", "trace_id")]
    assert len(re.findall(r"^UUID_RE=", script, re.MULTILINE)) == 1
    assert script.count("$UUID_RE") >= 2 and "[0-9a-f]{32}" not in script.replace(
        re.search(r"^UUID_RE=.*$", script, re.MULTILINE).group(0), "")


@pytest.fixture(scope="module")
def diff():
    spec = importlib.util.spec_from_file_location("zhen_schema_diff", REPO / "scripts/well/zhen_schema_diff.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def test_fingerprint_diff_passes_on_the_listed_changes_only(diff):
    changes = diff.read_changes(CHANGES)
    assert "constraint|zhen_agent_runs.zhen_agent_runs_card_id_fkey" in changes
    common = {"column|zhen_agent_runs.card_id|text NOT NULL", "index|zhen_agent_runs_card_idx|CREATE INDEX ..."}
    app = set(common) | {f"{k}|old" for k in changes}
    zhen = set(common) | {f"{k}|new" for k in changes if not k.startswith("constraint|")}
    assert diff.compare(app, zhen, changes) == []
    # A difference nobody listed is drift.
    drifted = zhen | {"column|zhen_captures.model|text"}
    assert any(p.startswith("unexpected: > column|zhen_captures.model") for p in diff.compare(app, drifted, changes))
    # A listed change that did not happen is a stale list.
    same = set(common) | {f"{k}|x" for k in changes}
    assert any(p.startswith("listed but the same in both:") for p in diff.compare(same, same, changes))
