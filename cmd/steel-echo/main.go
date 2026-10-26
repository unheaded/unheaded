// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// steel-echo is the S-REAL-STEEL game-day workload: a tiny HTTP service to
// deploy (Sword, through Cuirass, into LXD) and to break (Yaldabaoth), plus
// a load generator that measures availability from the client side.
//
//	steel-echo [serve] [--addr :8080]
//	steel-echo load --target http://127.0.0.1:26601/ --rps 20 --duration 60s --out run.csv
//	steel-echo load --discover http://127.0.0.1:19006 --svc steel-echo --port 8080 ...
//	steel-echo monad-send --iface steel0-h --dst fd00:5ee1::2 --flow-label 0x5EE1 --count 1000
//	steel-echo monad-count --iface steel0-n --seconds 30
//
// monad-send and monad-count are the Phase 13 lab traffic source and
// counter (monad.go): Monad-bearing IPv6 frames on a steel<N> veth, counted
// per flow label with the CHAOS bit and CRC state, headers only.
//
// serve answers GET /health ("ok"), /version, / (hostname and version, as
// JSON, nothing taken from the request) and /metrics (the house
// unheaded_http_* series). Two knobs from the environment make a bad build:
// STEEL_ERROR_RATE (0..1, the fraction of / answered 500) and
// STEEL_LATENCY_MS (0..10000, added to every /). Neither touches /health,
// so a bad build stays "healthy" to Cuirass and only canary analysis can
// catch it.
//
// The version comes from the build: -ldflags "-X main.version=v2".
package main

import (
	"fmt"
	"os"
)

// version is set at build time; each image (v1..v4) is one build.
var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && (args[0] == "serve" || args[0] == "load" || args[0] == "monad-send" || args[0] == "monad-count") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "load":
		err = runLoadCmd(args)
	case "monad-send":
		err = runMonadSend(args)
	case "monad-count":
		err = runMonadCount(args)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "steel-echo %s: %v\n", cmd, err)
		os.Exit(1)
	}
}
