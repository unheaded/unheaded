// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

// Command gen writes raft/scripts/zhen_prompt_generated.py from pkg/zhenrag,
// or prints one rendered system prompt.
//
//	go run ./pkg/zhenrag/gen -out raft/scripts/zhen_prompt_generated.py
//	go run ./pkg/zhenrag/gen -dump chat|web|agent
package main

import (
	"flag"
	"fmt"
	"os"

	"unheaded/pkg/zhenrag"
)

func main() {
	out := flag.String("out", "", "write the generated Python module to this path")
	dump := flag.String("dump", "", "print SystemPrompt(mode) for chat, web or agent")
	flag.Parse()

	switch {
	case *dump != "":
		m, err := zhenrag.ParseMode(*dump)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Print(zhenrag.SystemPrompt(m))
	case *out != "":
		if err := os.WriteFile(*out, zhenrag.PythonModule(), 0o644); err != nil { // #nosec G306 -- tracked source file, world-readable like every .py in the repo
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: gen -out <file.py> | -dump chat|web|agent")
		os.Exit(2)
	}
}
