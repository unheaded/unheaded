<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# go-nil-map

`NewRegistry().Add(...)` panics with "assignment to entry in nil map".
Fix the constructor so a new Registry is ready to use.
