// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import (
	"bytes"
	"errors"
	"testing"
)

type failWriter struct{ n int }

var errFull = errors.New("disk full")

func (f *failWriter) Write(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errFull
	}
	f.n--
	return len(p), nil
}

func TestWriteReportReturnsWriteError(t *testing.T) {
	if err := WriteReport(&failWriter{n: 1}, []string{"a", "b", "c"}); !errors.Is(err, errFull) {
		t.Fatalf("err = %v, want the write error", err)
	}
	var b bytes.Buffer
	if err := WriteReport(&b, []string{"a", "b"}); err != nil || b.String() != "a\nb\n" {
		t.Fatalf("happy path: %q %v", b.String(), err)
	}
}
