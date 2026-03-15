// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package rotation handles automatic certificate rotation.
package rotation

import (
	"sync"
	"time"
)

// Config configures certificate rotation.
type Config struct {
	// CheckInterval is how often to check for expiring certs.
	CheckInterval time.Duration
	// RenewBefore is how long before expiry to renew.
	RenewBefore time.Duration
	// MaxRetries is the maximum renewal attempts.
	MaxRetries int
	// RetryDelay is the delay between retries.
	RetryDelay time.Duration
}

// DefaultConfig returns default rotation configuration.
func DefaultConfig() *Config {
	return &Config{
		CheckInterval: 5 * time.Minute,
		RenewBefore:   15 * time.Minute,
		MaxRetries:    3,
		RetryDelay:    30 * time.Second,
	}
}

// RenewCallback is called when a certificate needs renewal.
type RenewCallback func(serial string)

// TrackedCert tracks a certificate for rotation.
type TrackedCert struct {
	Serial    string
	ExpiresAt time.Time
	Retries   int
}

// Rotator handles automatic certificate rotation.
type Rotator struct {
	config       *Config
	callback     RenewCallback
	tracked      map[string]*TrackedCert
	mu           sync.RWMutex
	started      bool
}

// New creates a new certificate rotator.
func New(config *Config, callback RenewCallback) *Rotator {
	if config == nil {
		config = DefaultConfig()
	}

	return &Rotator{
		config:   config,
		callback: callback,
		tracked:  make(map[string]*TrackedCert),
	}
}

// Start begins the rotation loop.
func (r *Rotator) Start(stopCh <-chan struct{}) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	ticker := time.NewTicker(r.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			r.checkAndRenew()
		}
	}
}

// Register adds a certificate for rotation tracking.
func (r *Rotator) Register(serial string, expiresAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tracked[serial] = &TrackedCert{
		Serial:    serial,
		ExpiresAt: expiresAt,
		Retries:   0,
	}
}

// Unregister removes a certificate from rotation tracking.
func (r *Rotator) Unregister(serial string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tracked, serial)
}

// GetTracked returns all tracked certificates.
func (r *Rotator) GetTracked() []*TrackedCert {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*TrackedCert, 0, len(r.tracked))
	for _, cert := range r.tracked {
		result = append(result, &TrackedCert{
			Serial:    cert.Serial,
			ExpiresAt: cert.ExpiresAt,
			Retries:   cert.Retries,
		})
	}
	return result
}

// checkAndRenew checks for expiring certificates and renews them.
func (r *Rotator) checkAndRenew() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	renewThreshold := now.Add(r.config.RenewBefore)

	for serial, cert := range r.tracked {
		if cert.ExpiresAt.Before(renewThreshold) {
			if cert.Retries >= r.config.MaxRetries {
				// Max retries reached, stop tracking
				delete(r.tracked, serial)
				continue
			}

			// Trigger renewal callback
			cert.Retries++
			go r.callback(serial)
		}
	}
}

// NeedsRenewal returns true if a certificate needs renewal.
func (r *Rotator) NeedsRenewal(serial string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cert, ok := r.tracked[serial]
	if !ok {
		return false
	}

	return time.Now().Add(r.config.RenewBefore).After(cert.ExpiresAt)
}

// TimeToRenewal returns when a certificate will be renewed.
func (r *Rotator) TimeToRenewal(serial string) time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cert, ok := r.tracked[serial]
	if !ok {
		return 0
	}

	renewTime := cert.ExpiresAt.Add(-r.config.RenewBefore)
	return time.Until(renewTime)
}
