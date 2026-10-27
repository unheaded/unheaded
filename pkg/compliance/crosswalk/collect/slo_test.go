// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// fakeVictoria answers /api/v1/query for slo:error_budget_remaining{slo="<name>"}
// from budgets (name -> sample values as Prometheus prints them; no entry
// is an empty result).
func fakeVictoria(t *testing.T, budgets map[string][]string, at float64) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query().Get("query")
		mu.Lock()
		queries = append(queries, q)
		mu.Unlock()
		name, ok := strings.CutPrefix(q, `slo:error_budget_remaining{slo="`)
		name, ok2 := strings.CutSuffix(name, `"}`)
		if !ok || !ok2 {
			http.Error(w, "bad query "+q, http.StatusBadRequest)
			return
		}
		var series []string
		for i, v := range budgets[name] {
			series = append(series, fmt.Sprintf(`{"metric":{"slo":%q,"service":"s%d"},"value":[%v,%q]}`, name, i, at, v))
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[%s]}}`, strings.Join(series, ","))
	}))
	t.Cleanup(srv.Close)
	return srv, &queries
}

func sloSources(names ...string) []crosswalk.Source {
	out := []crosswalk.Source{{Kind: crosswalk.KindGateScript, Ref: "scripts/x.sh"}}
	for _, n := range names {
		out = append(out, crosswalk.Source{Kind: crosswalk.KindSLO, Ref: n})
	}
	return out
}

func TestSLOCollect(t *testing.T) {
	now := time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)
	srv, queries := fakeVictoria(t, map[string][]string{
		"wotan":      {"0.97"},
		"ci-develop": {"-0.4"},
		"overlay":    {"0"},
		"well":       {"0.5", "0.2"}, // two series: the worse counts
		"dashboard":  {"NaN"},
	}, float64(now.Add(-30*time.Second).Unix()))
	c := &SLO{BaseURL: srv.URL, Now: func() time.Time { return now }}
	recs, err := c.Collect(context.Background(), sloSources("wotan", "ci-develop", "overlay", "well", "dashboard", "kev-feed"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]crosswalk.Record{}
	for _, r := range recs {
		got[r.Source.Key()] = r
	}
	want := map[string]crosswalk.Verdict{
		"slo:wotan":      crosswalk.VerdictPass,
		"slo:ci-develop": crosswalk.VerdictFail,
		"slo:overlay":    crosswalk.VerdictFail, // budget exactly spent
		"slo:well":       crosswalk.VerdictPass,
	}
	if len(got) != len(want) {
		t.Fatalf("records %v, want %v (dashboard NaN and absent kev-feed give none: NOT_ASSESSED)", recs, want)
	}
	for k, v := range want {
		if got[k].Verdict != v {
			t.Errorf("%s: %s, want %s (%s)", k, got[k].Verdict, v, got[k].Detail)
		}
	}
	if d := got["slo:well"].Detail; !strings.Contains(d, "20.0%") {
		t.Errorf("well detail %q: want the worse series (20.0%%)", d)
	}
	if !got["slo:wotan"].ObservedAt.Equal(now.Add(-30 * time.Second)) {
		t.Errorf("observed at %v, want the sample's time", got["slo:wotan"].ObservedAt)
	}
	if len(*queries) != 6 {
		t.Errorf("%d queries, want one per slo source: %v", len(*queries), *queries)
	}
}

// A sample stamped in the future (clock skew) is clamped to now: a
// future-dated record would not count as evidence at all.
func TestSLOCollectFutureSample(t *testing.T) {
	now := time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)
	srv, _ := fakeVictoria(t, map[string][]string{"wotan": {"0.9"}}, float64(now.Add(time.Hour).Unix()))
	recs, err := (&SLO{BaseURL: srv.URL, Now: func() time.Time { return now }}).Collect(context.Background(), sloSources("wotan"))
	if err != nil || len(recs) != 1 || !recs[0].ObservedAt.Equal(now) {
		t.Fatalf("recs %v, err %v", recs, err)
	}
}

func TestSLOCollectRefusals(t *testing.T) {
	srv, queries := fakeVictoria(t, nil, 0)
	ctx := context.Background()
	// No slo sources: no query at all.
	if recs, err := (&SLO{BaseURL: srv.URL}).Collect(ctx, sloSources()); err != nil || recs != nil || len(*queries) != 0 {
		t.Fatalf("no sources: %v %v %v", recs, err, *queries)
	}
	// A ref that is not an SLO name never reaches PromQL.
	for _, ref := range []string{`x"} or vector(1) or {a="`, "Wotan", "", strings.Repeat("a", 41)} {
		if _, err := (&SLO{BaseURL: srv.URL}).Collect(ctx, sloSources(ref)); err == nil {
			t.Errorf("ref %q accepted", ref)
		}
	}
	if len(*queries) != 0 {
		t.Errorf("a bad ref reached Victoria: %v", *queries)
	}
	for _, base := range []string{"ftp://x", "http://", "localhost:8428", "http://u:p@host:8428"} {
		if _, err := (&SLO{BaseURL: base}).Collect(ctx, sloSources("wotan")); err == nil {
			t.Errorf("base URL %q accepted", base)
		}
	}
}

func TestSLOCollectErrors(t *testing.T) {
	ctx := context.Background()
	for name, h := range map[string]http.HandlerFunc{
		"status 500":   func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", 500) },
		"not json":     func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "<html>") },
		"status error": func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"error","error":"bad"}`) },
		"matrix": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
		},
		"bad value": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"lots"]}]}}`)
		},
		"too big": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[],"pad":"`+strings.Repeat("x", MaxSLOResponseBytes)+`"}}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			if recs, err := (&SLO{BaseURL: srv.URL}).Collect(ctx, sloSources("wotan")); err == nil {
				t.Fatalf("want an error, got %v", recs)
			}
		})
	}
	// Unreachable: an error, so the caller says the evidence is unavailable.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if _, err := (&SLO{BaseURL: url}).Collect(ctx, sloSources("wotan")); err == nil {
		t.Fatal("unreachable Victoria: want an error")
	}
}

func TestSLOBaseURLFromEnv(t *testing.T) {
	t.Setenv("VICTORIA_URL", "")
	if got := (&SLO{}).baseURL(); got != DefaultVictoriaURL {
		t.Errorf("default %q", got)
	}
	t.Setenv("VICTORIA_URL", "http://victoria.invalid:8428/")
	if got := (&SLO{}).baseURL(); got != "http://victoria.invalid:8428" {
		t.Errorf("env %q", got)
	}
	if got := (&SLO{BaseURL: "http://x:1"}).baseURL(); got != "http://x:1" {
		t.Errorf("field %q", got)
	}
}
