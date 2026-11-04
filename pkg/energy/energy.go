// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package energy estimates the GPU energy of one LLM request (ADR-120). A
// Sampler reads hwmon's power1_average about every 250 ms while a request
// runs and integrates it above the idle baseline read at the start.
//
// These are estimates. The figure covers the whole card, so concurrent work
// inflates it; sampling misses short spikes; wall power is higher than the
// sensor reports; a model on the CPU reads near zero. CPU energy is not
// sampled. No estimate is nil, never 0.
package energy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// MethodGPUSampled is llm_requests.energy_method for this estimate.
	MethodGPUSampled = "gpu_sampled"
	// DefaultInterval is the sampling period (ADR-120).
	DefaultInterval = 250 * time.Millisecond
	// EnvPowerPath overrides discovery; "off" disables the sampler.
	EnvPowerPath = "ZHEN_GPU_POWER_PATH"

	sysRoot       = "/sys"
	maxPowerBytes = 32
)

// ErrNoGPUPower means no card under the tree reports power1_average.
var ErrNoGPUPower = errors.New("energy: no power1_average under class/drm")

// Sample is one power reading.
type Sample struct {
	At    time.Time
	Watts float64
}

// Estimate is what a run reports. Joules nil means no estimate.
type Estimate struct {
	Joules *float64
	Method string
}

// FindGPUPowerPath returns the first power1_average under root, in sorted
// order. Card and hwmon numbers move between boots, so nothing is hard
// coded; an integrated GPU reports power1_input instead and is skipped.
func FindGPUPowerPath(root string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(root, "class/drm/card[0-9]*/device/hwmon/hwmon[0-9]*/power1_average"))
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", ErrNoGPUPower
	}
	return matches[0], nil
}

func readWatts(path string) (float64, error) {
	f, err := os.Open(path) // #nosec G304 -- a sysfs path from FindGPUPowerPath or the operator's env
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, maxPowerBytes)
	n, err := f.Read(buf)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(buf[:n])), 10, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("energy: negative power %d", v)
	}
	return float64(v) / 1e6, nil
}

// Integrate returns the joules above the first sample's power, by
// trapezoids, net power floored at zero. It needs two samples and elapsed
// time; ok is false otherwise.
func Integrate(samples []Sample) (joules float64, ok bool) {
	if len(samples) < 2 {
		return 0, false
	}
	idle := samples[0].Watts
	net := func(w float64) float64 { return max(w-idle, 0) }
	elapsed := samples[len(samples)-1].At.Sub(samples[0].At)
	if elapsed <= 0 {
		return 0, false
	}
	for i := 1; i < len(samples); i++ {
		dt := samples[i].At.Sub(samples[i-1].At).Seconds()
		joules += (net(samples[i-1].Watts) + net(samples[i].Watts)) / 2 * dt
	}
	return joules, true
}

// Sampler is safe for concurrent Start calls. A nil *Sampler is off.
type Sampler struct {
	path     string
	interval time.Duration
	now      func() time.Time
	after    func(time.Duration) <-chan time.Time
	onSample func() // test hook, after each sample lands
}

// New returns a Sampler over one power file.
func New(path string) *Sampler {
	return &Sampler{path: path, interval: DefaultInterval, now: time.Now, after: time.After}
}

// Discover honours EnvPowerPath, else searches /sys. It returns nil when
// there is nothing to read; callers need no check, nil is a no-op.
func Discover() *Sampler {
	switch v := strings.TrimSpace(os.Getenv(EnvPowerPath)); v {
	case "off":
		return nil
	case "":
		p, err := FindGPUPowerPath(sysRoot)
		if err != nil {
			return nil
		}
		return New(p)
	default:
		return New(v)
	}
}

// Run is one request's sampling.
type Run struct {
	s       *Sampler
	mu      sync.Mutex
	samples []Sample
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	est     Estimate
}

func (r *Run) sample() {
	w, err := readWatts(r.s.path)
	if err != nil {
		return // a missed read is a wider trapezoid, not a failure
	}
	r.mu.Lock()
	r.samples = append(r.samples, Sample{At: r.s.now(), Watts: w})
	r.mu.Unlock()
	if r.s.onSample != nil {
		r.s.onSample()
	}
}

// Start reads the baseline and samples until Stop or ctx ends. It returns
// a nil Run, whose Stop is a no-op, when the sampler is off or the file
// cannot be read.
func (s *Sampler) Start(ctx context.Context) *Run {
	if s == nil {
		return nil
	}
	r := &Run{s: s, stop: make(chan struct{}), done: make(chan struct{})}
	r.sample()
	r.mu.Lock()
	n := len(r.samples)
	r.mu.Unlock()
	if n == 0 {
		return nil
	}
	go func() {
		defer close(r.done)
		for {
			select {
			case <-r.stop:
				return
			case <-ctx.Done():
				return
			case <-s.after(s.interval):
				r.sample()
			}
		}
	}()
	return r
}

// Stop takes a last sample and returns the estimate. Safe to call twice.
func (r *Run) Stop() Estimate {
	if r == nil {
		return Estimate{}
	}
	r.once.Do(func() {
		close(r.stop)
		<-r.done
		r.sample()
		r.mu.Lock()
		defer r.mu.Unlock()
		if j, ok := Integrate(r.samples); ok {
			r.est = Estimate{Joules: &j, Method: MethodGPUSampled}
		}
	})
	return r.est
}
