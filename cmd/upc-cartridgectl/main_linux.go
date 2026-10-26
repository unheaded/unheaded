// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/cartridge"
	"unheaded/pkg/gungnir"
	"unheaded/pkg/ports"
)

type globals struct {
	registryDir, sophiaURL, pubKeyPath, pinDir, obj, cartBin, eventsGroup string
	lab                                                                   bool
	pub                                                                   *mldsa65.PublicKey
}

func (g *globals) registry() Registry {
	files := &FileRegistry{Dir: g.registryDir}
	if g.sophiaURL == "" {
		return files
	}
	return &SophiaRegistry{BaseURL: g.sophiaURL, Token: os.Getenv("SOPHIA_TOKEN"), Blobs: files}
}

func (g *globals) prepare(raw, seal []byte) (*Prepared, error) {
	p, err := Prepare(raw, seal, g.pub, g.lab, cartValidator(g.cartBin))
	if err == nil && p.Lab {
		fmt.Fprintln(os.Stderr, "WARNING: --lab: loading an UNSIGNED cartridge; every metric for its slot carries lab=\"1\"")
	}
	return p, err
}

func readFileCapped(path string) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-supplied path
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxBlob+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBlob {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxBlob)
	}
	return b, nil
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "upc-cartridgectl: "+format+"\n", a...)
	os.Exit(1)
}

func main() {
	g := &globals{}
	fs := flag.NewFlagSet("upc-cartridgectl", flag.ExitOnError)
	fs.StringVar(&g.registryDir, "registry-dir", "/var/lib/unheaded/cartridges", "file-backed registry: records + content-addressed images (0700)")
	fs.StringVar(&g.sophiaURL, "sophia", "", "Sophia base URL: keep records in Sophia (namespace x-cartridge); SOPHIA_TOKEN = bearer token")
	fs.StringVar(&g.pubKeyPath, "pubkey", "", "pinned ML-DSA-65 public key that cartridge seals must verify against")
	fs.BoolVar(&g.lab, "lab", false, "accept UNSIGNED cartridges (lab only; marked lab=\"1\")")
	fs.StringVar(&g.pinDir, "pin-dir", cartridge.DefaultPinDir, "bpffs directory of the pinned interpreter")
	fs.StringVar(&g.obj, "obj", "ebpf/target/bpfel-unknown-none/release/cartridge-ebpf", "cartridge-ebpf object (for up)")
	fs.StringVar(&g.eventsGroup, "events-group", "", "up: make the CART_EVENTS pin readable by this group (cmd/anomaly's bridge); other pins stay root 0600")
	fs.StringVar(&g.cartBin, "cart", "crates/upc-cartridge/target/release/cart", "the cart CLI (validator)")
	_ = fs.Parse(os.Args[1:])
	args := fs.Args()
	if len(args) == 0 {
		fs.Usage()
		os.Exit(2)
	}
	if g.pubKeyPath != "" {
		pub, err := gungnir.LoadPublicKeyFile(g.pubKeyPath)
		if err != nil {
			fatal("pubkey: %v", err)
		}
		g.pub = pub
	}
	var err error
	switch args[0] {
	case "up":
		err = cmdUp(g)
	case "down":
		err = cmdDown(g)
	case "sign":
		err = cmdSign(args[1:])
	case "register":
		err = cmdRegister(g, args[1:])
	case "load":
		err = cmdLoad(g, args[1:])
	case "unload":
		err = cmdUnload(g, args[1:])
	case "swap":
		err = cmdSwap(g, args[1:])
	case "list":
		err = cmdList(g)
	case "stats":
		err = cmdStats(g)
	case "bench":
		err = cmdBench(g, args[1:])
	case "pump":
		err = cmdPump(g, args[1:])
	case "serve":
		err = cmdServe(g, args[1:])
	case "record":
		err = cmdRecord(g, args[1:])
	case "replay":
		err = cmdReplay(g, args[1:])
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	var ee *exitError
	if errors.As(err, &ee) {
		os.Exit(ee.code) // cart replay already reported
	}
	if err != nil {
		fatal("%s: %v", args[0], err)
	}
}

func cmdUp(g *globals) error {
	opts := cartridge.Options{PinDir: g.pinDir}
	if g.eventsGroup != "" {
		grp, err := user.LookupGroup(g.eventsGroup)
		if err != nil {
			return err
		}
		if opts.EventsGID, err = strconv.Atoi(grp.Gid); err != nil {
			return fmt.Errorf("group %s: gid %q: %w", g.eventsGroup, grp.Gid, err)
		}
	}
	c, err := cartridge.Load(g.obj, opts)
	if err != nil {
		return err
	}
	defer c.Close()
	fmt.Printf("interpreter up, pinned under %s (cart_xdp %d, cart_tick %d verifier insns)\n", g.pinDir, c.Verifier["cart_xdp"], c.Verifier["cart_tick"])
	return nil
}

func cmdDown(g *globals) error {
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Unpin(); err != nil {
		return err
	}
	if err := writeSlots(g.registryDir, map[int]SlotState{}); err != nil {
		return err
	}
	fmt.Println("interpreter unpinned (unloads when the last reference closes)")
	return nil
}

func cmdSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	key := fs.String("key", "", "ML-DSA-65 private key seed (0600)")
	ttl := fs.Duration("ttl", 7*24*time.Hour, "seal lifetime (<= 31 days)")
	_ = fs.Parse(args)
	if *key == "" || fs.NArg() != 1 {
		return errors.New("usage: sign --key SEED [--ttl D] FILE.cart")
	}
	priv, err := gungnir.LoadSeedFile(*key)
	if err != nil {
		return err
	}
	s, err := gungnir.NewSignerWithID(priv)
	if err != nil {
		return err
	}
	raw, err := readFileCapped(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := cartridge.DecodeImage(raw); err != nil {
		return err
	}
	seal, err := s.SignFor(raw, *ttl)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(seal, "", "  ")
	if err != nil {
		return err
	}
	out := fs.Arg(0) + ".seal"
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil { // #nosec G306 -- a seal is public
		return err
	}
	fmt.Printf("wrote %s (key %s, expires %s)\n", out, seal.KeyId, time.Unix(seal.ExpiresAtUnix, 0).UTC().Format(time.RFC3339))
	return nil
}

func stageName(s uint8) string {
	if s == cartridge.StageTick {
		return "tick"
	}
	return "xdp"
}

func cmdRegister(g *globals, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: register NAME FILE.cart")
	}
	raw, err := readFileCapped(args[1])
	if err != nil {
		return err
	}
	seal, err := readFileCapped(args[1] + ".seal")
	if errors.Is(err, os.ErrNotExist) {
		seal, err = nil, nil
	}
	if err != nil {
		return err
	}
	p, err := g.prepare(raw, seal)
	if err != nil {
		return err
	}
	rec := Record{
		Name: args[0], SHA256: p.SHA256, ImageSHA256: fmt.Sprintf("%x", p.Image.SHA256), KeyID: p.KeyID,
		Stage: stageName(p.Image.Stage), Surface: SurfaceVersion, MaxInsn: int(p.Image.MaxInsn), Slot: -1,
		Size: len(raw), Lab: p.Lab, Namespace: Namespace, RegisteredAt: time.Now().UTC(),
	}
	if err := g.registry().Put(rec, raw, seal); err != nil {
		return err
	}
	fmt.Printf("registered %s sha256=%s key=%s stage=%s\n", rec.Name, rec.SHA256, orDash(rec.KeyID), rec.Stage)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-(unsigned,lab)"
	}
	return s
}

// loadInto resolves name, re-runs every check on the bytes it fetched, and
// writes them into slot with the next generation.
func loadInto(g *globals, c *cartridge.Cart, name string, slot int) (SlotState, error) {
	rec, raw, seal, err := g.registry().Get(name)
	if err != nil {
		return SlotState{}, err
	}
	p, err := g.prepare(raw, seal)
	if err != nil {
		return SlotState{}, err
	}
	if p.SHA256 != rec.SHA256 {
		return SlotState{}, ErrBlob
	}
	cur, err := c.SlotConfig(slot)
	if err != nil {
		return SlotState{}, err
	}
	gen := cur.Gen + 1
	if err := c.WriteSlot(slot, p.Image, gen); err != nil {
		return SlotState{}, err
	}
	return SlotState{Name: name, SHA256: p.SHA256, KeyID: p.KeyID, Gen: gen, Lab: p.Lab, LoadedAt: time.Now().UTC()}, nil
}

func slotFlag(fs *flag.FlagSet, name string) *int { return fs.Int(name, -1, "slot 0..3") }

func checkSlot(n int) error {
	if n < 0 || n >= cartridge.Slots {
		return fmt.Errorf("slot %d outside 0..%d", n, cartridge.Slots-1)
	}
	return nil
}

func cmdLoad(g *globals, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: load NAME --slot N")
	}
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	slot := slotFlag(fs, "slot")
	_ = fs.Parse(args[1:])
	if err := checkSlot(*slot); err != nil {
		return err
	}
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	st, err := loadInto(g, c, args[0], *slot)
	if err != nil {
		return err
	}
	slots, err := readSlots(g.registryDir)
	if err != nil {
		return err
	}
	slots[*slot] = st
	if err := writeSlots(g.registryDir, slots); err != nil {
		return err
	}
	fmt.Printf("slot %d <- %s sha256=%s gen=%d key=%s\n", *slot, st.Name, st.SHA256, st.Gen, orDash(st.KeyID))
	return nil
}

func cmdUnload(g *globals, args []string) error {
	fs := flag.NewFlagSet("unload", flag.ExitOnError)
	slot := slotFlag(fs, "slot")
	_ = fs.Parse(args)
	if err := checkSlot(*slot); err != nil {
		return err
	}
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.SetSlotActive(*slot, false); err != nil {
		return err
	}
	slots, err := readSlots(g.registryDir)
	if err != nil {
		return err
	}
	delete(slots, *slot)
	if err := writeSlots(g.registryDir, slots); err != nil {
		return err
	}
	fmt.Printf("slot %d inactive\n", *slot)
	return nil
}

// cmdSwap loads NAME into the spare slot (activated only once complete,
// config written last), then deactivates the old slot. Packets see either
// the old cartridge, both briefly (the most restrictive verdict wins), or
// the new one — never a half-written slot. Flows start fresh in the new
// slot (its own state, its own generation).
func cmdSwap(g *globals, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: swap NAME --from A --to B")
	}
	fs := flag.NewFlagSet("swap", flag.ExitOnError)
	from, to := slotFlag(fs, "from"), slotFlag(fs, "to")
	_ = fs.Parse(args[1:])
	if err := checkSlot(*from); err != nil {
		return err
	}
	if err := checkSlot(*to); err != nil {
		return err
	}
	if *from == *to {
		return errors.New("--from and --to must differ (the spare slot takes the new image)")
	}
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	st, err := loadInto(g, c, args[0], *to)
	if err != nil {
		return err
	}
	if err := c.SetSlotActive(*from, false); err != nil {
		return err
	}
	slots, err := readSlots(g.registryDir)
	if err != nil {
		return err
	}
	delete(slots, *from)
	slots[*to] = st
	if err := writeSlots(g.registryDir, slots); err != nil {
		return err
	}
	fmt.Printf("swapped: slot %d <- %s (gen %d), slot %d inactive\n", *to, st.Name, st.Gen, *from)
	return nil
}

func cmdList(g *globals) error {
	recs, err := g.registry().List()
	if err != nil {
		return err
	}
	for _, r := range recs {
		fmt.Printf("%-16s sha256=%s key=%s stage=%s max_insn=%d size=%d\n", r.Name, r.SHA256, orDash(r.KeyID), r.Stage, r.MaxInsn, r.Size)
	}
	slots, err := readSlots(g.registryDir)
	if err != nil {
		return err
	}
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		fmt.Printf("(interpreter not up: %v)\n", err)
		return nil
	}
	defer c.Close()
	for s := 0; s < cartridge.Slots; s++ {
		cfg, err := c.SlotConfig(s)
		if err != nil {
			return err
		}
		st := slots[s]
		fmt.Printf("slot %d active=%d stage=%s gen=%d cartridge=%s sha256=%s key=%s\n", s, cfg.Active, stageName(cfg.Stage), cfg.Gen, orDash2(st.Name), orDash2(st.SHA256), orDash2(st.KeyID))
	}
	return nil
}

func orDash2(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func cmdStats(g *globals) error {
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	s, err := c.Stats()
	if err != nil {
		return err
	}
	slots, err := readSlots(g.registryDir)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(RenderMetrics(s, slots), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasSuffix(line, " 0") {
			fmt.Println(line)
		}
	}
	return nil
}

func cmdBench(g *globals, args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	repeat := fs.Int("repeat", 1_000_000, "test_run repeats")
	dport := fs.Uint("dport", 53, "UDP destination port of the test frame")
	size := fs.Int("size", 128, "frame length")
	_ = fs.Parse(args)
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	pkt := cartridge.IPv4Packet(17, [4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 4000, uint16(*dport), *size) // #nosec G115 -- port flag
	ret, d, err := c.BenchXDP(pkt, *repeat)
	if err != nil {
		return err
	}
	fmt.Printf("cart_xdp: %d ns/pkt over %d runs (ret %d)\n", d.Nanoseconds(), *repeat, ret)
	return nil
}

// cmdPump drives the tick stage: cart_tick until the queue is idle or max
// calls.
func cmdPump(g *globals, args []string) error {
	fs := flag.NewFlagSet("pump", flag.ExitOnError)
	maxCalls := fs.Int("max", 100_000, "cart_tick calls at most")
	_ = fs.Parse(args)
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	before, err := c.Stats()
	if err != nil {
		return err
	}
	calls := 0
	for ; calls < *maxCalls; calls++ {
		idle := before[cartridge.StatTickIdle]
		if err := c.Tick(); err != nil {
			return err
		}
		now, err := c.Stats()
		if err != nil {
			return err
		}
		if now[cartridge.StatTickIdle] > idle {
			break
		}
		before = now
	}
	after, _ := c.Stats()
	fmt.Printf("pump: %d calls, %d jobs finished in total\n", calls, after[cartridge.StatTickJobsDone])
	return nil
}

func cmdServe(g *globals, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("metrics-addr", "127.0.0.1:"+strconv.Itoa(ports.UPCCartridgeMetrics), "metrics/health listen address (loopback by default)")
	iface := fs.String("xdp-iface", "", "attach cart_xdp here while serving (the link is gone when this exits)")
	mode := fs.String("xdp-mode", "generic", "generic | native")
	_ = fs.Parse(args)
	if !isLoopbackAddr(*addr) {
		fmt.Fprintf(os.Stderr, "WARNING: metrics on non-loopback %s\n", *addr)
	}
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	if *iface != "" {
		l, err := c.AttachXDP(*iface, *mode)
		if err != nil {
			return err
		}
		defer l.Close()
		fmt.Printf("cart_xdp attached to %s (%s)\n", *iface, *mode)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		s, err := c.Stats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		slots, _ := readSlots(g.registryDir)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = io.WriteString(w, RenderMetrics(s, slots))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := c.SlotConfig(0); err != nil {
			http.Error(w, "interpreter maps unreadable", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ready\n")
	})
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	fmt.Printf("serving metrics on http://%s/metrics\n", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
