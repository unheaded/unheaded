// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package tls

import "crypto/tls"

// Hybrid ML-KEM key-exchange profiles (ADR-105 decision 1).
//
// A hybrid group combines classical ECDH with ML-KEM (FIPS 203), so a
// recorded session stays confidential if either holds. ML-KEM exists only
// in TLS 1.3, so both profiles floor at 1.3.
//
//   - Strict: TLS 1.3, hybrid groups only. A classical-only peer fails the
//     handshake. For links where both ends are Unheaded Go binaries
//     (pkg/transport/mtls, pkg/mesh, pkg/mesh/mtls, wotan-client to Wotan gRPC).
//   - Preferred: TLS 1.3, CurvePreferences nil (Go's default: hybrids first,
//     classical fallback). For servers third-party clients reach (Wotan HTTP
//     and /metrics, the dashboard, load-balancer frontends).
//
// Both raise a weaker setting, never lower a stronger one.

// StrictKEX returns the Strict profile's groups, preferred first. The slice
// is fresh on every call.
func StrictKEX() []tls.CurveID {
	return []tls.CurveID{tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024}
}

// IsHybrid reports whether id is a hybrid (classical + ML-KEM) group. A pure
// ML-KEM group is post-quantum but not hybrid, and returns false.
func IsHybrid(id tls.CurveID) bool {
	switch id {
	case tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024:
		return true
	}
	return false
}

// ApplyStrict sets the Strict profile on cfg and returns it. A nil cfg gets
// a new Config.
func ApplyStrict(cfg *tls.Config) *tls.Config {
	cfg = floor13(cfg)
	cfg.CurvePreferences = StrictKEX()
	return cfg
}

// ApplyPreferred sets the Preferred profile on cfg and returns it: TLS 1.3
// and Go's default group list (any classical pin is removed). A nil cfg gets
// a new Config.
func ApplyPreferred(cfg *tls.Config) *tls.Config {
	cfg = floor13(cfg)
	cfg.CurvePreferences = nil
	return cfg
}

// floor13 raises MinVersion to TLS 1.3 and lifts a MaxVersion cap below it
// (a cap below 1.3 would rule out ML-KEM, or every handshake).
func floor13(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		cfg = &tls.Config{MinVersion: tls.VersionTLS13}
	}
	if cfg.MinVersion < tls.VersionTLS13 {
		cfg.MinVersion = tls.VersionTLS13
	}
	if cfg.MaxVersion != 0 && cfg.MaxVersion < tls.VersionTLS13 {
		cfg.MaxVersion = tls.VersionTLS13
	}
	return cfg
}
