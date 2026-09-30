// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package collect gathers evidence records for crosswalk sources. Each
// collector turns an observation into a pass/fail record or into nothing:
// a skipped job, a cancelled run or a script that timed out says nothing
// about the control, so it produces no record (the control stays
// NOT_ASSESSED) rather than a verdict.
package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// MaxResponseBytes bounds every GitHub API response read.
const MaxResponseBytes = 8 << 20

var (
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	branchRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]{0,99}$`)
	scriptRe = regexp.MustCompile(`^scripts/[A-Za-z0-9_./-]+\.sh$`)
)

// GitHubJobs reads job conclusions from the latest completed run of each
// workflow on Branch. The public API needs no token for a public repo;
// Token, when set, is sent as a bearer token and never logged.
type GitHubJobs struct {
	BaseURL string // default https://api.github.com
	Repo    string // owner/name
	Branch  string
	Token   string
	Client  *http.Client
}

func (g *GitHubJobs) get(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > MaxResponseBytes {
		return fmt.Errorf("%s: response larger than %d bytes", url, MaxResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return json.Unmarshal(body, v)
}

// Collect returns one record per requested github-job source whose job
// concluded success or failure in its workflow's latest completed run.
func (g *GitHubJobs) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	if !repoRe.MatchString(g.Repo) || strings.Contains(g.Repo, "..") {
		return nil, fmt.Errorf("github repo %q", g.Repo)
	}
	if !branchRe.MatchString(g.Branch) {
		return nil, fmt.Errorf("branch %q", g.Branch)
	}
	base := g.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	want := make(map[string]map[string]bool) // workflow -> job names
	for _, s := range sources {
		if s.Kind != crosswalk.KindGitHubJob {
			continue
		}
		wf, jobName, ok := strings.Cut(s.Ref, "/")
		if !ok {
			return nil, fmt.Errorf("github-job ref %q is not <workflow>/<job>", s.Ref)
		}
		if want[wf] == nil {
			want[wf] = make(map[string]bool)
		}
		want[wf][jobName] = true
	}
	if len(want) == 0 {
		return nil, nil
	}

	var runs struct {
		WorkflowRuns []struct {
			ID      int64  `json:"id"`
			Name    string `json:"name"`
			HeadSHA string `json:"head_sha"`
		} `json:"workflow_runs"`
	}
	url := fmt.Sprintf("%s/repos/%s/actions/runs?branch=%s&status=completed&per_page=100", base, g.Repo, g.Branch)
	if err := g.get(ctx, url, &runs); err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	var recs []crosswalk.Record
	done := make(map[string]bool)
	for _, run := range runs.WorkflowRuns { // newest first
		if want[run.Name] == nil || done[run.Name] {
			continue
		}
		done[run.Name] = true
		var jobs struct {
			Jobs []struct {
				Name        string    `json:"name"`
				Conclusion  string    `json:"conclusion"`
				CompletedAt time.Time `json:"completed_at"`
				HTMLURL     string    `json:"html_url"`
			} `json:"jobs"`
		}
		if err := g.get(ctx, fmt.Sprintf("%s/repos/%s/actions/runs/%d/jobs?per_page=100", base, g.Repo, run.ID), &jobs); err != nil {
			return nil, fmt.Errorf("jobs of run %d: %w", run.ID, err)
		}
		for _, j := range jobs.Jobs {
			if !want[run.Name][j.Name] {
				continue
			}
			var v crosswalk.Verdict
			switch j.Conclusion {
			case "success":
				v = crosswalk.VerdictPass
			case "failure":
				v = crosswalk.VerdictFail
			default: // skipped, cancelled, neutral, timed_out...: not a verdict on the control
				continue
			}
			recs = append(recs, crosswalk.Record{
				Source:     crosswalk.Source{Kind: crosswalk.KindGitHubJob, Ref: run.Name + "/" + j.Name},
				Verdict:    v,
				ObservedAt: j.CompletedAt,
				Commit:     run.HeadSHA,
				Detail:     j.HTMLURL,
			})
		}
	}
	return recs, nil
}

// GitSignatures checks that the last Depth commits of a branch carry a good
// signature (git %G? == "G").
type GitSignatures struct {
	RepoDir string
	Depth   int
	Now     func() time.Time
}

// Collect returns one record per requested git-signatures source.
func (g *GitSignatures) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	var recs []crosswalk.Record
	for _, s := range sources {
		if s.Kind != crosswalk.KindGitSignatures {
			continue
		}
		if !branchRe.MatchString(s.Ref) {
			return nil, fmt.Errorf("git-signatures ref %q", s.Ref)
		}
		depth := g.Depth
		if depth <= 0 || depth > 1000 {
			depth = 50
		}
		// --end-of-options: the ref can never be read as an option.
		cmd := exec.CommandContext(ctx, "git", "-C", g.RepoDir, "log", "--format=%H %G?",
			fmt.Sprintf("--max-count=%d", depth), "--end-of-options", s.Ref)
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("git log %s: %v: %s", s.Ref, err, strings.TrimSpace(stderr.String()))
		}
		head, total, bad := parseSignatures(out.String())
		if total == 0 {
			continue
		}
		v := crosswalk.VerdictPass
		if bad > 0 {
			v = crosswalk.VerdictFail
		}
		recs = append(recs, crosswalk.Record{
			Source: s, Verdict: v, ObservedAt: now(), Commit: head,
			Detail: fmt.Sprintf("%d of %d most recent commits not signed with a good signature", bad, total),
		})
	}
	return recs, nil
}

// parseSignatures reads `git log --format='%H %G?'` output: the head commit,
// how many commits, how many without a good ("G") signature.
func parseSignatures(out string) (head string, total, bad int) {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		sha, status, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if head == "" {
			head = sha
		}
		total++
		if status != "G" {
			bad++
		}
	}
	return head, total, bad
}

// GateScripts runs repository gate scripts (scripts/*.sh) and records their
// exit status.
type GateScripts struct {
	RepoDir string
	Timeout time.Duration
	Now     func() time.Time
}

// Collect runs each requested gate-script source once.
func (g *GateScripts) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	var recs []crosswalk.Record
	for _, s := range sources {
		if s.Kind != crosswalk.KindGateScript {
			continue
		}
		clean := path.Clean(s.Ref)
		if !scriptRe.MatchString(s.Ref) || clean != s.Ref || strings.Contains(s.Ref, "..") {
			return nil, fmt.Errorf("gate-script ref %q must be scripts/<name>.sh", s.Ref)
		}
		timeout := g.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Minute
		}
		rctx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(rctx, "bash", filepath.Join(g.RepoDir, filepath.FromSlash(clean)))
		cmd.Dir = g.RepoDir
		cmd.Env = os.Environ()
		ownGroup(cmd)
		var tail bytes.Buffer
		cmd.Stdout, cmd.Stderr = io.Discard, &tail
		err := cmd.Run()
		timedOut := errors.Is(rctx.Err(), context.DeadlineExceeded)
		cancel()
		if timedOut {
			continue // no verdict: the gate did not finish
		}
		rec := crosswalk.Record{Source: s, Verdict: crosswalk.VerdictPass, ObservedAt: now(), Detail: "exit 0"}
		var exit *exec.ExitError
		switch {
		case err == nil:
		case errors.As(err, &exit):
			rec.Verdict = crosswalk.VerdictFail
			rec.Detail = fmt.Sprintf("exit %d", exit.ExitCode())
		default:
			return nil, fmt.Errorf("run %s: %w", s.Ref, err)
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

var sysctlRefRe = regexp.MustCompile(`^([a-z0-9_]+(?:\.[a-z0-9_]+)+)(>=|<=|=)(-?[0-9]{1,9})$`)

// HostSysctl reads kernel parameters on the host running the collector and
// compares each with the value its ref requires. The record names the host:
// this is evidence about that machine only.
type HostSysctl struct {
	ProcSys  string // default /proc/sys
	Hostname string // default os.Hostname()
	Now      func() time.Time
}

// Collect returns one record per host-sysctl source whose value was read.
func (h *HostSysctl) Collect(_ context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	proc := h.ProcSys
	if proc == "" {
		proc = "/proc/sys"
	}
	host := h.Hostname
	if host == "" {
		host, _ = os.Hostname()
	}
	var recs []crosswalk.Record
	for _, s := range sources {
		if s.Kind != crosswalk.KindHostSysctl {
			continue
		}
		m := sysctlRefRe.FindStringSubmatch(s.Ref)
		if m == nil {
			return nil, fmt.Errorf("host-sysctl ref %q must be <name><op><int>, op = >= <=", s.Ref)
		}
		name, op := m[1], m[2]
		want, _ := strconv.Atoi(m[3])
		raw, err := os.ReadFile(filepath.Join(proc, strings.ReplaceAll(name, ".", "/")))
		if err != nil {
			continue // unreadable here: no verdict
		}
		got, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			continue
		}
		ok := (op == "=" && got == want) || (op == ">=" && got >= want) || (op == "<=" && got <= want)
		v := crosswalk.VerdictFail
		if ok {
			v = crosswalk.VerdictPass
		}
		recs = append(recs, crosswalk.Record{
			Source: s, Verdict: v, ObservedAt: now(),
			Detail: fmt.Sprintf("%s: %s = %d (required %s%d)", host, name, got, op, want),
		})
	}
	return recs, nil
}
