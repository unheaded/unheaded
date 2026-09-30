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

	"gopkg.in/yaml.v3"

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

var (
	sysctlRefRe = regexp.MustCompile(`^(?:([a-z0-9][a-z0-9-]{0,62}):)?([a-z0-9_]+(?:\.[a-z0-9_]+)+)(>=|<=|=)(-?[0-9]{1,9})$`)
	sysctlValRe = regexp.MustCompile(`^-?[0-9]{1,12}$`)
)

// RemoteSysctl reads /proc/sys/<path> for each path on host and returns
// the values it could read, keyed by path.
type RemoteSysctl func(ctx context.Context, host string, paths []string) (map[string]string, error)

// SSHSysctl reads remote sysctls with one `ssh -o BatchMode=yes host cat
// ...` per host. Unreadable files are skipped by the remote shell, so a
// missing path simply has no value.
func SSHSysctl(ctx context.Context, host string, paths []string) (map[string]string, error) {
	// exit 0: an unreadable path is skipped, not a failed collection.
	script := `for p in "$@"; do v=$(cat "/proc/sys/$p" 2>/dev/null) && printf '%s %s\n' "$p" "$v"; done; exit 0`
	args := append([]string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "--", host, "sh", "-c", shQuote(script), "sysctl"}, quoteAll(paths)...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ssh %s: %w", host, err)
	}
	vals := map[string]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		p, v, ok := strings.Cut(line, " ")
		if ok {
			vals[p] = strings.TrimSpace(v)
		}
	}
	return vals, nil
}

// shQuote single-quotes s for the remote shell (paths are validated to
// [a-z0-9_/] already; this is belt and braces).
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = shQuote(s)
	}
	return out
}

// HostSysctl compares kernel parameters with required values. A ref is
// "[host:]name<op>int"; no host (or the collector's own hostname) is read
// from ProcSys, any other host through Remote. Each record names its host:
// the evidence is about that machine only. An unreachable host or an
// unreadable value produces no record.
type HostSysctl struct {
	ProcSys  string       // default /proc/sys
	Hostname string       // default os.Hostname()
	Remote   RemoteSysctl // default SSHSysctl
	Now      func() time.Time
}

type sysctlCheck struct {
	src        crosswalk.Source
	host, name string
	op         string
	want       int
}

// Collect returns one record per host-sysctl source whose value was read.
func (h *HostSysctl) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	proc := h.ProcSys
	if proc == "" {
		proc = "/proc/sys"
	}
	local := h.Hostname
	if local == "" {
		local, _ = os.Hostname()
	}
	remote := h.Remote
	if remote == nil {
		remote = SSHSysctl
	}

	byHost := map[string][]sysctlCheck{}
	var hosts []string
	for _, s := range sources {
		if s.Kind != crosswalk.KindHostSysctl {
			continue
		}
		m := sysctlRefRe.FindStringSubmatch(s.Ref)
		if m == nil {
			return nil, fmt.Errorf("host-sysctl ref %q must be [host:]<name><op><int>, op = >= <=", s.Ref)
		}
		host := m[1]
		if host == "" {
			host = local
		}
		want, _ := strconv.Atoi(m[4])
		if _, ok := byHost[host]; !ok {
			hosts = append(hosts, host)
		}
		byHost[host] = append(byHost[host], sysctlCheck{src: s, host: host, name: m[2], op: m[3], want: want})
	}

	var recs []crosswalk.Record
	for _, host := range hosts {
		checks := byHost[host]
		vals := map[string]string{}
		if host == local {
			for _, c := range checks {
				p := strings.ReplaceAll(c.name, ".", "/")
				if raw, err := os.ReadFile(filepath.Join(proc, p)); err == nil {
					vals[p] = strings.TrimSpace(string(raw))
				}
			}
		} else {
			paths := make([]string, len(checks))
			for i, c := range checks {
				paths[i] = strings.ReplaceAll(c.name, ".", "/")
			}
			got, err := remote(ctx, host, paths)
			if err != nil {
				continue // unreachable: no verdict for any of its checks
			}
			vals = got
		}
		for _, c := range checks {
			raw, ok := vals[strings.ReplaceAll(c.name, ".", "/")]
			if !ok || !sysctlValRe.MatchString(raw) {
				continue
			}
			got, err := strconv.Atoi(raw)
			if err != nil {
				continue
			}
			pass := (c.op == "=" && got == c.want) || (c.op == ">=" && got >= c.want) || (c.op == "<=" && got <= c.want)
			v := crosswalk.VerdictFail
			if pass {
				v = crosswalk.VerdictPass
			}
			recs = append(recs, crosswalk.Record{
				Source: c.src, Verdict: v, ObservedAt: now(),
				Detail: fmt.Sprintf("%s: %s = %d (required %s%d)", host, c.name, got, c.op, c.want),
			})
		}
	}
	return recs, nil
}

var attestationRe = regexp.MustCompile(`^compliance/attestations/[a-z0-9][a-z0-9-]{0,63}\.yaml$`)

// Attestation is a human statement kept in the repository. Its validity
// rests on the signature of the latest commit that touched the file: an
// unsigned attestation is a claim nobody vouched for.
type Attestation struct {
	Statement  string `yaml:"statement"`
	AttestedBy string `yaml:"attested_by"`
	AttestedAt string `yaml:"attested_at"` // YYYY-MM-DD
	ExpiresAt  string `yaml:"expires_at"`  // YYYY-MM-DD
}

// Attestations turns attestation files into records: pass when signed and
// unexpired, fail when unsigned or expired, nothing when missing or dated
// in the future. ObservedAt is the attestation date, so a control's
// freshness window bounds how old an attestation may be.
type Attestations struct {
	RepoDir string
	Now     func() time.Time
	// Signature returns the SHA of the latest commit touching path and
	// whether it carries a good signature. Default: git log -1 %H %G?.
	Signature func(ctx context.Context, path string) (commit string, good bool, err error)
}

func (a *Attestations) signature(ctx context.Context, path string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", a.RepoDir, "log", "-1", "--format=%H %G?", "--", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", false, err
	}
	sha, status, ok := strings.Cut(strings.TrimSpace(out.String()), " ")
	if !ok {
		return "", false, nil // never committed
	}
	return sha, status == "G", nil
}

// Collect returns one record per attestation source that exists.
func (a *Attestations) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	sig := a.Signature
	if sig == nil {
		sig = a.signature
	}
	var recs []crosswalk.Record
	for _, s := range sources {
		if s.Kind != crosswalk.KindAttestation {
			continue
		}
		if !attestationRe.MatchString(s.Ref) {
			return nil, fmt.Errorf("attestation ref %q must be compliance/attestations/<name>.yaml", s.Ref)
		}
		raw, err := os.ReadFile(filepath.Join(a.RepoDir, filepath.FromSlash(s.Ref)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(raw) > 64<<10 {
			return nil, fmt.Errorf("%s: larger than 64 KiB", s.Ref)
		}
		var at Attestation
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		if err := dec.Decode(&at); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Ref, err)
		}
		attested, err1 := time.Parse(time.DateOnly, at.AttestedAt)
		expires, err2 := time.Parse(time.DateOnly, at.ExpiresAt)
		if at.Statement == "" || at.AttestedBy == "" || err1 != nil || err2 != nil || !expires.After(attested) {
			return nil, fmt.Errorf("%s: needs statement, attested_by, and attested_at < expires_at as YYYY-MM-DD", s.Ref)
		}
		t := now()
		if attested.After(t) {
			continue // dated in the future: not evidence
		}
		commit, good, err := sig(ctx, s.Ref)
		if err != nil {
			return nil, fmt.Errorf("%s: signature: %w", s.Ref, err)
		}
		rec := crosswalk.Record{Source: s, Verdict: crosswalk.VerdictPass, ObservedAt: attested, Commit: commit,
			Detail: fmt.Sprintf("self-attested by %s on %s, expires %s", at.AttestedBy, at.AttestedAt, at.ExpiresAt)}
		switch {
		case !good:
			rec.Verdict = crosswalk.VerdictFail
			rec.Detail += "; latest commit not signed"
		case !t.Before(expires):
			rec.Verdict = crosswalk.VerdictFail
			rec.Detail += "; expired"
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

var sshdRefRe = regexp.MustCompile(`^(?:([a-z0-9][a-z0-9-]{0,62}):)?([a-z0-9]+)(>=|<=|=)([A-Za-z0-9@._,+-]{1,200})$`)

// SSHDDump returns `sshd -T` output for host: local sudo for the collector's
// own host, ssh + sudo for any other. Both need passwordless sudo (-n); without
// it there is no dump and so no record.
func SSHDDump(ctx context.Context, host string, local bool) (string, error) {
	var cmd *exec.Cmd
	if local {
		cmd = exec.CommandContext(ctx, "sudo", "-n", "sshd", "-T")
	} else {
		cmd = exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "--", host, "sudo -n sshd -T")
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("sshd -T on %s: %w", host, err)
	}
	return out.String(), nil
}

// HostSSHD checks effective sshd settings on each host its refs name.
type HostSSHD struct {
	Hostname string // default os.Hostname()
	// Dump returns `sshd -T` output for host. Default: SSHDDump.
	Dump func(ctx context.Context, host string) (string, error)
	Now  func() time.Time
}

// Collect returns one record per host-sshd source whose key the host reported.
func (h *HostSSHD) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	local := h.Hostname
	if local == "" {
		local, _ = os.Hostname()
	}
	dump := h.Dump
	if dump == nil {
		dump = func(ctx context.Context, host string) (string, error) { return SSHDDump(ctx, host, host == local) }
	}
	type check struct {
		src                crosswalk.Source
		host, key, op, val string
	}
	byHost := map[string][]check{}
	var hosts []string
	for _, s := range sources {
		if s.Kind != crosswalk.KindHostSSHD {
			continue
		}
		m := sshdRefRe.FindStringSubmatch(s.Ref)
		if m == nil {
			return nil, fmt.Errorf("host-sshd ref %q must be [host:]<key><op><value>", s.Ref)
		}
		if m[3] != "=" {
			if _, err := strconv.Atoi(m[4]); err != nil {
				return nil, fmt.Errorf("host-sshd ref %q: %s needs an integer", s.Ref, m[3])
			}
		}
		host := m[1]
		if host == "" {
			host = local
		}
		if _, ok := byHost[host]; !ok {
			hosts = append(hosts, host)
		}
		byHost[host] = append(byHost[host], check{s, host, m[2], m[3], m[4]})
	}
	var recs []crosswalk.Record
	for _, host := range hosts {
		out, err := dump(ctx, host)
		if err != nil {
			continue // no dump (unreachable, no sudo): no verdict
		}
		cfg := map[string]string{}
		for _, line := range strings.Split(out, "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
				if _, dup := cfg[k]; !dup { // sshd -T prints the effective (first) value
					cfg[k] = v
				}
			}
		}
		for _, c := range byHost[host] {
			got, ok := cfg[c.key]
			if !ok {
				continue
			}
			pass := false
			if c.op == "=" {
				pass = strings.EqualFold(got, c.val)
			} else if g, err := strconv.Atoi(got); err == nil {
				w, _ := strconv.Atoi(c.val)
				pass = (c.op == ">=" && g >= w) || (c.op == "<=" && g <= w)
			}
			v := crosswalk.VerdictFail
			if pass {
				v = crosswalk.VerdictPass
			}
			recs = append(recs, crosswalk.Record{Source: c.src, Verdict: v, ObservedAt: now(),
				Detail: fmt.Sprintf("%s: sshd %s %s (required %s%s)", host, c.key, got, c.op, c.val)})
		}
	}
	return recs, nil
}

// Probe reads one fact about a host with a fixed shell command and judges it.
type Probe struct {
	Command string                                      // run by sh on the host; may use sudo -n
	Judge   func(out string) (pass bool, detail string) // out is the command's stdout
	// Baseline marks a probe of host configuration, which the host agent
	// also evaluates (ADR-098). Operational probes (backup recency, which
	// depends on the operator's $HOME) stay central-only.
	Baseline bool
}

// Probes is the complete set of host probes the catalog may name. Commands
// are constants here, so catalog data can select a probe but never supply a
// command. Each reads the real state, not a unit's status: on east
// ufw.service is "active" while `ufw status` says inactive.
var Probes = map[string]Probe{
	"firewall-inbound-deny": {
		// Default deny inbound, either as the iptables INPUT policy for both
		// families or as the package-owned nftables table (ADR-098 step 6,
		// deploy/nftables), whose input chain drops by policy for IPv4 and
		// IPv6 at once. A drop policy in any input base chain is final.
		Baseline: true,
		Command: "sudo -n iptables -S INPUT; echo ---; sudo -n ip6tables -S INPUT; echo ---; " +
			"sudo -n nft list chain inet unheaded input | grep -m1 'hook input'",
		Judge: func(out string) (bool, string) {
			parts := strings.SplitN(out, "---", 3)
			for len(parts) < 3 {
				parts = append(parts, "")
			}
			pol := func(s string) string {
				for _, l := range strings.Split(s, "\n") {
					if strings.HasPrefix(strings.TrimSpace(l), "-P INPUT ") {
						return strings.TrimPrefix(strings.TrimSpace(l), "-P INPUT ")
					}
				}
				return "unknown"
			}
			p4, p6 := pol(parts[0]), pol(parts[1])
			nft := "absent"
			if hook := strings.TrimSpace(parts[2]); hook != "" {
				nft = "policy accept"
				if strings.Contains(hook, "policy drop") {
					nft = "policy drop"
				}
			}
			pass := (p4 == "DROP" && p6 == "DROP") || nft == "policy drop"
			return pass, fmt.Sprintf("INPUT policy IPv4 %s, IPv6 %s; nft inet unheaded input %s (required DROP, DROP or nft policy drop)", p4, p6, nft)
		},
	},
	"ntp-synchronized": {
		Baseline: true,
		Command:  "timedatectl show -p NTPSynchronized --value",
		Judge: func(out string) (bool, string) {
			v := strings.TrimSpace(out)
			return v == "yes", "NTPSynchronized=" + v
		},
	},
	"auditd-running": {
		Baseline: true,
		Command:  "systemctl is-active auditd",
		Judge: func(out string) (bool, string) {
			v := strings.TrimSpace(out)
			return v == "active", "auditd " + v
		},
	},
	"unattended-upgrades-enabled": {
		Baseline: true,
		Command:  "apt-config dump APT::Periodic::Unattended-Upgrade",
		Judge: func(out string) (bool, string) {
			v := strings.TrimSpace(out)
			return strings.Contains(v, `"1"`), strings.TrimSpace(strings.TrimSuffix(v, ";"))
		},
	},
	"db-backup-recent": {
		// The Well's dumps (runbook: pg_dump into ~/backups/unheaded/<date>/).
		Command: `find "$HOME/backups/unheaded" -type f -name '*.sql.gz' -mtime -7 -printf '%TF %f\n' | sort | tail -1; ` +
			`echo ---; find "$HOME/backups/unheaded" -type f -name '*.sql.gz' -printf '%TF\n' | sort | tail -1`,
		Judge: func(out string) (bool, string) {
			recent, newest, _ := strings.Cut(out, "---")
			recent, newest = strings.TrimSpace(recent), strings.TrimSpace(newest)
			if newest == "" {
				newest = "none"
			}
			return recent != "", "newest database backup " + newest + " (required within 7 days)"
		},
	},
	"apparmor-enabled": {
		Baseline: true,
		Command:  "cat /sys/module/apparmor/parameters/enabled",
		Judge: func(out string) (bool, string) {
			v := strings.TrimSpace(out)
			return v == "Y", "apparmor enabled=" + v
		},
	},
}

var probeRefRe = regexp.MustCompile(`^(?:([a-z0-9][a-z0-9-]{0,62}):)?([a-z0-9-]+)$`)

// HostProbes runs the named probes on each host its refs name: one script
// per host, sections delimited by "@@<probe>" lines.
type HostProbes struct {
	Hostname string // default os.Hostname()
	// Run executes script with sh on host (locally for the collector's own
	// host, else over ssh). Default: RunScript.
	Run func(ctx context.Context, host, script string) (string, error)
	Now func() time.Time
}

// RunScript runs a probe script locally or on host over ssh.
func RunScript(ctx context.Context, host, script string, local bool) (string, error) {
	var cmd *exec.Cmd
	if local {
		cmd = exec.CommandContext(ctx, "sh", "-c", script)
	} else {
		cmd = exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "--", host, "sh -c "+shQuote(script))
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	// A probe's own command may fail (auditd inactive exits 3); the script
	// always ends in exit 0, so an error here means the host was not reached.
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("probes on %s: %w", host, err)
	}
	return out.String(), nil
}

// Collect returns one record per host-probe source whose section came back.
func (p *HostProbes) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	local := p.Hostname
	if local == "" {
		local, _ = os.Hostname()
	}
	run := p.Run
	if run == nil {
		run = func(ctx context.Context, host, script string) (string, error) {
			return RunScript(ctx, host, script, host == local)
		}
	}
	type check struct {
		src   crosswalk.Source
		probe string
	}
	byHost := map[string][]check{}
	var hosts []string
	for _, s := range sources {
		if s.Kind != crosswalk.KindHostProbe {
			continue
		}
		m := probeRefRe.FindStringSubmatch(s.Ref)
		if m == nil {
			return nil, fmt.Errorf("host-probe ref %q must be [host:]<probe>", s.Ref)
		}
		if _, ok := Probes[m[2]]; !ok {
			return nil, fmt.Errorf("host-probe ref %q: unknown probe %q", s.Ref, m[2])
		}
		host := m[1]
		if host == "" {
			host = local
		}
		if _, ok := byHost[host]; !ok {
			hosts = append(hosts, host)
		}
		byHost[host] = append(byHost[host], check{s, m[2]})
	}
	var recs []crosswalk.Record
	for _, host := range hosts {
		var b strings.Builder
		seen := map[string]bool{}
		for _, c := range byHost[host] {
			if !seen[c.probe] {
				seen[c.probe] = true
				fmt.Fprintf(&b, "echo @@%s; { %s; } 2>/dev/null\n", c.probe, Probes[c.probe].Command)
			}
		}
		b.WriteString("exit 0\n")
		out, err := run(ctx, host, b.String())
		if err != nil {
			continue // unreachable: no verdicts
		}
		sections := map[string]string{}
		var cur string
		for _, line := range strings.Split(out, "\n") {
			if name, ok := strings.CutPrefix(line, "@@"); ok {
				cur = name
				sections[cur] = ""
				continue
			}
			if cur != "" {
				sections[cur] += line + "\n"
			}
		}
		for _, c := range byHost[host] {
			sec, ok := sections[c.probe]
			if !ok {
				continue
			}
			pass, detail := Probes[c.probe].Judge(sec)
			v := crosswalk.VerdictFail
			if pass {
				v = crosswalk.VerdictPass
			}
			recs = append(recs, crosswalk.Record{Source: c.src, Verdict: v, ObservedAt: now(), Detail: host + ": " + detail})
		}
	}
	return recs, nil
}
