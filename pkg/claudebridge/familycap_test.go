// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"errors"
	"testing"
)

// The cap invariant (S-CHAMPION-HARDENING Step 114, PLANS F-23): with
// zhen-claude's defaults (-daily-cap-usd 5, -max-tokens 16000), every
// request that reaches the API had its worst case covered by what the day
// had left, and the first request that would not fit is refused before
// it is sent.
func TestFamilyCap_LeavesWorstCaseHeadroom(t *testing.T) {
	capNano, _ := USDToNano(5)
	var responses []cannedMsg
	for range 30 {
		responses = append(responses, cannedMsg{StopReason: "end_turn", Blocks: []cannedBlock{{Text: "ok"}},
			Usage: map[string]any{"input_tokens": 1000, "output_tokens": 16000}})
	}
	h := newHarness(t, responses, nil, func(c *Config) {
		l, err := NewLedger("", capNano)
		if err != nil {
			t.Fatal(err)
		}
		c.Ledger = l
	})
	var capErr error
	sends := 0
	for range 30 {
		_, err := h.bridge.NewConversation().Send(context.Background(), "go")
		if err != nil {
			capErr = err
			break
		}
		sends++
	}
	var sce *SpendCapError
	if !errors.Is(capErr, ErrSpendCap) || !errors.As(capErr, &sce) {
		t.Fatalf("after %d sends: err = %v, want a spend-cap refusal", sends, capErr)
	}
	if n := len(h.api.reqs()); n != sends || sends < 2 {
		t.Fatalf("API saw %d request(s), %d sends succeeded", n, sends)
	}
	var spent NanoUSD
	for i, rec := range h.audit.reqs {
		left := capNano - spent
		if !rec.Sent {
			if i != len(h.audit.reqs)-1 || rec.Reserved <= left {
				t.Fatalf("record %d refused although its worst case %s fits %s", i, rec.Reserved.USD(), left.USD())
			}
			continue
		}
		if rec.Reserved > left {
			t.Fatalf("request %d sent with worst case %s over the %s left", i, rec.Reserved.USD(), left.USD())
		}
		spent += rec.Cost
	}
	if spent > capNano {
		t.Fatalf("spent %s over the cap %s", spent.USD(), capNano.USD())
	}
}

// A routed Claude leg runs under a small family cap; once a request's
// worst case at the configured max_tokens no longer fits what is left, the
// request is sent with the largest max_tokens that does fit, down to
// MinMaxTokens, instead of ending the leg after one request (F-23).
func TestFamilyCap_ShrinksMaxTokensToWhatIsLeft(t *testing.T) {
	ctx := context.Background()
	familyCap, _ := USDToNano(0.20)
	newLeg := func(t *testing.T, alreadySpent NanoUSD) (*harness, *Ledger) {
		t.Helper()
		l, err := NewLedger("", familyCap)
		if err != nil {
			t.Fatal(err)
		}
		r, _ := l.Reserve(0)
		if _, err := r.Commit(alreadySpent); err != nil {
			t.Fatal(err)
		}
		h := newHarness(t, []cannedMsg{endTurn("ok")}, nil, func(c *Config) {
			c.Ledger = l
			c.MaxTokens = 4096
			c.MinMaxTokens = 1024
			c.DisableFallbacks = true
		})
		return h, l
	}

	t.Run("shrinks", func(t *testing.T) {
		h, l := newLeg(t, familyCap-60_000_000) // $0.06 left: under 4096 out at $20/MTok
		left, err := l.Remaining()
		if err != nil || left != 60_000_000 {
			t.Fatalf("Remaining = %d, %v", left, err)
		}
		if _, err := h.bridge.NewConversation().Send(ctx, "go"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		reqs := h.api.reqs()
		if len(reqs) != 1 {
			t.Fatalf("requests = %d", len(reqs))
		}
		mt, _ := reqs[0].Body["max_tokens"].(float64)
		if mt < 1024 || mt >= 4096 {
			t.Fatalf("max_tokens sent = %v, want shrunk into [1024, 4096)", mt)
		}
		if rec := h.audit.reqs[0]; rec.Reserved > left {
			t.Fatalf("reserved %s over the %s left", rec.Reserved.USD(), left.USD())
		}
	})

	t.Run("refuses under the floor", func(t *testing.T) {
		h, _ := newLeg(t, familyCap-15_000_000) // $0.015 left: 1024 out alone is $0.02
		_, err := h.bridge.NewConversation().Send(ctx, "go")
		if !errors.Is(err, ErrSpendCap) {
			t.Fatalf("err = %v, want ErrSpendCap", err)
		}
		if n := len(h.api.reqs()); n != 0 {
			t.Fatalf("%d request(s) reached the API under the floor", n)
		}
	})

	t.Run("no floor keeps refusing", func(t *testing.T) {
		h, _ := newLeg(t, familyCap-60_000_000)
		h.bridge.cfg.MinMaxTokens = 0
		if _, err := h.bridge.NewConversation().Send(ctx, "go"); !errors.Is(err, ErrSpendCap) {
			t.Fatalf("err = %v, want ErrSpendCap (MinMaxTokens 0 never shrinks)", err)
		}
	})
}

// Without a refusal fallback the request can only be served (and billed)
// by the routed model, so its worst case is priced at that model, not at
// the table's dearest; with the fallback on it stays at the table maximum.
func TestReservation_RoutedModelPrice(t *testing.T) {
	prices, err := DefaultPrices()
	if err != nil {
		t.Fatal(err)
	}
	haiku, ok := prices.Lookup("claude-haiku-5-5")
	if !ok {
		t.Fatal("claude-haiku-5-5 not in prices.json")
	}
	for _, fallbacks := range []bool{false, true} {
		h := newHarness(t, nil, nil, func(c *Config) {
			c.Model = "claude-haiku-5-5"
			c.MaxTokens = 4096
			c.DisableFallbacks = !fallbacks
		})
		params := h.bridge.params(nil)
		need, err := h.bridge.reservation(params)
		if err != nil {
			t.Fatal(err)
		}
		outAtHaiku := NanoUSD(4096 * haiku.Output)
		outAtMax := NanoUSD(4096 * prices.Max().Output)
		switch {
		case !fallbacks && (need < outAtHaiku || need >= outAtMax):
			t.Errorf("no fallback: reserved %s, want priced at haiku (output alone %s)", need.USD(), outAtHaiku.USD())
		case fallbacks && need < 2*outAtMax:
			t.Errorf("fallback on: reserved %s, want at least twice the table-max output %s", need.USD(), outAtMax.USD())
		}
	}
}

func TestFitMaxTokens(t *testing.T) {
	// $0.06 left, 2000 prompt tokens at $5/MTok ($0.01), $20/MTok out, one hop.
	if got := FitMaxTokens(60_000_000, 2000, 5000, 20000, 1, 4096, 1024); got != 2500 {
		t.Errorf("fit = %d, want 2500", got)
	}
	if got := FitMaxTokens(60_000_000, 2000, 5000, 20000, 2, 4096, 1024); got != 0 {
		t.Errorf("two hops: fit = %d, want 0 (1000 tokens fit, under the floor)", got) // ($0.03 - $0.01) / $20/MTok
	}
	if got := FitMaxTokens(100_000_000, 2000, 5000, 20000, 2, 4096, 1024); got != 2000 {
		t.Errorf("two hops: fit = %d, want 2000", got)
	}
	if got := FitMaxTokens(1_000_000_000, 2000, 5000, 20000, 1, 4096, 1024); got != 4096 {
		t.Errorf("plenty left: fit = %d, want the configured 4096", got)
	}
	if got := FitMaxTokens(15_000_000, 2000, 5000, 20000, 1, 4096, 1024); got != 0 {
		t.Errorf("under the floor: fit = %d, want 0", got)
	}
	if got := FitMaxTokens(5_000_000, 2000, 5000, 20000, 1, 4096, 1024); got != 0 {
		t.Errorf("prompt alone over budget: fit = %d, want 0", got)
	}
	if w := WorstCase(2000, 5000, 2500, 20000, 1); w != 60_000_000 {
		t.Errorf("WorstCase = %d", w)
	}
}
