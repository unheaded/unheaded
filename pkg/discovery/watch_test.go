// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// watchFake is a Wotan client for Watch: it counts streams and delivers
// what the test sends.
type watchFake struct {
	mu      sync.Mutex
	streams int
	joins   int
	ch      chan *wotanClient.Message
}

func newWatchFake() *watchFake { return &watchFake{ch: make(chan *wotanClient.Message, 512)} }

func (f *watchFake) Subscribe(_ context.Context, topic, name string) (*wotanClient.Subscriber, error) {
	f.mu.Lock()
	f.joins++
	f.mu.Unlock()
	return &wotanClient.Subscriber{Topic: topic, DisplayName: name, Status: "approved"}, nil
}

func (f *watchFake) StreamMessages(context.Context, string) (<-chan *wotanClient.Message, error) {
	f.mu.Lock()
	f.streams++
	f.mu.Unlock()
	return f.ch, nil
}

func (f *watchFake) announce(sender, name, addr string, port int) {
	b, _ := json.Marshal(ServiceEntry{Name: name, Address: addr, Port: port, Health: "healthy"})
	f.ch <- &wotanClient.Message{Topic: DiscoveryTopic, Payload: string(b), VerifiedSender: sender, CreatedAt: time.Now()}
}

func refusedCount(t *testing.T, reason string) float64 {
	t.Helper()
	var buf bytes.Buffer
	if err := discoveryRefused.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return seriesIn(t, &buf, `unheaded_discovery_refused_total{reason="`+reason+`"} `)
}

func seriesIn(t *testing.T, r io.Reader, prefix string) float64 {
	t.Helper()
	b, _ := io.ReadAll(r)
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, prefix) {
			v, err := strconv.ParseFloat(strings.TrimPrefix(line, prefix), 64)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
	}
	return 0
}

func admitLab() Admit {
	return Admit{
		AllowSubnets: []netip.Prefix{netip.MustParsePrefix("10.10.20.0/24")},
		Resolve: func(_ context.Context, host string) ([]netip.Addr, error) {
			switch host {
			case "inside":
				return []netip.Addr{netip.MustParseAddr("10.10.20.9")}, nil
			case "outside":
				return []netip.Addr{netip.MustParseAddr("192.0.2.7")}, nil
			}
			return nil, errors.New("no such host")
		},
	}
}

func startWatch(t *testing.T, a Admit) (*Registry, *watchFake) {
	t.Helper()
	f := newWatchFake()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	reg, err := watch(ctx, f, "akira", a, DefaultTTL, ReapInterval)
	if err != nil {
		t.Fatal(err)
	}
	return reg, f
}

func TestWatchAdmitsVerifiedSender(t *testing.T) {
	reg, f := startWatch(t, admitLab())
	before := refusedCount(t, "sender")
	f.announce("sword", "sword", "10.10.20.7", 19011)
	f.announce("sword", "timeguru", "10.10.20.8", 19000) // sword may not announce timeguru
	waitFor(t, "sword admitted", func() bool { _, err := reg.Resolve("sword"); return err == nil })
	waitFor(t, "the forged name refused", func() bool { return refusedCount(t, "sender")-before == 1 })
	if _, err := reg.Resolve("timeguru"); err == nil {
		t.Fatal("a name announced by another sender was admitted")
	}
}

func TestWatchRefusesAddressOutsideSubnets(t *testing.T) {
	reg, f := startWatch(t, admitLab())
	before := refusedCount(t, "address")
	f.announce("sword", "sword", "10.10.20.7", 19011)
	f.announce("timeguru", "timeguru", "192.0.2.1", 19000)
	f.announce("kanban-app", "kanban-app", "outside", 20001)
	f.announce("akira", "akira", "", 19100)
	f.announce("trace-collector", "trace-collector", "inside", 16671)
	waitFor(t, "inside by name", func() bool { _, err := reg.Resolve("trace-collector"); return err == nil })
	waitFor(t, "three refused", func() bool { return refusedCount(t, "address")-before == 3 })
	for _, n := range []string{"timeguru", "kanban-app", "akira"} {
		if _, err := reg.Resolve(n); err == nil {
			t.Errorf("%s admitted from outside the subnets", n)
		}
	}
	e, _ := reg.Resolve("trace-collector")
	if e.Address != "10.10.20.9" {
		t.Errorf("kept address %q, want the checked IP 10.10.20.9", e.Address)
	}
}

func TestWatchBounded(t *testing.T) {
	a := admitLab()
	a.MaxEntries = 3
	reg, f := startWatch(t, a)
	before := refusedCount(t, "full")
	for i := 0; i < 4; i++ {
		n := "svc" + strconv.Itoa(i)
		f.announce(n, n, "10.10.20."+strconv.Itoa(i+1), 19000)
	}
	waitFor(t, "the fourth refused", func() bool { return refusedCount(t, "full")-before == 1 })
	if n := len(reg.ResolveAll()); n != 3 {
		t.Fatalf("%d entries, want 3", n)
	}
	if MaxDiscovered != 256 {
		t.Errorf("MaxDiscovered = %d, want 256", MaxDiscovered)
	}
}

func TestWatchReaps(t *testing.T) {
	f := newWatchFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg, err := watch(ctx, f, "akira", admitLab(), 30*time.Millisecond, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	f.announce("sword", "sword", "10.10.20.7", 19011)
	waitFor(t, "admitted", func() bool { _, err := reg.Resolve("sword"); return err == nil })
	time.Sleep(3 * 30 * time.Millisecond)
	if _, err := reg.Resolve("sword"); err == nil {
		t.Fatal("an entry not announced for 3 TTLs is still there")
	}
}

func TestWatchOneStream(t *testing.T) {
	_, f := startWatch(t, admitLab())
	waitFor(t, "the stream", func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.streams > 0 })
	time.Sleep(20 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streams != 1 {
		t.Fatalf("%d StreamMessages calls, want 1", f.streams)
	}
}

func TestNoSubnetsMeansOff(t *testing.T) {
	f := newWatchFake()
	if _, err := Watch(context.Background(), f, "akira", Admit{}); err == nil {
		t.Fatal("Watch with no subnets returned no error")
	}
	if f.joins != 0 || f.streams != 0 {
		t.Fatalf("subscribed with no subnets: joins %d streams %d", f.joins, f.streams)
	}
}
