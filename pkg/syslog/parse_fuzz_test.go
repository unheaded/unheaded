// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package syslog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"unicode/utf8"
)

// The corpus and its invariants are registered with LICH as
// tomb/lich/LICH-019-syslog-relay.
var parseSeeds = []string{
	"<34>1 2003-10-11T22:14:15.003Z mymachine.example.com su - ID47 - \xEF\xBB\xBF'su root' failed",
	`<165>1 2003-10-11T22:14:15.003Z h evntslog - ID47 [exampleSDID@32473 iut="3"][examplePriority@32473 class="high"] msg`,
	`<13>1 - - - - - [a@1 x="b\]c\"d\\"] msg`,
	"<34>Oct 11 22:14:15 mymachine su: 'su root' failed",
	"<134>Nov  4 01:02:03 pihole dnsmasq[123]: query[A] example.org",
	"Oct 11 22:14:15 su: no pri",
	"plain words",
	"<192>out of range",
	"<34>1 2003-10-11T22:14:15.003Z cut",
	"<13>1 - - - - - [unterminated",
	"<13>1 - - a - - - nul\x00in\xffthe\x1bmiddle\u2028\u202e",
	"<>", "<", "\x00", "\n", "<13>", "<13>1 ", "<13>1",
}

func FuzzParse(f *testing.F) {
	for _, s := range parseSeeds {
		f.Add([]byte(s), 0)
		f.Add([]byte(s), 16)
	}
	f.Fuzz(func(t *testing.T, in []byte, max int) {
		limit := max
		if limit <= 0 || limit > 1<<16 {
			limit = DefaultMaxMessage
		}
		m, err := ParseMax(in, limit)
		if err != nil && !errors.Is(err, ErrEmpty) && !errors.Is(err, ErrPRI) && !errors.Is(err, ErrHeader) && !errors.Is(err, ErrStructuredData) {
			t.Fatalf("unknown error %v", err)
		}
		if err != nil && !errors.Is(err, ErrEmpty) && m.Format != Unparsed {
			t.Errorf("error %v with Format %d", err, m.Format)
		}
		if m.Facility < 0 || m.Facility > 23 || m.Severity < 0 || m.Severity > 7 {
			t.Errorf("facility %d severity %d", m.Facility, m.Severity)
		}
		if len(m.Text) > limit || len(m.StructuredData) > limit {
			t.Errorf("Text %d, StructuredData %d, cap %d", len(m.Text), len(m.StructuredData), limit)
		}
		if len(m.Timestamp) > MaxTimestamp || len(m.Hostname) > MaxHostname || len(m.AppName) > MaxAppName || len(m.ProcID) > MaxProcID || len(m.MsgID) > MaxMsgID {
			t.Errorf("uncapped header field: %+v", m)
		}
		if len(in) > limit && !m.Truncated && err == nil {
			t.Errorf("%d bytes in, cap %d, Truncated not set", len(in), limit)
		}
		for _, v := range []string{m.Text, m.Timestamp, m.Hostname, m.AppName, m.ProcID, m.MsgID, m.StructuredData} {
			if err := checkClean(v); err != nil {
				t.Errorf("%v in %q", err, v)
			}
		}
	})
}

func FuzzFrame(f *testing.F) {
	for _, s := range []string{
		"5 hello3 abc", "<13>one\n<13>two\r\n", "\n\r\n<13>x", "65 x", "1048576 x", "99999999999999999999 x",
		"5 hello world\n", "0 x", "05 x", "5hello", "abc\n", "<13>no trailer", "10 short", "12",
	} {
		f.Add([]byte(s), 64)
	}
	f.Fuzz(func(t *testing.T, in []byte, max int) {
		if max <= 0 || max > 1<<16 {
			max = 64
		}
		src := bytes.NewReader(in)
		r := bufio.NewReaderSize(src, 16)
		for frames := 0; ; frames++ {
			got, err := Frame(r, max)
			if err != nil {
				if len(got) != 0 {
					t.Fatalf("error %v with %d bytes", err, len(got))
				}
				if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, ErrFrameTooLarge) && !errors.Is(err, ErrBadFrame) {
					t.Fatalf("unknown error %v", err)
				}
				return
			}
			if len(got) > max {
				t.Fatalf("frame of %d bytes, cap %d", len(got), max)
			}
			if frames > len(in) {
				t.Fatalf("%d frames from %d bytes: not consuming", frames, len(in))
			}
			// What Frame hands out is what the relay parses next.
			m, _ := ParseMax(got, max)
			if err := checkClean(m.Text); err != nil {
				t.Fatalf("%v in %q", err, m.Text)
			}
		}
	})
}

// Any payload survives octet counting byte for byte.
func FuzzFrameRoundTrip(f *testing.F) {
	f.Add([]byte("<13>1 - - - - - - x"))
	f.Add([]byte("\n\x00 5 <>"))
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) == 0 || len(payload) > 1<<16 {
			return
		}
		stream := append(fmt.Appendf(nil, "%d ", len(payload)), payload...)
		got, err := Frame(bufio.NewReader(bytes.NewReader(stream)), len(payload))
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("got %q, %v", got, err)
		}
	})
}

func checkClean(v string) error {
	if !utf8.ValidString(v) {
		return errors.New("invalid UTF-8")
	}
	for _, r := range v {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 {
			return fmt.Errorf("raw control %#x", r)
		}
	}
	return nil
}
