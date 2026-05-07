// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package rotation

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrRenewalFailed indicates renewal failed.
	ErrRenewalFailed = errors.New("certificate renewal failed")
	// ErrMaxRetriesExceeded indicates maximum retries were exceeded.
	ErrMaxRetriesExceeded = errors.New("maximum renewal retries exceeded")
)

// RenewalResult contains the result of a renewal attempt.
type RenewalResult struct {
	Serial    string
	Success   bool
	NewSerial string
	Error     error
	Retries   int
	RenewedAt time.Time
}

// RenewFunc is a function that performs certificate renewal.
type RenewFunc func(ctx context.Context, serial string) (newSerial string, err error)

// Renewer handles certificate renewal with retry logic.
type Renewer struct {
	config     *Config
	renewFunc  RenewFunc
	inProgress map[string]*RenewalState
	mu         sync.Mutex
}

// RenewalState tracks the state of a renewal.
type RenewalState struct {
	Serial    string
	StartedAt time.Time
	Retries   int
	LastError error
}

// NewRenewer creates a new certificate renewer.
func NewRenewer(config *Config, renewFunc RenewFunc) *Renewer {
	if config == nil {
		config = DefaultConfig()
	}

	return &Renewer{
		config:     config,
		renewFunc:  renewFunc,
		inProgress: make(map[string]*RenewalState),
	}
}

// Renew attempts to renew a certificate with retries.
func (r *Renewer) Renew(ctx context.Context, serial string) *RenewalResult {
	r.mu.Lock()

	// Check if already in progress
	if state, ok := r.inProgress[serial]; ok {
		r.mu.Unlock()
		return &RenewalResult{
			Serial:  serial,
			Success: false,
			Error:   errors.New("renewal already in progress"),
			Retries: state.Retries,
		}
	}

	// Mark as in progress
	state := &RenewalState{
		Serial:    serial,
		StartedAt: time.Now(),
		Retries:   0,
	}
	r.inProgress[serial] = state
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.inProgress, serial)
		r.mu.Unlock()
	}()

	// Attempt renewal with retries
	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		state.Retries = attempt

		newSerial, err := r.renewFunc(ctx, serial)
		if err == nil {
			return &RenewalResult{
				Serial:    serial,
				Success:   true,
				NewSerial: newSerial,
				Retries:   attempt,
				RenewedAt: time.Now(),
			}
		}

		lastErr = err
		state.LastError = err

		if attempt < r.config.MaxRetries {
			select {
			case <-ctx.Done():
				return &RenewalResult{
					Serial:  serial,
					Success: false,
					Error:   ctx.Err(),
					Retries: attempt,
				}
			case <-time.After(r.config.RetryDelay):
				// Continue to next retry
			}
		}
	}

	return &RenewalResult{
		Serial:  serial,
		Success: false,
		Error:   lastErr,
		Retries: r.config.MaxRetries,
	}
}

// IsInProgress returns true if a renewal is in progress.
func (r *Renewer) IsInProgress(serial string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.inProgress[serial]
	return ok
}

// GetState returns the state of an in-progress renewal.
func (r *Renewer) GetState(serial string) *RenewalState {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.inProgress[serial]
	if !ok {
		return nil
	}

	// Return a copy
	return &RenewalState{
		Serial:    state.Serial,
		StartedAt: state.StartedAt,
		Retries:   state.Retries,
		LastError: state.LastError,
	}
}

// BatchRenewer handles batch certificate renewals.
type BatchRenewer struct {
	renewer     *Renewer
	concurrency int
}

// NewBatchRenewer creates a new batch renewer.
func NewBatchRenewer(renewer *Renewer, concurrency int) *BatchRenewer {
	if concurrency <= 0 {
		concurrency = 5
	}
	return &BatchRenewer{
		renewer:     renewer,
		concurrency: concurrency,
	}
}

// RenewAll renews multiple certificates concurrently.
func (b *BatchRenewer) RenewAll(ctx context.Context, serials []string) []*RenewalResult {
	results := make([]*RenewalResult, len(serials))
	var wg sync.WaitGroup
	sem := make(chan struct{}, b.concurrency)

	for i, serial := range serials {
		wg.Add(1)
		go func(idx int, s string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = b.renewer.Renew(ctx, s)
		}(i, serial)
	}

	wg.Wait()
	return results
}
