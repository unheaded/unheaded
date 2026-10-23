<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# go-data-race

`Counter` is used from many goroutines and its count comes out wrong
(and `go test -race` reports a data race). Make it safe for concurrent
use without changing its methods' signatures.
