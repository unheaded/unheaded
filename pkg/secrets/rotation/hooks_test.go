// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package rotation

import (
	"context"
	"errors"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// --- HookManager ---

func TestNewHookManager_Defaults(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{})
	if hm == nil {
		t.Fatal("expected non-nil")
	}
	if hm.timeout != 30*time.Second {
		t.Fatalf("expected 30s default timeout, got %v", hm.timeout)
	}
}

func TestNewHookManager_CustomTimeout(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{DefaultTimeout: 5 * time.Second})
	if hm.timeout != 5*time.Second {
		t.Fatalf("expected 5s timeout, got %v", hm.timeout)
	}
}

func TestHookManager_RegisterAndList(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{})

	h := NewCallbackHook("cb1", HookTypePreRotation, 10, func(ctx context.Context, e *RotationEvent) error {
		return nil
	})
	hm.RegisterHook(h)

	hooks := hm.ListHooks()
	if len(hooks[HookTypePreRotation]) != 1 {
		t.Fatal("expected 1 pre-rotation hook")
	}
	if hooks[HookTypePreRotation][0] != "cb1" {
		t.Fatal("wrong hook id")
	}
}

func TestHookManager_Unregister(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{})

	h := NewCallbackHook("unreg", HookTypePostRotation, 1, func(ctx context.Context, e *RotationEvent) error {
		return nil
	})
	hm.RegisterHook(h)
	hm.UnregisterHook("unreg")

	hooks := hm.ListHooks()
	if len(hooks[HookTypePostRotation]) != 0 {
		t.Fatal("expected 0 hooks after unregister")
	}
}

func TestHookManager_UnregisterNonexistent(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{})
	// Should not panic
	hm.UnregisterHook("does-not-exist")
}

func TestHookManager_ExecuteHooks_NoHooks(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{})
	results, err := hm.ExecuteHooks(context.Background(), HookTypePreRotation, &RotationEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Fatal("expected nil results for no hooks")
	}
}

func TestHookManager_ExecuteHooks_Success(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{})
	called := false
	h := NewCallbackHook("ok", HookTypePostRotation, 1, func(ctx context.Context, e *RotationEvent) error {
		called = true
		return nil
	})
	hm.RegisterHook(h)

	results, err := hm.ExecuteHooks(context.Background(), HookTypePostRotation, &RotationEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("hook not called")
	}
	if len(results) != 1 || !results[0].Success {
		t.Fatal("expected 1 successful result")
	}
}

func TestHookManager_ExecuteHooks_Failure(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{})
	h := NewCallbackHook("fail", HookTypePreRotation, 1, func(ctx context.Context, e *RotationEvent) error {
		return errors.New("boom")
	})
	hm.RegisterHook(h)

	results, err := hm.ExecuteHooks(context.Background(), HookTypePreRotation, &RotationEvent{})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(results) != 1 || results[0].Success {
		t.Fatal("expected 1 failed result")
	}
}

func TestHookManager_ExecuteHooks_Timeout(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{DefaultTimeout: 50 * time.Millisecond})
	h := NewCallbackHook("slow", HookTypeVerification, 1, func(ctx context.Context, e *RotationEvent) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})
	hm.RegisterHook(h)

	results, err := hm.ExecuteHooks(context.Background(), HookTypeVerification, &RotationEvent{})
	if !errors.Is(err, ErrHookTimeout) {
		t.Fatalf("expected ErrHookTimeout, got: %v", err)
	}
	if len(results) != 1 || results[0].Success {
		t.Fatal("expected failed result on timeout")
	}
}

func TestHookManager_PriorityOrder(t *testing.T) {
	hm := NewHookManager(HookManagerConfig{})
	order := make([]string, 0)

	hm.RegisterHook(NewCallbackHook("low", HookTypePostRotation, 1, func(ctx context.Context, e *RotationEvent) error {
		order = append(order, "low")
		return nil
	}))
	hm.RegisterHook(NewCallbackHook("high", HookTypePostRotation, 100, func(ctx context.Context, e *RotationEvent) error {
		order = append(order, "high")
		return nil
	}))

	hm.ExecuteHooks(context.Background(), HookTypePostRotation, &RotationEvent{})

	if len(order) != 2 || order[0] != "high" || order[1] != "low" {
		t.Fatalf("expected [high, low], got %v", order)
	}
}

// --- CallbackHook ---

func TestCallbackHook_Methods(t *testing.T) {
	h := NewCallbackHook("cb", HookTypeNotification, 5, func(ctx context.Context, e *RotationEvent) error {
		return nil
	})
	if h.ID() != "cb" {
		t.Fatal("wrong id")
	}
	if h.Type() != HookTypeNotification {
		t.Fatal("wrong type")
	}
	if h.Priority() != 5 {
		t.Fatal("wrong priority")
	}
}

// --- ServiceNotifyHook ---

func TestServiceNotifyHook_Success(t *testing.T) {
	notified := make([]string, 0)
	h := NewServiceNotifyHook("snh", HookTypePostRotation, 1, []string{"svc1", "svc2"},
		func(ctx context.Context, service string, event *RotationEvent) error {
			notified = append(notified, service)
			return nil
		},
	)

	result, err := h.Execute(context.Background(), &RotationEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if len(notified) != 2 {
		t.Fatalf("expected 2 notified, got %d", len(notified))
	}
}

func TestServiceNotifyHook_PartialFailure(t *testing.T) {
	h := NewServiceNotifyHook("snhf", HookTypePostRotation, 1, []string{"ok", "fail"},
		func(ctx context.Context, service string, event *RotationEvent) error {
			if service == "fail" {
				return errors.New("failed")
			}
			return nil
		},
	)

	result, err := h.Execute(context.Background(), &RotationEvent{})
	if err == nil {
		t.Fatal("expected error for partial failure")
	}
	if result.Success {
		t.Fatal("expected failure")
	}
}

// --- HTTPVerificationHook ---

func TestHTTPVerificationHook_Success(t *testing.T) {
	h := NewHTTPVerificationHook("httpv", 1, func(ctx context.Context, secret *secrets.Secret) error {
		return nil
	})

	result, err := h.Execute(context.Background(), &RotationEvent{
		NewSecret: staticSecret("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if h.Type() != HookTypeVerification {
		t.Fatal("wrong type")
	}
}

func TestHTTPVerificationHook_NoSecret(t *testing.T) {
	h := NewHTTPVerificationHook("httpv2", 1, func(ctx context.Context, secret *secrets.Secret) error {
		return nil
	})

	result, err := h.Execute(context.Background(), &RotationEvent{})
	if !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("expected ErrVerificationFailed, got: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure")
	}
}

func TestHTTPVerificationHook_VerifyFails(t *testing.T) {
	h := NewHTTPVerificationHook("httpv3", 1, func(ctx context.Context, secret *secrets.Secret) error {
		return errors.New("bad cert")
	})

	_, err := h.Execute(context.Background(), &RotationEvent{
		NewSecret: staticSecret("test"),
	})
	if !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("expected ErrVerificationFailed, got: %v", err)
	}
}

// --- RollbackManager ---

func TestRollbackManager_Defaults(t *testing.T) {
	rm := NewRollbackManager(RollbackManagerConfig{
		Store: store.NewMemoryStore(store.MemoryStoreConfig{}),
	})
	if rm.maxVersions != 5 {
		t.Fatalf("expected maxVersions 5, got %d", rm.maxVersions)
	}
}

func TestRollbackManager_SaveAndRollback(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm := NewRollbackManager(RollbackManagerConfig{
		Store:       ms,
		MaxVersions: 3,
	})

	s := staticSecret("secrets/rb")
	rm.SaveForRollback(s)

	history := rm.GetRollbackHistory("secrets/rb")
	if len(history) != 1 {
		t.Fatalf("expected 1, got %d", len(history))
	}

	// Store the current version so rollback can restore it
	ms.Set(context.Background(), "secrets/rb", staticSecret("secrets/rb"))

	rolled, err := rm.Rollback(context.Background(), "secrets/rb")
	if err != nil {
		t.Fatal(err)
	}
	if rolled == nil {
		t.Fatal("expected non-nil rolled back secret")
	}
}

func TestRollbackManager_SaveNil(t *testing.T) {
	rm := NewRollbackManager(RollbackManagerConfig{
		Store: store.NewMemoryStore(store.MemoryStoreConfig{}),
	})
	// Should not panic
	rm.SaveForRollback(nil)
}

func TestRollbackManager_RollbackNoHistory(t *testing.T) {
	rm := NewRollbackManager(RollbackManagerConfig{
		Store: store.NewMemoryStore(store.MemoryStoreConfig{}),
	})
	_, err := rm.Rollback(context.Background(), "nohistory")
	if err == nil {
		t.Fatal("expected error for no history")
	}
}

func TestRollbackManager_MaxVersions(t *testing.T) {
	rm := NewRollbackManager(RollbackManagerConfig{
		Store:       store.NewMemoryStore(store.MemoryStoreConfig{}),
		MaxVersions: 2,
	})

	for i := 0; i < 5; i++ {
		s := staticSecret("secrets/trim")
		s.Version = int64(i)
		rm.SaveForRollback(s)
	}

	history := rm.GetRollbackHistory("secrets/trim")
	if len(history) != 2 {
		t.Fatalf("expected 2 (max), got %d", len(history))
	}
}

func TestRollbackManager_ClearHistory(t *testing.T) {
	rm := NewRollbackManager(RollbackManagerConfig{
		Store: store.NewMemoryStore(store.MemoryStoreConfig{}),
	})
	rm.SaveForRollback(staticSecret("secrets/clear"))
	rm.ClearHistory("secrets/clear")

	history := rm.GetRollbackHistory("secrets/clear")
	if history != nil {
		t.Fatal("expected nil after clear")
	}
}

func TestRollbackManager_GetRollbackHistory_None(t *testing.T) {
	rm := NewRollbackManager(RollbackManagerConfig{
		Store: store.NewMemoryStore(store.MemoryStoreConfig{}),
	})
	history := rm.GetRollbackHistory("nope")
	if history != nil {
		t.Fatal("expected nil")
	}
}

// --- GracePeriodManager ---

func TestGracePeriodManager_StartAndCheck(t *testing.T) {
	gpm := &GracePeriodManager{
		activePeriods: make(map[string]*gracePeriod),
	}

	old := staticSecret("secrets/gp")
	newS := staticSecret("secrets/gp")
	newS.Version = 2

	gpm.StartGracePeriod("secrets/gp", old, newS, time.Hour)

	if !gpm.IsInGracePeriod("secrets/gp") {
		t.Fatal("expected in grace period")
	}

	oldR, newR, ok := gpm.GetGracePeriodSecrets("secrets/gp")
	if !ok {
		t.Fatal("expected ok")
	}
	if oldR == nil || newR == nil {
		t.Fatal("expected non-nil secrets")
	}
}

func TestGracePeriodManager_NotInGracePeriod(t *testing.T) {
	gpm := &GracePeriodManager{
		activePeriods: make(map[string]*gracePeriod),
	}

	if gpm.IsInGracePeriod("nope") {
		t.Fatal("expected not in grace period")
	}

	_, _, ok := gpm.GetGracePeriodSecrets("nope")
	if ok {
		t.Fatal("expected not ok")
	}
}

func TestGracePeriodManager_EndGracePeriod(t *testing.T) {
	gpm := &GracePeriodManager{
		activePeriods: make(map[string]*gracePeriod),
	}

	gpm.StartGracePeriod("secrets/end", staticSecret("x"), staticSecret("x"), time.Hour)
	gpm.EndGracePeriod("secrets/end")

	if gpm.IsInGracePeriod("secrets/end") {
		t.Fatal("should not be in grace period after end")
	}
}

func TestGracePeriodManager_Expired(t *testing.T) {
	gpm := &GracePeriodManager{
		activePeriods: make(map[string]*gracePeriod),
	}

	gpm.StartGracePeriod("secrets/exp", staticSecret("x"), staticSecret("x"), time.Nanosecond)
	time.Sleep(2 * time.Millisecond)

	if gpm.IsInGracePeriod("secrets/exp") {
		t.Fatal("should not be in grace period after expiry")
	}

	_, _, ok := gpm.GetGracePeriodSecrets("secrets/exp")
	if ok {
		t.Fatal("expected not ok after expiry")
	}
}
