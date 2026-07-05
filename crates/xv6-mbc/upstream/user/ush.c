// SPDX-License-Identifier: GPL-3.0-or-later
//
// ush.c — UNHEADED sh: our own shell, the Track 2 Phase 2.2 finale
// (ADR-081 "evolve from xv6, never break the boot"). The last MIT
// program leaves the exec set.
//
// Built AS `sh` (target/sh.mbc — the PROGRAM_TABLE basename init execs),
// so init and the loader need no change; only the source is now 100%
// ours. The MIT xv6 sh.c stays in the vendored tree but is no longer
// wired into the build.
//
// Scope: EXEC-only — `line := WS* word (WS+ word)* '\n'`, then
// fork / exec / wait, prompt, loop. That is every clause the corpus
// exercises (battle plan docs/battle-plans/PHASE22-SH-REPLACE.md), and
// the rest of stock sh's grammar is dead code on the UPC by construction:
//   * PIPE needs pipe(4), cd needs chdir(9) — both unimplemented
//     (silent no-op fall-through in the ascend ecall dispatch);
//   * REDIR `>` needs open(15) O_CREATE, which the FS reader lacks;
//   * stock's parser mallocs every cmd node, and malloc rides sbrk(12) —
//     also unimplemented, so stock sh only ever worked by accident
//     (garbage a0 taken as a heap pointer). ush is malloc-free: one
//     static line buffer, argv tokenized in place.
//
// Documented divergences (all invisible to the corpus, all safer):
//   * metachars `<|>&;()` -> "syntax" from the child (stock: undefined
//     behavior through dead syscalls);
//   * `cd x` is just a command -> "exec cd failed" (stock: chdir no-op
//     that silently pretends to succeed);
//   * arg cap 8 = the exec(7) argv-harvest ABI (ARGV_MAX), not stock's
//     10 — the harvester drops args past 8 anyway, ush says so.
//
// Preserved stock behavior, byte for byte where the corpus can see it:
// "$ " prompt via write(2,..,2), getcmd shape (memset + gets, buf[100]),
// blank-line skip in the parent (no fork), panic text "syntax" /
// "too many args" / "fork" via fprintf(2,"%s\n") + exit(1) from the
// child, "exec %s failed" then child exit(0).

#include "kernel/types.h"
#include "user/user.h"
#include "kernel/fcntl.h"

// exec(7) argv-harvest cap (ARGV_MAX): 8 slots of 15 chars + NUL.
#define USH_MAXARGS 8

static char whitespace[] = " \t\r\n\v";
static char metachars[] = "<|>&;()";

// Hand-rolled set test — NOT xv6 strchr, whose never-matches-NUL quirk
// is a contract in uwc.c but a footgun in a tokenizer.
static int
in_set(char c, const char *set)
{
  for(; *set; set++)
    if(*set == c)
      return 1;
  return 0;
}

void
panic(char *s)
{
  fprintf(2, "%s\n", s);
  exit(1);
}

int
fork1(void)
{
  int pid;

  pid = fork();
  if(pid == -1)
    panic("fork");
  return pid;
}

// Split buf in place into NUL-terminated words. Runs in the child, so
// panics kill the failed command, never the shell.
static int
tokenize(char *buf, char *argv[])
{
  int argc;
  char *s;

  argc = 0;
  s = buf;
  for(;;){
    while(*s && in_set(*s, whitespace))
      s++;
    if(*s == 0)
      break;
    if(in_set(*s, metachars))
      panic("syntax");
    if(argc >= USH_MAXARGS)
      panic("too many args");
    argv[argc++] = s;
    while(*s && !in_set(*s, whitespace)){
      if(in_set(*s, metachars))
        panic("syntax");
      s++;
    }
    if(*s)
      *s++ = 0;
  }
  argv[argc] = 0;
  return argc;
}

void runcmd(char*) __attribute__((noreturn));

// Execute one command line. Never returns.
void
runcmd(char *buf)
{
  char *argv[USH_MAXARGS+1];

  if(tokenize(buf, argv) == 0)
    exit(1);
  exec(argv[0], argv);
  fprintf(2, "exec %s failed\n", argv[0]);
  exit(0);
}

int
getcmd(char *buf, int nbuf)
{
  write(2, "$ ", 2);
  memset(buf, 0, nbuf);
  gets(buf, nbuf);
  if(buf[0] == 0) // EOF
    return -1;
  return 0;
}

int
main(void)
{
  static char buf[100];
  int fd;

  // Ensure that three file descriptors are open.
  while((fd = open("console", O_RDWR)) >= 0){
    if(fd >= 3){
      close(fd);
      break;
    }
  }

  // Read and run input commands.
  while(getcmd(buf, sizeof(buf)) >= 0){
    char *cmd = buf;
    while(*cmd == ' ' || *cmd == '\t')
      cmd++;
    // Blank command: no fork (stock parity). The *cmd == 0 arm is
    // defensive only — gets() always keeps the '\n' while lines fit,
    // and the 64-byte KBD ring keeps them far under buf[100].
    if(*cmd == '\n' || *cmd == 0)
      continue;
    if(fork1() == 0)
      runcmd(cmd);
    wait(0);
  }
  exit(0);
}
