# Yggdrasil — UPC Integration Overlay

**Task #71**: UPC must be accessible AND present on the soft-fork OS.

Yggdrasil is not a vanilla Debian. It ships with the Unheaded Protocol Computer
(UPC) tooling installed, enabled, and self-checking out of the box. A user who
boots a fresh Yggdrasil image gets:

- `upc-bootctl(1)` on PATH — load and boot MBC kernel images
- `upc-tty-bridge(1)` on PATH and as an enabled `systemd` unit — Mode A
  (browser xterm) demo surface available immediately at port 26100
- `/opt/unheaded/share/monad-cpu-ebpf.o` — pre-built BPF object for the
  Yggdrasil pinned kernel (built with `--features ascend-linux`)
- `/opt/unheaded/share/upc-console.html` + `/opt/unheaded/share/vendor/xterm/`
  — vanilla-JS xterm.js client served by upc-tty-bridge
- `yggdrasil-doctor upc` — preflight check: kernel version, BPF support,
  XDP availability, verifier-budget headroom, MMIO region permissions

This is the C-pillar of OS-FORK-DISCIPLINE.md — Yggdrasil exists to be a host
that runs UPC. Without UPC integration, Yggdrasil is "just another hardened
Debian." With it, Yggdrasil is the only distro where `upc-bootctl boot xv6` is
a one-liner from a fresh install.

## Files in this overlay

| File | Purpose |
|------|---------|
| `series` | Quilt series listing patches in apply order |
| `0001-add-upc-apt-source.patch` | Add Unheaded apt repo to sources.list.d |
| `0002-preinstall-upc-tools.patch` | Mark upc-bootctl/upc-tty-bridge as Required |
| `../systemd/upc-tty-bridge.service` | systemd unit (enabled by default) |
| `../../bin/yggdrasil-doctor-upc` | Preflight self-check script |

## Apt repo layout (downstream of this overlay)

```
deb https://apt.unheaded.dev/yggdrasil bookworm main
  └── upc-bootctl        (cmd/upc-bootctl Go binary)
  └── upc-tty-bridge     (cmd/upc-tty-bridge Go binary)
  └── monad-cpu-ebpf     (BPF object built per kernel ABI)
  └── unheaded-runner    (crates/doom-runner Rust binary, optional)
  └── unheaded-shared    (HTML+JS+vendor/xterm asset pack)
```

The apt repo itself is built by the task #65 Debian hardening pipeline.

## Boot-time guarantees

After Yggdrasil boots:

1. `which upc-bootctl` → `/usr/bin/upc-bootctl` (exit 0)
2. `systemctl is-enabled upc-tty-bridge` → `enabled` (exit 0)
3. `systemctl is-active upc-tty-bridge` → `active` (exit 0)
4. `curl -fsS http://127.0.0.1:26100/healthz` → `OK\n` (exit 0)
5. `yggdrasil-doctor upc` → all green (exit 0)

If any of these fails, the Yggdrasil image has regressed and the build
pipeline (task #65) must reject the image.
