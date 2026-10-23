<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# go-table-test-extend

The table test for `Classify` has new rows for HTTP 429 and 503 that
fail. Extend `Classify` so 429 is "rate_limited" and 503 is
"unavailable", keeping every existing case.
