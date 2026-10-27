// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// DefaultVictoriaURL is VictoriaMetrics in the monitoring compose stack on
// west, where slo:error_budget_remaining is recorded (monitoring/prometheus/rules/slo.yml).
const DefaultVictoriaURL = "http://localhost:8428"

// MaxSLOResponseBytes bounds each query response read.
const MaxSLOResponseBytes = 1 << 20

// SLO collects slo evidence (ADR-100 Phase 2): per source, the SLO's
// error budget remaining over its window, slo:error_budget_remaining, as
// VictoriaMetrics answers an instant query for it now. Above zero passes;
// zero or below fails (the budget is spent); no series, or NaN, is no
// record (NOT_ASSESSED): a missing measurement is not a pass. It reads
// recorded SLI aggregates only, never a service's data.
type SLO struct {
	// BaseURL of the Prometheus-compatible query API; empty means
	// $VICTORIA_URL, else DefaultVictoriaURL.
	BaseURL string
	Client  *http.Client
	Now     func() time.Time
}

func (s *SLO) baseURL() string {
	b := s.BaseURL
	if b == "" {
		b = os.Getenv("VICTORIA_URL")
	}
	if b == "" {
		b = DefaultVictoriaURL
	}
	return strings.TrimRight(b, "/")
}

// queryResponse is the part of /api/v1/query's answer this reads.
type queryResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value [2]json.RawMessage `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// Collect returns one record per slo source whose budget series exists.
// An unreachable or malformed query API is an error: the caller reports
// the evidence unavailable and the controls stay NOT_ASSESSED.
func (s *SLO) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	var names []string
	for _, src := range sources {
		if src.Kind != crosswalk.KindSLO {
			continue
		}
		if !crosswalk.SLORefRe.MatchString(src.Ref) {
			return nil, fmt.Errorf("slo ref %q is not an SLO name", src.Ref)
		}
		names = append(names, src.Ref)
	}
	if len(names) == 0 {
		return nil, nil
	}
	base := s.baseURL()
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("victoria URL %q: want http(s)://host[:port] without credentials", base)
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	var recs []crosswalk.Record
	for _, name := range names {
		resp, err := s.query(ctx, client, base, fmt.Sprintf(`slo:error_budget_remaining{slo=%q}`, name))
		if err != nil {
			return nil, fmt.Errorf("slo %s: %w", name, err)
		}
		worst, at, found := math.Inf(1), time.Time{}, false
		for _, r := range resp.Data.Result {
			var ts float64
			var raw string
			if err := json.Unmarshal(r.Value[0], &ts); err != nil {
				return nil, fmt.Errorf("slo %s: sample time: %w", name, err)
			}
			if err := json.Unmarshal(r.Value[1], &raw); err != nil {
				return nil, fmt.Errorf("slo %s: sample value: %w", name, err)
			}
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return nil, fmt.Errorf("slo %s: sample value %q", name, raw)
			}
			if math.IsNaN(v) {
				continue
			}
			if v < worst {
				worst, found = v, true
				sec, frac := math.Modf(ts)
				at = time.Unix(int64(sec), int64(frac*1e9)).UTC()
			}
		}
		if !found {
			continue // no measurement: NOT_ASSESSED
		}
		if n := now(); at.After(n) {
			at = n
		}
		verdict := crosswalk.VerdictPass
		if worst <= 0 {
			verdict = crosswalk.VerdictFail
		}
		recs = append(recs, crosswalk.Record{
			Source:     crosswalk.Source{Kind: crosswalk.KindSLO, Ref: name},
			Verdict:    verdict,
			ObservedAt: at,
			Detail:     fmt.Sprintf("%.1f%% of the error budget left (slo:error_budget_remaining at %s)", 100*worst, base),
		})
	}
	return recs, nil
}

func (s *SLO) query(ctx context.Context, client *http.Client, base, promql string) (*queryResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/query?"+url.Values{"query": {promql}}.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxSLOResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxSLOResponseBytes {
		return nil, fmt.Errorf("response larger than %d bytes", MaxSLOResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var qr queryResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, err
	}
	if qr.Status != "success" {
		return nil, fmt.Errorf("query status %q: %s", qr.Status, qr.Error)
	}
	if qr.Data.ResultType != "vector" {
		return nil, fmt.Errorf("result type %q, want vector", qr.Data.ResultType)
	}
	return &qr, nil
}
