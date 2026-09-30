// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

func job(ref string) crosswalk.Source {
	return crosswalk.Source{Kind: crosswalk.KindGitHubJob, Ref: ref}
}

// fakeGitHub serves two runs of workflow "W" (the newer first, as the API
// orders them) and one of "V", with jobs of every conclusion that matters.
func fakeGitHub(t *testing.T, hits *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/o/r/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		*hits++
		if r.URL.Query().Get("branch") != "develop" || r.URL.Query().Get("status") != "completed" {
			t.Errorf("runs query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"workflow_runs": []map[string]any{
			{"id": 2, "name": "W", "head_sha": "bbbb"},
			{"id": 1, "name": "W", "head_sha": "aaaa"},
			{"id": 3, "name": "V", "head_sha": "bbbb"},
		}})
	})
	jobs := map[string][]map[string]any{
		"2": {
			{"name": "pass", "conclusion": "success", "completed_at": "2026-09-30T10:00:00Z", "html_url": "https://x/2/pass"},
			{"name": "fail", "conclusion": "failure", "completed_at": "2026-09-30T10:01:00Z", "html_url": "https://x/2/fail"},
			{"name": "skipped", "conclusion": "skipped", "completed_at": "2026-09-30T10:02:00Z"},
			{"name": "cancelled", "conclusion": "cancelled", "completed_at": "2026-09-30T10:02:00Z"},
		},
		"1": {{"name": "old", "conclusion": "success", "completed_at": "2026-09-29T10:00:00Z"}},
		"3": {{"name": "v", "conclusion": "success", "completed_at": "2026-09-30T09:00:00Z"}},
	}
	mux.HandleFunc("GET /repos/o/r/actions/runs/{id}/jobs", func(w http.ResponseWriter, r *http.Request) {
		*hits++
		_ = json.NewEncoder(w).Encode(map[string]any{"jobs": jobs[r.PathValue("id")]})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHubJobs(t *testing.T) {
	var hits int
	srv := fakeGitHub(t, &hits)
	g := &GitHubJobs{BaseURL: srv.URL, Repo: "o/r", Branch: "develop", Client: srv.Client()}

	recs, err := g.Collect(context.Background(), []crosswalk.Source{
		job("W/pass"), job("W/fail"), job("W/skipped"), job("W/cancelled"), job("W/old"), job("V/v"), job("Z/none"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]crosswalk.Record{}
	for _, r := range recs {
		got[r.Source.Ref] = r
	}
	if r := got["W/pass"]; r.Verdict != crosswalk.VerdictPass || r.Commit != "bbbb" || r.Detail != "https://x/2/pass" ||
		!r.ObservedAt.Equal(time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("W/pass = %+v", r)
	}
	if got["W/fail"].Verdict != crosswalk.VerdictFail {
		t.Errorf("W/fail = %+v", got["W/fail"])
	}
	if got["V/v"].Verdict != crosswalk.VerdictPass {
		t.Errorf("V/v = %+v", got["V/v"])
	}
	// Skipped and cancelled jobs, jobs only in an older run, and unknown
	// workflows produce no record: absence, not a verdict.
	for _, ref := range []string{"W/skipped", "W/cancelled", "W/old", "Z/none"} {
		if r, ok := got[ref]; ok {
			t.Errorf("%s produced a record: %+v", ref, r)
		}
	}
	if hits != 3 { // one runs listing + the latest run of W and of V
		t.Errorf("%d API calls, want 3", hits)
	}
}

func TestGitHubJobs_Errors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"http 403 (rate limited)", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "rate", http.StatusForbidden) }},
		{"not json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }},
		{"oversized", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"workflow_runs":[` + strings.Repeat(" ", MaxResponseBytes+1) + `]}`))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			g := &GitHubJobs{BaseURL: srv.URL, Repo: "o/r", Branch: "develop", Client: srv.Client()}
			if _, err := g.Collect(context.Background(), []crosswalk.Source{job("W/pass")}); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestGitHubJobs_RejectsBadRepoAndBranch(t *testing.T) {
	for _, g := range []*GitHubJobs{
		{Repo: "o/r/../x", Branch: "develop"},
		{Repo: "o", Branch: "develop"},
		{Repo: "o/r", Branch: "dev elop"},
		{Repo: "o/r", Branch: "-x"},
	} {
		if _, err := g.Collect(context.Background(), nil); err == nil {
			t.Errorf("%+v accepted", g)
		}
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "develop")
	run("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "--no-gpg-sign", "-m", "one")
	return dir
}

func TestGitSignatures_UnsignedFails(t *testing.T) {
	dir := gitRepo(t)
	g := &GitSignatures{RepoDir: dir, Depth: 10}
	recs, err := g.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindGitSignatures, Ref: "develop"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Verdict != crosswalk.VerdictFail || !strings.Contains(recs[0].Detail, "1 of 1") {
		t.Fatalf("records = %+v", recs)
	}
}

func TestGitSignatures_RejectsOptionLikeRef(t *testing.T) {
	g := &GitSignatures{RepoDir: t.TempDir(), Depth: 10}
	for _, ref := range []string{"--output=/tmp/x", "-n1", "a b"} {
		if _, err := g.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindGitSignatures, Ref: ref}}); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
}

func TestGateScripts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"ok.sh": "exit 0\n", "bad.sh": "echo nope >&2; exit 3\n", "slow.sh": "sleep 5\n"} {
		if err := os.WriteFile(filepath.Join(root, "scripts", name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	g := &GateScripts{RepoDir: root, Timeout: 500 * time.Millisecond}
	src := func(p string) crosswalk.Source { return crosswalk.Source{Kind: crosswalk.KindGateScript, Ref: p} }
	recs, err := g.Collect(context.Background(), []crosswalk.Source{src("scripts/ok.sh"), src("scripts/bad.sh"), src("scripts/slow.sh")})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]crosswalk.Record{}
	for _, r := range recs {
		got[r.Source.Ref] = r
	}
	if got["scripts/ok.sh"].Verdict != crosswalk.VerdictPass {
		t.Errorf("ok = %+v", got["scripts/ok.sh"])
	}
	if r := got["scripts/bad.sh"]; r.Verdict != crosswalk.VerdictFail || !strings.Contains(r.Detail, "exit 3") {
		t.Errorf("bad = %+v", r)
	}
	// A timeout is not a verdict on the control: no record.
	if r, ok := got["scripts/slow.sh"]; ok {
		t.Errorf("timed-out script produced %+v", r)
	}
}

func TestGateScripts_RejectsPathsOutsideScripts(t *testing.T) {
	g := &GateScripts{RepoDir: t.TempDir(), Timeout: time.Second}
	for _, p := range []string{"/bin/true", "../x.sh", "scripts/../../x.sh", "tools/x.sh", "scripts/x.py"} {
		_, err := g.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindGateScript, Ref: p}})
		if err == nil {
			t.Errorf("%q accepted", p)
		}
	}
}

func TestParseSignatures(t *testing.T) {
	head, total, bad := parseSignatures("aaa G\nbbb N\nccc G\nddd U\neee E\n")
	if head != "aaa" || total != 5 || bad != 3 {
		t.Fatalf("head=%s total=%d bad=%d, want aaa 5 3 (only G is good)", head, total, bad)
	}
	if _, total, _ := parseSignatures(""); total != 0 {
		t.Fatalf("empty output counted %d commits", total)
	}
}

func TestHostSysctl(t *testing.T) {
	proc := t.TempDir()
	for name, v := range map[string]string{
		"kernel/kptr_restrict":             "1\n",
		"kernel/unprivileged_bpf_disabled": "2\n",
		"fs/suid_dumpable":                 "2\n",
		"kernel/garbage":                   "abc\n",
	} {
		p := filepath.Join(proc, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := &HostSysctl{ProcSys: proc, Hostname: "west"}
	src := func(r string) crosswalk.Source { return crosswalk.Source{Kind: crosswalk.KindHostSysctl, Ref: r} }
	recs, err := h.Collect(context.Background(), []crosswalk.Source{
		src("kernel.kptr_restrict>=1"),
		src("kernel.unprivileged_bpf_disabled>=1"),
		src("fs.suid_dumpable=0"),
		src("kernel.missing=1"),
		src("kernel.garbage=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]crosswalk.Record{}
	for _, r := range recs {
		got[r.Source.Ref] = r
	}
	if r := got["kernel.kptr_restrict>=1"]; r.Verdict != crosswalk.VerdictPass || !strings.Contains(r.Detail, "west: kernel.kptr_restrict = 1") {
		t.Errorf("kptr = %+v", r)
	}
	if got["kernel.unprivileged_bpf_disabled>=1"].Verdict != crosswalk.VerdictPass {
		t.Errorf("bpf = %+v", got["kernel.unprivileged_bpf_disabled>=1"])
	}
	if r := got["fs.suid_dumpable=0"]; r.Verdict != crosswalk.VerdictFail || !strings.Contains(r.Detail, "= 2") {
		t.Errorf("suid_dumpable = %+v", r)
	}
	// Unreadable or non-numeric values are not a verdict.
	for _, ref := range []string{"kernel.missing=1", "kernel.garbage=1"} {
		if r, ok := got[ref]; ok {
			t.Errorf("%s produced %+v", ref, r)
		}
	}
}

func TestHostSysctl_RejectsBadRefs(t *testing.T) {
	h := &HostSysctl{ProcSys: t.TempDir()}
	for _, ref := range []string{"../../etc/shadow=1", "kernel.x", "kernel.x==1", "kernel/x=1", "kernel.x=abc"} {
		if _, err := h.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindHostSysctl, Ref: ref}}); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
}
