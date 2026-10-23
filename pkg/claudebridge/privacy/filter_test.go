// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package privacy

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// echoLocal is a local model that "summarises" by echoing a line with an
// address in it: the summary must be redacted again.
type echoLocal struct {
	calls int
	err   error
	got   string
}

func (e *echoLocal) Complete(_ context.Context, system, prompt string) (string, error) {
	e.calls++
	e.got = prompt
	if e.err != nil {
		return "", e.err
	}
	return "wotan at 10.10.10.10 on west restarted 3 times", nil
}

func bigOps() string {
	return strings.Repeat("service wotan status=degraded peer 192.168.13.2 errors=3\n", 400) // > 8 KiB
}

func TestFilter_SummaryIsRedactedAgain(t *testing.T) {
	local := &echoLocal{}
	f := &Filter{Redactor: newTestRedactor(t), Local: local}
	out, o, err := f.ToolResult(context.Background(), "service_health", nil, "", bigOps())
	if err != nil {
		t.Fatal(err)
	}
	if local.calls != 1 || !o.Summarised || o.Tier != TierOps {
		t.Fatalf("outcome = %+v, calls %d", o, local.calls)
	}
	if strings.Contains(out, "10.10.10.10") || strings.Contains(out, "west") {
		t.Errorf("the summary re-introduced an identifier: %s", out)
	}
	if strings.Contains(local.got, "192.168.13.2") {
		t.Error("the local model was sent unredacted text")
	}
	if len(out) > 2048 {
		t.Errorf("summary not small: %d bytes", len(out))
	}
}

func TestFilter_LocalDownCutsRedacted(t *testing.T) {
	f := &Filter{Redactor: newTestRedactor(t), Local: &echoLocal{err: errors.New("connection refused")}}
	out, o, err := f.ToolResult(context.Background(), "service_health", nil, "", bigOps())
	if err != nil {
		t.Fatal(err)
	}
	if !o.SummaryUnavailable || o.Summarised {
		t.Errorf("outcome = %+v", o)
	}
	if !strings.HasPrefix(out, "[summary_unavailable") || len(out) > DefaultFallbackBytes+200 {
		t.Errorf("fallback = %d bytes: %.120s", len(out), out)
	}
	if strings.Contains(out, "192.168.13.2") {
		t.Error("the cut was not redacted")
	}
}

func TestFilter_SmallOpsRedactedNotSummarised(t *testing.T) {
	local := &echoLocal{}
	f := &Filter{Redactor: newTestRedactor(t), Local: local}
	out, o, err := f.ToolResult(context.Background(), "service_health", nil, "", "wotan 10.10.10.10 down")
	if err != nil {
		t.Fatal(err)
	}
	if local.calls != 0 || o.Summarised || o.Counts.Total() != 1 || strings.Contains(out, "10.10.10.10") {
		t.Errorf("out %q outcome %+v", out, o)
	}
}

func TestFilter_UserDataWithheld(t *testing.T) {
	ud := t.TempDir()
	f := &Filter{Redactor: newTestRedactor(t), Classifier: Classifier{UserDataRoots: []string{ud}}}
	if _, o, err := f.ToolResult(context.Background(), "read_file", map[string]any{"path": ud + "/x.csv"}, "", "alice,bob"); !errors.Is(err, ErrWithheld) || !o.Withheld {
		t.Errorf("T2: %v %+v", err, o)
	}
}

func TestFilter_StrictWithholdsOps(t *testing.T) {
	f := &Filter{Redactor: newTestRedactor(t), Mode: ModeStrict}
	if _, o, err := f.ToolResult(context.Background(), "service_health", nil, "", "ok"); !errors.Is(err, ErrStrictWithheld) || !o.Withheld {
		t.Errorf("strict T1: %v %+v", err, o)
	}
	// T0 still passes in strict mode.
	if _, _, err := f.ToolResult(context.Background(), "runbook_list", nil, "", "a b c"); err != nil {
		t.Errorf("strict T0: %v", err)
	}
}

func TestFilter_NilFailsClosed(t *testing.T) {
	var f *Filter
	if _, _, err := f.ToolResult(context.Background(), "x", nil, "", "y"); err == nil {
		t.Error("nil filter let text through")
	}
	if _, _, err := (&Filter{}).Prompt(context.Background(), "y"); err == nil {
		t.Error("filter without redactor let a prompt through")
	}
}

func TestCounts_String(t *testing.T) {
	if s := (Counts{"ipv4": 2, "aws": 1}).String(); s != "aws=1,ipv4=2" {
		t.Errorf("%q", s)
	}
}
