# Security Findings Remediation Program

**Opened:** 2026-07-29
**Branch discipline:** all work lands on `develop`; `main` only by promotion PR after confirmation
**Decision:** Stevie, 2026-07-29 — *fix all findings, medium/high/low*. No severity is written off.

---

## Why the gate is a rule-ID allowlist, not a severity filter

The first pass at making CI green used `-severity high -confidence high`. That works,
but it is the wrong instrument:

- It is **opaque**. "high/high" does not tell you which 1172 findings you agreed to stop
  looking at, so nobody can audit the decision later.
- It does not **shrink monotonically**. Fixing 393 G115 findings does not change the flag,
  so the gate never records progress.
- It **silently absorbs new rules**. A future gosec release adding a MEDIUM-severity rule
  is auto-excluded, and no one finds out.

The ratchet below uses explicit `-exclude=<rule IDs>`. Every excluded rule is named, every
category closed removes an ID, and the end state is an empty exclude list. Progress is
visible in the workflow diff.

```yaml
# .github/workflows/security.yml — shrinks as phases close
args: '-exclude=G115,G404,G118,G104,G103,G304,G306,G301,G204 -fmt sarif -out gosec-results.sarif ./...'
```

---

## Baseline (measured 2026-07-29, gosec @latest)

| Rule | Sev | Conf | Count | What it is |
|------|-----|------|-------|------------|
| G115 | HIGH | MED | 393 | Integer overflow on conversion |
| G104 | LOW | HIGH | 318 | Unhandled errors |
| G103 | LOW | HIGH | 131 | Use of `unsafe` |
| G304 | MED | HIGH | 114 | File path from variable |
| G306 | MED | HIGH | 39 | WriteFile permissions |
| G301 | MED | HIGH | 35 | Directory permissions |
| G404 | HIGH | MED | 34 | Weak random (`math/rand`) |
| G204 | MED | HIGH | 33 | Subprocess with variable |
| G118 | HIGH | MED | 10 | (see gosec docs) |
| — | — | — | **1172** | total |

Severity totals: **HIGH 449 · MEDIUM 267 · LOW 456**

Other scanners: grype **16 MEDIUM / 17 LOW** (cutoff currently `high`); trivy configured
`CRITICAL,HIGH` only, so MEDIUM/LOW are never collected.

---

## Phase 0 — Gate integrity ✅ COMPLETE 2026-07-29

Not about finding counts. About whether the gates can fail at all.

- [x] **cargo-audit made real.** All three workspaces had `|| true`, swallowing the exit
      code. This hid **RUSTSEC-2026-0204** (`crossbeam-epoch` 0.9.18, invalid pointer
      deref) from 2026-07-06 until 2026-07-29 while the job reported success. Fixed to
      `crossbeam-epoch 0.9.20`; `|| true` removed; all three workspaces exit 0.
- [x] **gosec annotations made real.** ~20 sites used `//nolint:gosec // G703: …`, which
      gosec ignores — it only honors a comment *leading* with `#nosec`. Converted.
- [x] **No `continue-on-error` on SARIF upload.** Briefly added on a GHAS-403 theory that
      the trivy job's success falsified. Reverted.
- [x] govulncheck 0 · GPL boundary 0 (+14 classifier tests) · grype `--fail-on high` 0.

**Acceptance:** every job in `security.yml` can fail. Verified by the crossbeam find.

---

## Phase 1 — HIGH severity (449)

### 1a. G404 weak random — 34 sites
Triage each: if it feeds a token, session ID, nonce, or anything security-bearing →
`crypto/rand`. If it is jitter, backoff, sampling, or test fixtures → `#nosec G404 --`
with the reason. **Cannot be bulk-annotated** — the whole point is separating the two.
**Done when:** `-exclude` drops `G404`, gate green.

### 1b. G118 — 10 sites
Small. Read each.
**Done when:** `-exclude` drops `G118`.

### 1c. G115 integer overflow — 393 sites
The large one. Each needs a bounds check or a per-site justification. Expect this to
span multiple sessions. Suggested order: `pkg/` (library code, widest blast radius) →
`services/` → `cmd/` → `crates/` FFI boundaries.
Anti-pattern to avoid: a blanket `#nosec G115` sweep. That is annotation theatre and
reproduces the `//nolint` failure this program exists to correct.
**Done when:** `-exclude` drops `G115`. **This closes HIGH entirely.**

---

## Phase 2 — MEDIUM (267 gosec + 16 grype)

- G304 (114) file path from variable — most are config/operator paths; the work is
  proving that per site, not asserting it
- G306 (39) / G301 (35) permissions — mechanical, verify intent per path
- G204 (33) subprocess — highest real risk in this phase; audit argument construction
- grype: lower `severity-cutoff` to `medium`, fix or `.grype.yaml`-ignore the 16 with
  per-CVE rationale
- trivy: widen `severity` to include MEDIUM so it is collected at all

**Done when:** `-exclude` drops G304, G306, G301, G204; grype cutoff at `medium`.

---

## Phase 3 — LOW (456)

- G104 (318) unhandled errors — genuinely valuable; this is where real bugs hide in Go
- G103 (131) `unsafe` — mostly eBPF/FFI, expect a high legitimate-annotation ratio

**Done when:** `-exclude` is empty. Gate runs gosec unfiltered.

---

## Phase 4 — Coverage gaps (what nothing scans at all)

Raised by Stevie 2026-07-29: *"def should bare min do static analysis of ALL code."*
Current coverage is Go-only. Measured gaps:

| Language | Tracked files | Tool | Status |
|----------|--------------|------|--------|
| Go | ~267K LOC | gosec | running (ratcheting) |
| Rust | ~53K LOC | cargo-audit | **deps only — no `cargo clippy` lints in CI** |
| Shell | 154 files | — | **none — add `shellcheck`** |
| Python | 70 files | — | **none — add `ruff` + `bandit`** |
| JS | 40 files | — | **none — add `eslint`** |
| IaC | K8s/Helm/Docker | trivy misconfig | **was disabled; now reporting, not gating** |

### 4a. Trivy scanners (was running `vuln` only)

`scan-type: fs` defaults to the vulnerability scanner. `misconfig` and `secret` were
never enabled. Enabling them found:

- **secret: 0 findings** → gating at all severities from day one. Free, real gate.
- **misconfig: 522 findings** — 2 CRITICAL / 36 HIGH / 81 MEDIUM / 403 LOW.

The misconfig set independently rediscovers the run-as-identity problem this sweep
started from, and extends it to Docker:

- `DS-0002` *Image user should not be 'root'* — 6+ Dockerfiles (`docker/firewall/bird`,
  `frr`, `ipfire`, `opnsense`, `docker/routing/ospf`, `docker/services/dashboard-frontend`)
- `KSV-0118` *Default security context configured* — helm templates, cuirass, void-collector
- `KSV-0014` *Root filesystem not read-only*, `KSV-0009` *host network access*

The `pkg/uids` registry currently covers the telemetry tier only. Extending it to the
Docker images is the natural next increment.

**2 CRITICAL — both `kubernetes/manifests/base/haproxy-ingress/rbac.yaml`:**
- `KSV-0041` ClusterRole can get/list/watch/**create/patch/update** `secrets` cluster-wide
- `KSV-0046` `resources: ["*"]`

Assessment before anyone "fixes" these: the wildcard is **bounded to HAProxy's own CRD
API groups** (`ingress.v1.haproxy.org`, `ingress.v3.haproxy.org`), so KSV-0046 reads
worse than it is — remediate by enumerating those CRD resources, not by panicking.
KSV-0041 is genuine but the controller needs secret access for TLS cert handling; the
rules mirror the upstream haproxytech manifest. **Do not narrow it without a WEST
cluster test — a wrong cut breaks ingress TLS.** Track as its own task.

### 4b. Language linters — MEASURED baselines (2026-07-29)

| Tool | Scope | Baseline | Notes |
|------|-------|----------|-------|
| `shellcheck` | 154 tracked `.sh` | **361** — 11 error / 98 warning / 252 note | top: SC2034 unused (62), SC2317 unreachable (55), SC2015 `A && B \|\| C` (45), SC2086 unquoted (28) |
| `cargo clippy` | per-workspace | **13 in `crates/monad-mbc` alone** | full baseline needs a per-workspace sweep — see gotcha |
| `ruff` / `bandit` | 70 `.py` | not yet measured | |
| `eslint` | 40 `.js` | not yet measured | |

The 11 shellcheck **errors** are the priority — SC2148 (no shebang, so the analysed shell
is a guess) and friends are correctness bugs, not style. Given how much of this repo is
shell-driven (boot recipes, `scripts/`, runbooks executed with `sudo` against live
hardware), shell is arguably a higher-risk surface than the Go tree.

> **Gotcha — `cargo clippy --workspace` from the repo root is a no-op.**
> There is no root `Cargo.toml`; the tree has ~14 independent Cargo workspaces
> (`crates/*`, `cmd/*`, `ebpf/`). Running clippy at the root silently does nothing and
> a naive `| grep -c` reports **0 findings**, which reads as "clean" when it means
> "never ran". Any CI step must iterate the workspaces explicitly:
> ```bash
> for d in $(git ls-files '*/Cargo.toml' | xargs -n1 dirname | sort -u); do
>     (cd "$d" && cargo clippy --all-targets -- -D warnings) || exit 1
> done
> ```

Each lands non-gating first to measure, then ratchets, same as gosec.

### 4c. The ratchet is now enforced, not just documented

`scripts/check-gosec-ratchet.sh` (wired into `security.yml`) asserts the exclusion list
may only **shrink**. Verified in all three directions: current state PASSes, adding a
rule FAILs, removing a remediated rule PASSes and reports the win. Baseline pinned at
`docs/security/gosec-ratchet-baseline.txt` (26 rules).

Rationale: a suppression list guarded only by intent is the same failure that produced
`cargo audit … || true`, `//nolint:gosec`, and the disabled trivy scanners. Three
gates in this repo were green because they could not fail. A fourth was one config
edit away.

---

## Legal findings (Barrister, 2026-07-29)

**GPL-2.0-only is now a FAIL.** The classifier previously returned INFO for any `*GPL*`
that was not AGPL/LGPL. GPL-2.0-**only** is one-way incompatible with a
GPL-3.0-or-later project: we cannot downgrade to satisfy it, and it cannot be combined
with GPL-3.0 code. GPL-2.0-**or-later** is fine — we elect 3.0. Bare `GPL-2.0` is
treated as `-only`, because assuming the permissive reading of an ambiguous grant is
the assumption that loses. Latent today (only first-party GPL-2.0 crates exist and they
are skipped), so this closes a hole rather than fixing a live breach. 5 new tests.

**Election-based classification of `A OR B OR C` is sound**, and is standard SPDX `OR`
semantics — electing MIT from `Apache-2.0 OR LGPL-2.1-or-later OR MIT` means the LGPL
obligation never attaches. One caveat worth acting on: the election should be *recorded*
in `THIRD_PARTY.md` per dependency, so there is evidence of which branch was taken.
An undocumented election is harder to defend than a documented one.

---

## G115 classification (measured 2026-07-29)

393 findings. Triaged by conversion type and code pattern rather than site by
site, because the useful question is which conversions can actually truncate a
value that matters.

**The real bug it surfaced** — DNS name-compression pointers. `0xC000 | offset`
does not fail on an oversized offset, it merges into the marker bits, so any
message past 16 KiB emitted pointers to the wrong name. Fixed with
`MaxCompressionOffset`; see the commit and `pkg/dns/compression_test.go`.

**Checked and clean** — the other parsing paths:
- `pkg/netlink` — `int32(binary.LittleEndian.Uint32(...))` is a kernel-ABI
  round-trip; ifindex genuinely is int32. `uint8(prefixLen)` holds a 0-128 CIDR
  prefix.
- `pkg/waf/detection/ssrf.go` — `byte(num>>N)` is deliberate octet extraction.
- `pkg/upc/filesystem.go` — inode size bounded by the filesystem.
- `pkg/dns` label lengths were already validated against MaxLabelLength before
  the `byte()` conversion.

**Provably safe by pattern (187):**

| Class | Count | Why |
|---|---|---|
| `len-bounded` — `uintN(len(x))` | 92 | bounded by an existing allocation |
| `codegen-const` — `demos/`, `arch/` | 53 | assembler/build generators over internal constants |
| `abi-roundtrip` — kernel struct fields | 26 | width fixed by the kernel ABI |
| `octet-extract` — `byte(x>>N)` | 16 | deliberate byte extraction, safe by construction |

**Needs individual review (206)**, concentrated in:
`pkg/ebpf/loader.go` (43), `cmd/demo-trace-injector` (11),
`cmd/doom-go-injector` (10), `pkg/netlink` (9),
`services/wotan/internal/compute/memory.go` (9), then a long tail.

This is the remaining grind and is honestly several sessions of work. It should
NOT be closed with a blanket annotation — that reproduces the `//nolint`
failure this whole programme exists to correct. Work it file by file, highest
count first, and drop G115 from `-exclude=` only when the count reaches zero.

---

## Follow-ups raised during remediation

- **Warn loudly when `InsecureSkipVerify` is enabled.** All four G402 sites are
  config-driven and default to `false`, and the two mTLS ones still run
  `VerifyPeerCertificate` (Go invokes it even when `InsecureSkipVerify` is true), so
  peer certs are not left unchecked. But enabling them is currently *silent* — the
  same silent-fallback shape as the wotan credential default that G101 removed.
  `pkg/mesh/mtls/provider.go` has no logger in scope, so this needs a small plumbing
  change rather than a one-liner. Sites: `pkg/mesh/mtls/provider.go:421`,
  `pkg/mesh/mtls/mtls.go:319`, `pkg/loadbalancer/config.go:777`,
  `pkg/alerting/notify/email.go:196`.

- **`//nolint` is inert for more than gosec.** `go vet` does not parse it either —
  `pkg/ebpf/munmap_linux.go` carried `//nolint:govet,gosec` and had been failing
  `go vet` (and the pre-commit hook) the whole time. Fixed by removing the
  `unsafe.Pointer` conversion outright. Worth grepping for other `//nolint` uses that
  are load-bearing in someone's head but inert in the toolchain.

---

## Standing rules

1. **No new findings.** Once a rule leaves `-exclude`, it never goes back. A PR that
   reintroduces one fixes it or does not merge.
2. **Annotations state a reason.** `#nosec G304 --` with no rationale is rejected in
   review; it is indistinguishable from suppression.
3. **Fix beats annotate.** Annotation is for cases where the finding is genuinely wrong
   about the code, not where fixing is inconvenient.
4. **Promotion.** `develop` → `main` only when the phase's gate is green and confirmed
   on WEST. Merge commit, not squash (see CONTRIBUTOR-GUIDE §5).

---

## Related

- `docs/security/gosec-sweep-2026-05-08.md` — prior sweep; its high/high filter
  recommendation is superseded by the ratchet above
- `references/timeline.md` — 2026-07-29 CI green sweep entry
- `CONTRIBUTOR-GUIDE.md` §5 — develop/main branch discipline
