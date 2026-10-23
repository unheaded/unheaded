<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# sh-read-raw

`lines.sh` reads lines with `read` and mangles backslashes (SC2162).
Fix it so shellcheck passes at info level and lines are read as is.
