# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/well/well_lint.py reads the database each ledger file runs in from
the MANIFEST beside it (ADR-101): one table name in two databases is two
tables, not drift (ADR-149 moves Zhen's tables from unheaded_app to
unheaded_zhen); in one database, or where the database is unknown, a second
shape is still drift."""
import json
import pathlib
import subprocess
import sys

REPO = pathlib.Path(__file__).resolve().parents[2]
LINT = REPO / "scripts/well/well_lint.py"


def drift(tmp_path, manifest):
    d = tmp_path / "db" / "migrations"
    d.mkdir(parents=True)
    (d / "001_a.sql").write_text("CREATE TABLE t (id bigint PRIMARY KEY, x text NOT NULL);\n")
    (d / "002_b.sql").write_text("CREATE TABLE t (id bigint PRIMARY KEY, x integer NOT NULL);\n")
    if manifest is not None:
        (d / "MANIFEST").write_text(manifest)
    out = subprocess.run([sys.executable, "-I", str(LINT), "--format", "json", "--fail-on", "NONE",
                          str(d / "001_a.sql"), str(d / "002_b.sql")],
                         check=True, capture_output=True, text=True).stdout
    return [f for f in json.loads(out)["findings"] if f["rule"] == "shape-drift"]


def test_same_name_in_two_databases_is_not_drift(tmp_path):
    assert drift(tmp_path, "# comment\nalpha  001_a.sql\nbeta   002_b.sql   notx\n") == []


def test_two_shapes_in_one_database_are_drift(tmp_path):
    assert len(drift(tmp_path, "alpha 001_a.sql\nalpha 002_b.sql\n")) == 1


def test_unknown_database_is_still_drift(tmp_path):
    assert len(drift(tmp_path, None)) == 1
    # skip lines and a file applied to two databases give no database either.


def test_file_in_two_databases_has_no_single_database(tmp_path):
    assert len(drift(tmp_path, "alpha 001_a.sql\nbeta 001_a.sql\nbeta 002_b.sql\n")) == 1
