# SPDX-License-Identifier: GPL-3.0-or-later
# upc-rung0-assert.awk: ADR-109 rung 0 asserter (EVOLUTION-1 Stage 2 plan Step 86).
# Input: probe-build lines, one token per line, as `ugl` prints them:
#   UT:s=S;pid=P;ss=X;tf=T;a7=N;rf=R;   one per tagged trap (upc_slots.c)
#   SP:x17=V;                           init's x17 spill word at a child trap
#   GL:/GM:/GX: fork-ret=N              the probes' fork returns
# Checks: pid == s + 1 (I2); tf == ss and ss - 256*s is one base for every line
# (I1, per-slot trapframes); rf=1 on each slot's first trap and exactly once per
# fork that produced the slot (I3); a7 in {11, 22} (D4); no SP value of 11 (I8).
# Prints "RUNG0-ASSERT pass N" or "RUNG0-ASSERT fail <reason>" and exits 0 or 1.
function field(line, key,    r) {
  if (match(line, key "=-?[0-9]+;")) { r = substr(line, RSTART + length(key) + 1, RLENGTH - length(key) - 2); return r + 0 }
  return "x"
}
function bad(why) { if (!failed) { failed = 1; reason = why } }
/^UT:/ {
  n++
  s = field($0, "s"); pid = field($0, "pid"); ss = field($0, "ss"); tf = field($0, "tf"); a7 = field($0, "a7"); rf = field($0, "rf")
  if (s == "x" || pid == "x" || ss == "x" || tf == "x" || a7 == "x" || rf == "x") { bad("malformed UT line " n); next }
  if (s < 0 || s > 7) bad("slot " s " out of range")
  if (pid != s + 1) bad("pid " pid " for slot " s)
  if (tf != ss) bad("slot " s " row trapframe " tf " is not sscratch " ss)
  b = ss - 256 * s
  if (base == "") base = b; else if (b != base) bad("slot " s " trapframe not at base + 256*s")
  if (a7 != 11 && a7 != 22) bad("a7 " a7 " on slot " s)
  if (!(s in seen) && rf != 1) bad("first trap of slot " s " did not refresh")
  seen[s] = 1
  if (rf == 1) refresh[s]++
  next
}
/^SP:/ { v = field($0, "x17"); if (v == 11) bad("init's x17 spill word holds a child's a7 (I8)"); next }
/^G[LMX]: fork-ret=[0-9]+$/ { split($0, a, "="); forks[a[2] - 1]++; next }
END {
  if (n == 0) bad("no UT lines")
  for (k in forks) if (k > 0 && refresh[k] != forks[k]) bad("slot " k " refreshed " refresh[k] + 0 " times for " forks[k] " forks")
  for (k in refresh) if (!(k in forks)) bad("slot " k " refreshed without a fork")
  if (failed) { print "RUNG0-ASSERT fail " reason; exit 1 }
  print "RUNG0-ASSERT pass " n; exit 0
}
