// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package energy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func writePower(t *testing.T, path string, microwatts int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strconv.FormatInt(microwatts, 10)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindGPUPowerPath(t *testing.T) {
	root := t.TempDir()
	// An integrated GPU has power1_input only, as card0 does on west.
	writePower(t, filepath.Join(root, "class/drm/card0/device/hwmon/hwmon5/power1_input"), 1)
	want := filepath.Join(root, "class/drm/card1/device/hwmon/hwmon4/power1_average")
	writePower(t, want, 6000000)
	writePower(t, filepath.Join(root, "class/drm/card2/device/hwmon/hwmon9/power1_average"), 6000000)
	got, err := FindGPUPowerPath(root)
	if err != nil || got != want {
		t.Fatalf("got %q err %v, want %q", got, err, want)
	}
	if _, err := FindGPUPowerPath(t.TempDir()); !errors.Is(err, ErrNoGPUPower) {
		t.Fatalf("empty tree: %v", err)
	}
}

func TestReadWatts(t *testing.T) {
	p := filepath.Join(t.TempDir(), "power1_average")
	writePower(t, p, 6000000)
	if w, err := readWatts(p); err != nil || w != 6 {
		t.Fatalf("%v %v", w, err)
	}
	for _, bad := range []string{"", "abc\n", "-5\n"} {
		if err := os.WriteFile(p, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readWatts(p); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := readWatts(p + ".missing"); err == nil {
		t.Error("a missing file was accepted")
	}
}

func TestIntegrate(t *testing.T) {
	t0 := time.Unix(1000, 0)
	at := func(s float64, w float64) Sample {
		return Sample{At: t0.Add(time.Duration(s * float64(time.Second))), Watts: w}
	}
	cases := []struct {
		name    string
		samples []Sample
		want    float64
		ok      bool
	}{
		{"worked fixture", []Sample{at(0, 10), at(1, 30), at(2, 30), at(3, 10)}, 40, true},
		{"uneven spacing", []Sample{at(0, 10), at(0.5, 20), at(2.5, 20)}, 22.5, true},
		{"below idle counts as zero", []Sample{at(0, 10), at(1, 5), at(2, 5)}, 0, true},
		{"one sample", []Sample{at(0, 10)}, 0, false},
		{"none", nil, 0, false},
		{"no elapsed time", []Sample{at(1, 10), at(1, 30)}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Integrate(c.samples)
			if ok != c.ok || got != c.want {
				t.Fatalf("got %v %v, want %v %v", got, ok, c.want, c.ok)
			}
		})
	}
}

// fakeTime drives a Sampler: the test advances the clock, rewrites the
// power file, sends a tick and waits for the sample to land.
type fakeTime struct {
	mu   sync.Mutex
	now  time.Time
	tick chan time.Time
	ack  chan struct{}
}

func (f *fakeTime) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeTime) set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

func newFake(path string) (*Sampler, *fakeTime) {
	f := &fakeTime{now: time.Unix(1000, 0), tick: make(chan time.Time), ack: make(chan struct{}, 8)}
	s := New(path)
	s.now = f.Now
	s.after = func(time.Duration) <-chan time.Time { return f.tick }
	s.onSample = func() { f.ack <- struct{}{} }
	return s, f
}

func TestRunIntegratesTheFixture(t *testing.T) {
	p := filepath.Join(t.TempDir(), "power1_average")
	writePower(t, p, 10000000)
	s, f := newFake(p)
	r := s.Start(context.Background())
	<-f.ack // the baseline
	for i, w := range []int64{30000000, 30000000} {
		f.set(time.Unix(1001+int64(i), 0))
		writePower(t, p, w)
		f.tick <- f.Now()
		<-f.ack
	}
	f.set(time.Unix(1003, 0))
	writePower(t, p, 10000000)
	est := r.Stop()
	if est.Joules == nil || *est.Joules != 40 || est.Method != MethodGPUSampled {
		t.Fatalf("%+v", est)
	}
}

func TestRunWithoutASecondSampleHasNoEstimate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "power1_average")
	writePower(t, p, 10000000)
	s, f := newFake(p)
	r := s.Start(context.Background())
	<-f.ack
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	f.set(time.Unix(1003, 0))
	if est := r.Stop(); est.Joules != nil || est.Method != "" {
		t.Fatalf("%+v", est)
	}
}

func TestUnreadableAtStartIsNoEstimate(t *testing.T) {
	s, _ := newFake(filepath.Join(t.TempDir(), "missing"))
	if est := s.Start(context.Background()).Stop(); est.Joules != nil || est.Method != "" {
		t.Fatalf("%+v", est)
	}
}

func TestNilSamplerIsANoOp(t *testing.T) {
	var s *Sampler
	if est := s.Start(context.Background()).Stop(); est.Joules != nil || est.Method != "" {
		t.Fatalf("%+v", est)
	}
}

func TestStopTwiceAndCancelledContext(t *testing.T) {
	p := filepath.Join(t.TempDir(), "power1_average")
	writePower(t, p, 10000000)
	s, f := newFake(p)
	ctx, cancel := context.WithCancel(context.Background())
	r := s.Start(ctx)
	<-f.ack
	cancel()
	f.set(time.Unix(1001, 0))
	first := r.Stop()
	second := r.Stop()
	if first.Joules == nil || second.Joules == nil || *first.Joules != *second.Joules {
		t.Fatalf("%+v %+v", first, second)
	}
}

func TestDiscover(t *testing.T) {
	t.Setenv(EnvPowerPath, "off")
	if s := Discover(); s != nil {
		t.Fatal("off must disable the sampler")
	}
	p := filepath.Join(t.TempDir(), "power1_average")
	t.Setenv(EnvPowerPath, p)
	if s := Discover(); s == nil || s.path != p {
		t.Fatalf("%+v", s)
	}
}
