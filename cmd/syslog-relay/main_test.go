// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/logsources"
	"unheaded/pkg/metrics/metricstest"
	"unheaded/pkg/syslog"
)

const (
	// The fixture's senders. Linux answers on all of 127/8, so a client can
	// bind any of these and be a different sender.
	knownAddr    = "127.0.0.1"
	unknownAddr  = "127.0.0.2"
	taggedAddr   = "127.0.0.3"
	excludedAddr = "127.0.0.4"

	waitFor = 10 * time.Second
)

const fixtureRows = `schema: unheaded.logging.sources/v1
sources:
  - {id: test/local, host: west, kind: syslog-device, match: 127.0.0.1, retention: operational, max_silence: 24h, canary: logger, owner: sentinel, status: gap}
  - {id: pihole/dns, host: pihole, kind: syslog-device, match: "127.0.0.3 pihole-dns", retention: operational, max_silence: 24h, canary: logger, owner: sentinel, status: gap}
  - {id: pihole/ftl, host: pihole, kind: syslog-device, match: "127.0.0.3 pihole-ftl", retention: audit, max_silence: 24h, canary: logger, owner: sentinel, status: gap}
  - {id: test/laptop, host: west, kind: syslog-device, match: 127.0.0.4, retention: operational, max_silence: n/a, canary: logger, owner: sentinel, status: "excluded:a workstation, not a log source"}
`

type harness struct {
	t       *testing.T
	r       *relay
	journal *net.UnixConn
	path    string
	sources string
	udp     netip.AddrPort
	tcp     netip.AddrPort
	done    chan struct{}
	cancel  context.CancelFunc
}

func start(t *testing.T, mutate func(*config)) *harness {
	t.Helper()
	return startWith(t, mutate, nil)
}

// startWith starts a relay on fds, sockets systemd would have passed, or
// binding its own listeners when fds is nil.
func startWith(t *testing.T, mutate func(*config), fds []inheritedFD) *harness {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "journal")
	j, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: sock, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	src := filepath.Join(dir, "sources.yaml")
	if err := os.WriteFile(src, []byte(fixtureRows), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config{
		listens: []listenAddr{
			{"udp", netip.MustParseAddrPort("127.0.0.1:0")},
			{"tcp", netip.MustParseAddrPort("127.0.0.1:0")},
		},
		sources:        src,
		rate:           defaultRate,
		burst:          defaultBurst,
		maxMsg:         8192,
		maxConns:       8,
		maxConnsSender: 8,
		idle:           time.Minute,
		journalSocket:  sock,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	r, err := newRelay(cfg, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.listen(fds); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{t: t, r: r, journal: j, path: sock, sources: src, done: make(chan struct{}), cancel: cancel}
	if len(r.udp) > 0 {
		h.udp = r.udp[0].LocalAddr().(*net.UDPAddr).AddrPort()
	}
	if len(r.tcp) > 0 {
		h.tcp = r.tcp[0].Addr().(*net.TCPAddr).AddrPort()
	}
	go func() {
		defer close(h.done)
		r.serve(ctx)
	}()
	t.Cleanup(h.stop)
	return h
}

func (h *harness) stop() {
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(waitFor):
		h.t.Error("relay did not stop")
	}
}

// next waits on the fake journal with a deadline, never a sleep.
func (h *harness) next() map[string]string {
	h.t.Helper()
	if err := h.journal.SetReadDeadline(time.Now().Add(waitFor)); err != nil {
		h.t.Fatal(err)
	}
	buf := make([]byte, 1<<20)
	n, err := h.journal.Read(buf)
	if err != nil {
		h.t.Fatalf("nothing reached the journal: %v", err)
	}
	return decode(h.t, buf[:n])
}

// quiet asserts nothing else is on the journal socket.
func (h *harness) quiet() {
	h.t.Helper()
	if err := h.journal.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		h.t.Fatal(err)
	}
	buf := make([]byte, 1<<16)
	if n, err := h.journal.Read(buf); err == nil {
		h.t.Errorf("unexpected entry %q", buf[:n])
	}
}

func decode(t *testing.T, b []byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	for len(b) > 0 {
		nl := bytes.IndexByte(b, '\n')
		if nl < 0 {
			t.Fatalf("no newline in %q", b)
		}
		if eq := bytes.IndexByte(b[:nl], '='); eq >= 0 {
			out[string(b[:eq])] = string(b[eq+1 : nl])
			b = b[nl+1:]
			continue
		}
		name, rest := string(b[:nl]), b[nl+1:]
		n := binary.LittleEndian.Uint64(rest)
		out[name] = string(rest[8 : 8+n])
		b = rest[8+n+1:]
	}
	return out
}

func (h *harness) eventually(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(waitFor)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	h.t.Fatalf("never happened: %s", what)
}

func (h *harness) dropped(reason string) float64 {
	return h.r.metrics.dropped.WithLabelValues(reason).Value()
}

func (h *harness) sendUDP(from, msg string) {
	h.t.Helper()
	local := &net.UDPAddr{IP: net.ParseIP(from)}
	c, err := net.DialUDP("udp", local, net.UDPAddrFromAddrPort(h.udp))
	if err != nil {
		h.t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte(msg)); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) dialTCP(from string) *net.TCPConn {
	h.t.Helper()
	c, err := net.DialTCP("tcp", &net.TCPAddr{IP: net.ParseIP(from)}, net.TCPAddrFromAddrPort(h.tcp))
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = c.Close() })
	return c
}

// closedByPeer reports whether the relay ended c within the deadline.
func closedByPeer(t *testing.T, c net.Conn) {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(waitFor)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, c); err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatal("the relay kept the connection open")
		}
	}
}

func wantField(t *testing.T, got map[string]string, name, want string) {
	t.Helper()
	if v, ok := got[name]; !ok || v != want {
		t.Errorf("%s = %q (present %v), want %q", name, v, ok, want)
	}
}

func TestUDP5424(t *testing.T) {
	h := start(t, nil)
	h.sendUDP(knownAddr, "<165>1 2003-08-24T05:14:15.000003-07:00 claimed.example myproc 8710 ID47 [a@1 x=\"y\"] hello there")
	got := h.next()
	wantField(t, got, "MESSAGE", "hello there")
	wantField(t, got, "PRIORITY", "5")
	wantField(t, got, "SYSLOG_FACILITY", "20")
	wantField(t, got, "SYSLOG_IDENTIFIER", "myproc")
	wantField(t, got, "REMOTE_ADDR", knownAddr)
	wantField(t, got, "REMOTE_HOSTNAME_CLAIMED", "claimed.example")
	wantField(t, got, "UNHEADED_SOURCE", "test/local")
	wantField(t, got, "SYSLOG_TIMESTAMP", "2003-08-24T05:14:15.000003-07:00")
	wantField(t, got, "SYSLOG_PID", "8710")
	wantField(t, got, "SYSLOG_MSGID", "ID47")
	wantField(t, got, "SYSLOG_STRUCTURED_DATA", `[a@1 x="y"]`)
	for _, absent := range []string{"UNHEADED_TRUNCATED", "UNHEADED_MALFORMED"} {
		if _, ok := got[absent]; ok {
			t.Errorf("%s set on a clean message", absent)
		}
	}
	h.eventually("received counted", func() bool { return h.r.metrics.received.WithLabelValues("udp").Value() == 1 })
}

func TestTCPOctetCounted(t *testing.T) {
	h := start(t, nil)
	c := h.dialTCP(knownAddr)
	one := "<13>1 - host one - - - first"
	two := "<13>1 - host two - - - second"
	if _, err := fmt.Fprintf(c, "%d %s%d %s", len(one), one, len(two), two); err != nil {
		t.Fatal(err)
	}
	wantField(t, h.next(), "MESSAGE", "first")
	got := h.next()
	wantField(t, got, "MESSAGE", "second")
	wantField(t, got, "SYSLOG_IDENTIFIER", "two")
	wantField(t, got, "REMOTE_ADDR", knownAddr)
}

func TestTCPLFFramed(t *testing.T) {
	h := start(t, nil)
	c := h.dialTCP(knownAddr)
	if _, err := io.WriteString(c, "<134>Nov  4 01:02:03 pihole lh-canary: token-1\n<134>Nov  4 01:02:04 pihole lh-canary: token-2\n"); err != nil {
		t.Fatal(err)
	}
	got := h.next()
	wantField(t, got, "MESSAGE", "token-1")
	wantField(t, got, "SYSLOG_IDENTIFIER", "lh-canary")
	wantField(t, got, "REMOTE_HOSTNAME_CLAIMED", "pihole")
	wantField(t, got, "SYSLOG_FACILITY", "16")
	wantField(t, h.next(), "MESSAGE", "token-2")
}

func TestUnknownSenderIsDroppedAndCounted(t *testing.T) {
	h := start(t, nil)
	h.sendUDP(unknownAddr, "<13>1 - - app - - - from a stranger")
	h.eventually("counted", func() bool { return h.r.metrics.unknownSender.Value() == 1 })
	h.quiet()

	c := h.dialTCP(unknownAddr)
	closedByPeer(t, c)
	h.eventually("TCP counted", func() bool { return h.r.metrics.unknownSender.Value() == 2 })
	h.quiet()
}

// The inventory says this address must not be a source, so it is a stranger.
func TestExcludedRowIsNotASender(t *testing.T) {
	h := start(t, nil)
	h.sendUDP(excludedAddr, "<13>1 - - app - - - from the laptop")
	h.eventually("counted", func() bool { return h.r.metrics.unknownSender.Value() == 1 })
	h.quiet()
}

func TestBurstOverTheLimit(t *testing.T) {
	h := start(t, func(c *config) { c.rate, c.burst = 1, 5 })
	conn := h.dialTCP(knownAddr)
	// TCP, so the kernel cannot lose a message and the arithmetic is exact.
	for i := range 20 {
		if _, err := fmt.Fprintf(conn, "<13>1 - - app - - - n%d\n", i); err != nil {
			t.Fatal(err)
		}
	}
	h.eventually("20 received", func() bool { return h.r.metrics.received.WithLabelValues("tcp").Value() == 20 })
	if got := h.dropped(dropRate); got < 13 || got > 15 {
		t.Errorf("dropped %v of 20 with a burst of 5", got)
	}
	for range 20 - int(h.dropped(dropRate)) {
		h.next()
	}
	h.quiet()
}

// One sender's flood does not spend another's tokens.
func TestBucketsArePerSender(t *testing.T) {
	h := start(t, func(c *config) { c.rate, c.burst = 1, 1 })
	h.sendUDP(knownAddr, "<13>1 - - app - - - a")
	h.next()
	h.sendUDP(taggedAddr, "<13>1 - - pihole-dns - - - b")
	wantField(t, h.next(), "MESSAGE", "b")
}

func TestRowLowersTheRate(t *testing.T) {
	now := time.Now()
	rows := []logsources.Sender{
		{Addr: netip.MustParseAddr("192.0.2.1"), ID: "a", Rate: 2, Burst: 3},
		{Addr: netip.MustParseAddr("192.0.2.1"), Tag: "x", ID: "b"},
		{Addr: netip.MustParseAddr("192.0.2.2"), ID: "c", Rate: 5000, Burst: 5000},
	}
	list := buildAllowlist(rows, 200, 1000, now)
	s := list.senders[netip.MustParseAddr("192.0.2.1")]
	if s.bucket.rate != 2 || s.bucket.burst != 3 {
		t.Errorf("rate %v burst %v, want the row's", s.bucket.rate, s.bucket.burst)
	}
	// A row can only lower the limit: a typo must not open the valve.
	s = list.senders[netip.MustParseAddr("192.0.2.2")]
	if s.bucket.rate != 200 || s.bucket.burst != 1000 {
		t.Errorf("rate %v burst %v, want the flags'", s.bucket.rate, s.bucket.burst)
	}
}

func TestOneMiBFrameDoesNotGrowMemoryAndCloses(t *testing.T) {
	h := start(t, nil)
	payload := bytes.Repeat([]byte("A"), 1<<20)
	for _, tt := range []struct {
		name   string
		reason string
		header string
	}{
		{"octet counted", dropFrameLarge, fmt.Sprintf("%d ", len(payload))},
		{"LF framed", dropFrameLarge, "<13>1 - - app - - - "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := h.dropped(tt.reason)
			c := h.dialTCP(knownAddr)
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			allocated := ms.TotalAlloc

			// A write into a closing connection fails; that is the point.
			_, _ = c.Write([]byte(tt.header))
			_, _ = c.Write(payload)
			closedByPeer(t, c)
			h.eventually("counted", func() bool { return h.dropped(tt.reason) == before+1 })

			runtime.ReadMemStats(&ms)
			if grown := ms.TotalAlloc - allocated; grown >= uint64(len(payload)) {
				t.Errorf("allocated %d bytes for a frame over the cap", grown)
			}
			h.quiet()
		})
	}
}

// A frame between the message cap and the frame cap is cut, not refused:
// closing would make omfwd resend it forever.
func TestFrameOverTheMessageCapIsTruncated(t *testing.T) {
	h := start(t, nil)
	c := h.dialTCP(knownAddr)
	msg := "<13>1 - - app - - - " + strings.Repeat("B", 10000)
	if _, err := fmt.Fprintf(c, "%d %s", len(msg), msg); err != nil {
		t.Fatal(err)
	}
	got := h.next()
	wantField(t, got, "UNHEADED_TRUNCATED", "1")
	if n := len(got["MESSAGE"]); n > 8192 || n < 8000 {
		t.Errorf("MESSAGE is %d bytes", n)
	}
	h.eventually("counted", func() bool { return h.r.metrics.truncated.Value() == 1 })
}

func TestOversizeDatagramIsTruncated(t *testing.T) {
	h := start(t, nil)
	h.sendUDP(knownAddr, "<13>1 - - app - - - "+strings.Repeat("C", 20000))
	got := h.next()
	wantField(t, got, "UNHEADED_TRUNCATED", "1")
	if n := len(got["MESSAGE"]); n > 8192 {
		t.Errorf("MESSAGE is %d bytes", n)
	}
}

func TestBadFrameCloses(t *testing.T) {
	h := start(t, nil)
	c := h.dialTCP(knownAddr)
	if _, err := io.WriteString(c, "not a frame\n"); err != nil {
		t.Fatal(err)
	}
	closedByPeer(t, c)
	h.eventually("counted", func() bool { return h.dropped(dropBadFrame) == 1 })
}

func TestStreamEndingInsideAFrame(t *testing.T) {
	h := start(t, nil)
	c := h.dialTCP(knownAddr)
	if _, err := io.WriteString(c, "100 <13>short"); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	closedByPeer(t, c)
	h.eventually("counted", func() bool { return h.dropped(dropShortFrame) == 1 })
	h.quiet()
}

func TestHostileFieldsCannotForgeTheEntry(t *testing.T) {
	h := start(t, nil)
	// Octet counted, so the newline is inside the frame.
	msg := "<13>1 - ho\nst ap\x1b[31mp - - - line\nPRIORITY=0\n_SYSTEMD_UNIT=sshd.service\n\x00tail"
	c := h.dialTCP(knownAddr)
	if _, err := fmt.Fprintf(c, "%d %s", len(msg), msg); err != nil {
		t.Fatal(err)
	}
	got := h.next()
	wantField(t, got, "PRIORITY", "5")
	wantField(t, got, "MESSAGE", `line\x0aPRIORITY=0\x0a_SYSTEMD_UNIT=sshd.service\x0a\x00tail`)
	wantField(t, got, "REMOTE_HOSTNAME_CLAIMED", `ho\x0ast`)
	wantField(t, got, "SYSLOG_IDENTIFIER", `ap\x1b[31mp`)
	for name := range got {
		if strings.HasPrefix(name, "_") {
			t.Errorf("trusted field %s written", name)
		}
	}
}

// L3: journald writes an emerg entry to every login tty (ForwardToWall,
// MaxLevelWall=emerg by default), so no sender's claim may arrive at that level.
// What the sender claimed stays readable in its own field.
func TestEmergCannotReachTheWallPath(t *testing.T) {
	h := start(t, nil)
	for name, tc := range map[string]struct{ msg, facility string }{
		"5424, kernel": {"<0>1 - - app - - - boom", "0"},
		"5424, user":   {"<8>1 - - app - - - boom", "1"},
		"3164":         {"<8>Oct  5 12:00:00 host app: boom", "1"},
		"5424, local7": {"<184>1 - - app - - - boom", "23"},
	} {
		t.Run(name, func(t *testing.T) {
			h.sendUDP(knownAddr, tc.msg)
			got := h.next()
			wantField(t, got, "PRIORITY", "1")
			wantField(t, got, "UNHEADED_SEVERITY_CLAIMED", "0")
			wantField(t, got, "SYSLOG_FACILITY", tc.facility)
		})
	}
}

func TestEveryPRIStaysOffTheWallPath(t *testing.T) {
	for pri := 0; pri <= 191; pri++ {
		m, err := syslog.ParseMax([]byte(fmt.Sprintf("<%d>1 - - app - - - x", pri)), 8192)
		if err != nil {
			t.Fatalf("<%d>: %v", pri, err)
		}
		fields := journalFields(m, netip.MustParseAddr("127.0.0.1"), unknownRow, nil)
		got, err := strconv.Atoi(fields["PRIORITY"])
		if err != nil || got < 1 || got > 7 {
			t.Fatalf("<%d>: PRIORITY %q", pri, fields["PRIORITY"])
		}
		_, flagged := fields["UNHEADED_SEVERITY_CLAIMED"]
		if sev := pri % 8; flagged != (sev == 0) || (sev != 0 && got != sev) {
			t.Fatalf("<%d>: PRIORITY %d, claimed field %v", pri, got, flagged)
		}
	}
}

func TestMalformedIsShippedAndCounted(t *testing.T) {
	h := start(t, nil)
	for _, tt := range []struct{ in, kind string }{
		{"<192>bad pri", "pri"},
		{"<34>1 2003-10-11T22:14:15.003Z cut", "header"},
		{"<13>1 - - app - - [unterminated", "structured_data"},
	} {
		h.sendUDP(knownAddr, tt.in)
		got := h.next()
		wantField(t, got, "UNHEADED_MALFORMED", tt.kind)
		wantField(t, got, "MESSAGE", tt.in)
		wantField(t, got, "PRIORITY", "5")
		wantField(t, got, "SYSLOG_FACILITY", "1")
		wantField(t, got, "UNHEADED_SOURCE", "test/local")
	}
	h.eventually("counted", func() bool { return h.r.metrics.malformed.Value() == 3 })
}

func TestEmptyMessageIsDropped(t *testing.T) {
	h := start(t, nil)
	h.sendUDP(knownAddr, "\n")
	h.eventually("counted", func() bool { return h.dropped(dropEmpty) == 1 })
	h.quiet()
}

func TestTaggedRows(t *testing.T) {
	h := start(t, nil)
	tests := []struct{ from, msg, source, host, retention string }{
		{taggedAddr, "<134>Nov  4 01:02:03 pihole pihole-dns: q", "pihole/dns", "pihole", "operational"},
		{taggedAddr, "<134>Nov  4 01:02:03 pihole pihole-ftl: f", "pihole/ftl", "pihole", "audit"},
		// A known address with a tag no row names: kept, not attributed.
		{taggedAddr, "<134>Nov  4 01:02:03 pihole other: x", sourceUnknown, sourceUnknown, retentionUnknown},
		// A sender only reaches rows on its own address.
		{knownAddr, "<134>Nov  4 01:02:03 pihole pihole-dns: spoof", "test/local", "west", "operational"},
	}
	for _, tt := range tests {
		h.sendUDP(tt.from, tt.msg)
		got := h.next()
		wantField(t, got, "UNHEADED_SOURCE", tt.source)
		wantField(t, got, "UNHEADED_SOURCE_HOST", tt.host)
		wantField(t, got, "UNHEADED_RETENTION", tt.retention)
	}
}

// ADR-124 Consequences 2 (answer 2026-11-09): a sender picks its own PRI, so
// claiming auth (4) or authpriv (10) must not file its line as audit. The
// row decides; the claimed facility stays in its own field.
func TestRetentionIsTheRowsNotTheClaimedFacility(t *testing.T) {
	h := start(t, nil)
	for _, pri := range []string{"<34>", "<38>", "<82>", "<86>"} { // facility 4 and 10
		h.sendUDP(knownAddr, pri+"1 - - sshd - - - Failed password for root from 192.0.2.9 port 22 ssh2")
		got := h.next()
		wantField(t, got, "UNHEADED_RETENTION", "operational")
		if f := got["SYSLOG_FACILITY"]; f != "4" && f != "10" {
			t.Errorf("%s: SYSLOG_FACILITY %q", pri, f)
		}
	}
}

func TestAcceptUnknown(t *testing.T) {
	h := start(t, func(c *config) { c.acceptUnknown = true })
	h.sendUDP(unknownAddr, "<13>1 - - app - - - inventory")
	got := h.next()
	wantField(t, got, "UNHEADED_SOURCE", sourceUnknown)
	wantField(t, got, "UNHEADED_SOURCE_HOST", sourceUnknown)
	wantField(t, got, "UNHEADED_RETENTION", retentionUnknown)
	wantField(t, got, "REMOTE_ADDR", unknownAddr)
	if h.r.metrics.unknownSender.Value() != 1 {
		t.Error("an accepted unknown sender still counts")
	}
	c := h.dialTCP(unknownAddr)
	if _, err := io.WriteString(c, "<13>1 - - app - - - over tcp\n"); err != nil {
		t.Fatal(err)
	}
	wantField(t, h.next(), "MESSAGE", "over tcp")
}

// Spoofed sources under --accept-unknown share one bucket and allocate nothing.
func TestUnknownSendersShareOneBucket(t *testing.T) {
	h := start(t, func(c *config) {
		c.acceptUnknown = true
		c.rate, c.burst = 1, 1
	})
	h.sendUDP("127.0.0.10", "<13>1 - - app - - - one")
	h.next()
	h.sendUDP("127.0.0.11", "<13>1 - - app - - - two")
	h.eventually("rate limited", func() bool { return h.dropped(dropRate) == 1 })
	if n := len(h.r.list.Load().senders); n != 2 {
		t.Errorf("%d senders tracked; unknown senders must not add any", n)
	}
}

func TestReload(t *testing.T) {
	h := start(t, nil)
	h.sendUDP(unknownAddr, "<13>1 - - app - - - before")
	h.eventually("dropped", func() bool { return h.r.metrics.unknownSender.Value() == 1 })

	rows := fixtureRows + `  - {id: test/new, host: west, kind: syslog-device, match: 127.0.0.2, retention: operational, max_silence: 24h, canary: logger, owner: sentinel, status: gap}
`
	if err := os.WriteFile(h.sources, []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}
	h.r.reload()
	h.sendUDP(unknownAddr, "<13>1 - - app - - - after")
	wantField(t, h.next(), "UNHEADED_SOURCE", "test/new")
}

// A typo in the file must not open the door or close it.
func TestReloadFailureKeepsTheList(t *testing.T) {
	h := start(t, nil)
	if err := os.WriteFile(h.sources, []byte("schema: wrong\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.r.reload()
	if h.r.metrics.reloadFailures.Value() != 1 {
		t.Error("failure not counted")
	}
	h.sendUDP(knownAddr, "<13>1 - - app - - - still here")
	wantField(t, h.next(), "UNHEADED_SOURCE", "test/local")
}

// Removing a row ends that sender's open connection at its next message.
func TestReloadRemovingARowEndsTheConnection(t *testing.T) {
	h := start(t, nil)
	c := h.dialTCP(knownAddr)
	if _, err := io.WriteString(c, "<13>1 - - app - - - before\n"); err != nil {
		t.Fatal(err)
	}
	h.next()
	rows := strings.Replace(fixtureRows, "match: 127.0.0.1,", "match: 127.0.0.9,", 1)
	if err := os.WriteFile(h.sources, []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}
	h.r.reload()
	if _, err := io.WriteString(c, "<13>1 - - app - - - after\n"); err != nil {
		t.Fatal(err)
	}
	closedByPeer(t, c)
	h.quiet()
}

func TestConnectionCap(t *testing.T) {
	h := start(t, func(c *config) { c.maxConns = 2 })
	held := []*net.TCPConn{h.dialTCP(knownAddr), h.dialTCP(knownAddr)}
	for i, c := range held {
		if _, err := fmt.Fprintf(c, "<13>1 - - app - - - held%d\n", i); err != nil {
			t.Fatal(err)
		}
		h.next()
	}
	over := h.dialTCP(knownAddr)
	closedByPeer(t, over)
	h.eventually("counted", func() bool { return h.dropped(dropConns) == 1 })

	// Freeing a slot frees a slot.
	if err := held[0].Close(); err != nil {
		t.Fatal(err)
	}
	h.eventually("slot back", func() bool { return len(h.r.slots) == 1 })
	c := h.dialTCP(knownAddr)
	if _, err := io.WriteString(c, "<13>1 - - app - - - again\n"); err != nil {
		t.Fatal(err)
	}
	wantField(t, h.next(), "MESSAGE", "again")
}

// L3: one sender is held to its own share, and the rest of the slots stay free
// for everyone else.
func TestOneSenderCannotTakeEverySlot(t *testing.T) {
	h := start(t, func(c *config) { c.maxConns, c.maxConnsSender = 4, 2 })
	held := []*net.TCPConn{h.dialTCP(knownAddr), h.dialTCP(knownAddr)}
	for i, c := range held {
		if _, err := fmt.Fprintf(c, "<13>1 - - app - - - held%d\n", i); err != nil {
			t.Fatal(err)
		}
		h.next()
	}
	over := h.dialTCP(knownAddr)
	closedByPeer(t, over)
	h.eventually("counted", func() bool { return h.dropped(dropSenderConns) == 1 })
	if got := h.dropped(dropConns); got != 0 {
		t.Fatalf("the global cap counted %v; it was not reached", got)
	}

	// Another sender still gets a slot.
	other := h.dialTCP(taggedAddr)
	if _, err := io.WriteString(other, "<13>1 - - pihole-dns - - - other\n"); err != nil {
		t.Fatal(err)
	}
	wantField(t, h.next(), "MESSAGE", "other")

	// A closed connection gives the sender its share back.
	if err := held[0].Close(); err != nil {
		t.Fatal(err)
	}
	h.eventually("slot back", func() bool { return len(h.r.slots) == 2 })
	again := h.dialTCP(knownAddr)
	if _, err := io.WriteString(again, "<13>1 - - app - - - again\n"); err != nil {
		t.Fatal(err)
	}
	wantField(t, h.next(), "MESSAGE", "again")
}

// A refused or finished connection leaves no entry behind, so a stream of
// addresses under --accept-unknown cannot grow the table.
func TestSenderTableEmptiesWhenConnectionsEnd(t *testing.T) {
	h := start(t, func(c *config) { c.maxConns, c.maxConnsSender = 4, 1 })
	first := h.dialTCP(knownAddr)
	if _, err := io.WriteString(first, "<13>1 - - app - - - held\n"); err != nil {
		t.Fatal(err)
	}
	h.next()
	closedByPeer(t, h.dialTCP(knownAddr))
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	h.eventually("table empty", func() bool {
		h.r.perSenderMu.Lock()
		defer h.r.perSenderMu.Unlock()
		return len(h.r.perSender) == 0
	})
}

func TestIdleConnectionCloses(t *testing.T) {
	h := start(t, func(c *config) { c.idle = 50 * time.Millisecond })
	closedByPeer(t, h.dialTCP(knownAddr))
	if h.dropped(dropBadFrame) != 0 || h.dropped(dropShortFrame) != 0 {
		t.Error("an idle close is not a drop")
	}
}

// The deadline is per frame: dribbling does not hold a slot.
func TestDribblingSenderCloses(t *testing.T) {
	h := start(t, func(c *config) { c.idle = 100 * time.Millisecond })
	c := h.dialTCP(knownAddr)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
				if _, err := c.Write([]byte("x")); err != nil {
					return
				}
			}
		}
	}()
	if _, err := io.WriteString(c, "<13>"); err != nil {
		t.Fatal(err)
	}
	closedByPeer(t, c)
	h.quiet()
}

func TestJournalGoneIsCounted(t *testing.T) {
	h := start(t, nil)
	if err := h.journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(h.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	h.sendUDP(knownAddr, "<13>1 - - app - - - nowhere to go")
	h.eventually("counted", func() bool { return h.dropped(dropJournal) == 1 })
}

func TestLastSeenOnlyForWrittenMessages(t *testing.T) {
	h := start(t, nil)
	h.sendUDP(knownAddr, "<13>1 - - app - - - x")
	h.next()
	h.eventually("stamped", func() bool { return h.r.metrics.lastSeen.WithLabelValues("test/local").Value() > 0 })
}

func TestMetricsEndpoint(t *testing.T) {
	h := start(t, nil)
	h.sendUDP(knownAddr, "<13>1 - - app - - - x")
	h.next()
	srv := httptest.NewServer(h.r.metricsHandler())
	defer srv.Close()
	body := get(t, srv.URL+"/metrics", http.StatusOK)
	for _, want := range []string{
		`syslog_relay_received_total{transport="udp"}`,
		`syslog_relay_received_total{transport="tcp"} 0`,
		`syslog_relay_dropped_total{reason="rate"} 0`,
		`syslog_relay_dropped_total{reason="journal"} 0`,
		`syslog_relay_unknown_sender_total 0`,
		`syslog_relay_source_last_seen_timestamp_seconds{source="test/local"}`,
		`syslog_relay_tcp_connections 0`,
		`go_goroutines`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics has no %s", want)
		}
	}
	for _, problem := range metricstest.Lint(body) {
		t.Errorf("lint: %s", problem)
	}
	if !strings.Contains(get(t, srv.URL+"/health", http.StatusOK), "healthy") {
		t.Error("/health")
	}
	get(t, srv.URL+"/ready", http.StatusOK)
}

func get(t *testing.T, url string, want int) string {
	t.Helper()
	resp, err := http.Get(url) // #nosec G107 -- the test's own server
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != want {
		t.Fatalf("%s: %d", url, resp.StatusCode)
	}
	return string(b)
}

// A metrics address that cannot bind must not stop logs, and comes up when it
// can. The overlay address may appear after the relay starts.
func TestMetricsListenerRetries(t *testing.T) {
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := blocker.Addr().String()
	h := start(t, func(c *config) {
		c.metrics = []string{addr}
		c.metricsRetry = 5 * time.Millisecond
	})
	h.sendUDP(knownAddr, "<13>1 - - app - - - logs flow without metrics")
	wantField(t, h.next(), "MESSAGE", "logs flow without metrics")
	if err := blocker.Close(); err != nil {
		t.Fatal(err)
	}
	h.eventually("metrics listener up", func() bool {
		resp, err := http.Get("http://" + addr + "/health") // #nosec G107 -- the test's own server
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	})
}

func TestBucket(t *testing.T) {
	t0 := time.Unix(1000, 0)
	b := newBucket(2, 3, t0)
	for i := range 3 {
		if !b.allow(t0) {
			t.Fatalf("token %d refused", i)
		}
	}
	if b.allow(t0) {
		t.Error("over the burst")
	}
	if !b.allow(t0.Add(500 * time.Millisecond)) {
		t.Error("half a second at 2/s is one token")
	}
	if b.allow(t0.Add(500 * time.Millisecond)) {
		t.Error("only one token came back")
	}
	if b.allow(t0.Add(100 * time.Millisecond)) {
		t.Error("a clock stepping back must not mint tokens")
	}
	for i := range 3 {
		if !b.allow(t0.Add(time.Hour)) {
			t.Fatalf("after an hour, token %d refused", i)
		}
	}
	if b.allow(t0.Add(time.Hour)) {
		t.Error("tokens exceeded the burst")
	}
}

func TestParseFlags(t *testing.T) {
	ok := []string{"--listen", "udp://10.10.20.1:514", "--listen", "tcp://[2001:db8::1]:514"}
	c, err := parseFlags(ok, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.listens) != 2 || c.listens[0].network != "udp" || c.listens[1].network != "tcp" {
		t.Errorf("listens %+v", c.listens)
	}
	if len(c.metrics) != 1 || c.metrics[0] != "127.0.0.1:9599" {
		t.Errorf("metrics %v", c.metrics)
	}
	if c.rate != 200 || c.burst != 1000 || c.maxMsg != 8192 || c.frameCap() != 16384 {
		t.Errorf("defaults %+v", c)
	}
	if got := c.senderCap(); got != 16 {
		t.Errorf("default per-sender cap %d, want a sixteenth of 256", got)
	}
	c.maxConns = 8
	if got := c.senderCap(); got != 1 {
		t.Errorf("per-sender cap at --max-conns 8 is %d, want at least 1", got)
	}

	c, err = parseFlags(append(ok, "--metrics", "127.0.0.1:9599", "--metrics", "[fd00:dead:beef::1]:9599"), io.Discard, io.Discard)
	if err != nil || len(c.metrics) != 2 {
		t.Errorf("metrics %v, %v", c.metrics, err)
	}

	for name, args := range map[string][]string{
		"no listener":       {},
		"wildcard v4":       {"--listen", "udp://0.0.0.0:514"},
		"wildcard v6":       {"--listen", "tcp://[::]:514"},
		"no scheme":         {"--listen", "10.10.20.1:514"},
		"other scheme":      {"--listen", "tls://10.10.20.1:514"},
		"a name":            {"--listen", "udp://west:514"},
		"no port":           {"--listen", "udp://10.10.20.1"},
		"wildcard metrics":  append(ok[:2:2], "--metrics", "0.0.0.0:9599"),
		"max-msg too small": append(ok[:2:2], "--max-msg", "100"),
		"max-msg too large": append(ok[:2:2], "--max-msg", "1000000"),
		"zero rate":         append(ok[:2:2], "--rate", "0"),
		"zero burst":        append(ok[:2:2], "--burst", "0"),
		"zero conns":        append(ok[:2:2], "--max-conns", "0"),
		"zero idle":         append(ok[:2:2], "--idle-timeout", "0s"),
		"negative share":    append(ok[:2:2], "--max-conns-per-sender", "-1"),
		"share = cap":       append(ok[:2:2], "--max-conns", "8", "--max-conns-per-sender", "8"),
		"share over cap":    append(ok[:2:2], "--max-conns-per-sender", "257"),
		"stray operand":     append(ok[:2:2], "extra"),
		"unknown flag":      {"--nope"},
	} {
		if _, err := parseFlags(args, io.Discard, io.Discard); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestHelpAndMan(t *testing.T) {
	var out bytes.Buffer
	_, err := parseFlags([]string{"--help"}, &out, io.Discard)
	var exit *cmdline.ExitError
	if !errors.As(err, &exit) || exit.Code != 0 {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"--listen", "--accept-unknown", "--max-msg"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help has no %s", want)
		}
	}
	out.Reset()
	if _, err := parseFlags([]string{"--man"}, &out, io.Discard); !errors.As(err, &exit) || !strings.HasPrefix(out.String(), ".TH") {
		t.Errorf("--man: %v, %q", err, out.String())
	}
}

func TestRunRefusesAMissingJournal(t *testing.T) {
	cfg := config{journalSocket: filepath.Join(t.TempDir(), "absent")}
	if err := run(context.Background(), cfg, zerolog.Nop()); err == nil {
		t.Error("started with no journal socket")
	}
	file := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.journalSocket = file
	if err := run(context.Background(), cfg, zerolog.Nop()); err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Errorf("err = %v", err)
	}
}

func TestRunRefusesBadSources(t *testing.T) {
	h := start(t, nil)
	cfg := h.r.cfg
	cfg.sources = filepath.Join(t.TempDir(), "absent.yaml")
	if err := run(context.Background(), cfg, zerolog.Nop()); err == nil {
		t.Error("started with no sources file")
	}
}

// run takes SIGTERM's context and stops cleanly; the address is already held,
// so a second relay on it fails loudly at bind.
func TestRunBindFailure(t *testing.T) {
	h := start(t, nil)
	cfg := h.r.cfg
	cfg.listens = []listenAddr{{"tcp", h.tcp}}
	if err := run(context.Background(), cfg, zerolog.Nop()); err == nil {
		t.Error("bound the same address twice")
	}
}

func TestRunServesUntilCancelled(t *testing.T) {
	h := start(t, nil)
	cfg := h.r.cfg
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, zerolog.Nop()) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run: %v", err)
		}
	case <-time.After(waitFor):
		t.Fatal("run did not stop")
	}
}
