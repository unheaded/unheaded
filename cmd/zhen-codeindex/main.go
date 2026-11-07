// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Command zhen-codeindex builds the source shard (DP-216, ADR-057): one flat
// index of committed source under --out, refreshed by blob. A second run
// while one is going exits 0 and does nothing; the next commit catches up.
//
// With --corpus it indexes plain files instead, for the Grimoire's threat
// models and MITRE ATT&CK data (DP-719), and --search answers from that index.
// Both read files and nothing else, and the binary links no network code, so
// they run on the Grimoire's air-gapped VM. The corpus keeps its own --out, away
// from the shard.
package main

import (
	"context"
	"encoding/json"
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

// defaultOut is the source shard. Only a repository refresh writes it.
const defaultOut = "/var/zhen/code-index"

func main() {
	var (
		repo   = flag.String("repo", ".", "git repository")
		out    = flag.String("out", defaultOut, "index directory")
		corpus = flag.String("corpus", "", "index the plain files under this directory instead of a repository; --out must name the corpus index")
		search = flag.String("search", "", "search the corpus index in --out and print JSON; builds nothing")
		topK   = flag.Int("top-k", 8, "most results --search prints")
	)
	cmdline.Parse(cmdline.Spec{
		Name:    "zhen-codeindex",
		Purpose: "Build Zhen's source shard, or a text corpus index, and search the corpus: word search, no network.",
		Examples: []string{
			"zhen-codeindex --repo . --out /var/zhen/code-index",
			"zhen-codeindex --corpus /opt/tomb/grimoire --out /opt/tomb/grimoire/index",
			"zhen-codeindex --out /opt/tomb/grimoire/index --search T1566 --top-k 5",
		},
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := checkModes(*corpus, *search, *out)
	if err == nil {
		switch {
		case *search != "":
			err = runSearch(*out, *search, *topK, os.Stdout)
		case *corpus != "":
			err = runCorpus(ctx, *corpus, *out, os.Stdout)
		default:
			err = run(ctx, *repo, *out, os.Stdout)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "zhen-codeindex: %v\n", err)
		stop()
		os.Exit(1)
	}
}

// checkModes keeps a corpus away from the source shard, which the agent reads
// as code.
func checkModes(corpus, search, out string) error {
	if corpus != "" && search != "" {
		return errors.New("--corpus builds and --search reads: give one")
	}
	if (corpus != "" || search != "") && filepath.Clean(out) == defaultOut {
		return errors.New("--corpus and --search need an --out of their own, not the source shard")
	}
	return nil
}

// run refreshes the shard in out from repo.
func run(ctx context.Context, repo, out string, stdout io.Writer) error {
	return locked(out, stdout, func() error {
		st, err := codeindex.Refresh(ctx, repo, out)
		if err != nil {
			return err
		}
		return report(out, st, stdout)
	})
}

// runCorpus builds the corpus index in out from the files under root.
func runCorpus(ctx context.Context, root, out string, stdout io.Writer) error {
	return locked(out, stdout, func() error {
		st, err := codeindex.BuildCorpus(ctx, root, out)
		if err != nil {
			return err
		}
		return report(out, st, stdout)
	})
}

// searchResult is one hit; the keys are what tomb/provision.sh's rag-cerydwyn.py reads.
type searchResult struct {
	ID      string  `json:"id"`
	Source  string  `json:"source"`
	Trust   string  `json:"trust"`
	Score   float64 `json:"score"`
	Content string  `json:"content"`
}

type searchOutput struct {
	Query   string         `json:"query"`
	Results []searchResult `json:"results"`
}

// runSearch prints the k best chunks of the corpus index in dir as JSON, with
// the trust pkg/retrieval gives them. A missing index is an error here: the
// caller asked for this corpus by path.
func runSearch(dir, query string, k int, stdout io.Writer) error {
	ix, err := codeindex.Load(dir)
	if err != nil {
		return fmt.Errorf("no corpus index at %s, build one with --corpus: %w", dir, err)
	}
	hits := ix.Search(codeindex.Keywords(query), k)
	res := searchOutput{Query: query, Results: make([]searchResult, 0, len(hits))}
	for _, h := range hits {
		res.Results = append(res.Results, searchResult{
			ID: h.Chunk.ID, Source: h.Chunk.Path, Trust: codeindex.CorpusTrust(h.Chunk), Score: h.Score, Content: h.Chunk.Content,
		})
	}
	return json.NewEncoder(stdout).Encode(res)
}

// locked takes the out/.lock flock and runs build under it. Losing the lock to
// another run is not an error. The lock goes when the process does, so a
// killed run never leaves the next one locked out.
func locked(out string, stdout io.Writer, build func() error) error {
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
	return build()
}

func report(out string, st codeindex.Stats, stdout io.Writer) error {
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
