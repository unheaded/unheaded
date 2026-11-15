// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package crosswalk

import (
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/ports"
)

func hostServiceControls(ref string) string {
	return strings.Replace(strings.Replace(testControls, "kind: github-job", "kind: host-service", 1),
		`ref: "Security Scan (Daily + PR)/Go Vulnerability Check"`, "ref: '"+ref+"'", 1)
}

// The catalog names a service on a host; the table decides what that means.
func TestLoad_HostService(t *testing.T) {
	c, err := Load([]byte(testFrameworks), []byte(hostServiceControls("west:dashboard-backend")))
	if err != nil {
		t.Fatalf("west:dashboard-backend rejected: %v", err)
	}
	if got := c.Controls[0].Evidence[0].Key(); got != "host-service:west:dashboard-backend" {
		t.Errorf("source key = %q", got)
	}
	for _, tc := range []struct{ name, ref, want string }{
		{"unknown service", "west:vibes", "not a service"},
		{"a space", "west:dashboard backend", "<host>:<service>"},
		{"a semicolon", "west:dashboard-backend;id", "<host>:<service>"},
		{"a slash", "west:dashboard/backend", "<host>:<service>"},
		{"a command", "west:$(id)", "<host>:<service>"},
		{"missing host", "dashboard-backend", "<host>:<service>"},
		{"empty host", ":dashboard-backend", "<host>:<service>"},
		{"host the service does not run on", "nowhere:dashboard-backend", "does not run on"},
		{"exempt service", "west:auth", "exempt"},
		{"service that runs on no host", "west:sword", "does not run on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load([]byte(testFrameworks), []byte(hostServiceControls(tc.ref)))
			if err == nil {
				t.Fatalf("Load accepted ref %q", tc.ref)
			}
			if !errors.Is(err, ErrInvalidCatalog) {
				t.Errorf("error %v does not wrap ErrInvalidCatalog", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// Every entry is complete enough to probe: a port, a route, one auth form.
func TestHostServices_Entries(t *testing.T) {
	routeRe := regexp.MustCompile(`^/[A-Za-z0-9/_-]*$`)
	nameRe := regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,62}$`)
	tokenRe := regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
	for name, svc := range HostServices {
		if !hostServiceNameRe.MatchString(name) {
			t.Errorf("%s: name is not a service name", name)
		}
		if svc.Exempt != "" {
			if svc.AuthVar != "" || len(svc.Runs) != 0 {
				t.Errorf("%s: an exempt service has nothing to read", name)
			}
			continue
		}
		if svc.Port < 1024 || svc.Port > 65535 {
			t.Errorf("%s: port %d", name, svc.Port)
		}
		if !routeRe.MatchString(svc.Route) {
			t.Errorf("%s: route %q", name, svc.Route)
		}
		switch svc.Method {
		case "GET", "POST", "DELETE":
		default:
			t.Errorf("%s: method %q", name, svc.Method)
		}
		if svc.Method != "GET" && svc.Why == "" {
			t.Errorf("%s: a probe that is not a GET says why", name)
		}
		if svc.AuthVar != "" && !tokenRe.MatchString(svc.AuthVar) {
			t.Errorf("%s: token variable %q", name, svc.AuthVar)
		}
		for host, run := range svc.Runs {
			if !hostServiceNameRe.MatchString(host) {
				t.Errorf("%s: host %q", name, host)
			}
			if (run.Container == "") == (run.Unit == "") {
				t.Errorf("%s on %s: exactly one of container and unit", name, host)
			}
			for _, v := range []string{run.Container, run.Unit, run.Package, run.TLSName} {
				if v != "" && !nameRe.MatchString(v) {
					t.Errorf("%s on %s: %q is not a plain name", name, host, v)
				}
			}
			if run.Unit != "" && run.Package == "" {
				t.Errorf("%s on %s: a unit names its package", name, host)
			}
			if run.Addr == "" && run.Container == "" {
				t.Errorf("%s on %s: a unit needs an address", name, host)
			}
			if run.Addr == "" && run.TLSName == "" {
				t.Errorf("%s on %s: a container address needs the certificate's name", name, host)
			}
			if !strings.HasPrefix(run.CA, "/") {
				t.Errorf("%s on %s: CA path %q", name, host, run.CA)
			}
		}
	}
	// Ports come from pkg/ports, not from a second list.
	for name, want := range map[string]int{
		"wotan": ports.WotanHTTP, "timeguru": ports.Timeguru, "akira": ports.Akira,
		"cuirass":           ports.Cuirass,
		"dashboard-backend": ports.DashboardBackend, "kanban-app": ports.KanbanApp,
		"logstore-writer": ports.LogstoreWriter, "sword": ports.Sword,
	} {
		if got := HostServices[name].Port; got != want {
			t.Errorf("%s: port %d, pkg/ports says %d", name, got, want)
		}
	}
}

// composeFirstParty is the gate's first-party set: every compose service
// with a build: key (scripts/compliance/check-service-auth.sh).
func composeFirstParty(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Build yaml.Node `yaml:"build"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var names []string
	for n, s := range doc.Services {
		if !s.Build.IsZero() {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// scriptTable returns the keys of a Python dict literal NAME = { ... } in
// the gate script, with the first quoted string of each value.
func scriptTable(t *testing.T, name string) (map[string]string, bool) {
	t.Helper()
	raw, err := os.ReadFile("../../../scripts/compliance/check-service-auth.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(raw), "\n"+name+" = {")
	if !ok {
		return nil, false
	}
	body, _, ok := strings.Cut(rest, "\n}")
	if !ok {
		t.Fatalf("%s has no closing brace at the start of a line", name)
	}
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*"([a-z0-9-]+)":\s*\(?\s*"([^"]*)"`).FindAllStringSubmatch(body, -1) {
		out[m[1]] = m[2]
	}
	return out, true
}

// The table and the compose gate list the same services, exempt the same
// one, and name the same token variables: neither can gain or lose a
// service without the other.
func TestHostServices_MatchTheComposeGate(t *testing.T) {
	var table []string
	for n := range HostServices {
		table = append(table, n)
	}
	sort.Strings(table)
	if got, want := strings.Join(table, " "), strings.Join(composeFirstParty(t), " "); got != want {
		t.Errorf("table and compose first-party services differ:\n table   %s\n compose %s", got, want)
	}

	exempt, ok := scriptTable(t, "EXEMPT")
	if !ok {
		t.Fatal("check-service-auth.sh has no EXEMPT table")
	}
	for n, svc := range HostServices {
		if _, in := exempt[n]; in != (svc.Exempt != "") {
			t.Errorf("%s: exempt in the gate %v, in the table %v", n, in, svc.Exempt != "")
		}
	}
	for n := range exempt {
		if _, in := HostServices[n]; !in {
			t.Errorf("the gate exempts %s, which the table does not list", n)
		}
	}

	t.Run("token table", func(t *testing.T) {
		tokens, ok := scriptTable(t, "TOKEN_AUTH")
		if !ok {
			// S-EVIDENCE-RUNS Phase 2 adds TOKEN_AUTH to the gate; the two
			// phases were written in parallel. With both merged this runs.
			t.Skip("check-service-auth.sh has no TOKEN_AUTH table yet (S-EVIDENCE-RUNS Phase 2)")
		}
		for n, svc := range HostServices {
			if got := tokens[n]; got != svc.AuthVar {
				t.Errorf("%s: token variable %q in the gate, %q in the table", n, got, svc.AuthVar)
			}
		}
		for n := range tokens {
			if _, in := HostServices[n]; !in {
				t.Errorf("the gate lists token service %s, which the table does not", n)
			}
		}
	})
}

// UH-AUTHN-01 reads every service on every host the table says runs it, and
// names nothing else: a service added to the table without a source would
// leave the control passing on a partial reading.
func TestRepoCatalog_AuthnReadsEveryHostService(t *testing.T) {
	c, err := LoadFS(os.DirFS("../../.."), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want, hosts := map[string]bool{}, map[string]bool{}
	for name, svc := range HostServices {
		for host := range svc.Runs {
			want[host+":"+name] = true
			hosts[host] = true
		}
	}
	if len(want) < 7 { // 12 until the five role services left (ADR-147 decision 7)
		t.Fatalf("the table gives only %d host-service refs", len(want))
	}
	for _, ctl := range c.Controls {
		for _, e := range ctl.Evidence {
			if e.Kind != KindHostService {
				continue
			}
			if ctl.ID != "UH-AUTHN-01" {
				t.Errorf("%s uses a host-service source; the kind says what UH-AUTHN-01 claims", ctl.ID)
				continue
			}
			if !want[e.Ref] {
				t.Errorf("UH-AUTHN-01 names %s twice, or the table does not hold it", e.Ref)
			}
			delete(want, e.Ref)
		}
		if ctl.ID == "UH-AUTHN-01" && ctl.Declared == "" && !hosts["east"] {
			// East is not read yet (S-E1-LIVE Step 218): until its
			// entries are in the table the control may not read PASS.
			t.Error("UH-AUTHN-01 lost declared: before east's services are in the table")
		}
	}
	for ref := range want {
		t.Errorf("UH-AUTHN-01 has no source for %s", ref)
	}
}
