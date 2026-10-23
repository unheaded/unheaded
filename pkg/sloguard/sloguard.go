// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package sloguard is the Kingdom SLO guard (ADR-100): before and while a
// deploy or a chaos experiment runs, every page: true SLO in
// observability/alerting/policy.yaml must be below the fast burn tier,
// slo:error_ratio:5m < 14.4 x (1 - objective). A missing or unreadable
// series counts as bad (ADR-100: missing is bad), and an unreachable
// Victoria is an error the caller treats as a failed guard, never as a
// pass.
//
// Used by Sword's canary analysis (S-REAL-STEEL D-06) and by Yaldabaoth's
// steady-state hypothesis (D-20).
package sloguard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FastBurn is ADR-100's fast tier: 2% of a 30-day budget in an hour.
const FastBurn = 14.4

const (
	queryTimeout = 5 * time.Second
	maxAnswer    = 1 << 20
	maxPolicy    = 1 << 20
	errorRatio5m = "slo:error_ratio:5m"
)

var sloNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// SLO is one page: true objective.
type SLO struct {
	Name      string  `yaml:"name"`
	Objective float64 `yaml:"objective"` // percent, 0 < objective < 100
	Page      bool    `yaml:"page"`
}

// Threshold is the error ratio at which this SLO burns at the fast tier.
func (s SLO) Threshold() float64 { return FastBurn * (1 - s.Objective/100) }

// LoadPolicy reads the page: true SLOs from the alerting policy. A policy
// with none is an error: a guard that guards nothing would always pass.
func LoadPolicy(path string) ([]SLO, error) {
	f, err := os.Open(path) // #nosec G304 -- the operator-configured policy file
	if err != nil {
		return nil, fmt.Errorf("sloguard: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxPolicy))
	if err != nil {
		return nil, fmt.Errorf("sloguard: %w", err)
	}
	var p struct {
		SLOs []SLO `yaml:"slos"`
	}
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("sloguard: %s: %w", path, err)
	}
	seen := map[string]bool{}
	var out []SLO
	for _, s := range p.SLOs {
		if !sloNameRe.MatchString(s.Name) {
			return nil, fmt.Errorf("sloguard: SLO name %q is not a plain name", s.Name)
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("sloguard: SLO %q listed twice", s.Name)
		}
		seen[s.Name] = true
		if !s.Page {
			continue
		}
		if s.Objective <= 0 || s.Objective >= 100 || math.IsNaN(s.Objective) {
			return nil, fmt.Errorf("sloguard: SLO %q objective %v: want 0 < objective < 100", s.Name, s.Objective)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sloguard: %s has no page: true SLO to guard", path)
	}
	return out, nil
}

// Guard checks the page SLOs against Victoria.
type Guard struct {
	vm     string
	slos   []SLO
	client *http.Client
}

// New makes a Guard over the policy at policyPath, querying vmURL.
func New(vmURL, policyPath string) (*Guard, error) {
	u, err := url.Parse(vmURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("sloguard: bad Victoria URL %q", vmURL)
	}
	slos, err := LoadPolicy(policyPath)
	if err != nil {
		return nil, err
	}
	return &Guard{
		vm:   strings.TrimRight(vmURL, "/"),
		slos: slos,
		client: &http.Client{
			Timeout:       queryTimeout,
			Transport:     &http.Transport{Proxy: nil},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// SLOs returns the guarded objectives.
func (g *Guard) SLOs() []SLO { return append([]SLO(nil), g.slos...) }

// Verdict is one check.
type Verdict struct {
	OK      bool               `json:"ok"`
	Burning []string           `json:"burning,omitempty"`
	Missing []string           `json:"missing,omitempty"`
	Values  map[string]float64 `json:"values,omitempty"`
	Reason  string             `json:"reason,omitempty"`
}

// Check reads slo:error_ratio:5m once. err is set when Victoria could not
// answer; the verdict is then not OK either.
func (g *Guard) Check(ctx context.Context) (Verdict, error) {
	values, err := g.query(ctx)
	if err != nil {
		return Verdict{Reason: "SLO guard cannot read Victoria: " + err.Error()}, err
	}
	v := Verdict{Values: map[string]float64{}}
	for _, s := range g.slos {
		raw, ok := values[s.Name]
		val, perr := strconv.ParseFloat(raw, 64)
		if !ok || perr != nil || math.IsNaN(val) || math.IsInf(val, 0) || val < 0 {
			v.Missing = append(v.Missing, s.Name)
			continue
		}
		v.Values[s.Name] = val
		if val > s.Threshold() {
			v.Burning = append(v.Burning, s.Name)
		}
	}
	sort.Strings(v.Burning)
	sort.Strings(v.Missing)
	v.OK = len(v.Burning) == 0 && len(v.Missing) == 0
	var parts []string
	for _, n := range v.Burning {
		parts = append(parts, fmt.Sprintf("%s burning (error ratio %.4f over the 14.4x fast tier)", n, v.Values[n]))
	}
	for _, n := range v.Missing {
		parts = append(parts, n+" has no slo:error_ratio:5m series (missing counts as bad)")
	}
	v.Reason = strings.Join(parts, "; ")
	return v, nil
}

func (g *Guard) query(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	q := url.Values{"query": {errorRatio5m}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.vm+"/api/v1/query?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("victoria answered %d", resp.StatusCode)
	}
	var body struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAnswer)).Decode(&body); err != nil {
		return nil, err
	}
	if body.Status != "success" {
		return nil, errors.New("victoria query did not succeed")
	}
	out := map[string]string{}
	for _, r := range body.Data.Result {
		if len(r.Value) != 2 {
			continue
		}
		if s, ok := r.Value[1].(string); ok {
			out[r.Metric["slo"]] = s
		}
	}
	return out, nil
}
