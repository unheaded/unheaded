// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

// Options configure Load.
type Options struct {
	// PinDir pins every map by name under this directory (0700), so another
	// process (upc-cartridgectl) can find a loaded interpreter. "" = no pins.
	PinDir string
	// EventsGID, when > 0, makes the CART_EVENTS pin root:EventsGID 0660 so
	// a reader in that group (cmd/anomaly) can drain scores without CAP_BPF.
	// The other pins, including the signed ROM slots, stay root 0600. The
	// reader still can't traverse PinDir; it gets the pin by bind mount.
	EventsGID int
}

// Cart is a loaded cartridge-ebpf object.
type Cart struct {
	coll *ebpf.Collection
	// Verifier is processed-insn counts per program ("cart_xdp", "cart_tick").
	Verifier map[string]int
	pinDir   string
}

var processedRE = regexp.MustCompile(`processed (\d+) insns`)

// Load loads cartridge-ebpf from objPath with verifier statistics on. It
// attaches nothing.
func Load(objPath string, opts Options) (*Cart, error) {
	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		return nil, fmt.Errorf("cartridge: spec: %w", err)
	}
	copts := ebpf.CollectionOptions{Programs: ebpf.ProgramOptions{LogLevel: ebpf.LogLevelStats}}
	if os.Getenv("CARTRIDGE_VERIFIER_LOG") != "" {
		// Debug: the full per-instruction log (large) instead of the stats line.
		copts.Programs.LogLevel = ebpf.LogLevelBranch | ebpf.LogLevelStats
		copts.Programs.LogSizeStart = 1 << 30
	}
	if opts.PinDir != "" {
		if err := os.MkdirAll(opts.PinDir, 0o700); err != nil {
			return nil, fmt.Errorf("cartridge: pin dir: %w", err)
		}
		for _, m := range spec.Maps {
			m.Pinning = ebpf.PinByName
		}
		copts.Maps.PinPath = opts.PinDir
	}
	coll, err := ebpf.NewCollectionWithOptions(spec, copts)
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			if p := os.Getenv("CARTRIDGE_VERIFIER_LOG"); p != "" {
				_ = os.WriteFile(p, []byte(fmt.Sprintf("%+v", ve)), 0o600) // #nosec G703 -- operator-chosen debug path
			}
			return nil, fmt.Errorf("cartridge: verifier: %v", ve)
		}
		return nil, fmt.Errorf("cartridge: load: %w", err)
	}
	c := &Cart{coll: coll, Verifier: map[string]int{}, pinDir: opts.PinDir}
	for name, p := range coll.Programs {
		if m := processedRE.FindStringSubmatch(p.VerifierLog); m != nil {
			n, _ := strconv.Atoi(m[1])
			c.Verifier[name] = n
		}
	}
	if opts.PinDir != "" {
		// Pin the programs too, so the interpreter outlives this process and
		// another invocation (upc-cartridgectl load/bench/pump) can run it.
		for name, p := range coll.Programs {
			if err := p.Pin(filepath.Join(opts.PinDir, name)); err != nil {
				_ = c.Unpin()
				coll.Close()
				return nil, fmt.Errorf("cartridge: pin %s: %w", name, err)
			}
		}
		if opts.EventsGID > 0 {
			ev := filepath.Join(opts.PinDir, "CART_EVENTS")
			err := os.Chown(ev, -1, opts.EventsGID)
			if err == nil {
				err = os.Chmod(ev, 0o660) // #nosec G302 -- group read+write is the grant: the ringbuf consumer page is mapped writable
			}
			if err != nil {
				_ = c.Unpin()
				coll.Close()
				return nil, fmt.Errorf("cartridge: CART_EVENTS group: %w", err)
			}
		}
	}
	return c, nil
}

// Map and program names in the object (and under a pin directory).
var (
	MapNames     = []string{"CART_ROM", "CART_DATA", "CART_SLOT", "FLOW_STATE", "FLOW_META", "SCORE_QUEUE", "TICK_JOB", "CART_EVENTS", "CART_STATS", "CART_CFG", "BREAKER", "ZERO_STATE", "CART_TRACE", "TRACE_SEQ", "SLOT_ARGS"}
	ProgramNames = []string{"cart_xdp", "cart_tick"}
)

// Open attaches to an interpreter another process loaded with
// Options.PinDir (its pinned maps and programs). Close releases this
// process's references only; Unpin removes the pins.
func Open(pinDir string) (*Cart, error) {
	coll := &ebpf.Collection{Maps: map[string]*ebpf.Map{}, Programs: map[string]*ebpf.Program{}}
	fail := func(err error) (*Cart, error) {
		coll.Close()
		return nil, err
	}
	for _, n := range MapNames {
		m, err := ebpf.LoadPinnedMap(filepath.Join(pinDir, n), nil)
		if err != nil {
			return fail(fmt.Errorf("cartridge: open map %s: %w (is the interpreter up?)", n, err))
		}
		coll.Maps[n] = m
	}
	for _, n := range ProgramNames {
		p, err := ebpf.LoadPinnedProgram(filepath.Join(pinDir, n), nil)
		if err != nil {
			return fail(fmt.Errorf("cartridge: open program %s: %w", n, err))
		}
		coll.Programs[n] = p
	}
	return &Cart{coll: coll, Verifier: map[string]int{}, pinDir: pinDir}, nil
}

// OpenEvents opens only the pinned CART_EVENTS ring under pinDir, for a
// reader that has no right to the other maps (see Options.EventsGID).
// Events and DrainEvents work on the result; nothing else does.
func OpenEvents(pinDir string) (*Cart, error) {
	m, err := ebpf.LoadPinnedMap(filepath.Join(pinDir, "CART_EVENTS"), nil)
	if err != nil {
		return nil, fmt.Errorf("cartridge: open map CART_EVENTS: %w (is the interpreter up?)", err)
	}
	coll := &ebpf.Collection{Maps: map[string]*ebpf.Map{"CART_EVENTS": m}, Programs: map[string]*ebpf.Program{}}
	return &Cart{coll: coll, Verifier: map[string]int{}}, nil
}

// Close unloads the programs and closes the maps. Pins (if any) stay until
// Unpin.
func (c *Cart) Close() {
	if c != nil && c.coll != nil {
		c.coll.Close()
	}
}

// Unpin removes every map pin this Cart created.
func (c *Cart) Unpin() error {
	var errs []error
	for _, m := range c.coll.Maps {
		if m.IsPinned() {
			errs = append(errs, m.Unpin())
		}
	}
	for _, p := range c.coll.Programs {
		if p.IsPinned() {
			errs = append(errs, p.Unpin())
		}
	}
	if c.pinDir != "" {
		errs = append(errs, os.Remove(c.pinDir))
	}
	return errors.Join(errs...)
}

func (c *Cart) m(name string) *ebpf.Map { return c.coll.Maps[name] }

type mapWriter struct{ c *Cart }

func (w mapWriter) putROM(slot, i int, x uint32) error {
	return w.c.m("CART_ROM").Put(uint32(slot*SlotWords+i), x) // #nosec G115 -- slot < 4, i < 1024
}
func (w mapWriter) putData(slot, i int, x uint32) error {
	return w.c.m("CART_DATA").Put(uint32(slot*SlotWords+i), x) // #nosec G115 -- slot < 4, i < 1024
}
func (w mapWriter) putSlot(slot int, cfg SlotCfg) error {
	return w.c.m("CART_SLOT").Put(uint32(slot), cfg.MarshalBinary()) // #nosec G115 -- slot < 4
}

// WriteSlot loads img into slot with generation gen (config written last).
func (c *Cart) WriteSlot(slot int, img *Image, gen uint32) error {
	return writeSlot(mapWriter{c}, slot, img, gen)
}

// SlotConfig reads a slot's configuration.
func (c *Cart) SlotConfig(slot int) (SlotCfg, error) {
	var b []byte
	if err := c.m("CART_SLOT").Lookup(uint32(slot), &b); err != nil { // #nosec G115 -- caller-bounded
		return SlotCfg{}, err
	}
	return UnmarshalSlotCfg(b)
}

// SetSlotActive flips a slot's active byte, keeping the rest.
func (c *Cart) SetSlotActive(slot int, on bool) error {
	cfg, err := c.SlotConfig(slot)
	if err != nil {
		return err
	}
	cfg.Active = 0
	if on {
		cfg.Active = 1
	}
	return mapWriter{c}.putSlot(slot, cfg)
}

// SetCfg writes a CART_CFG knob.
func (c *Cart) SetCfg(key uint32, val uint64) error { return c.m("CART_CFG").Put(key, val) }

// RunXDP runs cart_xdp once on pkt through BPF_PROG_TEST_RUN.
func (c *Cart) RunXDP(pkt []byte) (uint32, error) {
	return c.coll.Programs["cart_xdp"].Run(&ebpf.RunOptions{Data: pkt})
}

// BenchXDP runs cart_xdp repeat times on pkt and returns the mean per run.
func (c *Cart) BenchXDP(pkt []byte, repeat int) (uint32, time.Duration, error) {
	return c.coll.Programs["cart_xdp"].Benchmark(pkt, repeat, nil)
}

// tickPkt is the dummy frame cart_tick runs on (it never reads it).
var tickPkt = make([]byte, 64)

// Tick runs cart_tick once.
func (c *Cart) Tick() error {
	_, err := c.coll.Programs["cart_tick"].Run(&ebpf.RunOptions{Data: tickPkt})
	return err
}

// BenchTick runs cart_tick repeat times and returns the mean per call.
func (c *Cart) BenchTick(repeat int) (time.Duration, error) {
	_, d, err := c.coll.Programs["cart_tick"].Benchmark(tickPkt, repeat, nil)
	return d, err
}

// State reads a flow's state for key (key.Slot selects the slot).
func (c *Cart) State(key FlowKey) (CartState, bool, error) {
	var b []byte
	err := c.m("FLOW_STATE").Lookup(key.MarshalBinary(), &b)
	if errors.Is(err, ebpf.ErrKeyNotExist) {
		return CartState{}, false, nil
	}
	if err != nil {
		return CartState{}, false, err
	}
	s, err := DecodeState(b)
	return s, err == nil, err
}

// FlowStates iterates every FLOW_STATE entry.
func (c *Cart) FlowStates() (map[FlowKey]CartState, error) {
	out := map[FlowKey]CartState{}
	var k, v []byte
	it := c.m("FLOW_STATE").Iterate()
	for it.Next(&k, &v) {
		s, err := DecodeState(v)
		if err != nil {
			return nil, err
		}
		out[decodeFlowKey(k)] = s
	}
	return out, it.Err()
}

// Stats sums the per-CPU CART_STATS counters.
func (c *Cart) Stats() ([StatCount]uint64, error) {
	var out [StatCount]uint64
	for i := range out {
		var per []uint64
		if err := c.m("CART_STATS").Lookup(uint32(i), &per); err != nil { // #nosec G115 -- i < 40
			return out, err
		}
		for _, v := range per {
			out[i] += v
		}
	}
	return out, nil
}

// QueueNonEmpty peeks SCORE_QUEUE (a BPF queue has no length; peek is the
// only non-consuming read).
func (c *Cart) QueueNonEmpty() bool {
	var b []byte
	return c.m("SCORE_QUEUE").Lookup(nil, &b) == nil
}

// Trace opens a reader on CART_TRACE (records of traced runs).
func (c *Cart) Trace() (*ringbuf.Reader, error) { return ringbuf.NewReader(c.m("CART_TRACE")) }

// DrainTrace reads every record currently in CART_TRACE (non-blocking).
func (c *Cart) DrainTrace(rd *ringbuf.Reader) ([][]byte, error) {
	var out [][]byte
	rd.SetDeadline(time.Now().Add(20 * time.Millisecond))
	for {
		rec, err := rd.Read()
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return out, nil
			}
			return out, err
		}
		out = append(out, append([]byte(nil), rec.RawSample...))
	}
}

// PopJob removes the oldest SCORE_QUEUE entry (ok = false when empty).
func (c *Cart) PopJob() (ScoreJob, bool, error) {
	var b []byte
	err := c.m("SCORE_QUEUE").LookupAndDelete(nil, &b)
	if errors.Is(err, ebpf.ErrKeyNotExist) {
		return ScoreJob{}, false, nil
	}
	if err != nil {
		return ScoreJob{}, false, err
	}
	j, err := DecodeScoreJob(b)
	return j, err == nil, err
}

// PushJob appends a job to SCORE_QUEUE from user space (tests, benchmarks).
func (c *Cart) PushJob(j ScoreJob) error {
	return c.m("SCORE_QUEUE").Update(nil, j.MarshalBinary(), ebpf.UpdateAny)
}

// TickJob reads the tick stage's job in progress (or the last one finished).
func (c *Cart) TickJob() (TickJob, error) {
	var b []byte
	if err := c.m("TICK_JOB").Lookup(uint32(0), &b); err != nil {
		return TickJob{}, err
	}
	return DecodeTickJob(b)
}

// Pump drives the tick stage: cart_tick until a call finds no job (the
// TICK_IDLE counter moves) or maxCalls. It returns the calls made.
func (c *Cart) Pump(maxCalls int) (int, error) {
	s, err := c.Stats()
	if err != nil {
		return 0, err
	}
	idle := s[StatTickIdle]
	for calls := 0; calls < maxCalls; calls++ {
		if err := c.Tick(); err != nil {
			return calls, err
		}
		s, err := c.Stats()
		if err != nil {
			return calls, err
		}
		if s[StatTickIdle] > idle {
			return calls + 1, nil
		}
	}
	return maxCalls, nil
}

// AttachXDP attaches cart_xdp to iface ("generic" or "native" mode). The
// link detaches when closed or when this process exits.
func (c *Cart) AttachXDP(iface, mode string) (link.Link, error) {
	ifc, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	flags := link.XDPGenericMode
	if mode == "native" {
		flags = link.XDPDriverMode
	}
	return link.AttachXDP(link.XDPOptions{Program: c.coll.Programs["cart_xdp"], Interface: ifc.Index, Flags: flags})
}

// Events opens a reader on CART_EVENTS.
func (c *Cart) Events() (*ringbuf.Reader, error) { return ringbuf.NewReader(c.m("CART_EVENTS")) }

// DrainEvents reads every event currently in CART_EVENTS (non-blocking).
func (c *Cart) DrainEvents(rd *ringbuf.Reader) ([]Event, error) {
	var out []Event
	rd.SetDeadline(time.Now().Add(20 * time.Millisecond))
	for {
		rec, err := rd.Read()
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return out, nil
			}
			return out, err
		}
		ev, err := DecodeEvent(rec.RawSample)
		if err != nil {
			return out, err
		}
		out = append(out, ev)
	}
}

// IPv6MonadPacket builds Ethernet/IPv6/Hop-by-Hop(Monad)/UDP of total length
// n (>= 86): the HbH header carries one Monad option (0x3E, 20 bytes, version
// 1, valid CRC-16/CCITT) with qos_class qos and flags fl. This is the frame
// shape qos-ebpf's token-bucket path needs, so benchmarks compare the native
// and cartridge rate limiters on the same bytes.
func IPv6MonadPacket(flowLabel uint32, sport, dport uint16, qos, fl uint8, n int) []byte {
	const hdr = 14 + 40 + 24 + 8
	if n < hdr {
		n = hdr
	}
	p := make([]byte, n)
	binary.BigEndian.PutUint16(p[12:], 0x86DD)
	ip := p[14:]
	binary.BigEndian.PutUint32(ip[0:], 6<<28|flowLabel&0xFFFFF)
	binary.BigEndian.PutUint16(ip[4:], uint16(n-54)) // #nosec G115 -- small test frames
	ip[6], ip[7] = 0, 64                             // next header HbH, hop limit
	ip[8], ip[23] = 0xfd, 1                          // fd00::1
	ip[24], ip[39] = 0xfd, 2                         // fd00::2
	hbh := ip[40:]
	hbh[0], hbh[1], hbh[2], hbh[3] = 17, 2, 0x3E, 20 // UDP next, (2+1)*8 = 24 bytes
	m := hbh[4:24]
	m[0], m[8], m[11] = 1, qos, fl
	binary.BigEndian.PutUint16(m[18:], crc16CCITT(m[:18]))
	udp := hbh[24:]
	binary.BigEndian.PutUint16(udp[0:], sport)
	binary.BigEndian.PutUint16(udp[2:], dport)
	binary.BigEndian.PutUint16(udp[4:], uint16(n-78)) // #nosec G115 -- small test frames
	return p
}

// crc16CCITT is CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF), the Monad checksum.
func crc16CCITT(b []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, x := range b {
		crc ^= uint16(x) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// Packet builders for tests and benchmarks (Ethernet + IPv4 + UDP/TCP).

// IPv4Packet builds a minimal Ethernet/IPv4/L4 frame of total length n (>= 42 for UDP).
func IPv4Packet(proto uint8, src, dst [4]byte, sport, dport uint16, n int) []byte {
	if n < 14+20+20 {
		n = 14 + 20 + 20
	}
	p := make([]byte, n)
	binary.BigEndian.PutUint16(p[12:], 0x0800)
	ip := p[14:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:], uint16(n-14)) // #nosec G115 -- small test frames
	ip[8], ip[9] = 64, proto
	copy(ip[12:16], src[:])
	copy(ip[16:20], dst[:])
	l4 := ip[20:]
	binary.BigEndian.PutUint16(l4[0:], sport)
	binary.BigEndian.PutUint16(l4[2:], dport)
	if proto == 17 {
		binary.BigEndian.PutUint16(l4[4:], uint16(n-34)) // #nosec G115 -- small test frames
	}
	return p
}
