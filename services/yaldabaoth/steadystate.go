// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Steady state (S-REAL-STEEL D-20). Two parts:
//
//   - target probes: real HTTP GETs to LXD instances the allowlist accepts,
//     at the address Cuirass reports (never a host from the request), on a
//     port and plain path; no redirects, no environment proxy, a timeout.
//     Before injection their success ratio must reach min_success_ratio
//     (default 1). During the experiment they measure impact; they abort
//     only when abort_below is set and the ratio falls under it.
//   - the Kingdom SLO guard (pkg/sloguard): every page: true SLO under the
//     14.4x fast-burn tier. Checked before (must hold; a missing series or
//     an unreadable Victoria refuses the start), every SafetyCheckInterval
//     during (aborts), and after the revert (part of the verdict).

// SteadyState is the experiment's hypothesis.
type SteadyState struct {
	Hypothesis      string  `json:"hypothesis,omitempty"`
	Probes          []Probe `json:"probes"`
	MinSuccessRatio float64 `json:"min_success_ratio,omitempty"` // 0 means 1
	AbortBelow      float64 `json:"abort_below,omitempty"`       // 0: probes never abort
}

func (ss *SteadyState) minRatio() float64 {
	if ss == nil || ss.MinSuccessRatio <= 0 || ss.MinSuccessRatio > 1 {
		return 1
	}
	return ss.MinSuccessRatio
}

// Probe is one HTTP check of an allowlisted LXD instance.
type Probe struct {
	Name string `json:"name"`
	// Instance is the LXD instance to probe; empty means the experiment's
	// own target (which must then be an LXD instance).
	Instance     string `json:"instance,omitempty"`
	Port         int    `json:"port"`
	Path         string `json:"path"`
	ExpectStatus int    `json:"expect_status,omitempty"` // default 200
	TimeoutMS    int    `json:"timeout_ms,omitempty"`    // default 2000, at most 5000
}

// ProbeResult captures a probe check result.
type ProbeResult struct {
	Name      string  `json:"name"`
	Success   bool    `json:"success"`
	Status    int     `json:"status,omitempty"`
	LatencyMS float64 `json:"latency_ms,omitempty"`
	Error     string  `json:"error,omitempty"`
	At        string  `json:"at"`
}

var probePathRe = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,255}$`)

const (
	defaultProbeTimeout = 2 * time.Second
	maxProbeTimeout     = 5 * time.Second
	probeBodyLimit      = 4 << 10
)

func (s *Service) setProbeURL(f func(info *InstanceInfo, p Probe) (string, error)) {
	s.probeMu.Lock()
	s.probeURLFn = f
	s.probeMu.Unlock()
}

func (s *Service) probeURL(info *InstanceInfo, p Probe) (string, error) {
	s.probeMu.RLock()
	f := s.probeURLFn
	s.probeMu.RUnlock()
	return f(info, p)
}

// defaultProbeURL is http://<instance IP>:<port><path>, refusing the
// addresses an instance has no business reporting.
func defaultProbeURL(info *InstanceInfo, p Probe) (string, error) {
	ip := net.ParseIP(info.IP)
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return "", fmt.Errorf("instance %s reports address %q, which is never probed", info.Name, info.IP)
	}
	return "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(p.Port)) + p.Path, nil
}

func validProbePath(p string) bool {
	if !probePathRe.MatchString(p) || strings.HasPrefix(p, "//") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// runProbe performs one probe; every refusal is a failed probe, never a
// skipped one.
func (s *Service) runProbe(ctx context.Context, e *Experiment, p Probe) ProbeResult {
	res := ProbeResult{Name: p.Name, At: time.Now().UTC().Format(time.RFC3339Nano)}
	fail := func(format string, a ...any) ProbeResult {
		res.Error = fmt.Sprintf(format, a...)
		return res
	}
	if p.Port < 1 || p.Port > 65535 {
		return fail("port %d out of range", p.Port)
	}
	if !validProbePath(p.Path) {
		return fail("path %q is not a plain absolute path", p.Path)
	}
	inst := p.Instance
	if inst == "" {
		if e == nil || e.Target.Kind != KindLXD {
			return fail("a probe of a non-LXD experiment must name an instance")
		}
		inst = e.Target.ID
	}
	// The probe target passes the same allowlist as a fault target would.
	if _, err := s.deps.Allowlist.CheckTarget(ctx, Target{Kind: KindLXD, ID: inst}, s.env()); err != nil {
		return fail("probe target: %v", err)
	}
	info, err := s.deps.Resolver.Instance(ctx, inst)
	if err != nil {
		return fail("probe target %s: %v", inst, err)
	}
	// The address is the instance's own claim: only the bridge subnets
	// (allowlist probe_subnets) are ever contacted.
	if _, err := s.deps.Allowlist.ProbeAddress(info.IP); err != nil {
		return fail("probe target %s: %v", inst, err)
	}
	u, err := s.probeURL(info, p)
	if err != nil {
		return fail("%v", err)
	}
	timeout := defaultProbeTimeout
	if p.TimeoutMS > 0 {
		timeout = min(time.Duration(p.TimeoutMS)*time.Millisecond, maxProbeTimeout)
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(pctx, http.MethodGet, u, nil)
	if err != nil {
		return fail("%v", err)
	}
	start := time.Now()
	resp, err := s.client.Do(req)
	res.LatencyMS = float64(time.Since(start)) / float64(time.Millisecond)
	if err != nil {
		return fail("%v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, probeBodyLimit))
	res.Status = resp.StatusCode
	want := p.ExpectStatus
	if want == 0 {
		want = http.StatusOK
	}
	res.Success = resp.StatusCode == want
	if !res.Success {
		res.Error = fmt.Sprintf("status %d, want %d", resp.StatusCode, want)
	}
	return res
}

// checkProbes runs every probe once and returns the results and the
// success ratio (1 when there are none).
func (s *Service) checkProbes(ctx context.Context, e *Experiment) ([]ProbeResult, float64) {
	if e == nil || e.SteadyState == nil || len(e.SteadyState.Probes) == 0 {
		return nil, 1
	}
	var out []ProbeResult
	ok := 0
	for _, p := range e.SteadyState.Probes {
		r := s.runProbe(ctx, e, p)
		if r.Success {
			ok++
		}
		out = append(out, r)
	}
	return out, float64(ok) / float64(len(out))
}

// shouldAbort is the during-experiment check: the abort file, the Kingdom
// SLO guard, and probes under abort_below. It returns the abort path
// (file, guard, probes) and why, or "" when the experiment goes on.
func (s *Service) shouldAbort(ctx context.Context, e *Experiment) (string, string) {
	if s.abortFileExists() {
		return "file", "abort file " + s.config.AbortFile
	}
	v, err := s.deps.Guard.Check(ctx)
	if err != nil {
		return "guard", "Kingdom SLO guard cannot read Victoria: " + err.Error()
	}
	if !v.OK {
		return "guard", "Kingdom SLO guard: " + v.Reason
	}
	if e.SteadyState != nil && e.SteadyState.AbortBelow > 0 && len(e.SteadyState.Probes) > 0 {
		results, ratio := s.checkProbes(ctx, e)
		s.update(e.ID, func(x *Experiment) {
			x.Results.SteadyStateProbes = append(x.Results.SteadyStateProbes, results...)
		})
		if ratio < e.SteadyState.AbortBelow {
			return "probes", fmt.Sprintf("probes %.0f%% healthy, under abort_below %.0f%%", ratio*100, e.SteadyState.AbortBelow*100)
		}
	}
	return "", ""
}
