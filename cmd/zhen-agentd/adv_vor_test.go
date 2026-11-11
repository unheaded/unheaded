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
	"testing"
	"time"

	"unheaded/pkg/champion"
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
