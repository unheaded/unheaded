#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Routing thresholds from the two-brains eval (eval/two-brains/PREREG.md).

Generate from the tune half:

    python3 scripts/two-brains/thresholds.py --out configs/zhen-routing.yaml \
        --table eval/two-brains/RESULTS-2026-10-23.md eval/two-brains/results-*.jsonl

Check the generated routes on the hold-out half and adopt or reject each
family (rewrites the config):

    python3 scripts/two-brains/thresholds.py --verify-holdout configs/zhen-routing.yaml eval/two-brains/results-*.jsonl

The rule is PREREG.md's and is not tuned here: change PREREG.md (with a
new commit, before new results) to change it.
"""
import argparse
import glob
import hashlib
import json
import math
import os
import statistics
import sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SPLIT = os.path.join(ROOT, "eval", "two-brains", "split.jsonl")
PREREG = os.path.join(ROOT, "eval", "two-brains", "PREREG.md")

POOL = {"syntax": "textbook", "review": "textbook"}  # PREREG: pooled for the decision
ESCALATE_ON = {
    "repo-task": ["check_fail", "parse_fail"],
}
DEFAULT_ESCALATE = ["judge_fail", "parse_fail", "refusal"]


def decide(local_rate, claude_rate):
    """PREREG rule. claude_rate None = no Claude data for the family."""
    if local_rate >= 0.85 and (claude_rate is None or claude_rate - local_rate < 0.10 - 1e-12):
        return "local_only"
    if local_rate >= 0.30 - 1e-12:
        return "local_first"
    return "claude_direct"


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        h.update(f.read())
    return h.hexdigest()


def load_results(paths):
    rows = []
    for pattern in paths:
        for p in sorted(glob.glob(pattern)) or [pattern]:
            with open(p, encoding="utf-8") as f:
                for n, line in enumerate(f, 1):
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        r = json.loads(line)
                    except json.JSONDecodeError as e:
                        sys.exit("%s:%d: %s" % (p, n, e))  # strict: never drop a line
                    r["_file"] = os.path.relpath(p, ROOT)
                    rows.append(r)
    return rows


def load_split(path=SPLIT):
    out = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            t = json.loads(line)
            out[t["id"]] = t
    return out


def config_key(r):
    return (r["backend"], r["model"])


def latest_per_task(rows):
    """One result per (backend, model, task): the last one written."""
    out = {}
    for r in rows:
        out[(r["backend"], r["model"], r["task"])] = r
    return out


def group(rows, split, half):
    """{(backend, model): {decision_family: {task: row}}} for one half."""
    g = {}
    for (b, m, task), r in latest_per_task(rows).items():
        t = split.get(task)
        if t is None or (half != "all" and t["split"] != half):
            continue
        fam = POOL.get(t["family"], t["family"])
        g.setdefault((b, m), {}).setdefault(fam, {})[task] = r
    return g


def rate(rows):
    return sum(1 for r in rows.values() if r.get("pass")) / len(rows) if rows else 0.0


def best_local(g):
    locals_ = [k for k in g if k[0] == "local"]
    if not locals_:
        return None

    def score(k):
        all_rows = {t: r for fam in g[k].values() for t, r in fam.items()}
        lat = statistics.mean([r.get("latency_ms", 0) for r in all_rows.values()]) if all_rows else 0
        return (-rate(all_rows), lat)
    return sorted(locals_, key=score)[0]


def claude_config(g):
    cs = [k for k in g if k[0] == "claude"]
    return sorted(cs)[0] if cs else None


def pct(xs, q):
    if not xs:
        return 0
    xs = sorted(xs)
    i = min(len(xs) - 1, max(0, math.ceil(q * len(xs)) - 1))
    return xs[i]


def family_stats(rows):
    lat = [ms for r in rows.values() for ms in (r.get("attempt_ms") or [r.get("latency_ms", 0)])]
    cost = sum(r.get("cost_nano_usd", 0) for r in rows.values())
    return {
        "n": len(rows),
        "pass": sum(1 for r in rows.values() if r.get("pass")),
        "rate": rate(rows),
        "usd_per_task": (cost / 1e9 / len(rows)) if rows else 0.0,
        "p50_ms": pct(lat, 0.50),
        "p95_ms": pct(lat, 0.95),
    }


def max_claude_usd(rows):
    costs = [r.get("cost_nano_usd", 0) / 1e9 for r in rows.values()]
    top = max(costs) if costs else 0.0
    return max(0.05, math.ceil(top * 2 * 100) / 100)


def generate(rows, split):
    tune = group(rows, split, "tune")
    loc, cla = best_local(tune), claude_config(tune)
    if loc is None:
        sys.exit("no local results")
    fams = sorted(set(tune.get(loc, {})) | set(tune.get(cla, {}) if cla else set()))
    families, table = {}, {}
    for fam in fams:
        lrows = tune.get(loc, {}).get(fam, {})
        crows = tune.get(cla, {}).get(fam, {}) if cla else {}
        L = rate(lrows)
        C = rate(crows) if crows else None
        route = decide(L, C)
        entry = {
            "route": route,
            "local_model": loc[1],
            "local_attempts": 2 if fam == "repo-task" else 1,
            "escalate_on": ESCALATE_ON.get(fam, DEFAULT_ESCALATE) if (route == "local_first" and cla) else [],
            "max_claude_usd": max_claude_usd(crows) if crows else 0.25,
            "tune": {"local_rate": round(L, 3), "claude_rate": None if C is None else round(C, 3),
                     "n": len(lrows)},
        }
        if cla is None and route != "local_only":
            entry["route"] = "local_first"
            entry["escalate_on"] = []  # PREREG: no Claude data -> escalation off
        families[fam] = entry
        table[fam] = {"local": family_stats(lrows), "claude": family_stats(crows) if crows else None}
    return families, table, loc, cla


def simulate(route, lr, cr):
    """Routed outcome on one task: (pass, cost_nano)."""
    if route == "local_only":
        return bool(lr and lr.get("pass")), 0
    if route == "claude_direct":
        return bool(cr and cr.get("pass")), (cr or {}).get("cost_nano_usd", 0)
    # local_first
    if lr and lr.get("pass"):
        return True, 0
    return bool(cr and cr.get("pass")), (cr or {}).get("cost_nano_usd", 0)


def verify(families, rows, split, loc, cla):
    hold = group(rows, split, "holdout")
    report = {}
    for fam, entry in families.items():
        lrows = hold.get(loc, {}).get(fam, {})
        crows = hold.get(cla, {}).get(fam, {}) if cla else {}
        tasks = sorted(set(lrows) | set(crows))
        n = len(tasks)
        if n == 0:
            report[fam] = {"n": 0, "adopted": False, "why": "no hold-out tasks"}
            continue
        routed_pass = routed_cost = cl_pass = cl_cost = lo_pass = 0
        for t in tasks:
            p, c = simulate(entry["route"], lrows.get(t), crows.get(t))
            routed_pass += p
            routed_cost += c
            cr = crows.get(t)
            cl_pass += bool(cr and cr.get("pass"))
            cl_cost += (cr or {}).get("cost_nano_usd", 0)
            lo_pass += bool(lrows.get(t) and lrows[t].get("pass"))
        checks = []
        if cla:
            checks.append(("routed >= claude - 5pp", routed_pass / n >= cl_pass / n - 0.05 - 1e-12))
            checks.append(("routed cost <= 50% claude", routed_cost <= 0.5 * cl_cost + 1e-9))
        checks.append(("routed >= local-only", routed_pass >= lo_pass))
        adopted = all(ok for _, ok in checks)
        report[fam] = {
            "n": n, "routed_pass": routed_pass, "claude_pass": cl_pass, "local_pass": lo_pass,
            "routed_usd": routed_cost / 1e9, "claude_usd": cl_cost / 1e9,
            "checks": {name: ok for name, ok in checks}, "adopted": adopted,
        }
    return report


def yaml_scalar(v):
    if v is None:
        return "null"
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return repr(v)
    return json.dumps(v)


def write_config(path, families, loc, cla, sources, report=None):
    lines = [
        "# SPDX-License-Identifier: GPL-3.0-or-later",
        "# Generated by scripts/two-brains/thresholds.py from the tune half of",
        "# " + ", ".join(sources) + ".",
        "# Do not hand-edit; re-run the eval. Rule: eval/two-brains/PREREG.md.",
        "version: 1",
        "source:",
        "  results: [" + ", ".join(json.dumps(s) for s in sources) + "]",
        "  split_sha256: " + json.dumps(sha256_file(SPLIT)),
        "  prereg_sha256: " + json.dumps(sha256_file(PREREG)),
        "  local: " + json.dumps("%s/%s" % loc),
        "  claude: " + (json.dumps("%s/%s" % cla) if cla else "null"),
        "families:",
    ]
    out_fams = {}
    for fam, e in families.items():
        e = dict(e)
        if report is not None:
            r = report.get(fam, {})
            e["holdout"] = {k: r[k] for k in ("n", "adopted") if k in r}
            if not r.get("adopted"):
                e["route_tuned"] = e["route"]
                if cla:
                    e["route"], e["escalate_on"] = "claude_direct", []
                else:
                    e["route"], e["escalate_on"] = "local_first", []
        out_fams[fam] = e
    # The pooled textbook decision applies to both of its families.
    expanded = {}
    for fam, e in out_fams.items():
        if fam == "textbook":
            for member in ("syntax", "review"):
                expanded[member] = dict(e, pooled_as="textbook")
        else:
            expanded[fam] = e
    for fam in sorted(expanded):
        e = expanded[fam]
        lines.append("  %s:" % fam)
        for k in ("route", "local_model", "local_attempts", "escalate_on", "max_claude_usd", "pooled_as", "route_tuned"):
            if k in e:
                v = e[k]
                lines.append("    %s: %s" % (k, "[" + ", ".join(v) + "]" if isinstance(v, list) else yaml_scalar(v)))
        for sub in ("tune", "holdout"):
            if sub in e:
                lines.append("    %s: {%s}" % (sub, ", ".join("%s: %s" % (k, yaml_scalar(v)) for k, v in e[sub].items())))
    lines.append("default: {route: claude_direct, max_claude_usd: 0.25}")
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


def write_table(path, table, loc, cla, report=None):
    out = ["<!-- SPDX-License-Identifier: GPL-3.0-or-later -->",
           "# Two-brains eval results (tune half)", "",
           "Local: `%s/%s`. Claude: `%s`." % (loc[0], loc[1], "%s/%s" % cla if cla else "none"), "",
           "| family | backend | n | pass | rate | $/task | p50 ms | p95 ms |", "|---|---|---|---|---|---|---|---|"]
    for fam in sorted(table):
        for b in ("local", "claude"):
            s = table[fam][b]
            if s is None:
                continue
            out.append("| %s | %s | %d | %d | %.2f | %.4f | %d | %d |" % (fam, b, s["n"], s["pass"], s["rate"], s["usd_per_task"], s["p50_ms"], s["p95_ms"]))
    if report:
        out += ["", "## Hold-out", "", "| family | n | routed pass | Claude pass | local pass | routed $ | Claude $ | adopted |", "|---|---|---|---|---|---|---|---|"]
        for fam in sorted(report):
            r = report[fam]
            if r.get("n", 0) == 0:
                out.append("| %s | 0 | | | | | | no (no tasks) |" % fam)
                continue
            out.append("| %s | %d | %d | %d | %d | %.4f | %.4f | %s |" % (fam, r["n"], r["routed_pass"], r["claude_pass"], r["local_pass"], r["routed_usd"], r["claude_usd"], "yes" if r["adopted"] else "no"))
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(out) + "\n")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("results", nargs="+")
    ap.add_argument("--out", default=os.path.join(ROOT, "configs", "zhen-routing.yaml"))
    ap.add_argument("--table")
    ap.add_argument("--verify-holdout", metavar="CONFIG")
    a = ap.parse_args(argv)
    rows = load_results(a.results)
    split = load_split()
    families, table, loc, cla = generate(rows, split)
    sources = sorted({r["_file"] for r in rows})
    if a.verify_holdout:
        report = verify(families, rows, split, loc, cla)
        write_config(a.verify_holdout, families, loc, cla, sources, report)
        if a.table:
            write_table(a.table, table, loc, cla, report)
        print(json.dumps(report, indent=2, sort_keys=True))
        return 0
    write_config(a.out, families, loc, cla, sources)
    if a.table:
        write_table(a.table, table, loc, cla)
    for fam in sorted(families):
        e = families[fam]
        print("%-10s %-14s L=%.2f C=%s" % (fam, e["route"], e["tune"]["local_rate"], e["tune"]["claude_rate"]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
