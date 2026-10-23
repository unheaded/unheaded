// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"context"
	"testing"
	"time"
)

// An insight expires at ExpiresAt. That field was set and never read: every
// insight (one per alerts.critical message) was kept forever, served by
// GetInsight and counted in total_insights long after it expired.
func TestInsights_ExpireAndGo(t *testing.T) {
	s := limitedService(func(c *Config) { c.InsightTTL = time.Hour })
	ctx := context.Background()

	var old []*Insight
	for i := 0; i < 3; i++ {
		in, err := s.GenerateInsight(ctx, "alert_analysis", []string{"e"})
		if err != nil {
			t.Fatal(err)
		}
		old = append(old, in)
	}
	// Two of them expired a minute ago.
	past := time.Now().Add(-time.Minute)
	s.mu.Lock()
	old[0].ExpiresAt, old[1].ExpiresAt = &past, &past
	s.mu.Unlock()

	if _, ok := s.GetInsight(old[0].ID); ok {
		t.Error("GetInsight served an expired insight")
	}
	if _, ok := s.GetInsight(old[2].ID); !ok {
		t.Error("GetInsight lost a live insight")
	}
	if n := s.Stats()["total_insights"]; n != 1 {
		t.Errorf("total_insights = %v, want 1 live", n)
	}

	if _, err := s.GenerateInsight(ctx, "alert_analysis", nil); err != nil {
		t.Fatal(err)
	}
	s.mu.RLock()
	held := len(s.insights)
	s.mu.RUnlock()
	if held != 2 {
		t.Errorf("insights held after the next one = %d, want 2 (expired dropped)", held)
	}
}

// A TTL of 0 means no expiry, not expired on creation.
func TestInsights_ZeroTTLKeeps(t *testing.T) {
	s := limitedService(func(c *Config) { c.InsightTTL = 0 })
	in, _ := s.GenerateInsight(context.Background(), "x", nil)
	time.Sleep(time.Millisecond)
	if _, ok := s.GetInsight(in.ID); !ok || in.ExpiresAt != nil {
		t.Errorf("TTL 0: found %v, expires_at %v; want kept, no expiry", ok, in.ExpiresAt)
	}
}
