<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# sh-cd-unchecked

`build.sh` runs `cd` without checking it worked (SC2164), so a missing
directory makes the rest run in the wrong place. Make the script stop
when `cd` fails; shellcheck must pass at info level.
