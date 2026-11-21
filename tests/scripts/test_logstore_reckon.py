# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/logstore-reckon.py turns a file of log store readings
(scripts/logstore-measure.sh) into the table against the bar (S-LOG-WEEK-2
Phase 1, ADR-147 decision 5). Every file here is made up in a temp directory;
the figures of the first test are the ranges printed in
docs/battle-plans/progress/S-E1-LIVE-progress.md, "Step 284, prepared for
Stevie"."""
import pathlib
import subprocess
import sys

REPO = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts/logstore-reckon.py"

QUERIES = ("q1_last_hour", "q2_text_24h", "q3_units_7d")


def write(tmp_path, readings, extra_lines=()):
    """readings: {time: {metric: value}}; None is written as NA."""
    lines = ["reading_utc\tmetric\tvalue"]
    for when, metrics in readings.items():
        for metric, value in metrics.items():
            lines.append(f"{when}\t{metric}\t{'NA' if value is None else value}")
    lines.extend(extra_lines)
    path = tmp_path / "readings.tsv"
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return path


def reckon(path, *args):
    return subprocess.run(
        [sys.executable, str(SCRIPT), str(path), *args],
        capture_output=True, text=True, timeout=30, check=False,
    )


def rows(stdout):
    """The table as {test: [bar, clickhouse, postgres, ratio, verdict]}."""
    table = {}
    for line in stdout.splitlines():
        if not line.startswith("| ") or line.startswith(("| Test ", "|---")):
            continue
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        table[cells[0]] = cells[1:]
    return table


def reading(ch_row=70.0, pg_row=1460.0, rows_=1_000_000, q=None, probe=0.2, first=None,
            backup=(60.0, 90_000_000), sends=0, dropped=0, day24=None, runs=None, started=None, io=None):
    """One reading. q: {query: (clickhouse p95 ms, postgres p95 ms)}."""
    r = {
        "ch_rows": rows_, "ch_bytes": int(ch_row * rows_),
        "pg_rows": rows_, "pg_bytes": int(pg_row * rows_),
        "well_probe_p50_ms": probe / 2, "well_probe_p95_ms": probe,
        "backup_seconds": backup[0], "backup_bytes": backup[1],
        "vector_record_sends_over_1s_total": sends, "vector_well_dropped_total": dropped,
    }
    for name in QUERIES:
        ch, pg = (q or {}).get(name, (20.0, 30.0))
        r[f"ch_{name}_p95_ms"] = ch
        r[f"pg_{name}_p95_ms"] = pg
    if first is not None:
        r["well_probe_first_p50_ms"] = first / 2
        r["well_probe_first_p95_ms"] = first
    if day24 is not None:
        r["vector_record_sends_over_1s_24h"], r["vector_well_dropped_24h"] = day24
    for name, n in (runs or {}).items():
        r[f"{name}_runs"] = n
    for name, at in (started or {}).items():
        r[name] = at
    if io is not None:
        r["io_some_avg300"] = io
    return r


BASELINE = {
    "ch_rows": 800_000, "ch_bytes": 800_000 * 66,
    "pg_rows": None, "pg_bytes": None,
    "ch_q1_last_hour_p95_ms": 11.0, "pg_q1_last_hour_p95_ms": None,
    "ch_q2_text_24h_p95_ms": 13.0, "pg_q2_text_24h_p95_ms": None,
    "ch_q3_units_7d_p95_ms": 14.0, "pg_q3_units_7d_p95_ms": None,
    "well_probe_p50_ms": 0.1, "well_probe_p95_ms": 0.178,
    "backup_seconds": 1.467, "backup_bytes": 349350,
    "vector_record_sends_over_1s_total": 0, "vector_well_dropped_total": 0,
}

# Week 1's shape: one reading before, six in the week. Each range of the
# prepared table has both its ends here.
WEEK1 = {
    "2026-11-10T18:28:23Z": BASELINE,
    "2026-11-11T22:16:01Z": reading(64, 1464, q={"q1_last_hour": (7, 0.9), "q2_text_24h": (20, 5180), "q3_units_7d": (13, 5083)},
                                    probe=0.197, backup=(57.0, 80_000_000)),
    "2026-11-12T06:10:38Z": reading(65, 1454, q={"q1_last_hour": (30, 0.7), "q2_text_24h": (26, 16100), "q3_units_7d": (16, 29328)},
                                    probe=0.289, backup=(57.0, 80_000_000), sends=1, dropped=263),
    "2026-11-15T01:50:00Z": reading(69, 1462, q={"q1_last_hour": (8, 0.9), "q2_text_24h": (21, 4300), "q3_units_7d": (20, 17982)},
                                    probe=0.214, backup=(60.0, 85_000_000)),
    "2026-11-15T20:50:13Z": reading(70, 1460, q={"q1_last_hour": (7, 0.8), "q2_text_24h": (24, 4818), "q3_units_7d": (22, 31000)},
                                    probe=0.223, backup=(72.0, 90_000_000)),
    "2026-11-16T00:07:49Z": reading(70, 1460, q={"q1_last_hour": (9, 0.9), "q2_text_24h": (33, 5841), "q3_units_7d": (20, None)},
                                    probe=0.204, backup=(72.0, 90_000_000)),
    "2026-11-17T00:13:24Z": reading(72, 1454, q={"q1_last_hour": (26, 0.9), "q2_text_24h": (23, 4644), "q3_units_7d": (23, 25145)},
                                    probe=0.272, backup=(57.0, 102_000_000)),
}
WEEK1_ARGS = ("--baseline-until", "2026-11-10T19", "--start", "2026-11-11T12")


def test_week1_shape_gives_the_bar_rows_and_week_1s_verdicts(tmp_path):
    p = reckon(write(tmp_path, WEEK1), *WEEK1_ARGS)
    assert p.returncode == 0, p.stderr
    t = rows(p.stdout)
    assert p.stdout.count("| Test | Bar | ClickHouse | Postgres copy | Ratio or movement | Verdict |") == 1
    bar = {
        "Disk, bytes a row": ("within 2x", "fails"),
        "q1 last hour, p95": ("within 2x", "passes"),
        "q2 text match over 24 h, p95": ("within 2x", "fails"),
        "q3 count by unit over 7 d, p95": ("within 2x", "fails"),
        "The Well's probe after the queries, p95": ("moves under 10%", "fails"),
        "Backup, seconds": ("moves under 10%", "fails"),
        "Sends over 1 s on the record sink, since Vector's start": ("zero", "fails"),
    }
    for test, (limit, verdict) in bar.items():
        assert test in t, (test, sorted(t))
        assert t[test][0] == limit
        assert t[test][4].startswith(verdict), (test, t[test])
    # The ranges and ratios of the prepared table.
    assert t["Disk, bytes a row"][1:4] == ["64 to 72", "1,454 to 1,464", "20x to 23x"]
    assert t["q1 last hour, p95"][1:3] == ["7.0 to 30 ms", "0.700 to 0.900 ms"]
    assert "Postgres faster" in t["q1 last hour, p95"][4]
    assert t["q2 text match over 24 h, p95"][1:4] == ["20 to 33 ms", "4.3 to 16.1 s", "177x to 619x"]
    assert t["q3 count by unit over 7 d, p95"][1:4] == ["13 to 23 ms", "5.1 to 31.0 s", "391x to 1,833x"]
    assert t["The Well's probe after the queries, p95"][2] == "before 0.178 ms (median of 1); 0.197 to 0.289 ms"
    assert t["The Well's probe after the queries, p95"][3] == "+11% to +62%"
    assert t["Backup, seconds"][2] == "before 1.5 s; 57.0 to 72.0 s"
    assert t["Sends over 1 s on the record sink, since Vector's start"][2] == "0 to 1"
    assert t["Rows dropped at the Well sink, since Vector's start"][2] == "0 to 263"
    assert t["Rows dropped at the Well sink, since Vector's start"][4] == "not a test"
    # No row for what week 1 did not measure.
    assert "The Well's probe before the queries, p95" not in t
    assert not any(name.endswith("over 24 h") for name in t)
    # Decision pack row 12: one failed test is enough.
    assert "Postgres fails 5 of the 5 tests" in p.stdout
    assert "ClickHouse stays" in p.stdout
    assert "stop and ask" not in p.stdout


def test_start_end_and_baseline_until_choose_the_readings(tmp_path):
    days = {f"2026-12-0{d}T00:15:0{d}Z": reading(q={"q2_text_24h": (10, 10 * d)}) for d in range(1, 8)}
    path = write(tmp_path, days)
    everything = reckon(path)
    assert "Readings used: 7 (baseline 0, week 7)" in everything.stdout
    assert rows(everything.stdout)["q2 text match over 24 h, p95"][3] == "1.0x to 7.0x"
    middle = reckon(path, "--start", "2026-12-03", "--end", "2026-12-05T12")
    assert "Readings used: 3 (baseline 0, week 3)" in middle.stdout
    assert rows(middle.stdout)["q2 text match over 24 h, p95"][3] == "3.0x to 5.0x"
    # A full timestamp, as `date -u +%FT%TZ` writes T0 and CLOSED: the reading
    # at that second is inside.
    exact = reckon(path, "--start", "2026-12-03T00:15:03Z", "--end", "2026-12-04T00:15:04Z")
    assert "Readings used: 2 (baseline 0, week 2)" in exact.stdout
    # The readings before --baseline-until are the "before", never the week.
    split = reckon(path, "--baseline-until", "2026-12-03T00:00:00Z", "--start", "2026-12-03T00:00:00Z")
    assert "Readings used: 7 (baseline 2, week 5)" in split.stdout
    assert rows(split.stdout)["q2 text match over 24 h, p95"][3] == "3.0x to 7.0x"
    assert rows(split.stdout)["The Well's probe after the queries, p95"][2].startswith("before 0.200 ms (median of 2)")


def test_a_bad_time_is_refused(tmp_path):
    p = reckon(write(tmp_path, {"2026-12-01T00:15:00Z": reading()}), "--start", "yesterday")
    assert p.returncode == 2
    assert "| Test |" not in p.stdout and "not a UTC time" in p.stderr


def test_na_is_listed_and_left_out_never_read_as_zero(tmp_path):
    p = reckon(write(tmp_path, WEEK1), *WEEK1_ARGS)
    notes = p.stdout.split("Readings with NA:", 1)[1]
    assert "2026-11-16T00:07:49Z pg_q3_units_7d_p95_ms" in notes
    # The baseline's NA is listed too (the table did not exist yet).
    assert "2026-11-10T18:28:23Z pg_rows" in notes
    t = rows(p.stdout)
    # Six readings, one NA: the range and the ratio come from the other five.
    assert t["q3 count by unit over 7 d, p95"][2] == "5.1 to 31.0 s"
    assert "0.00x" not in p.stdout and " 0 to 31" not in p.stdout
    # An NA in the one reading that would decide a row leaves it not judged.
    only = {"2026-12-01T00:15:00Z": reading(q={"q2_text_24h": (10, None)})}
    t = rows(reckon(write(tmp_path, only)).stdout)
    assert t["q2 text match over 24 h, p95"][4].startswith("not judged")


def test_fewer_runs_than_asked_for_are_marked(tmp_path):
    full = {name: 20 for name in ("ch_q3_units_7d", "pg_q3_units_7d", "ch_q2_text_24h", "pg_q2_text_24h")}
    short = dict(full, pg_q3_units_7d=6)
    week = {
        "2026-12-01T00:15:00Z": reading(q={"q3_units_7d": (20, 30_000)}, runs=full),
        "2026-12-02T00:15:00Z": reading(q={"q3_units_7d": (20, 150_000)}, runs=short),
    }
    p = reckon(write(tmp_path, week))
    t = rows(p.stdout)
    assert t["q3 count by unit over 7 d, p95"][2] == "30.0 to 150.0 s (6 runs)"
    assert "runs)" not in t["q3 count by unit over 7 d, p95"][1]
    assert "runs)" not in t["q2 text match over 24 h, p95"][2]
    assert "2026-12-02T00:15:00Z pg_q3_units_7d_runs 6" in p.stdout
    # --runs says what a full reading is.
    assert "(6 runs)" not in reckon(write(tmp_path, week), "--runs", "6").stdout


def test_output_holds_numbers_and_metric_names_only(tmp_path):
    junk = (
        "2026-12-01T00:15:00Z\tCANARY_metric\t5",
        "2026-12-01T00:15:00Z\tpg_rows\tCANARY row text",
        "CANARY when\tpg_rows\t5",
        "2026-12-01T00:15:00Z\tpg_rows\t5\tCANARY fourth field",
        "CANARY one field",
    )
    p = reckon(write(tmp_path, {"2026-12-01T00:15:00Z": reading()}, junk))
    assert p.returncode == 0
    assert "CANARY" not in p.stdout and "CANARY" not in p.stderr
    assert "Lines not understood: 5" in p.stdout
    # The reading's own pg_rows still stands: junk never replaces a number.
    assert rows(p.stdout)["Disk, bytes a row"][2] == "1,460"


def test_a_file_it_cannot_read_exits_2(tmp_path):
    p = reckon(tmp_path / "absent.tsv")
    assert p.returncode == 2
    assert p.stdout == "" and "cannot read the readings file" in p.stderr
    empty = tmp_path / "empty.tsv"
    empty.write_text("reading_utc\tmetric\tvalue\n", encoding="utf-8")
    p = reckon(empty)
    assert p.returncode == 2 and p.stdout == "" and "no reading in the file" in p.stderr


def week2(first_week=0.21, after_week=0.30, pg_row=140.0, q_pg=30.0, backup_week=2.1, day24=(0, 0), sends_total=1):
    """Week 2's shape: three readings with the writer stopped, three in the week."""
    q = {name: (20.0, q_pg) for name in QUERIES}
    files = {}
    for i, hour in enumerate(("00", "06", "12")):
        files[f"2026-12-01T{hour}:15:00Z"] = reading(
            rows_=1, pg_row=0, probe=0.20, first=0.20, backup=(60.0, 120_000_000) if i == 0 else (2.0, 9_000_000),
            sends=sends_total, day24=(sends_total if i == 0 else 0, 4000),
            started={"pg_started_unix": 1795000000, "writer_started_unix": None, "host_boot_unix": 1794000000}, io=1.5 + i)
        files[f"2026-12-01T{hour}:15:00Z"]["pg_rows"] = 0
        files[f"2026-12-01T{hour}:15:00Z"]["pg_bytes"] = 0
    for d in (3, 4, 5):
        files[f"2026-12-0{d}T00:15:00Z"] = reading(
            ch_row=70.0, pg_row=pg_row, q=q, probe=after_week, first=first_week, backup=(backup_week, 9_500_000),
            sends=sends_total, day24=day24,
            started={"pg_started_unix": 1795000000, "writer_started_unix": 1795100000, "host_boot_unix": 1794000000}, io=0.5)
    return files


WEEK2_ARGS = ("--baseline-until", "2026-12-02T00:00:00Z", "--start", "2026-12-02T00:00:00Z")


def test_the_probe_before_the_queries_decides_the_wells_row(tmp_path):
    p = reckon(write(tmp_path, week2()), *WEEK2_ARGS)
    t = rows(p.stdout)
    before = t["The Well's probe before the queries, p95"]
    after = t["The Well's probe after the queries, p95"]
    assert before[2] == "before 0.200 ms (median of 3); 0.210 ms" and before[3] == "+5%" and before[4] == "passes"
    assert after[3] == "+50%" and after[4].startswith("fails")
    summary = p.stdout.split("The five tests of the bar:", 1)[1].split("\n", 1)[0]
    assert "the Well's latency passes (the probe before the queries)" in summary
    # The 24 h count decides the stall row: the cumulative 1 is from before the week.
    assert t["Sends over 1 s on the record sink, over 24 h"][4] == "passes"
    assert t["Sends over 1 s on the record sink, since Vector's start"][4].startswith("fails")
    assert "stalls passes (over 24 h)" in summary
    assert t["Rows dropped at the Well sink, over 24 h"][4] == "not a test"


def test_rows_a_full_buffer_turned_away_are_shown_and_never_judged(tmp_path):
    files = week2(after_week=0.21)
    for i, r in enumerate(files.values()):
        r["vector_well_buffer_dropped_total"] = 4000 + i
        r["vector_well_buffer_dropped_24h"] = 300_000 if i < 3 else 0
    files["2026-12-03T00:15:00Z"]["vector_well_buffer_dropped_24h"] = 250_000   # the day the writer came back
    p = reckon(write(tmp_path, files), *WEEK2_ARGS)
    t = rows(p.stdout)
    assert t["Rows the Well sink's full buffer turned away, since Vector's start"] == ["not in the bar", "", "4,003 to 4,005", "", "not a test"]
    assert t["Rows the Well sink's full buffer turned away, over 24 h"][2] == "0 to 250,000"
    # They change no verdict: the Well sink may drop.
    assert "Postgres passes all 5 tests" in p.stdout
    # A file without them (week 1's) has no such row.
    assert not any("full buffer" in name for name in rows(reckon(write(tmp_path, WEEK1), *WEEK1_ARGS).stdout))


def test_the_backups_before_is_the_last_one_of_the_baseline(tmp_path):
    p = reckon(write(tmp_path, week2()), *WEEK2_ARGS)
    t = rows(p.stdout)
    # The first baseline reading still carries the night before (60 s, with
    # the copy in it); the backup taken inside the baseline is the "before".
    assert t["Backup, seconds"][2] == "before 2.0 s; 2.1 s"
    assert t["Backup, seconds"][3] == "+5%" and t["Backup, seconds"][4] == "passes"
    assert "A backup ran inside the baseline: yes" in p.stdout
    one = dict(list(week2().items())[:1] + list(week2().items())[3:])
    assert "A backup ran inside the baseline: no" in reckon(write(tmp_path, one), *WEEK2_ARGS).stdout


def test_all_five_passed_says_stop_and_ask(tmp_path):
    p = reckon(write(tmp_path, week2(after_week=0.21)), *WEEK2_ARGS)
    assert "Postgres passes all 5 tests" in p.stdout
    assert "stop and ask" in p.stdout
    assert "ClickHouse stays" not in p.stdout
    # One failed test of the five is enough for the other answer.
    for change in ({"pg_row": 150.0}, {"q_pg": 41.0}, {"first_week": 0.23}, {"backup_week": 2.3}, {"day24": (1, 0)}):
        out = reckon(write(tmp_path, week2(after_week=0.21, **change)), *WEEK2_ARGS).stdout
        assert "Postgres fails 1 of the 5 tests" in out, change
        assert "ClickHouse stays" in out and "stop and ask" not in out, change


def test_without_a_baseline_two_tests_are_not_judged_and_nothing_is_decided(tmp_path):
    files = {when: r for when, r in week2(after_week=0.21).items() if when >= "2026-12-03"}
    p = reckon(write(tmp_path, files))
    t = rows(p.stdout)
    assert t["The Well's probe before the queries, p95"][4].startswith("not judged")
    assert t["Backup, seconds"][4].startswith("not judged")
    assert "2 not judged" in p.stdout
    assert "ClickHouse stays" not in p.stdout and "stop and ask" not in p.stdout
    assert "no verdict from this file" in p.stdout


def test_a_baseline_alone_prints_what_step_58_reads(tmp_path):
    files = {when: r for when, r in week2().items() if when < "2026-12-02"}
    p = reckon(write(tmp_path, files))
    assert p.returncode == 0
    assert "Readings used: 3 (baseline 0, week 3)" in p.stdout
    notes = p.stdout.split("Readings with NA:", 1)[1].split("Restarts seen:", 1)[0]
    assert notes.count("writer_started_unix (the writer was not running)") == 3
    assert notes.count("2026-") == 3
    assert "Restarts seen: none" in p.stdout
    # A file with no baseline named is not a week's table, whatever its rows say.
    assert "ClickHouse stays" not in p.stdout and "no verdict from this file" in p.stdout
    assert "The backup's figures changed between 2026-12-01T00:15:00Z and 2026-12-01T06:15:00Z" in p.stdout
    # An empty copy has no bytes a row: not judged, not a division by zero.
    assert rows(p.stdout)["Disk, bytes a row"][4].startswith("not judged")


def test_restarts_and_io_pressure_are_reported(tmp_path):
    files = week2()
    files["2026-12-04T00:15:00Z"]["pg_started_unix"] = 1795300000
    files["2026-12-05T00:15:00Z"]["pg_started_unix"] = 1795300000
    files["2026-12-05T00:15:00Z"]["host_boot_unix"] = 1795400000
    files["2026-12-04T00:15:00Z"]["io_some_avg300"] = 41.5
    p = reckon(write(tmp_path, files), *WEEK2_ARGS)
    seen = p.stdout.split("Restarts seen:", 1)[1].split("Highest io_some_avg300", 1)[0]
    assert "pg_started_unix 1795000000 at 2026-12-03T00:15:00Z, 1795300000 at 2026-12-04T00:15:00Z" in seen
    assert "host_boot_unix 1794000000 at 2026-12-04T00:15:00Z, 1795400000 at 2026-12-05T00:15:00Z" in seen
    assert "writer_started_unix not running at 2026-12-01T12:15:00Z, 1795100000 at 2026-12-03T00:15:00Z" in seen
    assert seen.count("pg_started_unix") == 1
    assert "Highest io_some_avg300: 41.5 at 2026-12-04T00:15:00Z" in p.stdout


def test_the_copys_growth_against_the_writers_ceiling(tmp_path):
    week = {
        "2026-12-01T00:00:00Z": reading(pg_row=1000, rows_=500_000),      # 0.5 GB
        "2026-12-05T00:00:00Z": reading(pg_row=1000, rows_=2_500_000),    # 2.5 GB, four days on
    }
    p = reckon(write(tmp_path, week))
    line = next(x for x in p.stdout.splitlines() if x.startswith("The copy:"))
    assert "2,500,000,000 bytes at the last reading" in line
    assert "500 MB a day" in line
    # (4 GiB - 2.5 GB) / 0.5 GB a day
    assert "3.6 days to the writer's ceiling of 4,294,967,296 bytes" in line
    line = next(x for x in reckon(write(tmp_path, week), "--ceiling-bytes", "2000000000").stdout.splitlines() if x.startswith("The copy:"))
    assert "over the writer's ceiling of 2,000,000,000 bytes" in line


def test_the_bar_is_a_setting(tmp_path):
    files = week2(after_week=0.21, q_pg=50.0)
    assert "Postgres fails 1 of the 5 tests" in reckon(write(tmp_path, files), *WEEK2_ARGS).stdout
    out = reckon(write(tmp_path, files), *WEEK2_ARGS, "--ratio-bar", "3").stdout
    assert "Postgres passes all 5 tests" in out
    assert rows(out)["q2 text match over 24 h, p95"][0] == "within 3x"
