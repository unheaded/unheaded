// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const goList = `{"Path":"unheaded","Main":true}
{"Path":"gopkg.in/yaml.v3","Version":"v3.0.1"}
{"Path":"github.com/BurntSushi/toml","Version":"v1.6.0"}
{"Path":"golang.org/x/net","Version":"v0.40.0","Indirect":true}
{"Path":"github.com/unheaded/doomgeneric","Version":"v0.0.0","Replace":{"Path":"../doom","Version":""}}
`

func TestReadModules(t *testing.T) {
	mods, err := readModules(strings.NewReader(goList))
	if err != nil || len(mods) != 5 || !mods[0].Main || !mods[3].Indirect || mods[4].Replace == nil {
		t.Fatalf("mods=%+v err=%v", mods, err)
	}
	if _, err := readModules(strings.NewReader(`{"Path":`)); err == nil {
		t.Error("truncated JSON should fail")
	}
}

func TestAuditMarksStaleDirectOnly(t *testing.T) {
	mods, _ := readModules(strings.NewReader(goList))
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	times := map[string]time.Time{
		"gopkg.in/yaml.v3":           time.Date(2022, 5, 27, 0, 0, 0, 0, time.UTC),
		"github.com/BurntSushi/toml": time.Date(2025, 12, 18, 0, 0, 0, 0, time.UTC),
		"golang.org/x/net":           time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	get := func(p string) (latest, error) { return latest{Version: "vX", Time: times[p]}, nil }

	rows := audit(mods, false, 24, now, get)
	if len(rows) != 2 {
		t.Fatalf("direct-only should skip main, indirect and local replace: %+v", rows)
	}
	if !rows[0].Stale || rows[0].Months != 53 || rows[1].Stale {
		t.Errorf("yaml.v3 53 months STALE, toml fresh: %+v", rows)
	}
	if rows := audit(mods, true, 24, now, get); len(rows) != 3 {
		t.Errorf("--all adds the indirect module: %+v", rows)
	}
	errRows := audit(mods, false, 24, now, func(string) (latest, error) { return latest{}, errors.New("boom") })
	if errRows[0].Err == "" {
		t.Error("a lookup failure must be reported, not skipped")
	}
}

func TestEscapePath(t *testing.T) {
	if got := escapePath("github.com/BurntSushi/toml"); got != "github.com/!burnt!sushi/toml" {
		t.Errorf("got %q", got)
	}
}

func TestFetchLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/!burnt!sushi/toml/@latest":
			_, _ = w.Write([]byte(`{"Version":"v1.6.0","Time":"2025-12-18T00:00:00Z"}`))
		case "/example.com/empty/@latest":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := srv.Client()
	l, err := fetchLatest(c, srv.URL, "github.com/BurntSushi/toml")
	if err != nil || l.Version != "v1.6.0" || l.Time.Year() != 2025 {
		t.Fatalf("l=%+v err=%v", l, err)
	}
	if _, err := fetchLatest(c, srv.URL, "example.com/missing"); err == nil {
		t.Error("404 should be an error")
	}
	if _, err := fetchLatest(c, srv.URL, "example.com/empty"); err == nil {
		t.Error("an answer without version/time should be an error")
	}
}
