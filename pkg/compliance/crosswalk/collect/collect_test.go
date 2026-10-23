// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
		q := r.URL.Query()
		if q.Get("branch") != "develop" || q.Get("status") != "completed" || q.Get("event") != "push" {
			t.Errorf("runs query = %s", r.URL.RawQuery)
		}
		own := map[string]string{"full_name": "o/r"}
		_ = json.NewEncoder(w).Encode(map[string]any{"workflow_runs": []map[string]any{
			// Newest: a pull request from a fork's "develop" branch, which
			// ran the fork's own workflow. Never evidence for o/r.
			{"id": 9, "name": "W", "head_sha": "ffff", "event": "pull_request", "head_repository": map[string]string{"full_name": "evil/r"}},
			{"id": 8, "name": "W", "head_sha": "eeee", "event": "push", "head_repository": map[string]string{"full_name": "evil/r"}},
			{"id": 2, "name": "W", "head_sha": "bbbb", "event": "push", "head_repository": own},
			{"id": 1, "name": "W", "head_sha": "aaaa", "event": "push", "head_repository": own},
			{"id": 3, "name": "V", "head_sha": "bbbb", "event": "push", "head_repository": own},
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
		"9": {{"name": "fail", "conclusion": "success", "completed_at": "2026-09-30T11:00:00Z"}},
		"8": {{"name": "fail", "conclusion": "success", "completed_at": "2026-09-30T11:00:00Z"}},
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

func TestHostSysctl_RemoteHosts(t *testing.T) {
	var calls []string
	h := &HostSysctl{
		ProcSys:  t.TempDir(),
		Hostname: "west",
		Remote: func(_ context.Context, host string, paths []string) (map[string]string, error) {
			calls = append(calls, host)
			if host == "down" {
				return nil, errors.New("no route to host")
			}
			out := map[string]string{}
			for _, p := range paths {
				if p == "kernel/kptr_restrict" {
					out[p] = "1"
				}
			}
			return out, nil
		},
	}
	src := func(r string) crosswalk.Source { return crosswalk.Source{Kind: crosswalk.KindHostSysctl, Ref: r} }
	recs, err := h.Collect(context.Background(), []crosswalk.Source{
		src("east:kernel.kptr_restrict>=1"),
		src("east:kernel.dmesg_restrict=1"), // not returned by the host: no record
		src("down:kernel.kptr_restrict>=1"), // unreachable host: no record, not an error
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Source.Ref != "east:kernel.kptr_restrict>=1" || recs[0].Verdict != crosswalk.VerdictPass ||
		!strings.HasPrefix(recs[0].Detail, "east: ") {
		t.Fatalf("records = %+v", recs)
	}
	if len(calls) != 2 {
		t.Errorf("remote calls %v: want one batched call per host", calls)
	}
}

func TestHostSysctl_LocalNameIsLocal(t *testing.T) {
	proc := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proc, "kernel"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "kernel", "kptr_restrict"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &HostSysctl{ProcSys: proc, Hostname: "west", Remote: func(context.Context, string, []string) (map[string]string, error) {
		t.Fatal("the collector's own host must be read locally")
		return nil, nil
	}}
	recs, err := h.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindHostSysctl, Ref: "west:kernel.kptr_restrict>=1"}})
	if err != nil || len(recs) != 1 || recs[0].Verdict != crosswalk.VerdictPass {
		t.Fatalf("recs=%+v err=%v", recs, err)
	}
}

func TestHostSysctl_RejectsBadHost(t *testing.T) {
	h := &HostSysctl{ProcSys: t.TempDir()}
	for _, ref := range []string{"-oProxyCommand=x:kernel.a=1", "a b:kernel.a=1", "east;id:kernel.a=1"} {
		if _, err := h.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindHostSysctl, Ref: ref}}); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
}

func TestAttestations(t *testing.T) {
	dir := gitRepo(t)
	adir := filepath.Join(dir, "compliance", "attestations")
	if err := os.MkdirAll(adir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(adir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("current.yaml", "statement: Security policy reviewed and approved.\nattested_by: Stevie Bellis\nattested_at: 2026-09-01\nexpires_at: 2027-09-01\n")
	write("expired.yaml", "statement: Old review.\nattested_by: Stevie Bellis\nattested_at: 2024-01-01\nexpires_at: 2025-01-01\n")
	write("future.yaml", "statement: Dated ahead.\nattested_by: x\nattested_at: 2027-01-01\nexpires_at: 2028-01-01\n")
	write("bad.yaml", "statement: x\nsatisfied: true\n")
	cmd := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--no-gpg-sign", "-m", "attest")
	cmd.Dir = dir
	if out, err := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}

	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	signed := map[string]bool{}
	a := &Attestations{RepoDir: dir, Now: func() time.Time { return now },
		Signature: func(_ context.Context, path string) (string, bool, error) { return "abc", signed[path], nil }}
	src := func(n string) crosswalk.Source {
		return crosswalk.Source{Kind: crosswalk.KindAttestation, Ref: "compliance/attestations/" + n}
	}
	all := []crosswalk.Source{src("current.yaml"), src("expired.yaml"), src("future.yaml"), src("missing.yaml")}

	// Unsigned: a claim nobody vouched for fails.
	recs, err := a.Collect(context.Background(), all)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]crosswalk.Record{}
	for _, r := range recs {
		got[r.Source.Ref] = r
	}
	if r := got["compliance/attestations/current.yaml"]; r.Verdict != crosswalk.VerdictFail || !strings.Contains(r.Detail, "not signed") {
		t.Errorf("unsigned current = %+v", r)
	}

	signed["compliance/attestations/current.yaml"] = true
	signed["compliance/attestations/expired.yaml"] = true
	signed["compliance/attestations/future.yaml"] = true
	recs, err = a.Collect(context.Background(), all)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]crosswalk.Record{}
	for _, r := range recs {
		got[r.Source.Ref] = r
	}
	cur := got["compliance/attestations/current.yaml"]
	if cur.Verdict != crosswalk.VerdictPass || !cur.ObservedAt.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) ||
		!strings.Contains(cur.Detail, "Stevie Bellis") || cur.Commit != "abc" {
		t.Errorf("current = %+v (observed_at must be the attestation date, so freshness applies)", cur)
	}
	if r := got["compliance/attestations/expired.yaml"]; r.Verdict != crosswalk.VerdictFail || !strings.Contains(r.Detail, "expired") {
		t.Errorf("expired = %+v", r)
	}
	for _, ref := range []string{"compliance/attestations/future.yaml", "compliance/attestations/missing.yaml"} {
		if r, ok := got[ref]; ok {
			t.Errorf("%s produced %+v; a future-dated or missing attestation is not evidence", ref, r)
		}
	}

	if _, err := a.Collect(context.Background(), []crosswalk.Source{src("bad.yaml")}); err == nil {
		t.Error("attestation with unknown fields accepted")
	}
	for _, ref := range []string{"compliance/attestations/../x.yaml", "/etc/passwd", "compliance/x.yaml"} {
		if _, err := a.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindAttestation, Ref: ref}}); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
}

func TestAttestations_DefaultSignatureReadsGit(t *testing.T) {
	dir := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "f"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--no-gpg-sign", "-m", "f"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	a := &Attestations{RepoDir: dir}
	sha, good, err := a.signature(context.Background(), "f")
	if err != nil || len(sha) != 40 || good {
		t.Fatalf("sha=%q good=%v err=%v: an unsigned commit must not count as signed", sha, good, err)
	}
	if sha, good, err := a.signature(context.Background(), "never-committed"); err != nil || sha != "" || good {
		t.Fatalf("uncommitted path: sha=%q good=%v err=%v", sha, good, err)
	}
}

func TestHostSSHD(t *testing.T) {
	dumps := map[string]string{
		"west": "passwordauthentication no\nkbdinteractiveauthentication yes\nmaxauthtries 3\npermitrootlogin without-password\n",
		"east": "passwordauthentication yes\nmaxauthtries 6\n",
	}
	h := &HostSSHD{Hostname: "west", Dump: func(_ context.Context, host string) (string, error) {
		d, ok := dumps[host]
		if !ok {
			return "", errors.New("no sudo")
		}
		return d, nil
	}}
	src := func(r string) crosswalk.Source { return crosswalk.Source{Kind: crosswalk.KindHostSSHD, Ref: r} }
	recs, err := h.Collect(context.Background(), []crosswalk.Source{
		src("west:passwordauthentication=no"),
		src("west:kbdinteractiveauthentication=no"),
		src("west:maxauthtries<=4"),
		src("east:passwordauthentication=no"),
		src("east:maxauthtries<=4"),
		src("east:x11forwarding=no"),            // key absent from the dump: no record
		src("nosudo:passwordauthentication=no"), // dump unavailable: no record
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]crosswalk.Verdict{}
	for _, r := range recs {
		got[r.Source.Ref] = r.Verdict
		if !strings.HasPrefix(r.Detail, strings.SplitN(r.Source.Ref, ":", 2)[0]+": ") {
			t.Errorf("detail %q does not name its host", r.Detail)
		}
	}
	want := map[string]crosswalk.Verdict{
		"west:passwordauthentication=no":       crosswalk.VerdictPass,
		"west:kbdinteractiveauthentication=no": crosswalk.VerdictFail,
		"west:maxauthtries<=4":                 crosswalk.VerdictPass,
		"east:passwordauthentication=no":       crosswalk.VerdictFail,
		"east:maxauthtries<=4":                 crosswalk.VerdictFail,
	}
	if len(got) != len(want) {
		t.Errorf("records %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestHostSSHD_RejectsBadRefs(t *testing.T) {
	h := &HostSSHD{Dump: func(context.Context, string) (string, error) { return "", nil }}
	for _, ref := range []string{"-oX:a=b", "a b=c", "passwordauthentication", "maxauthtries<=many", "k=v;id"} {
		if _, err := h.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindHostSSHD, Ref: ref}}); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
}

// tagRe finds the section tag HostProbes put in a script.
var tagRe = regexp.MustCompile(`echo @@([0-9a-f]{32}) `)

// tagged answers script with out, its "@@<probe>" headers given the
// script's tag, as a real host's sh would print them.
func tagged(t *testing.T, script, out string) string {
	t.Helper()
	m := tagRe.FindStringSubmatch(script)
	if m == nil {
		t.Fatalf("no section tag in script")
	}
	var b strings.Builder
	for _, l := range strings.SplitAfter(out, "\n") {
		if name, ok := strings.CutPrefix(l, "@@"); ok {
			l = "@@" + m[1] + " " + name
		}
		b.WriteString(l)
	}
	return b.String()
}

func TestHostProbes(t *testing.T) {
	outputs := map[string]string{
		// west: firewall open, NTP synced, no auditd, updates on, AppArmor on
		"west": "@@firewall-inbound-deny\n-P INPUT ACCEPT\n---\n-P INPUT ACCEPT\n" +
			"@@ntp-synchronized\nyes\n@@auditd-running\ninactive\n" +
			"@@unattended-upgrades-enabled\nAPT::Periodic::Unattended-Upgrade \"1\";\ninstalled=unattended-upgrades \ntimer=enabled/active\n@@apparmor-enabled\nY\n",
		// east: firewall v4 drop but v6 accept -> still fails
		"east": "@@firewall-inbound-deny\n-P INPUT DROP\n---\n-P INPUT ACCEPT\n@@auditd-running\nactive\n",
	}
	var scripts []string
	p := &HostProbes{Hostname: "west", Run: func(_ context.Context, host, script string) (string, error) {
		scripts = append(scripts, script)
		o, ok := outputs[host]
		if !ok {
			return "", errors.New("unreachable")
		}
		return tagged(t, script, o), nil
	}}
	src := func(r string) crosswalk.Source { return crosswalk.Source{Kind: crosswalk.KindHostProbe, Ref: r} }
	recs, err := p.Collect(context.Background(), []crosswalk.Source{
		src("west:firewall-inbound-deny"), src("west:ntp-synchronized"), src("west:auditd-running"),
		src("west:unattended-upgrades-enabled"), src("west:apparmor-enabled"),
		src("east:firewall-inbound-deny"), src("east:auditd-running"),
		src("east:ntp-synchronized"), // no section in the output: no record
		src("gone:auditd-running"),   // unreachable: no record
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]crosswalk.Verdict{}
	for _, r := range recs {
		got[r.Source.Ref] = r.Verdict
	}
	want := map[string]crosswalk.Verdict{
		"west:firewall-inbound-deny":       crosswalk.VerdictFail,
		"west:ntp-synchronized":            crosswalk.VerdictPass,
		"west:auditd-running":              crosswalk.VerdictFail,
		"west:unattended-upgrades-enabled": crosswalk.VerdictPass,
		"west:apparmor-enabled":            crosswalk.VerdictPass,
		"east:firewall-inbound-deny":       crosswalk.VerdictFail,
		"east:auditd-running":              crosswalk.VerdictPass,
	}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	// One script per host, built only from the fixed probe table.
	if len(scripts) != 3 {
		t.Errorf("%d scripts, want one per host", len(scripts))
	}
}

// TestHostProbesIgnoreForgedSections: probe output carries names other
// users choose. A file in a home named ".x\n@@acct-shadow-no-empty\n@ok"
// (find -printf prints it raw) once replaced that probe's section with
// "@ok", passing a host with an empty password.
func TestHostProbesIgnoreForgedSections(t *testing.T) {
	src := func(r string) crosswalk.Source { return crosswalk.Source{Kind: crosswalk.KindHostProbe, Ref: r} }
	sources := []crosswalk.Source{src("west:acct-shadow-no-empty"), src("west:dot-files"), src("west:auditd-running")}
	var tags []string
	collectWith := func(forge func(tag string) string) map[string]crosswalk.Verdict {
		p := &HostProbes{Hostname: "west", Run: func(_ context.Context, _, script string) (string, error) {
			m := tagRe.FindStringSubmatch(script)
			if m == nil {
				t.Fatal("no section tag in script")
			}
			tags = append(tags, m[1])
			h := "@@" + m[1] + " "
			return h + "acct-shadow-no-empty\ngovan\n@ok\n" +
				h + "dot-files\ndot govan govan 644 /home/govan/.x\n" + forge(m[1]) + "\n@ok\n" +
				h + "auditd-running\nactive\n", nil
		}}
		recs, err := p.Collect(context.Background(), sources)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]crosswalk.Verdict{}
		for _, r := range recs {
			got[r.Source.Ref] = r.Verdict
		}
		return got
	}
	// An untagged header is content: the empty password still fails.
	got := collectWith(func(string) string { return "@@acct-shadow-no-empty\n@ok" })
	if got["west:acct-shadow-no-empty"] != crosswalk.VerdictFail {
		t.Errorf("forged untagged section: acct-shadow-no-empty = %q, want fail", got["west:acct-shadow-no-empty"])
	}
	if got["west:auditd-running"] != crosswalk.VerdictPass {
		t.Errorf("auditd-running = %q, want pass", got["west:auditd-running"])
	}
	// A header with the real tag twice: neither copy is believed.
	got = collectWith(func(tag string) string { return "@@" + tag + " acct-shadow-no-empty\n@ok" })
	if v, ok := got["west:acct-shadow-no-empty"]; ok {
		t.Errorf("duplicate section judged: acct-shadow-no-empty = %q, want no record", v)
	}
	if len(tags) != 2 || tags[0] == tags[1] {
		t.Errorf("tags %v: want a fresh one per run", tags)
	}
}

// TestRunScriptReadsStdin: the script reaches sh on stdin, so its section
// tag is in no process's argv.
func TestRunScriptReadsStdin(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	out, err := RunScript(context.Background(), "local", "echo \"$0 $#\"; echo second\nexit 0\n", true)
	if err != nil {
		t.Fatal(err)
	}
	if out != "sh 0\nsecond\n" {
		t.Errorf("RunScript = %q, want the script run from stdin with no arguments", out)
	}
}

func TestHostProbes_RejectsUnknownProbeAndBadHost(t *testing.T) {
	p := &HostProbes{Run: func(context.Context, string, string) (string, error) { return "", nil }}
	for _, ref := range []string{"west:rm -rf /", "west:nosuchprobe", "-oX:auditd-running", "a b:auditd-running"} {
		if _, err := p.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindHostProbe, Ref: ref}}); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
}

func TestProbeJudges(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"db-backup-recent", "2026-09-29 cluster.sql.gz\n---\n2026-09-29\n", true},
		{"db-backup-recent", "2026-10-14 2026-10-14/well-123313Z/SHA256SUMS\n---\n2026-10-14\n", true},
		{"db-backup-recent", "2026-10-15 scheduled\n---\n2026-10-15\n", true},
		{"db-backup-recent", "\n---\n2026-09-22\n", false},
		{"db-backup-recent", "\n---\n\n", false},
		{"db-backup-recent", "find: '/srv/backups': Permission denied\n---\n\n", false},
		{"firewall-inbound-deny", "-P INPUT DROP\n---\n-P INPUT DROP\n", true},
		{"firewall-inbound-deny", "-P INPUT DROP\n---\n", false},
		// the package-owned nft table (ADR-098 step 6) decides even with iptables at ACCEPT
		{"firewall-inbound-deny", "-P INPUT ACCEPT\n---\n-P INPUT ACCEPT\n---\n\t\ttype filter hook input priority filter; policy drop;\n", true},
		{"firewall-inbound-deny", "-P INPUT ACCEPT\n---\n-P INPUT ACCEPT\n---\n\t\ttype filter hook input priority filter; policy accept;\n", false},
		{"firewall-inbound-deny", "-P INPUT ACCEPT\n---\n-P INPUT ACCEPT\n---\n", false},
		{"unattended-upgrades-enabled", "APT::Periodic::Unattended-Upgrade \"0\";\n", false},
		{"unattended-upgrades-enabled", "APT::Periodic::Unattended-Upgrade \"1\";\ninstalled=unattended-upgrades \npresent=unattended-upgrades \ndpkg=installed\ntimer=enabled/active\n", true},
		// The setting alone once passed: nothing installs updates without the
		// package or with the timer that runs it masked.
		{"unattended-upgrades-enabled", "APT::Periodic::Unattended-Upgrade \"1\";\ninstalled=\npresent=\ndpkg=installed\ntimer=enabled/active\n", false},
		{"unattended-upgrades-enabled", "APT::Periodic::Unattended-Upgrade \"1\";\ninstalled=unattended-upgrades \npresent=unattended-upgrades \ndpkg=installed\ntimer=masked/inactive\n", false},
		{"unattended-upgrades-enabled", "APT::Periodic::Unattended-Upgrade \"1\";\ninstalled=unattended-upgrades \ntimer=/\n", false}, // systemctl did not answer
		{"unattended-upgrades-enabled", "APT::Periodic::Unattended-Upgrade \"0\";\nAPT::Periodic::Unattended-Upgrade::Note \"1\";\ninstalled=unattended-upgrades \ntimer=enabled/active\n", false},
		{"apparmor-enabled", "N\n", false},
		{"ntp-synchronized", "no\n", false},
	} {
		pass, detail := Probes[tc.probe].Judge(tc.out)
		if pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestKernelModuleProbes(t *testing.T) {
	for _, tc := range []struct {
		name, out string
		pass      bool
	}{
		{"disabled and blacklisted", "loaded=0\nloadable=install /bin/false \nblacklist=blacklist cramfs\n", true},
		{"not built for this kernel", "loaded=0\nloadable=modprobe: FATAL: Module cramfs not found in directory /lib/modules/6.17 \nblacklist=\n", true},
		{"loadable", "loaded=0\nloadable=insmod /lib/modules/6.17/kernel/fs/cramfs/cramfs.ko.zst \nblacklist=\n", false},
		{"install false but not blacklisted", "loaded=0\nloadable=install /bin/false \nblacklist=\n", false},
		{"loaded right now", "loaded=1\nloadable=install /bin/false \nblacklist=blacklist cramfs\n", false},
		{"built into the kernel", "loaded=0\nloadable=builtin cramfs \nblacklist=blacklist cramfs\n", false},
		{"no output", "", false},
		// The collector's user unit had no /usr/sbin on PATH: the shell's
		// own "not found" for modprobe passed every module as absent.
		{"modprobe not on PATH", "loaded=0\nloadable=sh: 1: modprobe: not found \nblacklist=\n", false},
		{"other module not found", "loaded=0\nloadable=modprobe: FATAL: Module mtd not found in directory /lib/modules/6.17 \nblacklist=\n", false},
		// modprobe -n -v prints dependencies first (west, 2026-10-23).
		{"dependency, then install false", "loaded=0\nloadable=insmod /lib/modules/6.17/kernel/drivers/mtd/mtd.ko.zst  install /bin/false  \nblacklist=blacklist cramfs\n", true},
		// An install command that falls through to a real load once passed
		// because it contained "install /bin/false" (modprobe -C output).
		{"install false or load", "loaded=0\nloadable=insmod /lib/modules/6.17/kernel/drivers/mtd/mtd.ko.zst  install /bin/false || /sbin/modprobe --ignore-install cramfs  \nblacklist=blacklist cramfs\n", false},
		{"install false then load", "loaded=0\nloadable=install /bin/false; /sbin/modprobe --ignore-install cramfs \nblacklist=blacklist cramfs\n", false},
		{"install true", "loaded=0\nloadable=install /bin/true \nblacklist=blacklist cramfs\n", true},
	} {
		pass, detail := Probes["kmod-cramfs-disabled"].Judge(tc.out)
		if pass != tc.pass {
			t.Errorf("%s: pass=%v (%s), want %v", tc.name, pass, detail, tc.pass)
		}
	}
	for _, m := range []string{"cramfs", "freevxfs", "hfs", "hfsplus", "jffs2", "usb-storage", "udf"} {
		p, ok := Probes["kmod-"+m+"-disabled"]
		if !ok || !p.Baseline {
			t.Errorf("kmod-%s-disabled missing or not a baseline probe", m)
			continue
		}
		if m == "usb-storage" && !strings.Contains(p.Command, "^usb_storage ") {
			t.Errorf("usb-storage is usb_storage in /proc/modules: %s", p.Command)
		}
	}
}

func TestFilePermissionProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"perm-etc-passwd", "/etc/passwd 644 0 root\n", true},
		{"perm-etc-passwd", "/etc/passwd 600 0 root\n", true}, // stricter is fine
		{"perm-etc-passwd", "/etc/passwd 664 0 root\n", false},
		{"perm-etc-passwd", "/etc/passwd 644 1000 root\n", false},
		{"perm-etc-passwd", "/etc/passwd 644 0 adm\n", false},
		{"perm-etc-passwd", "/etc/passwd ABSENT\n", false},
		{"perm-etc-passwd", "", false},
		{"perm-etc-shadow", "/etc/shadow 640 0 shadow\n", true},
		{"perm-etc-shadow", "/etc/shadow 640 0 root\n", false},
		{"perm-etc-shadow", "/etc/shadow 644 0 shadow\n", false},
		{"perm-etc-security-opasswd", "/etc/security/opasswd ABSENT\n", true}, // nothing to protect
		{"perm-sshd-config", "/etc/ssh/sshd_config 600 0 root\n/etc/ssh/sshd_config.d/50-cloud-init.conf 600 0 root\n", true},
		{"perm-sshd-config", "/etc/ssh/sshd_config 644 0 root\n/etc/ssh/sshd_config.d/* ABSENT\n", false},
		{"perm-sshd-config", "/etc/ssh/sshd_config 600 0 root\n/etc/ssh/sshd_config.d/* ABSENT\n", true},
		// sshd_config.d could not be listed: its glob stayed unexpanded and
		// once read as "no drop-ins".
		{"perm-sshd-config", "/etc/ssh/sshd_config 600 0 root\n@unreadable /etc/ssh/sshd_config.d\n/etc/ssh/sshd_config.d/* ABSENT\n", false},
		{"perm-ssh-host-private-keys", "/etc/ssh/ssh_host_ed25519_key 600 0 root\n/etc/ssh/ssh_host_rsa_key 640 0 root\n", false},
		{"perm-ssh-host-public-keys", "/etc/ssh/ssh_host_ed25519_key.pub 644 0 root\n", true},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

// Every probe command is valid sh inside the wrapper HostProbes builds
// ("{ cmd; } 2>/dev/null"): a syntax error loses the whole host's script,
// every probe on it, silently (no records, not failures).
func TestProbeCommandsParse(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	for name, p := range Probes {
		script := fmt.Sprintf("echo @@%s; { %s; } 2>/dev/null\nexit 0\n", name, p.Command)
		if out, err := exec.Command("sh", "-n", "-c", script).CombinedOutput(); err != nil {
			t.Errorf("%s: sh -n: %v %s\n%s", name, err, out, p.Command)
		}
	}
}

func TestMountProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"mount-tmp-separate", "/tmp tmpfs rw,nosuid,nodev,inode64\n", true},
		{"mount-tmp-noexec", "/tmp tmpfs rw,nosuid,nodev,inode64\n", false},
		{"mount-tmp-nosuid", "/tmp tmpfs rw,nosuid,nodev,inode64\n", true},
		{"mount-var-tmp-separate", "/var /dev/sda rw,relatime\n", false},
		{"mount-var-tmp-nodev", "/var /dev/sda rw,relatime\n", false}, // judged by /var's options
		{"mount-var-tmp-nodev", "/var /dev/sda rw,nodev,relatime\n", true},
		{"mount-home-nosuid", "/ /dev/sdb2 rw,relatime\n", false},
		{"mount-var-log-audit-separate", "ABSENT\n", false},
		{"mount-var-log-audit-noexec", "", false},
		{"mount-dev-shm-noexec", "/dev/shm tmpfs rw,nosuid,nodev,noexec\n", true},
		{"mount-tmp-nodev", "/tmp tmpfs rw,nodevice\n", false},                                // whole option names only
		{"mount-var-log-separate", "/var/log /dev/sda[/log] rw,nodev,nosuid,noexec\n", false}, // bind mount
		{"mount-var-log-separate", "/var/log /dev/sdc1 rw,nodev,nosuid,noexec\n", true},
		{"mount-var-log-nodev", "/var/log /dev/sda[/log] rw,nodev,nosuid,noexec\n", true}, // options still count
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestServiceClientCronProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"svc-web-server-not-in-use", "installed=\npresent=\ndpkg=installed\nnginx.service=not-found/inactive\napache2.service=not-found/inactive\n", true},
		{"svc-web-server-not-in-use", "installed=nginx\npresent=nginx\ndpkg=installed\nnginx.service=enabled/active\napache2.service=not-found/inactive\n", false},
		{"svc-web-server-not-in-use", "installed=nginx\npresent=nginx\ndpkg=installed\nnginx.service=masked/inactive\napache2.service=not-found/inactive\n", true},  // installed as a dependency, masked
		{"svc-web-server-not-in-use", "installed=nginx\npresent=nginx\ndpkg=installed\nnginx.service=disabled/active\napache2.service=not-found/inactive\n", false}, // running now
		{"svc-rsync-not-in-use", "installed=rsync\npresent=rsync\ndpkg=installed\nrsync.service=disabled/inactive\n", true},
		{"svc-rsync-not-in-use", "", false},
		{"pkg-telnet-client-absent", "installed=inetutils-telnet telnet\npresent=inetutils-telnet telnet\ndpkg=installed\n", false},
		{"pkg-telnet-client-absent", "installed=\npresent=\ndpkg=installed\n", true},
		{"pkg-telnet-client-absent", "installed=\npresent=telnet\ndpkg=installed\n", false}, // held or unpacked
		{"pkg-telnet-client-absent", "installed=\npresent=\n", false},                       // dpkg's database not read
		{"pkg-telnet-client-absent", "installed=\npresent=\ndpkg=\n", false},
		{"svc-rsync-not-in-use", "installed=\npresent=\ndpkg=\n", false},
		// systemd did not answer: nothing says the unit is stopped.
		{"svc-web-server-not-in-use", "installed=nginx\npresent=nginx\ndpkg=installed\nnginx.service=/\napache2.service=/\n", false},
		{"svc-rsync-not-in-use", "installed=rsync\npresent=rsync\ndpkg=installed\n", false},
		{"svc-rsync-not-in-use", "installed=rsync\npresent=rsync\ndpkg=installed\nrsync.service=generated/inactive\n", false},
		{"svc-rsync-not-in-use", "installed=rsync\npresent=rsync\ndpkg=installed\nrsync.service=disabled/deactivating\n", false},
		{"svc-rsync-not-in-use", "installed=rsync\npresent=rsync\ndpkg=installed\nrsync.service=not-found/inactive\n", true},
		{"svc-rsync-not-in-use", "installed=rsync\npresent=rsync\ndpkg=installed\nrsync.service=masked/failed\n", true},
		{"at-restricted", "installed=\npresent=\ndpkg=\n", false},
		{"pkg-telnet-client-absent", "", false},
		{"cron-active", "installed=cron\nenabled=enabled\nactive=active\n", true},
		{"cron-active", "installed=cron\nenabled=enabled\nactive=inactive\n", false},
		{"cron-active", "installed=\nenabled=\nactive=\n", false},
		{"cron-allow-restricted", "allow=/etc/cron.allow 640 0 crontab\ndeny=ABSENT\n", true},
		{"cron-allow-restricted", "allow=ABSENT\ndeny=ABSENT\n", false},
		{"cron-allow-restricted", "allow=/etc/cron.allow 640 0 crontab\ndeny=/etc/cron.deny 640 0 root\n", false},
		{"cron-allow-restricted", "allow=/etc/cron.allow 644 0 crontab\ndeny=ABSENT\n", false},
		{"at-restricted", "installed=\npresent=\ndpkg=installed\n", true}, // at not installed: nothing to restrict
		{"at-restricted", "installed=at\npresent=at\ndpkg=installed\nallow=ABSENT\ndeny=ABSENT\n", false},
		{"at-restricted", "installed=at\npresent=at\ndpkg=installed\nallow=/etc/at.allow 640 0 root\ndeny=/etc/at.deny 640 0 root\n", true},
		{"mta-local-only", "@ok\n", true}, // nothing listening on 25/465/587
		{"mta-local-only", "@ok\n127.0.0.1:25\n[::1]:25\n", true},
		{"mta-local-only", "@ok\n0.0.0.0:25\n", false},
		{"mta-local-only", "", false}, // ss did not run: not "nothing listening"
		{"perm-etc-crontab", "/etc/crontab 644 0 root\n", false},
		{"perm-etc-cron-d", "/etc/cron.d 700 0 root\n", true},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestAccountSudoBootProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"acct-passwd-shadowed", "@ok\n", true},
		{"acct-passwd-shadowed", "legacy\n@ok\n", false},
		{"acct-passwd-shadowed", "", false}, // no completion marker: never "no offenders"
		{"acct-shadow-no-empty", "@ok\n", true},
		{"acct-shadow-no-empty", "guest\n@ok\n", false},
		{"acct-shadow-no-empty", "sudo: a password is required\n", false},
		{"acct-unique-uid", "@ok\n", true},
		{"acct-unique-uid", "1000\n@ok\n", false},
		{"acct-shadow-group-empty", "@ok\n", true},
		{"acct-shadow-group-empty", "govan\n@ok\n", false},
		{"apparmor-installed", "installed=apparmor apparmor-utils\n", true},
		{"apparmor-installed", "installed=apparmor\n", false},
		{"apparmor-profiles-enforced", `{"version": "2", "profiles": {"a": "enforce", "b": "complain"}, "processes": {}}` + "\n", true},
		{"apparmor-profiles-enforced", `{"version": "2", "profiles": {"a": "enforce", "b": "unconfined"}, "processes": {}}` + "\n", false},
		{"apparmor-profiles-enforced", `{"version": "2", "profiles": {}, "processes": {}}` + "\n", false},
		{"apparmor-profiles-enforced", "garbage\n", false},
		{"grub-password", "set superusers=\"root\"\npassword_pbkdf2 root grub.pbkdf2.sha512.10000.X\n@ok\n", true},
		{"grub-password", "@ok\n", false},
		{"perm-boot-grub-cfg", "/boot/grub/grub.cfg 400 0 root\n", true},
		{"perm-boot-grub-cfg", "/boot/grub/grub.cfg 444 0 root\n", false},
		{"pkg-prelink-absent", "installed=\npresent=\ndpkg=installed\n", true},
		{"svc-apport-not-in-use", "installed=apport\npresent=apport\ndpkg=installed\napport.service=enabled/active\n", false},
		{"sudo-installed", "installed=sudo\n", true},
		{"sudo-use-pty", "Defaults use_pty\n@ok\n", true},
		{"sudo-use-pty", "Defaults env_reset,use_pty,mail_badpass\n@ok\n", true},
		{"sudo-use-pty", "Defaults !use_pty\n@ok\n", false},
		{"sudo-use-pty", "Defaults env_reset\n@ok\n", false},
		{"sudo-use-pty", "", false},
		{"sudo-logfile", "Defaults logfile=\"/var/log/sudo.log\"\n@ok\n", true},
		{"sudo-logfile", "Defaults env_reset\n@ok\n", false},
		{"sudo-no-noauth", "Defaults env_reset\n@ok\n", true},
		{"sudo-no-noauth", "govan ALL=(ALL) !authenticate: ALL\n@ok\n", false},
		{"sudo-no-noauth", "Defaults:admin !authenticate\n@ok\n", false},
		{"sudo-timeout", "Defaults timestamp_timeout=15\n@ok\n", true},
		{"sudo-timeout", "Defaults env_reset, timestamp_timeout=5\n@ok\n", true},
		{"sudo-timeout", "Defaults timestamp_timeout=-1\n@ok\n", false},
		{"sudo-timeout", "Defaults timestamp_timeout=30\n@ok\n", false},
		{"sudo-timeout", "Defaults env_reset\n@ok\n", false}, // ComplianceAsCode wants it set explicitly
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestPasswordPolicyProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"pw-max-days", "defs=365\n@ok\n", true},
		{"pw-max-days", "defs=99999\n@ok\n", false},
		{"pw-max-days", "defs=365\nuser govan\n@ok\n", false}, // an existing hashed account without a max age
		{"pw-max-days", "defs=365\n", false},                  // did not complete
		{"pw-warn-age", "defs=7\n@ok\n", true},
		{"pw-warn-age", "defs=3\n@ok\n", false},
		{"pw-warn-age", "defs=\n@ok\n", false},
		{"pw-hash-algo", "defs=YESCRYPT\n@ok\n", true},
		{"pw-hash-algo", "defs=SHA512\n@ok\n", true},
		{"pw-hash-algo", "defs=MD5\n@ok\n", false},
		{"pw-inactive", "defs=30\n@ok\n", true},
		{"pw-inactive", "defs=-1\n@ok\n", false},
		{"pw-inactive", "defs=45\nuser govan\n@ok\n", false},
		{"pw-change-past", "@ok\n", true},
		{"pw-change-past", "govan\n@ok\n", false},
		{"acct-root-only-uid0", "@ok\n", true},
		{"acct-root-only-uid0", "toor\n@ok\n", false},
		{"acct-system-no-shell", "@ok\n", true},
		{"acct-system-no-shell", "www-data /bin/bash\n@ok\n", false},
		{"acct-noshell-locked", "@ok\n", true},
		{"acct-noshell-locked", "daemon P\n@ok\n", false},
		{"shell-tmout", "readonly TMOUT=900 ; export TMOUT\n@ok\n", true},
		{"shell-tmout", "TMOUT=900\nreadonly TMOUT\nexport TMOUT\n@ok\n", true},
		{"shell-tmout", "TMOUT=900\nexport TMOUT\n@ok\n", false}, // not readonly: a user can unset it
		{"shell-tmout", "readonly TMOUT=1800 ; export TMOUT\n@ok\n", false},
		{"shell-tmout", "@ok\n", false},
		{"journald-active", "active\n", true},
		{"journald-active", "inactive\n", false},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestDefsProbeOnlyClaimsWhatItChecks(t *testing.T) {
	if _, d := Probes["pw-hash-algo"].Judge("defs=YESCRYPT\n@ok\n"); strings.Contains(d, "existing accounts") {
		t.Errorf("pw-hash-algo does not check accounts but says %q", d)
	}
	if _, d := Probes["pw-max-days"].Judge("defs=365\n@ok\n"); !strings.Contains(d, "existing accounts") {
		t.Errorf("pw-max-days checks accounts but says %q", d)
	}
}

func TestSSHDCryptoProbes(t *testing.T) {
	const port = "port 22\n"
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"sshd-ciphers", port + "ciphers chacha20-poly1305@openssh.com,aes256-gcm@openssh.com,aes192-ctr\n", true},
		{"sshd-ciphers", port + "ciphers aes256-cbc,aes256-ctr\n", false},
		{"sshd-ciphers", "", false}, // sshd -T gave nothing (sudo refused): never a pass
		{"sshd-macs", port + "macs hmac-sha2-512-etm@openssh.com,hmac-sha2-256\n", true},
		{"sshd-macs", port + "macs umac-64-etm@openssh.com,hmac-sha2-256\n", false},
		{"sshd-kex", port + "kexalgorithms mlkem768x25519-sha256,sntrup761x25519-sha512,curve25519-sha256\n", true}, // documented PQ additions
		{"sshd-kex", port + "kexalgorithms diffie-hellman-group1-sha1,curve25519-sha256\n", false},
		{"sshd-banner", port + "banner /etc/issue.net\n", true},
		{"sshd-banner", port + "banner none\n", false},
		{"sshd-access", port + "allowgroups sshusers\n", true},
		{"sshd-access", port, false},
		{"sshd-maxstartups", port + "maxstartups 10:30:60\n", true},
		{"sshd-maxstartups", port + "maxstartups 5:50:40\n", true}, // stricter on every field
		{"sshd-maxstartups", port + "maxstartups 10:30:100\n", false},
		{"sshd-maxstartups", port + "maxstartups 10\n", false},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestBannerRadioProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"banner-issue", "os=Ubuntu\nAuthorized users only. All activity may be monitored and reported.\n@ok\n", true},
		{"banner-issue", "os=Ubuntu\nUbuntu 25.10 \\n \\l\n@ok\n", false},
		{"banner-issue", "os=Ubuntu\nWelcome to \\s \\r\n@ok\n", false},
		{"banner-issue", "os=Ubuntu\n@ok\n", false}, // must be set
		{"banner-issue", "os=Ubuntu\nABSENT\n@ok\n", false},
		{"banner-motd", "os=Ubuntu\nABSENT\n@ok\n", true}, // motd is optional
		{"banner-motd", "os=Ubuntu\nubuntu news here\n@ok\n", false},
		{"banner-motd", "os=Ubuntu\n@unreadable /etc/motd\n@ok\n", false},  // once "states no OS details"
		{"banner-issue-net", "os=Ubuntu\nAuthorized users only.\n", false}, // no completion marker
		{"svc-bluetooth-not-in-use", "installed=\npresent=\ndpkg=installed\n", true},
		{"radio-wireless-down", "@ok\n", true},
		{"radio-wireless-down", "wlp17s0 up\n@ok\n", false},
		{"perm-etc-issue", "/etc/issue 644 0 root\n", true},
		{"perm-etc-motd", "/etc/motd ABSENT\n", true},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestAIDEProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"aide-installed", "installed=aide\ndb=present\n@ok\n", true},
		{"aide-installed", "installed=aide\ndb=absent\n@ok\n", false}, // never initialised
		{"aide-installed", "installed=\ndb=absent\n@ok\n", false},
		{"aide-installed", "installed=aide\n", false}, // the sudo'd database check did not run
		{"aide-timer", "dailyaidecheck.timer enabled\n@ok\n", true},
		{"aide-timer", "dailyaidecheck.timer disabled\n@ok\n", false},
		{"aide-timer", "@ok\n", false},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestLoggingProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"rsyslog-active", "installed=rsyslog\nenabled=enabled\nactive=active\n", true},
		{"rsyslog-active", "installed=rsyslog\nenabled=enabled\nactive=inactive\n", false},
		{"logging-single-system", "rsyslog=active\nupload=inactive\n", true},
		{"logging-single-system", "rsyslog=active\nupload=active\n", false},
		{"logging-single-system", "rsyslog=inactive\nupload=inactive\n", false},
		{"journald-forward-syslog", "ForwardToSyslog=no\nForwardToSyslog=yes\n@ok\n", true}, // last one wins
		{"journald-forward-syslog", "ForwardToSyslog=yes\nForwardToSyslog=no\n@ok\n", false},
		{"journald-forward-syslog", "@ok\n", false}, // journald's default is no
		{"rsyslog-filecreatemode", "$FileCreateMode 0640\n@ok\n", true},
		{"rsyslog-filecreatemode", "$FileCreateMode 0600\n@ok\n", true},
		{"rsyslog-filecreatemode", "$FileCreateMode 0644\n@ok\n", false},
		{"rsyslog-filecreatemode", "@ok\n", false}, // rsyslog's default is 0644
		{"rsyslog-no-listen", "@ok\n", true},
		{"rsyslog-no-listen", "module(load=\"imtcp\")\n@ok\n", false},
		{"rsyslog-no-listen", "$ModLoad imudp\n@ok\n", false},
		{"rsyslog-no-listen", "", false},
		{"rsyslog-no-listen", "@unreadable /etc/rsyslog.d/50-remote.conf\n@ok\n", false},
		{"journald-file-access", "@ok\n", true},
		{"journald-file-access", "/var/log/journal/x/system.journal\n@ok\n", false},
		{"journald-file-access", "", false},
		{"journald-file-access", "unreadable /var/log/journal\n@ok\n", false}, // find could not list it
		{"journald-rotation", "SystemMaxUse=1G\nRuntimeMaxUse=200M\n@ok\n", true},
		{"journald-rotation", "SystemMaxUse=1G\n@ok\n", false},
		{"journald-rotation", "SystemMaxUse=1G\nRuntimeMaxUse=200M\nSystemMaxUse=\n@ok\n", false}, // last one wins: reset to default
		{"journald-rotation", "@ok\n", false},
		{"logrotate-active", "installed=logrotate\nenabled=enabled\nactive=active\n", true},
		{"logrotate-active", "installed=logrotate\nenabled=disabled\nactive=inactive\n", false},
		{"logrotate-active", "installed=\nenabled=enabled\nactive=active\n", false},
		{"var-log-perms", "640 /var/log/syslog\n600 /var/log/x.log\n@ok\n", true},
		{"var-log-perms", "664 /var/log/wtmp\n660 /var/log/btmp.1\n664 /var/log/lastlog\n@ok\n", true},
		{"var-log-perms", "644 /var/log/cloud-init.log\n660 /var/log/sssd/sssd.log\n640 /var/log/journal/x/a.journal\n@ok\n", true},
		{"var-log-perms", "644 /var/log/dpkg.log\n@ok\n", false},
		{"var-log-perms", "666 /var/log/rocm_smi_lib/ROCm-SMI-lib.log\n@ok\n", false},
		{"var-log-perms", "664 /var/log/wtmpx\n@ok\n", false}, // not a login record, despite the prefix
		{"var-log-perms", "666 /var/log/wtmp\n@ok\n", false},  // login records may not be world-writable
		{"var-log-perms", "640 /var/log/syslog\n", false},     // did not complete
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestPAMProbes(t *testing.T) {
	const stock = "%%pam\n" +
		"auth [success=1 default=ignore] pam_unix.so nullok\n" +
		"account [success=1 new_authtok_reqd=done default=ignore] pam_unix.so\n" +
		"password [success=1 default=ignore] pam_unix.so obscure yescrypt\n" +
		"session required pam_unix.so\n" +
		"%%pwq\n# minlen = 9\n%%fl\n%%ph\n@ok\n"
	const hardened = "%%pam\n" +
		"auth requisite pam_faillock.so preauth\n" +
		"auth [success=1 default=ignore] pam_unix.so\n" +
		"auth [default=die] pam_faillock.so authfail\n" +
		"account required pam_faillock.so\n" +
		"account [success=1 default=ignore] pam_unix.so\n" +
		"password requisite pam_pwquality.so retry=3\n" +
		"password requisite pam_pwhistory.so remember=24 enforce_for_root use_authtok\n" +
		"password [success=1 default=ignore] pam_unix.so obscure use_authtok yescrypt\n" +
		"session required pam_unix.so\n" +
		"%%pwq\nminlen = 14\ndifok = 2\nmaxrepeat = 3\nmaxsequence = 3\nenforce_for_root\n" +
		"%%fl\ndeny = 4\nunlock_time = 900\n%%ph\n@ok\n"
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"pam-unix-enabled", stock, true},
		{"pam-faillock-enabled", stock, false},
		{"pam-faillock-enabled", hardened, true},
		{"pam-pwquality-enabled", hardened, true},
		{"pam-pwhistory-enabled", stock, false},
		{"pam-faillock-deny", stock, false}, // default deny=3, but faillock is not enabled
		{"pam-faillock-deny", hardened, true},
		{"pam-faillock-unlock", stock, false},
		{"pam-faillock-unlock", hardened, true},
		{"pam-pwq-difok", hardened, true},
		{"pam-pwq-minlen", stock, false}, // a commented minlen is not a setting
		{"pam-pwq-minlen", hardened, true},
		{"pam-pwq-maxrepeat", hardened, true},
		{"pam-pwq-maxsequence", stock, false},
		{"pam-pwq-dictcheck", stock, false}, // default 1, but pwquality is not enabled
		{"pam-pwq-dictcheck", hardened, true},
		{"pam-pwq-enforcing", stock, false},
		{"pam-pwq-enforcing", hardened, true},
		{"pam-pwq-enforce-root", hardened, true},
		{"pam-pwhistory-remember", hardened, true},
		{"pam-pwhistory-remember", stock, false},
		{"pam-pwhistory-enforce-root", hardened, true},
		{"pam-pwhistory-authtok", hardened, true},
		{"pam-unix-no-nullok", stock, false},
		{"pam-unix-no-nullok", hardened, true},
		{"pam-unix-no-remember", stock, true},
		{"pam-unix-strong-hash", stock, true},
		{"pam-unix-authtok", stock, false},
		{"pam-unix-authtok", hardened, true},
		{"pam-pwq-minlen", "%%pam\n", false}, // incomplete
		// common-auth (where nullok lives) unread: the rest of the stack once
		// passed no-nullok and no-remember.
		{"pam-unix-no-nullok", strings.Replace(hardened, "%%pam\n", "%%pam\n@unreadable /etc/pam.d/common-auth\n", 1), false},
		{"pam-unix-no-remember", strings.Replace(stock, "%%pam\n", "%%pam\n@unreadable /etc/pam.d/common-auth\n", 1), false},
		// an argument on the PAM line overrides the conf file
		{"pam-pwq-minlen", strings.Replace(hardened, "pam_pwquality.so retry=3", "pam_pwquality.so retry=3 minlen=8", 1), false},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s = %v (%s), want %v", tc.probe, pass, detail, tc.pass)
		}
	}
}

// No probe's own output may start a line with "@@": HostProbes uses that to
// split one host's script output into probe sections.
func TestProbeCommandsDoNotEmitSectionMarkers(t *testing.T) {
	for name, p := range Probes {
		if strings.Contains(p.Command, "echo @@") || strings.Contains(p.Command, "echo \"@@") {
			t.Errorf("%s prints an @@ line, which HostProbes reads as a new probe section", name)
		}
	}
}

func TestTimeSyncProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"time-single-daemon", "chrony=active\ntimesyncd=inactive\n", true},
		{"time-single-daemon", "chrony=active\ntimesyncd=active\n", false},
		{"time-single-daemon", "chrony=inactive\ntimesyncd=inactive\n", false},
		{"chrony-sources", "pool ntp.ubuntu.com iburst maxsources 4\n@ok\n", true},
		{"chrony-sources", "@ok\n", false},
		{"chrony-user", "_chrony\n_chrony\n@ok\n", true},
		{"chrony-user", "root\n_chrony\n@ok\n", false},
		{"chrony-user", "@ok\n", false},
		{"chrony-active", "enabled=enabled\nactive=active\n", true},
		{"chrony-active", "enabled=disabled\nactive=active\n", false},
		{"pkg-gdm-absent", "installed=\npresent=\ndpkg=installed\n", true},
		{"pkg-gdm-absent", "installed=gdm3\npresent=gdm3\ndpkg=installed\n", false},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestVarLogPermsDetail(t *testing.T) {
	var out strings.Builder
	for i := 0; i < 7; i++ {
		fmt.Fprintf(&out, "644 /var/log/f%d.log\n", i)
	}
	out.WriteString("@ok\n")
	pass, detail := Probes["var-log-perms"].Judge(out.String())
	if pass || !strings.HasPrefix(detail, "7 files looser than allowed: /var/log/f0.log 0644 (max 0640)") ||
		!strings.HasSuffix(detail, "and 2 more") {
		t.Fatalf("detail = %q", detail)
	}
}

func TestRootAndUserEnvironmentProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"su-restricted", "pam auth required pam_wheel.so use_uid group=sugroup\ngrp sugroup:x:1050:\n@ok\n", true},
		{"su-restricted", "pam auth required pam_wheel.so use_uid group=sugroup\ngrp sugroup:x:1050:govan\n@ok\n", false}, // group has a member
		{"su-restricted", "pam auth required pam_wheel.so use_uid group=sugroup\n@ok\n", false},                           // group missing
		{"su-restricted", "pam auth required pam_wheel.so group=sugroup\ngrp sugroup:x:1050:\n@ok\n", false},              // no use_uid
		{"su-restricted", "pam auth sufficient pam_wheel.so trust\n@ok\n", false},
		{"su-restricted", "@ok\n", false},
		{"root-access-controlled", "status L\n@ok\n", true},
		{"root-access-controlled", "status P\n@ok\n", true},
		{"root-access-controlled", "status NP\n@ok\n", false},
		{"root-access-controlled", "@ok\n", false},
		{"root-path", "path /usr/sbin:/usr/bin\ndir root 755 /usr/sbin\ndir root 755 /usr/bin\n@ok\n", true},
		{"root-path", "path /usr/sbin::/usr/bin\ndir root 755 /usr/sbin\ndir root 755 /usr/bin\n@ok\n", false}, // empty entry
		{"root-path", "path .:/usr/bin\ndir root 755 /usr/bin\n@ok\n", false},
		{"root-path", "path /usr/bin:/opt/x\ndir root 755 /usr/bin\ndir govan 755 /opt/x\n@ok\n", false},
		{"root-path", "path /usr/bin:/opt/x\ndir root 755 /usr/bin\ndir root 775 /opt/x\n@ok\n", false},
		{"root-path", "path /usr/bin:/opt/x\ndir root 755 /usr/bin\nmissing /opt/x\n@ok\n", false},
		{"root-path", "path \n@ok\n", false},
		// The privileged stat never ran (sudo refused): no entry was checked.
		{"root-path", "path /usr/sbin:/usr/bin\n@ok\n", false},
		{"root-path", "path /usr/sbin:/usr/bin\ndir root 755 /usr/sbin\n@ok\n", false},
		{"root-path", "@ok\n", false},
		{"root-umask", "@ok\n", true},
		{"root-umask", "/root/.bashrc:umask 077\n@ok\n", true},
		{"root-umask", "/root/.bashrc:umask 022\n@ok\n", false},
		{"default-umask", "/etc/login.defs:UMASK 027\n/etc/profile:umask 077\n@ok\n", true},
		{"default-umask", "/etc/pam.d/common-session:session optional pam_umask.so umask=0027\n@ok\n", true},
		{"default-umask", "/etc/login.defs:UMASK 022\n@ok\n", false},
		{"default-umask", "/etc/login.defs:UMASK 027\n/etc/bash.bashrc:umask u=rwx,g=rx,o=\n@ok\n", false}, // symbolic: not parsed, not claimed
		{"default-umask", "@ok\n", false},
		{"home-dirs", "home root root 700 /root\nhome govan govan 750 /home/govan\n@ok\n", true},
		{"home-dirs", "home govan govan 755 /home/govan\n@ok\n", false},
		{"home-dirs", "home govan root 700 /home/govan\n@ok\n", false},
		{"home-dirs", "missing frr /nonexistent\n@ok\n", false},
		{"home-dirs", "home root root 700 /root\nunreadable govan /home/govan\n@ok\n", false},
		{"dot-files", "dot govan govan 644 /home/govan/.profile\ndot govan govan 600 /home/govan/.bash_history\n@ok\n", true},
		{"dot-files", "dot govan govan 664 /home/govan/.gitconfig\n@ok\n", false},
		{"dot-files", "dot govan govan 640 /home/govan/.bash_history\n@ok\n", false},
		{"dot-files", "dot govan govan 600 /home/govan/.netrc\n@ok\n", false},
		{"dot-files", "dot govan root 644 /home/govan/.profile\n@ok\n", false},
		{"dot-files", "dot govan govan 644 /home/govan/.profile\n", false}, // did not complete
		// A refused sudo once listed nothing and passed: every home unread.
		{"dot-files", "unreadable root /root\nunreadable govan /home/govan\n@ok\n", false},
		{"dot-files", "dot govan govan 644 /home/govan/.profile\nunreadable root /root\n@ok\n", false},
		// A file the prober could not read is not a file with nothing in it.
		{"default-umask", "/etc/login.defs:UMASK 027\n@unreadable /etc/profile.d/x.sh\n@ok\n", false},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

func TestPAMVersionAndFirewallUtilityProbes(t *testing.T) {
	for _, tc := range []struct {
		probe, out string
		pass       bool
	}{
		{"pam-runtime-version", "version=1.7.0-5ubuntu2\nok=yes\n", true},
		{"pam-runtime-version", "version=1.5.2-6\nok=no\n", false},
		{"pam-runtime-version", "version=\nok=no\n", false},
		{"firewall-single-utility", "ufw=\nnftables=inactive\nunheaded-nft=inactive\niptables=active\n", true},
		{"firewall-single-utility", "ufw=inactive\nnftables=inactive\nunheaded-nft=active\niptables=inactive\n", true},
		{"firewall-single-utility", "ufw=inactive\nnftables=inactive\nunheaded-nft=inactive\niptables=inactive\n", false}, // east: ufw.service up, rules not
		{"firewall-single-utility", "ufw=active\nnftables=inactive\nunheaded-nft=inactive\niptables=active\n", false},
		{"firewall-single-utility", "ufw=inactive\nnftables=active\nunheaded-nft=active\niptables=inactive\n", true}, // both nftables units: one utility
		// systemd did not answer: ufw alone looked like the only utility.
		{"firewall-single-utility", "ufw=active\nnftables=\nunheaded-nft=\niptables=\n", false},
	} {
		p, ok := Probes[tc.probe]
		if !ok || !p.Baseline {
			t.Errorf("%s missing or not a baseline probe", tc.probe)
			continue
		}
		if pass, detail := p.Judge(tc.out); pass != tc.pass {
			t.Errorf("%s(%q) = %v (%s), want %v", tc.probe, tc.out, pass, detail, tc.pass)
		}
	}
}

// TestProbesUseUHPriv: a probe that hard-codes sudo cannot run in the agent
// (its own account, NoNewPrivileges); it would report UNEVALUATED there.
// Only the two root-facts fallbacks may name sudo, behind $UH_ROOT_FACTS.
func TestProbesUseUHPriv(t *testing.T) {
	for name, p := range Probes {
		if !strings.Contains(p.Command, "sudo -") { // sudo run with options, not the package name
			continue
		}
		if name == "apparmor-profiles-enforced" || name == "root-path" {
			if !strings.Contains(p.Command, `if [ -n "$UH_ROOT_FACTS" ]`) {
				t.Errorf("%s: sudo fallback without the root-facts branch", name)
			}
			continue
		}
		t.Errorf("%s hard-codes sudo; use $UH_PRIV: %s", name, p.Command)
	}
}

func TestHostProbesPrelude(t *testing.T) {
	cases := []struct {
		p    HostProbes
		want string
	}{
		{HostProbes{}, probePath + "UH_PRIV='sudo -n'\nUH_ROOT_FACTS=''\n"},
		{HostProbes{Direct: true, RootFacts: "/run/unheaded-baseline"}, probePath + "UH_PRIV=\nUH_ROOT_FACTS='/run/unheaded-baseline'\n"},
		{HostProbes{Direct: true, RootFacts: "/x'; rm -rf /"}, probePath + "UH_PRIV=\nUH_ROOT_FACTS='/x'\\''; rm -rf /'\n"},
	}
	for _, c := range cases {
		if got := c.p.probePrelude(); got != c.want {
			t.Errorf("prelude %+v = %q, want %q", c.p, got, c.want)
		}
	}
	var script string
	hp := &HostProbes{Hostname: "h", Direct: true, Run: func(_ context.Context, _, s string) (string, error) {
		script = s
		return "", nil
	}}
	if _, err := hp.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindHostProbe, Ref: "apparmor-enabled"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(script, probePath+"UH_PRIV=\n") {
		t.Fatalf("script does not start with the prelude: %q", script)
	}
}

// privRefusedFacts is a root-facts directory with a root PATH but no
// aa-status.json, for running probe commands without privilege.
func privRefusedFacts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "root-path"), []byte("/usr/bin:/bin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runProbeCommand runs one probe's command on this host the way HostProbes
// does, with $UH_PRIV set to priv, and judges it. Commands only read.
func runProbeCommand(t *testing.T, name, priv, facts string) (bool, string) {
	t.Helper()
	return Probes[name].Judge(probeSection(t, name, priv, facts))
}

// probeSection runs one probe's command as HostProbes does and returns its
// section; env is added to the environment.
func probeSection(t *testing.T, name, priv, facts string, env ...string) string {
	t.Helper()
	script := probePath + "UH_PRIV=" + priv + "\nUH_ROOT_FACTS=" + shQuote(facts) + "\n" +
		fmt.Sprintf("echo @@%s; { %s; } 2>/dev/null\nexit 0\n", name, Probes[name].Command)
	cmd := exec.Command("sh", "-c", script) // #nosec G204 -- the constant probe table
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	sec, ok := strings.CutPrefix(string(out), "@@"+name+"\n")
	if !ok {
		t.Fatalf("%s: no section in %q", name, out)
	}
	return sec
}

// fixturePriv writes a stand-in for "sudo -n" that runs its command with
// every "/etc/shadow" in its arguments replaced by $UH_TEST_SHADOW, so the
// shadow-reading probes run their real commands against a fixture.
func fixturePriv(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "priv")
	script := "#!/bin/sh\nfor a; do shift; set -- \"$@\" \"$(printf '%s' \"$a\" | sed \"s#/etc/shadow#$UH_TEST_SHADOW#g\")\"; done\nexec \"$@\"\n"
	if err := os.WriteFile(p, []byte(script), 0o700); err != nil { // #nosec G306 -- an executable test helper in a temp dir
		t.Fatal(err)
	}
	return p
}

// TestShadowProbesReadAccounts runs the /etc/shadow probes' real commands
// on a fixture. pw-max-days and pw-inactive once never listed an account:
// "$2" in their awk program was expanded by the outer shell, awk failed on
// "~/^$/", and the account half of both checks passed unchecked.
func TestShadowProbesReadAccounts(t *testing.T) {
	probeTools(t)
	if _, err := exec.LookPath("sed"); err != nil {
		t.Skip("no sed")
	}
	shadow := filepath.Join(t.TempDir(), "shadow")
	fixture := "root:!:19000:0:99999:7:::\n" +
		"alice:$y$j9T$salt$hash:19000:0:99999:7:::\n" + // hashed, never expires, no inactivity limit
		"bob:$6$salt$hash:19000:0:90:7:30::\n" + // hashed, compliant
		"svc:!*:19000::::::\n" + // locked: not judged
		"empty::19000:0:90:7:30::\n" + // no password at all
		"later:$y$x$y:999999:0:90:7:30::\n" // changed in the future
	if err := os.WriteFile(shadow, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	priv, env := fixturePriv(t), "UH_TEST_SHADOW="+shadow
	for _, tc := range []struct {
		probe string
		want  []string // offender lines the command must print
		not   []string
	}{
		{"pw-max-days", []string{"user alice"}, []string{"user bob", "user root", "user svc", "user empty"}},
		{"pw-inactive", []string{"user alice"}, []string{"user bob", "user root", "user svc"}},
		{"pw-change-past", []string{"later"}, []string{"alice", "bob"}},
		{"acct-shadow-no-empty", []string{"empty"}, []string{"alice", "svc"}},
		{"root-access-controlled", []string{"status L"}, nil},
	} {
		sec := probeSection(t, tc.probe, priv, "", env)
		lines := strings.Split(sec, "\n")
		has := func(s string) bool {
			for _, l := range lines {
				if strings.TrimSpace(l) == s {
					return true
				}
			}
			return false
		}
		if !has("@ok") {
			t.Errorf("%s did not complete: %q", tc.probe, sec)
		}
		for _, w := range tc.want {
			if !has(w) {
				t.Errorf("%s: no %q in %q", tc.probe, w, sec)
			}
		}
		for _, n := range tc.not {
			if has(n) {
				t.Errorf("%s: unexpected %q in %q", tc.probe, n, sec)
			}
		}
		if tc.probe == "root-access-controlled" {
			continue
		}
		if pass, detail := Probes[tc.probe].Judge(sec); pass {
			t.Errorf("%s passes with an offending account: %s", tc.probe, detail)
		}
	}
}

// probeTools skips a test that runs probe commands where they cannot run.
func probeTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"sh", "awk", "find", "stat"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
}

// TestProbeCommandsFailWhenPrivilegeRefused runs every probe that needs
// privilege with $UH_PRIV refusing (the central pull when sudo -n is
// refused). None may pass: dot-files once did, because its "@ok" came from
// outside the refused command, which had listed nothing.
func TestProbeCommandsFailWhenPrivilegeRefused(t *testing.T) {
	probeTools(t)
	facts := privRefusedFacts(t)
	for name, p := range Probes {
		if !strings.Contains(p.Command, "$UH_PRIV") {
			continue
		}
		if pass, detail := runProbeCommand(t, name, "false", facts); pass {
			t.Errorf("%s passes with privilege refused: %s", name, detail)
		}
	}
}

// TestProbeCommandsFailUnprivileged: the agent runs commands without sudo
// and relies on CAP_DAC_READ_SEARCH. Without it, a probe that reads a
// root-only file must not pass on having read nothing (pw-change-past,
// root-umask and sudo-no-noauth once did). journald-file-access,
// var-log-perms and root-path are left out: what they read may be readable
// by the test's own account.
func TestProbeCommandsFailUnprivileged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	probeTools(t)
	for _, f := range []string{"/etc/shadow", "/etc/sudoers"} {
		if fh, err := os.Open(f); err == nil {
			_ = fh.Close()
			t.Skipf("%s is readable without privilege here", f)
		}
	}
	if _, err := os.ReadDir("/root"); err == nil {
		t.Skip("/root is readable without privilege here")
	}
	facts := privRefusedFacts(t)
	for name, p := range Probes {
		if !strings.Contains(p.Command, "$UH_PRIV") {
			continue
		}
		switch name {
		case "journald-file-access", "var-log-perms", "root-path":
			continue
		}
		if pass, detail := runProbeCommand(t, name, "", facts); pass {
			t.Errorf("%s passes without privilege: %s", name, detail)
		}
	}
}

// TestPackageProbesReadDpkgStates runs the package probes' real commands
// against a stand-in dpkg-query. A held package ("hold ok installed") once
// read as absent, as did everything when dpkg-query could not run.
func TestPackageProbesReadDpkgStates(t *testing.T) {
	probeTools(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "db")
	fake := "#!/bin/sh\n" +
		"case \"$*\" in *'-f=${Status} dpkg') [ -n \"$UH_TEST_NODPKG\" ] && exit 2; printf 'install ok installed'; exit 0;; esac\n" +
		"[ -n \"$UH_TEST_NODPKG\" ] && exit 2\ncat \"$UH_TEST_DPKG\"\n"
	if err := os.WriteFile(filepath.Join(dir, "dpkg-query"), []byte(fake), 0o700); err != nil { // #nosec G306 -- an executable test helper in a temp dir
		t.Fatal(err)
	}
	path := "PATH=" + dir + ":" + os.Getenv("PATH")
	for _, tc := range []struct {
		name, db, probe string
		nodpkg          bool
		pass            bool
	}{
		{"held", "telnet hold ok installed\n", "pkg-telnet-client-absent", false, false},
		{"unpacked", "ftp install ok unpacked\n", "pkg-ftp-client-absent", false, false},
		{"config files only", "ftp deinstall ok config-files\ntnftp unknown ok not-installed\n", "pkg-ftp-client-absent", false, true},
		{"absent", "", "pkg-telnet-client-absent", false, true},
		{"dpkg-query fails", "", "pkg-telnet-client-absent", true, false},
		{"service held, enabled", "rsync hold ok installed\n", "svc-rsync-not-in-use", false, false},
		{"held but installed", "sudo hold ok installed\n", "sudo-installed", false, true},
	} {
		if err := os.WriteFile(db, []byte(tc.db), 0o600); err != nil {
			t.Fatal(err)
		}
		env := []string{path, "UH_TEST_DPKG=" + db}
		if tc.nodpkg {
			env = append(env, "UH_TEST_NODPKG=1")
		}
		sec := probeSection(t, tc.probe, "false", "", env...)
		if tc.probe == "svc-rsync-not-in-use" {
			sec += "rsync.service=enabled/active\n" // the unit half, as on a host running it
		}
		if pass, detail := Probes[tc.probe].Judge(sec); pass != tc.pass {
			t.Errorf("%s: %s = %v (%s), want %v; output %q", tc.name, tc.probe, pass, detail, tc.pass, sec)
		}
	}
}

// TestReadFilesReportUnreadable: the central pull reads config files
// unprivileged. A file it cannot read, or a directory whose glob it cannot
// expand, once read as empty: rsyslog-no-listen passed with an unreadable
// rsyslog.d file. Now each prints "@unreadable".
func TestReadFilesReportUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: nothing is unreadable")
	}
	probeTools(t)
	dir := t.TempDir()
	write := func(name, body string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	main := write("rsyslog.conf", "module(load=\"imuxsock\")\n", 0o600)
	hidden := write("rsyslog.d/50-remote.conf", "module(load=\"imtcp\")\n", 0o600)
	locked := write("locked/60-x.conf", "umask 022\n", 0o600)
	if err := os.Chmod(hidden, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(locked), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(locked), 0o700) })
	for _, tc := range []struct {
		name, script string
		want         []string
	}{
		{"cat", readFiles(main, filepath.Join(dir, "rsyslog.d/*.conf"), filepath.Join(dir, "locked/*.conf")),
			[]string{"module(load=\"imuxsock\")", "@unreadable " + hidden, "@unreadable " + filepath.Dir(locked)}},
		{"grep", grepFiles(`^[[:space:]]*umask`, main, hidden, filepath.Join(dir, "locked/*.conf")),
			[]string{"@unreadable " + hidden, "@unreadable " + filepath.Dir(locked)}},
		{"absent files are not unreadable", readFiles(filepath.Join(dir, "none.conf"), filepath.Join(dir, "nodir/*.conf")), nil},
	} {
		out, err := exec.Command("sh", "-c", "{ "+tc.script+"; } 2>/dev/null").Output() // #nosec G204 -- test scripts from the helpers
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(tc.want) == 0 {
			if strings.TrimSpace(string(out)) != "" {
				t.Errorf("%s: %q, want nothing", tc.name, out)
			}
			continue
		}
		for _, w := range tc.want {
			if !slices.Contains(got, w) {
				t.Errorf("%s: no %q in %q", tc.name, w, out)
			}
		}
	}
}

// TestProbesFailClosed: output that says nothing was checked never passes.
// A missing tool (modprobe off PATH), a refused sudo or an empty section
// once passed usb-storage, an MTA check and the backup check.
func TestProbesFailClosed(t *testing.T) {
	for name, p := range Probes {
		for _, out := range []string{"", "sh: 1: modprobe: not found\n", "sudo: a password is required\n",
			"Failed to connect to system scope bus via local transport: No such file or directory\n",
			"dpkg-query: no packages found matching x\n", "cat: /etc/shadow: Permission denied\n",
			"timeout: sending signal TERM to command\n", "@unreadable /etc/x\n@ok\n"} {
			if pass, detail := p.Judge(out); pass {
				t.Errorf("%s passes on %q: %s", name, out, detail)
			}
		}
	}
}
