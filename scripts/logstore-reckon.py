#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""logstore-reckon.py: a file of log store readings against the bar.

Reads what scripts/logstore-measure.sh appends (reading_utc, metric, value;
tab-separated) and prints one Markdown table: test, bar, ClickHouse, the
Postgres copy, ratio or movement, verdict (S-LOG-WEEK-2 Phase 1; ADR-117,
ADR-147 decision 5). The bar is the one chosen 2026-11-10 (S-E1-LIVE Step
278, (A)): the same rows within 2x on disk, each query's p95 within 2x, the
Well's own p95 and the backup's time moving under 10%, no send over 1 s on
the record sink.

  scripts/logstore-reckon.py FILE [--baseline-until T] [--start T] [--end T]

  --baseline-until T   the readings before T are the "before" (the Well's
                       probe: their median; the backup: the last of them)
  --start T, --end T   the week is the readings from T to T, both included
  --runs N             what a full reading runs each query (default 20); a
                       p95 from fewer is marked "(n runs)"
  --ratio-bar X        the "within Xx" of the bar (default 2)
  --move-bar P         the "moves under P%" of the bar (default 10)
  --ceiling-bytes B    the writer's --max-bytes (default 4294967296)

T is UTC: 2026-11-21, 2026-11-21T18, 2026-11-21T18:34 or 2026-11-21T18:34:56Z.

Under the table: the five tests of the bar and what the decision pack of
2026-11-18 (row 12) says follows from them, the readings used, every NA by
time and metric, run counts under --runs, restarts seen (a start time that
differs between two readings), the highest I/O pressure, the copy's growth
against the writer's ceiling, and whether a backup ran inside the baseline.

An NA is never read as zero: it is listed and left out of its row. Nothing
from the file is printed but reading times, numbers and the metric names this
script knows; a line it does not understand is counted, not shown.

Exit 0 when it printed a table, 2 on a file it cannot read or that holds no
reading, or on a bad option. Standard library only. Test:
tests/scripts/test_logstore_reckon.py.
"""
import argparse
import datetime
import itertools
import re
import statistics
import sys

QUERIES = (
    ("q1_last_hour", "q1 last hour, p95", "q1"),
    ("q2_text_24h", "q2 text match over 24 h, p95", "q2"),
    ("q3_units_7d", "q3 count by unit over 7 d, p95", "q3"),
)
STARTS = ("ch_started_unix", "pg_started_unix", "vector_started_unix", "writer_started_unix", "host_boot_unix")
KNOWN = frozenset(
    [
        "ch_rows_yesterday", "pg_rows_yesterday", "ch_rows_7d", "pg_rows_7d",
        "ch_bytes", "ch_rows", "pg_bytes", "pg_rows", "pg_bytes_yesterday",
        "well_probe_p50_ms", "well_probe_p95_ms", "well_probe_first_p50_ms", "well_probe_first_p95_ms",
        "well_calls_total", "well_exec_ms_total", "backup_seconds", "backup_bytes",
        "vector_record_buffer_events", "vector_record_buffer_bytes",
        "vector_record_sends_over_1s_total", "vector_well_dropped_total",
        "vector_record_sends_over_1s_24h", "vector_well_dropped_24h",
        "vector_well_buffer_dropped_total", "vector_well_buffer_dropped_24h",
        "io_some_avg300",
    ]
    + list(STARTS)
    + [f"{side}_{q}_{what}" for side in ("ch", "pg") for q, _, _ in QUERIES for what in ("p50_ms", "p95_ms", "runs")]
)
READING = re.compile(r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$")
NUMBER = re.compile(r"^-?\d+(\.\d+)?$")
WHEN = re.compile(r"^(\d{4})-(\d\d)-(\d\d)(?:T(\d\d)(?::(\d\d)(?::(\d\d))?)?)?Z?$")

PASS, FAIL = "passes", "fails"


def when(text):
    """A UTC time from the command line; missing parts are zero."""
    m = WHEN.match(text)
    if not m:
        raise argparse.ArgumentTypeError("not a UTC time like 2026-11-21T18:34:56Z")
    try:
        return datetime.datetime(*(int(g) if g else 0 for g in m.groups()), tzinfo=datetime.UTC)
    except ValueError as e:
        raise argparse.ArgumentTypeError("not a date") from e


def at(reading):
    return datetime.datetime.strptime(reading, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=datetime.UTC)


def load(path):
    """({metric: {reading: value or None}}, lines not understood)."""
    data, odd = {}, 0
    with open(path, encoding="utf-8", errors="replace") as f:
        for line in f:
            fields = line.rstrip("\n").split("\t")
            if fields == ["reading_utc", "metric", "value"] or not line.strip():
                continue
            if len(fields) != 3 or not READING.match(fields[0]) or fields[1] not in KNOWN:
                odd += 1
                continue
            reading, metric, value = fields
            if value == "NA":
                data.setdefault(metric, {}).setdefault(reading, None)
            elif NUMBER.match(value):
                data.setdefault(metric, {})[reading] = float(value)
            else:
                odd += 1
    return data, odd


# --- how numbers are written ----------------------------------------------------
def span(values, one):
    """'a to b' from the least and the greatest, or one value when they read the same."""
    low, high = one(min(values)), one(max(values))
    if low == high:
        return low
    unit = low.rsplit(" ", 1)[-1] if " " in low else ""
    if unit and high.endswith(" " + unit):
        low = low[: -len(unit) - 1]
    return f"{low} to {high}"


def ms(v):
    if v >= 1000:
        return f"{v / 1000:.1f} s"
    if v >= 10:
        return f"{v:.0f} ms"
    if v >= 1:
        return f"{v:.1f} ms"
    return f"{v:.3f} ms"


def times(v):
    if v >= 10:
        return f"{v:,.0f}x"
    if v >= 1:
        return f"{v:.1f}x"
    return f"{v:.2f}x"


def whole(v):
    return f"{v:,.0f}"


def seconds(v):
    return f"{v:.1f} s"


def moved(v):
    return f"{v:+,.0f}%"


class Reckoning:
    def __init__(self, data, baseline, week, opts):
        self.data, self.baseline, self.week, self.opts = data, baseline, week, opts
        self.rows = []
        self.notes = {}

    def get(self, metric, reading):
        return self.data.get(metric, {}).get(reading)

    def has(self, metric):
        return any(r in self.data.get(metric, {}) for r in self.baseline + self.week)

    def series(self, metric, readings):
        return [(r, self.get(metric, r)) for r in readings if self.get(metric, r) is not None]

    def row(self, test, bar, clickhouse, postgres, ratio, verdict):
        self.rows.append((test, bar, clickhouse, postgres, ratio, verdict))
        return verdict

    # --- the rows ---------------------------------------------------------------
    def disk(self):
        ch, pg, ratios = [], [], []
        for r in self.week:
            cb, cr, pb, pr = (self.get(m, r) for m in ("ch_bytes", "ch_rows", "pg_bytes", "pg_rows"))
            if None in (cb, cr, pb, pr) or cr <= 0 or pr <= 0:
                continue
            ch.append(cb / cr)
            pg.append(pb / pr)
            ratios.append((pb / pr) / (cb / cr))
        bar = f"within {self.opts.ratio_bar:g}x"
        if not ratios:
            return self.row("Disk, bytes a row", bar, "", "", "", "not judged: no reading with rows on both sides")
        verdict = self.ratio_verdict(ratios, "smaller")
        return self.row("Disk, bytes a row", bar, span(ch, whole), span(pg, whole), span(ratios, times), verdict)

    def ratio_verdict(self, ratios, better):
        if max(ratios) > self.opts.ratio_bar:
            return FAIL
        return f"{PASS} (Postgres {better})" if max(ratios) < 1 else PASS

    def runs_mark(self, side, query, readings):
        counts = [(r, self.get(f"{side}_{query}_runs", r)) for r in readings]
        few = [(r, n) for r, n in counts if n is not None and n < self.opts.runs]
        for r, n in few:
            self.notes.setdefault("runs", []).append(f"{r} {side}_{query}_runs {n:.0f}")
        return f" ({min(n for _, n in few):.0f} runs)" if few else ""

    def query(self, key, label):
        bar = f"within {self.opts.ratio_bar:g}x"
        ch = dict(self.series(f"ch_{key}_p95_ms", self.week))
        pg = dict(self.series(f"pg_{key}_p95_ms", self.week))
        ch_cell = span(list(ch.values()), ms) + self.runs_mark("ch", key, list(ch)) if ch else ""
        pg_cell = span(list(pg.values()), ms) + self.runs_mark("pg", key, list(pg)) if pg else ""
        ratios = [pg[r] / ch[r] for r in self.week if r in ch and r in pg and ch[r] > 0]
        if not ratios:
            return self.row(label, bar, ch_cell, pg_cell, "", "not judged: no reading with a p95 on both sides")
        return self.row(label, bar, ch_cell, pg_cell, span(ratios, times), self.ratio_verdict(ratios, "faster"))

    def movement(self, label, metric, before, how, one, judged=True):
        """A row for a figure of the Well alone, against its value before the week."""
        bar = f"moves under {self.opts.move_bar:g}%" if judged else "shown with the seconds"
        week = [v for _, v in self.series(metric, self.week)]
        if not week:
            cell = f"before {one(before)}{how}" if before is not None else ""
            return self.row(label, bar, "", cell, "", "not judged: no reading in the week" if judged else "not a test")
        if before is None or before <= 0:
            return self.row(label, bar, "", span(week, one), "", "not judged: no baseline reading" if judged else "not a test")
        moves = [(v / before - 1) * 100 for v in week]
        verdict = "not a test"
        if judged:
            verdict = PASS if max(moves) < self.opts.move_bar else FAIL
        return self.row(label, bar, "", f"before {one(before)}{how}; {span(week, one)}", span(moves, moved), verdict)

    def probe(self, metric, label):
        before = [v for _, v in self.series(metric, self.baseline)]
        base = statistics.median(before) if before else None
        return self.movement(label, metric, base, f" (median of {len(before)})", ms)

    def backup(self):
        seconds_before = self.series("backup_seconds", self.baseline)
        bytes_before = self.series("backup_bytes", self.baseline)
        verdict = self.movement("Backup, seconds", "backup_seconds", seconds_before[-1][1] if seconds_before else None, "", seconds)
        self.movement("Backup, bytes", "backup_bytes", bytes_before[-1][1] if bytes_before else None, "", whole, judged=False)
        return verdict

    def count(self, label, metric, bar):
        week = [v for _, v in self.series(metric, self.week)]
        if bar is None:
            return self.row(label, "not in the bar", "", span(week, whole) if week else "", "", "not a test")
        if not week:
            return self.row(label, bar, "", "", "", "not judged: no reading in the week")
        return self.row(label, bar, "", span(week, whole), "", PASS if max(week) == 0 else FAIL)

    # --- the whole ---------------------------------------------------------------
    def reckon(self):
        five = {}
        five["disk"] = (self.disk(), "")
        verdicts = [(short, self.query(key, label)) for key, label, short in QUERIES]
        failed = [short for short, v in verdicts if v.startswith(FAIL)]
        if failed:
            five["query p95"] = (FAIL, ", ".join(failed))
        elif all(v.startswith(PASS) for _, v in verdicts):
            five["query p95"] = (PASS, "")
        else:
            five["query p95"] = ("not judged", "")

        # The probe before the queries is the fair one (the one after them
        # follows minutes of scans, which the baseline's does not): it decides
        # when the file has it. Both are shown.
        first = self.probe("well_probe_first_p95_ms", "The Well's probe before the queries, p95") if self.has("well_probe_first_p95_ms") else None
        after = self.probe("well_probe_p95_ms", "The Well's probe after the queries, p95")
        if first is not None:
            five["the Well's latency"] = (first, "the probe before the queries")
        else:
            five["the Well's latency"] = (after, "the probe after the queries: this file has no probe before them")

        five["backup"] = (self.backup(), "")

        total = self.count("Sends over 1 s on the record sink, since Vector's start", "vector_record_sends_over_1s_total", "zero")
        if self.has("vector_record_sends_over_1s_24h"):
            day = self.count("Sends over 1 s on the record sink, over 24 h", "vector_record_sends_over_1s_24h", "zero")
            five["stalls"] = (day, "over 24 h")
        else:
            five["stalls"] = (total, "since Vector's start: this file has no 24 h count")
        # Not in the bar: the Well sink may drop. Two counts: rows the sink
        # gave up on, and rows its full buffer turned away.
        self.count("Rows dropped at the Well sink, since Vector's start", "vector_well_dropped_total", None)
        for metric, label in (
            ("vector_well_dropped_24h", "Rows dropped at the Well sink, over 24 h"),
            ("vector_well_buffer_dropped_total", "Rows the Well sink's full buffer turned away, since Vector's start"),
            ("vector_well_buffer_dropped_24h", "Rows the Well sink's full buffer turned away, over 24 h"),
        ):
            if self.has(metric):
                self.count(label, metric, None)
        return five

    def lines(self):
        o = self.opts
        five = self.reckon()
        out = ["| Test | Bar | ClickHouse | Postgres copy | Ratio or movement | Verdict |", "|---|---|---|---|---|---|"]
        out += ["| " + " | ".join(r) + " |" for r in self.rows]
        out.append("")

        said = []
        for name, (verdict, why) in five.items():
            word = verdict.split(":", 1)[0].split(" (", 1)[0]
            said.append(f"{name} {word}" + (f" ({why})" if why else ""))
        out.append("The five tests of the bar: " + "; ".join(said) + ".")
        fails = sum(1 for v, _ in five.values() if v.startswith(FAIL))
        passes = sum(1 for v, _ in five.values() if v.startswith(PASS))
        open_ = len(five) - fails - passes
        if passes == len(five):
            out.append(f"Postgres passes all {len(five)} tests.")
        else:
            out.append(f"Postgres fails {fails} of the {len(five)} tests, passes {passes}; {open_} not judged.")
        # What follows is Stevie's ruling, given ahead (DECISION-PACK-2026-11-18
        # row 12, for S-LOG-WEEK-2 Step 87). It is his, not this script's.
        ruling = "Step 87, answered ahead (DECISION-PACK-2026-11-18 row 12): "
        if not self.baseline:
            out.append(ruling + "no reading was named as the baseline (--baseline-until), so this is not a week's table: no verdict from this file.")
        elif fails:
            out.append(ruling + "Postgres fails the bar on at least one test: ClickHouse stays and the plan closes.")
        elif open_:
            out.append(ruling + f"{open_} of the five tests could not be judged and none failed: no verdict from this file.")
        else:
            out.append(ruling + "Postgres passes all five: stop and ask Stevie.")
        out.append("")

        used = self.baseline + self.week
        out.append(f"Readings used: {len(used)} (baseline {len(self.baseline)}, week {len(self.week)}), {used[0]} to {used[-1]}.")
        na = []
        for r in used:
            for metric in sorted(m for m in self.data if r in self.data[m] and self.data[m][r] is None):
                na.append(f"- {r} {metric}" + (" (the writer was not running)" if metric == "writer_started_unix" else ""))
        out.append(f"Readings with NA: {len({x.split()[1] for x in na})} ({len(na)} values)")
        out += na
        few = self.notes.get("runs", [])
        out.append(f"Fewer than {o.runs} runs: {len(few) if few else 'none'}")
        out += [f"- {x}" for x in sorted(set(few))]

        seen = []
        for metric in STARTS:
            last = None
            for r in used:
                if r not in self.data.get(metric, {}):
                    continue
                v = self.data[metric][r]
                if last is not None and v != last[1]:
                    then = "not running" if last[1] is None else f"{last[1]:.0f}"
                    if v is None:
                        seen.append(f"- {metric} {then} at {last[0]}, not running at {r}")
                    else:
                        new = datetime.datetime.fromtimestamp(v, datetime.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
                        seen.append(f"- {metric} {then} at {last[0]}, {v:.0f} at {r} (the new start: {new})")
                last = (r, v)
        if seen:
            out.append(f"Restarts seen: {len(seen)}")
            out += seen
        elif any(self.has(m) for m in STARTS):
            out.append("Restarts seen: none")
        else:
            out.append("Restarts seen: none (this file holds no start times)")

        io = self.series("io_some_avg300", used)
        if io:
            r, v = max(io, key=lambda x: x[1])
            out.append(f"Highest io_some_avg300: {v:g} at {r}")

        size = self.series("pg_bytes", self.week)
        if size:
            line = f"The copy: {size[-1][1]:,.0f} bytes at the last reading"
            days = (at(size[-1][0]) - at(size[0][0])).total_seconds() / 86400
            rate = (size[-1][1] - size[0][1]) / days if days > 0 else 0
            if days > 0:
                line += f"; {rate / 1e6:,.0f} MB a day over {days:.1f} days"
            if size[-1][1] >= o.ceiling_bytes:
                line += f"; over the writer's ceiling of {o.ceiling_bytes:,} bytes (--ceiling-bytes)"
            elif rate > 0:
                line += f"; {(o.ceiling_bytes - size[-1][1]) / rate:.1f} days to the writer's ceiling of {o.ceiling_bytes:,} bytes (--ceiling-bytes)"
            else:
                line += f"; the writer's ceiling is {o.ceiling_bytes:,} bytes (--ceiling-bytes)"
            out.append(line + ".")

        backups = self.series("backup_seconds", used)
        changes = [(a[0], b[0]) for a, b in itertools.pairwise(backups) if a[1] != b[1]]
        if changes:
            out.append("The backup's figures changed between " + "; ".join(f"{a} and {b}" for a, b in changes) + ".")
        if self.baseline:
            inside = [c for c in changes if c[1] in self.baseline]
            out.append("A backup ran inside the baseline: " + ("yes" if inside else "no (its figure is the same in every baseline reading: it may be older than the baseline)") + ".")
        out.append(f"Lines not understood: {o.odd}")
        return out


def main(argv):
    p = argparse.ArgumentParser(description="A file of log store readings (scripts/logstore-measure.sh) against the bar.")
    p.add_argument("file")
    p.add_argument("--baseline-until", type=when, metavar="T", help='the readings before T are the "before"')
    p.add_argument("--start", type=when, metavar="T", help="the week starts at T (included)")
    p.add_argument("--end", type=when, metavar="T", help="the week ends at T (included)")
    p.add_argument("--runs", type=int, default=20, metavar="N", help="what a full reading runs each query (default 20)")
    p.add_argument("--ratio-bar", type=float, default=2.0, metavar="X", help="the bar's 'within Xx' (default 2)")
    p.add_argument("--move-bar", type=float, default=10.0, metavar="P", help="the bar's 'moves under P%%' (default 10)")
    p.add_argument("--ceiling-bytes", type=int, default=4 << 30, metavar="B", help="the writer's --max-bytes (default 4294967296)")
    opts = p.parse_args(argv)
    if opts.runs < 1 or opts.ratio_bar <= 0 or opts.move_bar <= 0 or opts.ceiling_bytes < 1:
        p.error("--runs, --ratio-bar, --move-bar and --ceiling-bytes must be above zero")

    try:
        data, opts.odd = load(opts.file)
    except OSError as e:
        print(f"logstore-reckon: cannot read the readings file: {e.strerror}", file=sys.stderr)
        return 2
    readings = sorted({r for by_reading in data.values() for r in by_reading})
    baseline = [r for r in readings if opts.baseline_until and at(r) < opts.baseline_until]
    week = [
        r for r in readings
        if r not in baseline and (not opts.start or at(r) >= opts.start) and (not opts.end or at(r) <= opts.end)
    ]
    if not baseline and not week:
        print("logstore-reckon: no reading in the file for those times", file=sys.stderr)
        return 2
    print("\n".join(Reckoning(data, baseline, week, opts).lines()))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
