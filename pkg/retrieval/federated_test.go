// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package retrieval

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
	"unheaded/pkg/codeindex"
)

// longToken makes a poison name outrank the temp dir's path segments in the
// agent's longest-first token order, so the per-turn check looks it up.
const longToken = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

// fake answers the same way for every query.
type fake struct {
	name     string
	refs     []champion.Reference
	contents []agent.TopicContent
	err      error
	run      func(context.Context) ([]champion.Reference, []agent.TopicContent, error)
}

func (f *fake) Name() string { return f.name }

func (f *fake) Retrieve(ctx context.Context, _ string, _ int) ([]champion.Reference, []agent.TopicContent, error) {
	if f.run != nil {
		return f.run(ctx)
	}
	return f.refs, f.contents, f.err
}

// queryFake answers by query.
type queryFake struct {
	name string
	run  func(context.Context, string) ([]champion.Reference, []agent.TopicContent, error)
}

func (f *queryFake) Name() string { return f.name }

func (f *queryFake) Retrieve(ctx context.Context, q string, _ int) ([]champion.Reference, []agent.TopicContent, error) {
	return f.run(ctx, q)
}

type hit struct {
	topic, category, trust string
	score                  float64
}

// answering is a fake whose every hit carries text.
func answering(name string, hits ...hit) *fake {
	f := &fake{name: name}
	for _, h := range hits {
		r := champion.Reference{Topic: h.topic, Category: h.category, SourceTrust: h.trust, SourceLabel: name}
		f.refs = append(f.refs, r)
		f.contents = append(f.contents, agent.TopicContent{Ref: r, Content: "text of " + h.topic, Score: h.score})
	}
	return f
}

// script is an LLM with canned turns.
type script struct {
	out []string
	n   int
}

func (s *script) Complete(context.Context, []agent.Message, int, float64, int) (string, error) {
	if s.n >= len(s.out) {
		return "", errors.New("script: out of turns")
	}
	s.n++
	return s.out[s.n-1], nil
}

type nopActions struct{}

func (nopActions) LogAction(context.Context, *champion.Action) (int64, error) { return 1, nil }
func (nopActions) UpdateAction(context.Context, int64, string, string, string, int) error {
	return nil
}
func (nopActions) GetActions(context.Context, string, int) ([]champion.Action, error) {
	return nil, nil
}

// chunk builds a chunk from an ID of the form "path::Symbol".
func chunk(id, content string) codeindex.Chunk {
	path, symbol, _ := strings.Cut(id, "::")
	return codeindex.Chunk{ID: id, Path: path, Language: "go", Kind: "func", Symbol: symbol, Content: content}
}

func saveIndex(t *testing.T, chunks ...codeindex.Chunk) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "index")
	if err := codeindex.Save(dir, &codeindex.Index{Chunks: chunks}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func topics(refs []champion.Reference) string {
	s := make([]string, len(refs))
	for i, r := range refs {
		s[i] = r.Topic
	}
	return strings.Join(s, " ")
}

func texts(contents []agent.TopicContent) string {
	s := make([]string, len(contents))
	for i, c := range contents {
		s[i] = c.Ref.Topic
	}
	return strings.Join(s, " ")
}

func wantTopics(t *testing.T, refs []champion.Reference, want string) {
	t.Helper()
	if got := topics(refs); got != want {
		t.Fatalf("refs %q, want %q", got, want)
	}
}

func wantTexts(t *testing.T, contents []agent.TopicContent, want string) {
	t.Helper()
	if got := texts(contents); got != want {
		t.Fatalf("texts %q, want %q", got, want)
	}
}

// outcomes records what New's callback saw.
type outcomes struct {
	mu  sync.Mutex
	got map[string]int
}

func (o *outcomes) record(backend, outcome string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.got == nil {
		o.got = map[string]int{}
	}
	o.got[backend+"="+outcome]++
}

func (o *outcomes) count(key string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.got[key]
}

// blocked waits for its context, then gives up.
func blocked(name string) *fake {
	return &fake{name: name, run: func(ctx context.Context) ([]champion.Reference, []agent.TopicContent, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}}
}

// wedged ignores its context until the test ends.
func wedged(t *testing.T, name string) *fake {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	return &fake{name: name, run: func(context.Context) ([]champion.Reference, []agent.TopicContent, error) {
		<-release
		return nil, nil, nil
	}}
}

func TestFederated_OneBackendPassesThrough(t *testing.T) {
	b := answering("vor", hit{"low", "c", "external", 0.1}, hit{"high", "c", "canonical", 0.9}, hit{"mid", "c", "local", 0.5})
	refs, contents, err := New([]Backend{b}, time.Second, nil).Retrieve(context.Background(), "q", 1)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "low high mid")
	wantTexts(t, contents, "low high mid")

	boom := errors.New("boom")
	if _, _, err := New([]Backend{&fake{name: "vor", err: boom}}, time.Second, nil).Retrieve(context.Background(), "q", 5); err != boom {
		t.Fatalf("err %v, want its own", err)
	}
}

func TestFederated_NoBackendsIsAnError(t *testing.T) {
	if _, _, err := New(nil, time.Second, nil).Retrieve(context.Background(), "q", 5); err == nil {
		t.Fatal("no backends answered nil")
	}
}

func TestFederated_MergeOrderMixedTrust(t *testing.T) {
	a := answering("a", hit{"ext-hi", "c", "external", 9}, hit{"loc-lo", "c", "local", 1})
	b := answering("b", hit{"can", "c", "canonical", 0.1}, hit{"loc-hi", "c", "local", 2}, hit{"user", "c", "direct-user", 3})
	refs, contents, err := New([]Backend{a, b}, time.Second, nil).Retrieve(context.Background(), "q", 10)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "can loc-hi loc-lo ext-hi user")
	wantTexts(t, contents, "can loc-hi loc-lo ext-hi user")
	if contents[0].Score != 0.1 || contents[0].Content != "text of can" {
		t.Fatalf("score or text lost: %+v", contents[0])
	}
}

func TestFederated_TiesKeepBackendOrder(t *testing.T) {
	a := answering("a", hit{"a1", "c", "local", 1}, hit{"a2", "c", "local", 1})
	b := answering("b", hit{"b1", "c", "local", 1})
	refs, _, err := New([]Backend{b, a}, time.Second, nil).Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "b1 a1 a2")
}

func TestFederated_Deadline_SlowBackendSkipped(t *testing.T) {
	for name, slow := range map[string]func(*testing.T) *fake{
		"blocking": func(*testing.T) *fake { return blocked("slow") },
		"ignoring": func(t *testing.T) *fake { return wedged(t, "slow") },
	} {
		t.Run(name, func(t *testing.T) {
			var o outcomes
			good := answering("good", hit{"g", "c", "local", 1})
			t0 := time.Now()
			refs, contents, err := New([]Backend{slow(t), good}, 50*time.Millisecond, o.record).Retrieve(context.Background(), "q", 5)
			if d := time.Since(t0); d > 200*time.Millisecond {
				t.Fatalf("held %v", d)
			}
			if err != nil {
				t.Fatal(err)
			}
			wantTopics(t, refs, "g retrieval-unavailable/slow")
			wantTexts(t, contents, "g")
			if o.count("slow=timeout") != 1 || o.count("good=ok") != 1 {
				t.Fatalf("outcomes %v", o.got)
			}
		})
	}
}

func TestFederated_Deadline_AnswerAtTheDeadlineIsDropped(t *testing.T) {
	part := champion.Reference{Topic: "partial", Category: "c", SourceTrust: "canonical"}
	for name, late := range map[string]struct {
		refs     []champion.Reference
		contents []agent.TopicContent
	}{
		"partial": {[]champion.Reference{part}, []agent.TopicContent{{Ref: part, Content: "half", Score: 1}}},
		"empty":   {},
	} {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				var o outcomes
				vor := &fake{name: "vor", run: func(ctx context.Context) ([]champion.Reference, []agent.TopicContent, error) {
					<-ctx.Done()
					return late.refs, late.contents, nil
				}}
				code := answering("code", hit{"c1", "code", "local", 3}, hit{"c2", "code", "local", 2}, hit{"c3", "code", "local", 1})
				refs, contents, err := New([]Backend{vor, code}, 30*time.Millisecond, o.record).Retrieve(context.Background(), "q", 5)
				if err != nil {
					t.Fatal(err)
				}
				wantTopics(t, refs, "c1 c2 c3 retrieval-unavailable/vor")
				wantTexts(t, contents, "c1 c2 c3")
				if o.count("vor=timeout") != 1 {
					t.Fatalf("outcomes %v", o.got)
				}
			}
		})
	}
}

func TestFederated_Deadline_AloneIsAnError(t *testing.T) {
	var o outcomes
	part := champion.Reference{Topic: "partial", Category: "c", SourceTrust: "canonical"}
	vor := &fake{name: "vor", run: func(ctx context.Context) ([]champion.Reference, []agent.TopicContent, error) {
		<-ctx.Done()
		return []champion.Reference{part}, nil, nil
	}}
	refs, _, err := New([]Backend{vor}, 5*time.Millisecond, o.record).Retrieve(context.Background(), "q", 5)
	if !errors.Is(err, context.DeadlineExceeded) || refs != nil {
		t.Fatalf("refs %v err %v", refs, err)
	}
	if o.count("vor=timeout") != 1 {
		t.Fatalf("outcomes %v", o.got)
	}
}

func TestFederated_Deadline_TwoSilentBackends(t *testing.T) {
	var o outcomes
	bs := []Backend{
		answering("one", hit{"g1", "c", "local", 1}),
		wedged(t, "wedged"),
		answering("two", hit{"g2", "c", "local", 2}),
		blocked("slow"),
	}
	refs, _, err := New(bs, 50*time.Millisecond, o.record).Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "g2 g1 retrieval-unavailable/wedged retrieval-unavailable/slow")
	if o.count("wedged=timeout") != 1 || o.count("slow=timeout") != 1 {
		t.Fatalf("outcomes %v", o.got)
	}
}

func TestFederated_UnavailableRefIsWhatTheGateNeeds(t *testing.T) {
	bs := []Backend{&fake{name: "vor", err: errors.New("down")}, answering("code", hit{"c1", "code", "local", 1})}
	refs, _, err := New(bs, time.Second, nil).Retrieve(context.Background(), "q", 1)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "c1 retrieval-unavailable/vor")
	if got, want := refs[1], champion.Unavailable("vor"); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if want := (champion.Reference{Topic: "retrieval-unavailable/vor", Category: "retrieval", SourceKind: "unavailable", SourceTrust: "unavailable", SourceLabel: "vor"}); refs[1] != want {
		t.Fatalf("got %+v, want %+v", refs[1], want)
	}
	if !(&champion.ToolCall{Name: "write_file", Justification: refs}).HasUntrustedJustification() {
		t.Fatal("the gate would pass")
	}
}

func TestFederated_OneErrorOneSuccess(t *testing.T) {
	var o outcomes
	bs := []Backend{&fake{name: "bad", err: errors.New("boom")}, answering("good", hit{"g", "c", "local", 1})}
	refs, contents, err := New(bs, time.Second, o.record).Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "g retrieval-unavailable/bad")
	wantTexts(t, contents, "g")
	if o.count("bad=error") != 1 || o.count("good=ok") != 1 {
		t.Fatalf("outcomes %v", o.got)
	}
}

func TestFederated_AllErrorsGiveTheFirst(t *testing.T) {
	first, second := errors.New("first"), errors.New("second")
	bs := []Backend{&fake{name: "a", err: first}, &fake{name: "b", err: second}}
	refs, contents, err := New(bs, time.Second, nil).Retrieve(context.Background(), "q", 5)
	if err != first || refs != nil || contents != nil {
		t.Fatalf("refs %v contents %v err %v", refs, contents, err)
	}
}

func TestFederated_PanicIsAnError(t *testing.T) {
	var o outcomes
	bad := &fake{name: "bad", run: func(context.Context) ([]champion.Reference, []agent.TopicContent, error) { panic("boom") }}
	refs, _, err := New([]Backend{bad, answering("good", hit{"g", "c", "local", 1})}, time.Second, o.record).Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "g retrieval-unavailable/bad")
	if o.count("bad=error") != 1 {
		t.Fatalf("outcomes %v", o.got)
	}
	if _, _, err := New([]Backend{bad}, time.Second, nil).Retrieve(context.Background(), "q", 5); err == nil {
		t.Fatal("a panic alone answered nil")
	}
}

func TestFederated_CancelledParentReturnsPromptly(t *testing.T) {
	var o outcomes
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)
	t0 := time.Now()
	_, _, err := New([]Backend{blocked("a"), wedged(t, "b")}, time.Minute, o.record).Retrieve(ctx, "q", 5)
	if d := time.Since(t0); d > 500*time.Millisecond {
		t.Fatalf("held %v", d)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if o.count("a=timeout")+o.count("b=timeout") != 0 {
		t.Fatalf("a cancelled caller is not a slow backend: %v", o.got)
	}
}

func TestFederated_ParallelCalls(t *testing.T) {
	var o outcomes
	f := New([]Backend{
		answering("a", hit{"a1", "c", "local", 1}, hit{"e1", "c", "external", 1}),
		answering("b", hit{"b1", "c", "canonical", 1}),
		blocked("slow"),
	}, 100*time.Millisecond, o.record)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			refs, _, err := f.Retrieve(context.Background(), "q", 2)
			if err != nil {
				t.Error(err)
				return
			}
			if got := topics(refs); got != "b1 a1 e1 retrieval-unavailable/slow" {
				t.Errorf("refs %q", got)
			}
		}()
	}
	wg.Wait()
	if o.count("slow=timeout") != 50 {
		t.Fatalf("outcomes %v", o.got)
	}
}

func TestFederated_KNotPositive(t *testing.T) {
	for _, k := range []int{0, -3} {
		a := answering("a", hit{"a1", "c", "local", 1})
		b := answering("b", hit{"e1", "c", "external", 1})
		refs, contents, err := New([]Backend{a, b}, time.Second, nil).Retrieve(context.Background(), "q", k)
		if err != nil {
			t.Fatal(err)
		}
		wantTopics(t, refs, "e1")
		wantTexts(t, contents, "")
	}
}

func TestFederated_TextWithNoReferenceIsDropped(t *testing.T) {
	a := answering("a", hit{"a1", "c", "local", 1})
	stray := champion.Reference{Topic: "stray", Category: "c", SourceTrust: "canonical"}
	a.contents = append(a.contents, agent.TopicContent{Ref: stray, Content: "unreferenced", Score: 9})
	b := answering("b", hit{"b1", "c", "local", 1})
	refs, contents, err := New([]Backend{a, b}, time.Second, nil).Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "a1 b1")
	wantTexts(t, contents, "a1 b1")
}

func TestFederated_UntrustedRefsSurvivePastK(t *testing.T) {
	a := answering("a", hit{"a1", "c", "local", 3}, hit{"a2", "c", "local", 2}, hit{"a3", "c", "local", 1})
	e := answering("e", hit{"e1", "c", "external", 3}, hit{"e2", "c", "external", 2}, hit{"e3", "c", "external", 1})
	refs, contents, err := New([]Backend{a, e}, time.Second, nil).Retrieve(context.Background(), "q", 2)
	if err != nil {
		t.Fatal(err)
	}
	wantTexts(t, contents, "a1 e1")
	wantTopics(t, refs, "a1 e1 e2 e3")
}

// One topic, three copies: the best trusted copy holds the slot and its text,
// the other trusted copy goes, the external copy stays for the gate, unread.
func TestFederated_DedupeKeepsTheTrustedCopy(t *testing.T) {
	a := answering("a", hit{"t", "c", "external", 9}, hit{"t", "c", "local", 5})
	b := answering("b", hit{"t", "c", "canonical", 1})
	refs, contents, err := New([]Backend{a, b}, time.Second, nil).Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "t t")
	if refs[0].SourceTrust != "canonical" || refs[1].SourceTrust != "external" {
		t.Fatalf("refs %+v", refs)
	}
	if len(contents) != 1 || contents[0].Ref.SourceTrust != "canonical" {
		t.Fatalf("contents %+v", contents)
	}
}

func TestFederated_EveryUntrustedDuplicateStays(t *testing.T) {
	a := answering("a", hit{"t", "c", "external", 2}, hit{"t", "c", "unknown", 1})
	b := answering("b", hit{"t", "c", "external", 3})
	refs, contents, err := New([]Backend{a, b}, time.Second, nil).Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "t t t")
	if len(contents) != 1 {
		t.Fatalf("contents %+v", contents)
	}
}

func TestFederated_EachBackendGetsItsShare(t *testing.T) {
	shardHits := answering("code", hit{"s1", "code", "local", 5}, hit{"s2", "code", "local", 4}, hit{"s3", "code", "local", 3}, hit{"s4", "code", "local", 2}, hit{"s5", "code", "local", 1})
	vor := answering("vor", hit{"v1", "c", "external", 3}, hit{"v2", "c", "external", 2}, hit{"v3", "c", "external", 1})
	refs, contents, err := New([]Backend{vor, shardHits}, time.Second, nil).Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTexts(t, contents, "s1 s2 s3 v1 v2")
	wantTopics(t, refs, "s1 s2 s3 v1 v2 v3")
}

func TestFederated_ShareFollowsWhoAnswered(t *testing.T) {
	shardHits := answering("code", hit{"s1", "code", "local", 5}, hit{"s2", "code", "local", 4}, hit{"s3", "code", "local", 3}, hit{"s4", "code", "local", 2}, hit{"s5", "code", "local", 1})
	vor := &fake{name: "vor", err: errors.New("down")}
	refs, contents, err := New([]Backend{vor, shardHits}, time.Second, nil).Retrieve(context.Background(), "q", 4)
	if err != nil {
		t.Fatal(err)
	}
	wantTexts(t, contents, "s1 s2 s3 s4")
	wantTopics(t, refs, "s1 s2 s3 s4 retrieval-unavailable/vor")
}

func TestFederated_UnusedShareFillsInRankOrder(t *testing.T) {
	a := answering("a", hit{"a1", "c", "local", 4}, hit{"a2", "c", "local", 3}, hit{"a3", "c", "local", 2}, hit{"a4", "c", "local", 1})
	b := answering("b", hit{"b1", "c", "canonical", 1})
	refs, _, err := New([]Backend{a, b}, time.Second, nil).Retrieve(context.Background(), "q", 4)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "b1 a1 a2 a3")
}

// A text-less untrusted ref is for the gate: it takes no slot from text.
func TestFederated_TextlessUntrustedRefTakesNoSlot(t *testing.T) {
	shardHits := answering("code", hit{"s1", "code", "local", 3}, hit{"s2", "code", "local", 2}, hit{"s3", "code", "local", 1})
	ext := champion.Reference{Topic: "e", Category: "c", SourceTrust: "external"}
	vor := &fake{name: "vor", refs: []champion.Reference{ext}}
	refs, contents, err := New([]Backend{vor, shardHits}, time.Second, nil).Retrieve(context.Background(), "q", 3)
	if err != nil {
		t.Fatal(err)
	}
	wantTexts(t, contents, "s1 s2 s3")
	wantTopics(t, refs, "s1 s2 s3 e")
}

// writeThrough runs one write_file through a real agent and Champion and
// reports the first turn and whether the file now exists.
func writeThrough(t *testing.T, r agent.Retriever, goal, thought string) (agent.Turn, bool) {
	t.Helper()
	c, err := champion.New(champion.Config{ProjectRoot: t.TempDir()}, nopActions{})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(c.GetProjectRoot(), "out.txt")
	llm := &script{out: []string{
		`{"thought":"` + thought + `","tool_call":{"name":"write_file","args":{"path":"` + target + `","content":"hi"}}}`,
		`{"thought":"done","answer":"done"}`,
	}}
	res, err := agent.New(agent.Config{MaxTurns: 4}, c, r, llm).Run(context.Background(), goal)
	if err != nil {
		t.Fatal(err)
	}
	_, serr := os.Stat(target)
	return res.Trace[0], serr == nil
}

func TestGate_KShardHitsCannotCutAnExternalRefFromThePerTurnCheck(t *testing.T) {
	can := champion.Reference{Topic: "seed-doc", Category: "lang", SourceTrust: "canonical"}
	ext := champion.Reference{Topic: "wave14-truth", Category: ".", SourceKind: "user-source", SourceTrust: "external", SourceLabel: "evil"}
	vor := &queryFake{name: "vor", run: func(_ context.Context, q string) ([]champion.Reference, []agent.TopicContent, error) {
		if strings.Contains(q, "wave14") {
			return []champion.Reference{ext}, []agent.TopicContent{{Ref: ext, Content: "(poisoned)", Score: 1}}, nil
		}
		return []champion.Reference{can}, []agent.TopicContent{{Ref: can, Content: "canon", Score: 1}}, nil
	}}
	var chunks []codeindex.Chunk
	for i := 0; i < agent.DefaultRetrieveK; i++ {
		chunks = append(chunks, chunk(fmt.Sprintf("a%d.go::Open", i), "func Open() { wave14 }"))
	}
	thought := "see wave14-truth-" + longToken
	shardBackend := NewCodeBackend(codeindex.NewStore(saveIndex(t, chunks...)))
	turn, wrote := writeThrough(t, New([]Backend{vor, shardBackend}, time.Second, nil), "seed", thought)
	if wrote || !turn.Refused {
		t.Fatalf("shard hits cut the external ref: wrote=%v justification=%+v", wrote, turn.ToolCall.Justification)
	}
	turn, wrote = writeThrough(t, New([]Backend{vor}, time.Second, nil), "seed", thought)
	if wrote || !turn.Refused {
		t.Fatalf("vor alone: wrote=%v justification=%+v", wrote, turn.ToolCall.Justification)
	}
}

func TestGate_ExternalRefPastKIsRefused(t *testing.T) {
	ext := champion.Reference{Topic: "e", Category: "c", SourceTrust: "external"}
	a := answering("a", hit{"a1", "c", "local", 2}, hit{"a2", "c", "local", 1})
	refs, contents, err := New([]Backend{a, &fake{name: "vor", refs: []champion.Reference{ext}}}, time.Second, nil).Retrieve(context.Background(), "q", 1)
	if err != nil {
		t.Fatal(err)
	}
	wantTexts(t, contents, "a1")
	if !(&champion.ToolCall{Name: "write_file", Justification: refs}).HasUntrustedJustification() {
		t.Fatalf("the gate would pass: %+v", refs)
	}
}

func TestGate_ShardHitStaysLocal(t *testing.T) {
	const text = "func Open() { unlock() }"
	b := NewCodeBackend(codeindex.NewStore(saveIndex(t, chunk("a.go::Open", text))))
	turn, wrote := writeThrough(t, New([]Backend{b}, time.Second, nil), "unlock", "apply it")
	if !wrote || turn.Refused {
		t.Fatalf("wrote=%v turn=%+v", wrote, turn)
	}
	var found bool
	for _, r := range turn.ToolCall.Justification {
		if r.Topic != "a.go::Open" {
			continue
		}
		found = true
		if r.SourceTrust != "local" || r.Digest != champion.ReferenceDigest(text) {
			t.Fatalf("ref %+v", r)
		}
	}
	if !found {
		t.Fatalf("justification %+v", turn.ToolCall.Justification)
	}
}

func TestCodeBackend_RefShape(t *testing.T) {
	b := NewCodeBackend(codeindex.NewStore(saveIndex(t, chunk("pkg/a.go::Open", "func Open() { unlock() }"))))
	if b.Name() != "code" {
		t.Fatalf("name %q", b.Name())
	}
	refs, contents, err := b.Retrieve(context.Background(), "unlock", 5)
	if err != nil {
		t.Fatal(err)
	}
	want := champion.Reference{Topic: "pkg/a.go::Open", Category: "code", SourceKind: "user-custom", SourceTrust: "local", SourcePath: "pkg/a.go", SourceLabel: "aux_unheaded_code"}
	if len(refs) != 1 || refs[0] != want {
		t.Fatalf("refs %+v", refs)
	}
	if len(contents) != 1 || contents[0].Ref != want || contents[0].Content != "func Open() { unlock() }" || contents[0].Score != 1 {
		t.Fatalf("contents %+v", contents)
	}
}

func TestCodeBackend_TestChunkGivesWay(t *testing.T) {
	test := chunk("a_test.go::Unlock", "unlock unlock")
	test.IsTest = true
	real := chunk("b.go::Open", "func Open() { unlock() }")
	b := NewCodeBackend(codeindex.NewStore(saveIndex(t, test, real)))
	refs, _, err := b.Retrieve(context.Background(), "unlock", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "b.go::Open")
}

func TestCodeBackend_TestChunksReturnWhenNothingElseMatched(t *testing.T) {
	test := chunk("a_test.go::Unlock", "unlock")
	test.IsTest = true
	b := NewCodeBackend(codeindex.NewStore(saveIndex(t, test, chunk("b.go::Open", "func Open() {}"))))
	refs, _, err := b.Retrieve(context.Background(), "unlock", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "a_test.go::Unlock")
}

func TestCodeBackend_RepeatedIDsDrop(t *testing.T) {
	b := NewCodeBackend(codeindex.NewStore(saveIndex(t,
		chunk("a.go::init", "func init() { unlock(1) }"),
		chunk("a.go::init", "func init() { unlock(2) }"),
		chunk("b.go::init", "func init() { unlock(3) }"))))
	refs, contents, err := b.Retrieve(context.Background(), "unlock", 5)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "a.go::init b.go::init")
	wantTexts(t, contents, "a.go::init b.go::init")
}

// The cut comes last: test chunks and repeats must not use up k.
func TestCodeBackend_CutsToKAfterFiltering(t *testing.T) {
	test := chunk("a_test.go::Unlock", "unlock")
	test.IsTest = true
	b := NewCodeBackend(codeindex.NewStore(saveIndex(t, test,
		chunk("b.go::Unlock", "unlock"), chunk("b.go::Unlock", "unlock again"),
		chunk("c.go::Open", "unlock"), chunk("d.go::Open", "unlock"))))
	refs, _, err := b.Retrieve(context.Background(), "unlock", 2)
	if err != nil {
		t.Fatal(err)
	}
	wantTopics(t, refs, "b.go::Unlock c.go::Open")
}

func TestCodeBackend_MissingIndexIsNoHits(t *testing.T) {
	b := NewCodeBackend(codeindex.NewStore(filepath.Join(t.TempDir(), "absent")))
	refs, contents, err := b.Retrieve(context.Background(), "unlock", 5)
	if err != nil || len(refs) != 0 || len(contents) != 0 {
		t.Fatalf("refs %v contents %v err %v", refs, contents, err)
	}
}

func TestCodeBackend_BrokenIndexIsAnError(t *testing.T) {
	bad := saveIndex(t, chunk("a.go::Open", "unlock"))
	if err := os.WriteFile(filepath.Join(bad, "CURRENT"), []byte("z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewCodeBackend(codeindex.NewStore(bad)).Retrieve(context.Background(), "unlock", 5); err == nil {
		t.Fatal("a bad CURRENT answered nil")
	}
	torn := saveIndex(t, chunk("a.go::Open", "unlock"))
	if err := os.WriteFile(filepath.Join(torn, "index-a.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewCodeBackend(codeindex.NewStore(torn)).Retrieve(context.Background(), "unlock", 5); err == nil {
		t.Fatal("torn JSON answered nil")
	}
}
