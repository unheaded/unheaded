// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestSeedGen_LiveLocalModel is an opt-in smoke against the real, already-
// running llama-server (set LICH_E2E=1). It never starts or swaps a model and
// stays tiny (a handful of seeds). It proves the local-model driver produces
// usable seeds end to end; it is skipped by default so CI never needs a GPU.
func TestSeedGen_LiveLocalModel(t *testing.T) {
	if os.Getenv("LICH_E2E") == "" {
		t.Skip("set LICH_E2E=1 to run against the live llama-server")
	}
	url := os.Getenv("LICH_LOCAL_URL")
	if url == "" {
		url = DefaultLocalLLMURL
	}
	p := providerWithLocal(t, url, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := p.GenerateSeeds(ctx, SeedSpec{Campaign: "lich_001_monad_wire", Format: FormatMonadWire, Count: 6})
	if err != nil {
		t.Fatalf("live GenerateSeeds: %v", err)
	}
	t.Logf("live seed batch: requested=%d parsed=%d written=%d dropped=%d dup=%d dir=%s",
		res.Requested, res.Parsed, res.Written, res.Dropped, res.Duplicates, res.Dir)
	if res.Parsed == 0 && res.Dropped == 0 && res.Written == 0 {
		t.Fatal("the local model returned nothing parseable; check the endpoint")
	}
}
