<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# py-mutable-default

`add_tag` shares one list between calls that do not pass `tags`, so
tags leak from one call into the next. Fix it without changing the
signature's meaning (a caller may still pass its own list).
