// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package kev

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Outcome is what one Poll found.
type Outcome struct {
	NotModified bool  // 304, or 200 with the stored bytes
	Bootstrap   bool  // no usable previous catalog: Delta is empty by design
	Delta       Delta // vs the previous catalog
	State       State // as now stored
}

// Poller ties a Fetcher to a Store. It is safe for one goroutine to Poll
// while others call Current.
type Poller struct {
	Fetcher *Fetcher
	Store   Store
	Now     func() time.Time

	mu    sync.RWMutex
	cur   *Catalog
	state *State
}

// Init loads the stored catalog, so a restart diffs against what was last
// seen instead of treating the next pull as a bootstrap. A stored file that
// fails validation is returned as an error and otherwise ignored: the next
// good pull replaces it.
func (p *Poller) Init() error {
	c, st, err := p.Store.Load()
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.cur, p.state = c, st
	p.mu.Unlock()
	return nil
}

// Current returns the last good catalog's state, or nil before the first.
func (p *Poller) Current() *State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.state == nil {
		return nil
	}
	s := *p.state
	return &s
}

// Poll fetches, validates and stores. On any error the stored catalog is
// left as it was.
func (p *Poller) Poll(ctx context.Context) (Outcome, error) {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	p.mu.RLock()
	prev, prevState := p.cur, p.state
	p.mu.RUnlock()

	// Validators are only worth sending when the copy they describe is here.
	var cond *State
	if prev != nil {
		cond = prevState
	}
	res, err := p.Fetcher.Fetch(ctx, cond)
	if err != nil {
		return Outcome{}, err
	}
	if res.NotModified {
		if prev == nil {
			return Outcome{}, fmt.Errorf("HTTP 304 with no stored catalog")
		}
		st := *prevState
		st.FetchedAt = now()
		if err := p.Store.SaveState(st); err != nil {
			return Outcome{}, err
		}
		p.setState(prev, &st)
		return Outcome{NotModified: true, State: st}, nil
	}

	c, err := Parse(res.Body)
	if err != nil {
		return Outcome{}, err
	}
	st := State{FetchedAt: now(), ETag: res.ETag, LastModified: res.LastModified}
	if prevState != nil && prev != nil && Sum(res.Body) == prevState.SHA256 {
		// Same bytes, served without a 304: refresh validators only.
		st.SHA256, st.CatalogVersion, st.Count = prevState.SHA256, c.CatalogVersion, c.Count
		if err := p.Store.SaveState(st); err != nil {
			return Outcome{}, err
		}
		p.setState(prev, &st)
		return Outcome{NotModified: true, State: st}, nil
	}
	if err := p.Store.Save(res.Body, c, st, st.FetchedAt); err != nil {
		return Outcome{}, err
	}
	st.SHA256, st.CatalogVersion, st.Count = Sum(res.Body), c.CatalogVersion, c.Count
	p.setState(c, &st)
	return Outcome{Bootstrap: prev == nil, Delta: Diff(prev, c), State: st}, nil
}

func (p *Poller) setState(c *Catalog, st *State) {
	p.mu.Lock()
	p.cur, p.state = c, st
	p.mu.Unlock()
}
