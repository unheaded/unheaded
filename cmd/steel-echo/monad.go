// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/monadpkt"
)

// The Phase 13 lab traffic source and counter (S-REAL-STEEL D-25).
//
//	steel-echo monad-send  --iface steel0-h --dst fd00:5ee1::2 --flow-label 0x5EE1 --count 1000
//	steel-echo monad-count --iface steel0-n --seconds 30
//
// monad-send writes Ethernet/IPv6/HbH(Monad)/UDP frames straight to the
// interface (AF_PACKET), so yaldabaoth-ebpf at TCX egress and the prio/fw/
// netem tree both see them. monad-count reads frames on the peer and keeps,
// per flow label, how many arrived and whether the Monad CHAOS bit (0x80)
// was set and the CRC valid. It reads header bytes only (the first 86) and
// never the payload. Both need CAP_NET_RAW (the lead runs them with sudo in
// the lab netns). The interfaces are fenced to steel<N>-{h,n}: this never
// writes to a physical NIC.

var labIfaceRe = regexp.MustCompile(`^steel[0-9]+-[hn]$`)

const (
	maxSendCount = 1_000_000
	maxCountSecs = 3600
)

type sendConfig struct {
	iface     string
	dst       net.IP
	src       net.IP
	dstMAC    [6]byte
	flowLabel uint32
	count     int
	interval  time.Duration
	srcSvc    uint
	dstSvc    uint
	size      int
}

func parseMACFlag(s string) ([6]byte, error) {
	var out [6]byte
	if s == "" {
		return out, nil
	}
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return out, fmt.Errorf("--dst-mac %q: want aa:bb:cc:dd:ee:ff", s)
	}
	copy(out[:], hw)
	return out, nil
}

func parseIPv6Flag(name, s string) (net.IP, error) {
	if s == "" {
		return nil, nil
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() != nil || ip.To16() == nil {
		return nil, fmt.Errorf("--%s %q: want an IPv6 address", name, s)
	}
	return ip, nil
}

func parseSendFlags(args []string) (sendConfig, error) {
	var c sendConfig
	var dst, src, mac, fl string
	fs := flag.NewFlagSet("monad-send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.iface, "iface", "", "lab interface to write frames to (steel0-h)")
	fs.StringVar(&dst, "dst", "", "IPv6 destination (fd00:5ee1::2)")
	fs.StringVar(&src, "src", "fd00:5ee1::1", "IPv6 source")
	fs.StringVar(&mac, "dst-mac", "", "destination MAC (default 33:33:00:00:00:01, IPv6 all-nodes)")
	fs.StringVar(&fl, "flow-label", "", "IPv6 flow label (0x5EE1 or decimal)")
	fs.IntVar(&c.count, "count", 1000, "frames to send (1..1000000)")
	fs.DurationVar(&c.interval, "interval", time.Millisecond, "gap between frames (0..1s)")
	fs.UintVar(&c.srcSvc, "src-svc", 1, "Monad src_service_id (0..255)")
	fs.UintVar(&c.dstSvc, "dst-svc", 2, "Monad dst_service_id (0..255)")
	fs.IntVar(&c.size, "size", monadpkt.HeaderLen, "frame length in bytes (86..1500)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() != 0 {
		return c, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if !labIfaceRe.MatchString(c.iface) {
		return c, fmt.Errorf("--iface %q: want a lab interface (steel<N>-h or steel<N>-n)", c.iface)
	}
	var err error
	if c.dst, err = parseIPv6Flag("dst", dst); err != nil {
		return c, err
	}
	if c.dst == nil {
		return c, errors.New("set --dst")
	}
	if c.src, err = parseIPv6Flag("src", src); err != nil {
		return c, err
	}
	if c.dstMAC, err = parseMACFlag(mac); err != nil {
		return c, err
	}
	if fl == "" {
		return c, errors.New("set --flow-label")
	}
	if c.flowLabel, err = monadpkt.ParseFlowLabel(fl); err != nil {
		return c, err
	}
	switch {
	case c.count < 1 || c.count > maxSendCount:
		return c, fmt.Errorf("--count %d: want 1..%d", c.count, maxSendCount)
	case c.interval < 0 || c.interval > time.Second:
		return c, fmt.Errorf("--interval %v: want 0..1s", c.interval)
	case c.srcSvc > 255 || c.dstSvc > 255:
		return c, errors.New("--src-svc/--dst-svc: want 0..255")
	case c.size < monadpkt.HeaderLen || c.size > 1500:
		return c, fmt.Errorf("--size %d: want %d..1500", c.size, monadpkt.HeaderLen)
	}
	return c, nil
}

// frame builds the one frame monad-send repeats.
func (c sendConfig) frame(srcMAC [6]byte) ([]byte, error) {
	return monadpkt.Build(monadpkt.Frame{
		FlowLabel: c.flowLabel, Src: c.src, Dst: c.dst, SrcMAC: srcMAC, DstMAC: c.dstMAC,
		SrcSvc: uint8(c.srcSvc), DstSvc: uint8(c.dstSvc), // #nosec G115 -- checked <= 255
		SrcPort: 26600, DstPort: 26600, Len: c.size,
	})
}

// frameWriter is the raw socket (monad_linux.go) or a test sink.
type frameWriter interface {
	Write(b []byte) error
}

// sendFrames writes the frame count times, interval apart, until ctx ends.
func sendFrames(ctx context.Context, w frameWriter, frame []byte, count int, interval time.Duration) (int, error) {
	sent := 0
	var tick *time.Ticker
	if interval > 0 {
		tick = time.NewTicker(interval)
		defer tick.Stop()
	}
	for sent < count {
		if err := ctx.Err(); err != nil {
			return sent, err
		}
		if err := w.Write(frame); err != nil {
			return sent, err
		}
		sent++
		if tick != nil && sent < count {
			select {
			case <-ctx.Done():
				return sent, ctx.Err()
			case <-tick.C:
			}
		}
	}
	return sent, nil
}

// --- monad-count ---

type countConfig struct {
	iface   string
	seconds int
}

func parseCountFlags(args []string) (countConfig, error) {
	var c countConfig
	fs := flag.NewFlagSet("monad-count", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.iface, "iface", "", "lab interface to read frames from (steel0-n)")
	fs.IntVar(&c.seconds, "seconds", 30, "how long to count (1..3600)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() != 0 {
		return c, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if !labIfaceRe.MatchString(c.iface) {
		return c, fmt.Errorf("--iface %q: want a lab interface (steel<N>-h or steel<N>-n)", c.iface)
	}
	if c.seconds < 1 || c.seconds > maxCountSecs {
		return c, fmt.Errorf("--seconds %d: want 1..%d", c.seconds, maxCountSecs)
	}
	return c, nil
}

// flowCount is one flow label's tally.
type flowCount struct {
	Received   int `json:"received"`
	ChaosSet   int `json:"chaos_set"`
	ChaosClear int `json:"chaos_clear"`
	CRCOK      int `json:"crc_ok"`
	CRCBad     int `json:"crc_bad"`
}

// countReport is monad-count's JSON.
type countReport struct {
	Iface    string                `json:"iface"`
	Seconds  int                   `json:"seconds"`
	Frames   int                   `json:"frames"`    // every frame seen
	NonMonad int                   `json:"non_monad"` // IPv6 without our HbH option, ND, anything else
	Flows    map[string]*flowCount `json:"flows"`     // key "0x5ee1"
}

func newCountReport(iface string, seconds int) *countReport {
	return &countReport{Iface: iface, Seconds: seconds, Flows: map[string]*flowCount{}}
}

// add tallies one frame from its headers only.
func (r *countReport) add(frame []byte) {
	r.Frames++
	in, err := monadpkt.Parse(frame)
	if err != nil {
		r.NonMonad++
		return
	}
	key := fmt.Sprintf("0x%x", in.FlowLabel)
	fc := r.Flows[key]
	if fc == nil {
		fc = &flowCount{}
		r.Flows[key] = fc
	}
	fc.Received++
	if in.Chaos {
		fc.ChaosSet++
	} else {
		fc.ChaosClear++
	}
	if in.CRCOK {
		fc.CRCOK++
	} else {
		fc.CRCBad++
	}
}

func (r *countReport) write(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// summary is one line per flow for stderr.
func (r *countReport) summary() string {
	keys := make([]string, 0, len(r.Flows))
	for k := range r.Flows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	for _, k := range keys {
		f := r.Flows[k]
		parts = append(parts, fmt.Sprintf("%s received=%d chaos_set=%d chaos_clear=%d crc_bad=%d", k, f.Received, f.ChaosSet, f.ChaosClear, f.CRCBad))
	}
	parts = append(parts, fmt.Sprintf("non_monad=%d", r.NonMonad))
	return strings.Join(parts, "; ")
}

// frameReader is the raw socket (monad_linux.go) or a test source. Read
// returns the next frame's header bytes (at most monadpkt.HeaderLen).
type frameReader interface {
	Read(ctx context.Context) ([]byte, error)
}

// countFrames tallies frames until the deadline or ctx ends.
func countFrames(ctx context.Context, rd frameReader, r *countReport, d time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	for {
		b, err := rd.Read(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if b != nil {
			r.add(b)
		}
	}
}
