<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# go-off-by-one

`LastN(xs, n)` should return the last n elements of xs (all of xs when
n >= len(xs), none when n <= 0). It returns one element too many. Fix it.
