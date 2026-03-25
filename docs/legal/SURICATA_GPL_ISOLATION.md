# Suricata GPL-2.0 Isolation Boundary

SPDX-License-Identifier: GPL-3.0-or-later | Copyright (c) 2024-2026 Stevie Bellis | **Last updated:** 2026-03-25

## License Summary

| Component | License | SPDX Identifier |
|-----------|---------|-----------------|
| Unheaded Kingdom | GPL-3.0-or-later | GPL-3.0-or-later |
| Suricata IDS/IPS | GPL-2.0-only | GPL-2.0-only |

Suricata is an **optional** runtime dependency. Removing it does not break any
Unheaded binary, library, or service. The `services.unheaded.suricata.enable`
NixOS option defaults to false; the anamnesis bridge gracefully degrades when
no EVE JSON file exists.

## Process Isolation Architecture

Suricata runs as a **separate operating system process** under its own systemd
unit, user account (`suricata:suricata`), and filesystem namespace. No Suricata
source code, object code, or shared libraries are compiled into, linked with, or
bundled inside any Unheaded binary.

```
+----------------------------------------------------------+
|  Unheaded Platform  (GPL-3.0-or-later)                   |
|  Go + Rust binaries, vanilla JS frontend                 |
|                                                          |
|  pkg/anamnesis/suricata.go    nixos/modules/suricata.nix |
|       |              |                  |                 |
|  (1) read()     (2) bpf(2)        (3) connect()         |
|  EVE JSON file   BPF map lookup    Unix socket           |
+------|--------------|--------------------|---------------+
       |              |                    |
=======|==============|====================|========= OS / kernel boundary
       |              |                    |
+------v--------------v--------------------v---------------+
|  Suricata IDS/IPS  (GPL-2.0-only)                        |
|  Separate process — PID, UID, mount namespace isolated   |
|                                                          |
|  Writes eve.json    Reads BPF maps     Listens on socket |
|  /var/log/suricata/ /sys/fs/bpf/       /run/suricata/    |
+----------------------------------------------------------+
```

## Communication Channels

### 1. EVE JSON Log Files (Filesystem IPC)

Suricata writes structured JSON alerts to `/var/log/suricata/eve.json`. The
Unheaded bridge (`pkg/anamnesis/suricata.go`) reads this file via standard POSIX
`read(2)` with inotify notification. No Suricata headers, libraries, or source
are referenced. The FSF FAQ states that "the output of a program is not, in
general, covered by the copyright on the code."

### 2. BPF Maps (Kernel Syscall Boundary)

Shield eBPF programs (GPL-3.0) pin BPF maps to `/sys/fs/bpf/unheaded/`.
Suricata's AF_PACKET eBPF bypass (GPL-2.0) reads these maps via the `bpf(2)`
system call. Both programs interact through the kernel's BPF virtual filesystem
— a kernel-mediated data structure, not source-level linkage.

### 3. Unix Domain Sockets (IPC)

Suricata exposes a control socket at `/run/suricata/suricata.socket`. Unheaded
tooling may send JSON commands and receive JSON responses over this socket.
Socket IPC between separate processes is a textbook GPL isolation boundary.

## Why This Is NOT a Derivative Work

The legal analysis rests on established principles:

1. **Separate compilation and execution.** Suricata and Unheaded are compiled by
   independent toolchains (C vs. Go/Rust), produce independent binaries, and run
   in separate process address spaces. There is no static or dynamic linking.

2. **Linux kernel + userspace analogy.** The Linux kernel is GPL-2.0. Userspace
   programs communicate with it via system calls without becoming derivative
   works. Linus Torvalds codified this in the kernel's
   `LICENSES/exceptions/Linux-syscall-note`. The Suricata-to-Unheaded boundary
   is strictly analogous: two GPL programs in separate processes sharing data
   through kernel-mediated interfaces.

3. **FSF guidance on aggregation.** GPL-3.0 Section 5 defines an "aggregate" as
   separate and independent works not combined to form a larger program.
   Suricata and Unheaded are separate works on the same system. The FSF FAQ
   confirms that "mere aggregation" does not trigger copyleft.

4. **No intimate communication.** The three IPC channels exchange only serialized
   data (JSON, integer map values). There are no shared in-memory data
   structures, no function calls across the boundary, and no control flow
   coupling.

**References:**
- GNU GPL FAQ: https://www.gnu.org/licenses/gpl-faq.html
- GPL-3.0 Section 5 (Aggregation): https://www.gnu.org/licenses/gpl-3.0.html#section5
- Linux kernel syscall note: `LICENSES/exceptions/Linux-syscall-note`
- SFLC Practical Guide to GPL Compliance, Section 3.2

## Maintenance Rules

To preserve this boundary, all contributors MUST observe these rules:

- **DO NOT** `#include` or `import` any Suricata header or source file.
- **DO NOT** link against `libsuricata.so` or any Suricata shared library via CGo or FFI.
- **DO NOT** copy Suricata source code or modified versions into this repository.
- **DO** interact exclusively via EVE JSON files, BPF maps (`bpf(2)`), and Unix socket IPC.
- **DO** keep Suricata optional — all Unheaded functionality must work without it.

## Custom Suricata Rules — Configuration Data, Not Derivative Works

**Location:** `routing/suricata/rules/unheaded-monad.rules`
**SPDX Header:** `SPDX-License-Identifier: MIT`

Unheaded ships custom Suricata rule files that detect Monad protocol traffic
(HbH extension headers, anomalies, replay attacks, scanning). These rules are
**configuration data**, not derivative works of Suricata, for the following
reasons:

1. **Rules are input data.** Suricata rule files are structured text consumed
   by the Suricata engine at runtime, analogous to firewall rules consumed by
   iptables or SQL queries consumed by a database. The FSF FAQ confirms that
   data fed into a GPL program does not become a derivative work of that
   program.

2. **No Suricata source is incorporated.** The `.rules` files contain only
   Unheaded-authored detection signatures written in Suricata's documented
   rule language. They do not include, reference, or derive from any Suricata
   source code.

3. **Independent authorship.** All rules in `unheaded-monad.rules` (SID range
   9000001-9000099) are original works authored by Stevie Bellis, describing
   Monad protocol behavior defined in Unheaded's own Internet-Draft
   specifications.

4. **MIT licensed.** The rules carry an explicit `SPDX-License-Identifier: MIT`
   header, confirming they are independently licensed and not subject to
   Suricata's GPL-2.0.

**Analogy:** Writing a `.conf` file for Apache (Apache-2.0 licensed) does not
make your configuration a derivative work of Apache. The same principle applies
to Suricata rule files.

## No GPL Code in Unheaded Output Binaries

To be explicit: **no Suricata source code, object code, or GPL-2.0 licensed
material is compiled into, linked with, or bundled inside any Unheaded output
binary.** All Unheaded binaries are built from Go and Rust source files carrying
`SPDX-License-Identifier: GPL-3.0-or-later` headers. The project's SPDX
compliance is enforced by CI (see `scripts/check-spdx.sh`).

The interaction between Unheaded and Suricata is exclusively through the
runtime IPC channels documented above (EVE JSON, BPF maps, Unix sockets).

## SPDX Boundary Reference

| File | License | Role |
|------|---------|------|
| `pkg/anamnesis/suricata.go` | GPL-3.0-or-later | EVE JSON reader (Unheaded code) |
| `nixos/modules/suricata.nix` | MIT | NixOS service definition (config only) |
| `docker/suricata/suricata.yaml` | MIT | Docker Suricata config (config only) |
| `lxd/containers/suricata.yaml` | MIT | LXD container definition (config only) |
| Suricata binary / source | GPL-2.0-only | External dependency, not in this repo |
