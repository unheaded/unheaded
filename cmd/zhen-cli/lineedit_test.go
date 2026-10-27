// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// ADR-059 Phase 3 (S-CHAMPION-HARDENING Step 213): line editing and history
// without a readline dependency. The editor works on the bytes a raw-mode
// terminal sends; raw mode itself is lineedit_linux.go's job.
func TestLineEditor_Keys(t *testing.T) {
	hist := []string{"first", "second"}
	cases := []struct {
		name, in string
		history  []string
		want     string
		wantErr  error
	}{
		{"enter", "hello\r", nil, "hello", nil},
		{"newline enter", "hello\n", nil, "hello", nil},
		{"left then insert", "helo\x1b[Dl\r", nil, "hello", nil},
		{"right after left", "ab\x1b[D\x1b[Cc\r", nil, "abc", nil},
		{"backspace", "abc\x7f\x7fd\r", nil, "ad", nil},
		{"ctrl-h backspace", "abc\bd\r", nil, "abd", nil},
		{"backspace at start", "\x7fa\r", nil, "a", nil},
		{"up recalls newest", "\x1b[A\r", hist, "second", nil},
		{"up up", "\x1b[A\x1b[A\r", hist, "first", nil},
		{"up past oldest stays", "\x1b[A\x1b[A\x1b[A\r", hist, "first", nil},
		{"up up down", "\x1b[A\x1b[A\x1b[B\r", hist, "second", nil},
		{"down returns the draft", "dr\x1b[A\x1b[B\r", hist, "dr", nil},
		{"home", "xy\x1b[H0\r", nil, "0xy", nil},
		{"home tilde", "xy\x1b[1~0\r", nil, "0xy", nil},
		{"home ss3", "xy\x1bOH0\r", nil, "0xy", nil},
		{"end", "xy\x1b[H\x1b[F1\r", nil, "xy1", nil},
		{"end tilde", "xy\x1b[H\x1b[4~1\r", nil, "xy1", nil},
		{"ctrl-a ctrl-e", "x\x01a\x05b\r", nil, "axb", nil},
		{"delete at cursor", "abc\x1b[D\x1b[3~\r", nil, "ab", nil},
		{"ctrl-u kills to start", "abc\x15z\r", nil, "z", nil},
		{"utf-8 rune backspace", "é\x7fe\r", nil, "e", nil},
		{"utf-8 insert", "ca\x1b[Dé\r", nil, "céa", nil},
		{"unknown escape ignored", "a\x1b[Zb\r", nil, "ab", nil},
		{"other control ignored", "a\x07b\r", nil, "ab", nil},
		{"ctrl-d on empty is EOF", "\x04", nil, "", io.EOF},
		{"ctrl-d with text is ignored", "ab\x04c\r", nil, "abc", nil},
		{"ctrl-c", "ab\x03", nil, "", errInterrupt},
		{"eof mid-line returns the line", "abc", nil, "abc", nil},
		{"eof on empty", "", nil, "", io.EOF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			e := &lineEditor{in: bufio.NewReader(strings.NewReader(tc.in)), out: &out,
				history: append([]string(nil), tc.history...), maxHistory: 100}
			got, err := e.readLine("> ")
			if !errors.Is(err, tc.wantErr) || got != tc.want {
				t.Fatalf("readLine(%q) = %q, %v; want %q, %v", tc.in, got, err, tc.want, tc.wantErr)
			}
			if tc.wantErr == nil && !strings.Contains(out.String(), "> ") {
				t.Errorf("prompt not drawn: %q", out.String())
			}
		})
	}
}

func TestLineEditor_History(t *testing.T) {
	var out bytes.Buffer
	e := &lineEditor{in: bufio.NewReader(strings.NewReader("one\rone\r\rtwo\rthree\r\x1b[A\x1b[A\r")), out: &out, maxHistory: 2}
	var got []string
	for {
		line, err := e.readLine("> ")
		if err != nil {
			break
		}
		got = append(got, line)
	}
	if strings.Join(got, "|") != "one|one||two|three|two" {
		t.Fatalf("lines %q", got)
	}
	// Blank lines and repeats are not kept; the cap keeps the newest.
	if strings.Join(e.history, "|") != "three|two" && strings.Join(e.history, "|") != "two|three" {
		t.Fatalf("history %q", e.history)
	}
	if e.history[len(e.history)-1] != "two" {
		t.Errorf("newest entry %q, want the recalled two", e.history[len(e.history)-1])
	}
}

// Not a terminal: plain lines, no escape handling, no prompt redraws.
func TestLineReader_PlainFallback(t *testing.T) {
	var out bytes.Buffer
	read := plainLineReader(strings.NewReader("a b\nc\n"), &out)
	l1, err1 := read("> ")
	l2, err2 := read("> ")
	_, err3 := read("> ")
	if l1 != "a b" || l2 != "c" || err1 != nil || err2 != nil || !errors.Is(err3, io.EOF) {
		t.Fatalf("got %q %v, %q %v, %v", l1, err1, l2, err2, err3)
	}
}
