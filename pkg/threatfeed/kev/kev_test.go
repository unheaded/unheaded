// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package kev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func catalogJSON(t *testing.T, version string, ids ...string) []byte {
	t.Helper()
	c := Catalog{Title: "test", CatalogVersion: version, DateReleased: "2026-10-19T00:00:00Z", Count: len(ids)}
	for _, id := range ids {
		c.Vulnerabilities = append(c.Vulnerabilities, Vulnerability{CVEID: id, VendorProject: "v", Product: "p"})
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseRealSnapshot(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/security/feeds/cisa-kev-2026-04-27.json")
	if err != nil {
		t.Skipf("archived feed not present: %v", err)
	}
	c, err := Parse(raw)
	if err != nil {
		t.Fatalf("real CISA feed rejected: %v", err)
	}
	if c.Count != 1583 || c.CatalogVersion != "2026.04.24" {
		t.Fatalf("got count %d version %q", c.Count, c.CatalogVersion)
	}
}

func TestParseRejects(t *testing.T) {
	good := catalogJSON(t, "1", "CVE-2024-0001", "CVE-2024-0002")
	if _, err := Parse(good); err != nil {
		t.Fatalf("good catalog rejected: %v", err)
	}
	cases := map[string][]byte{
		"not json":       []byte("<html>503</html>"),
		"trailing":       append(append([]byte{}, good...), []byte(` {"x":1}`)...),
		"no version":     catalogJSON(t, "", "CVE-2024-0001"),
		"empty":          catalogJSON(t, "1"),
		"duplicate":      catalogJSON(t, "1", "CVE-2024-0001", "CVE-2024-0001"),
		"malformed id":   catalogJSON(t, "1", "CVE-2024-1"),
		"injected id":    catalogJSON(t, "1", "CVE-2024-0001\n"),
		"count mismatch": []byte(`{"catalogVersion":"1","count":3,"vulnerabilities":[{"cveID":"CVE-2024-0001"}]}`),
	}
	for name, raw := range cases {
		if _, err := Parse(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}

func TestDiff(t *testing.T) {
	a, _ := Parse(catalogJSON(t, "1", "CVE-2024-0003", "CVE-2024-0001"))
	b, _ := Parse(catalogJSON(t, "2", "CVE-2024-0001", "CVE-2024-0009", "CVE-2024-0004"))
	if d := Diff(nil, b); !d.Empty() {
		t.Fatalf("bootstrap must be an empty delta, got %+v", d)
	}
	d := Diff(a, b)
	if len(d.Added) != 2 || d.Added[0].CVEID != "CVE-2024-0004" || d.Added[1].CVEID != "CVE-2024-0009" {
		t.Fatalf("added = %+v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0] != "CVE-2024-0003" {
		t.Fatalf("removed = %v", d.Removed)
	}
}

func TestStoreRoundTripAndModes(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "kev")}
	if c, st, err := s.Load(); c != nil || st != nil || err != nil {
		t.Fatalf("empty store: %v %v %v", c, st, err)
	}
	raw := catalogJSON(t, "1", "CVE-2024-0001")
	c, _ := Parse(raw)
	now := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	if err := s.Save(raw, c, State{ETag: `"abc"`, FetchedAt: now}, now); err != nil {
		t.Fatal(err)
	}
	got, st, err := s.Load()
	if err != nil || got.CatalogVersion != "1" || st.ETag != `"abc"` || st.SHA256 != Sum(raw) {
		t.Fatalf("load: %+v %+v %v", got, st, err)
	}
	for _, name := range []string{"latest.json", "state.json"} {
		fi, err := os.Stat(filepath.Join(s.Dir, name))
		if err != nil || fi.Mode().Perm() != 0o640 {
			t.Fatalf("%s mode %v err %v", name, fi.Mode().Perm(), err)
		}
	}
	left, _ := filepath.Glob(filepath.Join(s.Dir, ".*tmp-*"))
	if len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}

func TestStoreLoadRejectsCorruptLatest(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(s.Dir, "latest.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Load(); err == nil {
		t.Fatal("corrupt latest.json loaded")
	}
}

func TestStateIgnoredWhenItDescribesOtherBytes(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	raw := catalogJSON(t, "1", "CVE-2024-0001")
	c, _ := Parse(raw)
	if err := s.Save(raw, c, State{ETag: `"old"`}, time.Now()); err != nil {
		t.Fatal(err)
	}
	// latest.json replaced by hand: its ETag must not be sent any more.
	if err := os.WriteFile(filepath.Join(s.Dir, "latest.json"), catalogJSON(t, "2", "CVE-2024-0002"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, st, err := s.Load()
	if err != nil || st.ETag != "" {
		t.Fatalf("stale validators kept: %+v %v", st, err)
	}
}

func TestPrune(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	dir := filepath.Join(s.Dir, "snapshots")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	for _, age := range []time.Duration{0, 10 * 24 * time.Hour, 40 * 24 * time.Hour, 90 * 24 * time.Hour} {
		name := fmt.Sprintf("%s-deadbeef.json", now.Add(-age).Format("20060102T150405Z"))
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep"), 0o600)
	n, err := s.Prune(30*24*time.Hour, now)
	if err != nil || n != 2 {
		t.Fatalf("pruned %d, err %v", n, err)
	}
	// The newest is never pruned, however old.
	n, err = s.Prune(0, now.Add(365*24*time.Hour))
	left, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || n != 1 || len(left) != 1 {
		t.Fatalf("second prune: n=%d left=%v err=%v", n, left, err)
	}
}

func TestNewFetcherSchemes(t *testing.T) {
	for _, u := range []string{"http://example.com/kev.json", "ftp://x/y", "https://", "file:///etc/passwd"} {
		if _, err := NewFetcher(u, time.Second, false); err == nil {
			t.Errorf("%s accepted", u)
		}
	}
	if _, err := NewFetcher(DefaultURL, time.Second, false); err != nil {
		t.Fatal(err)
	}
}

func TestFetchRefusesSchemeDowngradeRedirect(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(catalogJSON(t, "1", "CVE-2024-0001"))
	}))
	defer plain.Close()
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer tls.Close()
	f, err := NewFetcher(tls.URL, 5*time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	f.Client.Transport = tls.Client().Transport
	if _, err := f.Fetch(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("https->http redirect followed: %v", err)
	}
}

func TestFetchSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
	}))
	defer srv.Close()
	f, _ := NewFetcher(srv.URL, 5*time.Second, true)
	f.MaxBytes = 1024
	if _, err := f.Fetch(context.Background(), nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversize body: %v", err)
	}
}

// feed is a fake CISA endpoint with an ETag per body.
type feed struct {
	body      atomic.Value // []byte
	status    atomic.Int32
	gotINM    atomic.Value // string
	no304     atomic.Bool
	requested atomic.Int32
}

func (f *feed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requested.Add(1)
	f.gotINM.Store(r.Header.Get("If-None-Match"))
	if s := f.status.Load(); s != 0 {
		w.WriteHeader(int(s))
		return
	}
	b := f.body.Load().([]byte)
	etag := `"` + Sum(b)[:16] + `"`
	if !f.no304.Load() && r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	_, _ = w.Write(b)
}

func TestPollerLifecycle(t *testing.T) {
	fd := &feed{}
	fd.body.Store(catalogJSON(t, "1", "CVE-2024-0001", "CVE-2024-0002"))
	srv := httptest.NewServer(fd)
	defer srv.Close()
	dir := t.TempDir()
	newPoller := func() *Poller {
		f, err := NewFetcher(srv.URL, 5*time.Second, true)
		if err != nil {
			t.Fatal(err)
		}
		p := &Poller{Fetcher: f, Store: Store{Dir: dir}}
		if err := p.Init(); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := newPoller()
	ctx := context.Background()

	out, err := p.Poll(ctx)
	if err != nil || !out.Bootstrap || !out.Delta.Empty() || out.State.Count != 2 {
		t.Fatalf("bootstrap: %+v %v", out, err)
	}

	out, err = p.Poll(ctx)
	if err != nil || !out.NotModified || fd.gotINM.Load().(string) == "" {
		t.Fatalf("second poll should be a conditional 304: %+v %v", out, err)
	}

	fd.no304.Store(true) // same bytes, no 304 support
	out, err = p.Poll(ctx)
	if err != nil || !out.NotModified {
		t.Fatalf("same bytes: %+v %v", out, err)
	}
	fd.no304.Store(false)

	fd.body.Store(catalogJSON(t, "2", "CVE-2024-0002", "CVE-2024-0003"))
	out, err = p.Poll(ctx)
	if err != nil || out.Bootstrap || len(out.Delta.Added) != 1 || out.Delta.Added[0].CVEID != "CVE-2024-0003" ||
		len(out.Delta.Removed) != 1 || out.Delta.Removed[0] != "CVE-2024-0001" {
		t.Fatalf("change: %+v %v", out, err)
	}

	// A bad feed and an HTTP error leave the last good catalog in place.
	good := p.Current().SHA256
	fd.body.Store([]byte(`{"catalogVersion":"3","count":9,"vulnerabilities":[]}`))
	if _, err := p.Poll(ctx); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad feed accepted: %v", err)
	}
	fd.status.Store(http.StatusServiceUnavailable)
	if _, err := p.Poll(ctx); err == nil {
		t.Fatal("503 accepted")
	}
	fd.status.Store(0)
	if p.Current().SHA256 != good {
		t.Fatal("failed polls replaced the stored catalog")
	}

	// A restart diffs against what is on disk, not against nothing.
	fd.body.Store(catalogJSON(t, "4", "CVE-2024-0002", "CVE-2024-0003", "CVE-2024-0004"))
	p2 := newPoller()
	out, err = p2.Poll(ctx)
	if err != nil || out.Bootstrap || len(out.Delta.Added) != 1 || out.Delta.Added[0].CVEID != "CVE-2024-0004" {
		t.Fatalf("after restart: %+v %v", out, err)
	}
	snaps, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*.json"))
	if len(snaps) != 3 {
		t.Fatalf("want one snapshot per content change (3), got %d", len(snaps))
	}
}

func TestPoller304WithoutStoredCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()
	f, _ := NewFetcher(srv.URL, time.Second, true)
	p := &Poller{Fetcher: f, Store: Store{Dir: t.TempDir()}}
	if _, err := p.Poll(context.Background()); err == nil {
		t.Fatal("304 with nothing stored must be an error")
	}
}
