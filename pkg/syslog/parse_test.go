// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package syslog

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"
)

const bom = "\xEF\xBB\xBF"

// RFC 5424 §6.5, all four examples.
func TestParse5424Examples(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Message
	}{
		{
			name: "example 1: su, BOM, no structured data",
			in:   "<34>1 2003-10-11T22:14:15.003Z mymachine.example.com su - ID47 - " + bom + "'su root' failed for lonvick on /dev/pts/8",
			want: Message{Format: RFC5424, Facility: 4, Severity: 2, Timestamp: "2003-10-11T22:14:15.003Z", Hostname: "mymachine.example.com", AppName: "su", MsgID: "ID47", Text: "'su root' failed for lonvick on /dev/pts/8"},
		},
		{
			name: "example 2: no BOM, procid",
			in:   "<165>1 2003-08-24T05:14:15.000003-07:00 192.0.2.1 myproc 8710 - - %% It's time to make the do-nuts.",
			want: Message{Format: RFC5424, Facility: 20, Severity: 5, Timestamp: "2003-08-24T05:14:15.000003-07:00", Hostname: "192.0.2.1", AppName: "myproc", ProcID: "8710", Text: "%% It's time to make the do-nuts."},
		},
		{
			name: "example 3: structured data and a message",
			in:   `<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] ` + bom + "An application event log entry...",
			want: Message{Format: RFC5424, Facility: 20, Severity: 5, Timestamp: "2003-10-11T22:14:15.003Z", Hostname: "mymachine.example.com", AppName: "evntslog", MsgID: "ID47", StructuredData: `[exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"]`, Text: "An application event log entry..."},
		},
		{
			name: "example 4: two elements, no message",
			in:   `<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"][examplePriority@32473 class="high"]`,
			want: Message{Format: RFC5424, Facility: 20, Severity: 5, Timestamp: "2003-10-11T22:14:15.003Z", Hostname: "mymachine.example.com", AppName: "evntslog", MsgID: "ID47", StructuredData: `[exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"][examplePriority@32473 class="high"]`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func Test5424NilValuesAreEmpty(t *testing.T) {
	got, err := Parse([]byte("<13>1 - - - - - - body"))
	if err != nil {
		t.Fatal(err)
	}
	want := Message{Format: RFC5424, Facility: 1, Severity: 5, Text: "body"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParse3164(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Message
	}{
		{
			name: "RFC 3164 §5.4 with hostname",
			in:   "<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8",
			want: Message{Format: RFC3164, Facility: 4, Severity: 2, Timestamp: "Oct 11 22:14:15", Hostname: "mymachine", AppName: "su", Text: "'su root' failed for lonvick on /dev/pts/8"},
		},
		{
			name: "no hostname",
			in:   "<34>Oct 11 22:14:15 su: 'su root' failed",
			want: Message{Format: RFC3164, Facility: 4, Severity: 2, Timestamp: "Oct 11 22:14:15", AppName: "su", Text: "'su root' failed"},
		},
		{
			name: "tag with a pid, day padded with a space",
			in:   "<134>Nov  4 01:02:03 pihole dnsmasq[123]: query[A] example.org from 10.10.20.1",
			want: Message{Format: RFC3164, Facility: 16, Severity: 6, Timestamp: "Nov  4 01:02:03", Hostname: "pihole", AppName: "dnsmasq", ProcID: "123", Text: "query[A] example.org from 10.10.20.1"},
		},
		{
			name: "the Phase 6 canary",
			in:   "<134>Nov  4 01:02:03 pihole lh-canary: lh-ct-1730000000\n",
			want: Message{Format: RFC3164, Facility: 16, Severity: 6, Timestamp: "Nov  4 01:02:03", Hostname: "pihole", AppName: "lh-canary", Text: "lh-ct-1730000000"},
		},
		{
			name: "no PRI: user.notice, RFC 3164 §4.3.3",
			in:   "Oct 11 22:14:15 mymachine su: no pri here",
			want: Message{Format: RFC3164, Facility: 1, Severity: 5, Timestamp: "Oct 11 22:14:15", Hostname: "mymachine", AppName: "su", Text: "no pri here"},
		},
		{
			name: "no PRI, no header: all of it is the message",
			in:   "just some words on a wire",
			want: Message{Format: RFC3164, Facility: 1, Severity: 5, Text: "just some words on a wire"},
		},
		{
			name: "PRI and a tag, no timestamp",
			in:   "<13>su: short",
			want: Message{Format: RFC3164, Facility: 1, Severity: 5, AppName: "su", Text: "short"},
		},
		{
			name: "hostname and no tag",
			in:   "<13>Oct 11 22:14:15 mymachine the rest is words",
			want: Message{Format: RFC3164, Facility: 1, Severity: 5, Timestamp: "Oct 11 22:14:15", Hostname: "mymachine", Text: "the rest is words"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// "anF" is inside the month table but is not a month.
func TestBSDTimestampMonthMustBeAMonth(t *testing.T) {
	m, err := Parse([]byte("<13>anF 11 22:14:15 host tag: body"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Timestamp != "" || m.Text != "anF 11 22:14:15 host tag: body" {
		t.Errorf("got %+v", m)
	}
}

func TestPRIBounds(t *testing.T) {
	for _, in := range []string{"<0>x", "<191>x"} {
		if _, err := Parse([]byte(in)); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
	m, err := Parse([]byte("<191>x"))
	if err != nil || m.Facility != 23 || m.Severity != 7 {
		t.Errorf("<191>: %+v, %v", m, err)
	}
	for _, in := range []string{"<192>x", "<999>x", "<1000>x", "<>x", "<-1>x", "<034>x", "<00>x", "<34", "<", "<a>x", "<34 >x"} {
		m, err := Parse([]byte(in))
		if !errors.Is(err, ErrPRI) {
			t.Errorf("%q: err = %v, want ErrPRI", in, err)
		}
		// A malformed line is evidence: it comes back whole, as user.notice.
		if m.Text != in || m.Facility != 1 || m.Severity != 5 {
			t.Errorf("%q: got %+v, want the line back as user.notice", in, m)
		}
	}
}

func TestTruncatedHeader(t *testing.T) {
	for _, in := range []string{
		"<34>1 ",
		"<34>1 2003-10-11T22:14:15.003Z",
		"<34>1 2003-10-11T22:14:15.003Z mymachine",
		"<34>1 2003-10-11T22:14:15.003Z mymachine su",
		"<34>1 2003-10-11T22:14:15.003Z mymachine su -",
		"<34>1 2003-10-11T22:14:15.003Z mymachine su - ID47",
		"<34>1 2003-10-11T22:14:15.003Z mymachine su - ID47 ",
	} {
		m, err := Parse([]byte(in))
		if !errors.Is(err, ErrHeader) {
			t.Errorf("%q: err = %v, want ErrHeader", in, err)
		}
		if m.Text == "" || m.Facility != 1 || m.Severity != 5 {
			t.Errorf("%q: got %+v, want the line back as user.notice", in, m)
		}
	}
}

func TestEmpty(t *testing.T) {
	for _, in := range []string{"", "\n", "\r\n", "\x00", "\n\x00\r\n"} {
		if _, err := Parse([]byte(in)); !errors.Is(err, ErrEmpty) {
			t.Errorf("%q: err = %v, want ErrEmpty", in, err)
		}
	}
}

func TestNULInMessage(t *testing.T) {
	m, err := Parse([]byte("<13>1 - - app - - - before\x00after"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Text != `before\x00after` {
		t.Errorf("Text = %q", m.Text)
	}
	// A sender's trailing NUL or newline is framing, not content.
	m, err = Parse([]byte("<13>1 - - app - - - body\r\n\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Text != "body" {
		t.Errorf("Text = %q", m.Text)
	}
}

func TestOneMiBIsTruncated(t *testing.T) {
	in := append([]byte("<13>1 - - app - - - "), bytes.Repeat([]byte("A"), 1<<20)...)
	m, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Truncated {
		t.Error("Truncated not set")
	}
	if len(m.Text) > DefaultMaxMessage {
		t.Errorf("Text is %d bytes, cap %d", len(m.Text), DefaultMaxMessage)
	}
	if len(m.Text) < DefaultMaxMessage-64 {
		t.Errorf("Text is only %d bytes: truncation lost more than the header", len(m.Text))
	}
}

// Escaping grows a line up to fourfold; the cap holds on what comes out.
func TestEscapingCannotExceedTheCap(t *testing.T) {
	in := append([]byte("<13>1 - - app - - - "), bytes.Repeat([]byte{0x01}, DefaultMaxMessage-64)...)
	m, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Text) > DefaultMaxMessage {
		t.Errorf("Text is %d bytes, cap %d", len(m.Text), DefaultMaxMessage)
	}
	if !m.Truncated {
		t.Error("Truncated not set after the escaped text hit the cap")
	}
	if strings.HasSuffix(m.Text, `\`) || strings.HasSuffix(m.Text, `\x`) || strings.HasSuffix(m.Text, `\x0`) {
		t.Errorf("cut inside an escape: ...%q", m.Text[len(m.Text)-6:])
	}
}

func TestParseMaxBelowDefault(t *testing.T) {
	m, err := ParseMax([]byte("<13>1 - - app - - - abcdefghijklmnopqrstuvwxyz"), 24)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Truncated || len(m.Text) > 24 {
		t.Errorf("got %+v", m)
	}
}

func TestInvalidUTF8(t *testing.T) {
	m, err := Parse([]byte("<13>1 - - app - - - ok\xff\xfe\xc0\x80 tail"))
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(m.Text) {
		t.Fatalf("Text is not valid UTF-8: %q", m.Text)
	}
	if m.Text != `ok\xff\xfe\xc0\x80 tail` {
		t.Errorf("Text = %q", m.Text)
	}
}

func TestValidUTF8IsKept(t *testing.T) {
	m, err := Parse([]byte("<13>1 - - app - - - Mjölnir ⚡ 北"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Text != "Mjölnir ⚡ 北" {
		t.Errorf("Text = %q", m.Text)
	}
}

func TestControlCharactersAreEscaped(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a\nb", `a\x0ab`},
		{"a\rb", `a\x0db`},
		{"a\tb", `a\x09b`},
		{"\x1b[31mred", `\x1b[31mred`},
		{"a\x7fb", `a\x7fb`},
		{"a\u009bb", `a\u009bb`}, // C1 CSI, as UTF-8
		{"a\u2028b", `a\u2028b`}, // line separator
		{"a\u2029b", `a\u2029b`}, // paragraph separator
		{"a\u202eb", `a\u202eb`}, // right-to-left override
		{"a\u2066b", `a\u2066b`}, // left-to-right isolate
		{"a\U000e0001b", `a\U000e0001b`},
		{`a\x0ab`, `a\x0ab`}, // a literal backslash is left alone
	}
	for _, tt := range tests {
		m, err := Parse([]byte("<13>1 - - app - - - " + tt.in))
		if err != nil {
			t.Fatalf("%q: %v", tt.in, err)
		}
		if m.Text != tt.want {
			t.Errorf("%q: Text = %q, want %q", tt.in, m.Text, tt.want)
		}
	}
}

// Every field is hostile, not only the message.
func TestHeaderFieldsAreSanitised(t *testing.T) {
	m, err := Parse([]byte("<13>1 ti\x01me ho\nst ap\x1bp pr\x00oc ms\xffg [a@1 b=\"c\x07d\"] m"))
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]string{"Timestamp": m.Timestamp, "Hostname": m.Hostname, "AppName": m.AppName, "ProcID": m.ProcID, "MsgID": m.MsgID, "StructuredData": m.StructuredData} {
		if v == "" {
			t.Errorf("%s is empty", name)
		}
		assertClean(t, name, v)
	}
}

func TestHeaderFieldsAreCapped(t *testing.T) {
	long := strings.Repeat("h", 1000)
	m, err := Parse([]byte("<13>1 " + long + " " + long + " " + long + " " + long + " " + long + " - m"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Timestamp) > MaxTimestamp || len(m.Hostname) > MaxHostname || len(m.AppName) > MaxAppName || len(m.ProcID) > MaxProcID || len(m.MsgID) > MaxMsgID {
		t.Errorf("uncapped field: %d %d %d %d %d", len(m.Timestamp), len(m.Hostname), len(m.AppName), len(m.ProcID), len(m.MsgID))
	}
	if !m.Truncated {
		t.Error("Truncated not set")
	}
}

func TestStructuredData(t *testing.T) {
	tests := []struct {
		name   string
		sd     string
		text   string
		wantSD string
	}{
		{"escaped ] in a value", `[a@1 x="b\]c"] msg`, "msg", `[a@1 x="b\]c"]`},
		{"bare ] in a value stays in the value", `[a@1 x="b]c"] msg`, "msg", `[a@1 x="b]c"]`},
		{"escaped quote then ]", `[a@1 x="b\"]c"] msg`, "msg", `[a@1 x="b\"]c"]`},
		{"escaped backslash ends the value", `[a@1 x="b\\"] msg`, "msg", `[a@1 x="b\\"]`},
		{"[ in a value", `[a@1 x="[b"] msg`, "msg", `[a@1 x="[b"]`},
		{"no parameters", `[a@1] msg`, "msg", `[a@1]`},
		{"a message holding brackets", `[a@1 x="y"] [not sd]`, "[not sd]", `[a@1 x="y"]`},
		{"nil with a bracketed message", `- [not sd]`, "[not sd]", ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Parse([]byte("<13>1 - - app - - " + tt.sd))
			if err != nil {
				t.Fatal(err)
			}
			if m.StructuredData != tt.wantSD || m.Text != tt.text {
				t.Errorf("SD %q Text %q, want SD %q Text %q", m.StructuredData, m.Text, tt.wantSD, tt.text)
			}
		})
	}
	for _, sd := range []string{`[a@1 x="y`, `[a@1 x="y"`, `[a@1 x="y"]junk`, `[`, `x`, `[a@1 x="\`} {
		m, err := Parse([]byte("<13>1 - - app - - " + sd))
		if !errors.Is(err, ErrStructuredData) {
			t.Errorf("%q: err = %v, want ErrStructuredData", sd, err)
		}
		if m.Text == "" {
			t.Errorf("%q: the line did not come back", sd)
		}
	}
}

func TestTagNeverMintedFromGarbage(t *testing.T) {
	tag := strings.Repeat("t", MaxAppName+1)
	m, err := Parse([]byte("<13>" + tag + ": body"))
	if err != nil {
		t.Fatal(err)
	}
	if m.AppName != "" {
		t.Errorf("AppName = %q from an overlong tag", m.AppName)
	}
	m, err = Parse([]byte("<13>su[]: body"))
	if err != nil {
		t.Fatal(err)
	}
	if m.AppName != "" {
		t.Errorf("AppName = %q from an empty pid", m.AppName)
	}
}

func frameReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestFrameOctetCounting(t *testing.T) {
	r := frameReader("5 hello3 abc")
	for _, want := range []string{"hello", "abc"} {
		got, err := Frame(r, 64)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if _, err := Frame(r, 64); !errors.Is(err, io.EOF) {
		t.Errorf("err = %v, want EOF", err)
	}
}

func TestFrameOctetCountingAtTheCap(t *testing.T) {
	got, err := Frame(frameReader("5 hello"), 5)
	if err != nil || string(got) != "hello" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestFrameLengthOverTheCap(t *testing.T) {
	for _, in := range []string{"65 x", "1048576 x", "99999999999999999999999999 x"} {
		if _, err := Frame(frameReader(in), 64); !errors.Is(err, ErrFrameTooLarge) {
			t.Errorf("%q: err = %v, want ErrFrameTooLarge", in, err)
		}
	}
}

// A length smaller than the frame leaves the rest of it on the stream. The
// first frame is what the length said; the remainder is not a frame.
func TestFrameLengthSmallerThanTheFrame(t *testing.T) {
	r := frameReader("5 hello world\n")
	got, err := Frame(r, 64)
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := Frame(r, 64); !errors.Is(err, ErrBadFrame) {
		t.Errorf("err = %v, want ErrBadFrame", err)
	}
}

func TestFrameLengthLargerThanTheStream(t *testing.T) {
	if _, err := Frame(frameReader("10 short"), 64); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("err = %v, want ErrUnexpectedEOF", err)
	}
}

func TestFrameBadHeaders(t *testing.T) {
	for _, in := range []string{"0 x", "05 hello", "5hello", "5\nhello", "abc\n", " <13>x\n", "-5 hello"} {
		if _, err := Frame(frameReader(in), 64); !errors.Is(err, ErrBadFrame) {
			t.Errorf("%q: err = %v, want ErrBadFrame", in, err)
		}
	}
}

func TestFrameDigitsThenEOF(t *testing.T) {
	if _, err := Frame(frameReader("12"), 64); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("err = %v, want ErrUnexpectedEOF", err)
	}
}

func TestFrameLF(t *testing.T) {
	r := frameReader("<13>one\n<13>two\r\n\n\r\n<13>three")
	for _, want := range []string{"<13>one", "<13>two", "<13>three"} {
		got, err := Frame(r, 64)
		if err != nil {
			t.Fatalf("%s: %v", want, err)
		}
		if string(got) != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if _, err := Frame(r, 64); !errors.Is(err, io.EOF) {
		t.Errorf("err = %v, want EOF", err)
	}
}

func TestFrameLFOverTheCap(t *testing.T) {
	line := "<13>" + strings.Repeat("x", 1<<20) + "\n"
	if _, err := Frame(frameReader(line), 8192); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("err = %v, want ErrFrameTooLarge", err)
	}
	// Longer than bufio's buffer and under the cap still works.
	r := bufio.NewReaderSize(strings.NewReader("<13>"+strings.Repeat("y", 300)+"\n"), 16)
	got, err := Frame(r, 8192)
	if err != nil || len(got) != 304 {
		t.Errorf("len %d, %v", len(got), err)
	}
}

func TestFrameLFAtTheCap(t *testing.T) {
	line := "<13>" + strings.Repeat("x", 60)
	got, err := Frame(frameReader(line+"\n"), 64)
	if err != nil || string(got) != line {
		t.Errorf("exactly at the cap: %q, %v", got, err)
	}
	if _, err := Frame(frameReader(line+"x\n"), 64); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("one over: err = %v", err)
	}
}

func TestFrameMaxMustBePositive(t *testing.T) {
	if _, err := Frame(frameReader("1 x"), 0); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("err = %v", err)
	}
}

// A short read in the middle of a frame is not an error.
func TestFrameSurvivesOneByteReads(t *testing.T) {
	r := bufio.NewReader(iotest.OneByteReader(strings.NewReader("5 hello<13>x\n")))
	for _, want := range []string{"hello", "<13>x"} {
		got, err := Frame(r, 64)
		if err != nil || string(got) != want {
			t.Fatalf("got %q, %v, want %q", got, err, want)
		}
	}
}

// A read error in the middle of a frame reaches the caller; a timeout must
// never hand a half line to the parser as if it were whole.
func TestFrameReadErrorIsNotAFrame(t *testing.T) {
	boom := errors.New("boom")
	r := bufio.NewReader(io.MultiReader(strings.NewReader("<13>half"), iotest.ErrReader(boom)))
	if got, err := Frame(r, 64); !errors.Is(err, boom) {
		t.Errorf("got %q, err = %v, want boom", got, err)
	}
}

func assertClean(t *testing.T, name, v string) {
	t.Helper()
	if !utf8.ValidString(v) {
		t.Errorf("%s is not valid UTF-8: %q", name, v)
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] == 0x7f {
			t.Errorf("%s holds raw control byte %#x: %q", name, v[i], v)
			return
		}
	}
}
