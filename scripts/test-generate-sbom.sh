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
	printf '{"summary": {"totalDependencies": 1, "directDependencies": 1}, "packages": [{"module": "example.invalid/m", "version": "v1", "indirect": false, "license": "%s"}]}\n' "$1" >"$d/go-dependencies.json"
	printf '{"summary": {"totalExternalCrates": 1, "workspaceCrates": ["w"], "lockFiles": ["Cargo.lock"]}, "packages": [{"crate": "c", "version": "1", "license": "%s"}]}\n' "$2" >"$d/rust-dependencies.json"
	printf '{"packages": [{"package": "p", "version": "1", "license": "%s"}]}\n' "${3:-MIT}" >"$d/python-dependencies.json"
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
print('assumed:', assumed[:5], 'not named:', missing[:5], 'seen:', seen)
sys.exit(1 if assumed or missing or seen < 100 else 0)
PYEOF
}
ok "the committed report assumes nothing and names every unread or GPL license" committed_names_all

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
python3 "$lib" go 2026-01-01 "$F/go.json" "$F/mod" "$F/built.txt" >"$F/go-out.json" 2>/dev/null
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
ok "Go: a module compiled in is marked so" python3 -c 'import json, sys; p = {x["module"]: x for x in json.load(open(sys.argv[1]))["packages"]}; sys.exit(0 if p["e.invalid/mit"]["compiled"] and not p["e.invalid/absent"]["compiled"] else 1)' "$F/go-out.json"

C="$F/cargo"
mkdir -p "$C/registry/src/index-1/from-src-1.0.0" "$C/registry/src/index-1/no-field-1.0.0" "$C/registry/cache/index-1" "$F/crate/from-tar-1.0.0" "$F/proj"
printf '[package]\nname = "from-src"\nlicense = "MIT/Apache-2.0"\n' >"$C/registry/src/index-1/from-src-1.0.0/Cargo.toml"
printf '[package]\nname = "no-field"\n' >"$C/registry/src/index-1/no-field-1.0.0/Cargo.toml"
printf '[package]\nname = "from-tar"\nlicense = "Zlib"\n' >"$F/crate/from-tar-1.0.0/Cargo.toml"
tar -czf "$C/registry/cache/index-1/from-tar-1.0.0.crate" -C "$F/crate" from-tar-1.0.0
{
	for c in from-src from-tar no-field absent; do
		printf '[[package]]\nname = "%s"\nversion = "1.0.0"\nsource = "registry+https://example.invalid/index"\n\n' "$c"
	done
	printf '[[package]]\nname = "via-git"\nversion = "1.0.0"\nsource = "git+https://example.invalid/r#abc"\n\n[[package]]\nname = "ours"\nversion = "0.1.0"\n'
} >"$F/proj/Cargo.lock"
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
ok "Rust: workspace crates stay out of the third-party list" grep -q '"ours"' "$F/rust-out.json"

S="$F/site"
mkdist() { # mkdist NAME VERSION METADATA_LINE...
	local n=$1 v=$2
	shift 2
	mkdir -p "$S/$n-$v.dist-info"
	{
		printf 'Metadata-Version: 2.4\nName: %s\nVersion: %s\n' "$n" "$v"
		printf '%s\n' "$@"
	} >"$S/$n-$v.dist-info/METADATA"
}
mkdist expr_pkg 1.0 'License-Expression: BSD-3-Clause' 'Classifier: License :: OSI Approved :: MIT License'
mkdist classified 1.0 'License: UNKNOWN' 'Classifier: License :: OSI Approved :: GNU Lesser General Public License v3 (LGPLv3)' 'Classifier: Topic :: Software'
mkdist short_field 1.0 'License: Apache 2.0'
mkdist in_file 1.0 'License-File: LICENSE'
mkmod "$S/in_file-1.0.dist-info/licenses" "$mit"
mkdist long_field 1.0 "License: Apache-2.0 AND $(printf 'LicenseRef-scancode-x%.0s AND ' 1 2 3 4 5 6) GPL-1.0-or-later"
mkdist wrong_version 2.0 'License-Expression: MIT'
mkdist blank 1.0
cat >"$F/freeze.txt" <<'EOF'
# a comment
expr-pkg==1.0
classified==1.0
short-field==1.0
long-field==1.0
in_file==1.0
wrong-version==1.0
blank==1.0
absent==1.0
direct @ file:///x/direct.whl
EOF
python3 "$lib" python 2026-01-01 "$F/freeze.txt" "$S" >"$F/py-out.json" 2>/dev/null
ok "the Python reader runs on the fixture" [ $? -eq 0 ]
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
ok "Python: a long one-line License field, as written" grep -q 'GPL-1.0-or-later' "$F/py-out.json"
ok "Python: a license file in the dist-info" py_license in_file MIT
ok "Python: another version's metadata clears nothing" py_license wrong-version Unknown
ok "Python: no license data stays Unknown" py_license blank Unknown
ok "Python: not installed stays Unknown" py_license absent Unknown
ok "Python: a direct-URL line is still listed" py_license direct Unknown

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

ok "it lists the workspace crates and lock files the JSON holds" grep -q '^Rust lock files read: `' "$T/SBOM.md"

strip_date() { grep -v '^\*\*Generated:\*\*' "$1"; }
ok "the committed SBOM.md matches its detail files (scripts/generate-sbom.sh --report-only)" \
	cmp -s <(strip_date "$REPO_ROOT/sbom/SBOM.md") <(strip_date "$T/SBOM.md")

ok "the full run writes it too" grep -qE '^write_report$' "$REPO_ROOT/scripts/generate-sbom.sh"

[ "$fail" -eq 0 ] || exit 1
echo "[PASS] sbom/SBOM.md is generated and current"
