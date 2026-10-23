// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"unheaded/pkg/lichtools"
)

// confirmMain is `lich-mcp confirm {list|show <id>|approve <id>}` — the local,
// out-of-band approval CLI for mutating Lich tools (Phase 4, Step 104). It is
// authenticated by filesystem ownership of the ticket directory: there is no
// network path, and the approval code never passes through any model.
func confirmMain(args []string) int {
	return runConfirm(args, os.Stdout, os.Stderr)
}

func runConfirm(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lich-mcp confirm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "configs/lich-mcp.yaml", "path to the Lich MCP server policy")
	root := fs.String("project-root", "", "checkout root; defaults to $PWD")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: lich-mcp confirm [flags] <list | show <id> | approve <id>>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}

	projectRoot := *root
	if projectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "confirm: resolve project root: %v\n", err)
			return 1
		}
		projectRoot = wd
	}
	cfg, err := lichtools.LoadConfig(*cfgPath, projectRoot)
	if err != nil {
		fmt.Fprintf(stderr, "confirm: %v\n", err)
		return 1
	}
	store, err := lichtools.NewPendingStore(cfg.PendingDirAbs())
	if err != nil {
		fmt.Fprintf(stderr, "confirm: %v\n", err)
		return 1
	}

	switch rest[0] {
	case "list":
		return confirmList(store, stdout, stderr)
	case "show":
		if len(rest) < 2 {
			fmt.Fprintln(stderr, "confirm show: need a ticket id")
			return 2
		}
		return confirmShow(store, rest[1], stdout, stderr)
	case "approve":
		if len(rest) < 2 {
			fmt.Fprintln(stderr, "confirm approve: need a ticket id")
			return 2
		}
		return confirmApprove(store, rest[1], stdout, stderr)
	default:
		fs.Usage()
		return 2
	}
}

func confirmList(store *lichtools.PendingStore, stdout, stderr io.Writer) int {
	tickets, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "confirm list: %v\n", err)
		return 1
	}
	if len(tickets) == 0 {
		fmt.Fprintln(stdout, "no pending confirmation tickets")
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTOOL\tSTATUS\tREQUESTED\tSUMMARY")
	for _, t := range tickets {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			t.ID, t.Tool, t.Status, t.RequestedAt.Format(time.RFC3339), t.Summary)
	}
	_ = tw.Flush()
	return 0
}

func confirmShow(store *lichtools.PendingStore, id string, stdout, stderr io.Writer) int {
	t, err := store.Get(id)
	if err != nil {
		fmt.Fprintf(stderr, "confirm show: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "id:           %s\n", t.ID)
	fmt.Fprintf(stdout, "tool:         %s\n", t.Tool)
	fmt.Fprintf(stdout, "status:       %s\n", t.Status)
	fmt.Fprintf(stdout, "requested_at: %s\n", t.RequestedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "input_sha256: %s\n", t.InputSHA256)
	fmt.Fprintf(stdout, "call_hash:    %s\n", t.CallHash)
	fmt.Fprintf(stdout, "summary:      %s\n", t.Summary)
	fmt.Fprintf(stdout, "args:         %s\n", string(t.Args))
	return 0
}

func confirmApprove(store *lichtools.PendingStore, id string, stdout, stderr io.Writer) int {
	// Show what is about to be approved, recomputed from the stored ticket (not
	// trusted from a prior `show`), then approve exactly that one call.
	t, err := store.Approve(id)
	if err != nil {
		fmt.Fprintf(stderr, "confirm approve: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "approved ticket %s (%s): %s\n", t.ID, t.Tool, t.Summary)
	fmt.Fprintln(stdout, "the driver may now re-issue the identical call; it will proceed exactly once.")
	return 0
}
