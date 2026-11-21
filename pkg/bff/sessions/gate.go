// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package sessions

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// gate is the Store a front holds from its start. Until the Well is
// reached every call answers ErrUnavailable, so nothing is accepted on the
// strength of a store that is not there.
type gate struct {
	front string

	mu sync.RWMutex
	st Store

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// newGate tries connect once. A refusal (refused(err)) is returned: the
// caller does not start. Any other failure is waited out in the
// background, first after minWait, doubling to maxWait. A refusal met
// while waiting cannot stop a process that is already serving: it is
// logged as an error and retried at maxWait, and the gate stays shut.
func newGate(ctx context.Context, front string, connect func(context.Context) (Store, error), refused func(error) bool, minWait, maxWait time.Duration) (Store, error) {
	g := &gate{front: front, stop: make(chan struct{}), done: make(chan struct{})}
	st, err := connect(ctx)
	switch {
	case err == nil:
		g.st = st
		close(g.done)
		return g, nil
	case refused(err):
		return nil, err
	}
	log.Warn().Err(err).Str("service", front).
		Msg("sessions: the Well is not reachable; no session is accepted and nobody can log in until it is")
	// The wait outlives the start: it ends at Close, not with ctx.
	go g.wait(context.WithoutCancel(ctx), connect, refused, minWait, maxWait)
	return g, nil
}

func (g *gate) wait(base context.Context, connect func(context.Context) (Store, error), refused func(error) bool, minWait, maxWait time.Duration) {
	defer close(g.done)
	wait := minWait
	for {
		t := time.NewTimer(wait)
		select {
		case <-g.stop:
			t.Stop()
			return
		case <-t.C:
		}
		ctx, cancel := context.WithCancel(base)
		go func() {
			select {
			case <-g.stop:
				cancel()
			case <-ctx.Done():
			}
		}()
		st, err := connect(ctx)
		cancel()
		if err == nil {
			g.mu.Lock()
			select {
			case <-g.stop:
				g.mu.Unlock()
				_ = st.Close()
				return
			default:
			}
			g.st = st
			g.mu.Unlock()
			log.Info().Str("service", g.front).Msg("sessions: the Well is connected")
			return
		}
		if refused(err) {
			log.Error().Err(err).Str("service", g.front).
				Msg("sessions: the Well answers and refuses this front; no session is accepted until that is fixed")
			wait = maxWait
			continue
		}
		wait = min(wait*2, maxWait)
	}
}

func (g *gate) store() Store {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.st
}

func (g *gate) Create(ctx context.Context, s Session, maxLive int) error {
	if st := g.store(); st != nil {
		return st.Create(ctx, s, maxLive)
	}
	return ErrUnavailable
}

func (g *gate) Get(ctx context.Context, keyHash string) (Session, bool, error) {
	if st := g.store(); st != nil {
		return st.Get(ctx, keyHash)
	}
	return Session{}, false, ErrUnavailable
}

func (g *gate) Touch(ctx context.Context, keyHash string, now, expires time.Time) (bool, error) {
	if st := g.store(); st != nil {
		return st.Touch(ctx, keyHash, now, expires)
	}
	return false, ErrUnavailable
}

func (g *gate) Revoke(ctx context.Context, keyHash string, now time.Time) error {
	if st := g.store(); st != nil {
		return st.Revoke(ctx, keyHash, now)
	}
	return ErrUnavailable
}

func (g *gate) Sweep(ctx context.Context, endedBefore time.Time, limit int) (int, error) {
	if st := g.store(); st != nil {
		return st.Sweep(ctx, endedBefore, limit)
	}
	return 0, ErrUnavailable
}

// Close stops the wait and closes the store if there is one.
func (g *gate) Close() error {
	g.stopOnce.Do(func() { close(g.stop) })
	<-g.done
	g.mu.Lock()
	st := g.st
	g.st = nil
	g.mu.Unlock()
	if st != nil {
		return st.Close()
	}
	return nil
}
