// SPDX-License-Identifier: GPL-3.0-or-later
//
// uecho.c — UNHEADED echo: our own `echo`, the first Track 2 Phase 2.2
// coreutil replacement (ADR-081 "evolve from xv6, never break the boot").
//
// Built AS `echo` (target/echo.mbc — the PROGRAM_TABLE basename sh
// resolves at exec time), so the shell needs no change; only the source
// is now 100% ours. The MIT xv6 echo.c stays in the vendored tree but is
// no longer wired into the build.
//
// Phase 2.2 gate: identical observable behavior to the program it
// replaces —
//   * arguments echoed in order, one space between, newline after the
//     last;
//   * zero arguments emit zero bytes (xv6 echo prints no lone newline,
//     unlike POSIX echo — we match xv6, the behavior the corpus locked).
//
// Deliberately one write() per token: the UPC write(16) handler is
// re-entrant (short-write-safe, 0xD0C0 cursor), so buffering would only
// add code to a program whose whole job is six lines.

#include "kernel/types.h"
#include "user/user.h"

int
main(int argc, char *argv[])
{
  for(int i = 1; i < argc; i++){
    write(1, argv[i], strlen(argv[i]));
    write(1, i + 1 < argc ? " " : "\n", 1);
  }
  exit(0);
}
