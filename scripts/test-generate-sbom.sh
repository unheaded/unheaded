#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# scripts/generate-sbom.sh writes sbom/SBOM.md, and the committed one says what
# the committed JSON says: the summary may not sit behind its detail files,
# as it did from 2026-03-15 to 2026-11-05.
set -uo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
cp "$REPO_ROOT"/sbom/go-dependencies.json "$REPO_ROOT"/sbom/rust-dependencies.json \
	"$REPO_ROOT"/sbom/python-dependencies.json "$REPO_ROOT"/sbom/javascript-dependencies.json \
	"$REPO_ROOT"/sbom/python-pip-freeze.txt "$T/"
printf '**Generated:** 2026-03-15\n' >"$T/SBOM.md"

fail=0
ok() { # ok DESC COMMAND...
	local desc=$1
	shift
	if "$@"; then printf '[ok] %s\n' "$desc"; else printf '[FAIL] %s\n' "$desc"; fail=1; fi
}

SBOM_DIR="$T" "$REPO_ROOT/scripts/generate-sbom.sh" --report-only >"$T/out" 2>&1
ok "--report-only runs" [ $? -eq 0 ]
ok "a stale SBOM.md is replaced, with today's date" grep -qx "\*\*Generated:\*\* $(date -u +%Y-%m-%d)" "$T/SBOM.md"

go_total="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["summary"]["totalDependencies"])' "$T/go-dependencies.json")"
ok "its Go count is the JSON's" grep -q "^| Go | ${go_total} |" "$T/SBOM.md"

for path in $(grep -oE '`[A-Za-z_./-]+\.(md|json|txt|sh)`' "$T/SBOM.md" | tr -d '`' | sort -u); do
	ok "it names $path, which exists" [ -e "$REPO_ROOT/$path" ]
done

# The verdict: a license nobody read clears nothing.
verdict_for() { # verdict_for GO RUST [PYTHON [JS]]: the report's GPL boundary line
	local d="$T/verdict"
	mkdir -p "$d"
	: >"$d/python-pip-freeze.txt"
	printf '{"summary": {"totalDependencies": 1, "directDependencies": 1, "modules": ["m"]}, "packages": [{"module": "example.invalid/m", "version": "v1", "indirect": false, "license": "%s"}]}\n' "$1" >"$d/go-dependencies.json"
	printf '{"summary": {"totalExternalCrates": 1, "workspaceCrates": ["w"], "lockFiles": ["Cargo.lock"]}, "packages": [{"crate": "c", "version": "1", "license": "%s"}]}\n' "$2" >"$d/rust-dependencies.json"
	printf '{"summary": {"manifests": ["requirements.txt"]%s}, "packages": [%s]}\n' "${PY_SUMMARY-, \"venvRead\": true}" \
		"${PY_ROWS-{\"package\": \"p\", \"version\": \"1\", \"license\": \"${3:-MIT}\"${PY_EXTRA:-}\}}" >"$d/python-dependencies.json"
	printf '{"packages": [{"library": "j", "version": "1", "url": "https://example.invalid/j.js", "integrity": true, "loadedBy": ["a.html"], "license": "%s"}]}\n' "${4:-MIT}" >"$d/javascript-dependencies.json"
	SBOM_DIR="$d" "$REPO_ROOT/scripts/generate-sbom.sh" --report-only >/dev/null 2>&1
	sed -n '/^## GPL boundary$/{n;n;p}' "$d/SBOM.md"
}
ok "every license read, none GPL: PASS" grep -q '^PASS: every license was read' <(verdict_for MIT Apache-2.0)
ok "a GPL Go module: REVIEW, by name and license" grep -q '^REVIEW: .*example.invalid/m v1 (GPL-3.0-only)' <(verdict_for GPL-3.0-only MIT)
ok "a GPL Rust crate: REVIEW, by name and license" grep -q '^REVIEW: .*c 1 (GPL-2.0-only)' <(verdict_for MIT GPL-2.0-only)
ok "a GPL Python package: REVIEW, by name and license" grep -q '^REVIEW: .*p 1 (LGPL-3.0-or-later)' <(verdict_for MIT MIT LGPL-3.0-or-later)
ok "a classifier spelling of the GPL: REVIEW too" grep -q '^REVIEW: .*p 1 (GNU General Public License v3)' <(verdict_for MIT MIT 'GNU General Public License v3')
ok "an Unknown Go module: not PASS, and counted" grep -q '^INCOMPLETE: .*1 of 1 Go modules, 0 of 1 Rust crates, 0 of 1 Python packages and 0 of 1 JavaScript files' <(verdict_for Unknown MIT)
ok "an assumed Rust crate: not PASS, and counted" grep -q '^INCOMPLETE: .*0 of 1 Go modules, 1 of 1 Rust crates' <(verdict_for MIT 'MIT OR Apache-2.0 (assumed)')
ok "an Unknown Python package: not PASS, and counted" grep -q '^INCOMPLETE: .*1 of 1 Python packages' <(verdict_for MIT MIT Unknown)
ok "an Unknown JavaScript file: not PASS, and counted" grep -q '^INCOMPLETE: .*1 of 1 JavaScript files' <(verdict_for MIT MIT MIT Unknown)
ok "GPL and unread together: REVIEW still counts the unread" grep -q '^REVIEW: .*1 of 1 Go modules' <(verdict_for Unknown GPL-2.0-only)
ok "a package nobody read is listed by name" grep -q '^| Go | example.invalid/m | v1 |' <(verdict_for Unknown MIT >/dev/null; cat "$T/verdict/SBOM.md")
ok "a proprietary license: REVIEW, by name, never PASS" grep -q '^REVIEW: .*Proprietary licenses: p 1 (Other/Proprietary License)' <(verdict_for MIT MIT 'Other/Proprietary License')
ok "an NVIDIA license by any spelling: the same" grep -q '^REVIEW: .*Proprietary licenses: p 1 (LicenseRef-NVIDIA-SOFTWARE-LICENSE)' <(verdict_for MIT MIT LicenseRef-NVIDIA-SOFTWARE-LICENSE)
ok "GPL named only in a package's license files: REVIEW, by name" grep -q "^REVIEW: .*own license files, not its license field: p 1 (GPL-3.0)" <(PY_EXTRA=', "licenseFilesName": ["GPL-3.0"]' verdict_for MIT MIT BSD-3-Clause)
ok "a venv not read: never PASS, and it says so" grep -q '^INCOMPLETE: .*The Zhen venv was not read' <(PY_SUMMARY=', "venvRead": false' verdict_for MIT MIT)
ok "a report that does not say the venv was read: the same" grep -q '^INCOMPLETE: .*The Zhen venv was not read' <(PY_SUMMARY='' verdict_for MIT MIT)
ok "a venv not read beside a GPL license: REVIEW still says so" grep -q '^REVIEW: .*The Zhen venv was not read' <(PY_SUMMARY=', "venvRead": false' verdict_for MIT GPL-2.0-only)
ok "no Python packages at all: never PASS, and it says so" grep -q '^INCOMPLETE: .*No Python packages are listed at all' <(PY_ROWS='' verdict_for MIT MIT)

# The committed report: no assumption left, and everything unread or GPL by name.
committed_names_all() {
	python3 - "$REPO_ROOT/sbom" <<'PYEOF'
import json, re, sys

sbom = sys.argv[1]
report = open(f'{sbom}/SBOM.md').read()
verdict = report.split('## GPL boundary')[1].strip().splitlines()[0]
gpl = re.compile(r'GPL|General Public', re.I)
missing, assumed, seen = [], [], 0
for name, key in (('go', 'module'), ('rust', 'crate'), ('python', 'package'), ('javascript', 'library')):
    for p in json.load(open(f'{sbom}/{name}-dependencies.json'))['packages']:
        seen += 1
        lic = p['license']
        if '(assumed)' in lic:
            assumed.append(p[key])
        if (lic == 'Unknown' or 'licenseNotRead' in p) and f'| {p[key]} |' not in report:
            missing.append(p[key])
        if gpl.search(lic) and f'{p[key]} {p["version"]} ({lic})' not in verdict:
            missing.append(p[key])
        if re.search('proprietar', lic, re.I) and f'{p[key]} {p["version"]} ({lic})' not in verdict:
            missing.append(p[key])
        if p.get('licenseFilesName') and not gpl.search(lic) and f'{p[key]} {p["version"]} (' not in verdict:
            missing.append(p[key])
print('assumed:', assumed[:5], 'not named:', missing[:5], 'seen:', seen)
sys.exit(1 if assumed or missing or seen < 100 else 0)
PYEOF
}
ok "the committed report assumes nothing and names every unread, GPL or proprietary license" committed_names_all

# Every manifest the repo tracks is in the detail files, by name.
committed_covers_manifests() {
	python3 - "$REPO_ROOT" <<'PYEOF'
import json, re, subprocess, sys

root = sys.argv[1]
tracked = lambda *spec: subprocess.check_output(['git', '-C', root, 'ls-files', '--', *spec, ':(exclude,glob)**/testdata/**'], text=True).split()
normal = lambda name: re.sub(r'[-_.]+', '-', name).lower()
load = lambda kind: json.load(open(f'{root}/sbom/{kind}-dependencies.json'))
python = {normal(p['package']) for p in load('python')['packages']}
go = {p['module'] for p in load('go')['packages']}
missing = []
manifests = tracked('*requirements*.txt')
for path in manifests:
    for line in open(f'{root}/{path}'):
        m = re.match(r'([A-Za-z0-9][\w.-]*)', line)
        if m and normal(m.group(1)) not in python:
            missing.append(f'{path}: {m.group(1)}')
go_mods = tracked('go.mod', '*/go.mod')
for path in go_mods:
    for m in re.finditer(r'^\s*(?:require\s+)?([a-z0-9.-]+\.[a-z]+/\S+) v\S+', open(f'{root}/{path}').read(), re.M):
        if m.group(1) not in go:
            missing.append(f'{path}: {m.group(1)}')
print('not in sbom/:', missing[:8])
# By name, not by count: a manifest that leaves the index must be seen to.
known = {'.github/ci-python/requirements.txt', 'notebooks/requirements.txt', 'raft/requirements.txt'}
print('manifests gone:', sorted(known - set(manifests)))
sys.exit(1 if missing or not known <= set(manifests) or 'tools/gosec/go.mod' not in go_mods else 0)
PYEOF
}
ok "every tracked requirements file and go.mod is in the detail files" committed_covers_manifests

# THIRD_PARTY.md: every row it keeps is what the detail files say, and nothing
# in it rules for the whole list, because the report does not.
third_party_matches() {
	python3 - "$REPO_ROOT" <<'PYEOF'
import json, re, sys

root = sys.argv[1]
text = open(f'{root}/THIRD_PARTY.md').read()
load = lambda kind: json.load(open(f'{root}/sbom/{kind}-dependencies.json'))['packages']
go, rust, python, js = load('go'), load('rust'), load('python'), load('javascript')

def section(start, stop):
    return text.split(start, 1)[1].split(stop, 1)[0]

def rows(body):
    return [[c.strip() for c in line.strip('|').split('|')] for line in body.splitlines()
            if line.startswith('|') and not re.match(r'\|[ -]*\|', line)][1:]

wrong = []
direct = {p['module']: p for p in go if not p['indirect']}
table = {r[0]: r for r in rows(section('## Go Dependencies', '## Rust Dependencies'))}
wrong += [f'Go {m} missing' for m in direct if m not in table]
wrong += [f'Go {m} not direct' for m in table if m not in direct]
wrong += [f'Go {m}: {table[m][1:3]}' for m in table if m in direct
          and table[m][1:3] != [direct[m]['version'], direct[m]['license']]]

crates = {(p['crate'], p['version']): p['license'] for p in rust}
table = rows(section('## Rust Dependencies', '## JavaScript'))
wrong += [f'Rust {r[:3]}' for r in table if crates.get((r[0], r[1])) != r[2]]
if not table:
    wrong.append('no Rust rows')

packages = {(p['package'].lower(), p['version']): p for p in python}
table = {r[0].lower(): r for r in rows(section('## Python Dependencies', '## External Runtime'))}
declared = [l.split('==')[0].strip().lower() for l in open(f'{root}/raft/requirements.txt')
            if l.strip() and not l.startswith('#')]
wrong += [f'Python {n} missing' for n in declared if n not in table]
wrong += [f'Python {n} not declared' for n in table if n not in declared]
wrong += [f'Python {n}: {table[n][1:3]}' for n in table if n in declared and
          packages.get((n, table[n][1]), {}).get('license') != table[n][2]]

libraries = {p['library']: p['version'] for p in js if p['version']}
table = rows(section('at runtime from CDNs', 'Google Fonts'))
wrong += [f'CDN {r[:2]}' for r in table if libraries.get(r[0]) != r[1]]
if len(table) != len(libraries):
    wrong.append('CDN row count')

for claim in ('All Go dependencies use permissive', 'All dependencies are compatible', 'Compatible with GPL-3.0?',
              'doom1.wad', 'internal/bpf', 'docs/compliance/SBOM.md'):
    if claim in text:
        wrong.append(f'still says {claim!r}')
for path in ('crates/zhend/THIRD_PARTY.md', 'crates/zhend/README.md', 'crates/zhend/SECURITY.md',
             'LICENSES/sbom-reports/SBOM-CONSOLIDATED.md'):
    other = open(f'{root}/{path}').read()
    for claim in ('All dependencies are', 'All Go dependencies use', 'No copyleft dependency conflicts', 'no copyleft conflicts'):
        if claim in other:
            wrong.append(f'{path} still says {claim!r}')
if 'Superseded' not in open(f'{root}/LICENSES/sbom-reports/SBOM-CONSOLIDATED.md').read():
    wrong.append('SBOM-CONSOLIDATED.md does not say it is superseded')
if 'sbom/*-dependencies.json' in open(f'{root}/wiki/SBOM.md').read():
    wrong.append('wiki/SBOM.md offers cmd/vulnmatch files it cannot read')
updated = re.search(r'\*\*Last updated:\*\* (\d{4}-\d\d-\d\d)', text)
generated = re.search(r'\*\*Generated:\*\* (\d{4}-\d\d-\d\d)', open(f'{root}/sbom/SBOM.md').read())
if not updated or not generated or updated.group(1) < generated.group(1):
    wrong.append('its date is older than the report it matches')
print(wrong[:12])
sys.exit(1 if wrong else 0)
PYEOF
}
ok "THIRD_PARTY.md matches the detail files and rules on nothing the report does not" third_party_matches

# Reading: fixture trees, built here, so this needs no cache and no network.
F="$T/fixture"
lib="$REPO_ROOT/scripts/lib/sbom_licenses.py"
mit='Permission is hereby granted, free of charge, to any person obtaining a copy of this software. The above copyright notice and this permission notice shall be included in all copies.'
apache='Apache License Version 2.0, January 2004. TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION'
bsd_head='Redistribution and use in source and binary forms, with or without modification, are permitted. Redistributions of source code must retain the above copyright notice.'
bsd3="$bsd_head Neither the names of the authors nor the names of the contributors may be used to endorse or promote products derived from this software."
mpl='Mozilla Public License Version 2.0. "Secondary License" means either the GNU General Public License, Version 2.0, the GNU Lesser General Public License, Version 2.1, the GNU Affero General Public License, Version 3.0, or any later versions of those licenses.'
gpl='GNU GENERAL PUBLIC LICENSE Version 3, 29 June 2007'
mkmod() { # mkmod DIR LICENSE_TEXT [FILE]
	mkdir -p "$1"
	printf '%s\n' "$2" >"$1/${3:-LICENSE}"
}
mkmod "$F/mod/e.invalid/mit@v1" "$mit"
mkmod "$F/mod/e.invalid/mit@v1" 'package m // says nothing a license says' copyright_test.go
mkmod "$F/mod/e.invalid/pd@v1" 'All of the code has been dedicated to the public domain by the authors.'
mkmod "$F/mod/e.invalid/bsd2@v1" "$bsd_head"
mkmod "$F/mod/e.invalid/bsd3@v1" "$bsd3" LICENSE.txt
mkmod "$F/mod/e.invalid/two@v1" "$mit" LICENSE-MIT
mkmod "$F/mod/e.invalid/two@v1" "$apache" LICENSE-APACHE
mkmod "$F/mod/e.invalid/mpl@v1" "$mpl"
mkmod "$F/mod/e.invalid/gpl@v1" "$gpl" COPYING
mkmod "$F/mod/e.invalid/odd@v1" "Use this as you like, says the author."
mkmod "$F/mod/e.invalid/part@v1" "$mit" LICENSE
mkmod "$F/mod/e.invalid/part@v1" "Some other words." LICENSE.docs
mkmod "$F/mod/e.invalid/tool-only@v2" "$apache"
mkdir -p "$F/mod/e.invalid/bare@v1"
mkmod "$F/ziproot/e.invalid/!zipped@v1" "$mit"
mkdir -p "$F/mod/cache/download/e.invalid/!zipped/@v"
python3 - "$F" <<'PYEOF'
import sys, zipfile
f = sys.argv[1]
with zipfile.ZipFile(f'{f}/mod/cache/download/e.invalid/!zipped/@v/v1.zip', 'w') as z:
    z.write(f'{f}/ziproot/e.invalid/!zipped@v1/LICENSE', 'e.invalid/Zipped@v1/LICENSE')
PYEOF
{
	printf '{"Path": "unheaded", "Main": true}\n'
	for m in mit pd bsd2 bsd3 two mpl gpl odd part bare; do
		printf '{"Path": "e.invalid/%s", "Version": "v1", "Dir": "%s"}\n' "$m" "$F/mod/e.invalid/$m@v1"
	done
	printf '{"Path": "e.invalid/Zipped", "Version": "v1"}\n{"Path": "e.invalid/absent", "Version": "v1", "Indirect": true}\n'
} >"$F/go.json"
printf 'e.invalid/mit\n' >"$F/built.txt"
cat >"$F/gomod-edit.json" <<'EOF'
{"Module": {"Path": "unheaded/tools/x"}, "Require": [
	{"Path": "e.invalid/mit", "Version": "v1", "Indirect": true},
	{"Path": "e.invalid/tool-only", "Version": "v2", "Indirect": true},
	{"Path": "e.invalid/tool-absent", "Version": "v2", "Indirect": true}]}
EOF
printf '{"Module": {"Path": "a/fixture"}}\n' >"$F/gomod-empty.json"
python3 "$lib" go 2026-01-01 "$F/go.json" "$F/mod" "$F/built.txt" "$F/gomod-edit.json" "$F/gomod-empty.json" >"$F/go-out.json" 2>/dev/null
ok "the Go reader runs on the fixture" [ $? -eq 0 ]
go_license() { # go_license MODULE WANT
	python3 - "$F/go-out.json" "$1" "$2" <<'PYEOF'
import json, sys
got = {p['module']: p['license'] for p in json.load(open(sys.argv[1]))['packages']}[sys.argv[2]]
if got != sys.argv[3]:
    print(f'  got {got!r}, want {sys.argv[3]!r}')
    sys.exit(1)
PYEOF
}
ok "Go: an MIT file reads MIT" go_license e.invalid/mit MIT
ok "Go: two clauses read BSD-2-Clause" go_license e.invalid/bsd2 BSD-2-Clause
ok "Go: the 'names of the authors' wording reads BSD-3-Clause" go_license e.invalid/bsd3 BSD-3-Clause
ok "Go: two license files name both" go_license e.invalid/two 'Apache-2.0 AND MIT'
ok "Go: MPL's Secondary License clause is not a GPL read" go_license e.invalid/mpl MPL-2.0
ok "Go: GPL text reads GPL, never cleared" go_license e.invalid/gpl GPL-3.0
ok "Go: a file nobody recognises stays Unknown" go_license e.invalid/odd Unknown
ok "Go: a public domain dedication reads as one" go_license e.invalid/pd LicenseRef-public-domain
ok "Go: one unrecognised file of two keeps what was read" go_license e.invalid/part MIT
ok "Go: and says which file was not" grep -q '"licenseNotRead": "LICENSE.docs not recognised"' "$F/go-out.json"
ok "Go: no license file stays Unknown" go_license e.invalid/bare Unknown
ok "Go: a cached zip is read, capital path escaped" go_license e.invalid/Zipped MIT
ok "Go: a module with no source here stays Unknown" go_license e.invalid/absent Unknown
ok "Go: the reason an unread module was not read is kept" grep -q '"licenseNotRead": "no source in the module cache"' "$F/go-out.json"
ok "Go: a second go.mod's module is read" go_license e.invalid/tool-only Apache-2.0
ok "Go: one it could not read stays Unknown" go_license e.invalid/tool-absent Unknown
go_modules_read() {
	python3 - "$F/go-out.json" <<'PYEOF'
import json, sys
d = json.load(open(sys.argv[1]))
rows = {p['module']: p for p in d['packages']}
ok = (d['summary']['modules'] == ['unheaded', 'unheaded/tools/x']
      and sum(p['module'] == 'e.invalid/mit' for p in d['packages']) == 1
      and rows['e.invalid/mit']['foundIn'] == ['unheaded', 'unheaded/tools/x']
      and rows['e.invalid/mit']['indirect'] is False
      and rows['e.invalid/tool-only']['indirect'] is True)
sys.exit(0 if ok else 1)
PYEOF
}
ok "Go: a module two go.mods need is one row, direct if either says so" go_modules_read
ok "Go: a module compiled in is marked so" python3 -c 'import json, sys; p = {x["module"]: x for x in json.load(open(sys.argv[1]))["packages"]}; sys.exit(0 if p["e.invalid/mit"]["compiled"] and not p["e.invalid/absent"]["compiled"] else 1)' "$F/go-out.json"

C="$F/cargo"
mkdir -p "$C/registry/src/index-1/from-src-1.0.0" "$C/registry/src/index-1/no-field-1.0.0" "$C/registry/src/index-1/via-git-1.0.0" "$C/registry/cache/index-1" "$F/crate/from-tar-1.0.0" "$F/proj/testdata"
printf '[package]\nname = "from-src"\nlicense = "MIT/Apache-2.0"\n' >"$C/registry/src/index-1/from-src-1.0.0/Cargo.toml"
printf '[package]\nname = "no-field"\n' >"$C/registry/src/index-1/no-field-1.0.0/Cargo.toml"
printf '[package]\nname = "via-git"\nlicense = "MIT"\n' >"$C/registry/src/index-1/via-git-1.0.0/Cargo.toml"
printf '[package]\nname = "from-tar"\nlicense = "Zlib"\n' >"$F/crate/from-tar-1.0.0/Cargo.toml"
tar -czf "$C/registry/cache/index-1/from-tar-1.0.0.crate" -C "$F/crate" from-tar-1.0.0
{
	for c in from-src from-tar no-field absent; do
		printf '[[package]]\nname = "%s"\nversion = "1.0.0"\nsource = "registry+https://example.invalid/index"\n\n' "$c"
	done
	printf '[[package]]\nname = "via-git"\nversion = "1.0.0"\nsource = "git+https://example.invalid/r#abc"\n\n[[package]]\nname = "ours"\nversion = "0.1.0"\n'
} >"$F/proj/Cargo.lock"
printf '[[package]]\nname = "fixture-crate"\nversion = "1.0.0"\n\n[[package]]\nname = "fixture-dep"\nversion = "1.0.0"\nsource = "registry+https://example.invalid/index"\n' >"$F/proj/testdata/Cargo.lock"
python3 "$lib" rust 2026-01-01 "$F/proj" "$C" >"$F/rust-out.json" 2>/dev/null
ok "the Rust reader runs on the fixture" [ $? -eq 0 ]
rust_license() { # rust_license CRATE WANT
	python3 - "$F/rust-out.json" "$1" "$2" <<'PYEOF'
import json, sys
got = {p['crate']: p['license'] for p in json.load(open(sys.argv[1]))['packages']}[sys.argv[2]]
if got != sys.argv[3]:
    print(f'  got {got!r}, want {sys.argv[3]!r}')
    sys.exit(1)
PYEOF
}
ok "Rust: the manifest's own words, as written" rust_license from-src MIT/Apache-2.0
ok "Rust: a cached .crate is read" rust_license from-tar Zlib
ok "Rust: no license field stays Unknown" rust_license no-field Unknown
ok "Rust: a crate not on this host stays Unknown, never assumed" rust_license absent Unknown
ok "Rust: a git crate is not read from the registry" rust_license via-git Unknown
rust_lists() {
	python3 - "$F/rust-out.json" <<'PYEOF'
import json, sys
d = json.load(open(sys.argv[1]))
crates = {p['crate'] for p in d['packages']}
sys.exit(0 if (d['summary']['workspaceCrates'] == ['ours'] and 'ours' not in crates
               and crates == {'from-src', 'from-tar', 'no-field', 'absent', 'via-git'}
               and d['summary']['lockFiles'] == ['Cargo.lock']) else 1)
PYEOF
}
ok "Rust: workspace crates stay out of the third-party list, and testdata/ out of both" rust_lists

S="$F/site"
S2="$F/site2"
mkdist() { # mkdist NAME VERSION METADATA_LINE...
	local n=$1 v=$2 site=${SITE:-$S}
	shift 2
	mkdir -p "$site/$n-$v.dist-info"
	{
		printf 'Metadata-Version: 2.4\nName: %s\nVersion: %s\n' "$n" "$v"
		[ $# -eq 0 ] || printf '%s\n' "$@"
	} >"$site/$n-$v.dist-info/METADATA"
}
long_odd="$(printf 'These words name no license anyone knows of. %.0s' {1..14})"
mkdist expr_pkg 1.0 'License-Expression: BSD-3-Clause' 'Classifier: License :: OSI Approved :: MIT License'
mkdist classified 1.0 'License: UNKNOWN' 'Classifier: License :: OSI Approved :: GNU Lesser General Public License v3 (LGPLv3)' 'Classifier: Topic :: Software'
mkdist short_field 1.0 'License: Apache 2.0'
mkdist in_file 1.0 'License-File: LICENSE'
mkmod "$S/in_file-1.0.dist-info/licenses" "$mit"
mkdist one_line 1.0 "License: Apache-2.0 AND $(printf 'LicenseRef-scancode-x%.0s AND ' 1 2 3 4 5 6) MIT"
mkdist over_limit 1.0 "License: Use it freely. $long_odd $mit"
mkdist folded 1.0 'License: Permission is hereby granted, free of charge, to any person obtaining a copy of this' \
	'  software. The above copyright notice and this permission notice shall be included in all copies.'
mkdist long_odd 1.0 "License: $long_odd"
mkdist wrong_version 2.0 'License-Expression: MIT'
mkdist blank 1.0
mkdist bundles 1.0 'License-Expression: BSD-3-Clause'
mkmod "$S/bundles-1.0.dist-info/licenses" "$gpl" LICENSE.txt
mkdist offers 1.0 'Classifier: License :: OSI Approved :: MIT License'
mkmod "$S/offers-1.0.dist-info" "$gpl" COPYING.gpl
mkdist vendored 1.0 'License-Expression: MIT'
mkmod "$S/vendored/_lib/zzz" "$gpl" COPYING
printf 'vendored/__init__.py,,\nvendored/_lib/zzz/COPYING,,\n../escaped/LICENSE,,\nvendored-1.0.dist-info/RECORD,,\n' >"$S/vendored-1.0.dist-info/RECORD"
mkmod "$F/escaped" 'GNU AFFERO GENERAL PUBLIC LICENSE Version 3'
mkdist word_only 1.0 'License-Expression: MIT'
mkmod "$S/word_only-1.0.dist-info" 'Parts of this are under the GPLv2.' LICENSE.notice
mkdist mpl_only 1.0 'License-Expression: MPL-2.0'
mkmod "$S/mpl_only-1.0.dist-info" "$mpl"
SITE="$S2" mkdist pinned_elsewhere 3.0 'License-Expression: GPL-3.0-or-later'
SITE="$S2" mkdist wrong_version 3.0 'License-Expression: Apache-2.0'
mkdist ranged 1.0 'License-Expression: MIT'
cat >"$F/freeze.txt" <<'EOF'
# a comment
expr-pkg==1.0
classified==1.0
short-field==1.0
one-line==1.0
over-limit==1.0
folded==1.0
long-odd==1.0
in_file==1.0
wrong-version==1.0
blank==1.0
absent==1.0
direct @ file:///x/direct.whl
bundles==1.0
offers==1.0
vendored==1.0
word-only==1.0
mpl-only==1.0
EOF
mkdir -p "$F/pyroot/ci"
cat >"$F/pyroot/ci/requirements.txt" <<'EOF'
# a comment naming nothing==1.0
pinned-elsewhere==3.0 \
    --hash=sha256:aaaa \
    --hash=sha256:bbbb
    # via -r requirements.in
extra-pkg[socks]==9.9 ; python_version > "3.0"
ranged>=1.0,<2.0
bare
wrong-version==3.0
expr_pkg==1.0
-r other.txt
--no-binary :all:
EOF
printf 'ci/requirements.txt\n' >"$F/manifests.txt"
python3 "$lib" python 2026-01-01 "$F/freeze.txt" "$F/pyroot" "$F/manifests.txt" "$S" "$S2" >"$F/py-out.json" 2>/dev/null
ok "the Python reader runs on the fixture" [ $? -eq 0 ]
py_field() { # py_field PACKAGE VERSION FIELD WANT
	python3 - "$F/py-out.json" "$@" <<'PYEOF'
import json, sys
_, path, name, version, field, want = sys.argv
rows = [p for p in json.load(open(path))['packages'] if p['package'] == name and p['version'] == version]
got = json.dumps(rows[0].get(field)) if len(rows) == 1 else f'{len(rows)} rows'
if got != want:
    print(f'  got {got}, want {want}')
    sys.exit(1)
PYEOF
}
py_license() { # py_license PACKAGE WANT
	python3 - "$F/py-out.json" "$1" "$2" <<'PYEOF'
import json, sys
got = {p['package']: p['license'] for p in json.load(open(sys.argv[1]))['packages']}[sys.argv[2]]
if got != sys.argv[3]:
    print(f'  got {got!r}, want {sys.argv[3]!r}')
    sys.exit(1)
PYEOF
}
ok "Python: License-Expression wins" py_license expr-pkg BSD-3-Clause
ok "Python: classifier text, as written" py_license classified 'GNU Lesser General Public License v3 (LGPLv3)'
ok "Python: a short License field" py_license short-field 'Apache 2.0'
ok "Python: a long one-line License field under the limit, as written" py_field one-line 1.0 license "\"Apache-2.0 AND $(printf 'LicenseRef-scancode-x%.0s AND ' 1 2 3 4 5 6) MIT\""
ok "Python: a field over the limit is read as license text" py_license over-limit MIT
ok "Python: a field over several lines is read as license text" py_license folded MIT
ok "Python: a long field nobody recognises stays Unknown" py_license long-odd Unknown
ok "Python: a license file in the dist-info" py_license in_file MIT
ok "Python: another version's metadata clears nothing" py_field wrong-version 1.0 license '"Unknown"'
ok "Python: no license data stays Unknown" py_license blank Unknown
ok "Python: not installed stays Unknown" py_license absent Unknown
ok "Python: a direct-URL line is still listed" py_license direct Unknown
ok "Python: metadata stays the license field" py_license bundles BSD-3-Clause
ok "Python: GPL text in a license file is named beside it" py_field bundles 1.0 licenseFilesName '["GPL-3.0"]'
ok "Python: so is a second license file the metadata does not mention" py_field offers 1.0 licenseFilesName '["GPL-3.0"]'
ok "Python: so is one the RECORD lists outside the dist-info, and not one outside the venv" py_field vendored 1.0 licenseFilesName '["GPL-3.0"]'
ok "Python: so is a file that only says GPLv2" py_field word-only 1.0 licenseFilesName '["GPL"]'
ok "Python: MPL's Secondary License clause names nothing" py_field mpl-only 1.0 licenseFilesName 'null'
ok "Python: a manifest line is read from any venv given, at its version" py_field pinned-elsewhere 3.0 license '"GPL-3.0-or-later"'
ok "Python: continuation lines and hashes are not packages" python3 -c 'import json, sys; sys.exit(1 if any("sha256" in p["package"] or "hash" in p["version"] or p["package"].startswith("--") or p["package"] == "nothing" for p in json.load(open(sys.argv[1]))["packages"]) else 0)' "$F/py-out.json"
ok "Python: an include nothing follows is listed, not lost" py_field '-r other.txt' '' licenseNotRead '"not installed in a venv read"'
ok "Python: extras and markers leave the pin" py_field extra-pkg 9.9 licenseNotRead '"not installed in a venv read"'
ok "Python: a range names no version, so it is not read" py_field ranged '>=1.0,<2.0' license '"Unknown"'
ok "Python: and it says why" py_field ranged '>=1.0,<2.0' licenseNotRead '"no exact pin; the venvs read hold 1.0"'
ok "Python: a bare name likewise" py_field bare '' licenseNotRead '"not installed in a venv read"'
ok "Python: two versions of a package are two rows" py_field wrong-version 3.0 license '"Apache-2.0"'
ok "Python: a package in both is one row saying so" py_field expr-pkg 1.0 found_in '["sbom/python-pip-freeze.txt", "ci/requirements.txt"]'
ok "Python: the manifests and the venv are on record" python3 -c 'import json, sys; s = json.load(open(sys.argv[1]))["summary"]; sys.exit(0 if s["manifests"] == ["ci/requirements.txt"] and s["venvRead"] is True else 1)' "$F/py-out.json"
python3 "$lib" python 2026-01-01 "$F/freeze.txt" "$F/pyroot" "$F/manifests.txt" "$F/no-such-site" >"$F/py-none.json" 2>/dev/null
ok "Python: no venv at all is on record too" python3 -c 'import json, sys; d = json.load(open(sys.argv[1])); sys.exit(0 if d["summary"]["venvRead"] is False and all(p["license"] == "Unknown" for p in d["packages"]) else 1)' "$F/py-none.json"

W="$F/web"
mkdir -p "$W"
cat >"$W/a.html" <<'EOF'
<link rel="preconnect" href="https://fonts.example.invalid">
<link rel="stylesheet" href="https://fonts.example.invalid/css2?family=X">
<link rel="stylesheet" href="https://cdn.example.invalid/npm/lib@1.2.3/lib.css" integrity="sha384-AAAA">
<script src="https://cdn.example.invalid/npm/lib@1.2.3/lib.js" integrity="sha384-AAAA"></script>
<script src="https://cdn.example.invalid/other-4.5.6/other.js"></script>
<script src="/local.js"></script>
EOF
printf 'a.html\n' >"$F/pages.txt"
python3 "$lib" javascript 2026-01-01 "$W" "$F/pages.txt" >"$F/js-out.json" 2>/dev/null
ok "the JavaScript reader runs on the fixture" [ $? -eq 0 ]
js_check() {
	python3 - "$F/js-out.json" <<'PYEOF'
import json, sys
pk = json.load(open(sys.argv[1]))['packages']
rows = {(p['library'], p['version'], p['integrity'], p['license']) for p in pk}
want = {('lib', '1.2.3', True, 'Unknown'), ('other', '4.5.6', False, 'Unknown'), ('fonts.example.invalid', '', False, 'Unknown')}
if rows != want or any(p['loadedBy'] != ['a.html'] for p in pk):
    print(f'  got {sorted(rows)}')
    sys.exit(1)
PYEOF
}
ok "JavaScript: each CDN file by library, version and integrity, license never guessed" js_check

# A failed go list stops the run: an empty module list is not a clean SBOM.
mkdir -p "$F/nogo" "$F/nogo-out"
printf '#!/bin/sh\nexit 1\n' >"$F/nogo/go"
chmod +x "$F/nogo/go"
PATH="$F/nogo:$PATH" SBOM_DIR="$F/nogo-out" "$REPO_ROOT/scripts/generate-sbom.sh" >/dev/null 2>&1
ok "a failed go list fails the run" [ $? -ne 0 ]
ok "and leaves no SBOM.md behind" [ ! -e "$F/nogo-out/SBOM.md" ]

# A run on a host with no Zhen venv: it still finishes, for Jenkins and make, but
# the freeze stays, its packages stay named, and nothing reads as covered.
mkdir -p "$F/quietgo" "$F/novenv-out"
cat >"$F/quietgo/go" <<'EOF'
#!/bin/sh
[ "$1" = mod ] && echo '{"Module": {"Path": "example.invalid/tool"}, "Require": [{"Path": "e.invalid/from-go-mod", "Version": "v1", "Indirect": true}]}'
exit 0
EOF
chmod +x "$F/quietgo/go"
printf 'kept-pkg==1.0\n' >"$F/novenv-out/python-pip-freeze.txt"
PATH="$F/quietgo:$PATH" PYTHON_VENV="$F/no-such-venv" SBOM_DIR="$F/novenv-out" CARGO_HOME="$C" "$REPO_ROOT/scripts/generate-sbom.sh" >"$F/novenv.log" 2>&1
ok "a run with no venv finishes" [ $? -eq 0 ]
ok "and keeps the last freeze" grep -qx 'kept-pkg==1.0' "$F/novenv-out/python-pip-freeze.txt"
ok "and names what the freeze lists as not read" grep -q '^| Python | kept-pkg | 1.0 | not installed in a venv read |' "$F/novenv-out/SBOM.md"
ok "and says the venv was not read" grep -q 'The Zhen venv was not read' "$F/novenv-out/SBOM.md"
ok "and has no PASS" sh -c '! grep -q "^PASS" "$1"' sh "$F/novenv-out/SBOM.md"
ok "and lists the tracked manifests all the same" grep -q '^Python manifests read: .*`raft/requirements.txt`' "$F/novenv-out/SBOM.md"
ok "and reads every other tracked go.mod" grep -q '"module": "e.invalid/from-go-mod"' "$F/novenv-out/go-dependencies.json"
mkdir -p "$F/bare-out"
PATH="$F/quietgo:$PATH" PYTHON_VENV="$F/no-such-venv" SBOM_DIR="$F/bare-out" CARGO_HOME="$C" "$REPO_ROOT/scripts/generate-sbom.sh" >/dev/null 2>&1
ok "with no earlier freeze either, the file says why it is empty" grep -q '^# No Python virtualenv' "$F/bare-out/python-pip-freeze.txt"

ok "it lists the workspace crates and lock files the JSON holds" grep -q '^Rust lock files read: `' "$T/SBOM.md"

strip_date() { grep -v '^\*\*Generated:\*\*' "$1"; }
ok "the committed SBOM.md matches its detail files (scripts/generate-sbom.sh --report-only)" \
	cmp -s <(strip_date "$REPO_ROOT/sbom/SBOM.md") <(strip_date "$T/SBOM.md")

ok "the full run writes it too" grep -qE '^write_report$' "$REPO_ROOT/scripts/generate-sbom.sh"

# A version bump fails until sbom/ is regenerated (Stevie, 2026-11-06). Only the
# full run writes sbom/VERSION, so --report-only cannot clear the gate.
V="$T/version-tree"
mkdir -p "$V/scripts" "$V/debian" "$V/sbom"
cp "$REPO_ROOT/scripts/check-sbom-version.sh" "$V/scripts/" 2>/dev/null
printf 'unheaded (1.2.0~alpha) unheaded; urgency=medium\n' >"$V/debian/changelog"
printf '    VERSION="1.2.0~alpha+$(date +%%s)"\n' >"$V/scripts/build-all-debs.sh"
printf '1.2.0~alpha\n' >"$V/sbom/VERSION"
ok "the version gate passes when sbom/ was made for this version" "$V/scripts/check-sbom-version.sh"
ok "and for the tag of that version" "$V/scripts/check-sbom-version.sh" v1.2.0-alpha
not_ok() { # not_ok DESC COMMAND...
	local desc=$1
	shift
	if "$@" >/dev/null 2>&1; then printf '[FAIL] %s\n' "$desc"; fail=1; else printf '[ok] %s\n' "$desc"; fi
}
not_ok "a tag for another version fails" "$V/scripts/check-sbom-version.sh" v1.3.0
not_ok "a tag that is not one fails" "$V/scripts/check-sbom-version.sh" 1.2.0-alpha
printf 'unheaded (1.3.0) unheaded; urgency=medium\n' >"$V/debian/changelog"
not_ok "a changelog bump alone fails" "$V/scripts/check-sbom-version.sh"
printf 'unheaded (1.2.0~alpha) unheaded; urgency=medium\n' >"$V/debian/changelog"
printf '    VERSION="1.3.0+$(date +%%s)"\n' >"$V/scripts/build-all-debs.sh"
not_ok "a bump of the debs' default alone fails" "$V/scripts/check-sbom-version.sh"
printf '    VERSION="1.2.0~alpha+$(date +%%s)"\n' >"$V/scripts/build-all-debs.sh"
rm "$V/sbom/VERSION"
not_ok "no sbom/VERSION fails" "$V/scripts/check-sbom-version.sh"
ok "the committed sbom/ is current for this version" "$REPO_ROOT/scripts/check-sbom-version.sh"
ok "--report-only does not stamp the version" sh -c '! grep -q "VERSION" "$1"' sh <(sed -n '/--report-only" \]/,/^fi$/p' "$REPO_ROOT/scripts/generate-sbom.sh")
ok "the full run stamps it" grep -qx "$(sed -nE '1s/^unheaded \(([^)]+)\) .*/\1/p' "$REPO_ROOT/debian/changelog")" "$F/bare-out/VERSION"
ok "the release refuses to start without it" grep -q 'scripts/check-sbom-version.sh "$GITHUB_REF_NAME"' "$REPO_ROOT/.github/workflows/release.yml"

[ "$fail" -eq 0 ] || exit 1
echo "[PASS] sbom/SBOM.md is generated and current"
