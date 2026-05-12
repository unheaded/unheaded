# Yggdrasil — Unheaded OS (hardened Debian soft-fork)

**Status:** Phase 1 SCAFFOLDING — full directory contract landed 2026-05-11. Real builds light up at task #65 (currently Q4 2026 per ADR-69420). Every file in this tree is decision-ready scaffold; nothing runs in production yet.
**Owner:** unheaded-architect (canonical) + unheaded-developer (impl) + unheaded-moatghost (compliance gates).
**Discipline:** see `docs/OS-FORK-DISCIPLINE.md` (5 pillars: anchor / overlay format / rebase cadence / divergence budget / UPC integration).
**Source ADR:** `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md` §"Feature B".

---

## Layout (2026-05-11 snapshot)

```
nix/yggdrasil/
├── README.md                              ← you are here
├── anchor.nix                             ← Pillar 1: Debian anchor (bookworm 12.x)
├── Jenkinsfile                            ← CI/CD pipeline runner (task #65)
│
├── overlay/                               ← Pillars 2 + 5 (overlay + UPC)
│   ├── patches/                           ← Quilt-format base patches
│   │   ├── series
│   │   ├── 0001-sysctl-kernel-hardening.patch
│   │   ├── 0002-sshd-config-hardening.patch
│   │   └── 0003-disable-unused-services.patch
│   ├── upc/                               ← Pillar 5 (UPC integration, task #71)
│   │   ├── README.md
│   │   ├── series
│   │   ├── 0001-add-upc-apt-source.patch
│   │   └── 0002-preinstall-upc-tools.patch
│   └── systemd/
│       └── upc-tty-bridge.service         ← CIS-aligned systemd unit
│
├── packer/                                ← Phase 1 build
│   ├── template.pkr.hcl                   ← Full provisioner flow (250 lines)
│   └── http/preseed.cfg                   ← Debian autoinstall (LVM + CIS partitioning)
│
├── provisioners/                          ← In-VM build steps (6 scripts, all bash-n clean)
│   ├── 01-ssh-and-sudo.sh                 ← Key auth + sshd hardening
│   ├── 02-apply-overlay.sh                ← quilt push -a + OS-FORK-DISCIPLINE §8 budget gates
│   ├── 03-install-upc.sh                  ← Kingdom apt source + 6 UPC packages
│   ├── 05-cis-hardening.sh                ← CIS L1 (50+ settings: sysctl, audit, pam, ssh)
│   ├── 07-lynis-gate.sh                   ← BUILD FAILS if hardening < 90 or CIS < 95%
│   └── 08-reproducibility-clean.sh        ← Deterministic mtimes for byte-identical rebuilds
│
├── scripts/                               ← Build-host helpers
│   ├── yggdrasil-verify-anchor.sh         ← Codename + version regex check
│   ├── yggdrasil-verify-overlay.sh        ← Patch budget + structure check
│   └── yggdrasil-build-evidence-pack.sh   ← Tarball assembly per evidence-pack runbook
│
├── bin/
│   └── yggdrasil-doctor-upc               ← 8-check preflight (installs at /usr/local/bin/)
│
├── evidence-pack/                         ← Task #68 (signed manifest)
│   ├── README.md
│   ├── schema/manifest-v1.yaml            ← JSON Schema draft 2020-12
│   └── runbooks/
│       ├── build-evidence-pack.md         ← 12-step pipeline runbook
│       └── verify-evidence-pack.md        ← Operator-side verify recipe
│
├── repo/                                  ← Task #65 apt repo (reprepro-managed)
│   ├── README.md
│   └── publish.sh
│
└── tests/
    └── smoke-boot.sh                      ← qemu-boot + yggdrasil-doctor upc gate
```

Plus the GHA companion at `.github/workflows/yggdrasil-verify.yml` (lightweight PR gate, runs on every nix/yggdrasil/ change).

## How the pieces fit together

```
PR to nix/yggdrasil/ → GHA yggdrasil-verify.yml (5 jobs, lightweight)
                          │
                          ▼ on merge
Jenkinsfile (cron 4 AM + tag) → 7 stages
  1. Discipline gates       (anchor + overlay verify)
  2. Cargo audit + govulncheck
  3. packer build           (preseed → 8 provisioners → reproducibility)
  4. Verify evidence pack   (yggdrasil-evidence verify)
  5. Smoke boot             (qemu + yggdrasil-doctor upc)
  6. Publish apt repo       (tag only — reprepro + ML-DSA-65 sign)
  7. Publish image          (tag only — S3 release channel)

Output: qcow2 image + signed evidence-pack-<hash>.tar.gz
        Each artifact verifiable standalone, no network needed.
        Maps to FedRAMP/SOC2/ISO 27001/CIS/NIST 800-53 controls.
```

## Five pillars of OS-FORK-DISCIPLINE

| Pillar | Mechanism | Scaffold status |
|--------|-----------|-----------------|
| 1. Anchor release | `anchor.nix` + `scripts/yggdrasil-verify-anchor.sh` | ✅ |
| 2. Overlay patch format | quilt-3.0 + `overlay/patches/series` + `overlay/upc/series` | ✅ |
| 3. Rebase cadence | 14d for upstream point release; 7d for security CVEs | 📋 documented in `docs/OS-FORK-DISCIPLINE.md` |
| 4. Divergence budget | 50 patches max; 5000 LOC delta max; CI-enforced (Jenkinsfile + GHA) | ✅ |
| 5. UPC integration | 5 required surfaces + 6 invariants per task #71 | ✅ |

## Acceptance gates (Jenkinsfile-enforced)

- [ ] `packer build` produces byte-identical ISOs on repeat runs (reproducibility — `08-reproducibility-clean.sh` + `SOURCE_DATE_EPOCH` plumbing)
- [ ] `lynis audit` returns hardening index ≥ 90 (`07-lynis-gate.sh`)
- [ ] Custom CIS L1 check ≥ 95% (`07-lynis-gate.sh`)
- [ ] Signed `.deb` validates with the Yggdrasil ML-DSA-65 key on install (`repo/publish.sh` + `pkg/gungnir/`)
- [ ] Signed manifest emitted on every build (`scripts/yggdrasil-build-evidence-pack.sh`)
- [ ] `yggdrasil-doctor upc` exits 0 in smoke boot (`tests/smoke-boot.sh`)

## Status by task

| Task | Description | 2026-05-11 status |
|------|-------------|-------------------|
| #69 OS-FORK-DISCIPLINE doc | ✅ closed 2026-05-10 (4 pillars + Pillar 5 added 2026-05-11) |
| #65 P1 Debian hardening pipeline | 🟡 Scaffold complete 2026-05-11 (packer + provisioners + scripts + Jenkinsfile + GHA + apt repo + smoke harness). Real builds = Q4 2026 horizon. |
| #68 P1 signed-manifest evidence pack | 🟡 Scaffold complete 2026-05-11 (schema + 2 runbooks + cmd/yggdrasil-evidence CLI + build-evidence-pack.sh hook). Real signing = with #65. |
| #71 UPC accessible & present | 🟡 Scaffold complete 2026-05-11 (overlay patches + systemd unit + doctor preflight + apt-list). Real apt-install = with #65. |
| #66 P2 SELinux policy port | ⏸ Blocked on #65 |
| #67 P2 cloud image targets | ⏸ Blocked on #65 (template uses qemu source; cloud sources commented out) |

## What this scaffolding does NOT do (yet)

- Build any actual ISO. (Packer template is the contract; runner doesn't yet exist.)
- Sign anything with the real Yggdrasil ML-DSA-65 key. (Key issuance lights up with task #65.)
- Run on a real Jenkins or GitHub-actions self-hosted runner. (CI manifests landed; runner provisioning is separate.)
- Implement SELinux policy. (Task #66, blocked on #65.)
- Test cloud image variants. (Task #67, blocked on #65.)

The scaffolding is what the next operator picks up. Every file in this tree is the decision-ready CONTRACT for what task #65 will execute.

## See also

- `docs/OS-FORK-DISCIPLINE.md` — the 5 pillars
- `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md` — Feature B mandate
- `docs/adr/ADR-073-lint-policy-zero-findings.md` — the kingdom-wide lint ratchet
- `references/battle-plan-NORTH-STAR-2026-05-05.md` §"Q4 2026 Yggdrasil track"
- `references/next-session-pickup-2026-05-11.md` — canonical handoff for the next Marshal shift

---

*Free to use. Free to share. KGLW. Peace and love.*
