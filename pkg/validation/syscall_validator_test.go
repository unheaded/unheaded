package validation

import (
	"testing"
	"time"
)

func TestValidateSyscallNumber_ValidSyscalls(t *testing.T) {
	// D5-003: Only known syscalls (0x40-0x43) should be accepted
	validSyscalls := []uint32{SYS_DRAW_FRAME, SYS_GET_TICKS, SYS_SLEEP, SYS_GET_KEY}
	for _, num := range validSyscalls {
		if !ValidateSyscallNumber(num) {
			t.Errorf("ValidateSyscallNumber(0x%02x) = false, want true", num)
		}
	}
}

func TestValidateSyscallNumber_InvalidSyscalls(t *testing.T) {
	// D5-003: Default-deny for unknown syscall numbers
	invalidSyscalls := []uint32{
		0x00, 0x01, 0x3F, // Below range
		0x44, 0x45, 0xFF, // Above range
		0x100, 0xFFFF, 0xFFFFFFFF, // Way out of range
	}
	for _, num := range invalidSyscalls {
		if ValidateSyscallNumber(num) {
			t.Errorf("ValidateSyscallNumber(0x%02x) = true, want false", num)
		}
	}
}

func TestSyscallName(t *testing.T) {
	tests := []struct {
		num  uint32
		want string
	}{
		{SYS_DRAW_FRAME, "SYS_DRAW_FRAME"},
		{SYS_GET_TICKS, "SYS_GET_TICKS"},
		{SYS_SLEEP, "SYS_SLEEP"},
		{SYS_GET_KEY, "SYS_GET_KEY"},
		{0xFF, "UNKNOWN(0xff)"},
	}
	for _, tt := range tests {
		if got := SyscallName(tt.num); got != tt.want {
			t.Errorf("SyscallName(0x%02x) = %q, want %q", tt.num, got, tt.want)
		}
	}
}

func TestSyscallValidator_ValidMessage(t *testing.T) {
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{"doom-engine", "trace-collector"},
		MaxRate:        100,
		RateWindow:     time.Second,
	})

	msg := SyscallMessage{
		Source:        "doom-engine",
		SyscallNumber: SYS_DRAW_FRAME,
		Timestamp:     time.Now(),
	}

	if err := v.Validate(msg); err != nil {
		t.Errorf("valid message rejected: %v", err)
	}

	accepted, rejected := v.Stats()
	if accepted != 1 || rejected != 0 {
		t.Errorf("stats: accepted=%d, rejected=%d; want 1, 0", accepted, rejected)
	}
}

func TestSyscallValidator_UnknownSource(t *testing.T) {
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{"doom-engine"},
	})

	msg := SyscallMessage{
		Source:        "evil-attacker",
		SyscallNumber: SYS_DRAW_FRAME,
		Timestamp:     time.Now(),
	}

	err := v.Validate(msg)
	if err == nil {
		t.Fatal("unknown source should be rejected")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if ve.Code != 403 {
		t.Errorf("error code = %d, want 403", ve.Code)
	}
	if ve.Field != "source" {
		t.Errorf("error field = %q, want %q", ve.Field, "source")
	}
}

func TestSyscallValidator_InvalidSyscallNumber(t *testing.T) {
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{"doom-engine"},
	})

	msg := SyscallMessage{
		Source:        "doom-engine",
		SyscallNumber: 0xFF, // Invalid
		Timestamp:     time.Now(),
	}

	err := v.Validate(msg)
	if err == nil {
		t.Fatal("invalid syscall number should be rejected")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if ve.Code != 400 {
		t.Errorf("error code = %d, want 400", ve.Code)
	}
	if ve.Field != "syscall_number" {
		t.Errorf("error field = %q, want %q", ve.Field, "syscall_number")
	}
}

func TestSyscallValidator_RateLimit(t *testing.T) {
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{"doom-engine"},
		MaxRate:        5,
		RateWindow:     time.Second,
	})

	msg := SyscallMessage{
		Source:        "doom-engine",
		SyscallNumber: SYS_GET_TICKS,
		Timestamp:     time.Now(),
	}

	// First 5 should succeed
	for i := 0; i < 5; i++ {
		if err := v.Validate(msg); err != nil {
			t.Fatalf("message %d should succeed: %v", i+1, err)
		}
	}

	// 6th should be rate limited
	err := v.Validate(msg)
	if err == nil {
		t.Fatal("6th message should be rate limited")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if ve.Code != 429 {
		t.Errorf("error code = %d, want 429", ve.Code)
	}
}

func TestSyscallValidator_RateLimitWindowReset(t *testing.T) {
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{"doom-engine"},
		MaxRate:        2,
		RateWindow:     50 * time.Millisecond,
	})

	msg := SyscallMessage{
		Source:        "doom-engine",
		SyscallNumber: SYS_GET_TICKS,
		Timestamp:     time.Now(),
	}

	// Use up the rate limit
	_ = v.Validate(msg)
	_ = v.Validate(msg)

	// Should be rate limited now
	err := v.Validate(msg)
	if err == nil {
		t.Fatal("should be rate limited")
	}

	// Wait for window to reset
	time.Sleep(60 * time.Millisecond)

	// Should succeed again
	if err := v.Validate(msg); err != nil {
		t.Errorf("after window reset, should succeed: %v", err)
	}
}

func TestSyscallValidator_MemoryBoundsCheck(t *testing.T) {
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{"doom-engine"},
	})

	// Valid memory range
	msg := SyscallMessage{
		Source:        "doom-engine",
		SyscallNumber: SYS_DRAW_FRAME,
		MemoryAddr:    0x100000, // Framebuffer base
		MemorySize:    320 * 200,
		Timestamp:     time.Now(),
	}

	if err := v.Validate(msg); err != nil {
		t.Errorf("valid memory range rejected: %v", err)
	}

	// Invalid: overflows max address
	msg.MemoryAddr = DefaultMaxMemoryAddr - 10
	msg.MemorySize = 100 // addr + size > max

	err := v.Validate(msg)
	if err == nil {
		t.Fatal("out-of-bounds memory should be rejected")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if ve.Code != 400 {
		t.Errorf("error code = %d, want 400", ve.Code)
	}
}

func TestSyscallValidator_EmptySourceList(t *testing.T) {
	// Empty allowed sources means ALL sources are rejected
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{},
	})

	msg := SyscallMessage{
		Source:        "doom-engine",
		SyscallNumber: SYS_DRAW_FRAME,
		Timestamp:     time.Now(),
	}

	err := v.Validate(msg)
	if err == nil {
		t.Fatal("empty source list should reject all")
	}
}

func TestSyscallValidator_Stats(t *testing.T) {
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{"good-service"},
	})

	// 2 valid, 1 rejected (bad source), 1 rejected (bad syscall)
	_ = v.Validate(SyscallMessage{Source: "good-service", SyscallNumber: SYS_DRAW_FRAME, Timestamp: time.Now()})
	_ = v.Validate(SyscallMessage{Source: "good-service", SyscallNumber: SYS_GET_TICKS, Timestamp: time.Now()})
	_ = v.Validate(SyscallMessage{Source: "bad-source", SyscallNumber: SYS_DRAW_FRAME, Timestamp: time.Now()})
	_ = v.Validate(SyscallMessage{Source: "good-service", SyscallNumber: 0xFF, Timestamp: time.Now()})

	accepted, rejected := v.Stats()
	if accepted != 2 {
		t.Errorf("accepted = %d, want 2", accepted)
	}
	if rejected != 2 {
		t.Errorf("rejected = %d, want 2", rejected)
	}
}

func TestSyscallValidator_ResetStats(t *testing.T) {
	v := NewSyscallValidator(SyscallValidatorConfig{
		AllowedSources: []string{"svc"},
	})

	_ = v.Validate(SyscallMessage{Source: "svc", SyscallNumber: SYS_DRAW_FRAME, Timestamp: time.Now()})
	_ = v.Validate(SyscallMessage{Source: "bad", SyscallNumber: SYS_DRAW_FRAME, Timestamp: time.Now()})

	v.ResetStats()
	accepted, rejected := v.Stats()
	if accepted != 0 || rejected != 0 {
		t.Errorf("after reset: accepted=%d, rejected=%d; want 0, 0", accepted, rejected)
	}
}

func TestValidationError_Error(t *testing.T) {
	ve := &ValidationError{
		Code:    403,
		Field:   "source",
		Message: "unknown source",
	}
	s := ve.Error()
	if s != "[403] source: unknown source" {
		t.Errorf("Error() = %q, want %q", s, "[403] source: unknown source")
	}
}
