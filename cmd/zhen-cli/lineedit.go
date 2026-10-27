// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

// ADR-059 Phase 3: line editing with history, no readline dependency.
// lineEditor reads the bytes a raw-mode terminal sends and keeps the line
// on screen by redrawing it: left/right, home/end (also Ctrl-A/Ctrl-E),
// backspace, delete, Ctrl-U, up/down through this session's history.
// Ctrl-C returns errInterrupt, Ctrl-D on an empty line io.EOF. Widths
// are one column per rune; a line longer than the terminal redraws badly.

var errInterrupt = errors.New("interrupt")

const (
	defaultMaxHistory = 500
	maxLineRunes      = 1 << 20 // pasted code; the old Scanner capped at 1 MiB
)

type lineEditor struct {
	in         *bufio.Reader
	out        io.Writer
	history    []string
	maxHistory int
}

func (e *lineEditor) readLine(prompt string) (string, error) {
	var buf, draft []rune
	pos, hpos := 0, len(e.history)
	e.redraw(prompt, buf, pos)
	for {
		r, _, err := e.in.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf) > 0 {
				fmt.Fprint(e.out, "\r\n")
				return e.remember(string(buf)), nil
			}
			return "", err
		}
		switch r {
		case '\r', '\n':
			fmt.Fprint(e.out, "\r\n")
			return e.remember(string(buf)), nil
		case 0x03: // Ctrl-C
			fmt.Fprint(e.out, "^C\r\n")
			return "", errInterrupt
		case 0x04: // Ctrl-D
			if len(buf) == 0 {
				return "", io.EOF
			}
		case 0x7f, 0x08: // backspace
			if pos > 0 {
				buf = append(buf[:pos-1], buf[pos:]...)
				pos--
			}
		case 0x01: // Ctrl-A
			pos = 0
		case 0x05: // Ctrl-E
			pos = len(buf)
		case 0x15: // Ctrl-U
			buf = append([]rune(nil), buf[pos:]...)
			pos = 0
		case 0x1b:
			switch e.escape() {
			case keyLeft:
				if pos > 0 {
					pos--
				}
			case keyRight:
				if pos < len(buf) {
					pos++
				}
			case keyHome:
				pos = 0
			case keyEnd:
				pos = len(buf)
			case keyDelete:
				if pos < len(buf) {
					buf = append(buf[:pos], buf[pos+1:]...)
				}
			case keyUp:
				if hpos > 0 {
					if hpos == len(e.history) {
						draft = buf
					}
					hpos--
					buf = []rune(e.history[hpos])
					pos = len(buf)
				}
			case keyDown:
				if hpos < len(e.history) {
					hpos++
					if hpos == len(e.history) {
						buf = draft
					} else {
						buf = []rune(e.history[hpos])
					}
					pos = len(buf)
				}
			}
		default:
			if r < 0x20 || r == utf8.RuneError || len(buf) >= maxLineRunes {
				continue
			}
			buf = append(buf, 0)
			copy(buf[pos+1:], buf[pos:])
			buf[pos] = r
			pos++
		}
		e.redraw(prompt, buf, pos)
	}
}

type key int

const (
	keyNone key = iota
	keyUp
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyDelete
)

// escape reads the rest of an escape sequence (CSI "ESC [", SS3 "ESC O")
// and names the key; anything else is consumed and ignored.
func (e *lineEditor) escape() key {
	r, _, err := e.in.ReadRune()
	if err != nil {
		return keyNone
	}
	switch r {
	case 'O':
		r, _, err = e.in.ReadRune()
		if err != nil {
			return keyNone
		}
		return finalKey(r, "")
	case '[':
		var param strings.Builder
		for {
			r, _, err = e.in.ReadRune()
			if err != nil {
				return keyNone
			}
			if r >= 0x40 && r <= 0x7e {
				return finalKey(r, param.String())
			}
			if param.Len() > 16 { // not a key we know
				return keyNone
			}
			param.WriteRune(r)
		}
	}
	return keyNone
}

func finalKey(final rune, param string) key {
	switch final {
	case 'A':
		return keyUp
	case 'B':
		return keyDown
	case 'C':
		return keyRight
	case 'D':
		return keyLeft
	case 'H':
		return keyHome
	case 'F':
		return keyEnd
	case '~':
		switch param {
		case "1", "7":
			return keyHome
		case "4", "8":
			return keyEnd
		case "3":
			return keyDelete
		}
	}
	return keyNone
}

func (e *lineEditor) redraw(prompt string, buf []rune, pos int) {
	fmt.Fprintf(e.out, "\r%s%s\x1b[K", prompt, string(buf))
	if back := len(buf) - pos; back > 0 {
		fmt.Fprintf(e.out, "\x1b[%dD", back)
	}
}

// remember adds a non-blank line to the history unless it repeats the
// newest entry, keeping the newest maxHistory.
func (e *lineEditor) remember(line string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	if n := len(e.history); n > 0 && e.history[n-1] == line {
		return line
	}
	e.history = append(e.history, line)
	if e.maxHistory > 0 && len(e.history) > e.maxHistory {
		e.history = e.history[len(e.history)-e.maxHistory:]
	}
	return line
}

// plainLineReader is the reader for a stdin that is not a terminal, or one
// raw mode cannot be set on: the prompt, then one line per call.
func plainLineReader(in io.Reader, out io.Writer) func(string) (string, error) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MiB line cap for pasted code
	return func(prompt string) (string, error) {
		fmt.Fprint(out, prompt)
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return "", err
			}
			return "", io.EOF
		}
		return sc.Text(), nil
	}
}

var (
	termMu      sync.Mutex
	termRestore func()
)

// restoreTerminal puts the terminal back as it was if a line is being
// edited (the signal handler calls it before exiting).
func restoreTerminal() {
	termMu.Lock()
	defer termMu.Unlock()
	if termRestore != nil {
		termRestore()
		termRestore = nil
	}
}

// newLineReader returns the editor over a terminal stdin, raw only while
// a line is read (command output and the wait for an answer run in the
// terminal's normal mode), or plainLineReader when raw mode is refused.
func newLineReader(in, out *os.File) func(string) (string, error) {
	fd := int(in.Fd()) // #nosec G115 -- a file descriptor fits in int
	probe, err := makeRaw(fd)
	if err != nil {
		return plainLineReader(in, out)
	}
	probe()
	ed := &lineEditor{in: bufio.NewReader(in), out: out, maxHistory: defaultMaxHistory}
	return func(prompt string) (string, error) {
		restore, err := makeRaw(fd)
		if err != nil {
			return "", err
		}
		termMu.Lock()
		termRestore = restore
		termMu.Unlock()
		defer restoreTerminal()
		return ed.readLine(prompt)
	}
}
