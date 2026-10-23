<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# sh-quote-vars

shellcheck flags unquoted variables in `deploy.sh` (SC2086): a path
with a space would split into two arguments. Quote them so shellcheck
passes at info level, without changing what the script does.
