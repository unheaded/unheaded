// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestLedger_PersistsAndCaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "spend.json")
	l, err := NewLedger(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.Reserve(600)
	if err != nil {
		t.Fatal(err)
	}
	// In flight: a second 600 must not fit beside the first.
	if _, err := l.Reserve(600); !errors.Is(err, ErrSpendCap) {
		t.Fatalf("second reservation: %v", err)
	}
	if total, err := r.Commit(300); err != nil || total != 300 {
		t.Fatalf("commit: %d %v", total, err)
	}
	if _, err := r.Commit(300); err != nil {
		t.Fatal("double commit should be a no-op")
	}

	// A new process sees the committed spend.
	l2, err := NewLedger(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if spent, _ := l2.Spent(); spent != 300 {
		t.Fatalf("persisted spend = %d", spent)
	}
	if _, err := l2.Reserve(701); !errors.Is(err, ErrSpendCap) {
		t.Fatalf("over-cap reservation: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("ledger file mode: %v %v", info.Mode(), err)
	}
}

func TestLedger_DayRollover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spend.json")
	l, _ := NewLedger(path, 1000)
	day := time.Date(2026, 10, 23, 23, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return day }
	r, _ := l.Reserve(900)
	_, _ = r.Commit(900)
	if _, err := l.Reserve(200); !errors.Is(err, ErrSpendCap) {
		t.Fatal("cap should hold within the day")
	}
	l.now = func() time.Time { return day.Add(2 * time.Hour) }
	if _, err := l.Reserve(200); err != nil {
		t.Fatalf("new UTC day should start at zero: %v", err)
	}
}

func TestLedger_CorruptFileFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spend.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLedger(path, 1000); err == nil {
		t.Fatal("corrupt ledger accepted")
	}
}

func TestLedger_ZeroCapRefusesAll(t *testing.T) {
	l, _ := NewLedger("", 0)
	if _, err := l.Reserve(1); !errors.Is(err, ErrSpendCap) {
		t.Fatal("zero cap must refuse")
	}
}

func TestUSDToNano(t *testing.T) {
	if n, err := USDToNano(1.5); err != nil || n != 1_500_000_000 {
		t.Errorf("1.5 -> %d %v", n, err)
	}
	for _, bad := range []float64{-1, 2e6} {
		if _, err := USDToNano(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if NanoUSD(1_234_567_000).USD() != "$1.234567" {
		t.Error(NanoUSD(1_234_567_000).USD())
	}
}

func TestPrices(t *testing.T) {
	p, err := DefaultPrices()
	if err != nil {
		t.Fatal(err)
	}
	opus, ok := p.Lookup("claude-opus-5-5")
	if !ok || opus.Input != 4000 || opus.Output != 20000 || opus.CacheRead != 200 {
		t.Fatalf("opus 5.5 = %+v", opus)
	}
	if _, ok := p.Lookup("claude-unknown"); ok {
		t.Error("unknown model found")
	}
	if p.Max().Output != 25000 {
		t.Errorf("max output = %d", p.Max().Output)
	}

	// Plain usage, with a cache read and a 5m cache write.
	u := anthropic.BetaUsage{InputTokens: 100, OutputTokens: 10, CacheReadInputTokens: 1000, CacheCreationInputTokens: 50}
	cost, unknown := p.UsageCost("claude-opus-5-5", u)
	want := NanoUSD(100*4000 + 10*20000 + 1000*200 + 50*5000)
	if cost != want || len(unknown) != 0 {
		t.Errorf("cost = %d want %d (%v)", cost, want, unknown)
	}

	// Fallback hop: each iteration priced by the model that ran it; an
	// unknown model at the table maximum.
	u = anthropic.BetaUsage{Iterations: anthropic.BetaIterationsUsage{
		{Type: "message", Model: "claude-opus-5-5", InputTokens: 100},
		{Type: "fallback_message", Model: "claude-opus-4-8", InputTokens: 100, OutputTokens: 10},
		{Type: "fallback_message", Model: "claude-mystery", OutputTokens: 1},
	}}
	cost, unknown = p.UsageCost("claude-opus-4-8", u)
	want = NanoUSD(100*4000 + 100*5000 + 10*25000 + 1*25000)
	if cost != want || len(unknown) != 1 || unknown[0] != "claude-mystery" {
		t.Errorf("iteration cost = %d want %d (%v)", cost, want, unknown)
	}
}

func TestParsePrices_Rejects(t *testing.T) {
	for _, bad := range []string{
		`{}`,
		`{"models":{"m":{"input":0,"output":1}}}`,
		`{"models":{"m":{"input":-1,"output":1}}}`,
		`not json`,
	} {
		if _, err := ParsePrices([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func clearCredentialEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_PROFILE", "ANTHROPIC_FEDERATION_RULE_ID", "ANTHROPIC_ORGANIZATION_ID", "ANTHROPIC_IDENTITY_TOKEN_FILE", "ANTHROPIC_IDENTITY_TOKEN"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("ANTHROPIC_CONFIG_DIR", t.TempDir())
}

func TestCredentialSource(t *testing.T) {
	clearCredentialEnv(t)
	if _, err := CredentialSource(); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("no credentials: %v", err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "placeholder-not-a-key")
	src, err := CredentialSource()
	if err != nil || src != "env ANTHROPIC_API_KEY" {
		t.Fatalf("src = %q, %v", src, err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_PROFILE", "nosuch")
	if _, err := CredentialSource(); err == nil {
		t.Fatal("a named profile that does not exist must fail")
	}
}
