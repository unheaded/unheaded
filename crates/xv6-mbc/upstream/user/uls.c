// SPDX-License-Identifier: GPL-3.0-or-later
//
// uls.c — UNHEADED ls: our own `ls` (Track 2 Phase 2.2, ADR-081).
//
// Built AS `ls` (target/ls.mbc — the PROGRAM_TABLE basename sh resolves
// at exec time); the MIT xv6 ls.c stays vendored, unwired.
//
// Phase 2.2 gate: identical observable behavior. The output contract per
// entry is `<name blank-padded to DIRSIZ> <type> <inum> <size>`, locked
// by the corpus gates (`ls` root listing, `ls sub` → `NOTE           2`).
// Two quirks preserved deliberately:
//   * a final component of DIRSIZ or more prints unpadded (the padding
//     buffer only fits DIRSIZ columns);
//   * an inode of unknown type prints nothing at all — the original's
//     switch falls through silently for anything but
//     T_FILE / T_DEVICE / T_DIR, so we keep that silence.
//
// struct stat on this port is the ilp32e 16-byte layout (xv6 uint64 is
// 4 bytes here; size at offset 12 — the Gate D fstat fix). We only ever
// touch it through the headers, so the layout is the kernel's business.

#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fs.h"
#include "kernel/fcntl.h"
#include "user/user.h"

// Blank-padding buffer for the final path component. Static like the
// original's: the returned pointer only has to live until the next call.
static char padded[DIRSIZ+1];

// Return the final path component, blank-padded to DIRSIZ columns
// (unpadded if it already fills them).
static char*
basename_padded(char *path)
{
  char *p, *last;
  int len;

  last = path;
  for(p = path; *p; p++)
    if(*p == '/')
      last = p + 1;

  len = strlen(last);
  if(len >= DIRSIZ)
    return last;
  memmove(padded, last, len);
  memset(padded + len, ' ', DIRSIZ - len);
  padded[DIRSIZ] = '\0';
  return padded;
}

static void
print_entry(char *path, struct stat *st)
{
  printf("%s %d %d %d\n", basename_padded(path), st->type, st->ino,
         (int) st->size);
}

static void
list(char *path)
{
  char sub[512];
  char *tail;
  struct dirent de;
  struct stat st;
  int fd;

  if((fd = open(path, O_RDONLY)) < 0){
    fprintf(2, "ls: cannot open %s\n", path);
    return;
  }
  if(fstat(fd, &st) < 0){
    fprintf(2, "ls: cannot stat %s\n", path);
    close(fd);
    return;
  }

  if(st.type == T_FILE || st.type == T_DEVICE){
    print_entry(path, &st);
  } else if(st.type == T_DIR){
    if(strlen(path) + 1 + DIRSIZ + 1 > sizeof(sub)){
      printf("ls: path too long\n");
    } else {
      // Walk the raw dirents; each valid entry gets path/<name> stat'd
      // and printed. Skipped slots (inum 0) are unlinked entries.
      strcpy(sub, path);
      tail = sub + strlen(sub);
      *tail++ = '/';
      while(read(fd, &de, sizeof(de)) == sizeof(de)){
        if(de.inum == 0)
          continue;
        memmove(tail, de.name, DIRSIZ);
        tail[DIRSIZ] = '\0';
        if(stat(sub, &st) < 0){
          printf("ls: cannot stat %s\n", sub);
          continue;
        }
        print_entry(sub, &st);
      }
    }
  }
  close(fd);
}

int
main(int argc, char *argv[])
{
  if(argc < 2){
    list(".");
    exit(0);
  }
  for(int i = 1; i < argc; i++)
    list(argv[i]);
  exit(0);
}
