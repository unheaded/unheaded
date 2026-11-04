// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package journald

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// fakeJournal is a unixgram socket in a tempdir, standing in for journald.
type fakeJournal struct {
	path string
	conn *net.UnixConn
}

func newFakeJournal(t *testing.T) *fakeJournal {
	t.Helper()
	path := filepath.Join(t.TempDir(), "socket")
	f := &fakeJournal{path: path}
	f.listen(t)
	return f
}

func (f *fakeJournal) listen(t *testing.T) {
	t.Helper()
	c, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: f.path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	f.conn = c
	t.Cleanup(func() { _ = c.Close() })
}

func (f *fakeJournal) read(t *testing.T) []byte {
	t.Helper()
	if err := f.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1<<20)
	n, err := f.conn.Read(buf)
	if err != nil {
		t.Fatalf("no datagram: %v", err)
	}
	return buf[:n]
}

// decode reads the native protocol the way journald does.
func decode(t *testing.T, b []byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	for len(b) > 0 {
		nl := bytes.IndexByte(b, '\n')
		if nl < 0 {
			t.Fatalf("no newline in %q", b)
		}
		line := b[:nl]
		if eq := bytes.IndexByte(line, '='); eq >= 0 {
			out[string(line[:eq])] = string(line[eq+1:])
			b = b[nl+1:]
			continue
		}
		rest := b[nl+1:]
		if len(rest) < 8 {
			t.Fatalf("short length after %q", line)
		}
		n := binary.LittleEndian.Uint64(rest)
		rest = rest[8:]
		if uint64(len(rest)) < n+1 || rest[n] != '\n' {
			t.Fatalf("bad binary field %q", line)
		}
		out[string(line)] = string(rest[:n])
		b = rest[n+1:]
	}
	return out
}

func TestSendPlainFields(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	if err := w.Send(map[string]string{"PRIORITY": "6", "MESSAGE": "hello", "EMPTY": ""}); err != nil {
		t.Fatal(err)
	}
	got := j.read(t)
	if want := "EMPTY=\nMESSAGE=hello\nPRIORITY=6\n"; string(got) != want {
		t.Errorf("got %q, want %q (sorted, so a datagram is reproducible)", got, want)
	}
}

func TestSendNewlineUsesBinaryLengthForm(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	if err := w.Send(map[string]string{"MESSAGE": "one\ntwo"}); err != nil {
		t.Fatal(err)
	}
	got := j.read(t)
	want := append([]byte("MESSAGE\n"), 7, 0, 0, 0, 0, 0, 0, 0)
	want = append(want, "one\ntwo\n"...)
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A newline in a value is the only way to forge a second field; the length
// form has to carry it as data.
func TestValueCannotForgeAField(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	evil := "x\nPRIORITY=0\n_SYSTEMD_UNIT=sshd.service"
	if err := w.Send(map[string]string{"MESSAGE": evil, "PRIORITY": "6"}); err != nil {
		t.Fatal(err)
	}
	got := decode(t, j.read(t))
	if got["MESSAGE"] != evil || got["PRIORITY"] != "6" || len(got) != 2 {
		t.Errorf("got %q", got)
	}
}

func TestNULAndHighBytesSurvive(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	v := "a\x00b\xff"
	if err := w.Send(map[string]string{"MESSAGE": v}); err != nil {
		t.Fatal(err)
	}
	if got := decode(t, j.read(t))["MESSAGE"]; got != v {
		t.Errorf("got %q", got)
	}
}

func TestFieldNames(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	for _, name := range []string{"message", "Message", "MES SAGE", "MES-SAGE", "MES=SAGE", "MES\nSAGE", "", "9LIVES", "MÉSSAGE", strings.Repeat("A", 65)} {
		if err := w.Send(map[string]string{name: "x"}); !errors.Is(err, ErrFieldName) {
			t.Errorf("%q: err = %v, want ErrFieldName", name, err)
		}
	}
	for _, name := range []string{"A", "A1_B", strings.Repeat("A", 64)} {
		if err := w.Send(map[string]string{name: "x"}); err != nil {
			t.Errorf("%q: %v", name, err)
		}
		j.read(t)
	}
}

// _-prefixed fields are journald's own. Nothing we write may claim one.
func TestTrustedFieldsCannotBeSet(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	for _, name := range []string{"_PID", "_UID", "_SYSTEMD_UNIT", "_HOSTNAME", "__CURSOR", "_"} {
		if err := w.Send(map[string]string{"MESSAGE": "x", name: "1"}); !errors.Is(err, ErrFieldName) {
			t.Errorf("%q: err = %v, want ErrFieldName", name, err)
		}
	}
	if err := j.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if n, _ := j.conn.Read(make([]byte, 64)); n != 0 {
		t.Error("a rejected message reached the socket")
	}
}

func TestNoFieldsIsAnError(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	if err := w.Send(nil); !errors.Is(err, ErrNoFields) {
		t.Errorf("err = %v", err)
	}
}

func TestOversizeIsTruncatedAndFlagged(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	w.MaxDatagram = 1024
	if err := w.Send(map[string]string{"MESSAGE": strings.Repeat("A", 5000), "PRIORITY": "6"}); err != nil {
		t.Fatal(err)
	}
	raw := j.read(t)
	if len(raw) > 1024 {
		t.Fatalf("datagram is %d bytes, limit 1024", len(raw))
	}
	got := decode(t, raw)
	if got["UNHEADED_TRUNCATED"] != "1" || got["PRIORITY"] != "6" {
		t.Errorf("got %q", got)
	}
	if len(got["MESSAGE"]) < 900 || strings.Trim(got["MESSAGE"], "A") != "" {
		t.Errorf("MESSAGE is %d bytes", len(got["MESSAGE"]))
	}
}

func TestTruncationWithNewlinesAndRunes(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	w.MaxDatagram = 256
	if err := w.Send(map[string]string{"MESSAGE": strings.Repeat("ö\n", 500)}); err != nil {
		t.Fatal(err)
	}
	raw := j.read(t)
	if len(raw) > 256 {
		t.Fatalf("datagram is %d bytes", len(raw))
	}
	got := decode(t, raw)
	if !utf8.ValidString(got["MESSAGE"]) {
		t.Errorf("cut inside a rune: %q", got["MESSAGE"])
	}
	if got["UNHEADED_TRUNCATED"] != "1" {
		t.Errorf("got %q", got)
	}
}

// Only MESSAGE is cut. If everything else is already over, that is an error,
// not a silently mangled entry.
func TestOversizeWithoutMessageIsAnError(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	w.MaxDatagram = 64
	if err := w.Send(map[string]string{"OTHER": strings.Repeat("A", 500)}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
	if err := w.Send(map[string]string{"MESSAGE": "x", "OTHER": strings.Repeat("A", 500)}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestMissingSocketIsAnError(t *testing.T) {
	w := New(filepath.Join(t.TempDir(), "absent"))
	defer w.Close()
	if err := w.Send(map[string]string{"MESSAGE": "x"}); err == nil {
		t.Error("no error")
	}
}

// A journald restart replaces the peer; the next Send finds it again.
func TestSendRedialsAfterTheSocketComesBack(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	if err := w.Send(map[string]string{"MESSAGE": "first"}); err != nil {
		t.Fatal(err)
	}
	j.read(t)
	if err := j.conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(j.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	j.listen(t)
	if err := w.Send(map[string]string{"MESSAGE": "second"}); err != nil {
		t.Fatal(err)
	}
	if got := decode(t, j.read(t))["MESSAGE"]; got != "second" {
		t.Errorf("got %q", got)
	}
}

// A stalled journald must not hang the caller: a blocked read loop stops
// reading the network and takes everything else down with it.
func TestStalledJournalReturnsAnError(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	w.Timeout = 50 * time.Millisecond
	msg := map[string]string{"MESSAGE": strings.Repeat("A", 4096)}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := w.Send(msg); err != nil {
			return
		}
	}
	t.Fatal("nobody reads the socket, and Send kept succeeding")
}

func TestConcurrentSend(t *testing.T) {
	j := newFakeJournal(t)
	w := New(j.path)
	defer w.Close()
	const n = 64
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Send(map[string]string{"MESSAGE": "x"}); err != nil {
				t.Error(err)
			}
		}()
	}
	for range n {
		if got := decode(t, j.read(t)); got["MESSAGE"] != "x" {
			t.Errorf("got %q", got)
		}
	}
	wg.Wait()
}

func TestCloseIsSafeWithoutASend(t *testing.T) {
	w := New("/nonexistent")
	if err := w.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
