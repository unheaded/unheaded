// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// serviceFixture is one TLS server standing in for a first-party service,
// with a table that names it. status is what a request with no credentials
// gets.
type serviceFixture struct {
	t      *testing.T
	srv    *httptest.Server
	status int
	hits   []string // "<METHOD> <path>" of every request
	table  map[string]crosswalk.HostService
	ca     string
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	f := &serviceFixture{t: t, status: http.StatusUnauthorized}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits = append(f.hits, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "" || len(r.Cookies()) != 0 {
			t.Errorf("the probe sent credentials: %v %v", r.Header.Get("Authorization"), r.Cookies())
		}
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"leak":"s3cr3t-canary"}`))
	}))
	t.Cleanup(f.srv.Close)
	f.ca = filepath.Join(t.TempDir(), "ca.pem")
	block := &pem.Block{Type: "CERTIFICATE", Bytes: f.srv.Certificate().Raw}
	if err := os.WriteFile(f.ca, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	_, portStr, _ := net.SplitHostPort(f.srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	run := func(c string) map[string]crosswalk.ServiceRun {
		return map[string]crosswalk.ServiceRun{"west": {Container: c, Addr: "127.0.0.1", CA: f.ca}}
	}
	f.table = map[string]crosswalk.HostService{
		"api":   {Port: port, Method: "GET", Route: "/api/v1/things", Runs: run("unheaded-api")},
		"board": {Port: port, Method: "DELETE", Route: "/api/v1/tasks/", Why: "reads are open", Runs: run("unheaded-board")},
		"writer": {Port: port, Method: "POST", Route: "/ingest", AuthVar: "LOGSTORE_INGEST_TOKEN", Why: "one route",
			Runs: map[string]crosswalk.ServiceRun{"west": {Container: "unheaded-writer", TLSName: "example.com", CA: f.ca}}},
		"deb": {Port: port, Method: "GET", Route: "/api/v1/things",
			Runs: map[string]crosswalk.ServiceRun{"west": {Unit: "unheaded-deb", Package: "unheaded-deb", Addr: "127.0.0.1", CA: f.ca}}},
		"issuer": {Exempt: "public by design"},
	}
	return f
}

func svcSrc(ref string) crosswalk.Source {
	return crosswalk.Source{Kind: crosswalk.KindHostService, Ref: ref}
}

// collect runs the collector with a runner that answers out (its "@@name"
// headers given the script's tag).
func (f *serviceFixture) collect(out string, refs ...string) ([]crosswalk.Record, error) {
	f.t.Helper()
	h := &HostServices{Hostname: "west", Table: f.table,
		Now: func() time.Time { return time.Unix(1794700000, 0).UTC() },
		Run: func(_ context.Context, _, script string) (string, error) { return tagged(f.t, script, out), nil }}
	var srcs []crosswalk.Source
	for _, r := range refs {
		srcs = append(srcs, svcSrc(r))
	}
	return h.Collect(context.Background(), srcs)
}

func one(t *testing.T, recs []crosswalk.Record, err error) crosswalk.Record {
	t.Helper()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1: %+v", len(recs), recs)
	}
	return recs[0]
}

const goodContainer = "kind=container\nrunning=true\nimage=sha256:c7ed029904cfc3e42ce3a91c83036c04acea5785c69d3c890a9f6d47a2a88f21\naddr=172.28.1.11\nenv=AUTH_ENABLED=true\n"

func TestHostServices_PassNeedsRunningAuthAnd401(t *testing.T) {
	f := newServiceFixture(t)
	recs, err := f.collect("@@api\n"+goodContainer, "west:api")
	r := one(t, recs, err)
	if r.Verdict != crosswalk.VerdictPass {
		t.Fatalf("verdict %s: %s", r.Verdict, r.Detail)
	}
	for _, want := range []string{"west: api", "running", "sha256:c7ed02990", "AUTH_ENABLED=true", "GET /api/v1/things", "401"} {
		if !strings.Contains(r.Detail, want) {
			t.Errorf("detail %q lacks %q", r.Detail, want)
		}
	}
	if r.Source != svcSrc("west:api") || r.ObservedAt.IsZero() {
		t.Errorf("record = %+v", r)
	}
	if len(f.hits) != 1 || f.hits[0] != "GET /api/v1/things" {
		t.Errorf("requests = %v", f.hits)
	}
}

func TestHostServices_UnitNamesThePackageVersion(t *testing.T) {
	f := newServiceFixture(t)
	recs, err := f.collect("@@deb\nkind=unit\nrunning=active\nimage=0.9.4-1\nenv=AUTH_ENABLED=true\n", "west:deb")
	r := one(t, recs, err)
	if r.Verdict != crosswalk.VerdictPass || !strings.Contains(r.Detail, "unheaded-deb 0.9.4-1") {
		t.Errorf("verdict %s, detail %q", r.Verdict, r.Detail)
	}
	recs, err = f.collect("@@deb\nkind=unit\nrunning=inactive\nimage=0.9.4-1\n", "west:deb")
	if r = one(t, recs, err); r.Verdict != crosswalk.VerdictFail || !strings.Contains(r.Detail, "not running") {
		t.Errorf("inactive unit: verdict %s, detail %q", r.Verdict, r.Detail)
	}
}

func TestHostServices_Fails(t *testing.T) {
	for _, tc := range []struct {
		name, out, ref string
		status         int
		want           string
		noRequest      bool
	}{
		{"answers 200", "@@api\n" + goodContainer, "west:api", 200, "answers without a login", false},
		{"answers 403", "@@api\n" + goodContainer, "west:api", 403, "answers 403", false},
		{"answers 500", "@@api\n" + goodContainer, "west:api", 500, "answers 500", false},
		{"no auth variable", "@@api\nkind=container\nrunning=true\nimage=sha256:aa\naddr=10.0.0.1\n", "west:api", 401, "AUTH_ENABLED is not set", true},
		{"auth variable is True", "@@api\nkind=container\nrunning=true\nimage=sha256:aa\nenv=AUTH_ENABLED=True\n", "west:api", 401, `AUTH_ENABLED is not "true"`, true},
		{"auth variable twice", "@@api\nkind=container\nrunning=true\nimage=sha256:aa\nenv=AUTH_ENABLED=false\nenv=AUTH_ENABLED=true\n", "west:api", 401, "more than once", true},
		{"not running", "@@api\nkind=container\nrunning=false\nimage=sha256:aa\nenv=AUTH_ENABLED=true\n", "west:api", 401, "not running", true},
		{"no such container", "@@api\nabsent\n", "west:api", 401, "not running", true},
		{"token variable empty", "@@writer\nkind=container\nrunning=true\nimage=sha256:aa\naddr=127.0.0.1\ntoken=empty\n", "west:writer", 401, "LOGSTORE_INGEST_TOKEN is empty", true},
		{"token variable missing", "@@writer\nkind=container\nrunning=true\nimage=sha256:aa\naddr=127.0.0.1\n", "west:writer", 401, "LOGSTORE_INGEST_TOKEN is not set", true},
		{"token service answers 200", "@@writer\nkind=container\nrunning=true\nimage=sha256:aa\naddr=127.0.0.1\ntoken=set\n", "west:writer", 200, "answers without a login", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newServiceFixture(t)
			f.status = tc.status
			recs, err := f.collect(tc.out, tc.ref)
			r := one(t, recs, err)
			if r.Verdict != crosswalk.VerdictFail {
				t.Errorf("verdict %s, want fail: %s", r.Verdict, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.want) {
				t.Errorf("detail %q lacks %q", r.Detail, tc.want)
			}
			if tc.noRequest && len(f.hits) != 0 {
				t.Errorf("a service that already failed was probed: %v", f.hits)
			}
		})
	}
}

// The token-table service passes on its own variable, reached at the
// container's address and checked against the certificate's name.
func TestHostServices_TokenServiceAtTheContainerAddress(t *testing.T) {
	f := newServiceFixture(t)
	recs, err := f.collect("@@writer\nkind=container\nrunning=true\nimage=sha256:aa11\naddr=127.0.0.2\naddr=127.0.0.1\ntoken=set\n", "west:writer")
	r := one(t, recs, err)
	if r.Verdict != crosswalk.VerdictPass {
		t.Fatalf("verdict %s: %s", r.Verdict, r.Detail)
	}
	if !strings.Contains(r.Detail, "LOGSTORE_INGEST_TOKEN is set") || !strings.Contains(r.Detail, "POST /ingest") {
		t.Errorf("detail %q", r.Detail)
	}
	if len(f.hits) != 1 || f.hits[0] != "POST /ingest" {
		t.Errorf("requests = %v", f.hits)
	}
}

func TestHostServices_ProbeUsesTheTablesMethod(t *testing.T) {
	f := newServiceFixture(t)
	recs, err := f.collect("@@board\n"+goodContainer, "west:board")
	if r := one(t, recs, err); r.Verdict != crosswalk.VerdictPass {
		t.Fatalf("verdict %s: %s", r.Verdict, r.Detail)
	}
	if len(f.hits) != 1 || f.hits[0] != "DELETE /api/v1/tasks/" {
		t.Errorf("requests = %v", f.hits)
	}
}

// No reading is no record: the source stays not assessed.
func TestHostServices_NotAssessed(t *testing.T) {
	f := newServiceFixture(t)
	for name, out := range map[string]string{
		"docker unreachable":    "@@api\nunreachable\n",
		"environ unreadable":    "@@deb\nkind=unit\nrunning=active\nimage=1\nenviron=unreadable\n",
		"no section":            "",
		"section printed twice": "@@api\n" + goodContainer + "@@api\n" + goodContainer,
	} {
		ref := "west:api"
		if strings.Contains(out, "@@deb") {
			ref = "west:deb"
		}
		if recs, err := f.collect(out, ref); err != nil || len(recs) != 0 {
			t.Errorf("%s: recs %+v, err %v; want no record", name, recs, err)
		}
	}
	h := &HostServices{Hostname: "west", Table: f.table,
		Run: func(context.Context, string, string) (string, error) { return "", errors.New("ssh: connect refused") }}
	if recs, err := h.Collect(context.Background(), []crosswalk.Source{svcSrc("west:api")}); err != nil || len(recs) != 0 {
		t.Errorf("runner error: recs %+v, err %v; want no record", recs, err)
	}

	// Nothing answers on the port: the 401 was not seen, so no verdict.
	f.srv.Close()
	if recs, err := f.collect("@@api\n"+goodContainer, "west:api"); err != nil || len(recs) != 0 {
		t.Errorf("closed port: recs %+v, err %v; want no record", recs, err)
	}
}

// A certificate that is not the service's, or a CA file that cannot be
// read, proves nothing about who answered: no verdict.
func TestHostServices_UnverifiedCertificateIsNotAssessed(t *testing.T) {
	f := newServiceFixture(t)
	svc := f.table["api"]
	run := svc.Runs["west"]
	run.TLSName = "not-its-name.invalid"
	svc.Runs = map[string]crosswalk.ServiceRun{"west": run}
	f.table["api"] = svc
	if recs, err := f.collect("@@api\n"+goodContainer, "west:api"); err != nil || len(recs) != 0 {
		t.Errorf("name mismatch: recs %+v, err %v; want no record", recs, err)
	}
	run.TLSName, run.CA = "", filepath.Join(t.TempDir(), "missing.pem")
	svc.Runs = map[string]crosswalk.ServiceRun{"west": run}
	f.table["api"] = svc
	if recs, err := f.collect("@@api\n"+goodContainer, "west:api"); err != nil || len(recs) != 0 {
		t.Errorf("missing CA: recs %+v, err %v; want no record", recs, err)
	}
	if len(f.hits) != 0 {
		t.Errorf("a request was sent over an unverified connection: %v", f.hits)
	}
}

// The environment holds secrets. Even if the reader printed all of it, no
// value but the auth setting reaches a record, an error or a log line.
func TestHostServices_SecretsStayOut(t *testing.T) {
	const canaryPW, canaryTok = "s3cr3t-canary", "tok-canary"
	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)

	leaky := "kind=container\nrunning=true\nimage=sha256:aa\naddr=127.0.0.1\n" +
		"env=WELL_PASSWORD=" + canaryPW + "\nWELL_PASSWORD=" + canaryPW + "\n" +
		"env=LOGSTORE_INGEST_TOKEN=" + canaryTok + "\nLOGSTORE_INGEST_TOKEN=" + canaryTok + "\n"
	var all []string
	keep := func(recs []crosswalk.Record, err error) {
		t.Helper()
		b, jerr := json.Marshal(recs)
		if jerr != nil {
			t.Fatal(jerr)
		}
		all = append(all, string(b))
		if err != nil {
			all = append(all, err.Error())
		}
	}
	for _, status := range []int{401, 200} {
		f := newServiceFixture(t)
		f.status = status
		keep(f.collect("@@api\n"+leaky+"env=AUTH_ENABLED=true\n", "west:api"))
		keep(f.collect("@@api\n"+leaky, "west:api"))
		keep(f.collect("@@api\n"+leaky+"env=AUTH_ENABLED="+canaryPW+"\n", "west:api"))
		keep(f.collect("@@writer\n"+leaky+"token=set\n", "west:writer"))
		keep(f.collect("@@writer\n"+leaky+"token="+canaryTok+"\n", "west:writer"))
		keep(f.collect("@@writer\n"+leaky, "west:writer"))
		keep(f.collect("@@deb\nkind=unit\nrunning=active\nimage="+canaryPW+"\n"+leaky+"env=AUTH_ENABLED=true\n", "west:deb"))
	}
	all = append(all, logged.String())
	for _, s := range all {
		if strings.Contains(s, canaryPW) || strings.Contains(s, canaryTok) {
			t.Errorf("a canary value escaped: %s", s)
		}
	}
	if len(all) < 15 {
		t.Fatalf("only %d outputs checked", len(all))
	}
}

// The script is made of the table's constants: the reader's format names
// only the auth keys, and nothing from a ref reaches sh.
func TestHostServices_ScriptIsConstant(t *testing.T) {
	f := newServiceFixture(t)
	var script string
	h := &HostServices{Hostname: "west", Table: f.table,
		Run: func(_ context.Context, host, s string) (string, error) {
			if host != "west" {
				t.Errorf("host %q", host)
			}
			script = s
			return "", nil
		}}
	if _, err := h.Collect(context.Background(), []crosswalk.Source{svcSrc("west:api"), svcSrc("west:writer"), svcSrc("west:deb")}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"unheaded-api", "unheaded-writer", "unheaded-deb.service", `"AUTH_ENABLED"`, `"LOGSTORE_INGEST_TOKEN"`, "exit 0"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	for _, bad := range []string{"{{json", "{{.Config.Env}}", "printenv", "docker exec"} {
		if strings.Contains(script, bad) {
			t.Errorf("script contains %q: it would print the whole environment", bad)
		}
	}
	// The same sources give the same script but for the random tag.
	first := tagRe.ReplaceAllString(script, "echo @@TAG ")
	if _, err := h.Collect(context.Background(), []crosswalk.Source{svcSrc("west:api"), svcSrc("west:writer"), svcSrc("west:deb")}); err != nil {
		t.Fatal(err)
	}
	if second := tagRe.ReplaceAllString(script, "echo @@TAG "); first != second {
		t.Error("two runs over the same sources built different scripts")
	}
}

func TestHostServices_BadRefs(t *testing.T) {
	f := newServiceFixture(t)
	for _, ref := range []string{"api", "west:nope", "west:api;id", "west:$(id)", "east:api", "west:issuer", ":api", "west:"} {
		if _, err := f.collect("", ref); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
	// Other kinds are not this collector's.
	recs, err := f.collect("", []string{}...)
	if err != nil || len(recs) != 0 {
		t.Errorf("no sources: %+v %v", recs, err)
	}
	h := &HostServices{Hostname: "west", Table: f.table, Run: func(context.Context, string, string) (string, error) {
		t.Error("ran a script with no host-service source")
		return "", nil
	}}
	if _, err := h.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindHostProbe, Ref: "west:auditd-running"}}); err != nil {
		t.Fatal(err)
	}
}

// The shipped table is what the collector reads by default, and every west
// ref in it builds a script.
func TestHostServices_DefaultTable(t *testing.T) {
	var srcs []crosswalk.Source
	for name, svc := range crosswalk.HostServices {
		if _, ok := svc.Runs["west"]; ok {
			srcs = append(srcs, svcSrc("west:"+name))
		}
	}
	if len(srcs) < 7 { // 12 until the five role services left (ADR-147 decision 7)
		t.Fatalf("only %d west services in the table", len(srcs))
	}
	ran := 0
	h := &HostServices{Hostname: "west", Run: func(_ context.Context, _, script string) (string, error) {
		ran++
		for _, s := range srcs {
			_, name, _ := strings.Cut(s.Ref, ":")
			if !strings.Contains(script, " "+name+";") {
				t.Errorf("script has no section for %s", name)
			}
		}
		return "", nil
	}}
	if recs, err := h.Collect(context.Background(), srcs); err != nil || len(recs) != 0 {
		t.Fatalf("recs %d, err %v", len(recs), err)
	}
	if ran != 1 {
		t.Errorf("ran %d scripts for one host", ran)
	}
}
