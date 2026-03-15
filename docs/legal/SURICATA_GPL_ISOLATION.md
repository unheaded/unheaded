# Suricata GPL-2.0 Isolation Boundary Documentation
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

## Overview

Suricata (https://suricata.io) is licensed under the GNU General Public License, version 2.0 (GPL-2.0).
The Unheaded Kingdom codebase is licensed under the GPL-3.0 License. This document defines the
legal isolation boundary between the two codebases, ensuring clear separation between
GPL-3.0-licensed Unheaded code and GPL-2.0-licensed Suricata code.

## Isolation Principle

The GPL-2.0 "derivative work" trigger applies when GPL-licensed code is compiled or linked
with other code to produce a combined work. The Unheaded Kingdom avoids this by interacting
with Suricata exclusively through well-defined inter-process communication (IPC) interfaces
that are analogous to the Linux kernel "system call interface" boundary recognized under
GPL exceptions.

## Interaction Point 1: EVE JSON File / REST API

**How it works**: Suricata writes structured JSON (EVE format) to `/var/log/suricata/eve.json`.
The Unheaded `pkg/anamnesis/suricata.go` bridge reads this file using standard OS file I/O
(inotify watch + sequential read). No Suricata source code is compiled into the bridge.
No Suricata shared libraries are linked.

**Legal rationale**: Reading a file produced by a GPL program does not create a derivative work
of that program. The EVE JSON format is Suricata's documented public output API. This is
equivalent to reading log files from any other process — no different from `tail -f syslog`.
The LGPL FAQ (applicable by analogy) and FSF guidance both confirm that the output of a
GPL program is not automatically GPL. The bridge is clearly a separate, independently
developed work that happens to consume Suricata's output.

**SPDX boundary**: `pkg/anamnesis/suricata.go` — SPDX-License-Identifier: GPL-3.0-or-later. Zero Suricata GPL-2.0 code.

## Interaction Point 2: BPF Map Sharing

**How it works**: The Shield eBPF program (GPL-3.0-licensed) pins BPF maps to `/sys/fs/bpf/unheaded/`.
The Suricata AF_PACKET eBPF bypass (GPL-2.0 Suricata code) reads from these maps to determine
which flows to bypass. The maps are shared via the Linux BPF virtual filesystem.

**Legal rationale**: BPF maps accessed via the `bpf(2)` system call constitute interaction
through the standard Linux kernel system call interface. The FSF explicitly recognizes that
programs interacting through kernel system calls are not derivative works of each other.
The BPF map is a kernel data structure; neither the Shield eBPF program nor the Suricata
eBPF program is a derivative of the other — they share data through the kernel, not through
source-level linking. This is identical in principle to two processes sharing POSIX shared
memory via `mmap(2)`.

**SPDX boundary**: Shield eBPF programs — SPDX-License-Identifier: GPL-3.0-or-later (GPL-2.0 kernel
headers included via GPL-2.0-WITH-Linux-syscall-note exception, which is standard).

## Interaction Point 3: Unix Socket Command Interface

**How it works**: Suricata exposes a Unix domain socket (`/run/suricata/suricata.socket`)
for runtime control. No Unheaded code currently uses this interface, but it may be used
in future health-check scripts for `routing-health` or similar tooling.

**Legal rationale**: Communicating with a process via a Unix socket is IPC — a canonical
example of separate programs running in separate process spaces. FSF guidance, LGPL v2.1
Section 6, and the broader software industry consensus all recognize process-level IPC
(pipes, sockets, files) as the definitive GPL isolation boundary. Any Unheaded script
that sends JSON to the Suricata socket and reads back JSON responses is not a derivative
work of Suricata.

## Deployment Boundary

Suricata is deployed as a separate OS service (NixOS systemd unit, Docker container,
or LXD container). The Unheaded build system does NOT compile, link, or bundle Suricata
source code. Suricata is installed from:
- NixOS: `pkgs.suricata` (nixpkgs — pre-compiled GPL binary, not statically linked into Unheaded)
- Docker: Separate `docker/suricata/Dockerfile` produces a standalone container image
- LXD: Separate `lxd/containers/suricata.yaml` defines an isolated system container

In all cases, the GPL obligation applies to the Suricata binary/container only, not to
any GPL-3.0-licensed Unheaded component.

## Conclusion

The Unheaded Kingdom's GPL-3.0 license is unaffected by the Suricata GPL-2.0 license.
The three interaction points documented above (EVE JSON file, BPF map sharing via syscall,
Unix socket IPC) all operate at well-recognized process/OS-level boundaries that do not
trigger GPL copyleft propagation. This isolation is by design and must be maintained.

**DO NOT**: import Suricata C headers in any Go or Rust Unheaded source file.
**DO NOT**: CGo-link against any Suricata shared library (libsuricata.so).
**DO NOT**: copy Suricata source files or modified versions into the Unheaded repository.
**DO**: interact exclusively via EVE JSON files, BPF map filesystem (bpf(2) syscall), and Unix socket IPC.
