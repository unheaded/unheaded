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

func TestHostProbes(t *testing.T) {
	outputs := map[string]string{
		// west: firewall open, NTP synced, no auditd, updates on, AppArmor on
		"west": "@@firewall-inbound-deny\n-P INPUT ACCEPT\n---\n-P INPUT ACCEPT\n" +
			"@@ntp-synchronized\nyes\n@@auditd-running\ninactive\n" +
			"@@unattended-upgrades-enabled\nAPT::Periodic::Unattended-Upgrade \"1\";\n@@apparmor-enabled\nY\n",
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
		return o, nil
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
		{"db-backup-recent", "\n---\n2026-09-22\n", false},
		{"db-backup-recent", "\n---\n\n", false},
		{"firewall-inbound-deny", "-P INPUT DROP\n---\n-P INPUT DROP\n", true},
		{"firewall-inbound-deny", "-P INPUT DROP\n---\n", false},
		// the package-owned nft table (ADR-098 step 6) decides even with iptables at ACCEPT
		{"firewall-inbound-deny", "-P INPUT ACCEPT\n---\n-P INPUT ACCEPT\n---\n\t\ttype filter hook input priority filter; policy drop;\n", true},
		{"firewall-inbound-deny", "-P INPUT ACCEPT\n---\n-P INPUT ACCEPT\n---\n\t\ttype filter hook input priority filter; policy accept;\n", false},
		{"firewall-inbound-deny", "-P INPUT ACCEPT\n---\n-P INPUT ACCEPT\n---\n", false},
		{"unattended-upgrades-enabled", "APT::Periodic::Unattended-Upgrade \"0\";\n", false},
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
		{"svc-web-server-not-in-use", "installed=\nnginx.service=not-found/inactive\napache2.service=not-found/inactive\n", true},
		{"svc-web-server-not-in-use", "installed=nginx\nnginx.service=enabled/active\napache2.service=not-found/inactive\n", false},
		{"svc-web-server-not-in-use", "installed=nginx\nnginx.service=masked/inactive\napache2.service=not-found/inactive\n", true},  // installed as a dependency, masked
		{"svc-web-server-not-in-use", "installed=nginx\nnginx.service=disabled/active\napache2.service=not-found/inactive\n", false}, // running now
		{"svc-rsync-not-in-use", "installed=rsync\nrsync.service=disabled/inactive\n", true},
		{"svc-rsync-not-in-use", "", false},
		{"pkg-telnet-client-absent", "installed=inetutils-telnet telnet\n", false},
		{"pkg-telnet-client-absent", "installed=\n", true},
		{"pkg-telnet-client-absent", "", false},
		{"cron-active", "installed=cron\nenabled=enabled\nactive=active\n", true},
		{"cron-active", "installed=cron\nenabled=enabled\nactive=inactive\n", false},
		{"cron-active", "installed=\nenabled=\nactive=\n", false},
		{"cron-allow-restricted", "allow=/etc/cron.allow 640 0 crontab\ndeny=ABSENT\n", true},
		{"cron-allow-restricted", "allow=ABSENT\ndeny=ABSENT\n", false},
		{"cron-allow-restricted", "allow=/etc/cron.allow 640 0 crontab\ndeny=/etc/cron.deny 640 0 root\n", false},
		{"cron-allow-restricted", "allow=/etc/cron.allow 644 0 crontab\ndeny=ABSENT\n", false},
		{"at-restricted", "installed=\n", true}, // at not installed: nothing to restrict
		{"at-restricted", "installed=at\nallow=ABSENT\ndeny=ABSENT\n", false},
		{"at-restricted", "installed=at\nallow=/etc/at.allow 640 0 root\ndeny=/etc/at.deny 640 0 root\n", true},
		{"mta-local-only", "", true}, // nothing listening on 25/465/587
		{"mta-local-only", "127.0.0.1:25\n[::1]:25\n", true},
		{"mta-local-only", "0.0.0.0:25\n", false},
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
