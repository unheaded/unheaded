// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/bff"
	"unheaded/pkg/bff/bfftest"
)

// Every login front on a host reaches the issuer from one address, so the
// issuer's lockout counts their failures together. Each front holds a client
// to its own budget; the budgets protect anyone only while their sum over
// all the fronts stays under the issuer's limits (decision pack 2026-11-18,
// row 28). These tests count the fronts from where they are declared:
//
//   - a Go front is a binary that builds a BFF (bfftest.GoFronts reads the
//     source: no list to forget);
//   - a front that is not Go cannot be read that way, so foreignFronts lists
//     it, and the list is held to the tree from three sides: every file
//     outside Go that names the issuer key, every shipped unit and every
//     compose service that sets it must be accounted for.

const repoRoot = "../.."

// issuerURLKey is the key a login front reads its issuer from.
const issuerURLKey = "AUTH_ISSUER_URL"

// foreignFront is a login front that does not use pkg/bff.
type foreignFront struct {
	name string
	// source holds its budgets as NAME = <int> lines.
	source, perUser, perClient, window string
	// unit is the shipped unit that runs it.
	unit string
}

// foreignFronts: the Zhen web UI's app, pkg/bff's contract in Python.
var foreignFronts = []foreignFront{{
	name:      "unheaded-zhen-app",
	source:    "raft/zhen_login.py",
	perUser:   "CLIENT_USER_FAILURES",
	perClient: "CLIENT_FAILURES",
	window:    "FAILURE_WINDOW",
	unit:      "unheaded-zhen-app.service",
}}

// notFronts are files outside Go that name the issuer key and log nobody in.
var notFronts = map[string]string{
	"raft/start-zhen.sh":            "hands the key to zhen_app.py, whose login is raft/zhen_login.py",
	"raft/tests/test_zhen_paths.py": "a test's environment filter",
	"raft/tests/test_zhen_login.py": "the tests of raft/zhen_login.py",
}

// unitBinary reads the binary a unit starts: a unit that sets the issuer key
// must start a counted Go front.
var unitBinary = regexp.MustCompile(`(?m)^ExecStart=\S*/([a-z0-9-]+)(\s|$)`)

// repo is the repository as a file system: every file these tests read is
// named from its root.
var repo = os.DirFS(repoRoot)

func foreignBudget(t *testing.T, f foreignFront, name string) int {
	t.Helper()
	raw, err := fs.ReadFile(repo, f.source)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s*=\s*(\d+)\s*$`).FindSubmatch(raw)
	if m == nil {
		t.Fatalf("%s not found in %s", name, f.source)
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func goFronts(t *testing.T) []bfftest.Front {
	t.Helper()
	fronts, err := bfftest.GoFronts(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(fronts) == 0 {
		t.Fatal("no binary builds a BFF: the count would be empty")
	}
	return fronts
}

// budgetLedger is every front with its two budgets, and the sums.
type budgetLedger struct {
	lines              []string
	perUser, perClient int
}

func (l *budgetLedger) add(name string, perUser, perClient int) {
	l.lines = append(l.lines, fmt.Sprintf("%s %d and %d", name, perUser, perClient))
	l.perUser += perUser
	l.perClient += perClient
}

func (l *budgetLedger) String() string { return strings.Join(l.lines, "; ") }

func ledger(t *testing.T) *budgetLedger {
	t.Helper()
	l := &budgetLedger{}
	for _, f := range goFronts(t) {
		l.add(f.Name, bff.DefaultClientUserLoginFailures, bff.DefaultClientLoginFailures)
	}
	for _, f := range foreignFronts {
		l.add(f.name, foreignBudget(t, f, f.perUser), foreignBudget(t, f, f.perClient))
	}
	return l
}

// The sum of every front's budgets stays strictly under the issuer's two
// limits, read from the issuer's own defaults. A front added anywhere in the
// tree lands in this sum; if it no longer fits, the message says which
// fronts and what they add up to.
func TestLoginFronts_BudgetsFitTheIssuerLockout(t *testing.T) {
	l := ledger(t)
	if l.perUser >= maxFailuresPerPair {
		t.Errorf("failures per client address and username: the fronts allow %d in all, which must stay under the issuer's lock at %d (maxFailuresPerPair). Fronts: %s. Shrink a budget (pkg/bff DefaultClientUserLoginFailures, raft/zhen_login.py CLIENT_USER_FAILURES) before adding a front.",
			l.perUser, maxFailuresPerPair, l)
	}
	if l.perClient >= maxFailuresPerSource {
		t.Errorf("failures per client address: the fronts allow %d in all, which must stay under the issuer's lock at %d (maxFailuresPerSource). Fronts: %s. Shrink a budget (pkg/bff DefaultClientLoginFailures, raft/zhen_login.py CLIENT_FAILURES) before adding a front.",
			l.perClient, maxFailuresPerSource, l)
	}
	if bff.LoginFailureWindow < lockoutWindow {
		t.Errorf("pkg/bff counts a failure for %v, which must cover the issuer's %v", bff.LoginFailureWindow, lockoutWindow)
	}
	for _, f := range foreignFronts {
		if w := time.Duration(foreignBudget(t, f, f.window)) * time.Second; w < lockoutWindow {
			t.Errorf("%s counts a failure for %v, which must cover the issuer's %v", f.name, w, lockoutWindow)
		}
	}
	t.Logf("fronts: %s; sums %d and %d against the issuer's %d and %d", l, l.perUser, l.perClient, maxFailuresPerPair, maxFailuresPerSource)
}

// A front that is not Go is found by the key it reads: any source file
// outside Go that names AUTH_ISSUER_URL is a listed front or a listed
// non-front, or this fails naming the file.
func TestLoginFronts_EveryForeignReaderOfTheIssuerKeyIsAccountedFor(t *testing.T) {
	known := map[string]bool{}
	for _, f := range foreignFronts {
		known[f.source] = true
		if _, err := fs.Stat(repo, f.source); err != nil {
			t.Errorf("%s: its source %s is gone; take it out of foreignFronts and the sum", f.name, f.source)
		}
	}
	source := map[string]bool{".py": true, ".sh": true, ".rs": true, ".js": true, ".mjs": true, ".ts": true, ".rb": true, ".pl": true, ".php": true, ".lua": true, ".c": true}
	err := fs.WalkDir(repo, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch name := d.Name(); {
			case name == "node_modules", name == "target", name == "__pycache__", name == "venv", name == "docs", name == "wiki":
				return fs.SkipDir
			case rel != "." && strings.HasPrefix(name, "."):
				return fs.SkipDir
			}
			return nil
		}
		if !source[path.Ext(rel)] || !d.Type().IsRegular() {
			return nil
		}
		raw, rerr := fs.ReadFile(repo, rel)
		if rerr != nil {
			return rerr
		}
		if !bytes.Contains(raw, []byte(issuerURLKey)) || known[rel] {
			return nil
		}
		if _, ok := notFronts[rel]; !ok {
			t.Errorf("%s names %s and is in neither foreignFronts nor notFronts: if it logs people in at the issuer it is a login front, and its budgets belong in TestLoginFronts_BudgetsFitTheIssuerLockout", rel, issuerURLKey)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Every shipped unit and compose service that sets the issuer key runs a
// counted front. A unit that keeps the key in its environment file (the
// wiki's) is counted all the same: its binary builds a BFF.
func TestLoginFronts_ShippedConfigurationRunsOnlyCountedFronts(t *testing.T) {
	counted := map[string]bool{}
	for _, f := range goFronts(t) {
		counted[f.Name] = true
	}
	foreignUnit := map[string]bool{}
	for _, f := range foreignFronts {
		foreignUnit[f.unit] = false
	}
	units, err := fs.Glob(repo, "deploy/systemd/*.service")
	if err != nil || len(units) == 0 {
		t.Fatalf("no units found: %v", err)
	}
	for _, u := range units {
		raw, err := fs.ReadFile(repo, u)
		if err != nil {
			t.Fatal(err)
		}
		base := path.Base(u)
		for _, key := range []string{bff.EnvClientUserLoginFailures, bff.EnvClientLoginFailures} {
			if bytes.Contains(raw, []byte("Environment="+key+"=")) {
				t.Errorf("%s sets %s: the sum is taken over the defaults, so a shipped unit must not move one", base, key)
			}
		}
		if !bytes.Contains(raw, []byte("Environment="+issuerURLKey+"=")) {
			continue
		}
		if _, ok := foreignUnit[base]; ok {
			foreignUnit[base] = true
			continue
		}
		m := unitBinary.FindSubmatch(raw)
		if m == nil || !counted[string(m[1])] {
			t.Errorf("%s sets %s and starts no counted login front (pkg/bff fronts: %v): count it, or it spends the issuer's lockout unseen", base, issuerURLKey, sortedKeys(counted))
		}
	}
	for unit, seen := range foreignUnit {
		if !seen {
			t.Errorf("%s no longer sets %s: take its budgets out of foreignFronts", unit, issuerURLKey)
		}
	}

	compose, err := fs.ReadFile(repo, "docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{bff.EnvClientUserLoginFailures, bff.EnvClientLoginFailures} {
		if bytes.Contains(compose, []byte(key+":")) {
			t.Errorf("docker-compose.yml sets %s: the sum is taken over the defaults", key)
		}
	}
	for _, svc := range composeServicesSetting(t, string(compose), issuerURLKey) {
		if !counted[svc.binary] {
			t.Errorf("compose service %s (binary %q) sets %s and is no counted login front (pkg/bff fronts: %v)", svc.name, svc.binary, issuerURLKey, sortedKeys(counted))
		}
	}
}

type composeService struct{ name, binary string }

// composeServicesSetting returns the services of a compose file whose
// environment sets key, with the binary each builds (its build target).
// The file is read by indentation: services are the keys at two spaces.
func composeServicesSetting(t *testing.T, compose, key string) []composeService {
	t.Helper()
	var (
		out        []composeService
		inServices bool
		cur        composeService
		sets       bool
	)
	flush := func() {
		if cur.name != "" && sets {
			out = append(out, cur)
		}
		cur, sets = composeService{}, false
	}
	service := regexp.MustCompile(`^  ([A-Za-z0-9_.-]+):\s*(#.*)?$`)
	target := regexp.MustCompile(`^\s+target:\s*([A-Za-z0-9_.-]+)`)
	setsKey := regexp.MustCompile(`^\s+` + regexp.QuoteMeta(key) + `:`)
	for _, line := range strings.Split(compose, "\n") {
		if !strings.HasPrefix(line, " ") && strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
			flush()
			inServices = strings.HasPrefix(line, "services:")
			continue
		}
		if !inServices {
			continue
		}
		if m := service.FindStringSubmatch(line); m != nil {
			flush()
			cur.name, cur.binary = m[1], m[1]
			continue
		}
		if m := target.FindStringSubmatch(line); m != nil && cur.name != "" {
			cur.binary = m[1]
		}
		if setsKey.MatchString(line) {
			sets = true
		}
	}
	flush()
	if len(out) == 0 {
		t.Fatalf("no compose service sets %s: the reader no longer understands the file", key)
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The sum, proved against the running code: one client address fails on
// every front as often as each lets it, the Zhen app's share included, and
// the issuer's lock on the fronts' shared address still does not fall, so
// another client logs in. One front per binary bfftest.GoFronts finds.
func TestLoginFronts_OneClientOnEveryFrontCannotLockOutAUser(t *testing.T) {
	fronts := goFronts(t)
	ts, handlers := frontsBehindOneIssuer(t, fronts)
	const attacker = "192.0.2.7:4000"
	for i := 0; i < 2*maxFailuresPerPair; i++ {
		for _, h := range handlers {
			bffLogin(t, h, attacker, "dev", "wrong")
		}
	}
	for _, f := range foreignFronts {
		for i := 0; i < foreignBudget(t, f, f.perUser); i++ {
			issuerLogin(t, ts.URL, "dev", "wrong")
		}
	}
	if got := bffLogin(t, handlers[0], "192.0.2.8:4000", "dev", "correct horse battery"); got != http.StatusOK {
		t.Fatalf("another client's login after one client spent every front's budget (%s) = %d, want 200", ledger(t), got)
	}
}

func TestLoginFronts_OneClientOnEveryFrontCannotLockOutEveryone(t *testing.T) {
	fronts := goFronts(t)
	ts, handlers := frontsBehindOneIssuer(t, fronts)
	const attacker = "192.0.2.7:4000"
	for i := 0; i < 2*maxFailuresPerSource; i++ {
		for _, h := range handlers {
			bffLogin(t, h, attacker, fmt.Sprintf("nobody-%d", i), "wrong")
		}
	}
	for _, f := range foreignFronts {
		for i := 0; i < foreignBudget(t, f, f.perClient); i++ {
			issuerLogin(t, ts.URL, fmt.Sprintf("zhen-nobody-%d", i), "wrong")
		}
	}
	if got := bffLogin(t, handlers[0], "192.0.2.8:4000", "dev", "correct horse battery"); got != http.StatusOK {
		t.Fatalf("another client's login after one client spent every front's budget (%s) = %d, want 200", ledger(t), got)
	}
}

// frontsBehindOneIssuer is the real issuer behind one real BFF per front,
// all reaching it from this host's one address, as on a deployment.
func frontsBehindOneIssuer(t *testing.T, fronts []bfftest.Front) (*testServer, []http.Handler) {
	t.Helper()
	var audiences []string
	for _, f := range fronts {
		audiences = append(audiences, f.Name)
	}
	ts, _ := serve(t, newFixture(t), func(c *issuerConfig) { c.Audiences = audiences })
	var hs []http.Handler
	for _, f := range fronts {
		b, err := bff.New(bff.Config{ServiceName: f.Name, IssuerURL: ts.URL, DevPlaintext: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(b.Close)
		hs = append(hs, b.Handler())
	}
	return &testServer{URL: ts.URL}, hs
}

type testServer struct{ URL string }

// issuerLogin is a login sent straight to the issuer from this host, as a
// front that is not pkg/bff sends one.
func issuerLogin(t *testing.T, issuerURL, user, password string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": user, "password": password})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, issuerURL+"/login", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}
