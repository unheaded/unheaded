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
	"sync"
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

// categoryVor answers /api/topics/saml the way a vor of 2026-11-12 or later
// does: ?category= picks the sheet. It counts requests and keeps the last
// request URI.
type categoryVor struct {
	mu       sync.Mutex
	requests int
	last     string
}

func (f *categoryVor) saw(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	f.last = r.URL.RequestURI()
}

// seen returns how many requests came and the URI of the last.
func (f *categoryVor) seen() (int, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.last
}

func (f *categoryVor) client(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/topics/", func(w http.ResponseWriter, r *http.Request) {
		f.saw(r)
		name := strings.TrimPrefix(r.URL.Path, "/api/topics/")
		category := r.URL.Query().Get("category")
		switch {
		case strings.EqualFold(name, "saml") && (category == "auth" || category == "security"):
			_, _ = w.Write([]byte(`{"name":"saml","category":"` + category + `","content":"# ` + category + `\n","source_trust":"canonical"}`))
		case name == "README" && (category == "docs/adr" || category == "wiki"):
			// A vor of 2026-11-22 or later takes a directory as a category.
			_, _ = w.Write([]byte(`{"name":"README","category":"` + category + `","source_trust":"external"}`))
		case name == "old" && category == "auth":
			// A vor from before 2026-11-12 ignores category and answers
			// with whichever sheet wins the name.
			_, _ = w.Write([]byte(`{"name":"old","category":"security","source_trust":"canonical"}`))
		case name == "fuzzy" && category == "auth":
			// And answers an unknown name with its nearest match.
			_, _ = w.Write([]byte(`{"name":"fuzzy-logic","category":"auth","source_trust":"canonical"}`))
		case name == "broken":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	})
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		f.saw(r)
		_, _ = w.Write([]byte("null"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTopicIn(t *testing.T) {
	f := &categoryVor{}
	c := f.client(t)
	ctx := context.Background()

	got, err := c.TopicIn(ctx, "auth", "saml")
	if err != nil || got.Category != "auth" || got.Name != "saml" {
		t.Fatalf("TopicIn(auth, saml) = %+v, %v", got, err)
	}
	if _, last := f.seen(); last != "/api/topics/saml?category=auth" {
		t.Errorf("request = %s, want /api/topics/saml?category=auth", last)
	}
	got, err = c.TopicIn(ctx, "security", "saml")
	if err != nil || got.Category != "security" {
		t.Fatalf("TopicIn(security, saml) = %+v, %v", got, err)
	}
	// The name matches in any case, as vor matches it.
	if got, err = c.TopicIn(ctx, "auth", "SAML"); err != nil || got.Name != "saml" {
		t.Errorf("TopicIn(auth, SAML) = %+v, %v", got, err)
	}
	// A directory is a category.
	got, err = c.TopicIn(ctx, "docs/adr", "README")
	if err != nil || got.Category != "docs/adr" {
		t.Fatalf("TopicIn(docs/adr, README) = %+v, %v", got, err)
	}
	if _, last := f.seen(); last != "/api/topics/README?category=docs%2Fadr" {
		t.Errorf("request = %s, want /api/topics/README?category=docs%%2Fadr", last)
	}

	for _, c2 := range []struct{ category, name, why string }{
		{"docs/wiki", "README", "404"},
		{"nosuch", "saml", "404"},
		{"auth", "nosuch", "404"},
		{"auth", "old", "the reply is from another category"},
		{"auth", "fuzzy", "the reply is another topic"},
	} {
		if _, err := c.TopicIn(ctx, c2.category, c2.name); !errors.Is(err, ErrNotFound) {
			t.Errorf("TopicIn(%s, %s) = %v, want ErrNotFound (%s)", c2.category, c2.name, err, c2.why)
		}
	}

	if _, err := c.TopicIn(ctx, "auth", "broken"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("TopicIn against a 500 = %v, want an error that is not ErrNotFound", err)
	}
}

func TestTopicIn_RefusesWithoutARequest(t *testing.T) {
	f := &categoryVor{}
	c := f.client(t)
	ctx := context.Background()

	// An empty category is the caller's mistake, not a missing topic.
	if _, err := c.TopicIn(ctx, "", "saml"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("TopicIn with no category = %v, want an error that is not ErrNotFound", err)
	}
	// Never names, never categories. A category may be a directory, under
	// the rule vor applies (validCategory); a name is one segment.
	for _, p := range [][2]string{
		{"auth", ""}, {"auth", "."}, {"auth", ".."}, {"auth", "a/b"}, {"auth", "a..b"},
		{".", "saml"}, {"..", "saml"}, {"a..b", "saml"},
		{"/", "saml"}, {"/docs", "saml"}, {"docs/", "saml"}, {"docs//adr", "saml"},
		{"docs/./adr", "saml"}, {"docs/../adr", "saml"}, {"../docs", "saml"}, {"docs/..", "saml"},
		{"docs\\adr", "saml"}, {"docs/adr\x00", "saml"}, {strings.Repeat("a/", 128) + "a", "saml"},
	} {
		if _, err := c.TopicIn(ctx, p[0], p[1]); !errors.Is(err, ErrNotFound) {
			t.Errorf("TopicIn(%q, %q) = %v, want ErrNotFound", p[0], p[1], err)
		}
	}
	if n, _ := f.seen(); n != 0 {
		t.Errorf("%d requests sent, want none", n)
	}
}

func TestSearch_Words(t *testing.T) {
	f := &categoryVor{}
	c := f.client(t)
	ctx := context.Background()

	if _, err := c.Search(ctx, "ccna", SearchOptions{Words: true}); err != nil {
		t.Fatal(err)
	}
	if _, last := f.seen(); !strings.Contains(last, "match=word") {
		t.Errorf("Words: request = %s, want match=word", last)
	}
	if _, err := c.Search(ctx, "ccna", SearchOptions{Category: "certs"}); err != nil {
		t.Fatal(err)
	}
	if _, last := f.seen(); strings.Contains(last, "match") {
		t.Errorf("no Words: request = %s, want no match parameter", last)
	}
}

func TestTopicFor(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/topics/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.RequestURI())
		mu.Unlock()
		name := strings.TrimPrefix(r.URL.Path, "/api/topics/")
		category := r.URL.Query().Get("category")
		if category == "" {
			category = "docs/adr" // what vor has under the bare name
		}
		_, _ = w.Write([]byte(`{"name":"` + name + `","category":"` + category + `"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	for _, tc := range []struct{ category, name, wantRequest, wantCategory string }{
		// One segment or a directory: asked by category.
		{"auth", "saml", "/api/topics/saml?category=auth", "auth"},
		{"docs/adr", "ADR-001", "/api/topics/ADR-001?category=docs%2Fadr", "docs/adr"},
		{"wiki/ops", "README", "/api/topics/README?category=wiki%2Fops", "wiki/ops"},
		// The top of a directory, nothing, or what vor would refuse: by
		// name alone.
		{".", "README", "/api/topics/README", "docs/adr"},
		{"", "README", "/api/topics/README", "docs/adr"},
		{"a/../b", "x", "/api/topics/x", "docs/adr"},
	} {
		mu.Lock()
		requests = nil
		mu.Unlock()
		got, err := c.TopicFor(ctx, tc.category, tc.name)
		if err != nil || got.Category != tc.wantCategory {
			t.Errorf("TopicFor(%q, %q) = %+v, %v; want category %q", tc.category, tc.name, got, err, tc.wantCategory)
		}
		mu.Lock()
		if len(requests) != 1 || requests[0] != tc.wantRequest {
			t.Errorf("TopicFor(%q, %q) asked %q, want %q", tc.category, tc.name, requests, tc.wantRequest)
		}
		mu.Unlock()
	}
}

// A vor from before 2026-11-22 answers a directory category with 400
// "invalid category". TopicFor then asks by name alone, as it did for every
// directory before; a one-segment category gets no second request.
func TestTopicFor_VorWithoutDirectoryCategories(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/topics/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.RequestURI())
		mu.Unlock()
		name := strings.TrimPrefix(r.URL.Path, "/api/topics/")
		category := r.URL.Query().Get("category")
		switch {
		case strings.Contains(category, "/"):
			http.Error(w, `{"error":"invalid category"}`, http.StatusBadRequest)
		case category == "" && name == "README":
			_, _ = w.Write([]byte(`{"name":"README","category":"wiki"}`))
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	got, err := c.TopicFor(ctx, "docs/adr", "README")
	if err != nil || got.Category != "wiki" {
		t.Errorf("TopicFor(docs/adr, README) against an older vor = %+v, %v; want the bare-name topic", got, err)
	}
	mu.Lock()
	want := []string{"/api/topics/README?category=docs%2Fadr", "/api/topics/README"}
	if strings.Join(requests, " ") != strings.Join(want, " ") {
		t.Errorf("asked %q, want %q", requests, want)
	}
	requests = nil
	mu.Unlock()

	// TopicIn stays exact: no fallback.
	if _, err := c.TopicIn(ctx, "docs/adr", "README"); !errors.Is(err, ErrNotFound) {
		t.Errorf("TopicIn(docs/adr, README) against an older vor = %v, want ErrNotFound", err)
	}
	// A one-segment category that is not there is not found, once.
	if _, err := c.TopicFor(ctx, "auth", "README"); !errors.Is(err, ErrNotFound) {
		t.Errorf("TopicFor(auth, README) = %v, want ErrNotFound", err)
	}
	mu.Lock()
	if len(requests) != 2 {
		t.Errorf("asked %q, want one request each", requests)
	}
	mu.Unlock()
}

func TestValidCategory(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]bool{
		"auth": true, "docs/adr": true, "a/b/c": true, "ramp-up": true, "x.y/z": true,
		"": false, ".": false, "..": false, "/": false, "a/": false, "/a": false, "a//b": false,
		"a/./b": false, "a/../b": false, "a..b": false, "a\\b": false, "a\x00b": false,
		strings.Repeat("a", MaxCategoryBytes): true, strings.Repeat("a", MaxCategoryBytes+1): false,
	} {
		if got := validCategory(s); got != want {
			t.Errorf("validCategory(%q) = %v, want %v", s, got, want)
		}
	}
}
