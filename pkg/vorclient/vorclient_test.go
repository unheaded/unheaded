// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package vorclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeVor answers like vor: /api/search echoes the query in one hit, and
// /api/topics/<name> knows "bash" (any case) and answers every other name
// with its nearest match, as vor does.
func fakeVor(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		switch q := r.URL.Query().Get("q"); q {
		case "":
			http.Error(w, `{"error":"missing q parameter"}`, http.StatusBadRequest)
		case "nothing":
			_, _ = w.Write([]byte("null"))
		default:
			// The options come back in the section, so a test sees what vor got.
			v := r.URL.Query()
			opts := "category=" + v.Get("category") + " limit=" + v.Get("limit") + " per_topic=" + v.Get("per_topic")
			for _, k := range []string{"category", "limit", "per_topic"} {
				if v.Has(k) && v.Get(k) == "" {
					opts += " empty " + k
				}
			}
			_ = json.NewEncoder(w).Encode([]Hit{{Topic: "bash", Category: "shell", Section: opts, Line: q,
				SourceKind: "embedded", SourceTrust: TrustCanonical}})
		}
	})
	mux.HandleFunc("/api/topics", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"bash","category":"shell","title":"Bash","description":"The shell.",` +
			`"has_detail":true,"see_also":["zsh"],"source_kind":"embedded","source_trust":"canonical"},` +
			`{"name":"Home","category":"wiki","title":"Home"}]`))
	})
	mux.HandleFunc("/api/topics/", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/api/topics/") {
		case "":
			http.Error(w, `{"error":"invalid topic name"}`, http.StatusBadRequest)
		case "gone":
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		case "broken":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "huge":
			_, _ = w.Write([]byte(`{"name":"huge","content":"` + strings.Repeat("x", MaxResponseBytes) + `"}`))
		case "a b?c":
			_, _ = w.Write([]byte(`{"name":"a b?c","source_trust":"canonical"}`))
		default:
			_, _ = w.Write([]byte(`{"name":"bash","category":"shell","title":"Bash","content":"# bash\n",` +
				`"see_also":["zsh"],"source_kind":"embedded","source_trust":"canonical"}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNew_RefusesWhatIsNotAnHTTPURL(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "127.0.0.1:20110", "ftp://host", "http://", "file:///etc/passwd", "http://[::1"} {
		if _, err := New(bad, nil); err == nil {
			t.Errorf("New(%q): no error", bad)
		}
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()
	c := fakeVor(t)

	// The query reaches vor as one parameter, whatever it holds.
	const q = `for loop & "x" #frag ?a=b <b>`
	hits, err := c.Search(context.Background(), q, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// No options: none sent, so vor's defaults hold.
	want := Hit{Topic: "bash", Category: "shell", Section: "category= limit= per_topic=", Line: q,
		SourceKind: "embedded", SourceTrust: TrustCanonical}
	if len(hits) != 1 || hits[0] != want {
		t.Errorf("hits = %+v, want [%+v]", hits, want)
	}

	// Options reach vor as their own parameters, escaped like the query.
	hits, err = c.Search(context.Background(), q, SearchOptions{Category: "docs/a b&c", Limit: 500, PerTopic: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Section != "category=docs/a b&c limit=500 per_topic=2" || hits[0].Line != q {
		t.Errorf("with options: hits = %+v", hits)
	}

	if hits, err := c.Search(context.Background(), "nothing", SearchOptions{}); err != nil || len(hits) != 0 {
		t.Errorf("no match: hits = %v, err = %v; want none, nil", hits, err)
	}
	if _, err := c.Search(context.Background(), "", SearchOptions{}); err == nil {
		t.Error("empty query: vor's 400 was not reported")
	}
}

func TestTopic(t *testing.T) {
	t.Parallel()
	c := fakeVor(t)

	got, err := c.Topic(context.Background(), "bash")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "bash" || got.Title != "Bash" || got.Content != "# bash\n" || got.SourceTrust != TrustCanonical ||
		len(got.SeeAlso) != 1 {
		t.Errorf("topic = %+v", got)
	}

	// A name is one path segment, escaped.
	if got, err := c.Topic(context.Background(), "a b?c"); err != nil || got.Name != "a b?c" {
		t.Errorf("escaped name: topic = %+v, err = %v", got, err)
	}
}

func TestTopics(t *testing.T) {
	t.Parallel()
	got, err := fakeVor(t).Topics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "bash" || got[0].Category != "shell" || got[0].Title != "Bash" ||
		got[0].Description != "The shell." || !got[0].HasDetail || len(got[0].SeeAlso) != 1 ||
		got[0].SourceKind != "embedded" || got[0].SourceTrust != TrustCanonical ||
		got[1].Name != "Home" || got[1].Category != "wiki" || got[1].SourceTrust != "" {
		t.Errorf("topics = %+v", got)
	}

	down, err := New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := down.Topics(context.Background()); err == nil {
		t.Error("vor down: Topics returned no error")
	}
}

func TestTopic_NotFound(t *testing.T) {
	t.Parallel()
	c := fakeVor(t)
	for _, name := range []string{
		"nosuch",   // vor answers 200 with its nearest match
		"Bash",     // vor ignores case; the reply is "bash", not what was asked
		"gone",     // 404
		"",         // 400
		".",        // would resolve to /api/topics, the listing
		"..",       // would resolve to /api
		"../admin", // never a topic
		"a/b",      // never a topic
	} {
		if got, err := c.Topic(context.Background(), name); !errors.Is(err, ErrNotFound) {
			t.Errorf("Topic(%q) = %+v, %v; want ErrNotFound", name, got, err)
		}
	}
}

func TestTopic_FailuresAreNotNotFound(t *testing.T) {
	t.Parallel()
	c := fakeVor(t)
	for _, name := range []string{"broken", "huge"} {
		if _, err := c.Topic(context.Background(), name); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("Topic(%q): err = %v; want a failure that is not ErrNotFound", name, err)
		}
	}

	down, err := New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := down.Topic(context.Background(), "bash"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("vor down: err = %v; want a failure that is not ErrNotFound", err)
	}
	if _, err := down.Search(context.Background(), "bash", SearchOptions{}); err == nil {
		t.Error("vor down: Search returned no error")
	}
}

func TestCallsCarryADeadline(t *testing.T) {
	t.Parallel()
	seen := make(chan bool, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("null"))
		seen <- true
	}))
	t.Cleanup(srv.Close)

	// The caller's client and context set no limit; the call still has one.
	var deadline time.Time
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, _ = r.Context().Deadline()
		return http.DefaultTransport.RoundTrip(r)
	})}
	c, err := New(srv.URL, hc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Search(context.Background(), "x", SearchOptions{}); err != nil {
		t.Fatal(err)
	}
	<-seen
	if left := time.Until(deadline); deadline.IsZero() || left > Timeout {
		t.Errorf("deadline = %v (%v from now), want one within %v", deadline, left, Timeout)
	}

	// A caller's cancelled context stops the call.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Search(ctx, "x", SearchOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled context: err = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
