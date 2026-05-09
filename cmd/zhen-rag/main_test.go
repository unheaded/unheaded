// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"testing"
)

func TestSearchQuery_StripsTrailingPunctuation(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"hello":   "hello",
		"hello?":  "hello",
		"hello.":  "hello",
		"hello!":  "hello",
		"hi?!.":   "hi",
		"":        "",
		"...":     "",
		"a.b.c.":  "a.b.c",
	}
	for in, want := range cases {
		if got := searchQuery(in); got != want {
			t.Errorf("searchQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClipContent_NoOpsUnderLimit(t *testing.T) {
	t.Parallel()
	if got := clipContent("hello", 10); got != "hello" {
		t.Errorf("under-limit clip mutated: %q", got)
	}
}

func TestClipContent_NoOpsOnNonPositiveMax(t *testing.T) {
	t.Parallel()
	for _, max := range []int{0, -1, -100} {
		if got := clipContent("hello", max); got != "hello" {
			t.Errorf("max=%d should not clip; got %q", max, got)
		}
	}
}

func TestClipContent_TruncatesWithMarker(t *testing.T) {
	t.Parallel()
	want := "hell\n\n…[truncated]"
	if got := clipContent("hello world", 4); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEnvOr_Fallback(t *testing.T) {
	if got := envOr("ZHEN_RAG_NEVER_SET", "fb"); got != "fb" {
		t.Errorf("unset: got %q, want fb", got)
	}
}

func TestEnvOr_TrimsWhitespace(t *testing.T) {
	t.Setenv("ZHEN_RAG_TEST", "   ")
	if got := envOr("ZHEN_RAG_TEST", "fb"); got != "fb" {
		t.Errorf("whitespace-only should fall back, got %q", got)
	}
}

func TestEnvOr_UsesValue(t *testing.T) {
	t.Setenv("ZHEN_RAG_TEST", "real")
	if got := envOr("ZHEN_RAG_TEST", "fb"); got != "real" {
		t.Errorf("got %q, want real", got)
	}
}
