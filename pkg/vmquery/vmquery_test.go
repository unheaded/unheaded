// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package vmquery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeVictoria(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestInstantVector(t *testing.T) {
	var gotQuery string
	c := fakeVictoria(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("path %q, want /api/v1/query", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("query")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
			`{"metric":{"service":"wotan"},"value":[1700000000.5,"3.25"]},` +
			`{"metric":{"service":"timeguru"},"value":[1700000000.5,"NaN"]}]}}`))
	})
	q := `sum by (service) (rate(unheaded_http_requests_total{status=~"5.."}[1m]))`
	got, err := c.Instant(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != q {
		t.Errorf("query sent %q, want %q", gotQuery, q)
	}
	if len(got) != 2 {
		t.Fatalf("samples = %d, want 2", len(got))
	}
	if got[0].Metric["service"] != "wotan" || got[0].Value != 3.25 || got[0].Raw != "3.25" {
		t.Errorf("sample 0 = %+v", got[0])
	}
	if !got[0].Time.Equal(time.Unix(1700000000, 500000000)) {
		t.Errorf("time = %v", got[0].Time)
	}
	if got[1].Value == got[1].Value { // NaN is not equal to itself
		t.Errorf("NaN decoded as %v", got[1].Value)
	}
}

func TestRange(t *testing.T) {
	var form map[string]string
	c := fakeVictoria(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Errorf("path %q, want /api/v1/query_range", r.URL.Path)
		}
		form = map[string]string{}
		for _, k := range []string{"query", "start", "end", "step"} {
			form[k] = r.URL.Query().Get(k)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
			`{"metric":{"host":"west"},"values":[[100,"1"],[160,"2.5"]]}]}}`))
	})
	start, end := time.Unix(100, 0), time.Unix(160, 0)
	got, err := c.Range(context.Background(), "node_load1", start, end, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if form["query"] != "node_load1" || form["start"] != "100" || form["end"] != "160" || form["step"] != "60" {
		t.Errorf("parameters = %v", form)
	}
	if len(got) != 1 || got[0].Metric["host"] != "west" || len(got[0].Points) != 2 {
		t.Fatalf("series = %+v", got)
	}
	if p := got[0].Points[1]; p.Value != 2.5 || !p.Time.Equal(time.Unix(160, 0)) {
		t.Errorf("point 1 = %+v", p)
	}
	if _, err := c.Range(context.Background(), "x", end, start, time.Minute); err == nil {
		t.Error("end before start accepted")
	}
	if _, err := c.Range(context.Background(), "x", start, end, 0); err == nil {
		t.Error("zero step accepted")
	}
}

func TestAnswerCapped(t *testing.T) {
	big := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"x":"` +
		strings.Repeat("a", MaxAnswer) + `"},"value":[1,"1"]}]}}`
	c := fakeVictoria(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	})
	got, err := c.Instant(context.Background(), "x")
	if err == nil || got != nil {
		t.Fatalf("answer over %d bytes decoded: %d samples, err %v", MaxAnswer, len(got), err)
	}
}

func TestNon200(t *testing.T) {
	c := fakeVictoria(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := c.Instant(context.Background(), "up")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want one naming 500", err)
	}
	c = fakeVictoria(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":"bad query"}`))
	})
	if _, err := c.Instant(context.Background(), "up"); err == nil {
		t.Fatal("status error accepted")
	}
}

func TestTimeout(t *testing.T) {
	release := make(chan struct{})
	c := fakeVictoria(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	c.timeout = 100 * time.Millisecond
	start := time.Now()
	_, err := c.Instant(context.Background(), "up")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v, want about the timeout", d)
	}
}

func TestNoBaseURL(t *testing.T) {
	if _, err := New("", nil); !errors.Is(err, ErrNoVictoria) {
		t.Errorf("empty URL: err = %v, want ErrNoVictoria", err)
	}
	for _, bad := range []string{"ftp://x", "http://", "http://user:pw@host:8428", "::"} {
		if _, err := New(bad, nil); err == nil {
			t.Errorf("New(%q) accepted", bad)
		}
	}
}
