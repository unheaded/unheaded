# Yggdrasil SELinux Policy Port (Task #66)

**Status**: 📋 RESEARCH SCAFFOLD — task is blocked on #65 packer implementation. This directory is the contract for what task #66 will produce.
**Source**: RHEL reference policy (`selinux-policy` upstream)
**Target**: Debian 12 (anchor) + Yggdrasil-specific tooling (UPC, evidence-pack, Wotan client)
**Owner (when task starts)**: unheaded-architect (canonical) + unheaded-developer (impl) + unheaded-moatghost (compliance gates)

---

## Why SELinux on Yggdrasil

Debian's default SELinux story is "ships disabled, AppArmor-by-default." The CIS Level 1 baseline (`provisioners/05-cis-hardening.sh`) enables AppArmor. SELinux is the stronger Mandatory Access Control framework — used by RHEL, CentOS, Fedora, Android, and required by certain federal compliance regimes (FedRAMP High, certain DoD postures).

Yggdrasil's value proposition includes "compliance templates baked in, not bolted on" (per `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md`). To carry that promise into FedRAMP High territory, the image needs to be installable in **enforcing mode** for tenants who need it — alongside the AppArmor-by-default option for tenants who don't.

## The port problem

RHEL's `selinux-policy` (refpolicy + selinux-policy-targeted) is the canonical reference. Porting to Debian requires:

1. **Path translation**: Debian uses different system paths than RHEL (`/usr/lib` vs `/lib`, init systems differ, etc.). Every `gen_context()` filecontext entry needs review.
2. **Service-unit translation**: Debian's systemd service unit naming + paths differ from RHEL. Type definitions for daemons need re-keyed.
3. **Package-name translation**: `httpd` vs `apache2`, `mariadb-server` vs `mariadb`, etc.
4. **Kingdom-specific extensions**: SELinux types for UPC tooling (`upc_bootctl_t`, `upc_tty_bridge_t`, `monad_cpu_ebpf_t`, etc.) — none of these exist in upstream policy.
5. **Enforcing-mode regression testing**: every Yggdrasil package needs to be runnable under `setenforce 1` without AVC denials.

## Reference architecture

```
nix/yggdrasil/selinux/
├── README.md                      ← this file
├── refpolicy/                     ← Vendored copy of RHEL selinux-policy
│   └── (TBD — task #66 vendors a pinned version)
├── debian-overlay/                ← Path/service/package translations
│   ├── filecontexts.diff          ← /usr/lib paths, etc.
│   ├── systemd-units.diff
│   └── package-names.diff
├── kingdom-types/                 ← Kingdom-specific module
│   ├── upc.te                     ← UPC tooling types + rules
│   ├── monad-cpu-ebpf.te          ← BPF program type
│   ├── wotan.te                   ← Wotan client/server types
│   └── yggdrasil-evidence.te      ← Evidence-pack tooling
└── tests/                         ← Enforcing-mode test harness
    ├── avc-clean-boot.sh          ← Boot in enforcing, verify no AVC denials
    ├── upc-bootctl-policy.sh      ← Exercise upc-bootctl under enforcing
    └── upc-tty-bridge-policy.sh   ← Exercise upc-tty-bridge under enforcing
```

## Acceptance gates (when task #66 runs)

- [ ] `setenforce 1` succeeds on a fresh Yggdrasil image (currently impossible — SELinux not installed)
- [ ] Zero AVC denials during clean boot + `yggdrasil-doctor upc` execution
- [ ] All 6 UPC binaries (`upc-bootctl`, `upc-tty-bridge`, `monad-cpu-ebpf`, `unheaded-shared`, `unheaded-runner`, `yggdrasil-evidence`) runnable under `setenforce 1`
- [ ] Per-binary SELinux module loadable independently (no monolithic kingdom-module)
- [ ] CIS Level 2 score ≥ 95% in enforcing mode
- [ ] Lynis hardening index ≥ 95 in enforcing mode (Level 1 was ≥ 90)

## Integration plan (when task #66 runs)

1. **Vendor refpolicy at a pinned commit.** Add `nix/yggdrasil/selinux/refpolicy/` with the upstream tree.
2. **Author kingdom-types modules.** Start with `upc-bootctl` since it's the most-touched UPC surface.
3. **Add a Phase 6 provisioner** (`nix/yggdrasil/provisioners/06-selinux.sh`): installs `selinux-basics`, `auditd`, `policycoreutils`; copies kingdom-types modules; runs `semodule -i` for each; sets `SELINUX=enforcing` in `/etc/selinux/config`.
4. **Build a SELinux variant** of the Yggdrasil image (parallel to the AppArmor-default variant) via a new packer source or var-flag.
5. **Add enforcing-mode tests** to `nix/yggdrasil/tests/`.
6. **Document** which tenants should pick which variant (compliance posture matrix in `docs/SECURITY-VARIANTS.md`).

## Out of scope for task #66

- **SELinux on the host kingdom OS** (i.e., the development machines where Unheaded engineers work). That's a separate compliance posture decision.
- **Custom SECCOMP profiles** for the UPC binaries. SELinux types + AppArmor profiles cover the MAC story; SECCOMP is a syscall-level filter, distinct concern.
- **MLS / multi-level security**. Standard targeted policy is enough for FedRAMP High; MLS would be a much bigger undertaking.

## See also

- `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md` §"Feature B" Barrister §"License compatibility" — SELinux policy is GPL-2.0, compatible with Yggdrasil's GPL-3.0-or-later
- `docs/OS-FORK-DISCIPLINE.md` Pillar 4 — adding 5+ MAC-policy patches will likely require reviewing the 50-patch budget; document an exemption or split the policy into a separate source tree
- `provisioners/05-cis-hardening.sh` — current AppArmor-default baseline that task #66 builds on (does not replace; the two coexist via packer variants)
- `nix/yggdrasil/Jenkinsfile` — pipeline that task #66 will plug a second variant into

---

*Free to use. Free to share.*
