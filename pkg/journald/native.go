// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package journald writes entries to the local journal through its native
// protocol, the datagram socket at /run/systemd/journal/socket. Standard
// library only (ADR-124).
//
// A datagram is a run of fields. A field is `NAME=value\n`, or, when the value
// holds a newline, `NAME\n`, a 64-bit little-endian length, the value and `\n`.
// That second form is what keeps a newline in hostile text from starting a
// field of its own.
package journald

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// DefaultSocket is where journald listens.
const DefaultSocket = "/run/systemd/journal/socket"

// DefaultMaxDatagram stays under the kernel's default send buffer
// (net.core.wmem_default, 212992), past which a unix datagram fails EMSGSIZE.
// journald's memfd path for larger entries is out of scope.
const DefaultMaxDatagram = 128 << 10

// DefaultTimeout bounds a write to a journald that has stopped reading.
const DefaultTimeout = time.Second

const (
	maxNameLen    = 64 // journald's own limit on a field name
	truncatedName = "UNHEADED_TRUNCATED"
	messageName   = "MESSAGE"
)

// Errors from Send.
var (
	// ErrFieldName marks a name journald would refuse or reserve: outside
	// [A-Z0-9_], starting with a digit, over 64 bytes, or starting with '_',
	// which journald keeps for what it records itself (_PID, _UID,
	// _SYSTEMD_UNIT).
	ErrFieldName = errors.New("journald: bad field name")
	ErrNoFields  = errors.New("journald: no fields")
	// ErrTooLarge means the fields other than MESSAGE fill the datagram.
	ErrTooLarge = errors.New("journald: entry too large")
)

// Writer sends entries to one journal socket. It connects on first use and
// again after any failure. It is safe for concurrent use. Set the exported
// fields before the first Send.
type Writer struct {
	// MaxDatagram caps a datagram. Over it, MESSAGE is cut and
	// UNHEADED_TRUNCATED=1 is set.
	MaxDatagram int
	// Timeout bounds one write.
	Timeout time.Duration

	path string
	mu   sync.Mutex
	conn *net.UnixConn
}

// New returns a Writer for the socket at path. It does not connect.
func New(path string) *Writer {
	return &Writer{MaxDatagram: DefaultMaxDatagram, Timeout: DefaultTimeout, path: path}
}

// Send writes one entry. An entry that fails validation never reaches the
// socket. A connected datagram socket goes stale when journald's goes away, so
// a failed write reconnects and tries once more, unless the failure was a
// timeout: then the peer is there and not reading, and a retry only doubles
// the wait.
func (w *Writer) Send(fields map[string]string) error {
	datagram, err := encode(fields, w.MaxDatagram)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	err = w.write(datagram)
	var ne net.Error
	if err == nil || (errors.As(err, &ne) && ne.Timeout()) {
		return err
	}
	return w.write(datagram)
}

// write sends one datagram, dropping the connection on any failure.
func (w *Writer) write(datagram []byte) error {
	if w.conn == nil {
		c, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: w.path, Net: "unixgram"})
		if err != nil {
			return fmt.Errorf("journald: %w", err)
		}
		w.conn = c
	}
	if err := w.conn.SetWriteDeadline(time.Now().Add(w.Timeout)); err != nil {
		w.drop()
		return fmt.Errorf("journald: %w", err)
	}
	if _, err := w.conn.Write(datagram); err != nil {
		w.drop()
		return fmt.Errorf("journald: %w", err)
	}
	return nil
}

func (w *Writer) drop() {
	_ = w.conn.Close()
	w.conn = nil
}

// Close releases the socket. A later Send connects again.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn == nil {
		return nil
	}
	err := w.conn.Close()
	w.conn = nil
	return err
}

// encode builds the datagram, with names sorted so that one entry always
// encodes the same way.
func encode(fields map[string]string, max int) ([]byte, error) {
	if len(fields) == 0 {
		return nil, ErrNoFields
	}
	names := make([]string, 0, len(fields)+1)
	for name := range fields {
		if err := checkName(name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	sort.Strings(names)

	size := 0
	for _, name := range names {
		size += fieldSize(name, fields[name])
	}
	if size <= max {
		return appendFields(make([]byte, 0, size), names, fields, ""), nil
	}

	message, ok := fields[messageName]
	if !ok {
		return nil, fmt.Errorf("%w: %d bytes, limit %d", ErrTooLarge, size, max)
	}
	// What is left for the message once every other field and the flag are
	// in. The binary form's overhead is counted whatever the cut text holds.
	room := max - (size - fieldSize(messageName, message)) - fieldSize(truncatedName, "1") - len(messageName) - 10
	if room <= 0 {
		return nil, fmt.Errorf("%w: %d bytes without the message, limit %d", ErrTooLarge, size-len(message), max)
	}
	if _, set := fields[truncatedName]; !set {
		names = append(names, truncatedName)
		sort.Strings(names)
	}
	return appendFields(make([]byte, 0, max), names, fields, cutRunes(message, room)), nil
}

// appendFields writes every field; a non-empty cut replaces MESSAGE and sets
// the flag.
func appendFields(out []byte, names []string, fields map[string]string, cut string) []byte {
	for _, name := range names {
		value := fields[name]
		if cut != "" {
			switch name {
			case messageName:
				value = cut
			case truncatedName:
				value = "1"
			}
		}
		if strings.IndexByte(value, '\n') < 0 {
			out = append(out, name...)
			out = append(out, '=')
			out = append(out, value...)
			out = append(out, '\n')
			continue
		}
		out = append(out, name...)
		out = append(out, '\n')
		out = binary.LittleEndian.AppendUint64(out, uint64(len(value)))
		out = append(out, value...)
		out = append(out, '\n')
	}
	return out
}

func fieldSize(name, value string) int {
	if strings.IndexByte(value, '\n') < 0 {
		return len(name) + len(value) + 2
	}
	return len(name) + len(value) + 10
}

// cutRunes shortens s to at most n bytes, on a rune boundary when s is UTF-8.
func cutRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for i := n; i > 0 && i > n-utf8.UTFMax; i-- {
		if utf8.RuneStart(s[i]) {
			return s[:i]
		}
	}
	return s[:n]
}

func checkName(name string) error {
	if name == "" || len(name) > maxNameLen {
		return fmt.Errorf("%w: length %d", ErrFieldName, len(name))
	}
	if name[0] == '_' || (name[0] >= '0' && name[0] <= '9') {
		return fmt.Errorf("%w: %s", ErrFieldName, strconv.Quote(name))
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return fmt.Errorf("%w: %s", ErrFieldName, strconv.Quote(name))
		}
	}
	return nil
}
