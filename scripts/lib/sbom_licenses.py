#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Read dependency licenses from what is on this host, for scripts/generate-sbom.sh.

A license is what a dependency's own files say: the license files in its Go
module, the license field of its crate manifest, its Python metadata. Nothing
is guessed from a name and nothing is kept in a table here, so a package whose
files are not on this host lands as Unknown, with the reason, and the report
names it. This reads licenses; it does not rule on whether they fit together.

    sbom_licenses.py go TODAY MODULES_JSON GOMODCACHE COMPILED_LIST
    sbom_licenses.py rust TODAY PROJECT_ROOT CARGO_HOME
    sbom_licenses.py python TODAY FREEZE_FILE SITE_PACKAGES
    sbom_licenses.py javascript TODAY PROJECT_ROOT HTML_PAGE_LIST
    sbom_licenses.py report SBOM_DIR TODAY
"""
import collections
import email.parser
import glob
import json
import os
import re
import sys
import tarfile
import tomllib
import zipfile

UNKNOWN = 'Unknown'
LICENSE_FILE = re.compile(r'^(?!.*\.(?:go|rs|py|c|h|s|sh|js|json|ya?ml|toml)$)'
                          r'(licen[cs]e|copying|unlicense|copyright)([-._].*)?$', re.IGNORECASE)
GPL_FAMILY = re.compile(r'GPL|General Public', re.IGNORECASE)
SECONDARY_LICENSE = ('secondary license means either the gnu general public license version 2 0 '
                     'the gnu lesser general public license version 2 1 '
                     'the gnu affero general public license version 3 0 '
                     'or any later versions of those licenses')


def slurp(path, mode='r'):
    with open(path, mode, **({} if 'b' in mode else {'errors': 'replace'})) as f:
        return f.read()


def identify(text):
    """The licenses a license text states, by SPDX id. Empty when none is recognised."""
    t = ' '.join(re.sub(r'[^a-z0-9]+', ' ', text.lower()).split())
    found = set()
    if 'apache license' in t and 'version 2 0' in t and (
            'terms and conditions for use reproduction and distribution' in t
            or 'licensed under the apache license version 2 0' in t):
        found.add('Apache-2.0')
    if ('permission is hereby granted free of charge to any person obtaining a copy' in t
            and 'the above copyright notice and this permission notice shall be included' in t):
        found.add('MIT')
    if 'redistribution and use in source and binary forms' in t and 'redistributions of source code must retain' in t:
        if 'all advertising materials mentioning' in t:
            found.add('BSD-4-Clause')
        elif 'endorse or promote products derived from this software' in t:
            found.add('BSD-3-Clause')
        else:
            found.add('BSD-2-Clause')
    if re.search(r'permission to use copy modify and(?: or)? distribute this software for any purpose', t):
        found.add('ISC' if 'above copyright notice and this permission notice appear in all copies' in t else '0BSD')
    if 'mozilla public license' in t and '2 0' in t:
        found.add('MPL-2.0')
    if 'this is free and unencumbered software released into the public domain' in t:
        found.add('Unlicense')
    if 'altered source versions must be plainly marked' in t:
        found.add('Zlib')
    if 'dedicated to the public domain' in t:
        found.add('LicenseRef-public-domain')
    if 'cc0 1 0 universal' in t:
        found.add('CC0-1.0')
    if 'boost software license' in t:
        found.add('BSL-1.0')
    t = t.replace(SECONDARY_LICENSE, '')
    for name, spdx in (('gnu affero general public license', 'AGPL'),
                       ('gnu lesser general public license', 'LGPL'),
                       ('gnu library general public license', 'LGPL'),
                       ('gnu general public license', 'GPL')):
        for m in re.finditer(name + r'(?: version (\d)(?: ([01])\b)?)?', t):
            found.add(f'{spdx}-{m.group(1)}.{m.group(2) or "0"}' if m.group(1) else spdx)
    return found


def read_texts(texts):
    """(license, why not read) from {file name: text}. A file nobody recognises keeps the package unread."""
    if not texts:
        return UNKNOWN, 'no license file'
    found, strange = set(), []
    for name, text in sorted(texts.items()):
        ids = identify(text)
        found |= ids
        if not ids:
            strange.append(name)
    why = ', '.join(strange) + ' not recognised' if strange else None
    return ' AND '.join(sorted(found)) or UNKNOWN, why


def escaped(path):
    return re.sub('[A-Z]', lambda m: '!' + m.group().lower(), path)


def go_license_texts(obj, modcache):
    """{file: text} from the extracted module, else its cached zip. None when neither is here."""
    src = obj.get('Replace') or obj
    directory = src.get('Dir')
    if directory and os.path.isdir(directory):
        return {f: slurp(os.path.join(directory, f))
                for f in os.listdir(directory)
                if LICENSE_FILE.match(f) and os.path.isfile(os.path.join(directory, f))}
    archive = f"{modcache}/cache/download/{escaped(src.get('Path', ''))}/@v/{escaped(src.get('Version', ''))}.zip"
    if not os.path.isfile(archive):
        return None
    prefix = f"{src['Path']}@{src['Version']}/"
    with zipfile.ZipFile(archive) as z:
        return {n[len(prefix):]: z.read(n).decode(errors='replace') for n in z.namelist()
                if n.startswith(prefix) and '/' not in n[len(prefix):] and LICENSE_FILE.match(n[len(prefix):])}


def go_modules(path):
    raw, objects, at = slurp(path), [], 0
    decoder = json.JSONDecoder()
    while True:
        while at < len(raw) and raw[at].isspace():
            at += 1
        if at >= len(raw):
            return objects
        obj, at = decoder.raw_decode(raw, at)
        objects.append(obj)


def counts(packages):
    tally = collections.Counter(p['license'] for p in packages)
    return dict(sorted(tally.items(), key=lambda kv: (-kv[1], kv[0])))


def document(kind, today, tool, summary, packages):
    return {
        'spdxVersion': 'SPDX-2.3', 'dataLicense': 'CC0-1.0', 'SPDXID': 'SPDXRef-DOCUMENT',
        'name': f'unheaded-{kind}-dependencies',
        'documentNamespace': f'https://github.com/unheaded/unheaded/sbom/{kind}',
        'creationInfo': {'created': f'{today}T00:00:00Z', 'creators': [f'Tool: {tool}', 'Organization: Unheaded']},
        'summary': summary,
        'packages': packages,
    }


def emit(doc):
    json.dump(doc, sys.stdout, indent=2)
    print()


def read_go(today, modules_json, modcache, compiled_list):
    compiled = set(slurp(compiled_list).split())
    deps = []
    for obj in go_modules(modules_json):
        if obj.get('Main'):
            continue
        texts = go_license_texts(obj, modcache)
        lic, why = (UNKNOWN, 'no source in the module cache') if texts is None else read_texts(texts)
        dep = {'module': obj.get('Path', ''), 'version': obj.get('Version', ''),
               'indirect': obj.get('Indirect', False), 'compiled': obj.get('Path') in compiled, 'license': lic}
        if why:
            dep['licenseNotRead'] = why
        deps.append(dep)
    direct = [d for d in deps if not d['indirect']]
    emit(document('go', today, 'go-list-m-json', {
        'totalDependencies': len(deps), 'directDependencies': len(direct),
        'indirectDependencies': len(deps) - len(direct), 'licenseSummary': counts(deps)}, deps))
    print(f'  Go: {len(deps)} dependencies ({len(direct)} direct)', file=sys.stderr)


def parse_cargo_lock(path):
    crates, current = [], None
    for line in slurp(path).splitlines():
        line = line.strip()
        if line == '[[package]]':
            current = {}
            crates.append(current)
            continue
        m = re.match(r'(name|version|source) = "([^"]+)"', line)
        if m and current is not None:
            current[m.group(1)] = m.group(2)
    return crates


def crate_manifest(cargo_home, name, version):
    """Cargo.toml's bytes from the unpacked registry source, else the cached .crate. None when neither is here."""
    key = f'{name}-{version}'
    for path in glob.glob(f'{cargo_home}/registry/src/*/{key}/Cargo.toml'):
        return slurp(path, 'rb')
    for path in glob.glob(f'{cargo_home}/registry/cache/*/{key}.crate'):
        with tarfile.open(path) as tar:
            return tar.extractfile(f'{key}/Cargo.toml').read()
    return None


def read_crate(cargo_home, crate):
    if not crate.get('source', '').startswith('registry+'):
        return UNKNOWN, 'not from a registry'
    manifest = crate_manifest(cargo_home, crate['name'], crate.get('version', ''))
    if manifest is None:
        return UNKNOWN, 'no source in the cargo registry'
    lic = tomllib.loads(manifest.decode(errors='replace')).get('package', {}).get('license')
    return (lic, None) if isinstance(lic, str) and lic else (UNKNOWN, 'no license field in Cargo.toml')


def read_rust(today, root, cargo_home):
    lock_files = []
    for base, dirs, files in os.walk(root):
        dirs[:] = [d for d in dirs if d not in ('.git', 'node_modules', 'vendor', 'llama.cpp')]
        if 'Cargo.lock' in files:
            lock_files.append(os.path.relpath(os.path.join(base, 'Cargo.lock'), root))
    lock_files.sort()
    crates, workspace = {}, set()
    for lock in lock_files:
        for crate in parse_cargo_lock(os.path.join(root, lock)):
            if 'source' not in crate:
                workspace.add(crate['name'])
                continue
            key = (crate['name'], crate.get('version', 'unknown'))
            if key in crates:
                crates[key]['found_in'].append(lock)
                continue
            lic, why = read_crate(cargo_home, crate)
            crates[key] = {'crate': key[0], 'version': key[1], 'license': lic, 'found_in': [lock]}
            if why:
                crates[key]['licenseNotRead'] = why
    packages = [crates[k] for k in sorted(crates)]
    emit(document('rust', today, 'cargo-lock-parser', {
        'totalExternalCrates': len(packages), 'workspaceCrates': sorted(workspace),
        'licenseSummary': counts(packages), 'lockFiles': lock_files}, packages))
    print(f'  Rust: {len(packages)} external crates, {len(workspace)} workspace crates', file=sys.stderr)


def normal_name(name):
    return re.sub(r'[-_.]+', '-', name).lower()


def installed_dists(site):
    """{normal name: (version, METADATA, dist-info path)} for one site-packages."""
    dists = {}
    for info in glob.glob(f'{glob.escape(site)}/*.dist-info'):
        meta = email.parser.Parser().parsestr(slurp(f'{info}/METADATA'))
        dists[normal_name(meta['Name'])] = (meta['Version'], meta, info)
    return dists


def read_dist(meta, info):
    expression = meta.get('License-Expression')
    if expression:
        return expression, None
    classifiers = [c.split(' :: ')[-1] for c in meta.get_all('Classifier', []) if c.startswith('License ::')]
    classifiers = [c for c in classifiers if c != 'OSI Approved']
    if classifiers:
        return ' AND '.join(sorted(classifiers)), None
    field = (meta.get('License') or '').strip()
    if field and field.upper() != 'UNKNOWN':
        if len(field) <= 500 and '\n' not in field:
            return field, None
        if identify(field):
            return ' AND '.join(sorted(identify(field))), None
    texts = {}
    for base, _, files in os.walk(info):
        for f in files:
            if LICENSE_FILE.match(f):
                texts[os.path.relpath(os.path.join(base, f), info)] = slurp(os.path.join(base, f))
    return read_texts(texts) if texts else (UNKNOWN, 'no license in its metadata')


def read_python(today, freeze, site):
    dists = installed_dists(site)
    packages = []
    for line in slurp(freeze).splitlines():
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        m = re.match(r'([A-Za-z0-9._-]+)\s*(?:==\s*(\S+)|@\s*\S+)?', line)
        name, version = m.group(1), m.group(2) or ''
        dist = dists.get(normal_name(name))
        if dist is None:
            lic, why = UNKNOWN, 'not installed in the venv read'
        elif version and dist[0] != version:
            lic, why = UNKNOWN, f'venv holds {dist[0]}, not this version'
        else:
            lic, why = read_dist(dist[1], dist[2])
        package = {'package': name, 'version': version or (dist[0] if dist else ''), 'license': lic}
        if why:
            package['licenseNotRead'] = why
        packages.append(package)
    emit(document('python', today, 'python-dist-info-reader', {
        'totalPackages': len(packages), 'licenseSummary': counts(packages)}, packages))
    print(f'  Python: {len(packages)} packages', file=sys.stderr)


def cdn_files(html):
    """(url, has integrity) for each remote script and stylesheet a page loads."""
    for tag in re.finditer(r'<(script|link)\b[^>]*>', html, re.IGNORECASE):
        text = tag.group(0)
        if tag.group(1).lower() == 'link' and not re.search(r'rel=["\']?stylesheet', text, re.IGNORECASE):
            continue
        m = re.search(r'\b(?:src|href)=["\'](https?://[^"\']+)', text, re.IGNORECASE)
        if m:
            yield m.group(1), bool(re.search(r'\bintegrity=', text, re.IGNORECASE))


def read_javascript(today, root, page_list):
    found = {}
    for page in slurp(page_list).split('\n'):
        if not page:
            continue
        for url, integrity in cdn_files(slurp(os.path.join(root, page))):
            m = re.search(r'/((?:@[\w.-]+/)?[\w.-]+?)[@-](\d+\.\d+\.\d+)(?=/|$)', url)
            key = (m.group(1), m.group(2)) if m else (url.split('/')[2], '')
            entry = found.setdefault(key, {
                'library': key[0], 'version': key[1], 'url': url, 'integrity': True, 'loadedBy': [],
                'license': UNKNOWN, 'licenseNotRead': 'loaded from a CDN, no copy on this host'})
            entry['integrity'] = entry['integrity'] and integrity
            if page not in entry['loadedBy']:
                entry['loadedBy'].append(page)
    packages = [found[k] for k in sorted(found)]
    emit(document('javascript', today, 'html-script-tag-reader', {
        'totalFiles': len(packages), 'licenseSummary': counts(packages)}, packages))
    print(f'  JavaScript: {len(packages)} CDN-loaded libraries', file=sys.stderr)


def unread(package):
    lic = package.get('license', UNKNOWN)
    return lic.lower() == 'unknown' or '(assumed)' in lic or 'licenseNotRead' in package


def license_table(packages):
    rows = ['| License | Count |', '|---------|-------|']
    rows += [f'| {name} | {n} |' for name, n in counts(packages).items()]
    return '\n'.join(rows)


def report(sbom_dir, today):
    go, rust, python, javascript = (json.loads(slurp(f'{sbom_dir}/{kind}-dependencies.json'))
                                    for kind in ('go', 'rust', 'python', 'javascript'))
    gs, rs = go['summary'], rust['summary']
    sets = (('Go', 'module', go['packages'], 'Go modules'),
            ('Rust', 'crate', rust['packages'], 'Rust crates'),
            ('Python', 'package', python['packages'], 'Python packages'),
            ('JavaScript', 'library', javascript['packages'], 'JavaScript files'))

    gpl = [f"{p[key]} {p['version']} ({p['license']})" for _, key, packages, _ in sets
           for p in packages if GPL_FAMILY.search(p['license'])]
    tally = [f'{sum(1 for p in packages if unread(p))} of {len(packages)} {label}' for _, _, packages, label in sets]
    not_read = ', '.join(tally[:-1]) + f' and {tally[-1]} are not read.'
    any_unread = any(unread(p) for _, _, packages, _ in sets for p in packages)
    if gpl:
        verdict = 'REVIEW: licenses that name the GPL family, for a person to rule on: ' + ', '.join(gpl) + '. ' + not_read
    elif any_unread:
        verdict = f'INCOMPLETE: no license read names the GPL, but {not_read} Those are not cleared.'
    else:
        verdict = 'PASS: every license was read and none is GPL.'

    rows = []
    for label, key, packages, _ in sets:
        for p in sorted((p for p in packages if unread(p)), key=lambda p: p[key]):
            why = p.get('licenseNotRead', 'not read')
            if p.get('compiled') is False:
                why += '; compiled into no package here'
            rows.append(f"| {label} | {p[key]} | {p['version']} | {why} |")
    not_read_table = '\n'.join(['| Ecosystem | Name | Version | Why |', '|-----------|------|---------|-----|'] + rows) \
        if rows else 'None.'
    direct_rows = '\n'.join(f"| {d['module']} | {d['version']} | {d.get('license', UNKNOWN)} |"
                            for d in sorted((d for d in go['packages'] if not d['indirect']), key=lambda d: d['module']))
    cdn_rows = '\n'.join(f"| {p['library']} | {p['version'] or 'none'} | {'yes' if p['integrity'] else 'no'} | "
                         f"{', '.join(f'`{page}`' for page in p['loadedBy'])} |" for p in javascript['packages']) or '| none | | | |'
    summary = [(label, len(packages), sum(1 for p in packages if unread(p))) for label, _, packages, _ in sets]

    text = f"""# Unheaded Software Bill of Materials (SBOM)

**Generated:** {today}
**Format:** summary of SPDX-2.3 compatible files
**Main License:** GPL-3.0-or-later

Written by `scripts/generate-sbom.sh`; do not edit by hand. Runtime components
outside these lists are in `THIRD_PARTY.md`.

## Summary

| Ecosystem | Total | Direct | Licenses not read | Detail file |
|-----------|-------|--------|-------------------|-------------|
| Go | {gs['totalDependencies']} | {gs['directDependencies']} | {summary[0][2]} | `sbom/go-dependencies.json` |
| Rust | {rs['totalExternalCrates']} external, {len(rs['workspaceCrates'])} workspace | | {summary[1][2]} | `sbom/rust-dependencies.json` |
| Python | {summary[2][1]} | | {summary[2][2]} | `sbom/python-dependencies.json`, from `sbom/python-pip-freeze.txt`, a `pip freeze` of the Zhen venv, not a manifest |
| JavaScript | {summary[3][1]} | | {summary[3][2]} | `sbom/javascript-dependencies.json`, CDN-loaded files; nothing is installed |

Rust lock files read: {', '.join(f'`{f}`' for f in rs['lockFiles'])}.

Rust workspace crates: {', '.join(f'`{c}`' for c in rs['workspaceCrates'])}.

## Go licenses

Read from each module's license files, in the module cache. " AND " means the
files name each of these; they may be alternatives.

{license_table(go['packages'])}

## Direct Go modules

| Module | Version | License |
|--------|---------|---------|
{direct_rows}

## Rust licenses

The license field of each crate's own Cargo.toml, as written.

{license_table(rust['packages'])}

## Python licenses

Read from each package's metadata in the Zhen venv: License-Expression, else
the license classifiers, else the License field, else its license files.

{license_table(python['packages'])}

## JavaScript loaded from CDNs

| Library | Version | Integrity hash | Loaded by |
|---------|---------|----------------|-----------|
{cdn_rows}

## Licenses not read

{not_read_table}

## GPL boundary

{verdict}

This report reads licenses. It does not rule on whether they fit with
GPL-3.0-or-later. "Name the GPL family" includes a crate offering the LGPL as
one of several choices.

The compiled fixture under `doom/` is GPL-2.0. It runs inside the kernel's eBPF
VM and talks through BPF maps, so nothing links it. HAProxy (GPL-2.0) is a
separate process reached over sockets.
"""
    with open(f'{sbom_dir}/SBOM.md', 'w') as f:
        f.write(text)
    print(verdict, file=sys.stderr)


def main(argv):
    readers = {'go': read_go, 'rust': read_rust, 'python': read_python,
               'javascript': read_javascript, 'report': report}
    if len(argv) < 2 or argv[1] not in readers:
        sys.exit(__doc__)
    readers[argv[1]](*argv[2:])


if __name__ == '__main__':
    main(sys.argv)
