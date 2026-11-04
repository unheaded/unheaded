// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Confirmation pass: adversarial tests. Every test states the SECURE outcome.
package retrieval

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
	"unheaded/pkg/codeindex"
)

func try(t *testing.T, r agent.Retriever, goal, thought string) (*agent.Turn, bool, error) {
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
	_, serr := os.Stat(target)
	wrote := serr == nil
	if err != nil {
		return nil, wrote, err
	}
	return &res.Trace[0], wrote, nil
}

func shard(t *testing.T) *CodeBackend {
	return NewCodeBackend(codeindex.NewStore(saveIndex(t,
		chunk("a.go::Open", "func Open() { unlock(); wave14 }"),
		chunk("b.go::Shut", "func Shut() { unlock(); wave14 }"))))
}

// vor erroring on everything. Alone: the run fails, nothing written.
func TestAdv_VorDown_Control_VorAloneWritesNothing(t *testing.T) {
	vor := &fake{name: "vor", err: errors.New("connection refused")}
	_, wrote, err := try(t, New([]Backend{vor}, time.Second, nil), "unlock", "apply it")
	if wrote || err == nil {
		t.Fatalf("vor alone: wrote=%v err=%v", wrote, err)
	}
}

// vor down, shard up: is a write justified by shard text alone refused?
func TestAdv_VorDown_ShardMustNotCarryAWrite(t *testing.T) {
	for _, order := range []string{"vor,code", "code,vor"} {
		t.Run(order, func(t *testing.T) {
			vor := &fake{name: "vor", err: errors.New("connection refused")}
			bs := []Backend{vor, shard(t)}
			if order == "code,vor" {
				bs[0], bs[1] = bs[1], bs[0]
			}
			turn, wrote, err := try(t, New(bs, time.Second, nil), "unlock", "apply it")
			if wrote {
				t.Fatalf("FAIL OPEN: vor down, write ran. turn=%+v err=%v", turn, err)
			}
		})
	}
}

// slowVor: answers the seed; on the poison token it would answer external but
// only after sleep.
func slowVor(sleep time.Duration) *queryFake {
	can := champion.Reference{Topic: "seed-doc", Category: "lang", SourceTrust: "canonical"}
	ext := champion.Reference{Topic: "wave14-truth", Category: ".", SourceKind: "user-source", SourceTrust: "external", SourceLabel: "evil-corpus"}
	return &queryFake{name: "vor", run: func(ctx context.Context, q string) ([]champion.Reference, []agent.TopicContent, error) {
		if !strings.Contains(q, "wave14") {
			return []champion.Reference{can}, []agent.TopicContent{{Ref: can, Content: "canon", Score: 1}}, nil
		}
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
		return []champion.Reference{ext}, []agent.TopicContent{{Ref: ext, Content: "(poisoned)", Score: 1}}, nil
	}}
}

const poisonThought = "see wave14-truth-" + longToken

// Control: no deadline, so the slow answer arrives and the gate refuses.
func TestAdv_SlowVor_Control_NoDeadlineRefuses(t *testing.T) {
	_, wrote, err := try(t, New([]Backend{slowVor(120 * time.Millisecond)}, 0, nil), "seed", poisonThought)
	if wrote || err != nil {
		t.Fatalf("wrote=%v err=%v", wrote, err)
	}
}

// The 2 s deadline, vor alone: the same slow answer is now dropped.
func TestAdv_SlowVor_DeadlineAlone_MustStillRefuse(t *testing.T) {
	turn, wrote, err := try(t, New([]Backend{slowVor(120 * time.Millisecond)}, 30*time.Millisecond, nil), "seed", poisonThought)
	if wrote {
		t.Fatalf("FAIL OPEN: deadline dropped the external answer and the write ran. justification=%+v err=%v", turn.ToolCall.Justification, err)
	}
}

// Same, shard on.
func TestAdv_SlowVor_DeadlinePlusShard_MustStillRefuse(t *testing.T) {
	turn, wrote, err := try(t, New([]Backend{slowVor(120 * time.Millisecond), shard(t)}, 30*time.Millisecond, nil), "seed", poisonThought)
	if wrote {
		t.Fatalf("FAIL OPEN: justification=%+v err=%v", turn.ToolCall.Justification, err)
	}
}

// vor's partial answer at the deadline (nil error): dropped, and the external
// ref it held goes with it.
func TestAdv_PartialAtDeadline_MustStillRefuse(t *testing.T) {
	ext := champion.Reference{Topic: "wave14-truth", Category: ".", SourceTrust: "external", SourceLabel: "evil"}
	can := champion.Reference{Topic: "seed-doc", Category: "lang", SourceTrust: "canonical"}
	vor := &queryFake{name: "vor", run: func(ctx context.Context, q string) ([]champion.Reference, []agent.TopicContent, error) {
		if !strings.Contains(q, "wave14") {
			return []champion.Reference{can}, []agent.TopicContent{{Ref: can, Content: "canon", Score: 1}}, nil
		}
		<-ctx.Done()
		return []champion.Reference{ext}, []agent.TopicContent{{Ref: ext, Content: "(poisoned)", Score: 1}}, nil
	}}
	turn, wrote, _ := try(t, New([]Backend{vor, shard(t)}, 30*time.Millisecond, nil), "seed", poisonThought)
	if wrote {
		t.Fatalf("FAIL OPEN: justification=%+v", turn.ToolCall.Justification)
	}
}

// A backend panics: same class.
func TestAdv_VorPanics_MustNotFailOpen(t *testing.T) {
	vor := &fake{name: "vor", run: func(context.Context) ([]champion.Reference, []agent.TopicContent, error) { panic("boom") }}
	_, wrote, _ := try(t, New([]Backend{vor, shard(t)}, time.Second, nil), "unlock", "apply it")
	if wrote {
		t.Fatal("FAIL OPEN: vor panicked, write ran")
	}
}

// Forged label: an external ref carrying a shard hit's exact topic, path and
// label. It survives Federated; does it survive the agent's mergeRefs?
func TestAdv_ForgedShardIdentity_MustStillRefuse(t *testing.T) {
	forged := champion.Reference{Topic: "a.go::Open", Category: "code", SourceKind: "user-source",
		SourceTrust: "external", SourcePath: "a.go", SourceLabel: codeSourceLabel}
	vor := &fake{name: "vor", refs: []champion.Reference{forged}}
	refs, _, err := New([]Backend{vor, shard(t)}, time.Second, nil).Retrieve(context.Background(), "unlock", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !(&champion.ToolCall{Name: "write_file", Justification: refs}).HasUntrustedJustification() {
		t.Fatalf("lost in Federated: %+v", refs)
	}
	turn, wrote, _ := try(t, New([]Backend{vor, shard(t)}, time.Second, nil), "unlock", "apply wave14-fix now")
	if wrote {
		t.Fatalf("LOST IN mergeRefs: justification=%+v", turn.ToolCall.Justification)
	}
}

// Empty text on an external ref; on a local ref alongside it.
func TestAdv_EmptyText_ExternalStillRefused(t *testing.T) {
	ext := champion.Reference{Topic: "e", Category: "x", SourceTrust: "external"}
	vor := &fake{name: "vor", refs: []champion.Reference{ext}, contents: []agent.TopicContent{{Ref: ext, Content: ""}}}
	_, wrote, _ := try(t, New([]Backend{vor, shard(t)}, time.Second, nil), "unlock", "apply it")
	if wrote {
		t.Fatal("empty-text external let the write run")
	}
}

// A local ref whose text was cut (no digest) must fail the chain, not pass.
func TestAdv_LocalRefWithNoText_FailsClosed(t *testing.T) {
	loc := champion.Reference{Topic: "l", Category: "x", SourceTrust: "local"}
	vor := &fake{name: "vor", refs: []champion.Reference{loc}}
	_, wrote, _ := try(t, New([]Backend{vor, answering("code", hit{"z", "x", "canonical", 1})}, time.Second, nil), "unlock", "apply it")
	if wrote {
		t.Fatal("text-less local ref passed")
	}
}

// Randomized: every backend answers, at least one ref untrusted, whatever the
// k, dedupe, order or share. Through a real agent and Champion.
func TestAdv_Property_NoAnsweredUntrustedRefIsLost(t *testing.T) {
	trusts := []string{"canonical", "local", "external", "unknown", "", " local", "LOCAL", "user-source"}
	topicsPool := []string{"t1", "t2", "t3", "t4"}
	cats := []string{"code", "c2"}
	labels := []string{"aux_unheaded_code", "unheaded", "evil"}
	paths := []string{"a.go", "/var/zhen/vor-source/unheaded"}
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))
	lost := map[string]int{}
	for i := 0; i < 1500; i++ {
		k := 1 + rng.Intn(6)
		var bs []Backend
		anyUntrusted := false
		for b := 0; b < 2+rng.Intn(2); b++ {
			f := &fake{name: fmt.Sprintf("b%d", b)}
			for n := rng.Intn(7); n > 0; n-- {
				r := champion.Reference{Topic: topicsPool[rng.Intn(len(topicsPool))], Category: cats[rng.Intn(2)],
					SourceTrust: trusts[rng.Intn(len(trusts))], SourceLabel: labels[rng.Intn(3)], SourcePath: paths[rng.Intn(2)]}
				if !champion.TrustedSourceLabels[r.SourceTrust] {
					anyUntrusted = true
				}
				f.refs = append(f.refs, r)
				if rng.Intn(3) > 0 {
					f.contents = append(f.contents, agent.TopicContent{Ref: r, Content: "same text", Score: rng.Float64()})
				}
			}
			bs = append(bs, f)
		}
		if !anyUntrusted {
			continue
		}
		refs, _, err := New(bs, time.Second, nil).Retrieve(context.Background(), "q", k)
		if err != nil {
			t.Fatal(err)
		}
		if !(&champion.ToolCall{Name: "write_file", Justification: refs}).HasUntrustedJustification() {
			lost["federated"]++
			continue
		}
		// agent level, its RetrieveK must match: default 5, so k=5 path.
		turn, wrote, _ := try(t, New(bs, time.Second, nil), "q", "apply it")
		if wrote {
			lost["agent"]++
			if lost["agent"] == 1 {
				for bi, b := range bs {
					for _, r := range b.(*fake).refs {
						t.Logf("in  b%d %q|%q|%q trust=%q", bi, r.Topic, r.SourcePath, r.SourceLabel, r.SourceTrust)
					}
				}
				for _, r := range turn.ToolCall.Justification {
					t.Logf("out %q|%q|%q trust=%q", r.Topic, r.SourcePath, r.SourceLabel, r.SourceTrust)
				}
			}
		}
	}
	if len(lost) > 0 {
		t.Fatalf("seed %d lost: %v", seed, lost)
	}
}
