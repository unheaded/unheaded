// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"unheaded/pkg/database"
	"unheaded/pkg/health"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	wotanClient "unheaded/pkg/wotan-client"
)

type storedReport struct {
	Service  string `json:"service"`
	Reporter string `json:"reporter"`
}

// storeThrough runs msgs through g, returning what reached the store.
func storeThrough(t *testing.T, g *healthReportGate, msgs ...*wotanClient.Message) []storedReport {
	t.Helper()
	ch := make(chan *wotanClient.Message, len(msgs))
	for _, m := range msgs {
		ch <- m
	}
	close(ch)
	var got []storedReport
	g.consume(context.Background(), ch, func(_ context.Context, payload json.RawMessage) error {
		var r storedReport
		if err := json.Unmarshal(payload, &r); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
		return nil
	})
	return got
}

func testGate(t *testing.T, authEnabled string) *healthReportGate {
	t.Helper()
	t.Setenv("AUTH_ENABLED", authEnabled)
	return newHealthReportGate(metrics.NewRegistry(), logger.New(io.Discard))
}

func reportFrom(sender, host string) *wotanClient.Message {
	return &wotanClient.Message{
		VerifiedSender: sender,
		Payload:        fmt.Sprintf(`{"service":"kanban","reporter":"invented","host":%q,"timestamp":"2026-11-07T00:00:00Z"}`, host),
	}
}

// The Well keeps a report under the reporter the token allows, not the
// reporter the payload names.
func TestHealthReportGate_TakesTheReporterFromTheToken(t *testing.T) {
	got := storeThrough(t, testGate(t, "true"), reportFrom("kanban-app", "west"))
	if len(got) != 1 || got[0].Reporter != "kanban@west" {
		t.Fatalf("stored = %+v, want one row for kanban@west", got)
	}
}

func TestHealthReportGate_RefusesAnotherServicesRow(t *testing.T) {
	g := testGate(t, "true")
	if got := storeThrough(t, g, reportFrom("kev-poller", "west")); len(got) != 0 {
		t.Fatalf("stored = %+v, want none", got)
	}
	if got := g.refused.WithLabelValues("not_own_service").Value(); got != 1 {
		t.Fatalf("not_own_service = %v, want 1", got)
	}
}

// The gate the server runs reads the service's own auth setting: with auth on
// a report no one verified is stored nowhere, and with it off it passes.
func TestHealthReportGate_AuthSettingDecidesAnUnverifiedReport(t *testing.T) {
	g := testGate(t, "true")
	if got := storeThrough(t, g, reportFrom("", "west")); len(got) != 0 {
		t.Fatalf("auth on: stored = %+v, want none", got)
	}
	if got := g.refused.WithLabelValues("unverified").Value(); got != 1 {
		t.Fatalf("unverified = %v, want 1", got)
	}
	if got := storeThrough(t, testGate(t, ""), reportFrom("", "west")); len(got) != 1 {
		t.Fatalf("auth off: stored = %+v, want the report as written", got)
	}
}

// One identity inventing hosts gets a bounded number of rows, and a reporter
// that already has a row keeps being stored.
func TestHealthReportGate_BoundsTheRowsOfAService(t *testing.T) {
	g := testGate(t, "true")
	var msgs []*wotanClient.Message
	for i := range health.MaxPeerReporters + 10 {
		msgs = append(msgs, reportFrom("kanban-app", fmt.Sprintf("host-%d", i)))
	}
	msgs = append(msgs, reportFrom("kanban-app", "host-0"))
	got := storeThrough(t, g, msgs...)
	if len(got) != health.MaxPeerReporters+1 {
		t.Fatalf("stored %d, want %d and the known reporter again", len(got), health.MaxPeerReporters+1)
	}
	if got := g.refused.WithLabelValues("too_many_rows").Value(); got != 10 {
		t.Fatalf("too_many_rows = %v, want 10", got)
	}
}

func TestHealthReportGate_BoundsTheServices(t *testing.T) {
	g := testGate(t, "true")
	var msgs []*wotanClient.Message
	for i := range health.MaxPeerServices + 1 {
		msgs = append(msgs, &wotanClient.Message{
			VerifiedSender: "akira",
			Payload:        fmt.Sprintf(`{"service":"svc-%d","reporter":"west","timestamp":"2026-11-07T00:00:00Z"}`, i),
		})
	}
	if got := storeThrough(t, g, msgs...); len(got) != health.MaxPeerServices {
		t.Fatalf("stored %d, want %d", len(got), health.MaxPeerServices)
	}
}

// What the bridge calls a bad report is a counted refusal, not a log line each.
func TestHealthReportGate_CountsWhatTheBridgeRefuses(t *testing.T) {
	g := testGate(t, "true")
	ch := make(chan *wotanClient.Message, 1)
	ch <- reportFrom("kanban-app", "west")
	close(ch)
	g.consume(context.Background(), ch, func(context.Context, json.RawMessage) error { return database.ErrBadReport })
	if got := g.refused.WithLabelValues("malformed").Value(); got != 1 {
		t.Fatalf("malformed = %v, want 1", got)
	}
}
