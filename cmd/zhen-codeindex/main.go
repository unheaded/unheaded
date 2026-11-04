// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Command zhen-codeindex builds the source shard (DP-216, ADR-057): one flat
// index of committed source under --out, refreshed by blob. A second run
// while one is going exits 0 and does nothing; the next commit catches up.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/codeindex"
)

func main() {
	var (
		repo = flag.String("repo", ".", "git repository")
		out  = flag.String("out", "/var/zhen/code-index", "index directory")
	)
	cmdline.Parse(cmdline.Spec{
		Name:     "zhen-codeindex",
		Purpose:  "Build Zhen's source shard: symbols, Go call graph and word search over committed code.",
		Examples: []string{"zhen-codeindex --repo . --out /var/zhen/code-index"},
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *repo, *out, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "zhen-codeindex: %v\n", err)
		stop()
		os.Exit(1)
	}
}

// run takes the out/.lock flock and refreshes the index under it. Losing the
// lock to another run is not an error. The lock goes when the process does,
// so a killed run never leaves the next one locked out.
func run(ctx context.Context, repo, out string, stdout io.Writer) error {
	// #nosec G301 -- index of tracked text, readable by the services; same mode as codeindex.Save
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(out, ".lock")
	// #nosec G304 -- caller-chosen index dir
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	fd := int(lock.Fd()) // #nosec G115 -- fd fits in int
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			_, _ = fmt.Fprintln(stdout, "zhen-codeindex: busy")
			return nil
		}
		return fmt.Errorf("lock %s: %w", lockPath, err)
	}
	st, err := codeindex.Refresh(ctx, repo, out)
	if err != nil {
		return err
	}
	ix, err := codeindex.Load(out)
	if err != nil {
		return err
	}
	head := ix.Head
	if len(head) > 12 {
		head = head[:12]
	}
	_, _ = fmt.Fprintf(stdout, "zhen-codeindex: %s files=%d parsed=%d reused=%d deleted=%d chunks=%d calls=%d\n",
		head, st.Files, st.Parsed, st.Reused, st.Deleted, st.Chunks, st.Calls)
	return nil
}
