<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# The Zhen Python stack was invisible to the SBOM — and has 7 known CVEs

**Found:** 2026-08-04
**Status:** manifest added; **the 7 advisories below are NOT fixed**

## What was wrong

`raft/` had **no Python dependency manifest of any kind** — no `requirements.txt`,
`pyproject.toml`, `setup.py`, `Pipfile` or lock file. 27 Python files import 18
distinct third-party packages, and none of them was declared anywhere.

That matters because `ci.yml`'s SBOM step is `syft dir:.`, and syft discovers
Python packages from manifests or installed distributions. With neither present
it finds nothing, and `grype sbom:...` then has nothing to scan.

Measured, not inferred:

```
syft dir:raft   before: 0 artifacts
                after:  18 artifacts (all type=python)
```

So the SBOM's "553 dependencies audited" was Go and Rust. The Python surface —
Flask, psycopg2, requests, torch, transformers, the MCP SDK — contributed **zero**
entries, and no gate would have reported a CVE in any of them. There is also no
`pip-audit` or `safety` step anywhere in `.github/workflows/`.

## What the scan found once they were visible

`grype` against the new SBOM — **7 matches, 4 High**. Every one has a fix
released:

| Sev | Package | Installed | Advisory | Fixed in | What it is |
|---|---|---|---|---|---|
| **High** | `mcp` | 1.27.0 | GHSA-jpw9-pfvf-9f58 | **1.27.2** | HTTP transports serve session requests **without verifying the authenticated principal** |
| **High** | `mcp` | 1.27.0 | GHSA-hvrp-rf83-w775 | **1.27.2** | Experimental task handlers let any client access and cancel other clients' tasks |
| **High** | `mcp` | 1.27.0 | GHSA-vj7q-gjh5-988w | 1.28.1 | WebSocket transport does not support Host/Origin validation |
| **High** | `transformers` | 5.3.0 | GHSA-fgcw-684q-jj6r | 5.5.0 | Arbitrary code execution during LightGlue model initialisation |
| Medium | `langchain-text-splitters` | 1.1.1 | GHSA-fv5p-p927-qmxr | 1.1.2 | `split_text_from_url` SSRF redirect bypass |
| Medium | `requests` | 2.32.5 | GHSA-gc5v-m9x4-r6x2 | 2.33.0 | Insecure temp file reuse in `extract_zipped_paths()` |
| Low | `torch` | 2.10.0 | GHSA-rrmf-rvhw-rf47 | 2.13.0 | Memory corruption via `torch.jit.script` |

**The three `mcp` findings are the ones to look at first.** `raft/zhen_mcp_server.py`
is an MCP server exposing 7 tools including `file_write`, `file_patch` and
`runbook_execute`. Two of the three are fixed by a **patch bump, 1.27.0 → 1.27.2**.

## Why the upgrades are not done here

Bumping means installing versions that are not currently installed and have not
been exercised against this stack. `transformers` 5.3 → 5.5 and `torch` 2.10 →
2.13 are the kind of change that breaks a working inference path, and the ROCm
build adds a second axis. That needs an attended run with the model paths
present, not an unattended one.

The manifest deliberately records **the versions running in `~/.venv/zhen` today**,
each confirmed importable. It is an accurate description of what runs, which is
what makes the advisories above trustworthy. Changing them to versions nobody has
run would trade a true statement for a hopeful one.

## Suggested order

1. `mcp` 1.27.0 → 1.27.2 — patch, closes two Highs on the authenticated-principal
   check. Re-run `zhen_mcp_server.py`'s tool list to confirm.
2. `langchain-text-splitters` 1.1.1 → 1.1.2 and `requests` 2.32.5 → 2.33.0 —
   patch/minor, low blast radius.
3. `transformers` 5.3 → 5.5 — needs an inference smoke test.
4. `torch` 2.10 → 2.13 — Low severity, largest blast radius, do last and only
   with the accelerator build that matches.

Then re-run: `syft dir:raft -o json | grype sbom:- ` and expect zero.

## Follow-on worth doing

- Add a `pip-audit` step to `security.yml` so this cannot regress silently. It
  now has a manifest to read, which it did not before.
- `raft/start-zhen.sh -gated` runs `./bin/zhen-agentd`, which `make build` does
  not produce — only the separate `make build-zhen-agentd` target does. The
  script fails ten seconds later with "did not bind"; the log says
  "No such file or directory". Small, unrelated, noted here so it is not lost.
