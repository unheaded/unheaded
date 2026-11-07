// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package healthreport

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/health"
)

func reportJSON(t *testing.T, r health.HealthReport) []byte {
	t.Helper()
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now()
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// H6: the name a vote counts under comes from the token, not from the report.
func TestAdmit_ReporterComesFromTheToken(t *testing.T) {
	r, err := Admit("kanban-app", true, reportJSON(t, health.HealthReport{Service: "kanban", Reporter: "anyone-at-all", Host: "west"}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Reporter != "kanban@west" {
		t.Fatalf("reporter = %q, want kanban@west", r.Reporter)
	}
}

func TestAdmit_AnIdentityVotesOnlyOnItsOwnService(t *testing.T) {
	_, err := Admit("kev-poller", true, reportJSON(t, health.HealthReport{Service: "wotan", Reporter: "wotan@west", Host: "west"}))
	if !errors.Is(err, ErrNotOwnService) {
		t.Fatalf("err = %v, want ErrNotOwnService", err)
	}
}

// Akira's job is checking other services; its reporter is its node.
func TestAdmit_AkiraVotesOnAnyServiceAsItsNode(t *testing.T) {
	r, err := Admit("akira", true, reportJSON(t, health.HealthReport{Service: "wotan", Reporter: "east"}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Reporter != "east" {
		t.Fatalf("reporter = %q, want east", r.Reporter)
	}
}

func TestAdmit_UnverifiedReport(t *testing.T) {
	payload := reportJSON(t, health.HealthReport{Service: "wotan", Reporter: "wotan@west", Host: "west"})
	if _, err := Admit("", true, payload); !errors.Is(err, ErrUnverified) {
		t.Fatalf("auth on: err = %v, want ErrUnverified", err)
	}
	r, err := Admit("", false, payload)
	if err != nil {
		t.Fatalf("auth off: %v", err)
	}
	if r.Reporter != "wotan@west" {
		t.Fatalf("auth off: reporter = %q", r.Reporter)
	}
}

func TestAdmit_RefusesWhatCannotBeANodeName(t *testing.T) {
	for _, host := range []string{"", "a b", "a/b", "a@b", strings.Repeat("h", MaxNodeBytes+1)} {
		_, err := Admit("wotan", true, reportJSON(t, health.HealthReport{Service: "wotan", Reporter: "x", Host: host}))
		if !errors.Is(err, ErrBadNode) {
			t.Errorf("host %q: err = %v, want ErrBadNode", host, err)
		}
		_, err = Admit("akira", true, reportJSON(t, health.HealthReport{Service: "wotan", Reporter: host}))
		if !errors.Is(err, ErrBadNode) {
			t.Errorf("akira reporter %q: err = %v, want ErrBadNode", host, err)
		}
	}
}

func TestAdmit_RefusesAMalformedPayload(t *testing.T) {
	for _, p := range []string{"", "not json", `{"service":""}`} {
		if _, err := Admit("wotan", true, []byte(p)); !errors.Is(err, ErrBadReport) {
			t.Errorf("payload %q: err = %v, want ErrBadReport", p, err)
		}
	}
}

// A service's own report must survive the check under every name it can
// report as, or a legitimate vote disappears: each mapped identity, each
// plain identity, and any HEALTH_REPORT_SERVICE the compose stack sets.
func TestAdmit_EveryBallotNameIsOwned(t *testing.T) {
	for id, ballot := range ballotNames {
		if _, err := Admit(id, true, reportJSON(t, health.HealthReport{Service: ballot, Host: "west"})); err != nil {
			t.Errorf("%s as %s: %v", id, ballot, err)
		}
	}
	compose, err := os.ReadFile(filepath.Join("..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`HEALTH_REPORT_SERVICE:\s*"?([a-z0-9-]+)`).FindAllSubmatch(compose, -1)
	if len(found) == 0 {
		t.Fatal("no HEALTH_REPORT_SERVICE in docker-compose.yml; the walk is broken")
	}
	for _, m := range found {
		owned := false
		for id, names := range ballotAliases {
			for _, n := range names {
				if n == string(m[1]) {
					owned = true
					if _, err := Admit(id, true, reportJSON(t, health.HealthReport{Service: n, Host: "west"})); err != nil {
						t.Errorf("%s as %s: %v", id, n, err)
					}
				}
			}
		}
		if !owned {
			t.Errorf("compose reports as %q, but no identity owns that name in ballotAliases", m[1])
		}
	}
}
