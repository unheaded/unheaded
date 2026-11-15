// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/vorclient"
)

func gateRefuses(refs []champion.Reference) bool {
	return (&champion.ToolCall{Name: "write_file", Justification: refs}).HasUntrustedJustification()
}

func names(refs []champion.Reference) string {
	var s []string
	for _, r := range refs {
		s = append(s, r.Topic+"="+r.SourceTrust)
	}
	return strings.Join(s, " ")
}

// Real vor, the deployed retriever, vor alone, k=5 (agent.DefaultRetrieveK).
func TestAdv_RealVor_ExternalPastKInsideTheBackend(t *testing.T) {
	u := os.Getenv("ADV_VOR_URL")
	if u == "" {
		t.Skip("no ADV_VOR_URL")
	}
	refs, _, err := newRetriever(&http.Client{Timeout: 5 * time.Second}, u, "", "", 2*time.Second).Retrieve(context.Background(), "zetaword", 5)
	t.Logf("refs: %s", names(refs))
	if err != nil {
		t.Fatal(err)
	}
	if !gateRefuses(refs) {
		t.Fatal("vor's search returned an external hit; the retriever cut it at k and the gate would pass")
	}
}

// Real vor: the search hit is external, the fetch by flat name returns the
// longer trusted sheet of the same name.
func TestAdv_RealVor_FlatNameMasksExternalHit(t *testing.T) {
	u := os.Getenv("ADV_VOR_URL")
	if u == "" {
		t.Skip("no ADV_VOR_URL")
	}
	refs, _, err := newRetriever(&http.Client{Timeout: 5 * time.Second}, u, "", "", 2*time.Second).Retrieve(context.Background(), "zetaclash", 5)
	t.Logf("refs: %s", names(refs))
	if err != nil {
		t.Fatal(err)
	}
	if !gateRefuses(refs) {
		t.Fatal("search said external, the ref says trusted")
	}
}

// Stub vor: search answers, the external topic's fetch fails, nil error out.
func TestAdv_TopicFetchFailureDropsTheExternalRef(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"topic": "ok", "category": "c", "source_trust": "canonical", "source_kind": "embedded"},
			{"topic": "poison", "category": "c", "source_trust": "external", "source_kind": "user-source"},
		})
	})
	mux.HandleFunc("/api/topics", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("[]")) })
	mux.HandleFunc("/api/topics/ok", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "ok", "category": "c", "content": "x", "source_trust": "canonical"})
	})
	mux.HandleFunc("/api/topics/poison", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) })
	s := httptest.NewServer(mux)
	defer s.Close()
	refs, _, err := newRetriever(s.Client(), s.URL, "", "", 2*time.Second).Retrieve(context.Background(), "poison", 5)
	t.Logf("refs: %s err=%v", names(refs), err)
	if err == nil && !gateRefuses(refs) {
		t.Fatal("a failed fetch of an external hit left a clean chain")
	}
}

// A sheet the retriever read and then refused as a source (it does not carry
// the question's words) is still something retrieval touched. If it is
// untrusted, the gate hears of it, with no text.
func TestAdv_RejectedExternalTopicStillReachesTheGate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		// vor matches substrings: "ccna" finds the sheet through KRB5CCNAME.
		if strings.Contains(r.URL.Query().Get("q"), "ccna") {
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"topic": "poison", "category": "c", "source_trust": "external", "source_kind": "user-source"},
			})
			return
		}
		_, _ = w.Write([]byte("[]"))
	})
	mux.HandleFunc("/api/topics", func(w http.ResponseWriter, r *http.Request) {
		// Named for a query word, returned by no search: the second pass reads it.
		_ = json.NewEncoder(w).Encode([]map[string]string{{"name": "exam-notes", "category": "c"}})
	})
	mux.HandleFunc("/api/topics/poison", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "poison", "category": "c", "content": "export KRB5CCNAME=/tmp/x", "source_trust": "external"})
	})
	mux.HandleFunc("/api/topics/exam-notes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "exam-notes", "category": "c", "content": "nothing on the subject", "source_trust": "external"})
	})
	s := httptest.NewServer(mux)
	defer s.Close()

	for _, q := range []string{"ccna", "tell me about the exam for xyzzy"} {
		refs, contents, err := newRetriever(s.Client(), s.URL, "", "", 2*time.Second).Retrieve(context.Background(), q, 5)
		t.Logf("%q refs: %s err=%v", q, names(refs), err)
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		if len(contents) != 0 {
			t.Errorf("%q: a refused sheet's text reached the model: %d contents", q, len(contents))
		}
		if !gateRefuses(refs) {
			t.Errorf("%q: an external sheet was read and refused, and the chain came back clean", q)
		}
	}
}

// A vor body past vorclient's bound is a failed read, not memory: the topic
// gives no text, and an untrusted hit behind it still reaches the gate.
// Before 2026-11-12 the retriever decoded whatever length vor sent.
func TestAdv_OversizedVorBodyFailsBounded(t *testing.T) {
	huge := strings.Repeat("x", vorclient.MaxResponseBytes)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"topic": "poison", "category": "c", "source_trust": "external", "source_kind": "user-source"},
		})
	})
	mux.HandleFunc("/api/topics", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("[]")) })
	mux.HandleFunc("/api/topics/poison", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "poison", "category": "c", "content": "poison " + huge, "source_trust": "external"})
	})
	s := httptest.NewServer(mux)
	defer s.Close()

	refs, contents, err := newRetriever(s.Client(), s.URL, "", "", 5*time.Second).Retrieve(context.Background(), "poison", 5)
	t.Logf("refs: %s err=%v", names(refs), err)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 0 {
		t.Errorf("an oversized topic's text reached the model: %d contents", len(contents))
	}
	if !gateRefuses(refs) {
		t.Error("an oversized external topic failed to read and the chain came back clean")
	}

	// The search answer is bounded the same way: past the bound it is an
	// error, never a partial list.
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{{"topic": "poison", "category": "c", "source_trust": "external", "source_path": huge}})
	})
	mux2.HandleFunc("/api/topics", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("[]")) })
	s2 := httptest.NewServer(mux2)
	defer s2.Close()
	if _, contents, err := newRetriever(s2.Client(), s2.URL, "", "", 5*time.Second).Retrieve(context.Background(), "poison", 5); err == nil || len(contents) != 0 {
		t.Errorf("an oversized search answer: err=%v, %d contents; want an error and no text", err, len(contents))
	}
}

// The hit says which category its topic is in, and the read asks for that
// category. A vor that answers the name with another topic (one from before
// 2026-11-12 ignores ?category=, and the longer sheet of the name wins) gets
// no text into the answer under the hit's name, and the external hit still
// reaches the gate.
func TestAdv_SameNameInAnotherCategoryIsNotRead(t *testing.T) {
	for _, tc := range []struct {
		name   string
		honour bool // the vor honours ?category=
	}{
		{"vor that honours the category", true},
		{"vor that ignores the category", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var asked []string
			var mu sync.Mutex
			mux := http.NewServeMux()
			mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode([]map[string]string{
					{"topic": "clash", "category": "notes", "source_trust": "external", "source_kind": "user-source"},
				})
			})
			mux.HandleFunc("/api/topics", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("[]")) })
			mux.HandleFunc("/api/topics/clash", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				asked = append(asked, r.URL.RawQuery)
				mu.Unlock()
				if tc.honour && r.URL.Query().Get("category") == "notes" {
					_ = json.NewEncoder(w).Encode(map[string]string{"name": "clash", "category": "notes", "content": "clash, from the external notes", "source_trust": "external"})
					return
				}
				// The longer, trusted sheet of the same name.
				_ = json.NewEncoder(w).Encode(map[string]string{"name": "clash", "category": "shell", "content": "clash, the trusted sheet", "source_trust": "canonical"})
			})
			s := httptest.NewServer(mux)
			defer s.Close()

			refs, contents, err := newRetriever(s.Client(), s.URL, "", "", 5*time.Second).Retrieve(context.Background(), "clash", 5)
			t.Logf("refs: %s err=%v", names(refs), err)
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(asked) != 1 || asked[0] != "category=notes" {
				t.Errorf("the topic was asked for with %q, want one read with category=notes", asked)
			}
			if !gateRefuses(refs) {
				t.Error("search said external, the chain came back clean")
			}
			for _, c := range contents {
				if c.Ref.SourceTrust == "canonical" {
					t.Errorf("the trusted sheet of another category was read for the external hit: %+v", c.Ref)
				}
			}
			if !tc.honour && len(contents) != 0 {
				t.Errorf("a vor that ignored the category still gave %d contents", len(contents))
			}
		})
	}
}
