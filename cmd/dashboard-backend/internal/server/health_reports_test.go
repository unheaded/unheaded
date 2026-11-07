// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"encoding/json"
	"errors"
	"testing"

	"unheaded/pkg/healthreport"
	wotanClient "unheaded/pkg/wotan-client"
)

// The Well keeps a report under the reporter the token allows, not the
// reporter the payload names.
func TestAdmittedReport_TakesTheReporterFromTheToken(t *testing.T) {
	m := &wotanClient.Message{
		VerifiedSender: "kanban-app",
		Payload:        `{"service":"kanban","reporter":"invented","host":"west","timestamp":"2026-11-07T00:00:00Z"}`,
	}
	got, err := admittedReport(m, true)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Reporter string `json:"reporter"`
	}
	if err := json.Unmarshal(got, &r); err != nil {
		t.Fatal(err)
	}
	if r.Reporter != "kanban@west" {
		t.Fatalf("reporter = %q, want kanban@west", r.Reporter)
	}
}

func TestAdmittedReport_RefusesAnotherServicesRow(t *testing.T) {
	m := &wotanClient.Message{
		VerifiedSender: "kev-poller",
		Payload:        `{"service":"wotan","reporter":"wotan@west","host":"west","timestamp":"2026-11-07T00:00:00Z"}`,
	}
	if _, err := admittedReport(m, true); !errors.Is(err, healthreport.ErrNotOwnService) {
		t.Fatalf("err = %v, want ErrNotOwnService", err)
	}
}
