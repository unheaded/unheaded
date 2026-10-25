// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package vorquery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
)

func TestSearchTerms(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"what is wotan?":                    {"wotan"},
		"What IS Wotan":                     {"wotan"},
		"how do I trim a string in go":      {"trim", "string", "go"},
		"what is this":                      {"what", "is", "this"},
		"if not x, else (y)":                {"if", "not", "x", "else", "y"},
		"  ":                                nil,
		"explain `huginn`'s config!":        {"huginn`'s", "config"},
		"tell me about sophia dictionaries": {"sophia", "dictionaries"},
	}
	for in, want := range cases {
		if got := SearchTerms(in); !reflect.DeepEqual(got, want) {
			t.Errorf("SearchTerms(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNameTokens(t *testing.T) {
	t.Parallel()
	for in, want := range map[string][]string{
		"Service-Wotan":                  {"service", "wotan"},
		"WOTAN_TOPICS":                   {"wotan", "topics"},
		"draft-unheaded-wotan-memory-03": {"draft", "unheaded", "wotan", "memory", "03"},
		"a..b  c":                        {"a", "b", "c"},
		"":                               nil,
	} {
		if got := NameTokens(in); !reflect.DeepEqual(got, want) {
			t.Errorf("NameTokens(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNamedBy(t *testing.T) {
	t.Parallel()
	topics := []Topic{
		{Name: "ADR-023-wotan-virtual-memory", Category: "adr"},
		{Name: "Service-Wotan", Category: "wiki"},
		{Name: "Wotan-Memory-Model", Category: "wiki"},
		{Name: "Service-Wotan", Category: ".claude/worktrees/agent-x/wiki"},
		{Name: "wotan-cloud-battle-plan", Category: "battle-plans"},
		{Name: "what-is-unheaded", Category: "eli5"},
		{Name: "", Category: "wiki"},
	}
	got := NamedBy(topics, []string{"wotan"})
	want := []string{"Service-Wotan", "Wotan-Memory-Model", "wotan-cloud-battle-plan", "ADR-023-wotan-virtual-memory"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NamedBy(wotan) = %q, want %q", got, want)
	}
	if got := NamedBy(topics, []string{"wotan", "memory"}); !reflect.DeepEqual(got, []string{"Wotan-Memory-Model", "ADR-023-wotan-virtual-memory"}) {
		t.Errorf("NamedBy(wotan memory) = %q", got)
	}
	if got := NamedBy(topics, nil); got != nil {
		t.Errorf("NamedBy(nil) = %q, want nil", got)
	}
	if got := NamedBy(topics, []string{"nomatch"}); len(got) != 0 {
		t.Errorf("NamedBy(nomatch) = %q, want empty", got)
	}
}

func TestInterleave(t *testing.T) {
	t.Parallel()
	got := Interleave([]string{"n1", "n2", "n3"}, []string{"s1"})
	if want := []string{"n1", "s1", "n2", "n3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Interleave = %q, want %q", got, want)
	}
	if got := Interleave(nil, []string{"s1", "s2"}); !reflect.DeepEqual(got, []string{"s1", "s2"}) {
		t.Errorf("Interleave(nil, s) = %q", got)
	}
}

func TestIndexCachesAndFailsSoft(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/topics" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]Topic{{Name: "Service-Wotan", Category: "wiki"}})
	}))
	defer srv.Close()

	x := NewIndex(srv.Client(), srv.URL+"/")
	for i := 0; i < 3; i++ {
		got, err := x.Named(context.Background(), []string{"wotan"})
		if err != nil || !reflect.DeepEqual(got, []string{"Service-Wotan"}) {
			t.Fatalf("Named = %q, %v", got, err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("listing fetched %d times, want 1 (cached)", n)
	}

	down := NewIndex(srv.Client(), "http://127.0.0.1:1")
	if got, err := down.Named(context.Background(), []string{"wotan"}); err == nil || got != nil {
		t.Errorf("Named on dead vor = %q, %v; want nil, error", got, err)
	}
}

// TestMatchesPython runs raft/scripts/zhen_rag.py's own _search_terms and
// _name_tokens (extracted with ast, no imports beyond re) over the same
// inputs, so the live Python chat path and the Go tools shape a query alike.
func TestMatchesPython(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	const script = `
import ast, json, re, sys
tree = ast.parse(open(sys.argv[1], encoding="utf-8").read())
want = {"_QUERY_STOPWORDS", "_TERM_PUNCT", "_NAME_TOKEN_RE", "_search_terms", "_name_tokens"}
ns = {"re": re}
for node in tree.body:
    names = {getattr(t, "id", "") for t in getattr(node, "targets", [])} | {getattr(node, "name", "")}
    if names & want:
        exec(compile(ast.Module([node], []), "zhen_rag.py", "exec"), ns)
inputs = json.loads(sys.argv[2])
print(json.dumps({
    "stop": sorted(ns["_QUERY_STOPWORDS"]),
    "punct": ns["_TERM_PUNCT"],
    "terms": [ns["_search_terms"](q) for q in inputs],
    "tokens": [ns["_name_tokens"](q) for q in inputs],
}))
`
	inputs := []string{
		"what is wotan?", "How do I trim a string in Go", "what is this",
		"Service-Wotan", "draft-unheaded-wotan-memory-03", "WOTAN_TOPICS",
		"explain `huginn`'s config!", "if not x, else (y)", "a..b  c", "",
	}
	in, _ := json.Marshal(inputs)
	out, err := exec.Command("python3", "-I", "-c", script, "../../raft/scripts/zhen_rag.py", string(in)).Output()
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	var py struct {
		Stop   []string   `json:"stop"`
		Punct  string     `json:"punct"`
		Terms  [][]string `json:"terms"`
		Tokens [][]string `json:"tokens"`
	}
	if err := json.Unmarshal(out, &py); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}

	var goStop []string
	for w := range stopWords {
		goStop = append(goStop, w)
	}
	sort.Strings(goStop)
	if !reflect.DeepEqual(goStop, py.Stop) {
		t.Errorf("stop words differ:\n go: %q\n py: %q", goStop, py.Stop)
	}
	if py.Punct != termPunct {
		t.Errorf("term punctuation differs: go %q, py %q", termPunct, py.Punct)
	}
	norm := func(s []string) []string {
		if len(s) == 0 {
			return nil
		}
		return s
	}
	for i, q := range inputs {
		if g, p := norm(SearchTerms(q)), norm(py.Terms[i]); !reflect.DeepEqual(g, p) {
			t.Errorf("SearchTerms(%q): go %q, py %q", q, g, p)
		}
		if g, p := norm(NameTokens(q)), norm(py.Tokens[i]); !reflect.DeepEqual(g, p) {
			t.Errorf("NameTokens(%q): go %q, py %q", q, g, p)
		}
	}
}

func TestOrderDropsWorktreeHits(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Topic{
			{Name: "Service-Wotan", Category: "wiki"},
			{Name: "Service-Wotan", Category: ".claude/worktrees/agent-x/wiki"},
		})
	}))
	defer srv.Close()
	hits := []Topic{
		{Name: "Wotan-Topic-Signing", Category: ".claude/worktrees/agent-x/wiki"},
		{Name: "wotan-cloud-battle-plan", Category: "docs/battle-plans/tools"},
	}
	got, err := NewIndex(srv.Client(), srv.URL).Order(context.Background(), []string{"wotan"}, hits)
	if want := []string{"Service-Wotan", "wotan-cloud-battle-plan"}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Order = %q, %v; want %q", got, err, want)
	}

	got, err = NewIndex(srv.Client(), "http://127.0.0.1:1").Order(context.Background(), []string{"wotan"}, hits)
	if want := []string{"wotan-cloud-battle-plan"}; err == nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Order with dead listing = %q, %v; want %q and an error", got, err, want)
	}
}
