// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"
)

//go:embed prices.json
var defaultPricesJSON []byte

// NanoUSD is an amount of money in billionths of a US dollar. Integer, so
// a day of small requests sums exactly. A price of P USD per million
// tokens is P*1000 nano-USD per token, so cost = tokens * milli-USD/MTok.
type NanoUSD int64

// USD formats n as dollars.
func (n NanoUSD) USD() string {
	return "$" + strconv.FormatFloat(float64(n)/1e9, 'f', 6, 64)
}

// USDToNano converts a dollar amount (as typed on a command line) to
// NanoUSD, rejecting negatives and non-finite values.
func USDToNano(usd float64) (NanoUSD, error) {
	if math.IsNaN(usd) || math.IsInf(usd, 0) || usd < 0 {
		return 0, fmt.Errorf("invalid dollar amount %v", usd)
	}
	if usd > 1e6 {
		return 0, fmt.Errorf("dollar amount %v is implausibly large", usd)
	}
	return NanoUSD(math.Round(usd * 1e9)), nil
}

// Price is one model's list price. Fields are milli-USD per million
// tokens (4.00 USD/MTok = 4000), so tokens*field is nano-USD exactly.
type Price struct {
	Input        int64
	Output       int64
	CacheWrite5m int64
	CacheWrite1h int64
	CacheRead    int64
}

// PriceTable maps model IDs to prices.
type PriceTable struct {
	models map[string]Price
	max    Price // field-wise maximum, used for unknown models and reservations
}

type priceFile struct {
	Models map[string]struct {
		Input        float64 `json:"input"`
		Output       float64 `json:"output"`
		CacheWrite5m float64 `json:"cache_write_5m"`
		CacheWrite1h float64 `json:"cache_write_1h"`
		CacheRead    float64 `json:"cache_read"`
	} `json:"models"`
}

// DefaultPrices returns the embedded price table (prices.json).
func DefaultPrices() (*PriceTable, error) { return ParsePrices(defaultPricesJSON) }

// LoadPrices reads a price table from a JSON file in prices.json's shape.
func LoadPrices(path string) (*PriceTable, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- operator-supplied config path
	if err != nil {
		return nil, fmt.Errorf("read prices: %w", err)
	}
	return ParsePrices(b)
}

// ParsePrices parses a price table. Every price must be finite and
// non-negative and every model must have a non-zero input and output
// price: a zero would let the spend cap pass traffic it cannot count.
func ParsePrices(b []byte) (*PriceTable, error) {
	var f priceFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse prices: %w", err)
	}
	if len(f.Models) == 0 {
		return nil, errors.New("parse prices: no models")
	}
	t := &PriceTable{models: make(map[string]Price, len(f.Models))}
	for id, m := range f.Models {
		var p Price
		for _, c := range []struct {
			v   float64
			dst *int64
			nm  string
		}{
			{m.Input, &p.Input, "input"},
			{m.Output, &p.Output, "output"},
			{m.CacheWrite5m, &p.CacheWrite5m, "cache_write_5m"},
			{m.CacheWrite1h, &p.CacheWrite1h, "cache_write_1h"},
			{m.CacheRead, &p.CacheRead, "cache_read"},
		} {
			if math.IsNaN(c.v) || math.IsInf(c.v, 0) || c.v < 0 || c.v > 1e4 {
				return nil, fmt.Errorf("parse prices: %s.%s = %v", id, c.nm, c.v)
			}
			*c.dst = int64(math.Round(c.v * 1000))
		}
		if p.Input == 0 || p.Output == 0 {
			return nil, fmt.Errorf("parse prices: %s needs non-zero input and output prices", id)
		}
		t.models[id] = p
		t.max = Price{
			Input:        max(t.max.Input, p.Input),
			Output:       max(t.max.Output, p.Output),
			CacheWrite5m: max(t.max.CacheWrite5m, p.CacheWrite5m),
			CacheWrite1h: max(t.max.CacheWrite1h, p.CacheWrite1h),
			CacheRead:    max(t.max.CacheRead, p.CacheRead),
		}
	}
	return t, nil
}

// Lookup returns a model's price. ok is false for a model not in the
// table, which is then priced at the table's field-wise maximum so an
// unexpected fallback model can never be counted as cheaper than it is.
func (t *PriceTable) Lookup(model string) (Price, bool) {
	p, ok := t.models[model]
	if !ok {
		return t.max, false
	}
	return p, true
}

// Has reports whether model is in the table.
func (t *PriceTable) Has(model string) bool { _, ok := t.models[model]; return ok }

// Max is the field-wise maximum price across the table.
func (t *PriceTable) Max() Price { return t.max }

// tokenCost prices one block of token counts at p.
func tokenCost(p Price, input, output, cacheRead, cacheWrite5m, cacheWrite1h int64) NanoUSD {
	return NanoUSD(input*p.Input + output*p.Output + cacheRead*p.CacheRead +
		cacheWrite5m*p.CacheWrite5m + cacheWrite1h*p.CacheWrite1h)
}

// UsageCost prices a response's usage. When the response lists
// iterations (server-side fallback hops, compaction), each is priced at
// the model that ran it and the sum is the bill: the top-level usage
// covers only the attempt that produced the returned message. unknown
// lists models that were priced at the table maximum.
func (t *PriceTable) UsageCost(servedBy string, u anthropic.BetaUsage) (cost NanoUSD, unknown []string) {
	price := func(model string) Price {
		p, ok := t.Lookup(model)
		if !ok {
			unknown = append(unknown, model)
		}
		return p
	}
	if len(u.Iterations) == 0 {
		w5, w1 := splitCacheWrite(u.CacheCreationInputTokens, u.CacheCreation.Ephemeral5mInputTokens, u.CacheCreation.Ephemeral1hInputTokens)
		return tokenCost(price(servedBy), u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, w5, w1), unknown
	}
	for _, it := range u.Iterations {
		model := string(it.Model)
		if model == "" {
			model = servedBy
		}
		w5, w1 := splitCacheWrite(it.CacheCreationInputTokens, it.CacheCreation.Ephemeral5mInputTokens, it.CacheCreation.Ephemeral1hInputTokens)
		cost += tokenCost(price(model), it.InputTokens, it.OutputTokens, it.CacheReadInputTokens, w5, w1)
	}
	return cost, unknown
}

// splitCacheWrite divides cache-creation tokens by TTL. When the
// breakdown is absent the total is billed at the 5-minute rate, which is
// the only TTL this package requests.
func splitCacheWrite(total, w5, w1 int64) (int64, int64) {
	if w5+w1 == 0 {
		return total, 0
	}
	return w5, w1
}
