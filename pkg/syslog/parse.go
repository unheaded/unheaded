// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package syslog reads syslog messages from the network: RFC 5424 and RFC 3164
// bodies, and RFC 6587 TCP framing. Standard library only (ADR-124).
//
// Everything here is hostile input. Parse never returns a string holding a
// control byte, invalid UTF-8 or more bytes than its cap, in any field and on
// any path, error paths included. What a Message says about itself (its
// priority, its hostname, its name) is the sender's claim and nothing more.
package syslog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DefaultMaxMessage is the cap Parse applies. RFC 5424 §6.1 asks a receiver to
// accept at least 2048 octets.
const DefaultMaxMessage = 8192

// Field caps. HOSTNAME, APP-NAME, PROCID and MSGID are RFC 5424 §6's; the
// timestamp is its longest legal form, with room to spare.
const (
	MaxTimestamp = 64
	MaxHostname  = 255
	MaxAppName   = 48
	MaxProcID    = 128
	MaxMsgID     = 32
)

// A line with no usable PRI is user.notice (RFC 3164 §4.3.3).
const (
	defaultFacility = 1
	defaultSeverity = 5
)

// Format says how a message was read.
type Format int

// Formats. Unparsed marks a line that came back whole, as text.
const (
	Unparsed Format = iota
	RFC5424
	RFC3164
)

// Errors from Parse. Apart from ErrEmpty the Message that comes with them is
// still good: the whole line as Text, at user.notice. A line that will not
// parse is still evidence, so the caller decides what to do with it.
var (
	ErrEmpty          = errors.New("syslog: empty message")
	ErrPRI            = errors.New("syslog: bad PRI")
	ErrHeader         = errors.New("syslog: truncated RFC 5424 header")
	ErrStructuredData = errors.New("syslog: bad structured data")
)

// Errors from Frame. Both leave the stream at an unknown position, so the
// caller closes the connection.
var (
	ErrFrameTooLarge = errors.New("syslog: frame over the cap")
	ErrBadFrame      = errors.New("syslog: bad frame")
)

// Message is one parsed message. Every string is sanitised and capped. Absent
// and nil-valued fields are empty.
type Message struct {
	Format         Format
	Facility       int
	Severity       int
	Timestamp      string
	Hostname       string
	AppName        string
	ProcID         string
	MsgID          string
	StructuredData string
	Text           string
	// Truncated is set when the input or any field was cut to fit.
	Truncated bool
}

// Parse reads one message under DefaultMaxMessage.
func Parse(b []byte) (Message, error) {
	return ParseMax(b, DefaultMaxMessage)
}

// ParseMax reads one message, RFC 5424 if it says so and RFC 3164 otherwise.
// Input over max bytes is cut first and flagged; Text never exceeds max bytes,
// after escaping. A max of zero or less means DefaultMaxMessage.
func ParseMax(b []byte, max int) (Message, error) {
	if max <= 0 {
		max = DefaultMaxMessage
	}
	truncated := false
	if len(b) > max {
		b = b[:max]
		truncated = true
	}
	// A sender's trailing newline or NUL is framing, not content.
	b = bytes.TrimRight(b, "\r\n\x00")
	if len(b) == 0 {
		return Message{}, ErrEmpty
	}

	m := Message{Facility: defaultFacility, Severity: defaultSeverity, Truncated: truncated}
	if b[0] != '<' {
		parse3164(&m, b, max)
		return m, nil
	}
	pri, body, err := parsePRI(b)
	if err != nil {
		return whole(b, max, truncated), err
	}
	m.Facility, m.Severity = pri/8, pri%8
	if bytes.HasPrefix(body, []byte("1 ")) {
		if err := parse5424(&m, body[2:], max); err != nil {
			return whole(b, max, truncated), err
		}
		return m, nil
	}
	parse3164(&m, body, max)
	return m, nil
}

// whole is the fallback: the line as text, at the default priority.
func whole(b []byte, max int, truncated bool) Message {
	m := Message{Facility: defaultFacility, Severity: defaultSeverity, Truncated: truncated}
	m.Text = clean(b, max, &m.Truncated)
	return m
}

// parsePRI reads <N> at the start of b: one to three digits, no leading zero
// (RFC 5424 §6.2.1), at most 191.
func parsePRI(b []byte) (int, []byte, error) {
	end := bytes.IndexByte(b, '>')
	if end < 2 || end > 4 {
		return 0, nil, ErrPRI
	}
	digits := b[1:end]
	if len(digits) > 1 && digits[0] == '0' {
		return 0, nil, ErrPRI
	}
	pri := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, nil, ErrPRI
		}
		pri = pri*10 + int(c-'0')
	}
	if pri > 191 {
		return 0, nil, ErrPRI
	}
	return pri, b[end+1:], nil
}

// parse5424 reads what follows "VERSION SP". Every field but MSG is required.
func parse5424(m *Message, b []byte, max int) error {
	var tok [5][]byte
	for i := range tok {
		end := bytes.IndexByte(b, ' ')
		if end < 0 {
			return ErrHeader
		}
		tok[i], b = b[:end], b[end+1:]
	}
	if len(b) == 0 {
		return ErrHeader
	}
	sd, rest, err := splitStructuredData(b)
	if err != nil {
		return err
	}
	m.Format = RFC5424
	m.Timestamp = field(tok[0], MaxTimestamp, &m.Truncated)
	m.Hostname = field(tok[1], MaxHostname, &m.Truncated)
	m.AppName = field(tok[2], MaxAppName, &m.Truncated)
	m.ProcID = field(tok[3], MaxProcID, &m.Truncated)
	m.MsgID = field(tok[4], MaxMsgID, &m.Truncated)
	if !bytes.Equal(sd, []byte("-")) {
		m.StructuredData = clean(sd, max, &m.Truncated)
	}
	m.Text = clean(bytes.TrimPrefix(rest, []byte("\xEF\xBB\xBF")), max, &m.Truncated)
	return nil
}

// field reads a header field, where "-" is the nil value.
func field(b []byte, limit int, truncated *bool) string {
	if len(b) == 1 && b[0] == '-' {
		return ""
	}
	return clean(b, limit, truncated)
}

// splitStructuredData returns the STRUCTURED-DATA at the start of b and the
// MSG after it. A ']' inside a quoted value belongs to the value: RFC 5424
// wants it escaped, senders forget, and a scanner that stops there hands the
// rest of the value to the message.
func splitStructuredData(b []byte) (sd, msg []byte, err error) {
	if b[0] == '-' {
		switch {
		case len(b) == 1:
			return b, nil, nil
		case b[1] == ' ':
			return b[:1], b[2:], nil
		}
		return nil, nil, ErrStructuredData
	}
	i := 0
	for {
		if i >= len(b) || b[i] != '[' {
			return nil, nil, ErrStructuredData
		}
		end := elementEnd(b, i)
		if end < 0 {
			return nil, nil, ErrStructuredData
		}
		i = end + 1
		switch {
		case i == len(b):
			return b, nil, nil
		case b[i] == ' ':
			return b[:i], b[i+1:], nil
		}
	}
}

// elementEnd returns the index of the ']' closing the element at b[start], or
// -1 when it never closes.
func elementEnd(b []byte, start int) int {
	quoted := false
	for i := start + 1; i < len(b); i++ {
		switch c := b[i]; {
		case quoted && c == '\\':
			i++
		case c == '"':
			quoted = !quoted
		case !quoted && c == ']':
			return i
		}
	}
	return -1
}

// parse3164 reads what follows the PRI. RFC 3164 is a description of practice,
// not a grammar, so nothing here fails: what cannot be read as a header is
// content. Timestamp, then a hostname unless the next word is already a tag,
// then the tag.
func parse3164(m *Message, b []byte, max int) {
	m.Format = RFC3164
	stamped := false
	if isBSDTimestamp(b) {
		m.Timestamp = clean(b[:bsdTimestampLen], MaxTimestamp, &m.Truncated)
		stamped = true
		b = bytes.TrimPrefix(b[bsdTimestampLen:], []byte(" "))
	}
	word, rest := nextWord(b)
	if name, pid, ok := tag(word); ok {
		m.AppName, m.ProcID = name, pid
		b = rest
	} else if stamped && len(word) > 0 {
		m.Hostname = clean(word, MaxHostname, &m.Truncated)
		b = rest
		word, rest = nextWord(b)
		if name, pid, ok := tag(word); ok {
			m.AppName, m.ProcID = name, pid
			b = rest
		}
	}
	m.Text = clean(b, max, &m.Truncated)
}

func nextWord(b []byte) (word, rest []byte) {
	if i := bytes.IndexByte(b, ' '); i >= 0 {
		return b[:i], b[i+1:]
	}
	return b, nil
}

// tag reads `name:` or `name[pid]:`. Plain ASCII only, so its output needs no
// escaping and a word of junk is simply not a tag.
func tag(word []byte) (name, pid string, ok bool) {
	if len(word) < 2 || word[len(word)-1] != ':' {
		return "", "", false
	}
	body := word[:len(word)-1]
	if body[len(body)-1] == ']' {
		open := bytes.IndexByte(body, '[')
		if open < 0 {
			return "", "", false
		}
		p := body[open+1 : len(body)-1]
		if len(p) == 0 || len(p) > MaxProcID || !tagChars(p) {
			return "", "", false
		}
		pid = string(p)
		body = body[:open]
	}
	if len(body) == 0 || len(body) > MaxAppName || !tagChars(body) {
		return "", "", false
	}
	return string(body), pid, true
}

func tagChars(b []byte) bool {
	for _, c := range b {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.', c == '/':
		default:
			return false
		}
	}
	return true
}

// bsdTimestampLen is len("Mmm dd hh:mm:ss").
const bsdTimestampLen = 15

// isBSDTimestamp reports whether b starts with "Mmm dd hh:mm:ss" and the
// timestamp ends there. The day may be space-padded.
func isBSDTimestamp(b []byte) bool {
	if len(b) < bsdTimestampLen || (len(b) > bsdTimestampLen && b[bsdTimestampLen] != ' ') {
		return false
	}
	if i := strings.Index("JanFebMarAprMayJunJulAugSepOctNovDec", string(b[:3])); i < 0 || i%3 != 0 {
		return false
	}
	if b[3] != ' ' || b[6] != ' ' {
		return false
	}
	digit := func(i int) bool { return b[i] >= '0' && b[i] <= '9' }
	return (b[4] == ' ' || digit(4)) && digit(5) &&
		digit(7) && digit(8) && b[9] == ':' && digit(10) && digit(11) && b[12] == ':' && digit(13) && digit(14)
}

// clean makes b safe to store and to show, in at most limit bytes: C0, DEL and
// invalid UTF-8 become \xHH; C1, the line and paragraph separators and Unicode
// format characters (bidi overrides among them) become \uHHHH. A backslash
// stays as it is, so a sender can write text that looks like an escape but can
// never write the byte. It cuts between escapes, never inside one, and sets
// *truncated when it cuts.
func clean(b []byte, limit int, truncated *bool) string {
	var out strings.Builder
	out.Grow(min(len(b), limit))
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		piece := escape(r, b[:size])
		if out.Len()+len(piece) > limit {
			*truncated = true
			break
		}
		out.WriteString(piece)
		b = b[size:]
	}
	return out.String()
}

func escape(r rune, raw []byte) string {
	switch {
	case r == utf8.RuneError && len(raw) == 1:
		return fmt.Sprintf(`\x%02x`, raw[0])
	case r < 0x20 || r == 0x7f:
		return fmt.Sprintf(`\x%02x`, r)
	case r >= 0x80 && r <= 0x9f, r == 0x2028, r == 0x2029, unicode.Is(unicode.Cf, r):
		if r > 0xffff {
			return fmt.Sprintf(`\U%08x`, r)
		}
		return fmt.Sprintf(`\u%04x`, r)
	}
	return string(raw)
}

// Frame reads one message from a TCP stream, by RFC 6587: a first byte of 1-9
// means octet counting ("LEN SP MSG"), a '<' means a line ended by LF. Stray
// newlines between frames are skipped. It never holds more than max bytes of a
// frame, and an announced length is checked before anything is allocated.
//
// io.EOF means the stream ended between frames. A line cut by EOF is returned
// as a frame, since LF-framed senders may close without a trailer; a line cut
// by any other error is not.
func Frame(r *bufio.Reader, max int) ([]byte, error) {
	if max <= 0 {
		return nil, ErrFrameTooLarge
	}
	for {
		c, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		switch {
		case c == '\n', c == '\r', c == 0:
			continue
		case c >= '1' && c <= '9':
			return octetFrame(r, int(c-'0'), max)
		case c == '<':
			if err := r.UnreadByte(); err != nil {
				return nil, err
			}
			return lineFrame(r, max)
		}
		return nil, ErrBadFrame
	}
}

func octetFrame(r *bufio.Reader, n, max int) ([]byte, error) {
	for {
		if n > max {
			return nil, ErrFrameTooLarge
		}
		c, err := r.ReadByte()
		if err != nil {
			return nil, unexpected(err)
		}
		if c == ' ' {
			break
		}
		if c < '0' || c > '9' {
			return nil, ErrBadFrame
		}
		n = n*10 + int(c-'0')
	}
	frame := make([]byte, n)
	if _, err := io.ReadFull(r, frame); err != nil {
		return nil, unexpected(err)
	}
	return frame, nil
}

func lineFrame(r *bufio.Reader, max int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if err == nil {
			chunk = chunk[:len(chunk)-1]
		}
		if len(line)+len(chunk) > max {
			return nil, ErrFrameTooLarge
		}
		line = append(line, chunk...)
		switch {
		case err == nil:
			return bytes.TrimSuffix(line, []byte("\r")), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) > 0:
			return line, nil
		}
		return nil, err
	}
}

// unexpected turns EOF inside a frame into io.ErrUnexpectedEOF.
func unexpected(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
