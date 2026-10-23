#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Vendored 2026-10-15 from the unheaded-erda skill (scripts/well_lint.py), ruff-fixed
# to this repo's rules; run by scripts/check-well-lint.sh (ERDA 3.5).
"""
well_lint.py — Erda's static linter for PostgreSQL schema and migrations.

Scans .sql files and SQL embedded in Go/Python source for:
  * ledger hygiene     — DDL outside the migration ledger, psql meta-commands, secrets,
                         file naming/order, provisioning mixed into schema migrations
  * shape drift        — the same table defined more than once with different columns
  * lock hazards       — DDL that blocks or rewrites live tables (CREATE INDEX without
                         CONCURRENTLY, volatile defaults, type changes, constraints without
                         NOT VALID, SET NOT NULL, renames, drops, TRUNCATE, VACUUM FULL ...)
  * schema smells      — missing PK, SERIAL, timestamp without zone, json, nullable + DEFAULT '',
                         arrays of references, open vocabularies in CHECK lists, text IPs ...
  * index smells       — duplicate/redundant indexes, low-cardinality single-column indexes,
                         indexes on columns defaulting to '', unindexed foreign keys
  * access smells      — UPDATE/DELETE granted on append-only tables, grants to PUBLIC,
                         no default privileges
  * dialect errors     — MySQL syntax in PostgreSQL DDL

Standard library only. Heuristic by design: it reads text, not a live catalog — confirm
findings against the database (scripts/well_health.sql) before acting.

Usage:
  python3 well_lint.py PATH [PATH ...] [--format text|md|json] [--min-severity P3]
                        [--fail-on P1|P0|NONE] [--inventory] [--include-tests] [--no-code]
Severity: P0 (data loss / outage / exposure) · P1 (fix before merge) · P2 (fix soon) · P3 (hygiene) · INFO
Exit status: 2 if any P0, 1 if any P1, else 0 (same scale as the Well Report). --fail-on raises or lowers
the bar; --fail-on NONE always exits 0.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path

SEVERITIES = ["INFO", "P3", "P2", "P1", "P0"]
SEV_RANK = {s: i for i, s in enumerate(SEVERITIES)}

SKIP_DIRS = {".git", "node_modules", "vendor", "dist", "build", "target", "__pycache__",
             ".venv", "venv", "sbom-results", "tomb"}
CODE_EXT = {".go", ".py"}

DDL_RE = re.compile(r"\b(CREATE\s+(?:UNIQUE\s+)?INDEX|CREATE\s+TABLE|ALTER\s+TABLE|DROP\s+TABLE|DROP\s+INDEX)\b",
                    re.IGNORECASE)

REF = {
    "ledger": "references/migrations.md §1-2",
    "locks": "references/migrations.md §4",
    "expand": "references/migrations.md §5",
    "bootstrap": "references/migrations.md §7-8",
    "keys": "references/schema-design.md §3",
    "types": "references/schema-design.md §4",
    "nulls": "references/schema-design.md §5",
    "constraints": "references/schema-design.md §6",
    "vocab": "references/schema-design.md §7",
    "appendonly": "references/access-and-security.md §4",
    "grants": "references/access-and-security.md §2",
    "secrets": "references/access-and-security.md §5",
    "index": "references/indexing-and-query-tuning.md §10",
    "placement": "references/data-placement.md",
}


@dataclass
class Finding:
    severity: str
    rule: str
    file: str
    line: int
    message: str
    ref: str = ""


@dataclass
class Statement:
    text: str              # original text (comments removed)
    norm: str              # whitespace-collapsed
    file: str
    line: int
    origin: str            # "sql" | "code"
    engine: str            # "postgres" | "sqlite" | "mysql?"
    db: str | None = None  # database from \c / \connect when known


@dataclass
class TableDef:
    name: str
    file: str
    line: int
    origin: str
    engine: str
    db: str | None
    columns: dict = field(default_factory=dict)   # name -> {"type":..., "notnull":bool, "default":str|None, "pk":bool}
    has_pk: bool = False
    if_not_exists: bool = False


# ----------------------------------------------------------------------------- tokenizing

def split_sql(src: str, base_line: int = 1):
    """Split SQL text into statements. Handles -- and /* */ comments (nested), quoted strings,
    E'' strings, dollar quotes and quoted identifiers. psql meta-command lines are returned
    separately. Yields (kind, text, line) where kind is 'stmt' or 'meta'."""
    out = []
    i, n = 0, len(src)
    buf = []
    line = base_line
    stmt_line = None
    at_line_start = True

    def flush():
        nonlocal buf, stmt_line
        text = "".join(buf).strip()
        if text:
            out.append(("stmt", text, stmt_line or line))
        buf = []
        stmt_line = None

    while i < n:
        c = src[i]
        # psql meta-command: backslash at the start of a line (outside any literal)
        if at_line_start and c == "\\":
            j = src.find("\n", i)
            j = n if j == -1 else j
            out.append(("meta", src[i:j].strip(), line))
            i = j
            continue
        if c == "\n":
            line += 1
            at_line_start = True
            buf.append(c)
            i += 1
            continue
        if c in " \t\r":
            buf.append(c)
            i += 1
            continue
        at_line_start = False
        if stmt_line is None and not src.startswith("--", i) and not src.startswith("/*", i):
            stmt_line = line
        if src.startswith("--", i):
            j = src.find("\n", i)
            j = n if j == -1 else j
            i = j
            continue
        if src.startswith("/*", i):
            depth, j = 1, i + 2
            while j < n and depth:
                if src.startswith("/*", j):
                    depth += 1; j += 2
                elif src.startswith("*/", j):
                    depth -= 1; j += 2
                else:
                    if src[j] == "\n":
                        line += 1
                    j += 1
            buf.append(" ")
            i = j
            continue
        if c == "'" or ((c in "eE") and i + 1 < n and src[i + 1] == "'"):
            start = i
            if c != "'":
                i += 1
            i += 1
            escape_mode = c in "eE"
            while i < n:
                if escape_mode and src[i] == "\\":
                    i += 2
                    continue
                if src[i] == "'":
                    if i + 1 < n and src[i + 1] == "'":
                        i += 2
                        continue
                    i += 1
                    break
                if src[i] == "\n":
                    line += 1
                i += 1
            buf.append(src[start:i])
            continue
        if c == '"':
            j = src.find('"', i + 1)
            j = n - 1 if j == -1 else j
            buf.append(src[i:j + 1])
            line += src[i:j + 1].count("\n")
            i = j + 1
            continue
        if c == "$":
            m = re.match(r"\$([A-Za-z_][A-Za-z0-9_]*)?\$", src[i:])
            if m:
                tag = m.group(0)
                j = src.find(tag, i + len(tag))
                j = n if j == -1 else j + len(tag)
                chunk = src[i:j]
                buf.append(chunk)
                line += chunk.count("\n")
                i = j
                continue
        if c == ";":
            buf.append(";")
            flush()
            i += 1
            continue
        buf.append(c)
        i += 1
    flush()
    return out


def norm_ws(s: str) -> str:
    return re.sub(r"\s+", " ", s).strip()


def strip_literals(s: str) -> str:
    """Replace quoted strings and dollar-quoted bodies with placeholders for keyword matching."""
    s = re.sub(r"\$([A-Za-z_][A-Za-z0-9_]*)?\$.*?\$\1\$", " $BODY$ ", s, flags=re.DOTALL)
    s = re.sub(r"'(?:[^']|'')*'", " 'LIT' ", s)
    return s


def strip_sql_comments(s: str) -> str:
    """Drop -- line comments and /* */ blocks (literals first, so '--' inside a string survives)."""
    s = strip_literals(s)
    s = re.sub(r"/\*.*?\*/", " ", s, flags=re.DOTALL)
    return re.sub(r"--[^\n]*", "", s)


def unquote(name: str) -> str:
    name = name.strip()
    parts = [p.strip().strip('"').lower() for p in name.split(".")]
    return ".".join(parts)


def short_table(name: str) -> str:
    return name.split(".")[-1]


def split_top_level(body: str, sep: str = ","):
    parts, depth, cur, q = [], 0, [], None
    i = 0
    while i < len(body):
        ch = body[i]
        if q:
            cur.append(ch)
            if ch == q:
                if q == "'" and i + 1 < len(body) and body[i + 1] == "'":
                    cur.append("'"); i += 2; continue
                q = None
            i += 1
            continue
        if ch in "'\"":
            q = ch
        elif ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        elif ch == sep and depth == 0:
            parts.append("".join(cur).strip())
            cur = []
            i += 1
            continue
        cur.append(ch)
        i += 1
    if "".join(cur).strip():
        parts.append("".join(cur).strip())
    return parts


def paren_body(s: str, start: int):
    """Return (body, end_index) for the parenthesized group starting at s[start] == '('."""
    depth, q = 0, None
    for j in range(start, len(s)):
        ch = s[j]
        if q:
            if ch == q:
                q = None
            continue
        if ch in "'\"":
            q = ch
        elif ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth == 0:
                return s[start + 1:j], j
    return s[start + 1:], len(s)


# ----------------------------------------------------------------------------- code extraction

GO_RAW = re.compile(r"`([^`]*)`", re.DOTALL)
GO_STR = re.compile(r'"((?:[^"\\\n]|\\.)*)"')
PY_TRIPLE = re.compile(r'("""|\'\'\')(.*?)\1', re.DOTALL)
PY_STR = re.compile(r'"((?:[^"\\\n]|\\.)*)"|\'((?:[^\'\\\n]|\\.)*)\'')


def detect_engine(src: str, path: str) -> str:
    if re.search(r"modernc\.org/sqlite|mattn/go-sqlite3|sql\.Open\(\s*\"sqlite|import sqlite3|sqlite3\.connect",
                 src):
        return "sqlite"
    return "postgres"


def extract_code_sql(path: Path, src: str):
    """Yield (sql_text, line) for string literals in Go/Python that contain DDL."""
    out = []
    if path.suffix == ".go":
        for m in GO_RAW.finditer(src):
            body = m.group(1)
            if DDL_RE.search(body):
                out.append((body, src.count("\n", 0, m.start()) + 1))
        for m in GO_STR.finditer(src):
            body = m.group(1)
            if DDL_RE.search(body):
                out.append((bytes(body, "utf-8").decode("unicode_escape", errors="ignore"),
                            src.count("\n", 0, m.start()) + 1))
    elif path.suffix == ".py":
        spans = []
        for m in PY_TRIPLE.finditer(src):
            body = m.group(2)
            spans.append((m.start(), m.end()))
            if DDL_RE.search(body):
                out.append((body, src.count("\n", 0, m.start()) + 1))
        for m in PY_STR.finditer(src):
            if any(a <= m.start() < b for a, b in spans):
                continue
            body = m.group(1) or m.group(2) or ""
            if DDL_RE.search(body):
                out.append((body, src.count("\n", 0, m.start()) + 1))
    return out


# ----------------------------------------------------------------------------- linter

TYPE_STOP = r"(?:NOT|NULL|DEFAULT|PRIMARY|REFERENCES|CHECK|UNIQUE|GENERATED|CONSTRAINT|COLLATE|COMPRESSION|STORAGE)"
VOLATILE_DEFAULT = re.compile(r"\b(gen_random_uuid|uuid_generate_v[14]|clock_timestamp|random|timeofday|nextval|"
                              r"statement_timestamp)\s*\(", re.IGNORECASE)
MONEYISH = re.compile(r"(^|_)(amount|price|cost|balance|total|fee|money|usd|eur|revenue|payment)s?($|_)", re.IGNORECASE)
LOWCARD_NAMES = re.compile(r"^(status|state|priority|severity|level|role|type|kind|enabled|active|outcome|protocol|"
                           r"layer|category|value_type|tier|mode|direction|is_\w+|has_\w+)$", re.IGNORECASE)
APPEND_ONLY = re.compile(r"(audit|history|transition|snapshot|ledger|journal)", re.IGNORECASE)
OPEN_VOCAB = re.compile(r"(^|_)(type|action|event|kind|source|triggered_by|tool)s?$", re.IGNORECASE)
IPISH = re.compile(r"(^|_)(ip|ip_addr|ip_address|addr|address|source_ip|client_ip|remote_ip)$", re.IGNORECASE)
SQL_TYPES = {"VARCHAR", "CHARACTER", "CHAR", "TEXT", "INT", "INTEGER", "BIGINT", "SMALLINT", "SERIAL",
             "BIGSERIAL", "NUMERIC", "DECIMAL", "REAL", "FLOAT", "DOUBLE", "BOOLEAN", "BOOL", "UUID", "JSON",
             "JSONB", "BYTEA", "TIMESTAMP", "TIMESTAMPTZ", "DATE", "TIME", "INTERVAL", "INET", "CIDR", "VECTOR",
             "HALFVEC", "TSVECTOR", "MONEY", "BIT", "VARBIT", "XML", "POINT"}
MYSQL_TOKENS = re.compile(r"\bAUTO_INCREMENT\b|\bENGINE\s*=|\bUNSIGNED\b|\bTINYINT\b|\bDATETIME\b|"
                          r"\bMEDIUMTEXT\b|\bLONGTEXT\b|\bON UPDATE CURRENT_TIMESTAMP\b", re.IGNORECASE)


class Linter:
    def __init__(self, include_tests=False, scan_code=True):
        self.findings: list[Finding] = []
        self.tables: dict[tuple, list[TableDef]] = {}
        self.indexes: list[dict] = []
        self.fks: list[dict] = []
        self.grants_seen = False
        self.default_privs_seen = False
        self.include_tests = include_tests
        self.scan_code = scan_code
        self.files_scanned = 0
        self.statements_scanned = 0
        self.seen_tables: set[str] = set()   # tables created by files processed earlier
        self.vector_ops: list[dict] = []      # (column, operator) used in queries
        self.check_values: dict = {}          # (table, column) -> number of values in a CHECK IN list
        self.stmt_starts: dict = {}           # file -> sorted statement start lines
        self.ignores: dict = {}               # file -> [(line, {rules})]
        self.suppressed = 0
        self.conf_shared_buffers_mb = None
        self.compose_pg_mem_mb = None

    def add(self, sev, rule, file, line, msg, ref=""):
        if self.is_ignored(file, line, rule):
            self.suppressed += 1
            return
        self.findings.append(Finding(sev, rule, file, line, msg.replace("__dyn__", "<dynamic>"), ref))

    def stmt_of(self, file, line):
        starts = self.stmt_starts.get(file, [])
        cur = None
        for s0 in starts:
            if s0 <= line:
                cur = s0
            else:
                break
        return cur

    def is_ignored(self, file, line, rule):
        ign = self.ignores.get(file)
        if not ign:
            return False
        target = self.stmt_of(file, line)
        for iline, rules in ign:
            if ("*" in rules or rule in rules) and (iline == line or self.stmt_of(file, iline) == target):
                return True
        return False

    # -- discovery ------------------------------------------------------------------------
    def walk(self, paths):
        for p in paths:
            p = Path(p)
            if p.is_file():
                yield p
                continue
            for root, dirs, files in os.walk(p):
                dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS and not d.startswith("."))
                for f in sorted(files):
                    yield Path(root) / f

    def run(self, paths):
        migration_dirs = {}
        all_paths = list(self.walk(paths))
        # SQL first (ledger order), then code — code-embedded DDL runs at service start, after migrations
        ordered = sorted([p for p in all_paths if p.suffix.lower() == ".sql"], key=lambda p: str(p)) + \
                  [p for p in all_paths if p.suffix.lower() != ".sql"]
        for path in ordered:
            sfx = path.suffix.lower()
            if sfx == ".sql":
                self.lint_sql_file(path)
                if "migration" in str(path.parent).lower():
                    migration_dirs.setdefault(path.parent, []).append(path)
                elif re.search(r"\b(CREATE|ALTER)\s+TABLE\b", strip_sql_comments(self.read(path)), re.IGNORECASE):
                    embedded = any(re.search(r"//go:embed\s+.*\b" + re.escape(path.name) + r"\b", self.read(g))
                                   for g in path.parent.glob("*.go"))
                    self.add("P1" if embedded else "P2", "schema-outside-ledger", str(path), 1,
                             ("Schema SQL embedded into a Go binary (go:embed) and applied by code — a second ledger "
                              "whose shape depends on boot order" if embedded else
                              "Schema SQL outside the migrations directory — if code applies it at startup, it is a "
                              "second ledger"), REF["ledger"])
            elif self.scan_code and sfx in CODE_EXT:
                if not self.include_tests and (path.name.endswith("_test.go") or path.name.startswith("test_")
                                               or "/tests/" in str(path) or "/testdata/" in str(path)):
                    continue
                self.lint_code_file(path)
            elif re.match(r"^pg_hba.*\.conf$", path.name):
                self.check_pg_hba(path)
            elif re.match(r"^postgresql.*\.conf$", path.name):
                self.check_pg_conf(path)
            elif re.match(r"^(docker-)?compose.*\.ya?ml$", path.name):
                self.check_compose(path)
            elif path.parent.name.lower() == "migrations" and path.is_file():
                migration_dirs.setdefault(path.parent, [])
                if sfx in (".sh", ".bash"):
                    self.add("P3", "ledger-non-sql", str(path), 1,
                             "Non-SQL file in a migrations directory — docker initdb executes *.sh found here; "
                             "keep the ledger SQL-only", REF["bootstrap"])
        for d, files in migration_dirs.items():
            self.check_ledger_dir(d, files)
        self.cross_checks()
        return self.findings

    # -- files ------------------------------------------------------------------------------
    def read(self, path):
        try:
            return path.read_text(encoding="utf-8", errors="replace")
        except OSError as e:
            self.add("P3", "io", str(path), 1, f"unreadable: {e}")
            return ""

    def lint_sql_file(self, path):
        src = self.read(path)
        self.files_scanned += 1
        engine = "sqlite" if "sqlite" in path.name.lower() else "postgres"
        self.lint_sql_text(src, str(path), 1, "sql", engine)

    def lint_code_file(self, path):
        src = self.read(path)
        if path.suffix == ".go":
            self.check_go_sql_format(path, src)
        self.collect_vector_ops(src, str(path))
        chunks = extract_code_sql(path, src)
        if not chunks:
            return
        self.files_scanned += 1
        engine = detect_engine(src, str(path))
        for body, line in chunks:
            body = body.replace("%s", "__dyn__").replace("%v", "__dyn__")
            if engine == "sqlite":
                self.add("P3", "ddl-in-code-sqlite", str(path), line,
                         "SQLite schema created from application code — acceptable for a rebuildable L1 cache; "
                         "version the cache schema and keep Postgres as the source of truth",
                         "references/other-engines.md §3")
            else:
                self.add("P1", "ddl-outside-ledger", str(path), line,
                         "DDL executed from application code — a second source of schema truth; move it into the "
                         "migration ledger and make the service check the schema version instead", REF["ledger"])
            self.lint_sql_text(body, str(path), line, "code", engine)

    # -- statements ---------------------------------------------------------------------------
    def lint_sql_text(self, src, file, base_line, origin, engine):
        if origin == "sql":
            self.collect_vector_ops(src, file)
        items = split_sql(src, base_line)
        self.stmt_starts.setdefault(file, [])
        self.stmt_starts[file] = sorted(set(self.stmt_starts[file]) | {l for k, _, l in items if k == "stmt"})
        for off, raw in enumerate(src.splitlines()):
            im = re.search(r"--\s*(?:well|erda):ignore\s+([\w\-,*]+)\s*(.*)$", raw)
            if im:
                rules = {r.strip().lower() for r in im.group(1).split(",") if r.strip()}
                self.ignores.setdefault(file, []).append((base_line + off, rules))
                if not im.group(2).strip():
                    self.findings.append(Finding("P3", "ignore-without-rationale", file, base_line + off,
                                                 "well:ignore without a rationale — ADR-073: say why the signal is "
                                                 "wrong at this site", REF["ledger"]))
        metas = [(t, l) for k, t, l in items if k == "meta"]
        if metas:
            sample = ", ".join(sorted({t.split()[0] for t, _ in metas}))
            self.add("P2", "psql-meta", file, metas[0][1],
                     f"psql meta-commands in migration ({sample}) — ties the file to psql and to hard-coded "
                     f"database names; let the runner choose the database", REF["ledger"])
        db = None
        created_here: dict[str, str] = {}   # short name -> "new" | "maybe" | "existing"
        created_line: dict[str, int] = {}
        fk_mark = len(self.fks)
        file_flags = {"needs_lock_timeout": None, "has_lock_timeout": False, "in_tx": False}
        secret_lines, prov_lines = [], []
        tx_open = False
        stmts = []
        for kind, text, line in items:
            if kind == "meta":
                m = re.match(r"\\(?:c|connect)\s+(\S+)", text)
                if m:
                    db = m.group(1).strip(";")
                continue
            stmts.append(Statement(text, norm_ws(text), file, line, origin, engine, db))
        for st in stmts:
            self.statements_scanned += 1
            u = strip_literals(st.norm).upper()
            if re.match(r"^(BEGIN|START TRANSACTION)\b", u):
                tx_open = True
                continue
            if re.match(r"^(COMMIT|END|ROLLBACK)\b", u):
                tx_open = False
                continue
            if re.search(r"\bSET\s+(LOCAL\s+)?LOCK_TIMEOUT\b", u):
                file_flags["has_lock_timeout"] = True
            if MYSQL_TOKENS.search(u):
                self.add("P1", "mysql-syntax", file, st.line,
                         "MySQL-only syntax in PostgreSQL DDL (AUTO_INCREMENT/ENGINE/UNSIGNED/TINYINT/DATETIME...)",
                         REF["types"])
            if re.match(r"^CREATE\s+(UNLOGGED\s+|TEMP(ORARY)?\s+)?TABLE\b", u) and " AS SELECT" not in u:
                td = self.parse_create_table(st)
                if td:
                    sn = short_table(td.name)
                    created_line.setdefault(sn, st.line)
                    if not td.if_not_exists:
                        created_here[sn] = "new"
                    else:
                        created_here[sn] = "existing" if sn in self.seen_tables else "maybe"
            elif re.match(r"^CREATE\s+(UNIQUE\s+)?INDEX\b", u):
                self.parse_create_index(st, created_here, tx_open, file_flags)
            elif re.match(r"^ALTER\s+TABLE\b", u):
                self.check_alter(st, u, created_here, file_flags)
            elif re.match(r"^GRANT\b", u):
                self.check_grant(st, u)
            elif re.match(r"^ALTER\s+DEFAULT\s+PRIVILEGES\b", u):
                self.default_privs_seen = True
            elif re.match(r"^(CREATE|ALTER)\s+(USER|ROLE)\b", u):
                if re.search(r"\bPASSWORD\s+'LIT'", u):
                    secret_lines.append(st.line)
                if origin == "sql":
                    prov_lines.append(st.line)
            elif re.match(r"^CREATE\s+DATABASE\b", u):
                prov_lines.append(st.line)
            elif re.match(r"^CREATE\s+EXTENSION\b", u) and origin == "sql":
                self.add("P3", "extension-in-migration", file, st.line,
                         "CREATE EXTENSION needs elevated privileges and an image that ships the extension — make it a "
                         "provisioning step with a pinned version (VERSION '...')", "references/other-engines.md §8")
            elif re.match(r"^(TRUNCATE|VACUUM\s+FULL|CLUSTER)\b", u):
                self.add("P1", "lock-hazard", file, st.line,
                         f"{u.split()[0]} takes ACCESS EXCLUSIVE (rewrite/empties table) — never in a routine migration",
                         REF["locks"])
            elif re.match(r"^REFRESH\s+MATERIALIZED\s+VIEW\b", u) and "CONCURRENTLY" not in u:
                self.add("P2", "lock-hazard", file, st.line,
                         "REFRESH MATERIALIZED VIEW without CONCURRENTLY blocks readers (needs a unique index)",
                         REF["locks"])
            elif re.match(r"^DROP\s+TABLE\b", u):
                self.add("P2", "destructive", file, st.line,
                         "DROP TABLE — contract phase only, after a verified backup", REF["expand"])
            elif re.match(r"^DROP\s+INDEX\b", u) and "CONCURRENTLY" not in u and origin == "sql":
                self.add("P3", "lock-hazard", file, st.line,
                         "DROP INDEX without CONCURRENTLY takes ACCESS EXCLUSIVE", REF["locks"])
            elif re.match(r"^(UPDATE|DELETE\s+FROM)\b", u):
                if " WHERE " not in u:
                    self.add("P1", "unbatched-dml", file, st.line,
                             "UPDATE/DELETE without WHERE in a migration — rewrites/bloats the whole table in one "
                             "transaction; batch it", "references/migrations.md §6")
                else:
                    tm = re.match(r"^(?:UPDATE|DELETE\s+FROM)\s+(?:ONLY\s+)?([\w.\"]+)", st.norm, re.IGNORECASE)
                    tgt = short_table(unquote(tm.group(1))) if tm else ""
                    batched = re.search(r"\bBETWEEN\b|\bLIMIT\b|\bctid\b|\bid\s*[<>]=?", u, re.IGNORECASE)
                    if origin == "sql" and created_here.get(tgt) in (None, "existing") and not batched:
                        self.add("P2", "unbatched-backfill", file, st.line,
                                 f"Single-statement data change on existing table {tgt} inside a migration — on a large "
                                 f"table it holds row locks, bloats by its full size and blocks vacuum cleanup; move it "
                                 f"to a separate batched, resumable job", "references/migrations.md §6")
                    else:
                        self.add("P3", "backfill", file, st.line,
                                 "Data change in a migration — fine on small tables; on large tables batch by key range "
                                 "in a separate, resumable data migration", "references/migrations.md §6")
            elif re.match(r"^CREATE\s+(OR\s+REPLACE\s+)?FUNCTION\b", u) and "SECURITY DEFINER" in u \
                    and "SEARCH_PATH" not in u:
                self.add("P2", "security-definer", file, st.line,
                         "SECURITY DEFINER function without SET search_path", "references/access-and-security.md §8")
        if secret_lines:
            self.add("P1", "secret-in-sql", file, secret_lines[0],
                     f"{len(secret_lines)} password literal(s) in SQL (lines {', '.join(map(str, secret_lines))}) — "
                     f"credentials belong in the secrets backend, set at provisioning (CREATE ROLE ... PASSWORD NULL; "
                     f"ALTER ROLE from secrets)", REF["secrets"])
        if prov_lines:
            self.add("P3", "provisioning-in-ledger", file, prov_lines[0],
                     f"{len(prov_lines)} role/database creation statement(s) in the migration set — provisioning is "
                     f"environment-specific; keep it out of the schema ledger or parameterize it", REF["ledger"])
        for fk in self.fks[fk_mark:]:
            ref = short_table(fk["ref"])
            if ref in created_line and created_line[ref] > fk["line"] and ref not in self.seen_tables:
                self.add("P1", "fk-forward-reference", file, fk["line"],
                         f"{short_table(fk['table'])} references {ref}, which this file creates later (line "
                         f"{created_line[ref]}) — the migration fails; create referenced tables first",
                         REF["constraints"])
        self.seen_tables.update(created_here.keys())
        if file_flags["needs_lock_timeout"] and not file_flags["has_lock_timeout"] and origin == "sql":
            self.add("P2", "no-lock-timeout", file, file_flags["needs_lock_timeout"],
                     "Alters existing tables without SET lock_timeout — a blocked DDL queues every later query on "
                     "the table", REF["locks"])

    # -- CREATE TABLE -----------------------------------------------------------------------
    def parse_create_table(self, st: Statement):
        m = re.match(r"CREATE\s+(?:UNLOGGED\s+|TEMP(?:ORARY)?\s+)?TABLE\s+(IF\s+NOT\s+EXISTS\s+)?"
                     r"((?:\"[^\"]+\"|[\w$]+)(?:\.(?:\"[^\"]+\"|[\w$]+))?)\s*\(", st.norm, re.IGNORECASE)
        if not m:
            return None
        name = unquote(m.group(2))
        body, _ = paren_body(st.norm, m.end() - 1)
        td = TableDef(name=name, file=st.file, line=st.line, origin=st.origin, engine=st.engine, db=st.db,
                      if_not_exists=bool(m.group(1)))
        tail = st.norm[m.end() - 1:]
        if "__dyn__" in name:
            self.add("P2", "dynamic-table-name", st.file, st.line,
                     "Table name injected with a format verb — dynamic identifiers defeat review and grants; fix the "
                     "name in the ledger", "references/access-and-security.md §8")
        if st.origin == "sql" and td.if_not_exists:
            pass  # judged in cross-checks (drift); IF NOT EXISTS alone is informational
        varchar_cols, serial_cols, nullable_empty, nullable_ts, mysql_inline = [], [], [], [], []
        nullable_default = []
        for el in split_top_level(body):
            eu = strip_literals(el).upper()
            if re.match(r"^(CONSTRAINT\b|PRIMARY\s+KEY|UNIQUE\b|FOREIGN\s+KEY|CHECK\b|EXCLUDE\b|LIKE\b)", eu):
                if re.match(r"^(CONSTRAINT\s+\S+\s+)?PRIMARY\s+KEY", eu):
                    td.has_pk = True
                    cols = re.search(r"PRIMARY\s+KEY\s*\(([^)]*)\)", el, re.IGNORECASE)
                    if cols:
                        self.indexes.append({"table": name, "cols": tuple(unquote(c) for c in cols.group(1).split(",")),
                                             "method": "btree", "where": "", "unique": True, "name": f"{short_table(name)}_pkey",
                                             "file": st.file, "line": st.line, "implicit": True, "partial": False, "db": st.db})
                fk = re.search(r"FOREIGN\s+KEY\s*\(([^)]*)\)\s*REFERENCES\s+([\w.\"]+)", el, re.IGNORECASE)
                if fk:
                    self.fks.append({"table": name, "cols": tuple(unquote(c) for c in fk.group(1).split(",")),
                                     "ref": unquote(fk.group(2)), "file": st.file, "line": st.line})
                uq = re.match(r"^(?:CONSTRAINT\s+\S+\s+)?UNIQUE\s*\(([^)]*)\)", el, re.IGNORECASE)
                if uq:
                    self.indexes.append({"table": name, "cols": tuple(unquote(c) for c in uq.group(1).split(",")),
                                         "method": "btree", "where": "", "unique": True, "name": "(unique)",
                                         "file": st.file, "line": st.line, "implicit": True, "partial": False, "db": st.db})
                self.check_vocab(el, name, st)
                continue
            mi = re.match(r"^(INDEX|KEY)\s+([\w\"]+)\s*\([^()]*\)\s*$", eu)
            if mi and mi.group(2).strip('"') not in SQL_TYPES:
                mysql_inline.append(el.split("(")[0].strip())
                continue
            cm = re.match(r"^(\"[^\"]+\"|[\w$]+)\s+(.*)$", el, re.DOTALL)
            if not cm:
                continue
            col = unquote(cm.group(1))
            rest = cm.group(2)
            rest_u = strip_literals(rest).upper()
            tm = re.match(rf"^(.*?)(?=\s+{TYPE_STOP}\b|$)", rest, re.IGNORECASE | re.DOTALL)
            ctype = norm_ws(tm.group(1)).upper() if tm else rest.upper()
            notnull = bool(re.search(r"\bNOT\s+NULL\b", rest_u)) or bool(re.search(r"\bPRIMARY\s+KEY\b", rest_u))
            pk = bool(re.search(r"\bPRIMARY\s+KEY\b", rest_u))
            dm = re.search(r"\bDEFAULT\s+(.+?)(?=\s+(?:NOT\s+NULL|NULL|PRIMARY|REFERENCES|CHECK|UNIQUE|CONSTRAINT|"
                           r"GENERATED|COLLATE)\b|$)", rest, re.IGNORECASE | re.DOTALL)
            default = norm_ws(dm.group(1)) if dm else None
            td.columns[col] = {"type": ctype, "notnull": notnull, "default": default, "pk": pk}
            if pk:
                td.has_pk = True
                self.indexes.append({"table": name, "cols": (col,), "method": "btree", "where": "", "unique": True,
                                     "name": f"{short_table(name)}_pkey", "file": st.file, "line": st.line,
                                     "implicit": True, "partial": False})
            if re.search(r"\bUNIQUE\b", rest_u):
                self.indexes.append({"table": name, "cols": (col,), "method": "btree", "where": "", "unique": True,
                                     "name": "(unique)", "file": st.file, "line": st.line, "implicit": True,
                                     "partial": False, "db": st.db})
            rm = re.search(r"\bREFERENCES\s+([\w.\"]+)", rest, re.IGNORECASE)
            if rm:
                self.fks.append({"table": name, "cols": (col,), "ref": unquote(rm.group(1)), "file": st.file,
                                 "line": st.line})
            self.check_vocab(rest, name, st, col)
            if st.engine != "postgres":
                continue
            # --- type smells
            if re.match(r"^(SMALL|BIG)?SERIAL\b", ctype):
                serial_cols.append(col)
            if re.match(r"^(CHARACTER VARYING|VARCHAR)\s*\(", ctype):
                varchar_cols.append(col)
            self.check_column_type(st, name, col, ctype, default, pk)
            if not notnull and default and re.fullmatch(r"''(::\w+)?", default.strip()):
                nullable_empty.append(col)
            elif not notnull and default and not re.match(r"(?i)null\b", default.strip()) and \
                    col not in ("created_at", "updated_at"):
                nullable_default.append(col)
            if not notnull and col in ("created_at", "updated_at"):
                nullable_ts.append(col)
        if mysql_inline:
            self.add("P0" if st.engine == "postgres" else "P1", "mysql-syntax", st.file, st.line,
                     f"{short_table(name)}: inline {', '.join(mysql_inline)} inside CREATE TABLE is MySQL syntax — "
                     f"PostgreSQL rejects the statement; use separate CREATE INDEX statements", REF["types"])
        if not td.has_pk and st.engine == "postgres" and "__dyn__" not in name:
            self.add("P1", "no-primary-key", st.file, st.line,
                     f"{name} has no primary key", REF["constraints"])
        if serial_cols:
            self.add("P3", "serial", st.file, st.line,
                     f"{short_table(name)}: {', '.join(serial_cols)} use SERIAL — prefer GENERATED ALWAYS AS IDENTITY",
                     REF["keys"])
        if varchar_cols and len(varchar_cols) >= 3:
            self.add("INFO", "varchar-n", st.file, st.line,
                     f"{short_table(name)}: {len(varchar_cols)} varchar(n) columns — text + CHECK(length) is easier to "
                     f"evolve; keep varchar(n) only for real limits", REF["types"])
        if nullable_empty:
            self.add("P2", "nullable-empty-default", st.file, st.line,
                     f"{short_table(name)}: {', '.join(nullable_empty)} {'is' if len(nullable_empty) == 1 else 'are'} nullable with DEFAULT '' — two kinds of "
                     f"empty; make them NOT NULL or drop the fake default", REF["nulls"])
        if nullable_default and st.engine == "postgres":
            self.add("P3", "nullable-with-default", st.file, st.line,
                     f"{short_table(name)}: {', '.join(nullable_default)} have a DEFAULT but allow NULL — an explicit "
                     f"NULL still gets in; add NOT NULL unless NULL means something", REF["nulls"])
        if nullable_ts:
            self.add("P3", "nullable-timestamps", st.file, st.line,
                     f"{short_table(name)}: {', '.join(nullable_ts)} nullable — NOT NULL DEFAULT now()", REF["nulls"])
        key = (st.engine, short_table(name))
        self.tables.setdefault(key, []).append(td)
        # trailing table options
        if re.search(r"\)\s*ENGINE\s*=", tail, re.IGNORECASE):
            self.add("P1", "mysql-syntax", st.file, st.line, "ENGINE= table option is MySQL", REF["types"])
        return td

    def check_column_type(self, st, name, col, ctype, default, pk):
        """Per-column type smells — shared by CREATE TABLE and ALTER TABLE ... ADD COLUMN."""
        if re.match(r"^TIMESTAMP\b(?!\s*WITH\s+TIME\s+ZONE)|^TIMESTAMP\s+WITHOUT", ctype) and "TIMESTAMPTZ" not in ctype:
            self.add("P2", "timestamp-no-tz", st.file, st.line,
                     f"{short_table(name)}.{col} is TIMESTAMP without time zone — use timestamptz", REF["types"])
        if ctype == "JSON":
            self.add("P2", "json-not-jsonb", st.file, st.line,
                     f"{short_table(name)}.{col} is JSON — use JSONB (indexable, no re-parse)", REF["types"])
        if re.match(r"^(REAL|FLOAT|DOUBLE PRECISION|FLOAT[48])\b", ctype) and MONEYISH.search(col):
            self.add("P1", "float-money", st.file, st.line,
                     f"{short_table(name)}.{col} stores money-like values as binary float — use numeric or "
                     f"integer minor units", REF["types"])
        if ctype.startswith("MONEY"):
            self.add("P2", "money-type", st.file, st.line, f"{short_table(name)}.{col} uses MONEY — use numeric",
                     REF["types"])
        if ctype.endswith("[]") and (re.match(r"^(BIG)?INT|^INTEGER|^UUID", ctype)
                                     or re.search(r"(_ids|depends_on|dependencies|parents|children)$", col)):
            self.add("P2", "array-of-references", st.file, st.line,
                     f"{short_table(name)}.{col} is an array of references — no referential integrity; use a "
                     f"junction table", REF["constraints"])
        if (IPISH.search(col) and re.match(r"^(VARCHAR|CHARACTER VARYING|TEXT|CHAR)", ctype)) or \
                re.match(r"^(VARCHAR|CHARACTER VARYING)\s*\(\s*(39|45)\s*\)", ctype):
            self.add("P3", "ip-as-text", st.file, st.line,
                     f"{short_table(name)}.{col} stores an address as text — use inet/cidr", REF["types"])
        if re.search(r"(^|_)(trace|span)_id$", col) and re.match(r"^(VARCHAR|CHARACTER VARYING|TEXT)", ctype):
            self.add("P3", "traceid-as-text", st.file, st.line,
                     f"{short_table(name)}.{col} stores a trace/span id as text — uuid or fixed-width bytea; "
                     f"NULL when absent, not ''", REF["types"])
        if re.search(r"embedding|vector", col) and ctype.startswith("BYTEA"):
            self.add("P3", "embedding-bytea", st.file, st.line,
                     f"{short_table(name)}.{col} keeps an embedding as bytea — not searchable in-DB; "
                     f"vector/halfvec when similarity search is needed", "references/indexing-and-query-tuning.md §4")
        if pk and col == "id" and re.match(r"^(TEXT|VARCHAR|CHARACTER VARYING)", ctype):
            self.add("P3", "text-surrogate-pk", st.file, st.line,
                     f"{short_table(name)}.id is a text primary key — prefer bigint identity or uuid (v7) "
                     f"unless it is a natural key", REF["keys"])
        if pk and ctype.startswith("UUID") and default and re.search(r"gen_random_uuid|uuid_generate_v4", default, re.IGNORECASE):
            self.add("P2", "uuid-v4-pk", st.file, st.line,
                     f"{short_table(name)}.{col}: random UUIDv4 primary key scatters B-tree inserts — use UUIDv7 "
                     f"or bigint identity", REF["keys"])

    def check_vocab(self, text, table, st, col=None):
        for cm in re.finditer(r"CHECK\s*\(\s*\(?\s*([\w\"]+)\s*\)?\s+IN\s*\(", text, re.IGNORECASE):
            body, _ = paren_body(text, cm.end() - 1)
            values = re.findall(r"'((?:[^']|'')*)'", body)
            column = unquote(cm.group(1))
            self.check_values[(short_table(table), column)] = len(values)
            if len(values) > 12 or (OPEN_VOCAB.search(column) and len(values) > 6):
                self.add("P2", "open-vocabulary-check", st.file, st.line,
                         f"{short_table(table)}.{column}: CHECK lists {len(values)} values — if this vocabulary grows "
                         f"with the code, every new value becomes a failed INSERT; use a lookup table + FK or "
                         f"boundary validation", REF["vocab"])

    # -- CREATE INDEX ------------------------------------------------------------------------
    def parse_create_index(self, st, created_here, tx_open, file_flags):
        m = re.match(r"CREATE\s+(UNIQUE\s+)?INDEX\s+(CONCURRENTLY\s+)?(IF\s+NOT\s+EXISTS\s+)?([\w\"$]+\s+)?ON\s+"
                     r"(ONLY\s+)?([\w.\"$]+)\s*(USING\s+(\w+)\s*)?\(", st.norm, re.IGNORECASE)
        if not m:
            return
        unique = bool(m.group(1))
        concurrently = bool(m.group(2))
        iname = (m.group(4) or "").strip().strip('"')
        table = unquote(m.group(6))
        method = (m.group(8) or "btree").lower()
        body, end = paren_body(st.norm, m.end() - 1)
        where = ""
        wm = re.search(r"\bWHERE\b(.*)$", st.norm[end + 1:], re.IGNORECASE | re.DOTALL)
        if wm:
            where = norm_ws(wm.group(1)).rstrip(";").lower()
        cols = []
        for c in split_top_level(body):
            c0 = re.sub(r"\s+(ASC|DESC|NULLS\s+(FIRST|LAST)|\w+_ops)\b", "", c, flags=re.IGNORECASE).strip()
            cols.append(unquote(c0) if re.fullmatch(r"[\w\"$]+", c0) else norm_ws(c0).lower())
        opclass = (re.search(r"\b(\w+_ops)\b", body) or [None, None])[1]
        idx = {"table": table, "cols": tuple(cols), "method": method, "where": where, "unique": unique, "name": iname,
               "file": st.file, "line": st.line, "implicit": False, "partial": bool(where), "db": st.db,
               "opclass": (opclass or "").lower()}
        self.indexes.append(idx)
        if method == "ivfflat" and created_here.get(short_table(table)) in ("new", "maybe"):
            lists = re.search(r"lists\s*=\s*(\d+)", st.norm[end:], re.IGNORECASE)
            self.add("P1", "pgvector-ivfflat-empty", st.file, st.line,
                     f"IVFFlat index {iname} built in the same migration that creates {table} — k-means centroids "
                     f"trained on an empty table are useless{f' (lists={lists.group(1)})' if lists else ''}; use HNSW, "
                     f"or build IVFFlat after the data is loaded (lists ≈ rows/1000 up to 1M rows, √rows above)",
                     "references/indexing-and-query-tuning.md §4")
        if method in ("hnsw", "ivfflat") and not opclass:
            self.add("P2", "pgvector-no-opclass", st.file, st.line,
                     f"{method} index {iname} without an explicit opclass defaults to L2 — state the opclass that "
                     f"matches the query operator", "references/indexing-and-query-tuning.md §4")
        existing = created_here.get(short_table(table)) in (None, "existing")
        if st.engine == "postgres" and existing and not concurrently:
            self.add("P1", "index-not-concurrent", st.file, st.line,
                     f"CREATE INDEX {iname or ''} on existing table {table} without CONCURRENTLY — blocks writes for "
                     f"the whole build", REF["locks"])
            file_flags["needs_lock_timeout"] = file_flags["needs_lock_timeout"] or st.line
        if concurrently and tx_open:
            self.add("P1", "concurrently-in-tx", st.file, st.line,
                     "CREATE INDEX CONCURRENTLY inside BEGIN/COMMIT fails — run it in a no-transaction migration",
                     REF["locks"])

    # -- ALTER TABLE -----------------------------------------------------------------------------
    def check_alter(self, st, u, created_here, file_flags):
        m = re.match(r"ALTER\s+TABLE\s+(IF\s+EXISTS\s+)?(ONLY\s+)?([\w.\"$]+)\s+(.*)$", st.norm, re.IGNORECASE | re.DOTALL)
        if not m:
            return
        table = unquote(m.group(3))
        actions = split_top_level(m.group(4).rstrip().rstrip(";"))   # the last action must not carry the ';'
        existing = created_here.get(short_table(table)) in (None, "existing")
        if st.engine != "postgres":
            return
        for act in actions:
            au = strip_literals(act).upper()
            hazard = False
            if re.match(r"^ADD\s+(COLUMN\s+)?(IF\s+NOT\s+EXISTS\s+)?[\w\"]+\s", au) and not re.match(r"^ADD\s+(CONSTRAINT|PRIMARY|UNIQUE|FOREIGN|CHECK|EXCLUDE)\b", au):
                if VOLATILE_DEFAULT.search(act) and existing:
                    self.add("P1", "volatile-default-rewrite", st.file, st.line,
                             f"ADD COLUMN on {table} with a volatile DEFAULT rewrites the whole table under ACCESS "
                             f"EXCLUSIVE — add nullable, backfill in batches, then SET DEFAULT", REF["locks"])
                if re.search(r"\bGENERATED\s+ALWAYS\s+AS\s*\(.*\)\s*STORED", au) and existing:
                    self.add("P1", "stored-generated-rewrite", st.file, st.line,
                             f"ADD COLUMN ... GENERATED ... STORED on {table} rewrites the table", REF["locks"])
                if re.search(r"\bNOT\s+NULL\b", au) and "DEFAULT" not in au and existing:
                    self.add("P1", "add-not-null-no-default", st.file, st.line,
                             f"ADD COLUMN ... NOT NULL without DEFAULT on {table} fails if rows exist", REF["locks"])
                cm = re.match(r"ADD\s+(?:COLUMN\s+)?(?:IF\s+NOT\s+EXISTS\s+)?(\"[^\"]+\"|[\w$]+)\s+(.*)$", act,
                              re.IGNORECASE | re.DOTALL)
                if cm:
                    col, rest = unquote(cm.group(1)), cm.group(2)
                    tm = re.match(rf"^(.*?)(?=\s+{TYPE_STOP}\b|$)", rest, re.IGNORECASE | re.DOTALL)
                    ctype = norm_ws(tm.group(1)).upper() if tm else rest.upper()
                    dm = re.search(r"\bDEFAULT\s+(.+?)(?=\s+(?:NOT\s+NULL|NULL|PRIMARY|REFERENCES|CHECK|UNIQUE|"
                                   r"CONSTRAINT|GENERATED|COLLATE)\b|$)", rest, re.IGNORECASE | re.DOTALL)
                    self.check_column_type(st, table, col, ctype, norm_ws(dm.group(1)) if dm else None,
                                           bool(re.search(r"\bPRIMARY\s+KEY\b", au)))
                    rm = re.search(r"\bREFERENCES\s+([\w.\"]+)", rest, re.IGNORECASE)
                    if rm:
                        self.fks.append({"table": table, "cols": (col,), "ref": unquote(rm.group(1)),
                                         "file": st.file, "line": st.line})
                    inline = [k for k, rx in (("FOREIGN KEY", r"\bREFERENCES\b"), ("CHECK", r"\bCHECK\s*\("))
                              if re.search(rx, au)]
                    if inline and existing:
                        self.add("P1", "constraint-without-not-valid", st.file, st.line,
                                 f"ADD COLUMN {col} on existing {table} with inline {' + '.join(inline)} — validated "
                                 f"in the same statement under ACCESS EXCLUSIVE (inline constraints cannot be NOT "
                                 f"VALID); add the bare column, then ADD CONSTRAINT ... NOT VALID, then VALIDATE "
                                 f"CONSTRAINT", REF["locks"])
                hazard = True
            elif re.search(r"^ALTER\s+(COLUMN\s+)?[\w\"]+\s+(SET\s+DATA\s+)?TYPE\b", au):
                if existing:
                    self.add("P1", "type-change-rewrite", st.file, st.line,
                             f"ALTER COLUMN TYPE on {table} — usually a full rewrite + index rebuild under ACCESS "
                             f"EXCLUSIVE; expand/contract with a new column", REF["expand"])
                hazard = True
            elif re.search(r"^ALTER\s+(COLUMN\s+)?[\w\"]+\s+SET\s+NOT\s+NULL", au):
                if existing:
                    self.add("P2", "set-not-null-scan", st.file, st.line,
                             f"SET NOT NULL on {table} scans the table under ACCESS EXCLUSIVE — add CHECK (col IS NOT "
                             f"NULL) NOT VALID, VALIDATE, then SET NOT NULL", REF["locks"])
                hazard = True
            elif re.match(r"^ADD\s+(CONSTRAINT\s+\S+\s+)?(FOREIGN\s+KEY|CHECK)\b", au):
                fk = re.search(r"FOREIGN\s+KEY\s*\(([^)]*)\)\s*REFERENCES\s+([\w.\"]+)", act, re.IGNORECASE)
                if fk:
                    self.fks.append({"table": table, "cols": tuple(unquote(c) for c in fk.group(1).split(",")),
                                     "ref": unquote(fk.group(2)), "file": st.file, "line": st.line})
                if existing and "NOT VALID" not in au:
                    self.add("P1", "constraint-without-not-valid", st.file, st.line,
                             f"ADD {'FOREIGN KEY' if 'FOREIGN' in au else 'CHECK'} on existing {table} without NOT VALID "
                             f"— validates under a blocking lock; add NOT VALID then VALIDATE CONSTRAINT", REF["locks"])
                self.check_vocab(act, table, st)
                hazard = True
            elif re.match(r"^ADD\s+(CONSTRAINT\s+\S+\s+)?(PRIMARY\s+KEY|UNIQUE)\b", au):
                if existing and "USING INDEX" not in au:
                    self.add("P1", "constraint-index-build", st.file, st.line,
                             f"ADD PRIMARY KEY/UNIQUE on existing {table} builds an index under ACCESS EXCLUSIVE — "
                             f"CREATE UNIQUE INDEX CONCURRENTLY, then ADD CONSTRAINT ... USING INDEX", REF["locks"])
                hazard = True
            elif re.match(r"^RENAME\b", au):
                self.add("P2", "rename-breaks-code", st.file, st.line,
                         f"RENAME on {table} breaks running code — expand/contract or a compatibility view", REF["expand"])
                hazard = True
            elif re.match(r"^DROP\s+(COLUMN\b|[\w\"]+\s*$)", au) and not re.match(r"^DROP\s+(CONSTRAINT|DEFAULT|NOT)", au):
                self.add("P2", "destructive", st.file, st.line,
                         f"DROP COLUMN on {table} — contract phase only, after no deployed code reads it", REF["expand"])
                hazard = True
            elif re.match(r"^DETACH\s+PARTITION\b", au) and "CONCURRENTLY" not in au:
                self.add("P2", "lock-hazard", st.file, st.line,
                         f"DETACH PARTITION on {table} without CONCURRENTLY takes ACCESS EXCLUSIVE (PG14+ has CONCURRENTLY)",
                         REF["locks"])
                hazard = True
            elif re.match(r"^(ALTER\s+(COLUMN\s+)?[\w\"]+\s+(SET|DROP)\s+DEFAULT|DROP\s+CONSTRAINT|ENABLE|DISABLE)", au):
                hazard = True
            if hazard and existing:
                file_flags["needs_lock_timeout"] = file_flags["needs_lock_timeout"] or st.line

    # -- GRANT --------------------------------------------------------------------------------------
    def check_grant(self, st, u):
        m = re.match(r"GRANT\s+(.+?)\s+ON\s+(TABLE\s+)?(.+?)\s+TO\s+(.+)$", st.norm, re.IGNORECASE | re.DOTALL)
        if not m:
            return
        privs = m.group(1).upper()
        target = m.group(3)
        roles = m.group(4)
        if re.match(r"^(ALL\s+TABLES|SCHEMA|DATABASE|SEQUENCE|ALL\s+SEQUENCES|FUNCTION)", target.strip(), re.IGNORECASE):
            if "PUBLIC" in roles.upper().split():
                self.add("P2", "grant-public", st.file, st.line, f"GRANT ... TO PUBLIC on {target}", REF["grants"])
            return
        self.grants_seen = True
        tables = [unquote(t) for t in target.split(",")]
        if re.match(r"^ALL\b", privs.strip()):
            self.add("P2", "grant-all", st.file, st.line,
                     f"GRANT ALL on {', '.join(tables)} to {roles.strip()} — also grants TRUNCATE, REFERENCES and "
                     f"TRIGGER; grant the DML the role needs", REF["grants"])
        if re.search(r"\b(UPDATE|DELETE|TRUNCATE|ALL)\b", privs):
            hits = [t for t in tables if APPEND_ONLY.search(t)]
            if hits:
                self.add("P1", "append-only-writable", st.file, st.line,
                         f"{privs.strip()} granted on {', '.join(hits)} to {roles.strip()} — append-only history must "
                         f"be INSERT-only for writers", REF["appendonly"])
        if re.search(r"\bPUBLIC\b", roles, re.IGNORECASE):
            self.add("P2", "grant-public", st.file, st.line, f"GRANT ... TO PUBLIC on {target}", REF["grants"])

    # -- ledger dir ---------------------------------------------------------------------------------
    def check_ledger_dir(self, d, files):
        versions = {}
        for f in files:
            mm = re.match(r"^(\d+)[_\-.]", f.name)
            if not mm:
                self.add("P3", "ledger-naming", str(f), 1,
                         "Migration file without a numeric version prefix", REF["ledger"])
                continue
            v = int(mm.group(1))
            if v in versions:
                self.add("P1", "ledger-duplicate-version", str(f), 1,
                         f"Duplicate migration version {v} (also {versions[v].name})", REF["ledger"])
            versions[v] = f
        self.add("INFO", "ledger-check", str(d), 1,
                 f"{len(files)} migration files; confirm a ledger table records applied versions and checksums "
                 f"(none can be verified statically)", REF["ledger"])

    # -- cross-file checks ------------------------------------------------------------------------
    def cross_checks(self):
        # shape drift
        for (engine, tname), defs in self.tables.items():
            if len(defs) < 2 or "__dyn__" in tname:
                continue
            shapes = {}
            for d in defs:
                sig = tuple(sorted((c, re.sub(r"\s+", " ", v["type"])) for c, v in d.columns.items()))
                shapes.setdefault(sig, []).append(d)
            if len(shapes) > 1:
                in_code = any(d.origin == "code" for d in defs)
                where = "; ".join(f"{Path(d.file).name}:{d.line}" + (f" [db {d.db}]" if d.db else "") +
                                  (" (code)" if d.origin == "code" else "") for d in defs)
                col_sets = [set(d.columns) for d in defs]
                only = set.union(*col_sets) - set.intersection(*col_sets)
                self.add("P0" if in_code else "P1", "shape-drift", defs[0].file, defs[0].line,
                         f"Table {tname} is defined {len(defs)} times with {len(shapes)} different shapes ({where}). "
                         f"Columns not in every definition: {', '.join(sorted(only)) or '(types differ)'}. With IF NOT "
                         f"EXISTS the first definition to run wins and the rest silently no-op",
                         REF["ledger"])
        # duplicate / redundant indexes (explicit indexes only; compare per table)
        by_table = {}
        for ix in self.indexes:
            by_table.setdefault(short_table(ix["table"]), []).append(ix)
        for t, ixs in by_table.items():
            seen = {}
            for ix in ixs:
                key = (ix["method"], ix["cols"], ix["where"])
                a = seen.get(key)
                if a is None:
                    seen[key] = ix
                    continue
                same_name = a["name"] and a["name"] == ix["name"]
                other_db = a.get("db") and ix.get("db") and a["db"] != ix["db"]
                if (ix["implicit"] and a["implicit"]) or same_name or other_db:
                    continue          # same index declared twice (drift covers it) or different databases
                self.add("P2", "duplicate-index", ix["file"], ix["line"],
                         f"{t}: index {ix['name'] or '(unnamed)'} ({Path(ix['file']).name}) duplicates "
                         f"{a['name'] or '(unnamed)'} ({Path(a['file']).name}) on ({', '.join(ix['cols'])}) — "
                         f"if both exist in one database, drop one", REF["index"])
            explicit = [x for x in ixs if not x["implicit"] and not x["partial"]]
            reported = set()
            for a in explicit:
                if a["name"] in reported:
                    continue
                for b in ixs:
                    if a is b or b["partial"] or a["method"] != "btree" or b["method"] != "btree":
                        continue
                    if a.get("db") and b.get("db") and a["db"] != b["db"]:
                        continue
                    if len(b["cols"]) > len(a["cols"]) and b["cols"][:len(a["cols"])] == a["cols"]:
                        self.add("P3", "redundant-index", a["file"], a["line"],
                                 f"{t}: index {a['name'] or '(unnamed)'} ({', '.join(a['cols'])}) is a prefix of "
                                 f"{b['name'] or '(unnamed)'} ({', '.join(b['cols'])})", REF["index"])
                        reported.add(a["name"])
                        break
        # low-cardinality, '' defaults
        coltypes = {}
        for (engine, tname), defs in self.tables.items():
            for d in defs:
                for c, v in d.columns.items():
                    coltypes[(tname, c)] = v
        grouped = {}
        for ix in self.indexes:
            if ix["implicit"] or ix["partial"] or len(ix["cols"]) != 1 or ix["method"] != "btree":
                continue
            t, c = short_table(ix["table"]), ix["cols"][0]
            grouped.setdefault((t, c, ix["name"]), []).append(ix)
        for (t, c, name), group in grouped.items():
            ix = group[0]
            locs = ", ".join(f"{Path(g['file']).name}:{g['line']}" for g in group)
            meta = coltypes.get((t, c), {})
            nvals = self.check_values.get((t, c))
            if LOWCARD_NAMES.match(c) or meta.get("type", "").startswith("BOOL") or (nvals and nvals <= 10):
                self.add("P2", "low-cardinality-index", ix["file"], ix["line"],
                         f"{t}: single-column index {name or ''} on low-cardinality column {c} ({locs}) — rarely "
                         f"chosen by the planner, paid on every write; fold into a composite/partial index a real "
                         f"query uses", REF["index"])
            elif meta.get("default") and re.fullmatch(r"''(::\w+)?", meta["default"].strip()):
                self.add("P2", "index-on-empty-default", ix["file"], ix["line"],
                         f"{t}: index {name or ''} on {c} ({locs}), which defaults to '' — one giant duplicate value; "
                         f"partial index WHERE {c} <> '' (better: NULL when absent)", REF["index"])
        # unindexed foreign keys
        for fk in self.fks:
            t = short_table(fk["table"])
            covered = any(short_table(ix["table"]) == t and ix["method"] == "btree"
                          and ix["cols"][:len(fk["cols"])] == fk["cols"] for ix in self.indexes)
            if not covered:
                self.add("P2", "unindexed-fk", fk["file"], fk["line"],
                         f"{t}({', '.join(fk['cols'])}) references {fk['ref']} without an index on the referencing "
                         f"column(s)", REF["constraints"])
        if self.grants_seen and not self.default_privs_seen:
            self.add("P3", "no-default-privileges", "(corpus)", 0,
                     "Per-table GRANTs without ALTER DEFAULT PRIVILEGES — new tables silently miss grants",
                     REF["grants"])
        self.check_vector_ops()
        self.check_memory_fit()


    # -- pgvector operator/opclass agreement ---------------------------------------------------------
    def collect_vector_ops(self, src, file):
        for m in re.finditer(r"([\w.\"]+)\s*(<=>|<->|<#>)", src):
            col = unquote(m.group(1)).split(".")[-1]
            self.vector_ops.append({"col": col, "op": m.group(2), "file": file,
                                    "line": src.count("\n", 0, m.start()) + 1})

    def check_vector_ops(self):
        family = {"<=>": "cosine", "<->": "l2", "<#>": "ip"}
        for ix in self.indexes:
            if ix.get("method") not in ("hnsw", "ivfflat") or not ix["cols"]:
                continue
            col = ix["cols"][0]
            oc = ix.get("opclass") or "vector_l2_ops"
            fam = "cosine" if "cosine" in oc else "ip" if "_ip_" in oc else "l2" if "l2" in oc else None
            for use in self.vector_ops:
                if use["col"] == col and fam and family[use["op"]] != fam:
                    self.add("P1", "pgvector-opclass-mismatch", use["file"], use["line"],
                             f"query orders by {col} {use['op']} ({family[use['op']]}) but index {ix['name']} uses "
                             f"{oc} ({fam}) — the ANN index is never used; match the opclass to the operator",
                             "references/indexing-and-query-tuning.md §4")
                    break

    # -- Go: SQL built with fmt.Sprintf / concatenation -------------------------------------------------
    def check_go_sql_format(self, path, src):
        kw = r"\b(SELECT|INSERT|UPDATE|DELETE|WHERE|FROM|VALUES|ORDER BY)\b"
        sqlkw, sqlkw_any = re.compile(kw), re.compile(kw, re.IGNORECASE)

        # SQL is an upper-case keyword, or two different keywords in any case.
        # One lower-case word is prose: "redacted %d value(s) from the prompt
        # (%s)" in pkg/claudebridge read as SQL.
        def is_sql(f):
            return bool(sqlkw.search(f)) or len({k.lower() for k in sqlkw_any.findall(f)}) >= 2

        risky, numeric = [], []
        for m in re.finditer(r"fmt\.Sprintf\(\s*(`[^`]*`|\"(?:[^\"\\\n]|\\.)*\")", src):
            fmtstr = m.group(1)
            if not is_sql(fmtstr) or DDL_RE.search(fmtstr):
                continue
            verbs = re.findall(r"%[-+# 0]*\d*[svqd]", fmtstr)
            if not verbs:
                continue
            line = src.count("\n", 0, m.start()) + 1
            (risky if any(v.endswith(("s", "v", "q")) for v in verbs) else numeric).append(line)
        if risky:
            self.add("P1", "string-formatted-sql", str(path), risky[0],
                     f"{len(risky)} SQL statement(s) built with fmt.Sprintf and %s/%v (lines {', '.join(map(str, risky))}) "
                     f"— injection risk and no plan reuse; use $n parameters. If the verb only ever carries an "
                     f"allowlisted identifier, record that rationale on the line (ADR-073) and prefer removing the "
                     f"dynamic identifier", "references/access-and-security.md §8")
        if numeric:
            self.add("P3", "string-formatted-sql", str(path), numeric[0],
                     f"{len(numeric)} SQL statement(s) built with fmt.Sprintf %d — prefer $n parameters",
                     "references/access-and-security.md §8")
        for m in re.finditer(r"\"[^\"\n]*\b(WHERE|VALUES|SET)\b[^\"\n]*['=(]\s*\"\s*\+\s*\w", src, re.IGNORECASE):
            self.add("P1", "string-concatenated-sql", str(path), src.count("\n", 0, m.start()) + 1,
                     "SQL assembled by string concatenation — use $n parameters", "references/access-and-security.md §8")

    # -- config files -----------------------------------------------------------------------------------
    def check_pg_hba(self, path):
        self.files_scanned += 1
        nossl, wide172 = [], []
        for n, raw in enumerate(self.read(path).splitlines(), 1):
            line = raw.split("#", 1)[0].strip()
            if not line:
                continue
            parts = line.split()
            kind = parts[0]
            if kind not in ("host", "hostssl", "hostnossl", "local", "hostgssenc", "hostnogssenc"):
                continue
            method = parts[-1] if kind == "local" else (parts[4] if len(parts) > 4 else parts[-1])
            if "=" in method and len(parts) > 4:
                method = parts[4]
            user = parts[2] if len(parts) > 2 else ""
            addr = parts[3] if kind != "local" and len(parts) > 3 else ""
            if kind != "local" and method == "trust":
                self.add("P0", "hba-trust", str(path), n, f"network line with trust auth: {line}", REF["grants"])
            if kind != "local" and addr in ("0.0.0.0/0", "::/0", "all") and method != "reject":
                self.add("P0", "hba-open", str(path), n, f"line accepts any address: {line}",
                         "references/access-and-security.md §6")
            if method in ("md5", "password"):
                self.add("P1", "hba-weak-auth", str(path), n, f"{method} auth — use scram-sha-256: {line}",
                         "references/access-and-security.md §5")
            if method != "reject" and addr.startswith("172.") and addr.endswith("/8"):
                wide172.append(n)
            elif method != "reject" and re.search(r"/([0-7])$", addr):
                self.add("P2", "hba-wide-cidr", str(path), n, f"very wide source network {addr}: {line}",
                         "references/access-and-security.md §6")
            if kind != "local" and user in ("postgres", "unheaded", "all") and method != "reject" and \
                    parts[1] == "all":
                self.add("P1", "hba-superuser-network", str(path), n,
                         f"network login for {user!r} to all databases — superusers/bootstrap users should be local-only",
                         "references/access-and-security.md §6")
            if kind == "host" and method != "reject":
                nossl.append(n)
        if wide172:
            self.add("P1", "hba-wide-cidr", str(path), wide172[0],
                     f"{len(wide172)} line(s) allow 172.0.0.0/8, which includes public address space (private is "
                     f"172.16.0.0/12) — use the exact container/overlay subnets (lines "
                     f"{', '.join(map(str, wide172))})", "references/access-and-security.md §6")
        if nossl:
            self.add("P2", "hba-no-tls", str(path), nossl[0],
                     f"{len(nossl)} 'host' lines allow non-TLS connections — use 'hostssl' once ssl=on",
                     "references/access-and-security.md §6")

    def check_pg_conf(self, path):
        self.files_scanned += 1
        conf, lines = {}, {}
        for n, raw in enumerate(self.read(path).splitlines(), 1):
            line = raw.split("#", 1)[0].strip()
            m = re.match(r"^(\w+)\s*=?\s*'?([^']*)'?$", line)
            if m:
                conf[m.group(1).lower()] = m.group(2).strip()
                lines[m.group(1).lower()] = n
        f = str(path)
        if conf.get("log_duration", "off").lower() in ("on", "true", "1"):
            self.add("P2", "conf-log-duration", f, lines["log_duration"],
                     "log_duration = on logs the duration of every statement — rely on log_min_duration_statement",
                     "references/operations.md §1")
        if conf.get("ssl", "off").lower() not in ("on", "true", "1"):
            self.add("P1", "conf-no-ssl", f, lines.get("ssl", 1), "ssl is not enabled", "references/access-and-security.md §6")
        if "pg_stat_statements" not in conf.get("shared_preload_libraries", ""):
            self.add("P2", "conf-no-pss", f, lines.get("shared_preload_libraries", 1),
                     "pg_stat_statements not preloaded — no workload visibility", "references/operations.md §1")
        if conf.get("archive_mode", "off").lower() not in ("on", "always"):
            self.add("P2", "conf-no-archive", f, lines.get("archive_mode", 1),
                     "archive_mode off — no WAL archiving, no point-in-time recovery", "references/operations.md §4")
        st = conf.get("statement_timeout", "0")
        if st not in ("0", ""):
            self.add("P3", "conf-global-statement-timeout", f, lines["statement_timeout"],
                     f"global statement_timeout = {st} also cancels migrations and maintenance — set it per role",
                     "references/operations.md §1")
        if conf.get("listen_addresses") == "*":
            self.add("P2", "conf-listen-all", f, lines["listen_addresses"], "listen_addresses = '*'",
                     "references/access-and-security.md §6")
        if conf.get("password_encryption", "scram-sha-256").lower() == "md5":
            self.add("P1", "conf-md5", f, lines["password_encryption"], "password_encryption = md5",
                     "references/access-and-security.md §5")
        sb = conf.get("shared_buffers")
        if sb:
            mm = re.match(r"(\d+)\s*(kB|MB|GB)?", sb, re.IGNORECASE)
            if mm:
                v = int(mm.group(1)); u = (mm.group(2) or "kB").upper()
                self.conf_shared_buffers_mb = v / 1024 if u == "KB" else v * 1024 if u == "GB" else v
        if conf.get("random_page_cost") in (None, "4", "4.0"):
            self.add("P3", "conf-random-page-cost", f, lines.get("random_page_cost", 1),
                     "random_page_cost at the spinning-disk default (4.0) — 1.1 on SSD/NVMe", "references/operations.md §1")

    def check_compose(self, path):
        self.files_scanned += 1
        src = self.read(path)
        lines = src.splitlines()
        f = str(path)
        pg_user = None
        for n, line in enumerate(lines, 1):
            m = re.search(r"image:\s*\"?postgres:([\w.\-]+)", line)
            if m:
                self.add("INFO", "compose-pg-image", f, n, f"Postgres image tag {m.group(1)} — confirm it matches the "
                         f"major you test against", "references/operations.md §6")
            m = re.search(r"POSTGRES_USER:\s*\"?([\w-]+)", line)
            if m:
                pg_user = m.group(1)
            if re.search(r"POSTGRES_PASSWORD:\s*\$\{\w+:-[^}]+\}", line):
                self.add("P1", "compose-default-password", f, n,
                         "POSTGRES_PASSWORD falls back to a literal default — fail closed instead (${VAR:?required})",
                         "references/access-and-security.md §5")
            if re.search(r"docker-entrypoint-initdb\.d\s*$", line) and "migrations" in line:
                self.add("P1", "compose-initdb-migrations", f, n,
                         "migrations directory mounted into /docker-entrypoint-initdb.d — every *.sql runs against "
                         "POSTGRES_DB as the superuser, once, on an empty volume only; use the ledger runner",
                         "references/migrations.md §8")
            if re.search(r"(SSLMODE|sslmode)[\"']?\s*[:=]\s*[\"']?disable", line):
                self.add("P2", "compose-sslmode-disable", f, n, "client sslmode=disable",
                         "references/access-and-security.md §6")
        if pg_user:
            for n, line in enumerate(lines, 1):
                m = re.search(r"(WELL_USER|DB_USER|PGUSER|DATABASE_USER):\s*\"?([\w-]+)", line)
                if m and m.group(2) == pg_user:
                    self.add("P0", "compose-app-superuser", f, n,
                             f"service connects as {pg_user!r}, the image's POSTGRES_USER (a superuser) — give each "
                             f"service its own least-privilege role", "references/access-and-security.md §1")
        # memory limit of the postgres service block
        block = re.search(r"\n(\s{2})postgres:\s*\n(.*?)(?=\n\1\S)", src, re.DOTALL)
        if block:
            mm = re.search(r"memory:\s*(\d+)([MG])", block.group(2))
            if mm:
                self.compose_pg_mem_mb = int(mm.group(1)) * (1024 if mm.group(2) == "G" else 1)

    def check_memory_fit(self):
        if self.conf_shared_buffers_mb and self.compose_pg_mem_mb and \
                self.conf_shared_buffers_mb >= 0.5 * self.compose_pg_mem_mb:
            self.add("P1", "memory-mismatch", "(postgresql.conf vs compose)", 0,
                     f"shared_buffers {self.conf_shared_buffers_mb:.0f} MB vs container memory limit "
                     f"{self.compose_pg_mem_mb} MB — size from the container budget (~25%), or the conf is not the one "
                     f"in use", "references/operations.md §1")

    # -- inventory --------------------------------------------------------------------------------------
    def inventory(self):
        out = ["## WELL INVENTORY (static)", "", "| Table | Engine | Definitions (file:line) | Columns |",
               "|---|---|---|---|"]
        for (engine, t), defs in sorted(self.tables.items(), key=lambda kv: kv[0][1]):
            locs = "; ".join(f"{Path(d.file).name}:{d.line}" + (" (code)" if d.origin == "code" else "") for d in defs)
            cols = sorted(set().union(*[set(d.columns) for d in defs]))
            out.append(f"| {t.replace('__dyn__', '<dynamic>')} | {engine} | {locs} | {len(cols)} |")
        out += ["", "| Table | Index | Method | Columns | Partial | File |", "|---|---|---|---|---|---|"]
        for ix in sorted(self.indexes, key=lambda x: (short_table(x["table"]), x["name"] or "")):
            if ix["implicit"]:
                continue
            out.append(f"| {short_table(ix['table'])} | {ix['name'] or '(unnamed)'} | {ix['method']} | "
                       f"{', '.join(ix['cols'])} | {'yes' if ix['partial'] else ''} | {Path(ix['file']).name}:{ix['line']} |")
        return "\n".join(out)

# ----------------------------------------------------------------------------- self-test

SELFTEST = {
    "good.sql": ("""
CREATE TABLE boards (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name text NOT NULL UNIQUE);
CREATE TABLE tasks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    board_id bigint NOT NULL REFERENCES boards(id),
    status text NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','doing','done')),
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz);
CREATE INDEX tasks_board_status_idx ON tasks (board_id, status) WHERE deleted_at IS NULL;
""", set(), {"P0", "P1", "P2"}),
    "risky.sql": ("""
ALTER TABLE tasks ADD COLUMN guid uuid NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE tasks ALTER COLUMN title TYPE varchar(200);
ALTER TABLE tasks ADD CONSTRAINT t_fk FOREIGN KEY (owner_id) REFERENCES users(id);
CREATE INDEX tasks_created_idx ON tasks (created_at);
UPDATE tasks SET owner = 'x';
CREATE ROLE r LOGIN PASSWORD 'hunter2';
CREATE TABLE ev (ts timestamp, payload json, amount double precision, child_ids bigint[]);
CREATE INDEX ev_ts ON ev (ts); CREATE INDEX ev_ts2 ON ev (ts);
GRANT UPDATE, DELETE ON audit_events TO ops_writer;
""", {"volatile-default-rewrite", "type-change-rewrite", "constraint-without-not-valid", "index-not-concurrent",
      "unbatched-dml", "secret-in-sql", "timestamp-no-tz", "json-not-jsonb", "float-money", "array-of-references",
      "duplicate-index", "no-primary-key", "append-only-writable", "no-lock-timeout"}, set()),
    "vector.sql": ("""
BEGIN;
CREATE TABLE aux (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, embedding vector(384) NOT NULL);
CREATE INDEX aux_ivf ON aux USING ivfflat (embedding vector_l2_ops) WITH (lists = 4096);
CREATE INDEX CONCURRENTLY aux_hnsw ON aux USING hnsw (embedding vector_l2_ops);
COMMIT;
SELECT id FROM aux ORDER BY embedding <=> '[1,2,3]' LIMIT 5;
""", {"pgvector-ivfflat-empty", "concurrently-in-tx", "pgvector-opclass-mismatch"}, set()),
    "mysql.sql": ("CREATE TABLE a (id int PRIMARY KEY, k text, INDEX idx_k (k));\nCREATE TABLE b (key varchar(10) PRIMARY KEY);",
                  {"mysql-syntax"}, {"no-primary-key"}),
    "ignore.sql": ("CREATE TABLE c (id bigint PRIMARY KEY, note text DEFAULT 'x'); -- well:ignore nullable-with-default fixture",
                   set(), {"nullable-with-default"}),
    "alter.sql": ("""
SET lock_timeout = '5s';
ALTER TABLE events ADD COLUMN owner_id bigint REFERENCES users(id);
ALTER TABLE events ADD COLUMN src varchar(45), ADD COLUMN seen_at timestamp, ADD COLUMN meta json;
ALTER TABLE events DROP legacy;
""", {"constraint-without-not-valid", "unindexed-fk", "ip-as-text", "timestamp-no-tz", "json-not-jsonb",
      "destructive"}, set()),
    "alter_safe.sql": ("""
SET lock_timeout = '5s';
ALTER TABLE events ADD COLUMN owner_id bigint;
ALTER TABLE events ADD CONSTRAINT events_owner_fk FOREIGN KEY (owner_id) REFERENCES users(id) NOT VALID;
CREATE INDEX CONCURRENTLY events_owner_idx ON events (owner_id);
""", set(), {"constraint-without-not-valid", "unindexed-fk", "P0", "P1"}),
    "outside_commented.sql": ("""
-- ALTER TABLE kanban.tasks ADD COLUMN archived_at timestamptz;
/* CREATE TABLE x (id bigint PRIMARY KEY); */
SELECT 1;
""", set(), {"schema-outside-ledger"}),
    "outside_live.sql": ("CREATE TABLE y (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY);",
                         {"schema-outside-ledger"}, set()),
}


def selftest():
    import tempfile
    failures = 0
    with tempfile.TemporaryDirectory() as d:
        for name, (sql, must, must_not) in SELFTEST.items():
            sub = ("sql",) if name.startswith("outside_") else ("db", "migrations")   # outside_* = not a ledger
            mdir = Path(d, name.replace(".sql", ""), *sub)
            mdir.mkdir(parents=True)
            path = mdir / f"202610090000_{name}"
            path.write_text(sql)
            lint = Linter()
            got = lint.run([str(path)])
            rules = {f.rule for f in got}
            sevs = {f.severity for f in got}
            missing = must - rules
            unexpected = {r for r in must_not if r in rules or r in sevs}
            ok = not missing and not unexpected
            failures += not ok
            print(f"{'ok  ' if ok else 'FAIL'} {name:12} rules={len(rules)}"
                  + (f" missing={sorted(missing)}" if missing else "")
                  + (f" unexpected={sorted(unexpected)}" if unexpected else ""))
    return 1 if failures else 0


# ----------------------------------------------------------------------------- output

def render(findings, fmt, files_scanned, stmts):
    findings = sorted(findings, key=lambda f: (-SEV_RANK[f.severity], f.file, f.line, f.rule))
    counts = {s: sum(1 for f in findings if f.severity == s) for s in reversed(SEVERITIES)}
    if fmt == "json":
        return json.dumps({"schema": "unheaded.erda.well_lint/v1", "files_scanned": files_scanned,
                           "statements_scanned": stmts, "counts": counts,
                           "findings": [asdict(f) for f in findings]}, indent=2)
    if fmt == "md":
        lines = ["## WELL LINT", "",
                 f"Files scanned: {files_scanned} · statements: {stmts} · " +
                 " · ".join(f"{k}: {v}" for k, v in counts.items() if v), "",
                 "| Sev | Rule | Location | Finding |", "|---|---|---|---|"]
        for f in findings:
            loc = f"`{f.file}:{f.line}`" if f.line else f"`{f.file}`"
            msg = f.message.replace("|", "\\|")
            lines.append(f"| {f.severity} | {f.rule} | {loc} | {msg} |")
        return "\n".join(lines)
    out = [f"WELL LINT — files {files_scanned}, statements {stmts} — " +
           ", ".join(f"{k} {v}" for k, v in counts.items() if v)]
    for f in findings:
        loc = f"{f.file}:{f.line}" if f.line else f.file
        out.append(f"[{f.severity:4}] {f.rule:30} {loc}\n        {f.message}" + (f"\n        → {f.ref}" if f.ref else ""))
    return "\n".join(out)


def main(argv=None):
    ap = argparse.ArgumentParser(description="Static lint for PostgreSQL schema, migrations and Well config (Erda).")
    ap.add_argument("paths", nargs="*", help="files or directories (a repo root works)")
    ap.add_argument("--selftest", action="store_true", help="run the built-in fixtures and exit")
    ap.add_argument("--format", choices=["text", "md", "json"], default="text")
    ap.add_argument("--min-severity", choices=SEVERITIES, default="P3")
    ap.add_argument("--fail-on", choices=["P0", "P1", "P2", "P3", "NONE"], default=None,
                    help="exit 1 at/above this severity (default: 2 on P0, 1 on P1)")
    ap.add_argument("--inventory", action="store_true", help="also print the table and index map")
    ap.add_argument("--include-tests", action="store_true", help="also scan *_test.go / tests/")
    ap.add_argument("--no-code", action="store_true", help="only .sql/config files; skip SQL embedded in Go/Python")
    a = ap.parse_args(argv)
    if a.selftest:
        return selftest()
    if not a.paths:
        ap.error("give at least one path (or --selftest)")
    lint = Linter(include_tests=a.include_tests, scan_code=not a.no_code)
    findings = lint.run(a.paths)
    shown = [f for f in findings if SEV_RANK[f.severity] >= SEV_RANK[a.min_severity]]
    print(render(shown, a.format, lint.files_scanned, lint.statements_scanned))
    if a.inventory and a.format != "json":
        print()
        print(lint.inventory())
    if a.fail_on == "NONE":
        return 0
    if a.fail_on:
        return 1 if any(SEV_RANK[f.severity] >= SEV_RANK[a.fail_on] for f in findings) else 0
    if any(f.severity == "P0" for f in findings):
        return 2
    if any(f.severity == "P1" for f in findings):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
