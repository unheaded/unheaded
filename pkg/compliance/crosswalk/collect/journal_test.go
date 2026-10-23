// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/remediation"
)

var sumA, sumB = strings.Repeat("a", 64), strings.Repeat("b", 64)

func journalOut(t *testing.T, steps ...[3]string) string { // {fix, action, outcome}
	t.Helper()
	var buf bytes.Buffer
	prev := remediation.GenesisHash
	for i, s := range steps {
		sum := sumA
		if s[0] == "stale" {
			sum = sumB
		}
		e := remediation.JournalEntry{Host: "east", Seq: int64(i + 1), FixID: strings.TrimPrefix(s[0], "stale-"), FixSHA256: sum,
			Action: s[1], Outcome: s[2], Operator: "t", At: time.Date(2026, 10, 1, 4, i, 0, 0, time.UTC)}
		if s[0] == "stale" {
			e.FixID = "kmod-cis"
		}
		if err := e.Seal(prev); err != nil {
			t.Fatal(err)
		}
		prev = e.Hash
		if err := e.WriteJSONL(&buf); err != nil {
			t.Fatal(err)
		}
	}
	return buf.String() + "@ok\n"
}

func TestJudgeJournal(t *testing.T) {
	sums := map[string]string{"kmod-cis": sumA, "banner-cis": sumA}
	want := []string{"banner-cis", "kmod-cis"}
	cases := []struct {
		name string
		out  string
		want []string
		pass bool
		has  string
	}{
		{"all applied", journalOut(t, [3]string{"kmod-cis", "apply", "success"}, [3]string{"banner-cis", "apply", "success"}), want, true, "chain verifies"},
		{"one missing", journalOut(t, [3]string{"kmod-cis", "apply", "success"}), want, false, "not applied: banner-cis"},
		{"refused apply does not count", journalOut(t, [3]string{"kmod-cis", "apply", "success"}, [3]string{"banner-cis", "apply", "refused"}), want, false, "not applied: banner-cis"},
		{"undone since", journalOut(t, [3]string{"kmod-cis", "apply", "success"}, [3]string{"banner-cis", "apply", "success"}, [3]string{"kmod-cis", "undo", "success"}), want, false, "not applied: kmod-cis"},
		{"stale content", journalOut(t, [3]string{"stale", "apply", "success"}, [3]string{"banner-cis", "apply", "success"}), want, false, "re-apply): kmod-cis"},
		{"no journal yet", "@none\n@ok\n", want, false, "no journal"},
		{"no journal, nothing wanted", "@none\n@ok\n", nil, true, "turns no fix on"},
		{"read failed", "", want, false, "not read"},
		{"marker forged before the end", "@ok\nmore\n", want, false, "not read"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pass, detail, _ := JudgeJournal(c.out, c.want, sums)
			if pass != c.pass || !strings.Contains(detail, c.has) {
				t.Errorf("pass=%v %q, want pass=%v containing %q", pass, detail, c.pass, c.has)
			}
		})
	}
	tampered := strings.Replace(journalOut(t, [3]string{"kmod-cis", "apply", "success"}, [3]string{"banner-cis", "apply", "success"}), `"operator":"t"`, `"operator":"x"`, 1)
	if pass, detail, _ := JudgeJournal(tampered, want, sums); pass || !strings.Contains(detail, "chain broken") {
		t.Errorf("tampered journal: %v %q", pass, detail)
	}
}

func TestHostJournalCollect(t *testing.T) {
	out := journalOut(t, [3]string{"kmod-cis", "apply", "success"})
	h := &HostJournal{
		Wanted:   map[string][]string{"east": {"kmod-cis"}, "west": {"kmod-cis"}},
		SHA256:   map[string]string{"kmod-cis": sumA},
		Hostname: "elsewhere",
		Now:      func() time.Time { return time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC) },
		Run: func(_ context.Context, host, script string) (string, error) {
			if !strings.Contains(script, "sudo -n sh -c") || !strings.Contains(script, "echo @ok'") {
				t.Errorf("script does not print @ok inside sudo: %s", script)
			}
			if host == "west" {
				return "", errors.New("unreachable")
			}
			return out, nil
		},
	}
	srcs := []crosswalk.Source{
		{Kind: crosswalk.KindHostJournal, Ref: "east:apply-journal"},
		{Kind: crosswalk.KindHostJournal, Ref: "west:apply-journal"},
		{Kind: crosswalk.KindHostProbe, Ref: "east:shell-tmout"},
	}
	recs, err := h.Collect(context.Background(), srcs)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Source.Ref != "east:apply-journal" || recs[0].Verdict != crosswalk.VerdictPass {
		t.Fatalf("records %+v (west unreachable must be absent, not a pass)", recs)
	}
	if len(h.Journals["east"]) != 1 {
		t.Errorf("parsed journal not kept for ingest: %v", h.Journals)
	}

	// A host with no profile bound once passed "the profile turns no fix
	// on"; one bound to a profile with no fixes still does.
	h.Wanted = map[string][]string{"east": nil}
	h.Run = func(context.Context, string, string) (string, error) { return "@none\n@ok\n", nil }
	recs, err = h.Collect(context.Background(), []crosswalk.Source{
		{Kind: crosswalk.KindHostJournal, Ref: "east:apply-journal"},
		{Kind: crosswalk.KindHostJournal, Ref: "west:apply-journal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]crosswalk.Verdict{}
	for _, r := range recs {
		got[r.Source.Ref] = r.Verdict
	}
	if got["east:apply-journal"] != crosswalk.VerdictPass || got["west:apply-journal"] != crosswalk.VerdictFail {
		t.Errorf("bound to no fixes / not bound = %v, want east pass, west fail", got)
	}
}
