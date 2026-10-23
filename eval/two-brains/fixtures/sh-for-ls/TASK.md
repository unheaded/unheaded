<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# sh-for-ls

`sizes.sh` loops over `$(ls *.log)` (SC2045), which breaks on file
names with spaces. Loop over the glob directly; shellcheck must pass
at info level.
