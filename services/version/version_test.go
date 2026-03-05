// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package version

import (
	"context"
	"io"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that discards output, suitable for tests.
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestService creates a Service with sensible test defaults.
// The wotan client is nil because no network operations are performed.
func newTestService() *Service {
	return NewService(newTestLogger(), nil)
}

// makeRule creates a TranslationRule with reasonable defaults.
func makeRule(src, dst string) *TranslationRule {
	return &TranslationRule{
		SrcVersion:    src,
		DstVersion:    dst,
		FieldMask:     0xFF00,
		OffsetMap:     [8]byte{0, 4, 8, 12, 16, 20, 0, 0},
		WidthDeltas:   [4]int8{0, 2, -1, 0},
		DefaultValues: [4]byte{0, 0, 0xFF, 0},
		CRCRecalc:     true,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestTranslationRuleCRUD(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("add translation rule", func(t *testing.T) {
		rule := makeRule("v1", "v2")
		if err := svc.AddTranslationRule(rule); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		got, ok := svc.GetTranslationRule("v1", "v2")
		if !ok {
			t.Fatal("expected rule to exist")
		}
		if got.SrcVersion != "v1" || got.DstVersion != "v2" {
			t.Errorf("expected v1->v2, got %s->%s", got.SrcVersion, got.DstVersion)
		}
		if !got.CRCRecalc {
			t.Error("expected CRCRecalc to be true")
		}
		if got.FieldMask != 0xFF00 {
			t.Errorf("expected field_mask 0xFF00, got 0x%X", got.FieldMask)
		}
		if got.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
	})

	t.Run("update existing rule", func(t *testing.T) {
		rule := makeRule("v1", "v2")
		rule.CRCRecalc = false
		rule.FieldMask = 0x00FF
		if err := svc.AddTranslationRule(rule); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		got, _ := svc.GetTranslationRule("v1", "v2")
		if got.CRCRecalc {
			t.Error("expected CRCRecalc to be false after update")
		}
		if got.FieldMask != 0x00FF {
			t.Errorf("expected field_mask 0x00FF, got 0x%X", got.FieldMask)
		}
	})

	t.Run("list rules", func(t *testing.T) {
		svc.AddTranslationRule(makeRule("v2", "v3"))
		rules := svc.ListTranslationRules()
		if len(rules) < 2 {
			t.Errorf("expected at least 2 rules, got %d", len(rules))
		}
	})

	t.Run("delete rule", func(t *testing.T) {
		if err := svc.DeleteTranslationRule("v1", "v2"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if _, ok := svc.GetTranslationRule("v1", "v2"); ok {
			t.Error("expected rule to be deleted")
		}
	})

	t.Run("delete non-existent rule", func(t *testing.T) {
		if err := svc.DeleteTranslationRule("v99", "v100"); err == nil {
			t.Error("expected error for non-existent rule")
		}
	})

	t.Run("add rule with missing versions", func(t *testing.T) {
		rule := &TranslationRule{SrcVersion: "", DstVersion: "v2"}
		if err := svc.AddTranslationRule(rule); err == nil {
			t.Error("expected error for empty src_version")
		}

		rule = &TranslationRule{SrcVersion: "v1", DstVersion: ""}
		if err := svc.AddTranslationRule(rule); err == nil {
			t.Error("expected error for empty dst_version")
		}
	})
}

func TestServiceVersionRegistry(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	t.Run("update and get service version", func(t *testing.T) {
		svc.UpdateServiceVersion(ctx, "timeguru", "1.2.0", 0x0F)

		info, ok := svc.GetServiceVersion("timeguru")
		if !ok {
			t.Fatal("expected service version to exist")
		}
		if info.CurrentVersion != "1.2.0" {
			t.Errorf("expected version 1.2.0, got %s", info.CurrentVersion)
		}
		if info.Capabilities != 0x0F {
			t.Errorf("expected capabilities 0x0F, got 0x%X", info.Capabilities)
		}
		if info.LastUpdated.IsZero() {
			t.Error("expected LastUpdated to be set")
		}
	})

	t.Run("overwrite service version", func(t *testing.T) {
		svc.UpdateServiceVersion(ctx, "timeguru", "1.3.0", 0xFF)

		info, _ := svc.GetServiceVersion("timeguru")
		if info.CurrentVersion != "1.3.0" {
			t.Errorf("expected version 1.3.0, got %s", info.CurrentVersion)
		}
	})

	t.Run("list service versions", func(t *testing.T) {
		svc.UpdateServiceVersion(ctx, "captain", "2.0.0", 0x03)
		svc.UpdateServiceVersion(ctx, "architect", "1.1.0", 0x07)

		versions := svc.ListServiceVersions()
		if len(versions) < 3 {
			t.Errorf("expected at least 3 services, got %d", len(versions))
		}
	})

	t.Run("get non-existent service", func(t *testing.T) {
		if _, ok := svc.GetServiceVersion("non-existent"); ok {
			t.Error("expected service not to exist")
		}
	})
}

func TestTranslationStats(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("record translations", func(t *testing.T) {
		svc.RecordTranslation("v1", 100.0)
		svc.RecordTranslation("v1", 200.0)
		svc.RecordTranslation("v2", 50.0)

		stats := svc.GetTranslationStats()
		if stats.TotalTranslations != 3 {
			t.Errorf("expected 3 total translations, got %d", stats.TotalTranslations)
		}
		if stats.TranslationsByVersion["v1"] != 2 {
			t.Errorf("expected 2 v1 translations, got %d", stats.TranslationsByVersion["v1"])
		}
		if stats.TranslationsByVersion["v2"] != 1 {
			t.Errorf("expected 1 v2 translation, got %d", stats.TranslationsByVersion["v2"])
		}
		if stats.AvgLatencyUS <= 0 {
			t.Error("expected positive average latency")
		}
		if stats.LastUpdated.IsZero() {
			t.Error("expected LastUpdated to be set")
		}
	})

	t.Run("record errors", func(t *testing.T) {
		svc.RecordTranslationError()
		svc.RecordTranslationError()

		stats := svc.GetTranslationStats()
		if stats.ErrorCount != 2 {
			t.Errorf("expected 2 errors, got %d", stats.ErrorCount)
		}
	})

	t.Run("service-level stats", func(t *testing.T) {
		stats := svc.Stats()
		if stats["total_translations"].(uint64) != 3 {
			t.Errorf("expected 3 total translations in service stats")
		}
		if stats["error_count"].(uint64) != 2 {
			t.Errorf("expected 2 errors in service stats")
		}
	})
}

func TestHeatmapGeneration(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("add heatmap entries", func(t *testing.T) {
		now := time.Now()
		svc.AddHeatmapEntry(VersionHeatmapEntry{
			Timestamp:   now,
			Version:     "v1",
			PacketCount: 100,
		})
		svc.AddHeatmapEntry(VersionHeatmapEntry{
			Timestamp:   now,
			Version:     "v2",
			PacketCount: 200,
		})

		entries := svc.GetHeatmap()
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Version != "v1" {
			t.Errorf("expected first entry version v1, got %s", entries[0].Version)
		}
		if entries[1].PacketCount != 200 {
			t.Errorf("expected second entry packet count 200, got %d", entries[1].PacketCount)
		}
	})

	t.Run("heatmap snapshot generation", func(t *testing.T) {
		// Record some translations first to populate version stats
		svc.RecordTranslation("v1", 10.0)
		svc.RecordTranslation("v2", 20.0)

		// Trigger snapshot
		svc.generateHeatmapSnapshot()

		entries := svc.GetHeatmap()
		// Should have the 2 manually added + at least 2 from snapshot
		if len(entries) < 4 {
			t.Errorf("expected at least 4 heatmap entries after snapshot, got %d", len(entries))
		}
	})
}

func TestWotanDrops(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	// With nil Wotan, publish should increment drop counter
	svc.publishEvent(ctx, "test.topic", map[string]interface{}{"test": true})

	if svc.WotanDrops() != 1 {
		t.Errorf("expected 1 wotan drop, got %d", svc.WotanDrops())
	}
}

func TestNewService(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	if svc.log == nil {
		t.Error("expected logger to be set")
	}
	if svc.translationRules == nil {
		t.Error("expected translationRules map to be initialized")
	}
	if svc.serviceVersions == nil {
		t.Error("expected serviceVersions map to be initialized")
	}
	if svc.translationStats == nil {
		t.Error("expected translationStats to be initialized")
	}
	if svc.translationStats.TranslationsByVersion == nil {
		t.Error("expected TranslationsByVersion map to be initialized")
	}
}

func TestRuleKey(t *testing.T) {
	t.Parallel()

	key := ruleKey("v1", "v2")
	if key != "v1->v2" {
		t.Errorf("expected 'v1->v2', got %q", key)
	}
}
