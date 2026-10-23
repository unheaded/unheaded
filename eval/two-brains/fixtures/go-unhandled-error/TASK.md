<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# go-unhandled-error

`WriteReport` ignores write errors, so a full disk or a closed pipe
looks like success. Make it return the first write error.
