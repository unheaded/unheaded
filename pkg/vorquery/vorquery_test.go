// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package vorquery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"sort"
	"strings"
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

// TestMatchesPython runs raft/scripts/zhen_rag.py's own query helpers
// (extracted with ast, no imports beyond re) over the same inputs, so the
// live Python chat path and the Go tools shape a query and choose a source
// alike.
func TestMatchesPython(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	const script = `
import ast, json, re, sys
tree = ast.parse(open(sys.argv[1], encoding="utf-8").read())
want = {"_QUERY_STOPWORDS", "_TERM_PUNCT", "_NAME_TOKEN_RE", "_search_terms", "_name_tokens",
        "_WORKTREE_CATEGORY_PREFIX", "_ANCHOR_TOKEN_RE", "_MIN_FALLBACK_WORDS",
        "_MAX_FALLBACK_WORDS", "_MIN_FALLBACK_COVER", "_MAX_FETCHES", "_is_word_char",
        "_words", "_starts_word", "_covers", "_candidates", "_anchored"}
ns = {"re": re}
for node in tree.body:
    names = {getattr(t, "id", "") for t in getattr(node, "targets", [])} | {getattr(node, "name", "")}
    if names & want:
        exec(compile(ast.Module([node], []), "zhen_rag.py", "exec"), ns)
inputs = json.loads(sys.argv[2])
sel = json.loads(sys.argv[3])
pairs = lambda ts: [(t["name"], t["category"]) for t in ts or []]
count, names = ns["_anchored"](pairs(sel["topics"]), sel["words"])
print(json.dumps({
    "consts": [ns["_MIN_FALLBACK_WORDS"], ns["_MAX_FALLBACK_WORDS"], ns["_MIN_FALLBACK_COVER"], ns["_MAX_FETCHES"]],
    "words": [ns["_words"](ns["_search_terms"](q)) for q in inputs],
    "starts": [ns["_starts_word"](t, w) for t, w in sel["starts"]],
    "covers": [ns["_covers"](t, sel["words"]) for t in sel["texts"]],
    "candidates": ns["_candidates"]([pairs(h) for h in sel["per_word"]]),
    "anchored_count": count,
    "anchored_names": names,
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
		"explain the steps towards Junos, Associate (JNCIA-Junos)",
		"tell me about getting the ccna", "über-größe naïve café", "ipv6 2001:db8::1 x_y",
	}
	type sel struct {
		Topics  []Topic     `json:"topics"`
		Words   []string    `json:"words"`
		Starts  [][2]string `json:"starts"`
		Texts   []string    `json:"texts"`
		PerWord [][]Topic   `json:"per_word"`
	}
	s := sel{
		Topics: []Topic{
			{Name: "junos-interfaces", Category: "juniper"},
			{Name: "ospf", Category: "junos/routing"},
			{Name: "kerberos", Category: "security"},
			{Name: "JNCIA-Junos", Category: ".claude/worktrees/a/juniper"},
			{Name: "jncia_junos.notes", Category: "certs"},
			{Name: "", Category: "junos"},
			{Name: "junos-interfaces", Category: "jncia/junos"},
		},
		Words: []string{"steps", "junos", "associate", "jncia"},
		Starts: [][2]string{
			{"export krb5ccname=file", "ccna"}, {"ccna study", "ccna"}, {"the ccna", "ccna"},
			{"kubernetes", "kube"}, {"x-ccna", "ccna"}, {"accna ccna", "ccna"}, {"abc", ""},
			{"", "a"}, {"5ccna", "ccna"}, {"éccna", "ccna"}, {"_ccna", "ccna"},
		},
		Texts: []string{
			"Junos Associate steps", "the JNCIA-Junos exam", "KRB5CCNAME", "",
			"prejunos associated stepstool",
		},
		PerWord: [][]Topic{
			{{Name: "a", Category: "x"}, {Name: "b", Category: "x"}, {Name: "a", Category: "y"}},
			{{Name: "c", Category: "x"}, {Name: "b", Category: "x"}, {Name: "w", Category: ".claude/worktrees/z"}},
			{{Name: "", Category: "x"}, {Name: "b", Category: "x"}, {Name: "c", Category: "x"}, {Name: "d", Category: ""}},
			nil,
		},
	}
	in, _ := json.Marshal(inputs)
	selIn, _ := json.Marshal(s)
	out, err := exec.Command("python3", "-I", "-c", script, "../../raft/scripts/zhen_rag.py", string(in), string(selIn)).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("python3: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("python3: %v", err)
	}
	var py struct {
		Stop   []string   `json:"stop"`
		Punct  string     `json:"punct"`
		Terms  [][]string `json:"terms"`
		Tokens [][]string `json:"tokens"`

		Consts        []int          `json:"consts"`
		Words         [][]string     `json:"words"`
		Starts        []bool         `json:"starts"`
		Covers        []int          `json:"covers"`
		Candidates    []string       `json:"candidates"`
		AnchoredCount map[string]int `json:"anchored_count"`
		AnchoredNames []string       `json:"anchored_names"`
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
		if g, p := norm(Words(SearchTerms(q))), norm(py.Words[i]); !reflect.DeepEqual(g, p) {
			t.Errorf("Words(%q): go %q, py %q", q, g, p)
		}
	}

	goConsts := []int{MinFallbackWords, MaxFallbackWords, MinFallbackCover, MaxFetches}
	if !reflect.DeepEqual(goConsts, py.Consts) {
		t.Errorf("limits differ: go %v, py %v", goConsts, py.Consts)
	}
	if len(py.Starts) != len(s.Starts) || len(py.Covers) != len(s.Texts) {
		t.Fatalf("python answered %d starts, %d covers", len(py.Starts), len(py.Covers))
	}
	for i, c := range s.Starts {
		if g := StartsWord(c[0], c[1]); g != py.Starts[i] {
			t.Errorf("StartsWord(%q, %q): go %v, py %v", c[0], c[1], g, py.Starts[i])
		}
	}
	for i, text := range s.Texts {
		if g := Covers(text, s.Words); g != py.Covers[i] {
			t.Errorf("Covers(%q): go %d, py %d", text, g, py.Covers[i])
		}
	}
	if g, p := norm(Candidates(s.PerWord)), norm(py.Candidates); !reflect.DeepEqual(g, p) {
		t.Errorf("Candidates: go %q, py %q", g, p)
	}
	count, names := Anchored(s.Topics, s.Words)
	if !reflect.DeepEqual(norm(names), norm(py.AnchoredNames)) {
		t.Errorf("Anchored names: go %q, py %q", names, py.AnchoredNames)
	}
	if !reflect.DeepEqual(count, py.AnchoredCount) {
		t.Errorf("Anchored counts: go %v, py %v", count, py.AnchoredCount)
	}
	if len(names) == 0 || len(py.Candidates) == 0 {
		t.Error("the comparison ran on nothing")
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

func TestWords(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in   []string
		want []string
	}{
		{[]string{"junos", "jncia-junos"}, []string{"junos", "jncia"}},
		{[]string{"huginn`'s", "config"}, []string{"huginn", "s", "config"}},
		{[]string{"KRB5CCNAME"}, []string{"krb5ccname"}},
		{[]string{"a/b_c.d"}, []string{"a", "b", "c", "d"}},
		{nil, nil},
	} {
		if got := Words(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Words(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStartsWordAndCovers(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		text, word string
		want       bool
	}{
		{"export krb5ccname=/tmp/x", "ccna", false},
		{"the ccna exam", "ccna", true},
		{"ccna", "ccna", true},
		{"(ccna)", "ccna", true},
		{"kubernetes pods", "kube", true},
		{"xccna then ccna-200", "ccna", true},
		{"anything", "", false},
		{"épreuve ccna", "ccna", true},
		{"éccna", "ccna", false},
	} {
		if got := StartsWord(c.text, c.word); got != c.want {
			t.Errorf("StartsWord(%q, %q) = %v, want %v", c.text, c.word, got, c.want)
		}
	}
	if got := Covers("Getting a ticket: KRB5CCNAME=/tmp/x", []string{"getting", "ccna", "ticket"}); got != 2 {
		t.Errorf("Covers = %d, want 2", got)
	}
}

func TestCandidates(t *testing.T) {
	t.Parallel()
	got := Candidates([][]Topic{
		{{Name: "a", Category: "c"}, {Name: "b", Category: "c"}, {Name: "a", Category: "c"}},
		{{Name: "b", Category: "c"}, {Name: "w", Category: ".claude/worktrees/x"}, {Name: "", Category: "c"}},
		{{Name: "c", Category: "c"}, {Name: "b", Category: "c"}},
	})
	if want := []string{"b", "a", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Candidates = %q, want %q", got, want)
	}
}

func TestAnchored(t *testing.T) {
	t.Parallel()
	count, names := Anchored([]Topic{
		{Name: "junos-interfaces", Category: "network/juniper"},
		{Name: "kerberos", Category: "security"},
		{Name: "bgp", Category: "network/junos"},
		{Name: "jncia-junos-path", Category: "certs"},
		{Name: "junos-interfaces", Category: ".claude/worktrees/x"},
		{Name: "junosphere", Category: "misc"},
	}, []string{"junos", "jncia", "steps"})
	if want := []string{"junos-interfaces", "bgp", "jncia-junos-path"}; !reflect.DeepEqual(names, want) {
		t.Errorf("Anchored names = %q, want %q", names, want)
	}
	if want := map[string]int{"junos-interfaces": 1, "bgp": 1, "jncia-junos-path": 2}; !reflect.DeepEqual(count, want) {
		t.Errorf("Anchored count = %v, want %v", count, want)
	}
}

// fakeVor answers like vor: a search is an AND of case-insensitive substring
// matches over a topic's name and content, in corpus order.
type fakeVor struct {
	corpus   []fakeTopic
	searches []string
	fetches  map[string]int
	failWord string
}

type fakeTopic struct{ name, category, content string }

func (f *fakeVor) index(t *testing.T) *Index {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var out []Topic
		for _, tp := range f.corpus {
			out = append(out, Topic{Name: tp.name, Category: tp.category})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return NewIndex(srv.Client(), srv.URL)
}

func (f *fakeVor) search(_ context.Context, q string) ([]Topic, error) {
	f.searches = append(f.searches, q)
	if q == f.failWord {
		return nil, context.DeadlineExceeded
	}
	var out []Topic
	for _, tp := range f.corpus {
		text := strings.ToLower(tp.name + " " + tp.content)
		all := true
		for _, term := range strings.Fields(q) {
			if !strings.Contains(text, term) {
				all = false
				break
			}
		}
		if all {
			out = append(out, Topic{Name: tp.name, Category: tp.category})
		}
	}
	return out, nil
}

func (f *fakeVor) fetch(_ context.Context, name string) (string, bool) {
	if f.fetches == nil {
		f.fetches = make(map[string]int)
	}
	f.fetches[name]++
	for _, tp := range f.corpus {
		if tp.name == name {
			return Text(tp.name, tp.category, "", tp.content), true
		}
	}
	return "", false
}

func sheets() []fakeTopic {
	return []fakeTopic{
		{"kerberos", "security", "Getting a ticket. export KRB5CCNAME=/tmp/krb5cc"},
		{"career-notes", "misc", "the steps towards an associate role"},
		{"junos-interfaces", "network/juniper", "Junos interfaces. On the JNCIA blueprint."},
		{"junos-monitoring", "network/juniper", "Junos monitoring, a JNCIA topic."},
		{"junos-cli", "network/juniper", "set and commit"},
		{"Service-Wotan", "wiki", "Wotan is the message bus."},
		{"wotan-cloud-battle-plan", "battle-plans", "Wotan cloud plan."},
		{"what-is-unheaded", "eli5", "Unheaded, in plain words."},
	}
}

func TestSelect_SubstringIsNotAWord(t *testing.T) {
	t.Parallel()
	f := &fakeVor{corpus: sheets()}
	names, warn, err := f.index(t).Select(context.Background(), SearchTerms("tell me about getting the ccna"), 5, f.search, f.fetch)
	if err != nil || warn != nil {
		t.Fatalf("Select: warn=%v err=%v", warn, err)
	}
	if len(names) != 0 {
		t.Errorf("a CCNA question was answered from %q; vor has no CCNA sheet, so it has no source", names)
	}
	if f.fetches["kerberos"] != 1 {
		t.Errorf("kerberos fetched %d times, want 1 (read once, then refused)", f.fetches["kerberos"])
	}
}

func TestSelect_LongQuestionFallsBackToNamedTopics(t *testing.T) {
	t.Parallel()
	f := &fakeVor{corpus: sheets()}
	terms := SearchTerms("explain the steps towards Junos, Associate (JNCIA-Junos)")
	names, warn, err := f.index(t).Select(context.Background(), terms, 5, f.search, f.fetch)
	if err != nil || warn != nil {
		t.Fatalf("Select: warn=%v err=%v", warn, err)
	}
	if want := []string{"junos-interfaces", "junos-monitoring"}; !reflect.DeepEqual(names, want) {
		t.Errorf("Select = %q, want %q (career-notes carries three words and no query word names it; junos-cli carries one)", names, want)
	}
	if want := "steps towards junos associate jncia-junos"; len(f.searches) == 0 || f.searches[0] != want {
		t.Errorf("first search = %q, want %q", f.searches, want)
	}
	for name, n := range f.fetches {
		if n != 1 {
			t.Errorf("%s fetched %d times, want 1", name, n)
		}
	}
}

func TestSelect_FullQueryStillWins(t *testing.T) {
	t.Parallel()
	f := &fakeVor{corpus: sheets()}
	names, _, err := f.index(t).Select(context.Background(), SearchTerms("what is wotan?"), 5, f.search, f.fetch)
	if want := []string{"Service-Wotan", "wotan-cloud-battle-plan"}; err != nil || !reflect.DeepEqual(names, want) {
		t.Errorf("Select = %q, %v; want %q", names, err, want)
	}
	if len(f.searches) != 1 {
		t.Errorf("searches = %q, want one (a single word has no second pass)", f.searches)
	}

	names, _, _ = f.index(t).Select(context.Background(), SearchTerms("what is wotan?"), 1, f.search, f.fetch)
	if want := []string{"Service-Wotan"}; !reflect.DeepEqual(names, want) {
		t.Errorf("Select k=1 = %q, want %q", names, want)
	}
}

func TestSelect_FetchCap(t *testing.T) {
	t.Parallel()
	f := &fakeVor{}
	for i := 0; i < 30; i++ {
		f.corpus = append(f.corpus, fakeTopic{"doc" + string(rune('a'+i%26)) + string(rune('a'+i/26)), "c", "export KRB5CCNAME"})
	}
	names, _, err := f.index(t).Select(context.Background(), []string{"ccna"}, 5, f.search, f.fetch)
	if err != nil || len(names) != 0 {
		t.Fatalf("Select = %q, %v", names, err)
	}
	if len(f.fetches) != MaxFetches {
		t.Errorf("fetched %d topics, want the cap %d", len(f.fetches), MaxFetches)
	}
}

func TestSelect_Failures(t *testing.T) {
	t.Parallel()
	terms := SearchTerms("explain the steps towards Junos, Associate (JNCIA-Junos)")

	// The first search fails: that is the error.
	f := &fakeVor{corpus: sheets(), failWord: strings.Join(terms, " ")}
	if names, _, err := f.index(t).Select(context.Background(), terms, 5, f.search, f.fetch); err == nil || names != nil {
		t.Errorf("failed first search: Select = %q, %v; want an error", names, err)
	}

	// One word's search fails: the word is skipped, the rest still answer.
	f = &fakeVor{corpus: sheets(), failWord: "steps"}
	names, warn, err := f.index(t).Select(context.Background(), terms, 5, f.search, f.fetch)
	if want := []string{"junos-interfaces", "junos-monitoring"}; err != nil || warn != nil || !reflect.DeepEqual(names, want) {
		t.Errorf("failed word search: Select = %q, warn=%v err=%v; want %q", names, warn, err, want)
	}

	// No topic listing: nothing can be named, so the second pass keeps
	// nothing and says why.
	f = &fakeVor{corpus: sheets()}
	dead := NewIndex(http.DefaultClient, "http://127.0.0.1:1")
	names, warn, err = dead.Select(context.Background(), terms, 5, f.search, f.fetch)
	if err != nil || warn == nil || len(names) != 0 {
		t.Errorf("dead listing: Select = %q, warn=%v err=%v; want no names and a warning", names, warn, err)
	}
}
