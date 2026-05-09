// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"strings"
	"testing"
)

func TestSearchQuery_StripsTrailingPunctuation(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"what is unheaded":   "what is unheaded",
		"what is unheaded?":  "what is unheaded",
		"what is unheaded.":  "what is unheaded",
		"what is unheaded!":  "what is unheaded",
		"why??!.":            "why",
		"":                   "",
		"trailing.spaces.  ": "trailing.spaces.  ", // spaces aren't stripped
	}
	for in, want := range cases {
		if got := searchQuery(in); got != want {
			t.Errorf("searchQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClipContent_ReturnsAsIsWhenUnderLimit(t *testing.T) {
	t.Parallel()
	if got := clipContent("hello", 100); got != "hello" {
		t.Errorf("under-limit: got %q, want hello", got)
	}
}

func TestClipContent_ReturnsAsIsWhenLimitNonPositive(t *testing.T) {
	t.Parallel()
	for _, max := range []int{0, -1, -100} {
		if got := clipContent("hello", max); got != "hello" {
			t.Errorf("max=%d: got %q, want hello (no clip)", max, got)
		}
	}
}

func TestClipContent_TruncatesWithMarker(t *testing.T) {
	t.Parallel()
	got := clipContent("0123456789", 4)
	want := "0123\n\n…[truncated]"
	if got != want {
		t.Errorf("clipContent(\"0123456789\", 4) = %q, want %q", got, want)
	}
}

func TestEnvOr_FallsBackOnEmptyOrUnset(t *testing.T) {
	t.Setenv("ZHEN_TEST_VAR", "")
	if got := envOr("ZHEN_TEST_VAR", "fb"); got != "fb" {
		t.Errorf("empty env: got %q, want fb", got)
	}
	if got := envOr("ZHEN_NEVER_SET_VAR", "fb"); got != "fb" {
		t.Errorf("unset env: got %q, want fb", got)
	}
}

func TestEnvOr_TrimsWhitespace(t *testing.T) {
	t.Setenv("ZHEN_TEST_VAR2", "   ")
	if got := envOr("ZHEN_TEST_VAR2", "fb"); got != "fb" {
		t.Errorf("whitespace-only env should fall back; got %q", got)
	}
}

func TestEnvOr_UsesEnvWhenSet(t *testing.T) {
	t.Setenv("ZHEN_TEST_VAR3", "real-value")
	if got := envOr("ZHEN_TEST_VAR3", "fb"); got != "real-value" {
		t.Errorf("got %q, want real-value", got)
	}
}

func TestOneLine_FlattensNewlinesAndCaps160(t *testing.T) {
	t.Parallel()
	if got := oneLine("a\nb\nc"); got != "a b c" {
		t.Errorf("newline flatten: got %q", got)
	}
	long := strings.Repeat("x", 200)
	got := oneLine(long)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix on long input, got %q", got[len(got)-10:])
	}
	// 160 chars + ellipsis (3 bytes utf-8). Length check via rune count.
	if len([]rune(got)) != 161 {
		t.Errorf("got rune-len %d, want 161 (160 + ellipsis)", len([]rune(got)))
	}
}

func TestOneLine_LeavesShortStringsAlone(t *testing.T) {
	t.Parallel()
	if got := oneLine("short"); got != "short" {
		t.Errorf("got %q, want short", got)
	}
}

func TestBriefArgs_EmptyMap(t *testing.T) {
	t.Parallel()
	if got := briefArgs(map[string]any{}); got != "{}" {
		t.Errorf("got %q, want {}", got)
	}
	if got := briefArgs(nil); got != "{}" {
		t.Errorf("nil: got %q, want {}", got)
	}
}

func TestBriefArgs_ClipsLongStrings(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 200)
	got := briefArgs(map[string]any{"q": long})
	if !strings.Contains(got, "…") {
		t.Errorf("expected ellipsis in clipped output, got %q", got)
	}
	if strings.Contains(got, long) {
		t.Errorf("output should not contain unclipped 200-char string, got %q", got)
	}
}

func TestBriefArgs_PreservesShortValues(t *testing.T) {
	t.Parallel()
	got := briefArgs(map[string]any{"k": "short", "n": 42})
	if !strings.Contains(got, `"k":"short"`) {
		t.Errorf("got %q, want to contain k:short", got)
	}
	if !strings.Contains(got, `"n":42`) {
		t.Errorf("got %q, want to contain n:42", got)
	}
}
