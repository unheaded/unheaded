<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# Relicense candidates for Stevie's veto (DP-08)

DP-08 relicensed first-party files to GPL-3.0-or-later and asked for anything that looks deliberately different to be listed first. S-SUPPLY-FIRST left every row below exactly as it found it. Answer in the last column; "yes" means change it to GPL-3.0-or-later.

| # | Where | Says | Why it might be on purpose | Recommendation | Stevie |
|---|---|---|---|---|---|
| 1 | `dashboard/css/design-system.css:1`, `cmd/dashboard-backend/static/css/design-system.css:1` | `BUSL-1.1` | Not permissive and not free. The next line reads "Soul of unheaded.org, engineering of bellis.tech", so it may have come from a stylesheet shared with a site under other terms | Change, unless it is shared with something outside this repo | |
| 2 | `api/openapi.yaml:36` | `license: name: MIT` | An API description is what other people generate clients from; permissive would be a reasonable choice | Change, or match `LICENSE-PROTOCOLS` (GPL-3.0 / Apache-2.0) if client reuse was the point | |
| 3 | `crates/monad-mbc/Cargo.toml:6` | `license = "MIT"` | The MBC ISA is a spec other implementations may want. All 21 source headers already say GPL-3.0-or-later, so field and headers disagree | Change | |
| 4 | `nix/packages/wotan.nix:24`, `nix/containers/wotan.nix:132` | MIT | They describe `github.com/unheaded/wotan`, once a separate repository. The in-tree `services/wotan` is GPL-3.0-or-later | Change, unless that repository stays MIT | |
| 5 | `crates/lich-fuzz/Cargo.toml:17` | `license = "UNLICENSED"`, `publish = false` | Offensive tooling may be held back on purpose. The source headers say GPL-3.0-or-later | Change | |
| 6 | `crates/doom-runner/Cargo.toml:5` and six of its sources | `GPL-3.0-only` | Copyleft, not permissive, but deliberately narrower than or-later. It leaves for `unheaded-doom` in S-MOVE-OUT | Leave it for that repository | |
| 7 | `github.com/unheaded/doomgeneric` (outside this repo; `.github/workflows/ci.yml:203` says "SPDX MIT in every pkg/doom file") | MIT | First-party, and a go-licenses ignore rests on this | Decide with S-MOVE-OUT | |
| 8 | `crates/monad-mbc/fuzz/Cargo.toml` | no `license` field | It links `monad-common`, whose manifest says GPL-2.0-only, and it is a child of row 3 | Decide with row 3 | |
| 9 | `pkg/iac/chef.go:133` | writes `license 'MIT'` into generated cookbooks | Generated output belongs to the adopter, so this is a product question, not a header | Stevie's call: what a generated cookbook should declare | |
| 10 | `skills/unheaded-librarian/references/interchangeability-template.md:131` | "Unheaded adapter configs are MIT-licensed." | A template other docs are stamped from; adapter configs may have been meant to be copied freely | Change | |

## Not candidates, deliberately unchanged

- eBPF and kernel-side code that needs GPL-2.0: `ebpf/Cargo.toml`, `ebpf/af-xdp`, `cmd/ebpf-loader`, `cmd/ebpf-load-check`, `arch/mbc/`, and the crates with no licence field that compile into BPF (`ebpf/*`, `cmd/ebpf-collector/ebpf-programs`, `cmd/ebpf-collector/common`, `crates/heimdall-bpf`).
- `docs/protocol/`: dual GPL-3.0 / Apache-2.0 under `LICENSE-PROTOCOLS`.
- Vendored upstream code: `crates/xv6-mbc/upstream/` (MIT), `llama.cpp/`, `doom/`.
- Third-party licence tables: `LICENSES/`, `THIRD_PARTY.md`, `docs/compliance/`, `docs/legal/`.
- History: `docs/archive/`, `docs/battle-plans/`, `docs/sweeps/`.
