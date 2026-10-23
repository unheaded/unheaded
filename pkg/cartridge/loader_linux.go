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
	"regexp"
	"strconv"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

// DefaultPinDir is where Load pins the maps when asked to.
const DefaultPinDir = "/sys/fs/bpf/unheaded/cart"

// Options configure Load.
type Options struct {
	// PinDir pins every map by name under this directory (0700), so another
	// process (upc-cartridgectl) can find a loaded interpreter. "" = no pins.
	PinDir string
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
	return c, nil
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

// SlotStat is one per-slot counter.
func SlotStat(s [StatCount]uint64, slot, off int) uint64 { return s[slot*8+off] }

// QueueNonEmpty peeks SCORE_QUEUE (a BPF queue has no length; peek is the
// only non-consuming read).
func (c *Cart) QueueNonEmpty() bool {
	var b []byte
	return c.m("SCORE_QUEUE").Lookup(nil, &b) == nil
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
